package unit

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/ui"
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	_ = w.Close()
	var b strings.Builder
	_, _ = io.Copy(&b, r)
	return b.String()
}

func TestErrFromAddsPrefix(t *testing.T) {
	prev := ui.Enabled()
	ui.SetEnabled(false)
	defer ui.SetEnabled(prev)
	if got := captureStderr(t, func() { logx.ErrFrom(errors.New("boom")) }); got != "dockup: boom\n" {
		t.Fatalf("got %q", got)
	}
}

func TestErrFromNeverStutters(t *testing.T) {
	prev := ui.Enabled()
	ui.SetEnabled(false)
	defer ui.SetEnabled(prev)
	got := captureStderr(t, func() { logx.ErrFrom(errors.New("dockup is already running")) })
	if got != "dockup is already running\n" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "dockup: dockup") {
		t.Fatalf("stuttering prefix: %q", got)
	}
}
