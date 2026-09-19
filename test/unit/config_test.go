package unit

import (
	"strings"
	"testing"

	"github.com/CHE3MZ/dockup/internal/config"
)

func TestArchTarName(t *testing.T) {
	if got := config.ArchTarName("amd64"); got != "debian-12-nocloud-amd64.tar.xz" {
		t.Fatalf("amd64 tar = %q", got)
	}
	if got := config.ArchTarName("arm64"); got != "debian-12-nocloud-arm64.tar.xz" {
		t.Fatalf("arm64 tar = %q", got)
	}
}

func TestDebianURLs(t *testing.T) {
	u := config.DebianURL("amd64")
	if !strings.HasPrefix(u, "https://cloud.debian.org/images/cloud/bookworm/latest/") {
		t.Fatalf("primary url = %q", u)
	}
	if !strings.HasSuffix(u, ".tar.xz") {
		t.Fatalf("primary url suffix = %q", u)
	}
	f := config.FallbackURL("arm64")
	if !strings.Contains(f, "cdimage.debian.org") {
		t.Fatalf("fallback url = %q", f)
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
