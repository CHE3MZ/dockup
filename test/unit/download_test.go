package unit

import (
	"testing"

	"github.com/CHE3MZ/dockup/internal/download"
)

func TestFormatMB(t *testing.T) {
	if got := download.FormatMB(0); got != "0" {
		t.Fatalf("got %q", got)
	}
	if got := download.FormatMB(252 * 1024 * 1024); got != "252" {
		t.Fatalf("got %q", got)
	}
	if got := download.FormatMB(-1); got != "?" {
		t.Fatalf("got %q", got)
	}
}
