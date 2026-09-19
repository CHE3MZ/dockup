// Package doctor runs preflight checks and repairs stale state.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/autostart"
	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/docker"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/userconfig"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Run checks everything, repairs stale daemon PIDs, reports fixed vs attention.
func Run() error { return RunEx(false) }

// RunEx is Run plus optional remediation: with fix=true a missing or broken
// in-distro engine is reinstalled/reconfigured (repo, packages, systemd
// units, wsl.conf) and re-verified.
func RunEx(fix bool) error {
	fail := 0
	ok := func(name string) { logx.Ok("ok: %s", name) }
	bad := func(name, hint string) {
		fail++
		logx.Err("fail: %s — %s", name, hint)
	}

	// User config (~/.dockup/config.json)?
	ucfg, err := userconfig.Load()
	if err != nil {
		bad("config file", fmt.Sprintf("%s is corrupt: %v", userconfig.File(), err))
	} else {
		ucfg = ucfg.WithDefaults()
		ok(fmt.Sprintf("config file %s", userconfig.File()))
		if _, err := userconfig.NormalizePath(ucfg.DefaultPath); err != nil {
			bad("default_path", err.Error())
		} else {
			ok(fmt.Sprintf("default_path %s", ucfg.DefaultPath))
		}
		if ucfg.CurrentPath != "" {
			if _, err := userconfig.NormalizePath(ucfg.CurrentPath); err != nil {
				bad("current_path", err.Error())
			} else {
				ok(fmt.Sprintf("current_path %s", ucfg.CurrentPath))
			}
		} else {
			logx.Info("info: current_path empty (no distro installed)")
		}
		if err := userconfig.ValidatePort(ucfg.EffectivePort()); err != nil {
			bad("port", err.Error())
		} else if ucfg.UseTCP {
			ok(fmt.Sprintf("tcp bridge %s enabled", ucfg.TCPAddr()))
		} else {
			logx.Info("info: tcp bridge disabled (port %d reserved)", ucfg.EffectivePort())
		}
		autostartText := "off"
		if ucfg.Autostart {
			autostartText = "on"
		}
		ok(fmt.Sprintf("autostart %s", autostartText))
		// Reconcile the Startup entry with the setting (info-level only).
		if ucfg.Autostart && !autostart.IsEnabled() {
			logx.Info("info: autostart on but no Startup entry — run dockup daemon autostart on to repair")
		}
		if !ucfg.Autostart && autostart.IsEnabled() {
			logx.Info("info: Startup entry present but autostart off — run dockup daemon autostart off to remove")
		}
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
	pipe := config.PipeName
	if c, lerr := userconfig.Load(); lerr == nil {
		pipe = c.WithDefaults().EffectivePipe()
	}
	if relay.AliveOn(pipe) {
		if s.Installed {
			ok("pipe " + pipe + " alive")
		} else {
			logx.Info("info: pipe %s held by another program (not dockup) — stop it before running dockup", pipe)
		}
	} else {
		logx.Info("info: pipe %s not held (dockup stopped)", pipe)
	}
	// TCP bridge?
	if c, lerr := userconfig.Load(); lerr == nil && c.WithDefaults().UseTCP {
		addr := c.WithDefaults().TCPAddr()
		if relay.TCPAlive(addr) {
			ok("tcp " + addr + " alive")
		} else {
			logx.Info("info: tcp %s not held (dockup stopped or tcp starting)", addr)
		}
	}
	// docker CLI on Windows?
	if _, err := exec.LookPath("docker.exe"); err != nil {
		logx.Info("info: docker.exe not on PATH (install from docker.com; dockup does not bundle it)")
	} else {
		ok("docker.exe on PATH")
	}
	// Stale daemon pid repair.
	if s.Daemon.PID != 0 && !relay.AliveOn(pipe) {
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
	// Stale install record repair, but only on a CONFIRMED-absent distro:
	// wsl.Exists is blind to list errors, and clearing installed on a
	// transient wsl.exe failure would be exactly the false-positive we
	// must avoid.
	if distroList, lerr := wsl.List(); lerr == nil {
		found := false
		for _, d := range distroList {
			if strings.EqualFold(strings.TrimSpace(d), config.DistroName) {
				found = true
				break
			}
		}
		if !found {
			if c, cerr := userconfig.Load(); cerr == nil {
				fixed := false
				if c.CurrentPath != "" {
					c.CurrentPath = ""
					fixed = true
					logx.Info("fixed: cleared stale current_path")
				}
				if c.Installed {
					c.Installed = false
					fixed = true
					logx.Info("fixed: cleared stale installed flag")
				}
				if fixed {
					_ = userconfig.Save(c.WithDefaults())
				}
			}
		}
	}
	// Remediation: reinstall/repair a broken in-distro engine.
	if fix && wsl.Exists(config.DistroName) {
		if ferr := fixDistro(); ferr != nil {
			return ferr
		}
	}
	if fail > 0 {
		return fmt.Errorf("%d critical check(s) failed", fail)
	}
	logx.Info("doctor: all critical checks passed")
	return nil
}

// fixDistro restores dockup's internal files when they are missing, broken,
// or misconfigured: wsl.conf (systemd), the Docker apt repo, the engine
// packages, and the systemd units. It re-verifies the daemon afterwards.
func fixDistro() error {
	distro := config.DistroName
	if err := docker.TestDaemon(distro); err == nil {
		logx.Ok("fix: engine already healthy, nothing to reinstall")
	} else {
		logx.Warn("engine unhealthy (%v), reinstalling", err)
		if _, err := wsl.ExecScript(distro, 60*time.Second, docker.PreflightScript); err != nil {
			return fmt.Errorf("fix: kernel preflight failed: %w", err)
		}
		if _, err := wsl.RunRetry(distro, "fix install docker", 10*time.Minute, 2, docker.InstallScript); err != nil {
			return fmt.Errorf("fix: reinstall failed: %w", err)
		}
		logx.Ok("fix: engine packages reinstalled")
	}
	if _, err := wsl.ExecScript(distro, 3*time.Minute, docker.ConfigureScript); err != nil {
		return fmt.Errorf("fix: reconfigure failed: %w", err)
	}
	_ = wsl.Terminate(distro)
	time.Sleep(3 * time.Second)
	if err := docker.TestDaemon(distro); err != nil {
		return fmt.Errorf("fix: engine still unhealthy after repair: %w", err)
	}
	logx.Ok("fix: engine restored and answering")
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
