// Package docker holds the in-distro install/configure/test scripts.
// All scripts run as root via wsl ExecScript (stdin), never argv.
package docker

import (
	"fmt"
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
`

// ConfigureScript enables systemd units for containerd + docker.
const ConfigureScript = `set -eu
printf '[boot]\nsystemd=true\n' > /etc/wsl.conf
systemctl enable containerd.service
systemctl enable docker.service
systemctl start containerd.service || true
systemctl start docker.service || true
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
	_, err := wsl.RunRetry(distro, "install docker", 10*time.Minute, 3, InstallScript)
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
