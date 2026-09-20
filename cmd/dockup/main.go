// Command dockup is the single static Windows CLI per revision.md.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/autostart"
	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/daemon"
	"github.com/CHE3MZ/dockup/internal/docker"
	"github.com/CHE3MZ/dockup/internal/doctor"
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
	switch args[0] {
	case "--help", "-h", "help":
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
		fmt.Printf("%s", ui.White(fmt.Sprintf("you're running the %s version.\n", version)))
		return 0
	case "setup":
		return cmdSetup(cfg, args[1:])
	case "uninstall":
		if hasHelpFlag(args[1:]) {
			uninstallHelp()
			return 0
		}
		return setup.Uninstall()
	case "restore":
		return cmdRestore(args[1:])
	case "ps":
		if hasHelpFlag(args[1:]) {
			psHelp()
			return 0
		}
		return cmdPs(cfg)
	case "daemon":
		return cmdDaemon(cfg, args[1:])
	case "shutdown":
		if hasHelpFlag(args[1:]) {
			shutdownHelp()
			return 0
		}
		return cmdShutdown(cfg)
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
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: %v", err)))
			return 1
		}
		return 0
	case "upgrade":
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

func cmdSetup(cfg userconfig.Config, args []string) int {
	if hasHelpFlag(args) {
		setupHelp()
		return 0
	}
	var amd, arm, dryRun bool
	var pathFlag string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--amd":
			amd = true
		case a == "--arm":
			arm = true
		case a == "--dry-run":
			dryRun = true
		case a == "--path" && i+1 < len(args):
			i++
			pathFlag = args[i]
		case strings.HasPrefix(a, "--path="):
			pathFlag = strings.TrimPrefix(a, "--path=")
		default:
			logx.Err("unknown setup flag %q (try dockup setup --help)", a)
			return 1
		}
	}
	arch, msg := config.NormalizeArch(amd, arm)
	if msg != "" {
		logx.Err("%s", msg)
		return 1
	}
	return setup.RunEx(setup.Options{Arch: arch, Path: pathFlag, DryRun: dryRun, Cfg: cfg})
}

func usage() {
	fmt.Print(ui.Header("dockup") + ui.White(" — docker engine in a dedicated WSL distro.\n") + `
` + ui.LightBlue("Usage:") + `
  ` + ui.Bold("dockup") + `                     Foreground run (Ctrl+C to stop)
  ` + ui.Bold("dockup setup") + `               Launch the interactive setup wizard
  ` + ui.Bold("dockup uninstall") + `           Uninstall the dockup distro from WSL
  ` + ui.Bold("dockup restore [--full]") + `      Reset the distro to a clean state
  ` + ui.Bold("dockup ps") + `                  Show dockup's status
  ` + ui.Bold("dockup daemon") + `              Start | Stop | Restart | Status
  ` + ui.Bold("dockup shutdown") + `            Stop everything
  ` + ui.Bold("dockup doctor [--fix]") + `      Repair stale states
  ` + ui.Bold("dockup upgrade") + `             Upgrade the in-distro engine to latest
  ` + ui.Bold("dockup version") + `             Show current version
  ` + ui.Bold("dockup help [command]") + `      Show this help text
 ` + "\n" +
		ui.Gray("Config File: ~/.dockup/config.json") + `
 `)
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
	case "daemon":
		daemonHelp()
	case "shutdown":
		shutdownHelp()
	case "doctor":
		doctorHelp()
	case "upgrade":
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
  Install Debian into WSL as the ` + ui.Cyan(`"dockup"`) + ` distro, set up
  the Docker daemon on it, and check that the bridge works.

` + ui.LightBlue("Usage:") + `
  dockup setup [--amd|--arm] [--path=DIR] [--dry-run]

` + ui.LightBlue("Options:") + `
  ` + ui.Bold("--amd, --arm") + `    Debian architecture (default: --amd)
  ` + ui.Bold("--path=DIR") + `      Install here instead of asking, e.g. --path="D:/WSL".
                   The folder you give becomes the new default.
  ` + ui.Bold("--dry-run") + `       Show what setup would do, without changing anything.
  ` + ui.Bold("-h, --help") + `      Show this help.

  Without --path you get asked where to install. An empty answer keeps
  the shown default. The folder you pick is saved as both
  ` + ui.Cyan("default_path") + ` (suggested next time) and ` + ui.Cyan("current_path") + ` in
  ` + ui.Cyan("~/.dockup/config.json") + `.
`)
}

func uninstallHelp() {
	fmt.Print(ui.Header("dockup uninstall") + `
  Delete the dockup distro from WSL and clear its saved state.

` + ui.LightBlue("Usage:") + `
  dockup uninstall
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
  dockup restore [--full] [--amd|--arm] [--path=DIR]
`)
}

// cmdRestore parses restore flags. --amd/--arm/--path only apply to
// --full (lightweight restores in place).
func cmdRestore(args []string) int {
	if hasHelpFlag(args) {
		restoreHelp()
		return 0
	}
	var full, amd, arm bool
	var pathFlag string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--full":
			full = true
		case a == "--amd":
			amd = true
		case a == "--arm":
			arm = true
		case a == "--path" && i+1 < len(args):
			i++
			pathFlag = args[i]
		case strings.HasPrefix(a, "--path="):
			pathFlag = strings.TrimPrefix(a, "--path=")
		default:
			logx.Err("unknown restore flag %q (try dockup restore --help)", a)
			return 1
		}
	}
	arch, msg := config.NormalizeArch(amd, arm)
	if msg != "" {
		logx.Err("%s", msg)
		return 1
	}
	if (pathFlag != "" || amd || arm) && !full {
		logx.Err("--path/--amd/--arm only apply to restore --full")
		return 1
	}
	var archOverride string
	if amd || arm {
		archOverride = arch
	}
	return restore.Run(restore.Options{Full: full, Arch: archOverride, Path: pathFlag})
}

func psHelp() {
	fmt.Print(ui.Header("dockup ps") + `
  Show dockup's status:

    STATUS      AUTOSTART   INSTALLED   SIZE      MEMORY
    running     off         yes         452 MB    128 MB

  STATUS is ` + ui.Green("running") + ` (engine answering), ` + ui.White("starting...") + `
  (bridge up, engine still booting) or ` + ui.White("stopped") + `.
  AUTOSTART is ` + ui.Cyan("on") + ` or ` + ui.Cyan("off") + `, INSTALLED is ` + ui.Cyan("yes") + ` or ` + ui.Cyan("no") + `,
  SIZE is the distro's disk use, MEMORY its live RAM use (both from
  ~/.dockup/config.json state and live probes).

