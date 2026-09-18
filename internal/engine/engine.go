// Package engine starts/stops containerd -> dockerd -> socat inside WSL.
// All commands run as root via `wsl -d <distro> -u root` with nohup/setsid.
package engine

import (
	"fmt"
	"net"
	"time"

	"dockup/internal/wsl"
)

// PickFreePort returns a free 127.0.0.1 TCP port for the relay.
func PickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Start launches containerd, dockerd, then socat binding 127.0.0.1:port.
func Start(distro string, port int) error {
	// containerd first.
	if _, err := wsl.Exec(distro, 30*time.Second, "sh", "-c",
		"setsid nohup containerd >/var/log/dockup-containerd.log 2>&1 < /dev/null &"); err != nil {
		return fmt.Errorf("start containerd: %v", err)
	}
	// dockerd on unix socket only (no TCP in-distro).
	if _, err := wsl.Exec(distro, 30*time.Second, "sh", "-c",
		"setsid nohup dockerd -H unix:///var/run/docker.sock >/var/log/dockup-dockerd.log 2>&1 < /dev/null &"); err != nil {
		return fmt.Errorf("start dockerd: %v", err)
	}
	// socat bridges 127.0.0.1:port -> unix socket. NEVER 0.0.0.0.
	cmd := fmt.Sprintf("setsid nohup socat TCP-LISTEN:%d,bind=127.0.0.1,fork,reuseaddr UNIX-CONNECT:/var/run/docker.sock >/var/log/dockup-socat.log 2>&1 < /dev/null &", port)
	if _, err := wsl.Exec(distro, 15*time.Second, "sh", "-c", cmd); err != nil {
		return fmt.Errorf("start socat: %v", err)
	}
	return nil
}

// Stop kills only dockup-started processes by exact command-line match.
func Stop(distro string) error {
	// pkill -f with exact patterns; never broad `killall docker`.
	patterns := []string{"socat TCP-LISTEN", "dockerd -H unix:///var/run/docker.sock", "containerd"}
	for _, p := range patterns {
		_, _ = wsl.Exec(distro, 15*time.Second, "sh", "-c", "pkill -f '"+p+"'")
	}
	return nil
}

// Healthy dials 127.0.0.1:port (WSL localhost relay) with a short timeout.
func Healthy(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
