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

// AliveOn always false off Windows.
func AliveOn(_ string) bool { return false }

// TCPAlive always false off Windows.
func TCPAlive(_ string) bool { return false }

// WaitAlive always false off Windows.
func WaitAlive(_ time.Duration) bool { return false }

// WaitAliveOn always false off Windows.
func WaitAliveOn(_ string, _ time.Duration) bool { return false }

// WaitDead always true off Windows.
func WaitDead(_ time.Duration) bool { return true }

// WaitDeadOn always true off Windows.
func WaitDeadOn(_ string, _ time.Duration) bool { return true }

// Serve errors off Windows.
func Serve(_ context.Context, _ string) error {
	return errWindowsOnly
}

// ServeEx errors off Windows.
func ServeEx(_ context.Context, _ string, _ string, _ bool, _ int) error {
	return errWindowsOnly
}
