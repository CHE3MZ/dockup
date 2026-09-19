package unit

import (
	"strings"
	"testing"

	"github.com/CHE3MZ/dockup/internal/config"
)

func TestArchTarName(t *testing.T) {
	if got := config.ArchTarName("amd64"); !strings.Contains(got, "amd64") {
		t.Fatalf("amd64 tar = %q", got)
	}
	if got := config.ArchTarName("arm64"); !strings.Contains(got, "arm64") {
		t.Fatalf("arm64 tar = %q", got)
	}
}

func TestDebianURLs(t *testing.T) {
	u := config.RootfsURL("amd64")
	if !strings.Contains(u, "dist-amd64") || !strings.Contains(u, "bookworm") {
		t.Fatalf("primary url = %q", u)
	}
	if !strings.HasSuffix(u, "rootfs.tar.gz") {
		t.Fatalf("primary url suffix = %q", u)
	}
	f := config.RootfsFallbackURL("arm64")
	if !strings.Contains(f, "dist-arm64v8") {
		t.Fatalf("fallback url = %q", f)
	}
	// Compat wrappers follow the same sources.
	if config.DebianURL("amd64") != u {
		t.Fatalf("DebianURL compat mismatch")
	}
	if config.FallbackURL("arm64") != f {
		t.Fatalf("FallbackURL compat mismatch")
	}
}

func TestNormalizeArch(t *testing.T) {
	a, msg := config.NormalizeArch(false, false)
	if msg != "" || a != "amd64" {
		t.Fatalf("default = %q %q", a, msg)
	}
	a, msg = config.NormalizeArch(true, false)
	if msg != "" || a != "amd64" {
		t.Fatalf("amd = %q %q", a, msg)
	}
	a, msg = config.NormalizeArch(false, true)
	if msg != "" || a != "arm64" {
		t.Fatalf("arm = %q %q", a, msg)
	}
	if _, msg := config.NormalizeArch(true, true); msg == "" {
		t.Fatalf("both flags should error")
	}
}

func TestPipeName(t *testing.T) {
	if config.PipeName != `\\.\pipe\docker_engine` {
		t.Fatalf("pipe = %q", config.PipeName)
	}
	if config.DistroName != "dockup" {
		t.Fatalf("distro = %q", config.DistroName)
	}
}
