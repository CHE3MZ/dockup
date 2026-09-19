//go:build windows

package relay

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
)

// TestMessageModeCloseWriteEOF proves the mechanism our stdin-EOF fix
// relies on: a client CloseWrite on a message-mode pipe must surface as
// io.EOF on the server side after all bytes are delivered.
func TestMessageModeCloseWriteEOF(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\dockup-test-msgmode-%d`, os.Getpid())
	l, err := winio.ListenPipe(name, &winio.PipeConfig{MessageMode: true})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = l.Close() }()

	type result struct {
		data string
		eof  bool
		err  error
	}
	got := make(chan result, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			got <- result{err: err}
			return
		}
		defer func() { _ = conn.Close() }()
		var data []byte
		buf := make([]byte, 64)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				data = append(data, buf[:n]...)
			}
			if err == io.EOF {
				got <- result{data: string(data), eof: true}
				return
			}
			if err != nil {
				got <- result{data: string(data), err: err}
				return
			}
		}
	}()

	timeout := 5 * time.Second
	c, err := winio.DialPipe(name, &timeout)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Write([]byte("probe-eof\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	cw, ok := c.(interface{ CloseWrite() error })
	if !ok {
		t.Fatal("client conn does not support CloseWrite")
	}
	if err := cw.CloseWrite(); err != nil {
		t.Fatalf("closewrite: %v", err)
	}

	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("server error: %v", r.err)
		}
		if !r.eof {
			t.Fatalf("server never saw EOF, data=%q", r.data)
		}
		if r.data != "probe-eof\n" {
			t.Fatalf("server data=%q, want %q", r.data, "probe-eof\n")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for server EOF")
	}
}
