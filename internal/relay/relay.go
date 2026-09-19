//go:build windows

// Package relay serves \\.\pipe\docker_engine (go-winio). Each accepted
// client gets a private in-distro bridge:
//
//	wsl -d dockup -u root -- socat STDIO UNIX-CONNECT:/var/run/docker.sock
//
// Raw byte-copy supports HTTP hijack (run -it, logs -f, exec). No TCP ports,
// no firewall: immune to WSL2 NAT vs mirrored quirks. Windows-only.
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
	"github.com/CHE3MZ/dockup/internal/config"
)

// PipeName re-exports the canonical pipe.
const PipeName = config.PipeName

func dbg(format string, a ...any) {
	if os.Getenv("DOCKUP_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "relay: "+format+"\n", a...)
	}
}

// Alive dials the pipe with a short timeout. True = something holds the name
// (ours or foreign — callers must check state before attaching).
func Alive() bool {
	timeout := 2 * time.Second
	c, err := winio.DialPipe(PipeName, &timeout)
	if err != nil {
		return false
		}
	_ = c.Close()
	return true
}

// WaitAlive polls until the pipe answers or timeout elapses.
func WaitAlive(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if Alive() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// WaitDead polls until the pipe stops answering or timeout elapses.
func WaitDead(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !Alive() {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
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
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		go bridgeConn(pc, distro)
	}
}

func bridgeConn(pipe net.Conn, distro string) {
	defer pipe.Close()
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
	// Either direction finishing must tear the whole bridge down:
	// some `docker run` streams print output but keep the socket half-open,
	// which deadlocked the old sequential Wait (GH e2e hung 16min after
	// hello-world output). Kill socat as soon as one side ends.
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(toProc, pipe)
		_ = toProc.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(pipe, fromProc)
		done <- struct{}{}
	}()
	<-done
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
