// Package cli parses args and dispatches subcommands.
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"dockup/internal/dockercli"
	"dockup/internal/doctor"
	"dockup/internal/engine"
	"dockup/internal/picker"
	"dockup/internal/state"
	"dockup/internal/wsl"
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
	fmt.Printf("%-24s %-8s %-8s\n", "NAME", "DEFAULT", "ACTIVE")
	for name := range s.Distros {
		def, act := "", ""
		if name == s.Default {
			def = "*"
		}
		if name == s.ActiveDistro {
			act = "up?"
		}
		fmt.Printf("%-24s %-8s %-8s\n", name, def, act)
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
	if _, ok := s.Distros[name]; !ok {
		fmt.Fprintf(os.Stderr, "dockup: distro %q is not configured\n", name)
		return 1
	}
	s.Default = name
	if err := state.Save(s); err != nil {
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
	d := s.Distros[s.ActiveDistro]
	alive := engine.Healthy(d.RelayPort)
	status := "stopped"
	if alive {
		status = fmt.Sprintf("running (port %d, pid %d, since %s)", d.RelayPort, d.Daemon.PID, d.Daemon.StartedAt)
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
	fmt.Printf("setup %q: not yet implemented in this scaffold (P1 builds it next)\n", name)
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
	name, s, err := resolveDistro(distroFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if s.ActiveDistro != "" && s.ActiveDistro != name {
		fmt.Fprintf(os.Stderr, "dockup: %q is active — stop/shutdown first (single-active)\n", s.ActiveDistro)
		return 1
	}
	fmt.Printf("foreground run %q (port %d): relay not yet implemented in this scaffold (P1 next)\n", name, portFlag)
	return 0
}
