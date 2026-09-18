// Package relay serves \\.\pipe\docker_engine (go-winio) and proxies
// raw bytes to 127.0.0.1:<port>. Raw byte-copy supports HTTP hijack
// (run -it, logs -f, exec). Windows-only.
package relay

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

const PipeName = `\\.\pipe\docker_engine`

// Alive dials the pipe with a short timeout. True = relay already up
// (double-start guard).
func Alive() bool {
	timeout := 2 * time.Second
	c, err := winio.DialPipe(PipeName, &timeout)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Serve blocks accepting pipe clients and proxying each to
// 127.0.0.1:targetPort. Stops on ctx cancel. One goroutine per connection.
func Serve(ctx context.Context, targetPort int) error {
	if targetPort <= 0 || targetPort > 65535 {
		return fmt.Errorf("bad target port %d", targetPort)
	}
	target := fmt.Sprintf("127.0.0.1:%d", targetPort)
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
		go proxyConn(pc, target)
	}
}

func proxyConn(pipe net.Conn, target string) {
	defer pipe.Close()
	backend, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return
	}
	defer backend.Close()
	// Raw copy both directions; hijacked streams stay open.
	go func() {
		_, _ = io.Copy(backend, pipe)
		// Half-close backend write side if supported.
		if tc, ok := backend.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	_, _ = io.Copy(pipe, backend)
}
