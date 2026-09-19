// Package logx handles foreground prints and daemon file logging with rotation.
// Info is plain white, Ok is green (success), Warn is yellow, Err is red.
package logx

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/ui"
)

// Info prints to stdout.
func Info(format string, a ...any) {
	fmt.Printf("%s\n", ui.White(fmt.Sprintf(format, a...)))
}

// Ok prints a green success line to stdout.
func Ok(format string, a ...any) {
	fmt.Printf("%s\n", ui.Green(fmt.Sprintf(format, a...)))
}

// Warn prints a yellow warning to stderr.
func Warn(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s\n", ui.Yellow("warning: "+fmt.Sprintf(format, a...)))
}

// Err prints to stderr with dockup prefix in red.
func Err(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s\n", ui.Red("dockup: "+fmt.Sprintf(format, a...)))
}

// Append writes to the daemon log, rotating when over config.MaxLogBytes.
func Append(msg string) {
	path := config.LogFile()
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	if st, err := os.Stat(path); err == nil && st.Size() > config.MaxLogBytes {
		rotate(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- path is our own log file location, never remote input
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(msg + "\n")
}

func rotate(path string) {
	_ = os.Remove(path + ".3")
	_ = os.Rename(path+".2", path+".3")
	_ = os.Rename(path+".1", path+".2")
	_ = os.Rename(path, path+".1")
}
