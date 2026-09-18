// Package engine starts/stops containerd -> dockerd -> socat inside WSL.
// All commands run as root via `wsl -d <distro> -u root` with nohup/setsid.
package engine

import (
	"fmt"
	"net"
	"time"

	"github.com/CHE3MZ/dockup/internal/wsl"
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

// WaitRelay polls Healthy until timeout. Ensures socat is reachable from Windows.
func WaitRelay(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if Healthy(port) {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("relay 127.0.0.1:%d not reachable", port)
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
