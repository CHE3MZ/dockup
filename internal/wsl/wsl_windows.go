//go:build windows

package wsl

import (
	"os/exec"
	"syscall"
)

// detachConsole isolates a wsl.exe child from our console. All dockup-driven
// wsl calls talk over captured pipes or files and need no console I/O, while
// sharing it lets a child flip our console input mode (which silently kills
// Ctrl+C delivery for the whole console) and hold console handles past our
// death.
func detachConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
