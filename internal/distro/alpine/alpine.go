// Package alpine handles apk-based engine install + snapshot for revert.
package alpine

import (
	"fmt"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

var dockupPackages = []string{"docker", "containerd", "socat", "docker-cli-compose", "iptables"}

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
	hadPrior := HasDocker(distro)
	out, _ := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"for p in "+strings.Join(dockupPackages, " ")+"; do apk info -e \"$p\" >/dev/null 2>&1 && echo \"KEEP:$p\"; done")
	keep := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "KEEP:"); ok {
			keep[name] = true
		}
	}
	var delta []string
	for _, p := range dockupPackages {
		if !keep[p] {
			delta = append(delta, p)
		}
	}
	return state.InstalledByDockup{
		Packages: delta,
		Repos:    []string{},
		Files:    []string{"/var/log/dockup-dockerd.log", "/var/log/dockup-containerd.log", "/var/log/dockup-socat.log"},
	}, hadPrior
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
// Idempotent: if dockerd + socat + iptables already present, returns the
// snapshot without reinstalling. iptables installs BEFORE the preflight so
// the preflight is meaningful on minimal images.
func Setup(distro string) (state.InstalledByDockup, bool, error) {
	snap, hadPrior := Snapshot(distro)
	if out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"command -v dockerd && command -v socat && command -v iptables"); err == nil && len(out) > 0 {
		return snap, hadPrior, nil
	}
	if _, err := wsl.RunRetry(distro, "prereq install", 10*time.Minute, 3, "apk add --no-cache iptables"); err != nil {
		return snap, hadPrior, err
	}
	if err := Preflight(distro); err != nil {
		return snap, hadPrior, err
	}
	script := `set -e
apk add --no-cache docker containerd socat docker-cli-compose
`
	if _, err := wsl.RunRetry(distro, "engine install", 10*time.Minute, 3, script); err != nil {
		return snap, hadPrior, err
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
