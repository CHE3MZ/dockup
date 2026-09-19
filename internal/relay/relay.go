// Package relay serves \\.\pipe\docker_engine (go-winio). Each accepted
// client gets a private in-distro bridge:
//
//	wsl -d <distro> -u root -- socat STDIO UNIX-CONNECT:/var/run/docker.sock
//
// Raw byte-copy supports HTTP hijack (run -it, logs -f, exec). No TCP ports,
// no port picking, no firewall questions: the design is immune to WSL
// localhost-forwarding quirks (NAT vs mirrored mode) because nothing crosses
// the Windows/WSL boundary over TCP. Cost: one short-lived wsl.exe per
// connection (~0.2-0.5s), fine for a manual-use tool. Windows-only.
package relay

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/Microsoft/go-winio"
)

const PipeName = `\\.\pipe\docker_engine`

// Diagnostics go to STDERR only (never protocol bytes, never STDOUT).
// Verbose per-bridge tracing is opt-in via DOCKUP_DEBUG=1; spawn failures
// always log (a silent bridge is undebuggable).
func dbg(format string, a ...any) {
	if os.Getenv("DOCKUP_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "relay: "+format+"\n", a...)
	}
}

// Alive dials the pipe with a short timeout. True = a relay already holds
// the name (ours or foreign — callers must check state before attaching).
func Alive() bool {
	timeout := 2 * time.Second
	c, err := winio.DialPipe(PipeName, &timeout)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Serve blocks accepting pipe clients and bridging each into the distro.
// Stops on ctx cancel. One goroutine (+ one wsl.exe) per connection.
func Serve(ctx context.Context, distro string) error {
	if distro == "" {
		return fmt.Errorf("no distro")
	}
	l, err := winio.ListenPipe(PipeName, nil)
	if err != nil {
		return fmt.Errorf("listen %s: %w", PipeName, err)
	}
	defer l.Close()

	// Close listener on ctx cancel to unblock Accept.
	go func() {
		<-ctx.Done()
		l.Close()
	}()

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
				// Transient accept error; brief backoff.
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		go bridgeConn(pc, distro)
	}
}

// bridgeConn splices one pipe client to one `socat STDIO ...` process.
// EOF on either side tears the whole bridge down; the process is reaped.
func bridgeConn(pipe net.Conn, distro string) {
	defer pipe.Close()
	start := time.Now()
	dbg("bridge accepted")
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
	cmd.Stderr = nil // socat chatter stays out of the API stream.
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "relay: wsl spawn:", err)
		return
	}
	dbg("bridge wsl pid %d", cmd.Process.Pid)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(toProc, pipe)
		_ = toProc.Close() // EOF downstream so socat can exit.
	}()
	_, _ = io.Copy(pipe, fromProc)
	werr := cmd.Wait()
	<-done
	dbg("bridge done in %s (wait=%v)", time.Since(start).Round(time.Millisecond), werr)
}
