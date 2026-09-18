// Package cli parses args and dispatches subcommands.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CHE3MZ/dockup/internal/distro/alpine"
	"github.com/CHE3MZ/dockup/internal/distro/debian"

	"github.com/CHE3MZ/dockup/internal/distro"
	"github.com/CHE3MZ/dockup/internal/dockercli"
	"github.com/CHE3MZ/dockup/internal/doctor"
	"github.com/CHE3MZ/dockup/internal/engine"
	"github.com/CHE3MZ/dockup/internal/picker"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Run dispatches. Returns process exit code.
func Run(args []string, version, commit string) int {
	// Global flags: -d|--distro <name>, --port <n>.
	distroFlag, portFlag, rest := parseGlobals(args)

	if len(rest) == 0 {
		// Bare foreground run.
		return bareRun(distroFlag, portFlag)
	}

	switch rest[0] {
	case "--help", "-h", "help":
		usage()
		return 0
	case "version":
		fmt.Printf("dockup %s (windows/amd64, commit %s)\n", version, commit)
		return 0
	case "doctor":
		if err := doctor.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		return 0
	case "list", "ls":
		return cmdList()
	case "default":
		return cmdDefault(rest[1:])
	case "env":
		return cmdEnv(rest[1:])
	case "ps":
		return cmdPs()
	case "setup":
		return cmdSetup(rest[1:])
	case "revert":
		return cmdRevert(rest[1:])
	case "cleanup":
		return cmdCleanup()
	case "shutdown":
		return cmdShutdown()
	case "daemon":
		return cmdDaemon(distroFlag, rest[1:])
	default:
		fmt.Fprintf(os.Stderr, "dockup: unknown command %q (try --help)\n", rest[0])
		return 1
	}
}

func parseGlobals(args []string) (distro string, port int, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-d" || a == "--distro":
			if i+1 < len(args) {
				distro = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--distro="):
			distro = strings.TrimPrefix(a, "--distro=")
		case a == "--port":
			if i+1 < len(args) {
				port, _ = strconv.Atoi(args[i+1])
				i++
			}
		case strings.HasPrefix(a, "--port="):
			port, _ = strconv.Atoi(strings.TrimPrefix(a, "--port="))
		default:
			rest = append(rest, a)
		}
	}
	return distro, port, rest
}

func usage() {
	fmt.Print(`dockup — manual Docker Engine in your own WSL2 distro (Windows only)

Usage:
  dockup [-d|--distro NAME] [--port N]     foreground run (single-active)
  dockup setup [NAME]        install engine in distro
  dockup revert [NAME]       remove what setup added (confirm, no CLI needed)
  dockup default [NAME]      set/show default distro
  dockup list|ls             configured distros
  dockup daemon <start|stop|restart|status> [-d NAME]
  dockup ps                  live dockup processes
  dockup shutdown            stop active set
  dockup env [--shell powershell|cmd]
  dockup cleanup             repair stale state (no CLI needed)
  dockup doctor              preflight checks (no CLI needed)
  dockup version
`)
}

// requireCLI enforces the docker-CLI policy: warn-and-continue=false means
// hard error (run/daemon/env/ps need it).
func requireCLI(warnOnly bool) bool {
	if dockercli.Check() {
		return true
	}
	fmt.Fprintln(os.Stderr, "dockup: "+dockercli.InstallHint)
	return warnOnly
}

func resolveDistro(flag string) (string, *state.State, error) {
	s, err := state.Load()
	if err != nil {
		return "", nil, err
	}
	if flag != "" {
		if _, ok := s.Distros[flag]; !ok {
			return "", nil, fmt.Errorf("distro %q is not configured (dockup setup first)", flag)
		}
		return flag, s, nil
	}
	if s.Default == "" {
		return "", nil, fmt.Errorf("no default distro (dockup default <name>)")
	}
	return s.Default, s, nil
}

// teardownDistro stops dockup-managed processes and, ONLY if dockup booted
// the distro, terminates it. Clears active/port/daemon bookkeeping.
// Best-effort: never fails, so teardown always converges.
func teardownDistro(s *state.State, name string) {
	d, ok := s.Distros[name]
	if !ok {
		s.ActiveDistro = ""
		return
	}
	_ = engine.Stop(name)
	if d.BootedByDockup {
		_ = wsl.Terminate(name)
	}
	d.RelayPort = 0
	d.WSLWasRunning = false
	d.BootedByDockup = false
	d.Daemon = state.DaemonInfo{}
	s.Distros[name] = d
	if s.ActiveDistro == name {
		s.ActiveDistro = ""
	}
}

