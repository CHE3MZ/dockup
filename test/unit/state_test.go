package unit

import (
	"errors"
	"testing"

	"github.com/CHE3MZ/dockup/internal/state"
)

func TestTransactionRoundTrip(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if err := state.Transaction(func(s *state.State) error {
		s.Distros["Debian"] = state.DistroState{}
		s.Default = "Debian"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Default != "Debian" {
		t.Fatalf("default = %q", s.Default)
	}
	if err := state.Transaction(func(s *state.State) error {
		s.ActiveDistro = "Debian"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s, _ = state.Load()
	if s.ActiveDistro != "Debian" {
		t.Fatalf("active = %q", s.ActiveDistro)
	}
}

func TestTransactionAbortKeepsState(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	if err := state.Transaction(func(s *state.State) error {
		s.Default = "X"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := state.Transaction(func(s *state.State) error {
		s.Default = "Y"
		return errBoom
	})
	if err != errBoom {
		t.Fatalf("expected abort error, got %v", err)
	}
	s, _ := state.Load()
	if s.Default != "X" {
		t.Fatalf("aborted txn leaked: default = %q", s.Default)
	}
}

var errBoom = errors.New("boom")
