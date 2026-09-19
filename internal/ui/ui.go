// Package ui provides clean CLI formatting: normal/bold text plus a small
// color palette (white, light blue, gray, yellow, red, green).
// Colors are enabled by default for interactive terminals and automatically
// disabled when piped, dumb terminals, NO_COLOR, or CI environments so logs
// stay clean. The config.json "color" option can also force it off.
package ui

import (
	"fmt"
	"os"
	"strings"
)

const (
	esc   = "\x1b["
	reset = esc + "0m"
	bold  = esc + "1m"

	white     = esc + "37m"
	lightBlue = esc + "94m" // bright blue, not dark blue
	gray      = esc + "90m"
	yellow    = esc + "33m"
	red       = esc + "31m"
	green     = esc + "32m"
	cyan      = esc + "96m"
)

var enabled = autoEnabled()

func autoEnabled() bool {
	if v := os.Getenv("NO_COLOR"); v != "" {
		return false
	}
	if term := os.Getenv("TERM"); strings.EqualFold(term, "dumb") {
		return false
	}
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		return false
	}
	return true
}

// SetEnabled forces color output on or off (used by config + tests).
func SetEnabled(v bool) { enabled = v }

// Enabled reports whether styling is currently on.
func Enabled() bool { return enabled }

func paint(code, s string) string {
	if !enabled {
		return s
	}
	return code + s + reset
}

// Bold renders bold text.
func Bold(s string) string { return paint(bold, s) }

// White renders normal white text.
func White(s string) string { return paint(white, s) }

// LightBlue renders light-blue text (help headers, accents).
func LightBlue(s string) string { return paint(lightBlue, s) }

// Cyan renders cyan text (values, paths, URLs).
func Cyan(s string) string { return paint(cyan, s) }

// Gray renders muted gray text (hints, secondary info).
func Gray(s string) string { return paint(gray, s) }

// Yellow renders yellow text (warnings).
func Yellow(s string) string { return paint(yellow, s) }

// Red renders red text (errors).
func Red(s string) string { return paint(red, s) }

// Green renders green text (success).
func Green(s string) string { return paint(green, s) }

// Header renders a section header: bold light-blue.
func Header(s string) string {
	if !enabled {
		return s
	}
	return bold + lightBlue + s + reset
}

// Printf writes formatted normal text to stdout.
func Printf(format string, a ...any) {
	fmt.Printf("%s\n", White(fmt.Sprintf(format, a...)))
}

// PrintBold writes bold text to stdout.
func PrintBold(format string, a ...any) {
	fmt.Printf("%s\n", Bold(fmt.Sprintf(format, a...)))
}

// Success writes a green success line to stdout.
func Success(format string, a ...any) {
	fmt.Printf("%s\n", Green(fmt.Sprintf(format, a...)))
}

// Hint writes a gray hint line to stdout.
func Hint(format string, a ...any) {
	fmt.Printf("%s\n", Gray(fmt.Sprintf(format, a...)))
}

// Warn writes a yellow warning line to stderr.
func Warn(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s\n", Yellow("warning: "+fmt.Sprintf(format, a...)))
}

// Error writes a red error line to stderr.
func Error(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "%s\n", Red("dockup: "+fmt.Sprintf(format, a...)))
}
