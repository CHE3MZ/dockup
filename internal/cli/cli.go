// Package cli parses args and dispatches subcommands.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/CHE3MZ/dockup/internal/distro/alpine"
	"github.com/CHE3MZ/dockup/internal/distro/debian"

	"github.com/CHE3MZ/dockup/internal/distro"
	"github.com/CHE3MZ/dockup/internal/dockercli"
	"github.com/CHE3MZ/dockup/internal/doctor"
	"github.com/CHE3MZ/dockup/internal/engine"
	"github.com/CHE3MZ/dockup/internal/log"
	"github.com/CHE3MZ/dockup/internal/picker"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Run dispatches. Returns process exit code.
func Run(args []string, version, commit string) int {
	// Global flag: -d|--distro <name>.
	distroFlag, rest := parseGlobals(args)

	if len(rest) == 0 {
		// Bare foreground run.
		return bareRun(distroFlag)
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
	case "__serve":
		// Hidden: relay holder spawned by `daemon start`. Not for humans.
		return cmdServeInner(distroFlag)
	default:
		fmt.Fprintf(os.Stderr, "dockup: unknown command %q (try --help)\n", rest[0])
		return 1
	}
}

func parseGlobals(args []string) (distro string, rest []string) {
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
		default:
			rest = append(rest, a)
		}
	}
	return distro, rest
}

