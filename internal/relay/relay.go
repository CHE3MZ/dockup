//go:build windows

// Package relay serves the dockup named pipe (go-winio) and, when enabled in
// ~/.dockup/config.json, an additional 127.0.0.1-only TCP bridge. Each
// accepted client (pipe or TCP) gets a private in-distro bridge:
//
//	wsl -d dockup -u root -- socat STDIO UNIX-CONNECT:/var/run/docker.sock
//
// Raw byte-copy supports HTTP hijack (run -it, logs -f, exec). The named pipe
// is always served; TCP is opt-in (use_tcp) so there is no LAN exposure by
// default. Windows-only.
package relay

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/userconfig"
	"github.com/Microsoft/go-winio"
)

// PipeName re-exports the default pipe.
const PipeName = config.PipeName

func dbg(format string, a ...any) {
	if os.Getenv("DOCKUP_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "relay: "+format+"\n", a...)
	}
}

// AliveOn dials a specific pipe with a short timeout.
func AliveOn(pipe string) bool {
	timeout := 2 * time.Second
	c, err := winio.DialPipe(pipe, &timeout)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// Alive dials the default pipe. True = something holds the name
// (ours or foreign — callers must check state before attaching).
func Alive() bool { return AliveOn(PipeName) }

// TCPAlive dials a TCP bridge address (e.g. 127.0.0.1:2375).
func TCPAlive(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// EngineReady asks the Engine API (GET /_ping) through the named pipe.
// True = dockerd is answering. False = pipe held by nothing useful, or the
// engine is still booting (ps reports "starting..." then).
func EngineReady(pipe string) bool {
	if pipe == "" {
		pipe = PipeName
	}
	timeout := 4 * time.Second
	c, err := winio.DialPipe(pipe, &timeout)
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(timeout))
	if _, err := c.Write([]byte("GET /_ping HTTP/1.0\r\nHost: localhost\r\n\r\n")); err != nil {
		return false
	}
	buf := make([]byte, 512)
	n, err := c.Read(buf)
	if err != nil || n == 0 {
		return false
	}
	return ParsePingOK(buf[:n])
}

// ParsePingOK reports whether an HTTP response head signals success.
// Pure (unit-testable): Docker answers /_ping with "200 OK".
func ParsePingOK(head []byte) bool {
	if len(head) < 12 {
		return false
	}
	for i := 0; i+3 <= len(head); i++ {
		if head[i] == '2' && head[i+1] == '0' && head[i+2] == '0' {
			return true
		}
	}
	return false
}

// WaitAlive polls until the default pipe answers or timeout elapses.
func WaitAlive(timeout time.Duration) bool {
	return WaitAliveOn(PipeName, timeout)
}

// WaitAliveOn polls a specific pipe.
func WaitAliveOn(pipe string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if AliveOn(pipe) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// WaitDead polls until the default pipe stops answering or timeout elapses.
func WaitDead(timeout time.Duration) bool {
	return WaitDeadOn(PipeName, timeout)
}

// WaitDeadOn polls a specific pipe.
func WaitDeadOn(pipe string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !AliveOn(pipe) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// Serve blocks serving the default pipe only (no TCP). Kept for compat.
func Serve(ctx context.Context, distro string) error {
	return ServeEx(ctx, distro, PipeName, false, userconfig.DefaultPort)
}

// ServeEx serves pipe plus, when useTCP is true, a 127.0.0.1:port bridge.
// The pipe is served in message mode so client stdin CloseWrite arrives as
// EOF (lets `docker run -i` containers see stdin end). Stops on ctx cancel.
// One goroutine (+ one wsl.exe) per connection.
func ServeEx(ctx context.Context, distro, pipe string, useTCP bool, port int) error {
	if distro == "" {
		return fmt.Errorf("no distro")
	}
	if pipe == "" {
		pipe = PipeName
	}
	if useTCP {
		if err := userconfig.ValidatePort(port); err != nil {
			return err
		}
	}
	l, err := winio.ListenPipe(pipe, &winio.PipeConfig{MessageMode: true})
	if err != nil {
		return fmt.Errorf("listen %s: %w", pipe, err)
	}
	defer func() { _ = l.Close() }()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()

	if useTCP {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		tl, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", addr, err)
		}
		defer func() { _ = tl.Close() }()
		go func() {
			<-ctx.Done()
			_ = tl.Close()
		}()
		go func() {
			for {
				c, err := tl.Accept()
				if err != nil {
					select {
					case <-ctx.Done():
						return
					default:
						time.Sleep(50 * time.Millisecond)
						continue
					}
				}
				go bridgeConn(c, distro)
			}
		}()
		dbg("tcp bridge on %s", addr)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		pc, err := l.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		go bridgeConn(pc, distro)
	}
}

func bridgeConn(client net.Conn, distro string) {
	defer func() { _ = client.Close() }()
	cmd := exec.Command("wsl.exe", "-d", distro, "-u", "root", "--",
		"socat", "STDIO", "UNIX-CONNECT:/var/run/docker.sock")
	toProc, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay: stdin pipe:", err)
		return
	}
	fromProc, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "relay: stdout pipe:", err)
		return
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "relay: wsl spawn:", err)
		return
	}
	dbg("bridge wsl pid %d", cmd.Process.Pid)
	// Teardown rules (message-mode pipe: a client stdin CloseWrite arrives
	// here as EOF, so stdin finishing first is NORMAL, not a wedge):
	// - daemon side finishes first: the exchange is over, reap immediately
	//   (this also fixed the old sequential-Wait deadlock where GH e2e hung
	//   16min after hello-world output on a half-open socket).
	// - stdin side finishes first: half-close downstream (socat propagates
	//   stdin EOF to the daemon) and give the container a grace period to
	//   flush output and exit before reaping.
	done := make(chan string, 2)
	go func() {
		_, _ = io.Copy(toProc, client)
		_ = toProc.Close()
		done <- "stdin"
	}()
	go func() {
		_, _ = io.Copy(client, fromProc)
		done <- "stdout"
	}()
	if first := <-done; first == "stdin" {
		timer := time.NewTimer(60 * time.Second)
		select {
		case <-done:
		case <-timer.C:
		}
		timer.Stop()
	}
	_ = cmd.Process.Kill()
	timer := time.NewTimer(5 * time.Second)
	select {
	case <-done:
	case <-timer.C:
	}
	timer.Stop()
	werr := cmd.Wait()
	dbg("bridge done (wait=%v)", werr)
}
