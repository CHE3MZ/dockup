package unit

import (
	"testing"

	"github.com/CHE3MZ/dockup/internal/wsl"
)

func TestParseListOutputUTF16(t *testing.T) {
	// "Debian\nUbuntu\n" as UTF-16LE with BOM.
	raw := []byte{0xFF, 0xFE}
	for _, r := range "Debian\nUbuntu\n" {
		raw = append(raw, byte(r), 0x00)
	}
	got := wsl.ParseListOutput(raw)
	if len(got) != 2 || got[0] != "Debian" || got[1] != "Ubuntu" {
		t.Fatalf("got %q", got)
	}
}

func TestParseListOutputUTF8(t *testing.T) {
	got := wsl.ParseListOutput([]byte("Debian\nAlpine\n"))
	if len(got) != 2 {
		t.Fatalf("got %q", got)
	}
}
