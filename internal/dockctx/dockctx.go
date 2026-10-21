// Package dockctx manages a Docker CLI context named "dockup" that points at
// the relay pipe. It follows the pattern Colima uses for its profiles: setup
// creates the context so clients can select it without -H or DOCKER_HOST, and
// activation stays explicit (docker context use) because switching the user's
// default daemon is their call, not setup's.
package dockctx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// Name is the managed context name.
	Name = "dockup"
	// Description is shown by `docker context ls`.
	Description = "dockup Docker engine (WSL2)"
)

// Host converts a Windows pipe path to the Docker host URL.
// `\\.\pipe\dockup_engine` becomes `npipe:////./pipe/dockup_engine`.
func Host(pipe string) string {
	p := strings.TrimSpace(pipe)
	if strings.HasPrefix(p, "npipe://") {
		return p
	}
	p = strings.TrimPrefix(p, `\\.\pipe\`)
	return "npipe:////./pipe/" + p
}

// Info is a point-in-time view of the managed context.
type Info struct {
	CLI      bool   // docker CLI found on PATH
	Created  bool   // context exists
	Active   bool   // context is the current one
	Host     string // host configured in the context
	Current  string // current context name
	WantHost string // host the context should have
}

// Available reports whether the docker CLI is on PATH.
func Available() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

func run(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- fixed binary; argv is constants plus our own pipe host
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg != "" {
			return "", fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, msg)
		}
		return "", fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func inspect() (string, error) {
	return run("context", "inspect", Name, "--format", "{{.Endpoints.docker.Host}}")
}

// Get reports the managed context's state. Best-effort: failures to inspect
// simply mean the context is not usable (Created=false).
func Get(pipe string) Info {
	info := Info{CLI: Available(), WantHost: Host(pipe)}
	if !info.CLI {
		return info
	}
	if host, err := inspect(); err == nil {
		info.Created = true
		info.Host = host
	}
	if cur, err := run("context", "show"); err == nil {
		info.Current = cur
		info.Active = cur == Name
	}
	return info
}

// Ensure creates the context or updates it when the configured pipe changed.
// It reports whether anything was written.
func Ensure(pipe string) (bool, error) {
	if !Available() {
		return false, fmt.Errorf("docker CLI not found on PATH (install it from docker.com or with scoop)")
	}
	want := Host(pipe)
	if host, err := inspect(); err == nil {
		if host == want {
			return false, nil
		}
		if _, err := run("context", "rm", "-f", Name); err != nil {
			return false, err
		}
	}
	if _, err := run("context", "create", Name, "--description", Description, "--docker", "host="+want); err != nil {
		return false, err
	}
	return true, nil
}

// Use creates/updates the context and makes it Docker's current context.
func Use(pipe string) error {
	if _, err := Ensure(pipe); err != nil {
		return err
	}
	_, err := run("context", "use", Name)
	return err
}

// UseContext switches to an arbitrary existing context.
func UseContext(name string) error {
	_, err := run("context", "use", name)
	return err
}

// Exists reports whether a context is present.
func Exists(name string) bool {
	_, err := run("context", "inspect", name)
	return err == nil
}

// Remove deletes the managed context, but only when it points at pip. A
// context with our name pointing elsewhere belongs to the user, so it is
// left alone.
func Remove(pipe string) error {
	if !Available() {
		return nil
	}
	host, err := inspect()
	if err != nil || host != Host(pipe) {
		return nil
	}
	_, err = run("context", "rm", "-f", Name)
	return err
}
