//go:build windows

package wsl

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wslAPIDLL        = windows.NewLazySystemDLL("api-ms-win-wsl-api-l1-1-0.dll")
	procIsRegistered = wslAPIDLL.NewProc("WslIsDistributionRegistered")
)

// IsRegistered reports via the WSL API whether name is registered, without
// spawning wsl.exe (fast path for the constant "is dockup there" question).
// Unlike Exists it distinguishes failure: err != nil means unknown (old WSL,
// missing API — fall back to List), never absent.
func IsRegistered(name string) (bool, error) {
	name16, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	if err := procIsRegistered.Find(); err != nil {
		return false, err
	}
	// #nosec G103 -- audited unsafe: standard syscall string passing (pointer held alive across the call), same pattern as x/sys itself
	r, _, _ := procIsRegistered.Call(uintptr(unsafe.Pointer(name16)))
	runtime.KeepAlive(name16)
	return r != 0, nil
}
