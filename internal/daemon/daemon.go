//go:build windows

// Package daemon manages the background pipe holder (`dockup __serve`).
// No services, tasks, or autostart: a detached process with a PID in state.
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/userconfig"
	"golang.org/x/sys/windows"
)

// processAlive checks a PID via OpenProcess.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

// DaemonAlive reports whether the recorded daemon process is up.
func DaemonAlive(s state.State) bool {
	return processAlive(s.Daemon.PID)
}

// Start spawns detached `dockup __serve` and waits for the pipe.
func Start() error {
	ucfg, _ := userconfig.Load()
	pipe := ucfg.WithDefaults().EffectivePipe()
	var startErr error
	err := state.WithLock(func(s *state.State) error {
		if !s.Installed {
			return fmt.Errorf("dockup has not been setup yet run \"dockup setup\" to set it up")
		}
		if relay.AliveOn(pipe) && s.Daemon.PID == 0 {
			return fmt.Errorf("dockup is already running as a foreground process, ctrl + C it to stop it and re-run")
		}
		if s.Daemon.PID != 0 {
			if processAlive(s.Daemon.PID) {
				return fmt.Errorf("dockup is already running as a daemon process, run dockup daemon restart to restart it")
			}
			s.Daemon = state.Daemon{}
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		_ = os.MkdirAll(filepath.Dir(config.LogFile()), 0o755)
		logf, err := os.OpenFile(config.LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer func() { _ = logf.Close() }()
		cmd := exec.Command(exe, "__serve")
		cmd.Stdout = logf
		cmd.Stderr = logf
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start helper: %w", err)
		}
		pid := cmd.Process.Pid
		// Detach: parent does not Wait; child outlives us via no-window.
		_ = cmd.Process.Release()
		s.Daemon = state.Daemon{PID: pid, StartedAt: time.Now().UTC().Format(time.RFC3339)}
		return nil
	})
	if err != nil {
		return err
	}
	if !relay.WaitAliveOn(pipe, 15*time.Second) {
		_ = state.WithLock(func(s *state.State) error {
			s.Daemon = state.Daemon{}
			return nil
		})
		startErr = fmt.Errorf("helper failed to come up (pipe %s never answered)", pipe)
		return startErr
	}
	s, _ := state.Load()
	logx.Ok("dockup started (daemon pid %d, pipe %s)", s.Daemon.PID, pipe)
	if ucfg.WithDefaults().UseTCP {
		logx.Info("tcp bridge on %s", ucfg.WithDefaults().TCPAddr())
	}
	logx.Info("use: docker -H npipe:////./pipe/dockup_engine version")
	return nil
}

// Stop kills the daemon and waits for pipe death.
func Stop() error {
	var pid int
	err := state.WithLock(func(s *state.State) error {
		pid = s.Daemon.PID
		if pid == 0 {
			return fmt.Errorf("dockup daemon is not running")
		}
		if !processAlive(pid) {
			s.Daemon = state.Daemon{}
			pid = 0
			return fmt.Errorf("dockup daemon is not running (stale pid repaired)")
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		_ = proc.Kill()
		s.Daemon = state.Daemon{}
		return nil
	})
	if err != nil {
		return err
	}
	if pid != 0 {
		ucfg2, _ := userconfig.Load()
		relay.WaitDeadOn(ucfg2.WithDefaults().EffectivePipe(), 10*time.Second)
		logx.Ok("dockup stopped")
	}
	return nil
}

// Status prints brief health. Returns 0 running, 1 stopped.
func Status() int {
	s, _ := state.Load()
	ucfg, _ := userconfig.Load()
	ucfg = ucfg.WithDefaults()
	alive := relay.AliveOn(ucfg.EffectivePipe())
	daemonUp := processAlive(s.Daemon.PID)
	tcpNote := ""
	if ucfg.UseTCP {
		if relay.TCPAlive(ucfg.TCPAddr()) {
			tcpNote = ", tcp " + ucfg.TCPAddr() + " ok"
		} else {
			tcpNote = ", tcp " + ucfg.TCPAddr() + " down"
		}
	}
	switch {
	case daemonUp && alive:
		logx.Ok("running (daemon pid %d, pipe ok%s)", s.Daemon.PID, tcpNote)
		return 0
	case alive && s.Installed && s.Daemon.PID == 0:
		logx.Info("running (foreground process holds the pipe%s)", tcpNote)
		return 0
	case alive && !s.Installed:
		logx.Info("stopped (pipe held by another program, not dockup)")
		return 1
	case s.Installed && alive:
		logx.Info("running (pipe alive, owner unknown%s)", tcpNote)
		return 0
	default:
		if s.Daemon.PID != 0 && !daemonUp {
			logx.Info("stopped (stale daemon pid %d — run dockup doctor)", s.Daemon.PID)
		} else {
			logx.Info("stopped")
		}
		return 1
	}
}
