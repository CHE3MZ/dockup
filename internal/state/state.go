// Package state persists dockup's Windows-side state.
// Single file: %APPDATA%\dockup\state.json. Absent file = not setup.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
)

// Daemon tracks the background pipe holder.
type Daemon struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
}

// State is the whole file.
type State struct {
	Installed bool   `json:"installed"`
	Arch      string `json:"arch"`
	SetupAt   string `json:"setupAt"`
	Daemon    Daemon `json:"daemon"`
}

// Load reads state.json. Missing file returns zero State, nil error.
func Load() (State, error) {
	var s State
	data, err := os.ReadFile(config.StateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parse state.json: %w", err)
	}
	return s, nil
}

// Save writes state.json atomically.
func Save(s State) error {
	if err := os.MkdirAll(filepath.Dir(config.StateFile()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := config.StateFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, config.StateFile())
}

// ClearDaemon zeroes the daemon record.
func ClearDaemon(s *State) {
	s.Daemon = Daemon{}
}

// WithLock serializes read-modify-write via a lockfile with retries.
// Portable (no syscalls): create-exclusive + stale-age expiry.
func WithLock(fn func(s *State) error) error {
	if err := os.MkdirAll(filepath.Dir(config.LockFile()), 0o755); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		f, err := os.OpenFile(config.LockFile(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprint(f, os.Getpid())
			_ = f.Close()
			break
		}
		// Expire stale locks older than 30s (crashed holder).
		if st, serr := os.Stat(config.LockFile()); serr == nil && time.Since(st.ModTime()) > 30*time.Second {
			_ = os.Remove(config.LockFile())
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("state is locked by another dockup process")
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer func() { _ = os.Remove(config.LockFile()) }()
	s, err := Load()
	if err != nil {
		return err
	}
	if err := fn(&s); err != nil {
		return err
	}
	return Save(s)
}