func cmdList() int {
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if len(s.Distros) == 0 {
		fmt.Println("(no configured distros — run: dockup setup)")
		return 0
	}
	fmt.Printf("%-24s %-8s %-8s %-6s\n", "NAME", "DEFAULT", "ACTIVE", "PORT")
	for name, d := range s.Distros {
		def, act, port := "", "", ""
		if name == s.Default {
			def = "*"
		}
		if name == s.ActiveDistro {
			act = "yes"
			if d.RelayPort != 0 {
				port = strconv.Itoa(d.RelayPort)
			}
		}
		fmt.Printf("%-24s %-8s %-8s %-6s\n", name, def, act, port)
	}
	return 0
}

func cmdDefault(args []string) int {
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if len(args) == 0 {
		names := make([]string, 0, len(s.Distros))
		for n := range s.Distros {
			names = append(names, n)
		}
		if len(names) == 0 {
			fmt.Fprintln(os.Stderr, "dockup: no configured distros")
			return 1
		}
		name, err := picker.Pick("Choose default distro:", names)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		args = []string{name}
	}
	name := args[0]
	if err := state.Transaction(func(s *state.State) error {
		if _, ok := s.Distros[name]; !ok {
			return fmt.Errorf("distro %q is not configured", name)
		}
		s.Default = name
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	fmt.Println("default:", name)
	return 0
}

func cmdEnv(args []string) int {
	if !requireCLI(false) {
		return 1
	}
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if s.ActiveDistro == "" {
		fmt.Fprintln(os.Stderr, "dockup: no relay up (start dockup first)")
		return 1
	}
	if !relay.Alive() {
		fmt.Fprintln(os.Stderr, "dockup: relay pipe is down (run dockup cleanup, then start again)")
		return 1
	}
	shell := "powershell"
	for i := 0; i < len(args); i++ {
		if args[i] == "--shell" && i+1 < len(args) {
			shell = args[i+1]
		}
	}
	switch shell {
	case "cmd":
		fmt.Println(`set DOCKER_HOST=npipe:////./pipe/docker_engine`)
	default:
		fmt.Println(`$env:DOCKER_HOST='npipe:////./pipe/docker_engine'`)
	}
	return 0
}

func cmdPs() int {
	if !requireCLI(false) {
		return 1
	}
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if s.ActiveDistro == "" {
		fmt.Println("stopped (no active distro)")
		return 0
	}
	d, ok := s.Distros[s.ActiveDistro]
	if !ok || d.RelayPort == 0 {
		fmt.Printf("%-24s %s\n", s.ActiveDistro, "stopped (stale state, run dockup cleanup)")
		return 0
	}
	relayUp := engine.Healthy(d.RelayPort)
	sockUp := engine.SocketAlive(s.ActiveDistro)
	var status string
	switch {
	case relayUp && sockUp:
		status = fmt.Sprintf("running (port %d, pid %d, since %s)", d.RelayPort, d.Daemon.PID, d.Daemon.StartedAt)
	case relayUp:
		status = fmt.Sprintf("degraded (relay up on %d, engine not responding)", d.RelayPort)
	default:
		status = "stopped (relay down, run dockup cleanup)"
	}
	fmt.Printf("%-24s %s\n", s.ActiveDistro, status)
	return 0
}

func cmdSetup(args []string) int {
	requireCLI(true) // warn-and-continue.
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		all, err := wsl.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		s, _ := state.Load()
		var fresh []string
		for _, d := range all {
			if _, ok := s.Distros[d]; !ok {
				fresh = append(fresh, d)
			}
		}
		if len(fresh) == 0 {
			fmt.Fprintln(os.Stderr, "dockup: all distros already configured")
			return 1
		}
		name, err = picker.Pick("Choose distro to set up:", fresh)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
	}

	all, err := wsl.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	found := false
	for _, d := range all {
		if d == name {
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "dockup: distro %q not found (wsl --list)\n", name)
		return 1
	}
	if s, _ := state.Load(); s != nil {
		if _, ok := s.Distros[name]; ok {
			fmt.Fprintf(os.Stderr, "dockup: %q is already configured\n", name)
			return 1
		}
	}

	fam := distro.Detect(name)
	var snap state.InstalledByDockup
	var hadPrior bool
	switch fam {
	case distro.Debian:
		snap, hadPrior, err = debian.Setup(name)
	case distro.Alpine:
		snap, hadPrior, err = alpine.Setup(name)
	default:
		fmt.Fprintf(os.Stderr, "dockup: unsupported distro %q (need Debian/Ubuntu or Alpine)\n", name)
		return 1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}

	if err := state.Transaction(func(s *state.State) error {
		s.Distros[name] = state.DistroState{
			InstalledByDockup: snap,
			HadPriorDocker:    hadPrior,
		}
		if s.Default == "" {
			s.Default = name
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	fmt.Printf("setup %q done (family %s)\n", name, fam)
	if hadPrior {
		fmt.Fprintf(os.Stderr, "dockup: note: %q already had docker — revert will only remove what setup added\n", name)
	}
	return 0
}

func cmdRevert(args []string) int {
	var name string
	if len(args) > 0 {
		name = args[0]
	} else {
		s, err := state.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		var names []string
		for n := range s.Distros {
			names = append(names, n)
		}
		if len(names) == 0 {
			fmt.Fprintln(os.Stderr, "dockup: nothing configured")
			return 1
		}
		var err2 error
		name, err2 = picker.Pick("Choose configured distro to revert:", names)
		if err2 != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err2)
			return 1
		}
	}
	if !picker.Confirm("ARE YOU SURE you want to revert " + name + "? This removes only what setup added") {
		fmt.Println("aborted")
		return 0
	}
	fmt.Printf("revert %q: not yet implemented in this scaffold (P3)\n", name)
	return 0
}

func cmdCleanup() int {
	fmt.Println("cleanup: not yet implemented in this scaffold (P3)")
	return 0
}

func cmdShutdown() int {
	if !requireCLI(false) {
		return 1
	}
	fmt.Println("shutdown: not yet implemented in this scaffold (P2)")
	return 0
}

func cmdDaemon(globalDistro string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "dockup: daemon needs start|stop|restart|status")
		return 1
	}
	if !requireCLI(false) {
		return 1
	}
	fmt.Printf("daemon %s: not yet implemented in this scaffold (P2)\n", args[0])
	return 0
}

