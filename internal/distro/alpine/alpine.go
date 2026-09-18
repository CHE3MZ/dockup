// Package alpine handles apk-based engine install + snapshot for revert.
package alpine

import (
	"time"

	"dockup/internal/wsl"
)

// Setup installs docker, containerd, socat. No rc-update (no autostart).
func Setup(distro string) error {
	script := `set -e
apk add --no-cache docker containerd socat docker-cli-compose
`
	_, err := wsl.Exec(distro, 10*time.Minute, "sh", "-c", script)
	return err
}

// Remove deletes only packages dockup installed.
func Remove(distro string, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	script := "apk del " + shellJoin(packages)
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
