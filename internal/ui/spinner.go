// Spinner is a single-line indeterminate progress indicator for long steps
// (import, apt install) that cannot report real percentages.
//
// Interactive terminals get a live rewrite: `\r<text> <frame> (<elapsed>s)`
// ticking until Done prints `\r<text> done (<elapsed>s)`. Anywhere colors are
// disabled (piped output, CI logs) it degrades to two plain lines — start and
// done — so logs stay grep-able.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// spinnerFrames are ASCII-only so legacy consoles render them.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// spinnerFrame cycles frames (pure, unit-tested).
func spinnerFrame(i int) string { return spinnerFrames[i%len(spinnerFrames)] }

// Spinner tracks one running step. Zero value is useless; use NewSpinner.
type Spinner struct {
	out   io.Writer
	text  string
	start time.Time
	stop  chan struct{}
	once  sync.Once
	wg    sync.WaitGroup
}

// NewSpinner builds a spinner writing to out (nil means stdout).
func NewSpinner(out io.Writer, text string) *Spinner {
	if out == nil {
		out = os.Stdout
	}
	return &Spinner{out: out, text: text, stop: make(chan struct{})}
}

// put writes progress output; errors are dropped (best-effort display).
func (s *Spinner) put(format string, a ...any) {
	_, _ = fmt.Fprintf(s.out, format, a...)
}

// Start prints the step and begins ticking (no-op second call).
func (s *Spinner) Start() {
	s.once.Do(func() {
		s.start = time.Now()
		if !Enabled() {
			s.put("%s\n", White(s.text))
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			tick := time.NewTicker(150 * time.Millisecond)
			defer tick.Stop()
			for i := 0; ; i++ {
				select {
				case <-s.stop:
					return
				case <-tick.C:
					s.put("\r%s %s (%ds)",
						White(s.text), spinnerFrame(i), int(time.Since(s.start).Seconds()))
				}
			}
		}()
	})
}

// halt stops ticking; true if this call stopped it.
func (s *Spinner) halt() bool {
	select {
	case <-s.stop:
		return false
	default:
		close(s.stop)
	}
	s.wg.Wait()
	return true
}

// Done ends the step as complete.
func (s *Spinner) Done() {
	live := s.halt()
	secs := int(time.Since(s.start).Seconds())
	if !Enabled() {
		if live {
			s.put("%s done\n", White(s.text))
		}
		return
	}
	s.put("\r%s done (%ds)\n", White(s.text), secs)
}

// Stop ends the step without the done line (caller reports the failure).
func (s *Spinner) Stop() {
	if !s.halt() {
		return
	}
	if Enabled() {
		s.put("\n")
	}
}
