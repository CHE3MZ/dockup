// Package wsl wraps wsl.exe interop. All execs have timeouts.
// wsl.exe stdout is UTF-16LE; decode before parsing. Never parse stderr
// for control flow.
package wsl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"
)

// ParseListOutput decodes `wsl --list --quiet` output (UTF-16 or UTF-8)
// into distro names, one per line.
func ParseListOutput(data []byte) []string {
	text := decodeWslOutput(data)
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "\x00\r"))
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func decodeWslOutput(data []byte) string {
	if len(data) >= 2 && ((data[0] == 0xFF && data[1] == 0xFE) || bytes.Contains(data, []byte{0x00})) {
		// UTF-16LE (wsl.exe default). Strip BOM.
		u16 := make([]uint16, 0, len(data)/2)
		start := 0
		if data[0] == 0xFF && data[1] == 0xFE {
			start = 2
		}
		for i := start; i+1 < len(data); i += 2 {
			u16 = append(u16, uint16(data[i])|uint16(data[i+1])<<8)
		}
		return string(utf16.Decode(u16))
	}
	return string(data)
}

func runWsl(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// NOTE: stderr deliberately ignored for control flow.
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}

// List returns all installed distro names via `wsl --list --quiet`.
func List() ([]string, error) {
	out, err := runWsl(context.Background(), "--list", "--quiet")
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return ParseListOutput(out), nil
}

// Running returns the set of currently-running distros.
func Running() (map[string]bool, error) {
	out, err := runWsl(context.Background(), "--list", "--running", "--quiet")
	if err != nil && len(out) == 0 {
		return nil, err
	}
	set := map[string]bool{}
	for _, d := range ParseListOutput(out) {
		set[d] = true
	}
	return set, nil
}

// RunningVerbose is `wsl --list --verbose` raw output for `list` (name+version).
func RunningVerbose() ([]byte, error) {
	return runWsl(context.Background(), "--list", "--verbose")
}

// Exec runs `wsl -d <distro> -u root -- <args...>` with a timeout.
// Always pass -u root explicitly: default user may be non-root.
// stderr is never used for control flow, but IS attached to the error
// so failures are diagnosable (opaque `exit status N` helps nobody).
func Exec(distro string, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	full := append([]string{"-d", distro, "-u", "root", "--"}, args...)
	cmd := exec.CommandContext(ctx, "wsl.exe", full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("wsl -d %s: %w\n%s", distro, err, Tail(stderr.Bytes(), 2000))
	}
	return stdout.Bytes(), nil
}

// Tail returns the last n bytes as string, for error context.
func Tail(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}

// RunRetry runs `sh -c script` as root, retrying transient failures
// (apt/apk mirror hiccups, dpkg locks on fresh images). Progress and the
// failing tail go to stderr so CI logs show what happened.
func RunRetry(distro, what string, timeout time.Duration, tries int, script string) ([]byte, error) {
	var out []byte
	var err error
	for i := 1; i <= tries; i++ {
		out, err = Exec(distro, timeout, "sh", "-c", script)
		if err == nil {
			return out, nil
		}
		fmt.Fprintf(os.Stderr, "dockup: %s attempt %d/%d failed, retrying...\n%s\n", what, i, tries, Tail(out, 1500))
		time.Sleep(time.Duration(i) * 10 * time.Second)
	}
	return out, fmt.Errorf("%s failed after %d tries: %w\n%s", what, tries, err, Tail(out, 4000))
}

// Terminate runs `wsl --terminate <distro>`. Caller must respect the
// booted-by-dockup rule (plan §6) before invoking.
func Terminate(distro string) error {
	_, err := runWsl(context.Background(), "--terminate", distro)
	return err
}