` + ui.LightBlue("Usage:") + `
  dockup ps
`)
}

func daemonHelp() {
	fmt.Print(ui.Header("dockup daemon") + `
  Run dockup in the background (named pipe, plus TCP if enabled).

` + ui.LightBlue("Usage:") + `
  dockup daemon start     start the background process
  dockup daemon stop      stop the background process
  dockup daemon restart   restart the background process
  dockup daemon status    brief health (running / stopped)
  dockup daemon log       follow the log (read-only, Ctrl+C to exit)
  dockup daemon autostart [on|off]
                          show (no arg) or set starting dockup at Windows login
                          (default off, stored in ~/.dockup/config.json)
`)
}

func shutdownHelp() {
	fmt.Print(ui.Header("dockup shutdown") + `
  Stop everything dockup is running and shut down its WSL distro.

` + ui.LightBlue("Usage:") + `
  dockup shutdown
`)
}

func doctorHelp() {
	fmt.Print(ui.Header("dockup doctor") + `
  Check that everything dockup needs is healthy, and clean up
  stale state left behind by crashes or reboots.

  With ` + ui.Bold("--fix") + `, a missing or broken in-distro engine is
  reinstalled and reconfigured, then verified.

` + ui.LightBlue("Usage:") + `
  dockup doctor [--fix]
`)
}

func upgradeHelp() {
	fmt.Print(ui.Header("dockup upgrade") + `
  Update Docker and its dependencies inside the dockup distro to the
  latest versions, restart its services, and verify the daemon answers.

` + ui.LightBlue("Usage:") + `
  dockup upgrade
`)
}

// cmdUpgrade upgrades the in-distro engine to latest.
func cmdUpgrade() int {
	s, _ := state.Load()
	if !s.Installed && !wsl.Exists(config.DistroName) {
		fmt.Fprintln(os.Stderr, ui.Red(`dockup has not been setup yet run "dockup setup" to set it up.`))
		return 1
	}
	fmt.Printf("%s [y/n]\n", ui.White("Upgrade the engine inside the dockup distro to the latest versions? This needs an internet connection."))
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	if l := strings.ToLower(strings.TrimSpace(line)); l != "y" && l != "yes" {
		logx.Info("aborted")
		return 0
	}
	fmt.Printf("%s\n", ui.White("upgrading docker engine inside dockup..."))
	if err := docker.Upgrade(config.DistroName); err != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: upgrade failed: %v", err)))
		return 1
	}
	logx.Ok("upgrade complete")
	return 0
}

func versionHelp() {
	fmt.Print(ui.Header("dockup version") + `
  Show the dockup version.

` + ui.LightBlue("Usage:") + `
  dockup version
