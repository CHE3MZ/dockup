// Command dockup implements the dockup CLI.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/CHE3MZ/dockup/internal/autostart"
	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/daemon"
	"github.com/CHE3MZ/dockup/internal/dockctx"
	"github.com/CHE3MZ/dockup/internal/docker"
	"github.com/CHE3MZ/dockup/internal/doctor"
	"github.com/CHE3MZ/dockup/internal/image"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/pstable"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/restore"
	"github.com/CHE3MZ/dockup/internal/setup"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/sysinfo"
	"github.com/CHE3MZ/dockup/internal/ui"
	"github.com/CHE3MZ/dockup/internal/userconfig"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

var version = config.Version

func main() {
	os.Exit(run(os.Args[1:]))
}

// ensureUserConfig creates ~/.dockup/config.json on first run and applies
// the color setting. It never fails startup: on error it warns and
// continues with defaults. Old files may still carry an "installed" key
// from before the flag moved to state.json; Load ignores it and the next
// Save drops it.
func ensureUserConfig() userconfig.Config {
	cfg, err := userconfig.Ensure()
	if err != nil {
		ui.Warn("could not set up %s: %v", userconfig.File(), err)
		return userconfig.Defaults()
	}
	cfg = cfg.WithDefaults()
	ui.SetEnabled(cfg.Color && ui.Enabled())
	return cfg
}

func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func run(args []string) int {
	cfg := ensureUserConfig()
	if len(args) == 0 {
		return foreground(cfg)
	}
	// Like -h, --version short-circuits anywhere it appears first.
	if args[0] == "--version" || args[0] == "-v" {
		printVersion()
		return 0
	}
	switch args[0] {
	case "--help", "-h", "help":
		if len(args) > 2 {
			logx.Err("too many arguments (try dockup help [command])")
			return 1
		}
		if len(args) > 1 {
			return helpTopic(args[1])
		}
		usage()
		return 0
	case "version":
		if hasHelpFlag(args[1:]) {
			versionHelp()
			return 0
		}
		if len(args[1:]) > 0 {
			logx.Err("dockup version takes no arguments (try dockup version --help)")
			return 1
		}
		printVersion()
		return 0
	case "setup":
		return cmdSetup(cfg, args[1:])
	case "uninstall":
		if hasHelpFlag(args[1:]) {
			uninstallHelp()
			return 0
		}
		force := false
		for _, a := range args[1:] {
			if a == "--force" || a == "-f" {
				force = true
				continue
			}
			logx.Err("unknown uninstall flag %q (try dockup uninstall --help)", a)
			return 1
		}
		return setup.Uninstall(force)
	case "restore":
		return cmdRestore(args[1:])
	case "ps":
		if hasHelpFlag(args[1:]) {
			psHelp()
			return 0
		}
		jsonOut := false
		for _, a := range args[1:] {
			if a == "--json" || a == "-j" {
				jsonOut = true
				continue
			}
			logx.Err("unknown ps flag %q (try dockup ps --help)", a)
			return 1
		}
		return cmdPs(cfg, jsonOut)
	case "context":
		return cmdContext(cfg, args[1:])
	case "daemon":
		return cmdDaemon(cfg, args[1:])
	case "shutdown":
		if hasHelpFlag(args[1:]) {
			shutdownHelp()
			return 0
		}
		if len(args[1:]) > 0 {
			logx.Err("dockup shutdown takes no arguments (try dockup shutdown --help)")
			return 1
		}
		return cmdShutdown(cfg)
	case "ssh":
		return cmdSSH(args[1:])
	case "prune":
		return cmdPrune(args[1:])
	case "doctor":
		if hasHelpFlag(args[1:]) {
			doctorHelp()
			return 0
		}
		fix := false
		for _, a := range args[1:] {
			if a == "--fix" {
				fix = true
			} else {
				logx.Err("unknown doctor flag %q (try dockup doctor --help)", a)
				return 1
			}
		}
		if err := doctor.RunEx(fix); err != nil {
			logx.ErrFrom(err)
			return 1
		}
		return 0
	case "upgrade", "update":
		if hasHelpFlag(args[1:]) {
			upgradeHelp()
			return 0
		}
		if len(args[1:]) > 0 {
			logx.Err("unknown upgrade flag %q (try dockup upgrade --help)", args[1])
			return 1
		}
		return cmdUpgrade()
	case "__serve":
		return serveForever(cfg)
	default:
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: unknown command %q (try --help)", args[0])))
		return 1
	}
}

// installFlags are the arch/location flags shared by setup and restore.
type installFlags struct {
	amd  bool
	arm  bool
	path string
}

// parseInstallFlags consumes --amd/--arm/--path* from args, returning the
// rest untouched for per-command flags. cmd names the command for hints.
func parseInstallFlags(cmd string, args []string) (f installFlags, rest []string, err error) {
	pathErr := fmt.Errorf("flag --path needs a value, e.g. --path=\"D:/WSL/Dockup\" (try dockup %s --help)", cmd)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--amd":
			f.amd = true
		case a == "--arm":
			f.arm = true
		case a == "--path":
			if i+1 >= len(args) {
				return f, nil, pathErr
			}
			i++
			f.path = args[i]
		case strings.HasPrefix(a, "--path="):
			f.path = strings.TrimPrefix(a, "--path=")
			if f.path == "" {
				return f, nil, pathErr
			}
		default:
			rest = append(rest, a)
		}
	}
	return f, rest, nil
}

