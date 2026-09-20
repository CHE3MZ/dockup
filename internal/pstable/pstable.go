// Package pstable renders `dockup ps` as a table with bold headers and
// plain values:
//
//	STATUS      AUTOSTART   INSTALLED   SIZE      MEMORY
//	running     off         yes         452 MB    128 MB
package pstable

import (
	"fmt"
	"strings"

	"github.com/CHE3MZ/dockup/internal/ui"
)

const (
	StatusRunning  = "running"
	StatusStopped  = "stopped"
	StatusStarting = "starting..."
)

// Classify maps probe results to a ps status.
// pipeAlive=false -> stopped. Foreign holder (pipe alive but dockup never
// installed it) -> stopped (caller adds the hint line). Pipe alive without
// an answering engine -> starting.... Otherwise running.
func Classify(pipeAlive, installed, engineReady bool) string {
	if !pipeAlive {
		return StatusStopped
	}
	if !installed {
		return StatusStopped
	}
	if !engineReady {
		return StatusStarting
	}
	return StatusRunning
}

// AutostartText renders the config flag.
func AutostartText(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// InstalledText renders whether the distro is installed.
func InstalledText(installed bool) string {
	if installed {
		return "yes"
	}
	return "no"
}

// Row is one ps table body line. Size and Memory carry their own unit
// ("452 MB", "1.2 GB") or "-" when unknown; see sysinfo.FormatSize.
type Row struct {
	Status    string
	Autostart string
	Installed string
	Size      string
	Memory    string
}

// RenderRow builds the table. Headers are bold, values are plain.
func RenderRow(r Row) string {
	headers := []string{"STATUS", "AUTOSTART", "INSTALLED", "SIZE", "MEMORY"}
	values := []string{r.Status, r.Autostart, r.Installed, r.Size, r.Memory}
	widths := make([]int, len(headers))
	for i := range headers {
		widths[i] = max(len(headers[i]), len(values[i])) + 4
	}
	var b strings.Builder
	for i, h := range headers {
		b.WriteString(ui.Bold(fmt.Sprintf("%-*s", widths[i], h)))
	}
	b.WriteString("\n")
	for i, v := range values {
		_, _ = fmt.Fprintf(&b, "%-*s", widths[i], v)
	}
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