func usage() {
	fmt.Print(`dockup — manual Docker Engine in your own WSL2 distro (Windows only)

Usage:
  dockup [-d|--distro NAME]     foreground run (single-active)
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
// the distro, terminates it. Clears active/daemon bookkeeping.
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
	fmt.Printf("%-24s %-8s %-8s\n", "NAME", "DEFAULT", "ACTIVE")
	for name := range s.Distros {
		def, act := "", ""
		if name == s.Default {
			def = "*"
		}
		if name == s.ActiveDistro {
			act = "yes"
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
	if !ok {
		fmt.Printf("%-24s %s\n", s.ActiveDistro, "stopped (stale state, run dockup cleanup)")
		return 0
	}
	relayUp := relay.Alive()
	sockUp := engine.SocketAlive(s.ActiveDistro)
	var status string
	switch {
	case relayUp && sockUp:
		status = fmt.Sprintf("running (pid %d, since %s)", d.Daemon.PID, d.Daemon.StartedAt)
	case relayUp:
		status = "degraded (relay up, engine not responding)"
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
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	d, ok := s.Distros[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "dockup: distro %q is not configured\n", name)
		return 1
	}
	// Stop anything running first (lifecycle rule: terminate only if booted).
	if s.ActiveDistro == name {
		stopOwnerPID(d.Daemon.PID)
		_ = engine.Stop(name)
	}
	// Remove only the setup delta. Unknown family (e.g. distro replaced):
	// skip package removal, still drop the state entry.
	switch distro.Detect(name) {
	case distro.Debian:
		if err := debian.Remove(name, d.InstalledByDockup); err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
	case distro.Alpine:
		if err := alpine.Remove(name, d.InstalledByDockup); err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "dockup: warning: %q family unknown, skipping package removal\n", name)
	}
	npkg := len(d.InstalledByDockup.Packages)
	nrepo := len(d.InstalledByDockup.Repos)
	hadPrior := d.HadPriorDocker
	if err := state.Transaction(func(s *state.State) error {
		teardownDistro(s, name)
		delete(s.Distros, name)
		if s.Default == name {
			s.Default = ""
			for other := range s.Distros {
				s.Default = other
				break
			}
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	fmt.Printf("reverted %q (removed %d packages, %d repo files; images/volumes/distro untouched)\n", name, npkg, nrepo)
	if hadPrior {
		fmt.Fprintf(os.Stderr, "dockup: note: %q had docker before setup — pre-existing install left alone\n", name)
	}
	return 0
}

func cmdCleanup() int {
	fixed, attention := 0, 0
	if err := state.Transaction(func(s *state.State) error {
		running, err := wsl.List()
		if err != nil {
			return err
		}
		exists := map[string]bool{}
		for _, d := range running {
			exists[d] = true
		}
		// Prune configs for distros that no longer exist (config only).
		for name := range s.Distros {
			if !exists[name] {
				fmt.Printf("cleanup: %q no longer installed, dropping config\n", name)
				teardownDistro(s, name)
				delete(s.Distros, name)
				if s.Default == name {
					s.Default = ""
				}
				fixed++
			}
		}
		if s.ActiveDistro == "" && s.Default != "" {
			if _, ok := s.Distros[s.Default]; !ok {
				s.Default = ""
			}
		}
		for name := range s.Distros {
			if name == s.ActiveDistro {
				relayUp := relay.Alive()
				sockUp := relayUp && engine.SocketAlive(name)
				switch {
				case relayUp && sockUp:
					fmt.Printf("cleanup: %q healthy, nothing to do\n", name)
				case relayUp:
					fmt.Printf("cleanup: %q NEEDS ATTENTION (relay up, engine not responding)\n", name)
					attention++
				default:
					fmt.Printf("cleanup: %q had stale active entry, cleared\n", name)
					teardownDistro(s, name)
					fixed++
				}
				continue
			}
			// Not active: engine must be down. Signature-scoped stop only.
			if engine.SocketAlive(name) {
				fmt.Printf("cleanup: %q has orphan engine, stopping (signature match only)\n", name)
				_ = engine.Stop(name)
				if engine.SocketAlive(name) {
					fmt.Printf("cleanup: %q NEEDS ATTENTION (engine still responding after stop)\n", name)
					attention++
				} else {
					fixed++
				}
			}
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	fmt.Printf("cleanup: %d fixed, %d need attention\n", fixed, attention)
	// A live pipe with no active owner is foreign (e.g. Docker Desktop) or a
	// stale relay whose owner died. Never kill unknown PIDs — report it.
	if relay.Alive() {
		if s2, err := state.Load(); err == nil && s2.ActiveDistro == "" {
			fmt.Println("cleanup: NEEDS ATTENTION (unknown process holds the relay pipe; stop Docker Desktop or stale relays, then re-run)")
			attention++
		}
	}
	if attention > 0 {
		return 1
	}
	return 0
}

func cmdShutdown() int {
	if !requireCLI(false) {
		return 1
	}
	var msg string
	if err := state.Transaction(func(s *state.State) error {
		if s.ActiveDistro == "" {
			return fmt.Errorf("nothing running")
		}
		name := s.ActiveDistro
		terminated := s.Distros[name].BootedByDockup
		stopOwnerPID(s.Distros[name].Daemon.PID)
		teardownDistro(s, name)
		if terminated {
			msg = fmt.Sprintf("shutdown: %q stopped, distro terminated (it was booted by dockup)", name)
		} else {
			msg = fmt.Sprintf("shutdown: %q stopped, distro left running (it was already running)", name)
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	fmt.Println(msg)
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
	switch args[0] {
	case "start":
		name, _, err := resolveDistro(globalDistro)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		return daemonStart(name)
	case "stop":
		name, err := activeOrFlag(globalDistro)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		return daemonStop(name)
	case "restart":
		name, _, err := resolveDistro(globalDistro)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dockup:", err)
			return 1
		}
		_ = daemonStop(name) // not running is fine; start fresh.
		return daemonStart(name)
	case "status":
		name := globalDistro
		if name == "" {
			var err error
			name, err = activeOrFlag("")
			if err != nil {
				fmt.Fprintln(os.Stderr, "dockup:", err)
				return 1
			}
		}
		return daemonStatus(name)
	default:
		fmt.Fprintf(os.Stderr, "dockup: unknown daemon command %q\n", args[0])
		return 1
	}
}

// activeOrFlag returns flag if set, else the active distro, else an error.
func activeOrFlag(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	s, err := state.Load()
	if err != nil {
		return "", err
	}
	if s.ActiveDistro == "" {
		return "", fmt.Errorf("nothing running (pass -d <name> or start first)")
	}
	return s.ActiveDistro, nil
}

func bareRun(distroFlag string) int {
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
		if s.ActiveDistro == name {
			fmt.Printf("relay already up for %q (attaching, not double-starting)\n", name)
			fmt.Println(`point your shell at it: dockup env --shell powershell | Invoke-Expression`)
			return 0
		}
		// Pipe alive but not ours (e.g. Docker Desktop, or a foreign daemon).
		// Never hijack it: attaching would claim success while managing nothing.
		fmt.Fprintln(os.Stderr, `dockup: something else is listening on \\.\pipe\docker_engine (e.g. Docker Desktop).`)
		fmt.Fprintln(os.Stderr, "dockup: stop it first, or run `dockup cleanup` if it is a stale dockup relay.")
		return 1
	}

	wasRunning, err := prepareEngine(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	bootedByDockup := !wasRunning

	if err := claimActive(name, wasRunning, os.Getpid(), "foreground"); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		_ = engine.Stop(name)
		if bootedByDockup {
			_ = wsl.Terminate(name)
		}
		return 1
	}

	fmt.Printf("up: %q on npipe:////./pipe/docker_engine (Ctrl-C to tear down)\n", name)
	fmt.Println(`  $env:DOCKER_HOST='npipe:////./pipe/docker_engine'`)
	_ = relay.Serve(ctx, name) // returns on Ctrl-C

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

// prepareEngine starts containerd->dockerd and waits until the socket
// answers. Returns wasRunning (distro already up before us).
// On failure it stops what it started and terminates only a distro it booted.
func prepareEngine(name string) (wasRunning bool, err error) {
	running, err := wsl.Running()
	if err != nil {
		return false, err
	}
	wasRunning = running[name]
	booted := !wasRunning
	fmt.Printf("starting engine in %q...\n", name)
	fail := func(e error) (bool, error) {
		_ = engine.Stop(name)
		if booted {
			_ = wsl.Terminate(name)
		}
		return false, e
	}
	if err := engine.Start(name); err != nil {
		return fail(err)
	}
	if err := engine.WaitSocket(name, 2*time.Minute); err != nil {
		return fail(err)
	}
	return wasRunning, nil
}

// claimActive records the single active set under the state lock.
func claimActive(name string, wasRunning bool, pid int, mode string) error {
	startedAt := time.Now().UTC().Format(time.RFC3339)
	return state.Transaction(func(s *state.State) error {
		if s.ActiveDistro != "" && s.ActiveDistro != name {
			return fmt.Errorf("%q became active while starting — aborting", s.ActiveDistro)
		}
		d := s.Distros[name]
		d.WSLWasRunning = wasRunning
		d.BootedByDockup = !wasRunning
		d.Daemon = state.DaemonInfo{PID: pid, StartedAt: startedAt, Mode: mode}
		s.Distros[name] = d
		s.ActiveDistro = name
		return nil
	})
}

// stopOwnerPID kills the recorded relay owner (foreground or daemon child).
// Best-effort: dead/missing PIDs are not errors.
func stopOwnerPID(pid int) {
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill() // Windows: TerminateProcess.
	}
}

// daemonStart boots the engine, then re-launches this same binary headless
// (__serve) to hold the pipe. No second binary.
func daemonStart(name string) int {
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if _, ok := s.Distros[name]; !ok {
		fmt.Fprintf(os.Stderr, "dockup: distro %q is not configured (dockup setup first)\n", name)
		return 1
	}
	if s.ActiveDistro != "" && s.ActiveDistro != name {
		fmt.Fprintf(os.Stderr, "dockup: %q is active — stop/shutdown first (single-active)\n", s.ActiveDistro)
		return 1
	}
	if relay.Alive() {
		if s.ActiveDistro == name {
			fmt.Printf("relay already up for %q (not double-starting)\n", name)
			return 0
		}
		fmt.Fprintln(os.Stderr, `dockup: something else is listening on \\.\pipe\docker_engine (e.g. Docker Desktop).`)
		fmt.Fprintln(os.Stderr, "dockup: stop it first, or run `dockup cleanup` if it is a stale dockup relay.")
		return 1
	}
	wasRunning, err := prepareEngine(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		_ = engine.Stop(name)
		if !wasRunning {
			_ = wsl.Terminate(name)
		}
		return 1
	}
	logPath, err := log.PathFor(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if err := log.RotateIfNeeded(name); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	lf, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	defer lf.Close()
	child := exec.Command(exe, "__serve", "-d", name)
	child.Stdout = lf
	child.Stderr = lf
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		_ = engine.Stop(name)
		if !wasRunning {
			_ = wsl.Terminate(name)
		}
		return 1
	}
	childPID := child.Process.Pid
	// Detach: never Wait on the daemon child.

	if err := claimActive(name, wasRunning, childPID, "daemon"); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		_ = child.Process.Kill()
		_ = engine.Stop(name)
		if !wasRunning {
			_ = wsl.Terminate(name)
		}
		return 1
	}
	// Confirm the child actually bound the pipe.
	deadline := time.Now().Add(15 * time.Second)
	for !relay.Alive() && time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
	}
	if !relay.Alive() {
		fmt.Fprintln(os.Stderr, "dockup: daemon child failed to serve the pipe (see logs)")
		_ = child.Process.Kill()
		_ = state.Transaction(func(s *state.State) error {
			teardownDistro(s, name)
			return nil
		})
		return 1
	}
	fmt.Printf("daemon up: %q (pid %d, log %s)\n", name, childPID, logPath)
	return 0
}

func daemonStop(name string) int {
	var msg string
	if err := state.Transaction(func(s *state.State) error {
		if s.ActiveDistro == "" {
			return fmt.Errorf("nothing running")
		}
		if s.ActiveDistro != name {
			return fmt.Errorf("%q is active, not %q", s.ActiveDistro, name)
		}
		terminated := s.Distros[name].BootedByDockup
		stopOwnerPID(s.Distros[name].Daemon.PID)
		teardownDistro(s, name)
		if terminated {
			msg = fmt.Sprintf("daemon %q stopped, distro terminated (it was booted by dockup)", name)
		} else {
			msg = fmt.Sprintf("daemon %q stopped, distro left running (it was already running)", name)
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	fmt.Println(msg)
	return 0
}

func daemonStatus(name string) int {
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	d, ok := s.Distros[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "dockup: distro %q is not configured\n", name)
		return 1
	}
	if s.ActiveDistro != name {
		fmt.Printf("%-24s stopped\n", name)
		return 0
	}
	relayUp := relay.Alive()
	sockUp := engine.SocketAlive(name)
	mode := d.Daemon.Mode
	if mode == "" {
		mode = "foreground"
	}
	switch {
	case relayUp && sockUp:
		fmt.Printf("%-24s running (%s, pid %d, since %s)\n",
			name, mode, d.Daemon.PID, d.Daemon.StartedAt)
	default:
		fmt.Printf("%-24s stopped%s\n", name, stoppedReason(relayUp, sockUp))
	}
	return 0
}

func stoppedReason(relayUp, sockUp bool) string {
	if !relayUp {
		return " (relay down)"
	}
	return " (engine not responding)"
}

// cmdServeInner is the hidden relay holder spawned by `daemon start`.
// It serves the pipe until killed; teardown belongs to the stopper.
func cmdServeInner(distroFlag string) int {
	if distroFlag == "" {
		fmt.Fprintln(os.Stderr, "dockup: __serve is internal (spawned by daemon start)")
		return 1
	}
	s, err := state.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	if s.ActiveDistro != distroFlag {
		fmt.Fprintln(os.Stderr, "dockup: __serve state mismatch (stale spawn, exiting)")
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := relay.Serve(ctx, distroFlag); err != nil {
		fmt.Fprintln(os.Stderr, "dockup:", err)
		return 1
	}
	return 0
}
