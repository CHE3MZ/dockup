// Package debian handles apt-based engine install + snapshot for revert.
package debian

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

var dockupPackages = []string{"docker-ce", "docker-ce-cli", "containerd.io", "socat", "iptables"}

const repoFile = "/etc/apt/sources.list.d/docker.list"
const keyFile = "/etc/apt/keyrings/docker.asc"

// HasDocker reports whether dockerd or docker-ce is already present.
func HasDocker(distro string) bool {
	out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"command -v dockerd; dpkg -l docker-ce 2>/dev/null | grep -q '^ii'")
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return true
	}
	// command -v exits 0 when found; be conservative: check output too.
	return err == nil
}

// Snapshot captures pre-install state so revert removes only the delta:
// packages limited to ones NOT already installed, repo files limited to
// ones we actually create.
func Snapshot(distro string) (state.InstalledByDockup, bool) {
	hadPrior := HasDocker(distro)
	keep := installedPkgs(distro)
	var delta []string
	for _, p := range dockupPackages {
		if !keep[p] {
			delta = append(delta, p)
		}
	}
	var repos []string
	for _, f := range []string{repoFile, keyFile} {
		if _, err := wsl.Exec(distro, 15*time.Second, "sh", "-c", "test -f "+f); err != nil {
			repos = append(repos, f) // absent now => setup creates it => revert removes it
		}
	}
	snap := state.InstalledByDockup{
		Packages: delta,
		Repos:    repos,
		Files:    []string{"/var/log/dockup-dockerd.log", "/var/log/dockup-containerd.log", "/var/log/dockup-socat.log"},
	}
	return snap, hadPrior
}

// installedPkgs returns the subset of dockupPackages already installed.
func installedPkgs(distro string) map[string]bool {
	out, _ := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"for p in "+strings.Join(dockupPackages, " ")+"; do dpkg-query -W -f='${Status}' \"$p\" 2>/dev/null | grep -q 'install ok installed' && echo \"KEEP:$p\"; done")
	keep := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "KEEP:"); ok {
			keep[name] = true
		}
	}
	return keep
}

// Preflight checks kernel features dockerd needs. Fails with a clear message
// on custom kernels missing iptables/nat or overlayfs.
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

// osCodename returns VERSION_CODENAME, mapping trixie->bookworm with a warning
// if Docker's repo does not serve trixie yet.
func osCodename(distro string) (codename string, trixieFallback bool) {
	out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c", ". /etc/os-release; echo $VERSION_CODENAME")
	if err != nil {
		return "bookworm", false
	}
	codename = strings.TrimSpace(string(out))
	if codename == "" {
		return "bookworm", false
	}
	if codename == "trixie" {
		return "bookworm", true
	}
	return codename, false
}

// Setup installs docker-ce, containerd.io, socat from Docker's official repo.
// Idempotent: if engine + socat + iptables already present, returns the
// snapshot without reinstalling. Prereqs (incl. iptables, which minimal WSL
// images lack) install BEFORE the preflight so the preflight is meaningful.
// Returns snapshot + hadPrior for state.
func Setup(distro string) (state.InstalledByDockup, bool, error) {
	snap, hadPrior := Snapshot(distro)
	// Idempotent fast-path: everything already present.
	if out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"command -v dockerd && command -v socat && command -v iptables && test -f "+repoFile); err == nil && len(out) > 0 {
		return snap, hadPrior, nil
	}
	prereq := `set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl gnupg iptables
`
	if _, err := wsl.RunRetry(distro, "prereq install", 10*time.Minute, 3, prereq); err != nil {
		return snap, hadPrior, err
	}
	if err := Preflight(distro); err != nil {
		return snap, hadPrior, err
	}
	codename, fellBack := osCodename(distro)
	if fellBack {
		fmt.Fprintln(os.Stderr, "dockup: trixie detected, using bookworm repo track (Docker repo lag)")
	}
	script := `set -e
export DEBIAN_FRONTEND=noninteractive
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
ARCH=$(dpkg --print-architecture)
CODENAME="` + codename + `"
. /etc/os-release
if [ "$ID" = "ubuntu" ]; then
  echo "deb [arch=$ARCH signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu $VERSION_CODENAME stable" > /etc/apt/sources.list.d/docker.list
else
  echo "deb [arch=$ARCH signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $CODENAME stable" > /etc/apt/sources.list.d/docker.list
fi
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io socat
`
	if _, err := wsl.RunRetry(distro, "engine install", 10*time.Minute, 3, script); err != nil {
		return snap, hadPrior, err
	}
	// Loud if apt claimed success but binaries are missing (never proceed blind).
	if out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"command -v dockerd && command -v socat && command -v containerd"); err != nil || len(out) == 0 {
		return snap, hadPrior, fmt.Errorf("engine install finished but binaries missing: %v", err)
	}
	// The .deb postinst auto-enables docker/containerd under systemd, which
	// would autostart the engine on every WSL boot and double-run beside
	// ours. dockup owns these processes exclusively: disable everything —
	// but only when WE caused it (never touch a pre-existing install).
	if !hadPrior {
		_, _ = wsl.ExecScript(distro, 30*time.Second,
			"command -v systemctl >/dev/null && systemctl disable --now docker.service docker.socket containerd.service || true")
	} else {
		fmt.Fprintln(os.Stderr, "dockup: warning: pre-existing docker install kept as-is (may conflict on the socket)")
	}
	return snap, hadPrior, nil
}

// Remove deletes ONLY what setup added (snapshot): delta packages, created
// repo/key files, own log files. Images, volumes, vhdx, distro untouched.
func Remove(distro string, snap state.InstalledByDockup) error {
	if len(snap.Packages) > 0 {
		script := "DEBIAN_FRONTEND=noninteractive apt-get remove -y " + strings.Join(snap.Packages, " ")
		if _, err := wsl.RunRetry(distro, "package removal", 10*time.Minute, 2, script); err != nil {
			return err
		}
	}
	if len(snap.Repos) > 0 {
		_, _ = wsl.ExecScript(distro, 15*time.Second, "rm -f "+strings.Join(snap.Repos, " "))
		_, _ = wsl.ExecScript(distro, 5*time.Minute, "DEBIAN_FRONTEND=noninteractive apt-get update")
	}
	if len(snap.Files) > 0 {
		_, _ = wsl.ExecScript(distro, 15*time.Second, "rm -f "+strings.Join(snap.Files, " "))
	}
	return nil
}