func cmdSetup(cfg userconfig.Config, args []string) int {
	if hasHelpFlag(args) {
		setupHelp()
		return 0
	}
	f, rest, err := parseInstallFlags("setup", args)
	if err != nil {
		logx.ErrFrom(err)
		return 1
	}
	var dryRun bool
	for _, a := range rest {
		if a == "--dry-run" {
			dryRun = true
			continue
		}
		logx.Err("unknown setup flag %q (try dockup setup --help)", a)
		return 1
	}
	arch, msg := config.NormalizeArch(f.amd, f.arm)
	if msg != "" {
		logx.Err("%s", msg)
		return 1
	}
	return setup.RunEx(setup.Options{Arch: arch, Path: f.path, DryRun: dryRun, Cfg: cfg})
}

// helpCmd renders one aligned command row. The command column is padded
// before styling so the escape codes never skew the layout.
func helpCmd(command, description string) string {
	const width = 20
	pad := "  "
	if len(command) < width {
		pad = strings.Repeat(" ", width-len(command))
	}
	return "  " + ui.Bold(command) + pad + ui.White(description) + "\n"
}

func usage() {
	fmt.Print(ui.Header("dockup") + "\n" + "\n" +
		ui.LightBlue("Usage:") + "\n" +
		helpCmd("dockup", "Run in the foreground (Ctrl+C to stop)") +
		helpCmd("dockup <command>", "Run a command") + "\n" +
		ui.LightBlue("Commands:") + "\n" +
		helpCmd("setup", "Set up the dockup distro") +
		helpCmd("ps", "Show status") +
		helpCmd("context", "Manage the Docker CLI context") +
		helpCmd("ssh [-- command]", "Open a shell inside the distro") +
		helpCmd("version", "Show the version") +
		helpCmd("help [command]", "Show help for a command") + "\n" +
		ui.LightBlue("Daemon:") + "\n" +
		helpCmd("daemon", "Start, stop, restart, or inspect the background process") +
		helpCmd("shutdown", "Stop everything") +
		helpCmd("upgrade", "Upgrade the engine and cached setup image") + "\n" +
		ui.LightBlue("Maintenance:") + "\n" +
		helpCmd("restore", "Reset the distro to a clean state") +
		helpCmd("prune", "Reclaim disk space and cached images") +
		helpCmd("doctor", "Check health and repair stale state") +
		helpCmd("uninstall", "Remove the distro and saved state") + "\n" +
		ui.Gray("Config file: ~/.dockup/config.json") + "\n" )
}

func helpTopic(name string) int {
	switch name {
	case "setup":
		setupHelp()
	case "uninstall":
		uninstallHelp()
	case "restore":
		restoreHelp()
	case "ps":
		psHelp()
	case "context":
		contextHelp()
	case "daemon":
		daemonHelp()
	case "shutdown":
		shutdownHelp()
	case "ssh":
		sshHelp()
	case "prune":
		pruneHelp()
	case "doctor":
		doctorHelp()
	case "upgrade", "update":
		upgradeHelp()
	case "version":
		versionHelp()
	default:
		logx.Err("unknown help topic %q (try --help)", name)
		return 1
	}
	return 0
}

