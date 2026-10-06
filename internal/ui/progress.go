// Progress is a themed byte-count bar for downloads, wrapping
// schollz/progressbar. It renders only when styled output is enabled;
// piped output and CI logs stay silent (completion is announced by the
// caller's own lines, e.g. the downloaded summary).
package ui

import (
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
)

// Progress tracks one download. Zero value is useless; use NewProgress.
type Progress struct {
	bar     *progressbar.ProgressBar
	enabled bool
}

// NewProgress builds a download bar for total bytes (-1 when unknown).
func NewProgress(total int64, desc string) *Progress {
	if !Enabled() {
		return &Progress{}
	}
	bar := progressbar.NewOptions64(total,
		progressbar.OptionSetDescription(White(desc)),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(24),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]█[reset]",
			SaucerHead:    "[green]█[reset]",
			SaucerPadding: "░",
			BarStart:      "",
			BarEnd:        "",
		}),
		progressbar.OptionEnableColorCodes(true),
	)
	return &Progress{bar: bar, enabled: true}
}

// Add advances the bar by n bytes (no-op when disabled).
func (p *Progress) Add(n int64) {
	if p.enabled {
		_ = p.bar.Add64(n)
	}
}

// Finish ends the bar with a newline (no-op when disabled).
func (p *Progress) Finish() {
	if p.enabled {
		_ = p.bar.Finish()
	}
}
