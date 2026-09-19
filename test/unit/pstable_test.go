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
	out := pstable.Render(pstable.StatusStarting, true)
	if !strings.Contains(out, "STATUS") || !strings.Contains(out, "AUTOSTART") {
		t.Fatalf("missing bold headers: %q", out)
	}
	if !strings.Contains(out, "\x1b[1m") {
		t.Fatalf("headers not bold: %q", out)
	}
	if !strings.Contains(out, "starting...") || !strings.Contains(out, "on") {
		t.Fatalf("missing values: %q", out)
	}
	// Values must be plain (non-bold): only two bold sequences (headers).
	if n := strings.Count(out, "\x1b[1m"); n != 2 {
		t.Fatalf("expected 2 bold headers, got %d: %q", n, out)
	}
}

func TestRenderStoppedOff(t *testing.T) {
	ui.SetEnabled(false)
	out := pstable.Render(pstable.StatusStopped, false)
	if !strings.Contains(out, "STATUS") || !strings.Contains(out, "stopped") || !strings.Contains(out, "off") {
		t.Fatalf("got %q", out)
	}
}
