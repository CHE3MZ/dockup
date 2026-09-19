//go:build !windows

package relay

import (
	"context"
	"errors"
	"time"
)

// PipeName stub for non-Windows builds.
const PipeName = `\\.\pipe\dockup_engine`

var errWindowsOnly = errors.New("dockup is Windows-only")

// Alive always false off Windows.
func Alive() bool { return false }

// WaitAlive always false off Windows.
func WaitAlive(_ time.Duration) bool { return false }

// WaitDead always true off Windows.
func WaitDead(_ time.Duration) bool { return true }

// Serve errors off Windows.
func Serve(_ context.Context, _ string) error {
	return errWindowsOnly
}
