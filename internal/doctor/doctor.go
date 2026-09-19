// Package doctor implements `dockup doctor` preflight + health checks.
package doctor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/CHE3MZ/dockup/internal/distro"
	"github.com/CHE3MZ/dockup/internal/distro/alpine"
	"github.com/CHE3MZ/dockup/internal/distro/debian"
	"github.com/CHE3MZ/dockup/internal/dockercli"
	"github.com/CHE3MZ/dockup/internal/engine"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Run prints checks, returns non-zero-worthy error if critical fails.
func Run() error {
	fmt.Printf("dockup doctor (%s/%s)\n", runtime.GOOS, runtime.GOARCH)
	fails := 0
	check := func(name string, ok bool, detail string) {
		status := "ok"
		if !ok {
			status = "FAIL"
			fails++
		}
		fmt.Printf("  [%s] %s %s\n", status, name, detail)
	}
	info := func(name string, detail string) {
		fmt.Printf("  [info] %s %s\n", name, detail)
	}

	cliPath, err := dockercli.Path()
	check("docker CLI", err == nil, firstNonEmpty(cliPath, dockercli.InstallHint))
	check("docker compose", dockercli.ComposeCheck(), "")

	distros, err := wsl.List()
	check("wsl --list", err == nil, fmt.Sprintf("(%d distros)", len(distros)))

	s, err := state.Load()
	if err != nil {
		check("state file", false, err.Error())
		s = &state.State{Distros: map[string]state.DistroState{}}
	} else {
		check("state file", true, fmt.Sprintf("(default=%q active=%q)", s.Default, s.ActiveDistro))
	}
	info("wsl distros:", fmt.Sprintf("%v", distros))
	if s.Default == "" {
		info("default:", "(none — run: dockup default <name>)")
	}

	// End-to-end: if the relay is up, prove `docker version` answers through it.
	if s.ActiveDistro != "" {
		up := relay.Alive()
		check("relay pipe", up, fmt.Sprintf("(active=%q)", s.ActiveDistro))
		if up {
			ver, verr := serverVersion()
			check("engine via relay", verr == nil, firstNonEmpty("("+ver+")", "docker version failed"))
		}
	}

	// Kernel preflight per configured distro (catches custom-kernel issues
	// before setup/run rather than mid-install).
	for name := range s.Distros {
		switch distro.Detect(name) {
		case distro.Debian:
			err := debian.Preflight(name)
			check("kernel "+name, err == nil, firstNonEmpty("(iptables+overlayfs)", errString(err)))
		case distro.Alpine:
			err := alpine.Preflight(name)
			check("kernel "+name, err == nil, firstNonEmpty("(iptables+overlayfs)", errString(err)))
		default:
			info("kernel "+name+":", "(unknown family, skipped)")
		}
	}

	if fails > 0 {
		return fmt.Errorf("%d check(s) failed", fails)
	}
	return nil
}

// serverVersion runs `docker --host <pipe> version` explicitly (no env needed).
func serverVersion() (string, error) {
	cli, err := dockercli.Path()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "--host", "npipe:////./pipe/docker_engine",
		"version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		return "", err
	}
	return string(engine.TrimOutput(out)), nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
