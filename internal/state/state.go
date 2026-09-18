// Package state manages %APPDATA%/dockup/state.json.
// Liveness is never trusted from the file — callers must probe.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// InstalledByDockup snapshots what setup added so revert removes only the delta.
type InstalledByDockup struct {
	Packages []string `json:"packages"`
	Repos    []string `json:"repos"`
	Files    []string `json:"files"`
}

// DaemonInfo tracks the background/foreground relay owner.
type DaemonInfo struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	Mode      string `json:"mode"` // "foreground" | "daemon" | ""
}

// DistroState is per-distro inventory + runtime hints.
type DistroState struct {
	RelayPort         int               `json:"relayPort"`
	WSLWasRunning     bool              `json:"wslWasRunning"`
	BootedByDockup    bool              `json:"bootedByDockup"`
	InstalledByDockup InstalledByDockup `json:"installedByDockup"`
	HadPriorDocker    bool              `json:"hadPriorDocker"`
	Daemon            DaemonInfo        `json:"daemon"`
}

// State is the whole file. ActiveDistro enforces single-active rule.
type State struct {
	Default      string                 `json:"default"`
	ActiveDistro string                 `json:"activeDistro"`
	Distros      map[string]DistroState `json:"distros"`
}

// Dir returns %APPDATA%/dockup (the ONLY place dockup persists on Windows).
func Dir() (string, error) {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return "", fmt.Errorf("APPDATA is not set")
	}
	return filepath.Join(appdata, "dockup"), nil
}

// Path returns the state.json path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// Load reads state, returning an empty state if the file does not exist.
func Load() (*State, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	s := &State{Distros: map[string]DistroState{}}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("corrupt state file %s: %w", p, err)
	}
	if s.Distros == nil {
		s.Distros = map[string]DistroState{}
	}
	return s, nil
}

// Save writes state atomically (tmp + rename). For read-modify-write across
// processes (double-start from two terminals) use Transaction.
func Save(s *State) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// lockPath returns the lockfile path next to state.json.
func lockPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.lock"), nil
}

// acquire creates the lockfile exclusively, retrying until timeout.
// A lock older than 30s is treated as stale (crashed owner) and removed.
func acquire(timeout time.Duration) (release func(), err error) {
	lp, err := lockPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(lp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%d", os.Getpid())
			f.Close()
			return func() { _ = os.Remove(lp) }, nil
		}
		if st, statErr := os.Stat(lp); statErr == nil && time.Since(st.ModTime()) > 30*time.Second {
			_ = os.Remove(lp)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("state is locked by another dockup (try again)")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Transaction holds the lock across load → mutate → save, so two terminals
// racing to start cannot corrupt state or double-claim activeDistro.
func Transaction(fn func(*State) error) error {
	release, err := acquire(10 * time.Second)
	if err != nil {
		return err
	}
	defer release()
	s, err := Load()
	if err != nil {
		return err
	}
	if err := fn(s); err != nil {
		return err
	}
	return Save(s)
}
