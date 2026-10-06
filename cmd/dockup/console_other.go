//go:build !windows

package main

import "context"

// watchConsoleExit is a no-op off Windows (dockup is Windows-only;
// the stubs in relay/daemon already cover the rest there).
func watchConsoleExit(_ context.CancelFunc) {}

// ensureProcessedInput is a no-op off Windows.
func ensureProcessedInput() bool { return false }
