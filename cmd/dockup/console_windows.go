//go:build windows

package main

import (
	"context"
	"syscall"

	"golang.org/x/sys/windows"
)

// consoleHandlerRef keeps the control handler reachable: the OS calls it
// long after install, and a collected callback would crash the process.
var consoleHandlerRef uintptr

// ensureProcessedInput makes sure our console translates Ctrl+C keystrokes
// into control events. Windows only generates CTRL_C_EVENT when the console
// input mode includes ENABLE_PROCESSED_INPUT — any process sharing our
// console (historically our own wsl.exe bridges) can clear that flag with
// SetConsoleMode, after which Ctrl+C keystrokes land in the input buffer as
// dead key events and no process ever sees a signal. Best-effort: without a
// console there is nothing to fix. Reports whether processed input is on.
func ensureProcessedInput() bool {
	h, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	const want = windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT
	if mode&windows.ENABLE_PROCESSED_INPUT != 0 {
		return true
	}
	if err := windows.SetConsoleMode(h, mode|want); err != nil {
		return false
	}
	return true
}

// watchConsoleExit cancels ctx on console teardown events the Go runtime
// ignores: closing the window, logoff, and shutdown. Without this, an
// X-closed foreground keeps holding the pipe as a headless process.
// Ctrl+C / Ctrl+Break are left to the runtime (return 0 passes them on).
func watchConsoleExit(cancel context.CancelFunc) {
	consoleHandlerRef = syscall.NewCallback(func(ctrlType uint32) uintptr {
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
	_, _, _ = proc.Call(consoleHandlerRef, 1)
}
