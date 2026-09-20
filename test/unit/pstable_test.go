package unit

import (
	"strings"
	"testing"

	"github.com/CHE3MZ/dockup/internal/pstable"
	"github.com/CHE3MZ/dockup/internal/ui"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name              string
		pipe, inst, ready bool
		want              string
	}{
		{"stopped when no pipe", false, false, false, pstable.StatusStopped},
		{"stopped when no pipe even if installed", false, true, false, pstable.StatusStopped},
		{"stopped on foreign pipe", true, false, false, pstable.StatusStopped},
		{"stopped on foreign pipe even if pingable", true, false, true, pstable.StatusStopped},
		{"starting when engine not ready", true, true, false, pstable.StatusStarting},
		{"running when engine ready", true, true, true, pstable.StatusRunning},
	}
	for _, c := range cases {
		if got := pstable.Classify(c.pipe, c.inst, c.ready); got != c.want {
			t.Fatalf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestAutostartText(t *testing.T) {
	if pstable.AutostartText(true) != "on" || pstable.AutostartText(false) != "off" {
		t.Fatal("bad autostart text")
	}
}

func TestRenderTable(t *testing.T) {
	ui.SetEnabled(true)
	defer ui.SetEnabled(false)
	out := pstable.RenderRow(pstable.Row{
		Status:    pstable.StatusStarting,
		Autostart: pstable.AutostartText(true),
		Installed: pstable.InstalledText(true),
		Size:      "452 MB",
		Memory:    "128 MB",
	})
	for _, want := range []string{"STATUS", "AUTOSTART", "INSTALLED", "SIZE", "MEMORY"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing bold header %s: %q", want, out)
		}
	}
	if !strings.Contains(out, "\x1b[1m") {
		t.Fatalf("headers not bold: %q", out)
	}
	for _, want := range []string{"starting...", "on", "yes", "452 MB", "128 MB"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing value %s: %q", want, out)
		}
	}
	// Values must be plain (non-bold): one bold sequence per header.
	if n := strings.Count(out, "\x1b[1m"); n != 5 {
		t.Fatalf("expected 5 bold headers, got %d: %q", n, out)
	}
}

func TestRenderStoppedOff(t *testing.T) {
	ui.SetEnabled(false)
	out := pstable.RenderRow(pstable.Row{
		Status:    pstable.StatusStopped,
		Autostart: pstable.AutostartText(false),
		Installed: pstable.InstalledText(false),
		Size:      "-",
		Memory:    "-",
	})
	for _, want := range []string{"STATUS", "stopped", "off", "no"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s: %q", want, out)
		}
	}
}

func TestInstalledText(t *testing.T) {
	if pstable.InstalledText(true) != "yes" || pstable.InstalledText(false) != "no" {
		t.Fatal("bad installed text")
	}
}
