//go:build windows

package main

import (
	"context"
	"syscall"

	"golang.org/x/sys/windows"
)

// watchConsoleExit cancels ctx on console teardown events the Go runtime
// ignores: closing the window, logoff, and shutdown. Without this, an
// X-closed foreground keeps holding the pipe as a headless process.
// Ctrl+C / Ctrl+Break are left to the runtime (return 0 passes them on).
func watchConsoleExit(cancel context.CancelFunc) {
	handler := syscall.NewCallback(func(ctrlType uint32) uintptr {
		switch ctrlType {
		case windows.CTRL_CLOSE_EVENT,
			windows.CTRL_LOGOFF_EVENT,
			windows.CTRL_SHUTDOWN_EVENT:
			cancel()
			return 1
		}
		return 0
	})
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	_, _, _ = proc.Call(handler, 1)
}
