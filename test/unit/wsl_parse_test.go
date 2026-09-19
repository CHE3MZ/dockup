package unit

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"

	"github.com/CHE3MZ/dockup/internal/wsl"
)

func utf16LE(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2+len(u)*2)
	b[0], b[1] = 0xFF, 0xFE
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[2+i*2:], v)
	}
	return b
}

func TestParseListUTF16(t *testing.T) {
	got := wsl.ParseListOutput(utf16LE("dockup\r\nUbuntu\r\n"))
	if len(got) != 2 || got[0] != "dockup" || got[1] != "Ubuntu" {
		t.Fatalf("got %q", got)
	}
}

func TestParseListUTF8(t *testing.T) {
	got := wsl.ParseListOutput([]byte("dockup\nUbuntu\n\n"))
	if len(got) != 2 {
		t.Fatalf("got %q", got)
	}
}

func TestParseListEmpty(t *testing.T) {
	if got := wsl.ParseListOutput([]byte("")); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}
