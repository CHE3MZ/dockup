// Package pstable renders `dockup ps` as a two-column table with bold
// headers and plain values:
//
//	STATUS      AUTOSTART
//	running     off
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

// Render builds the table. Headers are bold, values are plain.
func Render(status string, autostartOn bool) string {
	h1, h2 := "STATUS", "AUTOSTART"
	v2 := AutostartText(autostartOn)
	w1 := max(len(h1), len(status)) + 4
	w2 := max(len(h2), len(v2)) + 2
	var b strings.Builder
	b.WriteString(ui.Bold(fmt.Sprintf("%-*s", w1, h1)))
	b.WriteString(ui.Bold(fmt.Sprintf("%-*s", w2, h2)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("%-*s", w1, status))
	b.WriteString(fmt.Sprintf("%-*s", w2, v2))
	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
