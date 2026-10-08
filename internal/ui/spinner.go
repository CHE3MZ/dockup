// Spinner is a single-line indeterminate progress indicator for long steps
// (import, apt install) that cannot report real percentages. It wraps
// briandowns/spinner: interactive terminals get a live braille animation,
// while piped output and CI logs degrade to two plain lines (start + done)
// so logs stay grep-able.
package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	bspinner "github.com/briandowns/spinner"
)

// Spinner tracks one running step. Zero value is useless; use NewSpinner.
// Start/Done/Stop are safe for concurrent use: an interrupt watcher may stop
// the animation at the same instant the main flow finishes it.
type Spinner struct {
	out      io.Writer
	text     string
	lib      *bspinner.Spinner
	once     sync.Once
	mu       sync.Mutex
	started  bool
	finished bool
}

// NewSpinner builds a spinner writing to out (nil means stdout).
func NewSpinner(out io.Writer, text string) *Spinner {
	if out == nil {
		out = os.Stdout
	}
	return &Spinner{out: out, text: text}
}

// put writes progress output; errors are dropped (best-effort display).
func (s *Spinner) put(format string, a ...any) {
	_, _ = fmt.Fprintf(s.out, format, a...)
}

// Start prints the step and begins animating (no-op second call).
func (s *Spinner) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.once.Do(func() {
		s.started = true
		if !Enabled() {
			s.put("%s\n", White(s.text))
			return
		}
		// Braille dots: the house style (plain ASCII looked cheap here).
		// Non-terminals get plain lines regardless of charset.
		s.lib = bspinner.New(bspinner.CharSets[14], 100*time.Millisecond,
			bspinner.WithWriter(s.out),
			bspinner.WithColor("green"),
			bspinner.WithSuffix(" "+s.text))
		s.lib.Start()
	})
}

// Done ends the step as complete.
func (s *Spinner) Done() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.finished = true
	if s.lib != nil {
		s.lib.FinalMSG = White("✓ "+s.text+" done") + "\n"
		s.lib.Stop()
		return
	}
	if s.started {
		s.put("%s\n", White("✓ "+s.text+" done"))
	}
}

// Stop ends the step without the done line (caller reports the failure).
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.finished = true
	if s.lib != nil {
		s.lib.Stop()
	}
}
