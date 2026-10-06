//go:build windows

package main

import (
	"context"
	"testing"
)

// Console helpers are best-effort by design (headless contexts have no
// console): these tests pin that they never fail, with or without one.
func TestEnsureProcessedInputSafe(t *testing.T) {
	_ = ensureProcessedInput()
	_ = ensureProcessedInput()
}

func TestWatchConsoleExitSafe(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	watchConsoleExit(cancel)
}
