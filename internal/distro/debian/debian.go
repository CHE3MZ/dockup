// Package debian handles apt-based engine install + snapshot for revert.
package debian

import (
	"time"

	"dockup/internal/wsl"
)

// Setup installs docker-ce, containerd.io, socat from Docker's official repo.
// Idempotent: re-runs verify/repair. Caller snapshots prior state first.
func Setup(distro string) error {
	script := `set -e
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y ca-certificates curl gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
. /etc/os-release
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian $VERSION_CODENAME stable" > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io socat
`
	_, err := wsl.Exec(distro, 10*time.Minute, "sh", "-c", script)
	return err
}

// Remove deletes only packages dockup installed (names from snapshot).
func Remove(distro string, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	script := "DEBIAN_FRONTEND=noninteractive apt-get remove -y " + shellJoin(packages)
	_, err := wsl.Exec(distro, 10*time.Minute, "sh", "-c", script)
	return err
}

func shellJoin(pkgs []string) string {
	out := ""
	for i, p := range pkgs {
		if i > 0 {
			out += " "
		}
		out += p
	}
	return out
}
