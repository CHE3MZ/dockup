package unit

import (
	"runtime"
	"testing"

	"github.com/CHE3MZ/dockup/internal/doctor"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/userconfig"
)

// seedInstalledRecord fakes a successful setup: state installed plus a
// current_path pointing somewhere.
func seedInstalledRecord(t *testing.T) {
	t.Helper()
	if err := state.Save(state.State{Installed: true, Arch: "amd64"}); err != nil {
		t.Fatal(err)
	}
	c, err := userconfig.Ensure()
	if err != nil {
		t.Fatal(err)
	}
	c = c.WithDefaults()
	c.CurrentPath = "C:/dockup-test-wsl"
	if err := userconfig.Save(c); err != nil {
		t.Fatal(err)
	}
}

func installedRecord(t *testing.T) (bool, string) {
	t.Helper()
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	c, err := userconfig.Load()
	if err != nil {
		t.Fatal(err)
	}
	return s.Installed, c.WithDefaults().CurrentPath
}

// TestReconcileDistro asserts the record rules in every environment:
//   - list error (no wsl.exe): untouched, reported via err (blind, never wipe)
//   - distro present: untouched
//   - confirmed absent: installed cleared + current_path cleared
func TestReconcileDistro(t *testing.T) {
	withTempHome(t)
	seedInstalledRecord(t)
	present, err := doctor.ReconcileDistro()
	if err != nil {
		// No working wsl.exe (non-Windows, arm runners): the record must
		// survive blindly.
		if installed, current := installedRecord(t); !installed || current == "" {
			t.Fatalf("blind reconcile wiped the record: installed=%v current=%q", installed, current)
		}
		return
	}
	if present {
		// A real dockup distro is installed (dev machine): preserve it.
		if installed, current := installedRecord(t); !installed || current == "" {
			t.Fatalf("present reconcile wiped the record: installed=%v current=%q", installed, current)
		}
		return
	}
	// Fresh CI runners have no dockup distro: confirmed-absent repair.
	if installed, current := installedRecord(t); installed || current != "" {
		t.Fatalf("absent reconcile kept stale record: installed=%v current=%q", installed, current)
	}
	if runtime.GOOS != "windows" {
		t.Fatal("confirmed-absent reconcile needs wsl.exe list to succeed")
	}
}
