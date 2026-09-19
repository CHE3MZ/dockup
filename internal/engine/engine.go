// Package engine starts/stops containerd -> dockerd inside WSL.
// All commands run as root via stdin scripts with nohup/setsid.
// (Client traffic reaches dockerd via per-connection `socat STDIO` bridges
// spawned by the relay — no persistent TCP listener in-distro.)
package engine

import (
	"fmt"
	"time"

	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Start launches containerd, then dockerd on its unix socket.
// Scripts go via stdin (ExecScript): redirections must arrive verbatim.
func Start(distro string) error {
	// containerd first.
	if _, err := wsl.ExecScript(distro, 30*time.Second,
		"setsid nohup containerd >/var/log/dockup-containerd.log 2>&1 < /dev/null &"); err != nil {
		return fmt.Errorf("start containerd: %v", err)
	}
	// dockerd on unix socket only (no TCP in-distro).
	if _, err := wsl.ExecScript(distro, 30*time.Second,
		"setsid nohup dockerd -H unix:///var/run/docker.sock >/var/log/dockup-dockerd.log 2>&1 < /dev/null &"); err != nil {
		return fmt.Errorf("start dockerd: %v", err)
	}
	return nil
}

// Stop kills only dockup-started processes by exact command-line match:
// dockerd with our socket flag, bare containerd, and our STDIO bridge
// socats. Never broad `killall docker`.
func Stop(distro string) error {
	patterns := []string{"socat STDIO UNIX-CONNECT", "dockerd -H unix:///var/run/docker.sock", "containerd"}
	for _, p := range patterns {
		_, _ = wsl.ExecScript(distro, 15*time.Second, "pkill -f '"+p+"'")
	}
	return nil
}

// SocketAlive checks /var/run/docker.sock exists and `docker version` answers
// inside the distro (proves dockerd is up, not just socat listening).
func SocketAlive(distro string) bool {
	out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c",
		"test -S /var/run/docker.sock && docker version --format '{{.Server.Version}}'")
	if err != nil {
		return false
	}
	return len(trimSpace(out)) > 0
}

// WaitSocket polls SocketAlive until timeout.
func WaitSocket(distro string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if SocketAlive(distro) {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("dockerd socket in %q not ready", distro)
}

func trimSpace(b []byte) []byte {
	s := string(b)
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' || s[i] == 0) {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\n' || s[j-1] == '\r' || s[j-1] == '\t' || s[j-1] == 0) {
		j--
	}
	return []byte(s[i:j])
}

// TrimOutput trims whitespace/NULs from command output for display.
func TrimOutput(b []byte) string {
	return string(trimSpace(b))
}