`)
}

// foreground starts the relay inline. Refuses if already running or not setup.
func foreground(cfg userconfig.Config) int {
	pipe := cfg.EffectivePipe()
	s, _ := state.Load()
	if !s.Installed && !wsl.Exists(config.DistroName) {
		fmt.Fprintln(os.Stderr, ui.Red(`dockup has not been setup yet run "dockup setup" to set it up. run "dockup help" to see all available commands.`))
		return 1
	}
	if s.Daemon.PID != 0 && daemon.DaemonAlive(s) {
		fmt.Fprintln(os.Stderr, ui.Red("dockup is already running in the background — run dockup daemon stop first."))
		return 1
	}
	if relay.AliveOn(pipe) {
		if !s.Installed {
			fmt.Fprintln(os.Stderr, ui.Red("dockup: pipe is held by another program — stop it before running dockup."))
		} else {
			fmt.Fprintln(os.Stderr, ui.Red("dockup is already running (another foreground or daemon holds the pipe)."))
		}
		return 1
	}
	fmt.Printf("%s\n", ui.White("Starting dockup..."))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- relay.ServeEx(ctx, config.DistroName, pipe, cfg.UseTCP, cfg.EffectivePort())
	}()
	if !relay.WaitAliveOn(pipe, 10*time.Second) {
		select {
		case err := <-errCh:
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
		default:
			fmt.Fprintln(os.Stderr, ui.Red("dockup: helper failed (pipe never came up)"))
		}
		cancel()
		return 1
	}
	fmt.Printf("%s\n", ui.Green(fmt.Sprintf("dockup is up on %s", pipe)))
	if cfg.UseTCP {
		fmt.Printf("%s\n", ui.White(fmt.Sprintf("tcp bridge on %s", cfg.TCPAddr())))
	}
	fmt.Printf("%s\n", ui.Cyan("use: docker -H npipe:////./pipe/dockup_engine version"))
	fmt.Printf("%s\n", ui.White("Running in the foreground — press Ctrl+C to stop."))
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	select {
	case <-sig:
	case err := <-errCh:
		if err != nil {
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
			return 1
		}
	}
	cancel()
	_ = relay.WaitDeadOn(pipe, 5*time.Second)
	fmt.Printf("%s\n", ui.White("dockup stopped"))
	return 0
}

// serveForever is the hidden daemon child holding the pipe.
func serveForever(cfg userconfig.Config) int {
	s, _ := state.Load()
	if !s.Installed {
		fmt.Fprintln(os.Stderr, ui.Red("dockup: helper failed (not setup)"))
		return 1
	}
	if err := userconfig.ValidatePort(cfg.EffectivePort()); cfg.UseTCP && err != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
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
	if err := relay.ServeEx(ctx, config.DistroName, cfg.EffectivePipe(), cfg.UseTCP, cfg.EffectivePort()); err != nil {
		fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: helper failed: %v", err)))
		return 1
	}
	return 0
}

func cmdPs(cfg userconfig.Config) int {
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
		if n, err := sysinfo.DirSize(userconfig.InstallDir()); err == nil {
			row.Size = sysinfo.FormatSize(n)
		}
		if alive {
			if total := sysinfo.DockupWindowsRSS() + dockupDistroRSS(); total > 0 {
				row.Memory = sysinfo.FormatSize(total)
			}
		}
	}
	fmt.Println(pstable.RenderRow(row))
	if alive && !s.Installed {
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

// dockupDistroRSS returns in-distro memory, zero when unreachable.
func dockupDistroRSS() uint64 {
	n, err := sysinfo.DistroRSS(config.DistroName)
	if err != nil {
		return 0
	}
	return n
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
			fmt.Fprintln(os.Stderr, ui.Red("dockup is already running in the foreground — stop it with Ctrl+C first."))
			return 1
		}
	}
	switch args[0] {
	case "start":
		if err := daemon.Start(); err != nil {
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: %v", err)))
			return 1
		}
		return 0
	case "stop":
		if err := daemon.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: %v", err)))
			return 1
		}
		return 0
	case "restart":
		_ = daemon.Stop()
		time.Sleep(1 * time.Second)
		if err := daemon.Start(); err != nil {
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: %v", err)))
			return 1
		}
		return 0
	case "status":
		return daemon.Status()
	case "log":
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
	_ = wsl.Terminate(config.DistroName)
	if relay.AliveOn(cfg.EffectivePipe()) {
		fmt.Fprintln(os.Stderr, ui.Red("dockup: pipe still held (a foreground dockup may still be running — Ctrl+C it)"))
		return 1
	}
	fmt.Printf("%s\n", ui.Green("dockup shutdown complete (stopped)"))
	return 0
}
