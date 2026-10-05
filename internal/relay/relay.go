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
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/userconfig"
	"github.com/Microsoft/go-winio"
)

// PipeName re-exports the default pipe.
const PipeName = config.PipeName

// DefaultDockerPipe is Docker Desktop's conventional pipe. dockup serves it
// as a convenience mirror ONLY when nothing else holds it, so bare `docker`
// commands work on machines without Docker Desktop. A held pipe is never
// hijacked: the mirror is skipped and the caller is told why.
const DefaultDockerPipe = `\\.\pipe\docker_engine`

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

// serveListener bridges every accepted connection until ctx ends.
func serveListener(ctx context.Context, l net.Listener, distro string) {
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					time.Sleep(50 * time.Millisecond)
					continue
				}
			}
			go bridgeConn(ctx, c, distro)
		}
	}()
}

// ServeEx serves pipe plus, when useTCP is true, a 127.0.0.1:port bridge.
// When the default docker_engine pipe is free it is served too as a mirror
// (same per-connection bridge), and onMirror — when non-nil — is invoked
// once at startup reporting whether the mirror is up. The pipes are served
// in message mode so client stdin CloseWrite arrives as EOF (lets
// `docker run -i` containers see stdin end). Stops on ctx cancel.
// One goroutine (+ one wsl.exe) per connection.
func ServeEx(ctx context.Context, distro, pipe string, useTCP bool, port int, onMirror func(served bool)) error {
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

	// Mirror first, serve second: a live main pipe then implies the callback
	// below already ran, so parents reading state after WaitAlive never race it.
	mirror := false
	var ml net.Listener
	if !strings.EqualFold(pipe, DefaultDockerPipe) {
		if m, merr := winio.ListenPipe(DefaultDockerPipe, &winio.PipeConfig{MessageMode: true}); merr == nil {
			mirror = true
			ml = m
			defer func() { _ = ml.Close() }()
		} else {
			dbg("default pipe held, mirror skipped: %v", merr)
		}
	}
	if onMirror != nil {
		onMirror(mirror)
	}
	serveListener(ctx, l, distro)
	if mirror {
		serveListener(ctx, ml, distro)
	}

	if useTCP {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		tl, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listen %s: %w", addr, err)
		}
		defer func() { _ = tl.Close() }()
		serveListener(ctx, tl, distro)
		dbg("tcp bridge on %s", addr)
	}

	<-ctx.Done()
	return nil
}

// gatewayError is served when the backend dies before producing any output.
// A loud protocol error beats silent EOF: the client fails retryably
// instead of succeeding with empty output.
const gatewayError = "HTTP/1.0 500 Internal Server Error\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"

type sideResult struct {
	side string
	n    int64
}

func bridgeConn(ctx context.Context, client net.Conn, distro string) {
	defer func() { _ = client.Close() }()
	cmd := exec.Command("wsl.exe", "-d", distro, "-u", "root", "--", // #nosec G204 -- fixed binary and argv; distro is our constant, never a shell
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
		_, _ = fmt.Fprint(client, gatewayError)
		return
	}
	dbg("bridge wsl pid %d", cmd.Process.Pid)
	// Shutdown slays in-flight bridges: without this, streaming clients
	// (logs -f) orphan wsl.exe processes and stall teardown.
	gone := make(chan struct{})
	defer close(gone)
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
		case <-gone:
		}
	}()
	// Teardown rules (message-mode pipe: a client stdin CloseWrite arrives
	// here as EOF, so stdin finishing first is NORMAL, not a wedge):
	// - daemon side finishes first: the exchange is over, reap immediately
	//   (this also fixed the old sequential-Wait deadlock where GH e2e hung
	//   16min after hello-world output on a half-open socket).
	// - stdin side finishes first: half-close downstream (socat propagates
	//   stdin EOF to the daemon) and give the container a grace period to
	//   flush output and exit before reaping.
	done := make(chan sideResult, 2)
	go func() {
		_, _ = io.Copy(toProc, client)
		_ = toProc.Close()
		done <- sideResult{side: "stdin"}
	}()
	go func() {
		n, _ := io.Copy(client, fromProc)
		done <- sideResult{side: "stdout", n: n}
	}()
	// outN tracks daemon->client bytes once observed; -1 = unknown.
	var outN int64 = -1
	note := func(r sideResult) {
		if r.side == "stdout" {
			outN = r.n
		}
	}
	if first := <-done; first.side == "stdin" {
		// Client half-closed stdin: let the container flush and exit.
		timer := time.NewTimer(60 * time.Second)
		select {
		case r := <-done:
			note(r)
		case <-timer.C:
		}
		timer.Stop()
	} else {
		note(first)
	}
	_ = cmd.Process.Kill()
	werr := cmd.Wait()
	if outN < 0 {
		// A copy parked on the live client can hide a dead backend;
		// insist briefly on the daemon-side verdict instead of closing
		// blind (a blind close is what produced silent empty responses).
		timer := time.NewTimer(10 * time.Second)
		select {
		case r := <-done:
			note(r)
		case <-timer.C:
		}
		timer.Stop()
	}
	// A bridge reaped by shutdown stays quiet: the client is gone with us,
	// so a gateway error here would only pollute the daemon log.
	if ctx.Err() != nil {
		dbg("bridge reaped by shutdown (wait=%v)", werr)
		return
	}
	switch {
	case werr != nil && outN == 0:
		logx.Append(fmt.Sprintf("bridge: backend for %s died before output (%v)", distro, werr))
		_, _ = fmt.Fprint(client, gatewayError)
	case werr != nil && outN < 0:
		logx.Append(fmt.Sprintf("bridge: backend for %s failed, client state unknown (%v)", distro, werr))
	}
	dbg("bridge done (wait=%v)", werr)
}