func setupHelp() {
	fmt.Print(ui.Header("dockup setup") + `
  Download the signed prebuilt image, import it into WSL as the
  ` + ui.Cyan(`"dockup"`) + ` distro, and check that the daemon and bridge work.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup setup") + ` [--amd|--arm] [--path=DIR] [--dry-run]

` + ui.LightBlue("Options:") + `
  ` + ui.Bold("--amd, --arm") + `    Architecture (default: --amd)
  ` + ui.Bold("--path=DIR") + `      Install root, e.g. --path="D:/WSL/Dockup".
                  The folder you give becomes the new default.
  ` + ui.Bold("--dry-run") + `       Show what setup would do, without changing anything.
  ` + ui.Bold("-h, --help") + `      Show this help.

  Without --path you get asked where to install. An empty answer keeps
  the shown default. The root holds ` + ui.Cyan("distro\\") + ` (the WSL import)
  and ` + ui.Cyan("cache\\images\\") + ` (the verified setup image); uninstall keeps
  the cache, and ` + ui.Bold("dockup prune") + ` removes it.
`)
}

func uninstallHelp() {
	fmt.Print(ui.Header("dockup uninstall") + `
  Delete the dockup distro from WSL and clear its saved state. The
  cached setup image is kept so a later setup does not download it
  again; use ` + ui.Bold("dockup prune") + ` to remove it.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup uninstall") + ` [--force]

  ` + ui.Bold("--force, -f") + `   Skip the confirmation (for scripts).
`)
}

func sshHelp() {
	fmt.Print(ui.Header("dockup ssh") + `
  Open a shell inside the dockup distro (great for journalctl, configs,
  and network debugging), or run one remote command and exit.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup ssh") + ` [-- command ...]

` + ui.LightBlue("Examples:") + `
  dockup ssh
  dockup ssh -- journalctl -u docker.service --no-pager -n 30
  dockup ssh -- df -h /

  A leading ` + ui.Bold("--") + ` ends dockup's own parsing: everything after it
  runs remotely verbatim. Ctrl+C reaches the remote command.
`)
}

func pruneHelp() {
	fmt.Print(ui.Header("dockup prune") + `
  Reclaim distro disk space (a dynamic VHDX only ever grows) by removing
  unneeded Docker objects, and delete the cached setup image. Works after
  uninstall too, when only the cache is left.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup prune") + ` [--all] [--volumes] [--keep-image] [--force]

  ` + ui.Bold("--all") + `         Also remove all unused images (default: dangling only).
  ` + ui.Bold("--volumes") + `     Also remove unused volumes (destroys their data).
  ` + ui.Bold("--keep-image") + `  Keep the cached setup image.
  ` + ui.Bold("--force, -f") + `   Skip the confirmation (for scripts).
`)
}

func restoreHelp() {
	fmt.Print(ui.Header("dockup restore") + `
  Reset a tampered-with distro to its default state. The lightweight
  default removes packages added since setup, reinstalls the engine if
  needed, rewrites managed configs (a broken daemon.json is kept as
  daemon.json.bak), and restarts services. Images, containers, and
  volumes are preserved. For engine-only repair without touching
  packages, use dockup doctor --fix instead.

  With ` + ui.Bold("--full") + `, the distro is deleted and reinstalled
  from scratch: guaranteed pristine, but containers, images, and
  volumes are destroyed. "--amd"/"--arm"/"--path" pick a different
  architecture or install folder for the reinstall (defaults: last
  setup's arch, current install folder).

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup restore") + ` [--full] [--amd|--arm] [--path=DIR] [--force]

  ` + ui.Bold("--force, -f") + `   Skip the confirmation (for scripts).
`)
}

// cmdRestore parses restore flags. --amd/--arm/--path only apply to
// --full (lightweight restores in place).
func cmdRestore(args []string) int {
	if hasHelpFlag(args) {
		restoreHelp()
		return 0
	}
	f, rest, err := parseInstallFlags("restore", args)
	if err != nil {
		logx.ErrFrom(err)
		return 1
	}
	var full, force bool
	for _, a := range rest {
		switch a {
		case "--full":
			full = true
		case "--force", "-f":
			force = true
		default:
			logx.Err("unknown restore flag %q (try dockup restore --help)", a)
			return 1
		}
	}
	arch, msg := config.NormalizeArch(f.amd, f.arm)
	if msg != "" {
		logx.Err("%s", msg)
		return 1
	}
	if (f.path != "" || f.amd || f.arm) && !full {
		logx.Err("--path/--amd/--arm only apply to restore --full")
		return 1
	}
	var archOverride string
	if f.amd || f.arm {
		archOverride = arch
	}
	return restore.Run(restore.Options{Full: full, Arch: archOverride, Path: f.path, Force: force})
}

func psHelp() {
	fmt.Print(ui.Header("dockup ps") + `
  Show dockup's status:

    STATUS      AUTOSTART   INSTALLED   SIZE      MEMORY
    running     off         yes         452 MB    128 MB

  STATUS is ` + ui.Green("running") + ` (engine answering), ` + ui.White("starting...") + `
  (bridge up, engine still booting) or ` + ui.White("stopped") + `.
  AUTOSTART is ` + ui.Cyan("on") + ` or ` + ui.Cyan("off") + `, INSTALLED is ` + ui.Cyan("yes") + ` or ` + ui.Cyan("no") + `,
  SIZE is the distro's disk use, MEMORY its live RAM use (install record
  plus live probes).

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup ps") + ` [--json]

  ` + ui.Bold("--json, -j") + `   Machine-readable output for scripts.
`)
}

func contextHelp() {
	fmt.Print(ui.Header("dockup context") + `
  dockup keeps a Docker CLI context named ` + ui.Cyan(`"dockup"`) + ` that points at
  the bridge, so clients can select it without -H or DOCKER_HOST.

  Setup creates the context. Activating it stays your choice.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup context") + `        Show the context, its host, and who is active
  ` + ui.Bold("dockup context use") + `    Create/update it and make it the default
  ` + ui.Bold("dockup context off") + `    Switch Docker back to the previous context

  An active context applies unless ` + ui.Bold("DOCKER_HOST") + ` is set; the
  environment variable always wins.

  Pick it for one command with ` + ui.Bold("docker --context dockup ps") + `,
  or remove it with ` + ui.Bold("docker context rm dockup") + `.
`)
}

func daemonHelp() {
	fmt.Print(ui.Header("dockup daemon") + `
  Run dockup in the background (named pipe, plus TCP if enabled).

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup daemon") + ` <command>

` + ui.LightBlue("Commands:") + `
` + helpCmd("start", "Start the background process") +
		helpCmd("stop", "Stop the background process") +
		helpCmd("restart", "Restart the background process") +
		helpCmd("status", "Brief health (running or stopped)") +
		helpCmd("log", "Follow the log (read-only, Ctrl+C to exit)") +
		helpCmd("autostart", "Show or set autostart at Windows login (default off)") + `
  Stored in ` + ui.Cyan("~/.dockup/config.json") + `.
`)
}

func shutdownHelp() {
	fmt.Print(ui.Header("dockup shutdown") + `
  Stop everything dockup is running and shut down its WSL distro.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup shutdown") + `
`)
}

func doctorHelp() {
	fmt.Print(ui.Header("dockup doctor") + `
  Check that everything dockup needs is healthy, and clean up
  stale state left behind by crashes or reboots.

  With ` + ui.Bold("--fix") + `, a missing or broken in-distro engine is
  reinstalled and reconfigured, then verified.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup doctor") + ` [--fix]
`)
}

func upgradeHelp() {
	fmt.Print(ui.Header("dockup upgrade") + `
  Update Docker and its dependencies inside the dockup distro to the
  latest versions, restart its services, and verify the daemon answers.
  If a cached setup image exists, refresh it when its release changed.

  ` + ui.Bold("update") + ` is an alias for this command.

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup upgrade") + `
  ` + ui.Bold("dockup update") + `
`)
}

// cmdUpgrade upgrades the in-distro engine to latest and refreshes an
// existing cached setup image when the release changed.
func cmdUpgrade() int {
	s, _ := state.Load()
	if !s.Installed && !wsl.Exists(config.DistroName) {
		fmt.Fprintln(os.Stderr, ui.Red(`dockup has not been setup yet run "dockup setup" to set it up.`))
		return 1
	}
	root := userconfig.InstallRoot()
	arch := s.Arch
	if arch != "amd64" && arch != "arm64" {
		arch = "amd64"
	}
	cachePresent := image.CacheExists(root)
	msg := "Upgrade the engine inside the dockup distro to the latest versions? This needs an internet connection."
	if cachePresent {
		msg = "Upgrade the engine inside the dockup distro and refresh the cached setup image if it changed? This needs an internet connection."
	}
	if !ui.Confirm(ui.White(msg)) {
		logx.Info("aborted")
		return 0
	}
	fmt.Printf("%s\n", ui.White("upgrading docker engine inside dockup..."))
	if err := docker.Upgrade(config.DistroName); err != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: upgrade failed: %v", err)))
		return 1
	}
	if cachePresent {
		logx.Info("checking the cached setup image...")
		var pb *ui.Progress
		var last int64
		res, err := image.Ensure(root, arch, true, func(done, total int64) {
			if pb == nil {
				pb = ui.NewProgress(total, "refreshing cached image...")
			}
			pb.Add(done - last)
			last = done
		})
		if pb != nil {
			pb.Finish()
		}
		switch {
		case err != nil:
			logx.Warn("cached setup image not refreshed: %v", err)
		case res.Cached:
			logx.Info("cached setup image is up to date")
		default:
			logx.Info("cached setup image updated")
		}
	}
	logx.Ok("upgrade complete")
	return 0
}

// printVersion prints the name and version on one line, easy to parse.
func printVersion() {
	fmt.Printf("%s %s\n", ui.Header("dockup"), ui.White(version))
}

func versionHelp() {
	fmt.Print(ui.Header("dockup version") + `
  Show the dockup version (` + ui.Bold("-v") + ` and ` + ui.Bold("--version") + ` do the same thing).

` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup version") + `
`)
}

// interrupter arms Ctrl+C handling for one run: the first signal stops the
// active spinner (if any) so the message lands on a clean line instead of
// freezing mid-frame, announces once, and cancels; a second signal while
// teardown is stuck force-exits. One instance per process (a dockup process
// runs exactly one command).
type interrupter struct {
	downOnce sync.Once
	tripped  atomic.Bool
	mu       sync.Mutex
	spinner  *ui.Spinner
}

// sayDown announces the shutdown exactly once, on a fresh line.
func (in *interrupter) sayDown() {
	in.downOnce.Do(func() {
		fmt.Printf("%s %s\n", ui.Red("✘"), ui.White("shutting down dockup..."))
	})
}

// setSpinner registers the currently animating spinner (nil clears it).
// Stopped/finished spinners are no-ops, so a stale pointer is harmless.
func (in *interrupter) setSpinner(sp *ui.Spinner) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.spinner = sp
}

// arm starts watching for interrupts; cancel runs the teardown.
func (in *interrupter) arm(cancel context.CancelFunc) {
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		in.tripped.Store(true)
		in.mu.Lock()
		sp := in.spinner
		in.mu.Unlock()
		if sp != nil {
			sp.Stop()
		}
		in.sayDown()
		cancel()
		<-sig
		fmt.Fprintln(os.Stderr, ui.Red("dockup: interrupted again, forcing exit"))
		os.Exit(130)
	}()
}

// wasInterrupted reports whether a signal arrived.
func (in *interrupter) wasInterrupted() bool { return in.tripped.Load() }

// newInterrupter builds an unarmed interrupter.
func newInterrupter() *interrupter { return &interrupter{} }

// foreground starts the relay inline. Refuses if already running or not setup.
func foreground(cfg userconfig.Config) int {
	// Narrate before the slow checks (cold wsl.exe spawns take seconds):
	// silence is what makes startup feel hung and Ctrl+C feel dead.
	fmt.Printf("%s\n", ui.White("Starting dockup..."))
	ensureProcessedInput()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watchConsoleExit(cancel)
	// Armed before anything slow: an interrupt during startup checks or
	// waits exits 130 instead of falling into unrelated error paths.
	in := newInterrupter()
	in.arm(cancel)
	pipe := cfg.EffectivePipe()
	s, _ := state.Load()
	if !s.Installed && !wsl.Exists(config.DistroName) {
		fmt.Fprintln(os.Stderr, ui.Red(`dockup has not been setup yet run "dockup setup" to set it up. run "dockup help" to see all available commands.`))
		return 1
	}
	if s.Daemon.PID != 0 && daemon.DaemonAlive(s) {
		fmt.Fprintln(os.Stderr, ui.Red("dockup is already running in the background. Run dockup daemon stop first."))
		return 1
	}
	if relay.AliveOn(pipe) {
		if !s.Installed {
			fmt.Fprintln(os.Stderr, ui.Red("dockup: pipe is held by another program. Stop it before running dockup."))
		} else {
			fmt.Fprintln(os.Stderr, ui.Red("dockup is already running (another foreground or daemon holds the pipe)."))
		}
		return 1
	}
	if in.wasInterrupted() {
		in.sayDown()
		return 130
	}
	if s.Installed {
		// State claims an install: prove the distro survived. An external
		// wsl --unregister must flip us to not-installed, never serve dead.
		if present, err := doctor.ReconcileDistro(); err == nil && !present {
			logx.Err("distro \"dockup\" is missing from WSL. It may have been removed outside dockup. Run dockup doctor, then dockup setup to reinstall.")
			return 1
		}
	}
	errCh := make(chan error, 1)
	mirrorCh := make(chan bool, 1)
	go func() {
		errCh <- relay.ServeEx(ctx, config.DistroName, pipe, cfg.UseTCP, cfg.EffectivePort(), func(served bool) {
			mirrorCh <- served
		})
	}()
	if !relay.WaitAliveOn(pipe, 10*time.Second) {
		if in.wasInterrupted() {
			in.sayDown()
			cancel()
			return 130
		}
		select {
		case err := <-errCh:
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
		default:
			fmt.Fprintln(os.Stderr, ui.Red("dockup: helper failed (pipe never came up)"))
		}
		cancel()
		return 1
	}
	fmt.Printf("%s\n", ui.Green("dockup is up and running!"))
	select {
	case served := <-mirrorCh:
		recordMirror(served)
	case <-time.After(5 * time.Second):
	}
	fmt.Printf("%s\n", ui.White("waiting for dockup on wsl..."))
	// Eager boot: the distro otherwise boots lazily on the first client
	// connection, leaving the first docker command hanging with no word in
	// this window. Warming it here moves that wait into the open. This does
	// NOT wait for the engine itself — ps reports starting... until dockerd
	// answers. A failed warmup only warns: serve anyway and let ps/doctor
	// tell the rest of the story.
	warm := ui.NewSpinner(nil, "warming up the dockup distro...")
	warm.Start()
	in.setSpinner(warm)
	if _, err := wsl.ExecCtx(ctx, config.DistroName, 60*time.Second, "sh", "-c", "echo ok"); err != nil {
		warm.Stop()
		if in.wasInterrupted() {
			// Interrupted mid-warmup: the watcher already cancelled, the
			// child is dead, the pipe barely lived. (A bare timeout lands
			// below instead — that path must refuse, not serve dead air.)
			in.sayDown()
			return 130
		}
		logx.Err("distro \"dockup\" is present but not responding (%v). Run dockup doctor --fix to repair it", err)
		return 1
	} else {
		warm.Done()
	}
	if cfg.UseTCP {
		fmt.Printf("%s\n", ui.White(fmt.Sprintf("tcp bridge on %s", cfg.TCPAddr())))
	}
	fmt.Printf("%s %s\n", ui.White("Running in the foreground!"), ui.Gray("press Ctrl+C to stop."))
	if err := <-errCh; err != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
		return 1
	}
	cancel()
	_ = relay.WaitDeadOn(pipe, 5*time.Second)
	fmt.Printf("%s %s\n", ui.Green("•"), ui.White("dockup stopped successfully!"))
	// Reaching teardown with a clean relay means ctx was cancelled, and only
	// the interrupt watcher cancels it — so this exit was user-requested.
	return 130
}

// serveForever is the hidden daemon child holding the pipe.
func serveForever(cfg userconfig.Config) int {
	ensureProcessedInput()
	s, _ := state.Load()
	if !s.Installed {
		fmt.Fprintln(os.Stderr, ui.Red("dockup: helper failed (not setup)"))
		return 1
	}
	if present, err := doctor.ReconcileDistro(); err == nil && !present {
		fmt.Fprintln(os.Stderr, ui.Red("dockup: helper failed (distro \"dockup\" is missing from WSL. Run dockup doctor, then dockup setup to reinstall)"))
		return 1
	}
	if err := userconfig.ValidatePort(cfg.EffectivePort()); cfg.UseTCP && err != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
		return 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watchConsoleExit(cancel)
	newInterrupter().arm(cancel)
	if err := relay.ServeEx(ctx, config.DistroName, cfg.EffectivePipe(), cfg.UseTCP, cfg.EffectivePort(), recordMirror); err != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
		return 1
	}
	return 0
}

// recordMirror records the default-pipe mirror outcome in state, but only
// when this process IS the recorded daemon child (a foreground has no daemon
// PID, so for it this is a silent no-op after a lock-free read).
func recordMirror(served bool) {
	me := os.Getpid()
	if s, _ := state.Load(); s.Daemon.PID != me {
		return
	}
	_ = state.WithLock(func(s *state.State) error {
		if s.Daemon.PID == me {
			s.Daemon.Mirror = served
		}
		return nil
	})
}

func cmdPs(cfg userconfig.Config, jsonOut bool) int {
	s, _ := state.Load()
	pipe := cfg.EffectivePipe()
	alive := relay.AliveOn(pipe)
	status := pstable.Classify(alive, s.Installed, alive && relay.EngineReady(pipe))
	row := pstable.Row{
		Status:    status,
		Autostart: pstable.AutostartText(cfg.Autostart),
		Installed: pstable.InstalledText(s.Installed),
		Size:      "-",
		Memory:    "-",
	}
	if s.Installed {
		if n, err := sysinfo.DistroSize(userconfig.DistroDirCurrent()); err == nil {
			row.Size = sysinfo.FormatSize(n)
		}
		if alive {
			if total := sysinfo.DockupWindowsRSS() + dockupDistroRSS(); total > 0 {
				row.Memory = sysinfo.FormatSize(total)
			}
		}
	}
	if jsonOut {
		out, err := pstable.RenderJSON(row)
		if err != nil {
			logx.Err("cannot render status: %v", err)
			return 1
		}
		fmt.Println(out)
		return 0
	}
	fmt.Println(pstable.RenderRow(row))
	if !alive && !s.Installed {
		fmt.Printf("%s\n", ui.Gray("(not installed. Run dockup setup to create it)"))
	} else if alive && !s.Installed {
		fmt.Printf("%s\n", ui.Gray("(pipe held by another program, not dockup)"))
	}
	if cfg.UseTCP {
		if relay.TCPAlive(cfg.TCPAddr()) {
			fmt.Printf("%s\n", ui.Gray(fmt.Sprintf("tcp %s ok", cfg.TCPAddr())))
		} else {
			fmt.Printf("%s\n", ui.Gray(fmt.Sprintf("tcp %s down", cfg.TCPAddr())))
		}
	}
	return 0
}

// cmdContext manages the Docker CLI context that points at the bridge.
func cmdContext(cfg userconfig.Config, args []string) int {
	if hasHelpFlag(args) {
		contextHelp()
		return 0
	}
	if len(args) > 1 {
		logx.Err("too many arguments (try dockup context --help)")
		return 1
	}
	if len(args) == 1 {
		switch args[0] {
		case "use":
			return contextUse(cfg)
		case "off":
			return contextOff(cfg)
		default:
			logx.Err("unknown context command %q (try dockup context --help)", args[0])
			return 1
		}
	}

	info := dockctx.Get(cfg.EffectivePipe())
	if !info.CLI {
		logx.Err("docker CLI not found on PATH (install it from docker.com or with scoop)")
		return 1
	}
	switch {
	case info.Active:
		logx.Ok("context %q is active", dockctx.Name)
	case info.Created:
		logx.Info("context %q exists, not active (current: %s)", dockctx.Name, info.Current)
	default:
		logx.Info("context %q is not created yet", dockctx.Name)
	}
	host := info.WantHost
	if info.Created {
		host = info.Host
	}
	ui.Printf("host: %s", host)
	ui.Printf("current: %s", info.Current)
	if info.Created && info.Host != info.WantHost {
		logx.Warn("context host differs from config - run dockup context use to update it to %s", info.WantHost)
	}
	if !info.Active {
		ui.Hint(`enable it with "dockup context use"`)
	}
	return 0
}

// contextUse activates the managed context, remembering the previous one so
// "dockup context off" can restore it.
func contextUse(cfg userconfig.Config) int {
	info := dockctx.Get(cfg.EffectivePipe())
	if !info.CLI {
		logx.Err("docker CLI not found on PATH (install it from docker.com or with scoop)")
		return 1
	}
	prev := ""
	if info.Current != "" && info.Current != dockctx.Name {
		prev = info.Current
	}
	if err := dockctx.Use(cfg.EffectivePipe()); err != nil {
		logx.ErrFrom(err)
		return 1
	}
	if prev != "" && prev != cfg.DockerContextPrev {
		cfg.DockerContextPrev = prev
		_ = userconfig.Save(cfg)
	}
	logx.Ok("docker now uses context %q", dockctx.Name)
	if !relay.AliveOn(cfg.EffectivePipe()) {
		ui.Hint(`dockup is not running yet - start it with "dockup daemon start" or "dockup"`)
	}
	if prev != "" {
		ui.Hint(`switch back with "dockup context off"`)
	}
	return 0
}

// contextOff restores the context that was current before "dockup context use".
func contextOff(cfg userconfig.Config) int {
	info := dockctx.Get(cfg.EffectivePipe())
	if !info.CLI {
		logx.Err("docker CLI not found on PATH (install it from docker.com or with scoop)")
		return 1
	}
	if !info.Active {
		logx.Info("docker already uses context %q", info.Current)
		return 0
	}
	target := strings.TrimSpace(cfg.DockerContextPrev)
	if target == "" || target == dockctx.Name || !dockctx.Exists(target) {
		target = "default"
	}
	if err := dockctx.UseContext(target); err != nil {
		logx.ErrFrom(err)
		return 1
	}
	if cfg.DockerContextPrev != "" {
		cfg.DockerContextPrev = ""
		_ = userconfig.Save(cfg)
	}
	logx.Ok("docker now uses context %q", target)
	return 0
}

// dockupDistroRSS returns in-distro memory, zero when unreachable.
func dockupDistroRSS() uint64 {
	n, err := sysinfo.DistroRSS(config.DistroName)
	if err != nil {
		return 0
	}
	return n
}

// cmdSSH opens a shell in the distro (no args) or runs one remote command.
// A leading -- ends our parsing: everything after it runs remotely verbatim.
func cmdSSH(args []string) int {
	verbatim := false
	if len(args) > 0 && args[0] == "--" {
		verbatim = true
		args = args[1:]
	}
	if !verbatim && hasHelpFlag(args) {
		sshHelp()
		return 0
	}
	s, _ := state.Load()
	if !s.Installed && !wsl.Exists(config.DistroName) {
		fmt.Fprintln(os.Stderr, ui.Red(`dockup has not been setup yet run "dockup setup" to set it up.`))
		return 1
	}
	return wsl.Shell(config.DistroName, args...)
}

// cmdPrune reclaims distro disk space via in-distro docker prune and, unless
// --keep-image is set, deletes the cached setup image. It works after
// uninstall too, when only the cache is left.
func cmdPrune(args []string) int {
	if hasHelpFlag(args) {
		pruneHelp()
		return 0
	}
	var all, volumes, force, keepImage bool
	for _, a := range args {
		switch a {
		case "--all":
			all = true
		case "--volumes":
			volumes = true
		case "--force", "-f":
			force = true
		case "--keep-image":
			keepImage = true
		default:
			logx.Err("unknown prune flag %q (try dockup prune --help)", a)
			return 1
		}
	}
	root := userconfig.InstallRoot()
	s, _ := state.Load()
	distroPresent := wsl.Exists(config.DistroName)
	if s.Installed && !distroPresent {
		logx.Warn("state says installed but WSL has no dockup distro; skipping the in-distro prune (run dockup doctor)")
	}
	cachePresent := image.CacheExists(root)
	if !distroPresent && !cachePresent {
		fmt.Fprintln(os.Stderr, ui.Red(`dockup has not been setup yet run "dockup setup" to set it up.`))
		return 1
	}
	if !distroPresent && (all || volumes) {
		logx.Warn("distro is not installed; --all/--volumes have no effect")
	}
	if !distroPresent && keepImage {
		logx.Info("nothing to prune (image cache kept)")
		return 0
	}
	var parts []string
	if distroPresent {
		dockerWhat := "stopped containers, unused networks and dangling images"
		if all {
			dockerWhat = "stopped containers, unused networks and all unused images"
		}
		if volumes {
			dockerWhat += ", plus unused volumes (their data is destroyed)"
		}
		parts = append(parts, dockerWhat)
	}
	if cachePresent && !keepImage {
		parts = append(parts, "the cached setup image (setup will download it again)")
	}
	what := strings.Join(parts, ", plus ")
	if !force && !ui.Confirm(ui.White(fmt.Sprintf("Reclaim space? This removes %s.", what))) {
		logx.Info("aborted")
		return 0
	}
	var firstErr error
	if distroPresent {
		if err := docker.Prune(config.DistroName, all, volumes); err != nil {
			firstErr = err
		}
	}
	if cachePresent && !keepImage {
		if n, err := image.Clear(root); err != nil && firstErr == nil {
			firstErr = err
		} else if err == nil && n > 0 {
			logx.Info("removed cached setup image (%s)", sysinfo.FormatSize(uint64(n)))
		}
	}
	if firstErr != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: prune failed: %v", firstErr)))
		return 1
	}
	logx.Ok("prune complete")
	return 0
}

func cmdDaemon(cfg userconfig.Config, args []string) int {
	if len(args) == 0 || hasHelpFlag(args) {
		daemonHelp()
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, ui.Red("dockup: daemon needs start|stop|restart|status|log|autostart"))
			return 1
		}
		return 0
	}
	// If foreground holds the pipe, daemon start refuses.
	s, _ := state.Load()
	if relay.AliveOn(cfg.EffectivePipe()) && s.Daemon.PID == 0 && !daemon.DaemonAlive(s) {
		if args[0] == "start" {
			fmt.Fprintln(os.Stderr, ui.Red("dockup is already running in the foreground. Stop it with Ctrl+C or run dockup shutdown."))
			return 1
		}
	}
	noArgs := func(sub string) bool {
		if len(args[1:]) > 0 {
			logx.Err("dockup daemon %s takes no arguments (try dockup daemon --help)", sub)
			return false
		}
		return true
	}
	switch args[0] {
	case "start":
		if !noArgs("start") {
			return 1
		}
		if err := daemon.Start(); err != nil {
			logx.ErrFrom(err)
			return 1
		}
		return 0
	case "stop":
		if !noArgs("stop") {
			return 1
		}
		if err := daemon.Stop(); err != nil {
			logx.ErrFrom(err)
			return 1
		}
		return 0
	case "restart":
		if !noArgs("restart") {
			return 1
		}
		_ = daemon.Stop()
		time.Sleep(1 * time.Second)
		if err := daemon.Start(); err != nil {
			logx.ErrFrom(err)
			return 1
		}
		return 0
	case "status":
		if !noArgs("status") {
			return 1
		}
		return daemon.Status()
	case "log":
		if !noArgs("log") {
			return 1
		}
		return daemonLog()
	case "autostart":
		return cmdAutostart(args[1:])
	default:
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: unknown daemon command %q", args[0])))
		return 1
	}
}

// cmdAutostart queries or sets Windows login autostart.
// `dockup daemon autostart` prints "autostart: on|off";
// `dockup daemon autostart on|off` applies it (config + Startup entry).
func cmdAutostart(args []string) int {
	if hasHelpFlag(args) {
		daemonHelp()
		return 0
	}
	cfg, err := userconfig.Ensure()
	if err != nil {
		logx.Err("%v", err)
		return 1
	}
	cfg = cfg.WithDefaults()
	if len(args) == 0 {
		fmt.Printf("%s\n", ui.White(fmt.Sprintf("autostart: %s", pstable.AutostartText(cfg.Autostart))))
		return 0
	}
	if len(args) > 1 {
		logx.Err("usage: dockup daemon autostart [on|off]")
		return 1
	}
	var on bool
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on", "enable", "true":
		on = true
	case "off", "disable", "false":
		on = false
	default:
		logx.Err("usage: dockup daemon autostart [on|off]")
		return 1
	}
	exe, _ := os.Executable()
	if err := autostart.SetEnabled(exe, on); err != nil {
		logx.Err("%v", err)
		return 1
	}
	cfg.Autostart = on
	if err := userconfig.Save(cfg); err != nil {
		logx.Err("%v", err)
		return 1
	}
	logx.Ok("autostart: %s", pstable.AutostartText(on))
	return 0
}

// daemonLog tails the log file read-only until Ctrl+C.
func daemonLog() int {
	path := config.LogFile()
	fmt.Printf("%s\n", ui.Gray(fmt.Sprintf("showing %s (read-only, Ctrl+C to exit)...", path)))
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
		data, err := os.ReadFile(path) // #nosec G304 -- path is our own log file location, never remote input
		if err != nil {
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: no log yet (%v)", err)))
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

func cmdShutdown(cfg userconfig.Config) int {
	_ = daemon.Stop()
	// Escape hatch for an unresponsive foreground: kill any other process of
	// our own binary (name-verified, so a reused PID can never hit an
	// unrelated process), then wait for the pipe to die before terminating
	// the distro. Failures are loud: a silent skip is exactly how a stuck
	// foreground survives shutdown unnoticed.
	for _, pid := range sysinfo.DockupPIDs() {
		proc, err := os.FindProcess(pid)
		if err != nil {
			logx.Warn("cannot signal pid %d: %v", pid, err)
			continue
		}
		if err := proc.Kill(); err != nil {
			logx.Warn("cannot stop dockup (pid %d): %v; try an elevated terminal", pid, err)
			continue
		}
		logx.Info("stopped foreground dockup (pid %d)", pid)
	}
	_ = relay.WaitDeadOn(cfg.EffectivePipe(), 10*time.Second)
	_ = wsl.Terminate(config.DistroName)
	if relay.AliveOn(cfg.EffectivePipe()) {
		fmt.Fprintln(os.Stderr, ui.Red("dockup: pipe still held by another program. Stop it before retrying"))
		return 1
	}
	fmt.Printf("%s\n", ui.Green("dockup shutdown complete (stopped)"))
	return 0
}
