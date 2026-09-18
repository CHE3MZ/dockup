// Package dockercli checks for the user-installed Windows Docker CLI.
// dockup never installs it; user runs e.g. `scoop install docker docker-compose`.
package dockercli

import (
	"fmt"
	"os/exec"
)

const InstallHint = "docker CLI not found — install with: scoop install docker docker-compose"

// Path returns docker.exe path or an error.
func Path() (string, error) {
	if p, err := exec.LookPath("docker.exe"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("docker"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s", InstallHint)
}

// Check reports whether the CLI exists (no daemon dial).
func Check() (found bool) {
	_, err := Path()
	return err == nil
}

// ComposeCheck reports whether `docker compose version` works.
func ComposeCheck() bool {
	for _, name := range []string{"docker.exe", "docker"} {
		cmd := exec.Command(name, "compose", "version")
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}
