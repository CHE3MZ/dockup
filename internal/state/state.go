// Package state manages %APPDATA%/dockup/state.json.
// Liveness is never trusted from the file — callers must probe.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Save writes state atomically (tmp + rename). TODO: real file locking for
// concurrent double-start from two terminals (lockfile + retry).
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
