// Package pstable renders `dockup ps` as a table with bold headers and
// plain values:
//
//	STATUS      AUTOSTART   INSTALLED   SIZE      MEMORY
//	running     off         yes         452 MB    128 MB
package pstable

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/CHE3MZ/dockup/internal/ui"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visibleLen(s string) int { return len(ansi.ReplaceAllString(s, "")) }

// pad aligns a value by its visible width, so color codes do not skew columns.
func pad(s string, width int) string {
	if n := visibleLen(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

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

// coloredStatus paints the one value that is a live signal: green while the
// engine answers, yellow while it boots, gray when stopped (a stopped distro
// is normal, not an error, so never red).
func coloredStatus(s string) string {
	switch s {
	case StatusRunning:
		return ui.Green(s)
	case StatusStarting:
		return ui.Yellow(s)
	default:
		return ui.Gray(s)
	}
}

// RenderRow builds the table. Headers are bold, values plain except STATUS.
func RenderRow(r Row) string {
	headers := []string{"STATUS", "AUTOSTART", "INSTALLED", "SIZE", "MEMORY"}
	values := []string{coloredStatus(r.Status), r.Autostart, r.Installed, r.Size, r.Memory}
	widths := make([]int, len(headers))
	for i := range headers {
		widths[i] = max(len(headers[i]), visibleLen(values[i])) + 4
	}
	var b strings.Builder
	for i, h := range headers {
		b.WriteString(pad(ui.Bold(h), widths[i]))
	}
	b.WriteString("\n")
	for i, v := range values {
		b.WriteString(pad(v, widths[i]))
	}
	return strings.TrimRight(b.String(), " ")
}

// RenderJSON renders the row as indented JSON for scripts (ps --json).
func RenderJSON(r Row) (string, error) {
	data, err := json.MarshalIndent(struct {
		Status    string `json:"status"`
		Autostart string `json:"autostart"`
		Installed string `json:"installed"`
		Size      string `json:"size"`
		Memory    string `json:"memory"`
	}{
		Status:    r.Status,
		Autostart: r.Autostart,
		Installed: r.Installed,
		Size:      r.Size,
		Memory:    r.Memory,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
