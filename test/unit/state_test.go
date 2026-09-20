package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/CHE3MZ/dockup/internal/state"
)

func withTempAppData(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("TEMP", dir)
	t.Setenv("TMP", dir)
}

func TestStateRoundTrip(t *testing.T) {
	withTempAppData(t)
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Installed {
		t.Fatal("fresh state should not be installed")
	}
	s.Installed = true
	s.Arch = "amd64"
	if err := state.Save(s); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Installed || loaded.Arch != "amd64" {
		t.Fatalf("got %+v", loaded)
	}
}

func TestStateWithLock(t *testing.T) {
	withTempAppData(t)
	if err := state.WithLock(func(s *state.State) error {
		s.Installed = true
		s.Arch = "arm64"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s, _ := state.Load()
	if !s.Installed || s.Arch != "arm64" {
		t.Fatalf("got %+v", s)
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "x")); !os.IsNotExist(err) {
		t.Fatal("unexpected")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	withTempAppData(t)
	s := state.State{
		Installed: true,
		Arch:      "amd64",
		Snapshot:  state.Snapshot{At: "2026-01-01T00:00:00Z", Manual: []string{"curl", "docker-ce"}},
	}
	if err := state.Save(s); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Snapshot.Manual) != 2 || loaded.Snapshot.Manual[0] != "curl" {
		t.Fatalf("got %+v", loaded.Snapshot)
	}
	// Old state files without a snapshot must load as empty, not error.
	legacy := state.State{Installed: true}
	if len(legacy.Snapshot.Manual) != 0 {
		t.Fatal("fresh snapshot should be empty")
	}
}
