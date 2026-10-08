//go:build !windows

package wsl

import "errors"

var errAPIWindowsOnly = errors.New("dockup is Windows-only")

// IsRegistered is unavailable off Windows; callers fall back to List.
func IsRegistered(_ string) (bool, error) {
	return false, errAPIWindowsOnly
}
