// Command dockup is the single static Windows CLI per revision.md.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/daemon"
	"github.com/CHE3MZ/dockup/internal/doctor"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/setup"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

var version = config.Version

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return foreground()
	}
	switch args[0] {
	case "--help", "-h", "help":
		usage()
		return 0
	case "version":
		fmt.Printf("you're running the %s version.\n", version)
		return 0
	case "setup":
		amd, arm := false, false
		for _, a := range args[1:] {
			switch a {
			case "--amd":
				amd = true
			case "--arm":
				arm = true
			default:
				logx.Err("unknown setup flag %q (try --amd or --arm)", a)
				return 1
			}
		}
		arch, msg := config.NormalizeArch(amd, arm)
		if msg != "" {
			logx.Err("%s", msg)
			return 1
		}
		return setup.Run(arch)
	case "uninstall":
		return setup.Uninstall()
	case "ps":
		return cmdPs()
	case "daemon":
		return cmdDaemon(args[1:])
	case "shutdown":
		return cmdShutdown()
	case "doctor":
		if err := doctor.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		return 0
	case "__serve":
		return serveForever()
	default:
		fmt.Fprintf(os.Stderr, "dockup: unknown command %q (try --help)\n", args[0])
		return 1
	}
}

func usage() {
	fmt.Print(`dockup — docker engine in a dedicated WSL distro (Windows only)

Usage:
  dockup                  foreground run (Ctrl+C to stop)
  dockup setup [--amd|--arm]
  dockup uninstall
  dockup ps
  dockup daemon start|stop|restart|status|log
  dockup shutdown           stop everything
  dockup doctor             preflight + repair stale state
  dockup version
`)
}

// foreground starts the relay inline. Refuses if already running or not setup.
func foreground() int {
	s, _ := state.Load()
	if !s.Installed && !wsl.Exists(config.DistroName) {
		fmt.Fprintln(os.Stderr, "dockup has not been setup yet run \"dockup setup\" to set it up.")
		return 1
	}
	if s.Daemon.PID != 0 && daemon.DaemonAlive(s) {
		fmt.Fprintln(os.Stderr, "dockup is already running as a daemon process, run dockup daemon stop first.")
		return 1
	}
	if relay.Alive() {
		fmt.Fprintln(os.Stderr, "dockup is already running (another foreground or daemon holds the pipe).")
		return 1
	}
	fmt.Printf("starting helper...\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- relay.Serve(ctx, config.DistroName)
	}()
	if !relay.WaitAlive(10 * time.Second) {
		select {
		case err := <-errCh:
			fmt.Fprintf(os.Stderr, "dockup: helper failed: %v\n", err)
		default:
			fmt.Fprintln(os.Stderr, "dockup: helper failed (pipe never came up)")
		}
		cancel()
		return 1
	}
	fmt.Printf("helper up and running on %s\n", config.PipeName)
	fmt.Printf("dockup running in foreground (Ctrl+C to stop)...\n")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	select {
	case <-sig:
	case err := <-errCh:
		if err != nil {
			fmt.Fprintf(os.Stderr, "dockup: helper failed: %v\n", err)
			return 1
		}
	}
	cancel()
	_ = relay.WaitDead(5 * time.Second)
	fmt.Printf("dockup stopped\n")
	return 0
}

// serveForever is the hidden daemon child holding the pipe.
func serveForever() int {
	s, _ := state.Load()
	if !s.Installed {
		fmt.Fprintln(os.Stderr, "dockup: helper failed (not setup)")
		return 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		cancel()
	}()
	if err := relay.Serve(ctx, config.DistroName); err != nil {
		fmt.Fprintf(os.Stderr, "dockup: helper failed: %v\n", err)
		return 1
	}
	return 0
}

func cmdPs() int {
	s, _ := state.Load()
	if relay.Alive() {
		if s.Daemon.PID != 0 && daemon.DaemonAlive(s) {
			fmt.Printf("running (daemon pid %d)\n", s.Daemon.PID)
		} else {
			fmt.Printf("running (foreground)\n")
		}
		return 0
	}
	fmt.Printf("stopped\n")
	return 0
}

func cmdDaemon(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "dockup: daemon needs start|stop|restart|status|log")
		return 1
	}
	// If foreground holds the pipe, daemon cmds (except status/log) refuse.
	s, _ := state.Load()
	if relay.Alive() && s.Daemon.PID == 0 && !daemon.DaemonAlive(s) {
		if args[0] == "start" {
			fmt.Fprintln(os.Stderr, "dockup is already running as a foreground process, ctrl + C in order to use the dockup daemon.")
			return 1
		}
	}
	switch args[0] {
	case "start":
		if err := daemon.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		return 0
	case "stop":
		if err := daemon.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		return 0
	case "restart":
		_ = daemon.Stop()
		time.Sleep(1 * time.Second)
		if err := daemon.Start(); err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		return 0
	case "status":
		return daemon.Status()
	case "log":
		return daemonLog()
	default:
		fmt.Fprintf(os.Stderr, "dockup: unknown daemon command %q\n", args[0])
		return 1
	}
}

// daemonLog tails the log file read-only until Ctrl+C.
func daemonLog() int {
	path := config.LogFile()
	fmt.Printf("showing %s (read-only, Ctrl+C to exit)...\n", path)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	var offset int64
	if st, err := os.Stat(path); err == nil {
		offset = st.Size()
		if offset > 8000 {
			offset -= 8000
		}
	}
	for {
		select {
		case <-sig:
			return 0
		default:
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "dockup: no log yet (%v)\n", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if int64(len(data)) > offset {
			chunk := string(data[offset:])
			if !strings.HasSuffix(chunk, "\n") {
				chunk += "\n"
			}
			fmt.Print(chunk)
			offset = int64(len(data))
		}
		time.Sleep(1 * time.Second)
	}
}

func cmdShutdown() int {
	_ = daemon.Stop()
	_ = wsl.Terminate(config.DistroName)
	if relay.Alive() {
		fmt.Fprintln(os.Stderr, "dockup: pipe still held (a foreground dockup may still be running — Ctrl+C it)")
		return 1
	}
	fmt.Printf("dockup shutdown complete (stopped)\n")
	return 0
}
