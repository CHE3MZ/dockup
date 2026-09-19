package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CHE3MZ/dockup/internal/userconfig"
)

func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	t.Setenv("HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)
	return dir
}

func TestEnsureCreatesBareMinimum(t *testing.T) {
	withTempHome(t)
	c, err := userconfig.Ensure()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != userconfig.DefaultPort {
		t.Fatalf("port = %d", c.Port)
	}
	if c.UseTCP {
		t.Fatal("use_tcp should default false")
	}
	if c.PipeName != userconfig.DefaultPipeName {
		t.Fatalf("pipe = %q", c.PipeName)
	}
	if !c.Color {
		t.Fatal("color should default true")
	}
	if c.DefaultPath == "" {
		t.Fatal("default_path should be prefilled")
	}
	if _, err := os.Stat(userconfig.File()); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	// Second Ensure keeps existing values.
	c.DefaultPath = "D:/WSL"
	if err := userconfig.Save(c); err != nil {
		t.Fatal(err)
	}
	kept, err := userconfig.Ensure()
	if err != nil {
		t.Fatal(err)
	}
	if kept.DefaultPath != "D:/WSL" {
		t.Fatalf("default_path = %q", kept.DefaultPath)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withTempHome(t)
	want := userconfig.Config{
		DefaultPath: "Z:/WSL/distros",
		CurrentPath: "F:/WSL/test",
		Port:        2376,
		UseTCP:      true,
		PipeName:    `\\.\pipe\dockup_engine`,
		Color:       false,
	}
	if err := userconfig.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	withTempHome(t)
	c, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != userconfig.DefaultPort || c.PipeName == "" {
		t.Fatalf("got %+v", c)
	}
}

func TestLoadCorruptErrors(t *testing.T) {
	withTempHome(t)
	if err := os.MkdirAll(userconfig.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userconfig.File(), []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := userconfig.Load(); err == nil {
		t.Fatal("expected error for corrupt config")
	}
}

func TestNormalizePath(t *testing.T) {
	got, err := userconfig.NormalizePath("D:/WSL")
	if err != nil || got != "D:/WSL" {
		t.Fatalf("got %q %v", got, err)
	}
	got, err = userconfig.NormalizePath(`  "Z:\WSL\distros\"  `)
	if err != nil || got != "Z:/WSL/distros" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := userconfig.NormalizePath("relative/dir"); err == nil {
		t.Fatal("relative path should error")
	}
	if _, err := userconfig.NormalizePath("   "); err == nil {
		t.Fatal("empty path should error")
	}
}

func TestResolveInstallDirFlagWins(t *testing.T) {
	withTempHome(t)
	cfg, _ := userconfig.Ensure()
	cfg.DefaultPath = "Z:/WSL/distros"
	got, err := userconfig.ResolveInstallDir("F:/WSL/test", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.ToSlash(got) != "F:/WSL/test" {
		t.Fatalf("got %q", got)
	}
	got, err = userconfig.ResolveInstallDir("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.ToSlash(got) != "Z:/WSL/distros" {
		t.Fatalf("got %q", got)
	}
}

func TestValidatePort(t *testing.T) {
	for _, p := range []int{1, 2375, 2376, 65535} {
		if err := userconfig.ValidatePort(p); err != nil {
			t.Fatalf("port %d: %v", p, err)
		}
	}
	for _, p := range []int{0, -1, 65536, 99999} {
		if err := userconfig.ValidatePort(p); err == nil {
			t.Fatalf("port %d should error", p)
		}
	}
}

func TestTCPAddrLoopbackOnly(t *testing.T) {
	c := userconfig.Defaults()
	c.Port = 2376
	if got := c.TCPAddr(); got != "127.0.0.1:2376" {
		t.Fatalf("got %q", got)
	}
}

func TestEffectivePipeDefaultAndCustom(t *testing.T) {
	c := userconfig.Defaults()
	if c.EffectivePipe() != userconfig.DefaultPipeName {
		t.Fatalf("got %q", c.EffectivePipe())
	}
	c.PipeName = `\\.\pipe\custom_engine`
	if c.EffectivePipe() != `\\.\pipe\custom_engine` {
		t.Fatalf("got %q", c.EffectivePipe())
	}
}

func TestInstalledDefaultsFalse(t *testing.T) {
	withTempHome(t)
	c, err := userconfig.Ensure()
	if err != nil {
		t.Fatal(err)
	}
	if c.Installed {
		t.Fatal("installed should default to false")
	}
}

func TestInstalledRoundTrip(t *testing.T) {
	withTempHome(t)
	c := userconfig.Defaults()
	c.Installed = true
	if err := userconfig.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Installed {
		t.Fatal("installed not persisted")
	}
}

func TestReconcileAdoptsButNeverClears(t *testing.T) {
	withTempHome(t)
	if _, err := userconfig.Ensure(); err != nil {
		t.Fatal(err)
	}
	changed, err := userconfig.ReconcileInstalled(true)
	if err != nil || !changed {
		t.Fatalf("expected flip, got changed=%v err=%v", changed, err)
	}
	changed, err = userconfig.ReconcileInstalled(true)
	if err != nil || changed {
		t.Fatalf("expected no-op, got changed=%v err=%v", changed, err)
	}
	// false must never clear a true flag (no false-positive wipeouts).
	if _, err := userconfig.ReconcileInstalled(false); err != nil {
		t.Fatal(err)
	}
	got, err := userconfig.Load()
	if err != nil || !got.Installed {
		t.Fatalf("flag was cleared: %+v %v", got, err)
	}
}
