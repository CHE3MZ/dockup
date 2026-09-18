// Package log handles daemon log files with size-cap rotation (~5 MB).
package log

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/CHE3MZ/dockup/internal/state"
)

const maxBytes = 5 * 1024 * 1024

// PathFor returns %APPDATA%/dockup/logs/<distro>.log.
func PathFor(distro string) (string, error) {
	dir, err := state.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs", distro+".log"), nil
}

// Append writes p to the distro log, truncating if over cap.
func Append(distro string, p []byte) error {
	path, err := PathFor(distro)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if st, err := os.Stat(path); err == nil && st.Size() > maxBytes {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return fmt.Errorf("rotate log: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(p)
	return err
}

// RotateIfNeeded truncates the log if over cap. Call at daemon start:
// the child writes via a held handle, so rotation must happen before open.
func RotateIfNeeded(distro string) error {
	path, err := PathFor(distro)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if st, err := os.Stat(path); err == nil && st.Size() > maxBytes {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return fmt.Errorf("rotate log: %w", err)
		}
	}
	return nil
}
