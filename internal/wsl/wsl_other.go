//go:build !windows

package wsl

import "os/exec"

// detachConsole is a no-op off Windows.
func detachConsole(_ *exec.Cmd) {}
