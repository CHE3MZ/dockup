// Package logx handles foreground prints and daemon file logging with rotation.
package logx

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/CHE3MZ/dockup/internal/config"
)

// Info prints to stdout.
func Info(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// Err prints to stderr with dockup prefix.
func Err(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "dockup: "+format+"\n", a...)
}

// Append writes to the daemon log, rotating when over config.MaxLogBytes.
func Append(msg string) {
	path := config.LogFile()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if st, err := os.Stat(path); err == nil && st.Size() > config.MaxLogBytes {
		rotate(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(msg + "\n")
}

func rotate(path string) {
	_ = os.Remove(path + ".3")
	_ = os.Rename(path+".2", path+".3")
	_ = os.Rename(path+".1", path+".2")
	_ = os.Rename(path, path+".1")
}
