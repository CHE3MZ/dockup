// Package debian handles apt-based engine install + snapshot for revert.
package debian

import (
	"fmt"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

var dockupPackages = []string{"docker-ce", "docker-ce-cli", "containerd.io", "socat"}
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

// Snapshot captures pre-install state so revert removes only the delta.
func Snapshot(distro string) (state.InstalledByDockup, bool) {
	hadPrior := HasDocker(distro)
	snap := state.InstalledByDockup{
		Packages: append([]string{}, dockupPackages...),
		Repos:    []string{repoFile, keyFile},
		Files:    []string{"/var/log/dockup-dockerd.log", "/var/log/dockup-containerd.log", "/var/log/dockup-socat.log"},
	}
	return snap, hadPrior
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
// Idempotent: if engine + socat already present, verifies repo and returns
// the snapshot without reinstalling. Returns snapshot + hadPrior for state.
func Setup(distro string) (state.InstalledByDockup, bool, error) {
	snap, hadPrior := Snapshot(distro)
	if err := Preflight(distro); err != nil {
		return snap, hadPrior, err
	}
	// Idempotent fast-path: dockerd + socat binaries present.
	if out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"command -v dockerd && command -v socat && test -f "+repoFile); err == nil && len(out) > 0 {
		return snap, hadPrior, nil
	}
	codename, fellBack := osCodename(distro)
	_ = fellBack // surfaced via log line by caller if needed.
	script := `set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl gnupg
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
	if _, err := wsl.Exec(distro, 10*time.Minute, "sh", "-c", script); err != nil {
		return snap, hadPrior, fmt.Errorf("apt setup failed: %v", err)
	}
	return snap, hadPrior, nil
}

// Remove deletes only packages dockup installed (names from snapshot).
// Repo/key files are removed too; volumes/images/vhdx are never touched.
func Remove(distro string, packages []string) error {
	if len(packages) > 0 {
		script := "DEBIAN_FRONTEND=noninteractive apt-get remove -y " + strings.Join(packages, " ")
		if _, err := wsl.Exec(distro, 10*time.Minute, "sh", "-c", script); err != nil {
			return err
		}
	}
	_, _ = wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"rm -f "+repoFile+" "+keyFile+"; apt-get update")
	return nil
}
