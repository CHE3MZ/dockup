package unit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CHE3MZ/dockup/internal/state"
)

func TestTransactionStaleLockRecovered(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, "state.lock")
	if err := os.WriteFile(lock, []byte("999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if err := state.Transaction(func(s *state.State) error {
		s.Default = "Debian"
		return nil
	}); err != nil {
		t.Fatalf("stale lock should be recovered: %v", err)
	}
	s, _ := state.Load()
	if s.Default != "Debian" {
		t.Fatalf("default = %q", s.Default)
	}
}