func bareRun(distroFlag string, portFlag int) int {
	if !requireCLI(false) {
		return 1
	}
	name, _, err := resolveDistro(distroFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}

	// Catch Ctrl-C from here on so teardown always runs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if s.ActiveDistro != "" && s.ActiveDistro != name {
		fmt.Fprintf(os.Stderr, "dockup: %q is active — stop/shutdown first (single-active)\n", s.ActiveDistro)
		return 1
	}
	if relay.Alive() {
		fmt.Printf("relay already up for %q (attaching, not double-starting)\n", name)
		fmt.Println(`point your shell at it: dockup env --shell powershell | Invoke-Expression`)
		return 0
	}

	port := portFlag
	if port == 0 {
		port, err = engine.PickFreePort()
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
	} else if port < 1 || port > 65535 {
		fmt.Fprintln(os.Stderr, "dockup: bad --port (1-65535)")
		return 1
	}

	running, err := wsl.Running()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	wasRunning := running[name]
	bootedByDockup := !wasRunning

	fmt.Printf("starting engine in %q (relay 127.0.0.1:%d)...\n", name, port)
	if err := engine.Start(name, port); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if err := engine.WaitSocket(name, 2*time.Minute); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		_ = engine.Stop(name)
		if bootedByDockup {
			_ = wsl.Terminate(name)
		}
		return 1
	}
	if err := engine.WaitRelay(port, 30*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		_ = engine.Stop(name)
		if bootedByDockup {
			_ = wsl.Terminate(name)
		}
		return 1
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)
	selfPID := os.Getpid()
	if err := state.Transaction(func(s *state.State) error {
		if s.ActiveDistro != "" && s.ActiveDistro != name {
			return fmt.Errorf("%q became active while starting — aborting", s.ActiveDistro)
		}
		d := s.Distros[name]
		d.RelayPort = port
		d.WSLWasRunning = wasRunning
		d.BootedByDockup = bootedByDockup
		d.Daemon = state.DaemonInfo{PID: selfPID, StartedAt: startedAt, Mode: "foreground"}
		s.Distros[name] = d
		s.ActiveDistro = name
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		_ = engine.Stop(name)
		if bootedByDockup {
			_ = wsl.Terminate(name)
		}
		return 1
	}

	fmt.Printf("up: %q on npipe:////./pipe/docker_engine (Ctrl-C to tear down)\n", name)
	fmt.Println(`  $env:DOCKER_HOST='npipe:////./pipe/docker_engine'`)
	_ = relay.Serve(ctx, port) // returns on Ctrl-C

	if err := state.Transaction(func(s *state.State) error {
		teardownDistro(s, name)
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if bootedByDockup {
		fmt.Printf("down: processes stopped, distro %q terminated (it was booted by dockup)\n", name)
	} else {
		fmt.Printf("down: dockup processes stopped, distro %q left running (it was already running)\n", name)
	}
	return 0
}
