//go:build !windows

package autostart

import "errors"

// ShortcutName is the Startup folder entry name.
const ShortcutName = "dockup.lnk"

var errWindowsOnly = errors.New("dockup is Windows-only")

// ShortcutPath is unsupported off Windows.
func ShortcutPath() string { return "" }

// IsEnabled is always false off Windows.
func IsEnabled() bool { return false }

// SetEnabled errors off Windows.
func SetEnabled(_ string, _ bool) error { return errWindowsOnly }
