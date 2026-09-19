// Command dockup is the single static Windows CLI per revision.md.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/autostart"
	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/daemon"
	"github.com/CHE3MZ/dockup/internal/doctor"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/pstable"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/setup"
	"github.com/CHE3MZ/dockup/internal/state"
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
// continues with defaults.
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
		if err := doctor.Run(); err != nil {
			fmt.Fprintln(os.Stderr, ui.Red(fmt.Sprintf("dockup: %v", err)))
			return 1
		}
		return 0
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
  ` + ui.Bold("dockup") + `                  foreground run (Ctrl+C to stop)
  ` + ui.Bold("dockup setup [--amd|--arm] [--path=DIR] [--dry-run]") + `
  ` + ui.Bold("dockup uninstall") + `
  ` + ui.Bold("dockup ps") + `                     STATUS / AUTOSTART table
  ` + ui.Bold("dockup daemon start|stop|restart|status|log|autostart") + `
  ` + ui.Bold("dockup shutdown") + `           stop everything
  ` + ui.Bold("dockup doctor") + `             preflight + repair stale state
  ` + ui.Bold("dockup version") + `
  ` + ui.Bold("dockup help [command]") + `     show help (also -h / --help everywhere)
` + ui.Gray("Config: ~/.dockup/config.json (default_path, current_path, port, use_tcp, pipe_name, color)") + `
`)
}

func helpTopic(name string) int {
	switch name {
	case "setup":
		setupHelp()
	case "uninstall":
		uninstallHelp()
	case "ps":
		psHelp()
	case "daemon":
		daemonHelp()
	case "shutdown":
		shutdownHelp()
	case "doctor":
		doctorHelp()
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
  Install the dedicated ` + ui.Cyan(`"dockup"`) + ` Debian distro into WSL,
  install + configure the Docker daemon (systemd), and verify the bridge.

` + ui.LightBlue("Usage:") + `
  dockup setup [--amd|--arm] [--path=DIR] [--dry-run]

` + ui.LightBlue("Options:") + `
  ` + ui.Bold("--amd, --arm") + `     distro architecture (default: --amd)
  ` + ui.Bold("--path=DIR") + `     install directory, e.g. --path="D:/WSL"
                  skips the interactive prompt; becomes the new default
  ` + ui.Bold("--dry-run") + `       print what would happen without changing anything
  ` + ui.Bold("-h, --help") + `       show this help

  Without --path you are asked:
    ` + ui.Gray(`where do you want to install the dockup distro? [default: <last used>]`) + `
  An empty answer keeps the default. The chosen path is saved as both
  ` + ui.Cyan("default_path") + ` (prefilled next time) and ` + ui.Cyan("current_path") + ` in
  ` + ui.Cyan("~/.dockup/config.json") + `.
`)
}

func uninstallHelp() {
	fmt.Print(ui.Header("dockup uninstall") + `
  Remove the dockup WSL distro and clear its state.

` + ui.LightBlue("Usage:") + `
  dockup uninstall
`)
}

func psHelp() {
	fmt.Print(ui.Header("dockup ps") + `
  Show dockup status as a table (bold headers, plain values):

    STATUS      AUTOSTART
    running     off

  STATUS is ` + ui.Green("running") + ` (engine answering), ` + ui.White("starting...") + `
  (bridge up, engine still booting) or ` + ui.White("stopped") + `.
  AUTOSTART mirrors ~/.dockup/config.json (` + ui.Cyan("on") + `/` + ui.Cyan("off") + `).

` + ui.LightBlue("Usage:") + `
  dockup ps
`)
}

func daemonHelp() {
	fmt.Print(ui.Header("dockup daemon") + `
  Manage the background dockup process (named pipe + optional TCP bridge).

` + ui.LightBlue("Usage:") + `
  dockup daemon start     start the background process
  dockup daemon stop      stop the background process
  dockup daemon restart   restart the background process
  dockup daemon status    brief health (running / stopped)
  dockup daemon log       follow the log (read-only, Ctrl+C to exit)
  dockup daemon autostart [on|off]
                          query (no arg) or set Windows login autostart
                          (default off, stored in ~/.dockup/config.json)
`)
}

func shutdownHelp() {
	fmt.Print(ui.Header("dockup shutdown") + `
  Stop all dockup processes (foreground hint + daemon) and terminate
  the dockup WSL distro.

` + ui.LightBlue("Usage:") + `
  dockup shutdown
`)
}

func doctorHelp() {
	fmt.Print(ui.Header("dockup doctor") + `
  Preflight checks (WSL, distro, systemd, docker socket, pipe/TCP,
  config) plus repair of stale state.

` + ui.LightBlue("Usage:") + `
  dockup doctor
`)
}

func versionHelp() {
	fmt.Print(ui.Header("dockup version") + `
  Print the running version.

` + ui.LightBlue("Usage:") + `
  dockup version
`)
}

// foreground starts the relay inline. Refuses if already running or not setup.
func foreground(cfg userconfig.Config) int {
	pipe := cfg.EffectivePipe()
	s, _ := state.Load()
	if !s.Installed && !wsl.Exists(config.DistroName) {
		fmt.Fprintln(os.Stderr, ui.Red(`dockup has not been setup yet run "dockup setup" to set it up.`))
		return 1
	}
	if s.Daemon.PID != 0 && daemon.DaemonAlive(s) {
		fmt.Fprintln(os.Stderr, ui.Red("dockup is already running as a daemon process, run dockup daemon stop first."))
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
	fmt.Printf("%s\n", ui.White("starting helper..."))
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
	fmt.Printf("%s\n", ui.Green(fmt.Sprintf("helper up and running on %s", pipe)))
	if cfg.UseTCP {
		fmt.Printf("%s\n", ui.White(fmt.Sprintf("tcp bridge on %s", cfg.TCPAddr())))
	}
	fmt.Printf("%s\n", ui.Cyan("use: docker -H npipe:////./pipe/dockup_engine version"))
	fmt.Printf("%s\n", ui.White("dockup running in foreground (Ctrl+C to stop)..."))
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
	fmt.Println(pstable.Render(status, cfg.Autostart))
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

func cmdDaemon(cfg userconfig.Config, args []string) int {
	if len(args) == 0 || hasHelpFlag(args) {
		daemonHelp()
		if len(args) == 0 {
			fmt.Fprintln(os.Stderr, ui.Red("dockup: daemon needs start|stop|restart|status|log"))
			return 1
		}
		return 0
	}
	// If foreground holds the pipe, daemon start refuses.
	s, _ := state.Load()
	if relay.AliveOn(cfg.EffectivePipe()) && s.Daemon.PID == 0 && !daemon.DaemonAlive(s) {
		if args[0] == "start" {
			fmt.Fprintln(os.Stderr, ui.Red("dockup is already running as a foreground process, ctrl + C in order to use the dockup daemon."))
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
		data, err := os.ReadFile(path)
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
