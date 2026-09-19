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
