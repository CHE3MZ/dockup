// Package doctor runs preflight checks and repairs stale state.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Run checks everything, repairs stale daemon PIDs, reports fixed vs attention.
func Run() error {
	fail := 0
	ok := func(name string) { logx.Info("ok: %s", name) }
	bad := func(name, hint string) {
		fail++
		logx.Err("fail: %s — %s", name, hint)
	}

	// WSL present?
	if _, err := exec.LookPath("wsl.exe"); err != nil {
		bad("wsl.exe present", "install WSL2 (wsl --install)")
	} else {
		ok("wsl.exe present")
	}
	// Distro exists?
	s, _ := state.Load()
	if wsl.Exists(config.DistroName) {
		ok("distro dockup installed")
	} else if s.Installed {
		bad("distro dockup installed", "state says installed but WSL has no dockup distro — run dockup setup")
	} else {
		logx.Info("info: distro dockup not installed (run dockup setup)")
	}
	// Running?
	if wsl.IsRunning(config.DistroName) {
		ok("distro dockup running")
	} else {
		logx.Info("info: distro dockup stopped")
	}
	// systemd PID 1?
	if wsl.Exists(config.DistroName) && wsl.IsRunning(config.DistroName) {
		if out, err := wsl.Exec(config.DistroName, 15*time.Second, "sh", "-c", "ps -p 1 -o comm="); err == nil {
			if contains(string(out), "systemd") {
				ok("systemd is PID 1")
			} else {
				bad("systemd is PID 1", "got "+trim(string(out))+" — reinstall via dockup setup")
			}
		}
		// docker socket?
		if out, err := wsl.Exec(config.DistroName, 15*time.Second, "sh", "-c", "test -S /var/run/docker.sock && echo ok"); err == nil && contains(string(out), "ok") {
			ok("docker socket present")
		} else {
			logx.Info("info: docker socket absent (distro stopped or docker down)")
		}
	}
	// Pipe? Distinguish ours vs foreign (e.g. Docker Desktop preinstalled on GH runners).
	if relay.Alive() {
		if s.Installed {
			ok("pipe " + config.PipeName + " alive")
		} else {
			logx.Info("info: pipe %s held by another program (not dockup) — stop it before running dockup", config.PipeName)
		}
	} else {
		logx.Info("info: pipe %s not held (dockup stopped)", config.PipeName)
	}
	// docker CLI on Windows?
	if _, err := exec.LookPath("docker.exe"); err != nil {
		logx.Info("info: docker.exe not on PATH (install from docker.com; dockup does not bundle it)")
	} else {
		ok("docker.exe on PATH")
	}
	// Stale daemon pid repair.
	if s.Daemon.PID != 0 && !relay.Alive() {
		proc, err := os.FindProcess(s.Daemon.PID)
		_ = proc
		_ = err
		// Best-effort: if pipe dead, clear stale pid.
		_ = state.WithLock(func(ns *state.State) error {
			if ns.Daemon.PID == s.Daemon.PID {
				ns.Daemon = state.Daemon{}
			}
			return nil
		})
		logx.Info("fixed: cleared stale daemon pid %d", s.Daemon.PID)
	}
	if fail > 0 {
		return fmt.Errorf("%d critical check(s) failed", fail)
	}
	logx.Info("doctor: all critical checks passed")
	return nil
}

func contains(hay, needle string) bool {
	return len(hay) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(hay); i++ {
			if hay[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func trim(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' || s[i] == 0) {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\n' || s[j-1] == '\r' || s[j-1] == '\t' || s[j-1] == 0) {
		j--
	}
	return s[i:j]
}
