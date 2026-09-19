//go:build windows

// Package autostart manages dockup's Windows login autostart via a Startup
// folder shortcut (<exe> daemon start). Opt-in only, default off; the
// source of truth is ~/.dockup/config.json ("autostart"), applied here.
package autostart

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ShortcutName is the Startup folder entry name.
const ShortcutName = "dockup.lnk"

// ShortcutPath returns the Startup folder shortcut path.
func ShortcutPath() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		if home, err := os.UserHomeDir(); err == nil {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
	}
	return filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", ShortcutName)
}

// IsEnabled reports whether the Startup entry exists.
func IsEnabled() bool {
	_, err := os.Stat(ShortcutPath())
	return err == nil
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// SetEnabled creates (on=true) or removes (on=false) the Startup shortcut
// for the given dockup executable.
func SetEnabled(exe string, on bool) error {
	if !on {
		if err := os.Remove(ShortcutPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			return err
		}
	}
	script := fmt.Sprintf(
		`$ws = New-Object -ComObject WScript.Shell; `+
			`$sc = $ws.CreateShortcut(%s); `+
			`$sc.TargetPath = %s; `+
			`$sc.Arguments = 'daemon start'; `+
			`$sc.WorkingDirectory = %s; `+
			`$sc.WindowStyle = 7; `+
			`$sc.Description = 'dockup docker engine (autostart)'; `+
			`$sc.Save()`,
		psQuote(ShortcutPath()), psQuote(exe), psQuote(filepath.Dir(exe)))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", // #nosec G204 -- fixed binary; script is built internally, never from user input
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create startup shortcut: %w\n%s", err, string(out))
	}
	return nil
}
