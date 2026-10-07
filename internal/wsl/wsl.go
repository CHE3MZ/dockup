// Package wsl wraps wsl.exe interop. All execs have timeouts.
// wsl.exe list output is UTF-16LE; decode before parsing.
// Never parse stderr for control flow.
package wsl

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

func statFile(path string) (int64, error) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

// ParseListOutput decodes `wsl --list --quiet` output into distro names.
func ParseListOutput(data []byte) []string {
	text := DecodeWslOutput(data)
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "\x00\r"))
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// DecodeWslOutput handles UTF-16LE (wsl.exe default) and UTF-8.
func DecodeWslOutput(data []byte) string {
	if len(data) >= 2 && ((data[0] == 0xFF && data[1] == 0xFE) || bytes.Contains(data, []byte{0x00})) {
		u16 := make([]uint16, 0, len(data)/2)
		start := 0
		if data[0] == 0xFF && data[1] == 0xFE {
			start = 2
		}
		for i := start; i+1 < len(data); i += 2 {
			u16 = append(u16, uint16(data[i])|uint16(data[i+1])<<8)
		}
		return string(utf16.Decode(u16))
	}
	return string(data)
}

func runWsl(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", args...) // #nosec G204 -- fixed binary, argv never touches a shell
	detachConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("wsl %s: %w\n%s", strings.Join(args, " "), err, Tail(stderr.Bytes(), 2000))
	}
	return stdout.Bytes(), nil
}

// Tail returns the last n bytes as string.
func Tail(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[len(b)-n:])
}

// List returns all installed distro names.
func List() ([]string, error) {
	out, err := runWsl(context.Background(), "--list", "--quiet")
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return ParseListOutput(out), nil
}

// Exists reports whether a distro is installed.
func Exists(name string) bool {
	list, err := List()
	if err != nil {
		return false
	}
	for _, d := range list {
		if strings.EqualFold(strings.TrimSpace(d), name) {
			return true
		}
	}
	return false
}

// RunningSet returns currently-running distros.
func RunningSet() (map[string]bool, error) {
	out, err := runWsl(context.Background(), "--list", "--running", "--quiet")
	if err != nil && len(out) == 0 {
		return nil, err
	}
	set := map[string]bool{}
	for _, d := range ParseListOutput(out) {
		set[d] = true
	}
	return set, nil
}

// IsRunning reports whether a distro is running.
func IsRunning(name string) bool {
	set, err := RunningSet()
	if err != nil {
		return false
	}
	for k := range set {
		if strings.EqualFold(strings.TrimSpace(k), name) {
			return true
		}
	}
	return false
}

// Exec runs `wsl -d <distro> -u root -- <args...>` (quoteless one-liners only).
func Exec(distro string, timeout time.Duration, args ...string) ([]byte, error) {
	return ExecCtx(context.Background(), distro, timeout, args...)
}

// ExecCtx is Exec bound to a parent context: cancel kills the child instead
// of leaving the caller stuck until timeout. Used where Ctrl+C must stay
// responsive (foreground warmup); everything else keeps plain Exec.
func ExecCtx(ctx context.Context, distro string, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	full := append([]string{"-d", distro, "-u", "root", "--"}, args...)
	cmd := exec.CommandContext(ctx, "wsl.exe", full...) // #nosec G204 -- fixed binary, argv never touches a shell
	detachConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("wsl -d %s: %w\n%s", distro, err, Tail(stderr.Bytes(), 2000))
	}
	return stdout.Bytes(), nil
}

// ExecScript runs a multi-line shell script as root via stdin (`sh -s`).
// Mandatory for anything with quotes, $(), or redirections: wsl.exe
// corrupts those when passed as argv.
func ExecScript(distro string, timeout time.Duration, script string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "-d", distro, "-u", "root", "--", "sh", "-s") // #nosec G204 -- fixed binary and argv; script goes via stdin, never a shell
	detachConsole(cmd)
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("wsl -d %s script: %w\nstdout: %s\nstderr: %s",
			distro, err, Tail(stdout.Bytes(), 2000), Tail(stderr.Bytes(), 2000))
	}
	return stdout.Bytes(), nil
}

// RunRetry runs a script with retries for transient apt/mirror flakes.
func RunRetry(distro, what string, timeout time.Duration, tries int, script string) ([]byte, error) {
	var out []byte
	var err error
	for i := 1; i <= tries; i++ {
		out, err = ExecScript(distro, timeout, script)
		if err == nil {
			return out, nil
		}
		fmt.Fprintf(os.Stderr, "dockup: %s attempt %d/%d failed, retrying...\n%s\n", what, i, tries, Tail(out, 1500))
		time.Sleep(time.Duration(i) * 5 * time.Second)
	}
	return out, fmt.Errorf("%s failed after %d tries: %w\n%s", what, tries, err, Tail(out, 4000))
}

// Import runs `wsl --import <distro> <dir> <tar> --version 2`.
// Captures both stdout and stderr so GH logs show why imports fail
// (wsl prints progress/errors on stdout, not stderr).
func Import(distro, dir, tar string) error {
	fi, statErr := statFile(tar)
	if statErr != nil {
		return fmt.Errorf("wsl --import: tar not accessible %q: %w", tar, statErr)
	}
	if fi < 10<<20 {
		return fmt.Errorf("wsl --import: tar %q suspiciously small (%d bytes), re-download", tar, fi)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", "--import", distro, dir, tar, "--version", "2") // #nosec G204 -- fixed binary, argv never touches a shell
	detachConsole(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("wsl --import %q (%d bytes) into %q: %w\nstdout: %s\nstderr: %s",
			tar, fi, dir, err, Tail(stdout.Bytes(), 3000), Tail(stderr.Bytes(), 3000))
	}
	return nil
}

// Unregister runs `wsl --unregister <distro>`.
func Unregister(distro string) error {
	_, err := runWsl(context.Background(), "--unregister", distro)
	return err
}

// Terminate runs `wsl --terminate <distro>`.
func Terminate(distro string) error {
	_, err := runWsl(context.Background(), "--terminate", distro)
	return err
}

// Shell runs an interactive command attached to the caller's console (no
// args means a login shell). Stdin/stdout/stderr are inherited so editors,
// pagers, and job control behave natively; the first Ctrl+C is left to the
// child (otherwise the wrapper would die instead of the remote foreground
// job), a second one falls back to default and kills the wrapper.
// Returns the child's exit code (1 when unknown).
func Shell(distro string, args ...string) int {
	full := []string{"-d", distro, "-u", "root"}
	if len(args) > 0 {
		full = append(full, "--")
		full = append(full, args...)
	}
	cmd := exec.Command("wsl.exe", full...) // #nosec G204 -- fixed binary; trailing args are the user's explicit remote command, never a local shell
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "dockup: cannot open shell: %v\n", err)
		return 1
	}
	go func() {
		<-sig
		signal.Stop(sig)
	}()
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 1
	}
	return 0
}
