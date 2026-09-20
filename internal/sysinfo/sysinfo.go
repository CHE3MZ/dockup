// Package sysinfo reports distro disk usage and process memory for ps.
// FormatSize and DirSize are portable; DockupWindowsRSS is implemented per
// OS (real measurement on Windows, zero elsewhere); DistroRSS sums the
// RSS of dockup-owned processes inside the distro via wsl.exe.
package sysinfo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/wsl"
)

// FormatSize renders bytes as whole MB below 1 GiB, else one-decimal GB.
func FormatSize(b uint64) string {
	const gib = 1 << 30
	if b < gib {
		return fmt.Sprintf("%d MB", b/(1<<20))
	}
	return fmt.Sprintf("%.1f GB", float64(b)/float64(gib))
}

// DirSize sums file sizes under dir (no file contents are read).
func DirSize(dir string) (uint64, error) {
	var total uint64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		size := info.Size()
		if !info.IsDir() && size > 0 {
			total += uint64(size)
		}
		return nil
	})
	return total, err
}

// DockupWindowsRSS sums the working-set bytes of all dockup.exe processes
// except the caller, via tasklist CSV (inbox on Windows, no new
// dependencies). The memory figure is digits-only parsed, so it is immune
// to locale thousand separators. Zero when none run or tasklist is absent.
func DockupWindowsRSS() uint64 {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tasklist",
		"/FI", "IMAGENAME eq dockup.exe", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return 0
	}
	self := os.Getpid()
	var total uint64
	for _, line := range strings.Split(string(out), "\n") {
		f := splitCSV(strings.TrimSpace(line))
		if len(f) < 5 || !strings.EqualFold(f[0], "dockup.exe") {
			continue
		}
		pid, err := strconv.Atoi(f[1])
		if err != nil || pid == self {
			continue
		}
		kb, err := strconv.ParseUint(digitsOnly(f[4]), 10, 64)
		if err != nil {
			continue
		}
		total += kb * 1024
	}
	return total
}

// splitCSV splits one tasklist CSV line on commas outside quotes.
func splitCSV(line string) []string {
	var fields []string
	var cur strings.Builder
	inQuotes := false
	for _, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case r == ',' && !inQuotes:
			fields = append(fields, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	fields = append(fields, cur.String())
	return fields
}

// digitsOnly keeps 0-9, dropping locale separators and the " K" suffix.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// distroProcs are the in-distro processes attributed to dockup.
var distroProcs = map[string]bool{
	"dockerd":    true,
	"containerd": true,
	"socat":      true,
}

// DistroRSS sums RSS (bytes) of dockup-owned processes in the distro.
// Returns an error when the distro is unreachable.
func DistroRSS(distro string) (uint64, error) {
	out, err := wsl.Exec(distro, 15*time.Second, "ps", "-eo", "rss=,comm=")
	if err != nil {
		return 0, err
	}
	var total uint64
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		if !distroProcs[f[1]] {
			continue
		}
		kb, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		total += kb * 1024
	}
	return total, nil
}
