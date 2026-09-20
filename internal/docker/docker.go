// Package docker holds the in-distro install/configure/test scripts.
// All scripts run as root via wsl ExecScript (stdin), never argv.
package docker

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/wsl"
)

// InstallScript adds Docker's official apt repo and installs engine + socat.
const InstallScript = `set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
CODENAME="$(. /etc/os-release && echo "$VERSION_CODENAME")"
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $CODENAME stable" > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin socat
# Keep the image lean: drop downloaded .debs (a dynamic VHDX only grows).
apt-get clean
`

// ConfigureScript enables systemd units for containerd + docker.
const ConfigureScript = `set -eu
printf '[boot]\nsystemd=true\n' > /etc/wsl.conf
systemctl reset-failed containerd.service docker.service docker.socket || true
systemctl enable containerd.service
systemctl enable docker.service
systemctl start containerd.service || true
systemctl start docker.service || true
`

// UpgradeScript refreshes apt and moves engine packages to their latest
// versions, then restarts the systemd units. Idempotent.
const UpgradeScript = `set -eu
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin socat
# Keep the image lean: drop downloaded .debs (a dynamic VHDX only grows).
apt-get clean
systemctl restart containerd.service
systemctl restart docker.service
`

// PreflightScript checks kernel features dockerd needs.
// NOTE: iptables is NOT checked here — minimal rootfs has no iptables binary
// yet (it arrives as a docker-ce dependency). iptables is verified in
// TestDaemon after install.
const PreflightScript = `set -eu
grep -q overlay /proc/filesystems || { echo "missing overlayfs"; exit 1; }
echo ok
`

// Install runs the apt flow with retries.
func Install(distro string) error {
	out, err := wsl.ExecScript(distro, 60*time.Second, PreflightScript)
	if err != nil {
		return fmt.Errorf("kernel preflight: %w (out: %s)", err, wsl.Tail(out, 1000))
	}
	_, err = wsl.RunRetry(distro, "install docker", 10*time.Minute, 3, InstallScript)
	return err
}

// Configure enables systemd + services, then reboots the distro once.
func Configure(distro string) error {
	if _, err := wsl.ExecScript(distro, 3*time.Minute, ConfigureScript); err != nil {
		return fmt.Errorf("configure docker: %w", err)
	}
	// Reboot so systemd becomes PID 1 with the new wsl.conf.
	_ = wsl.Terminate(distro)
	time.Sleep(3 * time.Second)
	out, err := wsl.Exec(distro, 30*time.Second, "sh", "-c", "ps -p 1 -o comm=")
	if err == nil && !strings.Contains(string(out), "systemd") {
		return fmt.Errorf("systemd did not become PID 1 (got %q); run dockup doctor", strings.TrimSpace(string(out)))
	}
	// Ensure services survived the reboot.
	_, _ = wsl.ExecScript(distro, 2*time.Minute, "systemctl start containerd.service docker.service || true")
	return nil
}

// TestDaemon verifies dockerd answers inside the distro.
func TestDaemon(distro string) error {
	out, err := wsl.ExecScript(distro, 60*time.Second, "docker version --format '{{.Server.Version}}'")
	if err != nil {
		return fmt.Errorf("docker not answering: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("docker server version empty")
	}
	return nil
}

// WaitDaemon polls TestDaemon until the daemon answers or the timeout
// elapses. Mandatory after any restart: dockerd needs seconds to boot,
// and a single-shot check right after `systemctl restart` flakes.
func WaitDaemon(distro string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var err error
	for time.Now().Before(deadline) {
		if err = TestDaemon(distro); err == nil {
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return err
}

// Upgrade moves engine packages to their latest versions and verifies the
// daemon answers afterwards.
func Upgrade(distro string) error {
	if _, err := wsl.RunRetry(distro, "upgrade docker", 10*time.Minute, 2, UpgradeScript); err != nil {
		return err
	}
	return WaitDaemon(distro, 90*time.Second)
}

// EnginePackages is the dockup-managed set: always reinstalled by repair,
// never removed by restore's delta cleanup.
var EnginePackages = []string{
	"docker-ce", "docker-ce-cli", "containerd.io",
	"docker-buildx-plugin", "docker-compose-plugin", "socat",
}

// ManualPackages lists explicitly-installed packages (apt-mark showmanual),
// sorted for stable snapshots.
func ManualPackages(distro string) ([]string, error) {
	out, err := wsl.Exec(distro, 60*time.Second, "apt-mark", "showmanual")
	if err != nil {
		return nil, fmt.Errorf("apt-mark showmanual: %w", err)
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// RepairScript rewrites managed config and restarts units without rebooting.
// reset-failed comes first: after repeated crashes systemd refuses further
// restarts ("start request repeated too quickly") until the failure state
// is cleared — without it, repair restarts are silently refused.
const RepairScript = `set -eu
printf '[boot]\nsystemd=true\n' > /etc/wsl.conf
systemctl reset-failed containerd.service docker.service docker.socket || true
systemctl enable containerd.service
systemctl enable docker.service
systemctl restart containerd.service || true
systemctl restart docker.service || true
`

// Repair restores managed config and a working engine without rebooting,
// unless PID 1 is not systemd (then it reboots like Configure). A clobbered
// /etc/docker/daemon.json is backed up to daemon.json.bak and reset, but
// only when the daemon refuses to start with it. Returns a note describing
// any extra rescue performed ("" when it was a plain repair).
func Repair(distro string) (string, error) {
	if _, err := wsl.ExecScript(distro, 3*time.Minute, RepairScript); err != nil {
		return "", fmt.Errorf("repair config: %w", err)
	}
	if out, err := wsl.Exec(distro, 30*time.Second, "sh", "-c", "ps -p 1 -o comm="); err == nil && !strings.Contains(string(out), "systemd") {
		_ = wsl.Terminate(distro)
		time.Sleep(3 * time.Second)
	}
	if err := TestDaemon(distro); err == nil {
		return "", nil
	}
	if _, serr := wsl.Exec(distro, 15*time.Second, "sh", "-c", "test -f /etc/docker/daemon.json"); serr == nil {
		rescue := `set -eu
cp /etc/docker/daemon.json /etc/docker/daemon.json.bak
rm /etc/docker/daemon.json
systemctl reset-failed docker.service docker.socket || true
systemctl restart docker.service || true`
		_, _ = wsl.ExecScript(distro, 2*time.Minute, rescue)
		if err := WaitDaemon(distro, 90*time.Second); err == nil {
			return "reset /etc/docker/daemon.json (yours is kept at daemon.json.bak)", nil
		}
	}
	if err := WaitDaemon(distro, 90*time.Second); err != nil {
		return "", fmt.Errorf("engine still unhealthy after repair: %w (try dockup restore --full)", err)
	}
	return "", nil
}
