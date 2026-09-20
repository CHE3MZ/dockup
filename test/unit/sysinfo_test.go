package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CHE3MZ/dockup/internal/sysinfo"
)

func TestFormatSize(t *testing.T) {
	cases := map[uint64]string{
		0:                       "0 MB",
		1 << 20:                 "1 MB",
		452 << 20:               "452 MB",
		(1 << 30) - 1:           "1023 MB",
		1 << 30:                 "1.0 GB",
		(5 << 30) + (512 << 20): "5.5 GB",
	}
	for in, want := range cases {
		if got := sysinfo.FormatSize(in); got != want {
			t.Fatalf("FormatSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestDirSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.bin"), make([]byte, 156), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := sysinfo.DirSize(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != 256 {
		t.Fatalf("got %d, want 256", got)
	}
	if _, err := sysinfo.DirSize(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error for missing dir")
	}
}

func TestDistroRSSUnreachable(t *testing.T) {
	if _, err := sysinfo.DistroRSS("definitely-not-a-distro-xyz"); err == nil {
		t.Fatal("expected error for missing distro")
	}
}
