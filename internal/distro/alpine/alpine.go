// Package alpine handles apk-based engine install + snapshot for revert.
package alpine

import (
	"fmt"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

var dockupPackages = []string{"docker", "containerd", "socat", "docker-cli-compose"}

// HasDocker reports whether dockerd is already present.
func HasDocker(distro string) bool {
	out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c", "command -v dockerd")
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// Snapshot captures pre-install state so revert removes only the delta.
func Snapshot(distro string) (state.InstalledByDockup, bool) {
	return state.InstalledByDockup{
		Packages: append([]string{}, dockupPackages...),
		Repos:    []string{},
		Files:    []string{"/var/log/dockup-dockerd.log", "/var/log/dockup-containerd.log", "/var/log/dockup-socat.log"},
	}, HasDocker(distro)
}

// Preflight checks kernel features dockerd needs.
func Preflight(distro string) error {
	if _, err := wsl.Exec(distro, 15*time.Second, "sh", "-c", "iptables -L -n >/dev/null 2>&1"); err != nil {
		return fmt.Errorf("iptables not usable in %q (custom kernel?) — dockerd needs iptables/nat", distro)
	}
	out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c", "cat /proc/filesystems")
	if err != nil || !strings.Contains(string(out), "overlay") {
		return fmt.Errorf("overlayfs not available in %q — dockerd needs overlayfs", distro)
	}
	return nil
}

// Setup installs docker, containerd, socat. No rc-update (no autostart).
// Idempotent: if dockerd + socat already present, verifies and returns
// the snapshot without reinstalling.
func Setup(distro string) (state.InstalledByDockup, bool, error) {
	snap, hadPrior := Snapshot(distro)
	if err := Preflight(distro); err != nil {
		return snap, hadPrior, err
	}
	if out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"command -v dockerd && command -v socat"); err == nil && len(out) > 0 {
		return snap, hadPrior, nil
	}
	script := `set -e
apk add --no-cache docker containerd socat docker-cli-compose
`
	if _, err := wsl.Exec(distro, 10*time.Minute, "sh", "-c", script); err != nil {
		return snap, hadPrior, fmt.Errorf("apk setup failed: %v", err)
	}
	return snap, hadPrior, nil
}

// Remove deletes only packages dockup installed. Volumes/images are untouched.
func Remove(distro string, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	script := "apk del " + strings.Join(packages, " ")
	_, err := wsl.Exec(distro, 10*time.Minute, "sh", "-c", script)
	return err
}
