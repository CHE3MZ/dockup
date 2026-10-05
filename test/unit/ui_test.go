package unit

import (
	"strings"
	"testing"

	"github.com/CHE3MZ/dockup/internal/ui"
)

func TestPaletteCodes(t *testing.T) {
	ui.SetEnabled(true)
	defer ui.SetEnabled(false)
	cases := map[string]func(string) string{
		"37": ui.White,
		"94": ui.LightBlue,
		"90": ui.Gray,
		"33": ui.Yellow,
		"31": ui.Red,
		"32": ui.Green,
	}
	for code, fn := range cases {
		got := fn("x")
		if !strings.Contains(got, "\x1b["+code+"m") || !strings.Contains(got, "\x1b[0m") {
			t.Fatalf("code %s: got %q", code, got)
		}
	}
	if got := ui.Bold("x"); !strings.Contains(got, "\x1b[1m") {
		t.Fatalf("bold: got %q", got)
	}
}

func TestDisabledIsPlain(t *testing.T) {
	ui.SetEnabled(false)
	if got := ui.Red("oops"); got != "oops" {
		t.Fatalf("got %q", got)
	}
	if got := ui.Header("h"); got != "h" {
		t.Fatalf("got %q", got)
	}
}

func TestSpinnerNonInteractiveLines(t *testing.T) {
	prev := ui.Enabled()
	ui.SetEnabled(false)
	defer ui.SetEnabled(prev)
	var b strings.Builder
	sp := ui.NewSpinner(&b, "working")
	sp.Start()
	sp.Done()
	out := b.String()
	if strings.Contains(out, "\r") {
		t.Fatalf("non-interactive spinner must not use carriage returns: %q", out)
	}
	if !strings.Contains(out, "working\n") || !strings.Contains(out, "working done\n") {
		t.Fatalf("want start + done lines, got %q", out)
	}
}

func TestSpinnerStopSkipsDone(t *testing.T) {
	prev := ui.Enabled()
	ui.SetEnabled(false)
	defer ui.SetEnabled(prev)
	var b strings.Builder
	sp := ui.NewSpinner(&b, "working")
	sp.Start()
	sp.Stop()
	if strings.Contains(b.String(), "done") {
		t.Fatalf("stopped spinner must not print done: %q", b.String())
	}
}

func TestSpinnerDoneOnce(t *testing.T) {
	prev := ui.Enabled()
	ui.SetEnabled(false)
	defer ui.SetEnabled(prev)
	var b strings.Builder
	sp := ui.NewSpinner(&b, "working")
	sp.Start()
	sp.Done()
	sp.Done()
	if n := strings.Count(b.String(), "done"); n != 1 {
		t.Fatalf("done printed %d times, want 1: %q", n, b.String())
	}
}
