// Package restore returns a tampered-with distro to its default state.
//
// Lightweight (default): removes packages added after setup (via the
// setup-time snapshot), reinstalls the engine set, rewrites managed
// configs, rescues a broken daemon.json with backup, restarts units.
// Docker data (images, containers, volumes) is preserved.
//
// Full (--full): unregister + reinstall from the rootfs through the setup
// flow. Guaranteed pristine, destroys docker data.
package restore

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/docker"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/setup"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/ui"
	"github.com/CHE3MZ/dockup/internal/userconfig"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

// RemovalList returns installed-manual packages absent from the snapshot:
// the user-added set. Engine packages are never removed even if an ancient
// snapshot lacks them. Name-based, so legitimate upgrades are safe.
func RemovalList(current, snapshot []string) []string {
	engine := map[string]bool{}
	for _, p := range docker.EnginePackages {
		engine[p] = true
	}
	have := map[string]bool{}
	for _, p := range snapshot {
		have[p] = true
	}
	var out []string
	for _, p := range current {
		if have[p] || engine[p] {
			continue
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Options configures a restore run. Arch/Path only apply to --full
// (lightweight restores in place); empty means current settings.
type Options struct {
	Full bool
	Arch string
	Path string
}

// Run restores the distro; full selects the unregister+reinstall path.
func Run(o Options) int {
	distro := config.DistroName
	if !wsl.Exists(distro) {
		logx.Err("nothing to restore — run dockup setup first")
		return 1
	}
	if o.Full {
		return runFull(o)
	}
	return runLight()
}

func runFull(o Options) int {
	arch := o.Arch
	if arch == "" {
		arch, _ = stateArch()
	}
	if arch != "amd64" && arch != "arm64" {
		arch = "amd64"
	}
	installDir := o.Path
	if installDir == "" {
		installDir = userconfig.InstallDir()
	}
	fmt.Printf("%s [y/n]\n", ui.Yellow("Are you sure you want to fully restore dockup? This will uninstall and reinstall the entire distro."))
	if !readYes() {
		logx.Info("aborted")
		return 0
	}
	return setup.RunEx(setup.Options{Arch: arch, Path: installDir, Cfg: mustConfig()})
}

// stateArch reads the last successful setup arch without failing when
// state.json is missing or corrupt (a full restore must not need it).
func stateArch() (string, bool) {
	st, err := state.Load()
	if err != nil || (st.Arch != "amd64" && st.Arch != "arm64") {
		return "", false
	}
	return st.Arch, true
}

func runLight() int {
	distro := config.DistroName
	st, _ := state.Load()
	current, err := docker.ManualPackages(distro)
	if err != nil {
		logx.Err("cannot read package list: %v (try dockup restore --full)", err)
		return 1
	}
	removal := RemovalList(current, st.Snapshot.Manual)
	fmt.Printf("%s\n", ui.White("Restore plan (docker data is preserved):"))
	if len(removal) == 0 {
		fmt.Printf("%s\n", ui.Gray("  no user-added packages to remove"))
	} else {
		for _, p := range removal {
			fmt.Printf("%s\n", ui.White(fmt.Sprintf("  remove package: %s", p)))
		}
	}
	if len(st.Snapshot.Manual) == 0 {
		fmt.Printf("%s\n", ui.Yellow("  no setup snapshot: engine repair only"))
	}
	fmt.Printf("%s\n", ui.White("  rewrite wsl.conf, reinstall engine packages, restart services"))
	fmt.Printf("%s\n", ui.Yellow("  running containers will stop when the daemon restarts"))
	fmt.Printf("%s [y/n]\n", ui.White("Are you sure you want to restore dockup? This will reinstall and uninstall some packages and will need an internet connection."))
	if !readYes() {
		logx.Info("aborted")
		return 0
	}
	if len(removal) > 0 {
		logx.Info("removing %d added package(s)...", len(removal))
		script := "set -eu\nexport DEBIAN_FRONTEND=noninteractive\napt-get remove -y " + strings.Join(removal, " ") + "\napt-get autoremove -y\n"
		if _, err := wsl.RunRetry(distro, "remove added packages", 5*time.Minute, 2, script); err != nil {
			logx.Err("%v", err)
			return 1
		}
		logx.Ok("removed %d added package(s)", len(removal))
	}
	logx.Info("reinstalling engine packages...")
	if err := docker.Install(distro); err != nil {
		logx.Err("%v", err)
		return 1
	}
	logx.Info("repairing config and services...")
	note, err := docker.Repair(distro)
	if err != nil {
		logx.Err("%v", err)
		return 1
	}
	if note != "" {
		logx.Warn("%s", note)
	}
	if err := docker.TestDaemon(distro); err != nil {
		logx.Err("engine still unhealthy: %v (try dockup restore --full)", err)
		logx.Err("distro state for triage:\n%s", triage(distro))
		return 1
	}
	snap, _ := docker.ManualPackages(distro)
	_ = state.WithLock(func(s *state.State) error {
		s.Snapshot = state.Snapshot{At: time.Now().UTC().Format(time.RFC3339), Manual: snap}
		return nil
	})
	logx.Ok("restore complete — images, containers, and volumes are intact")
	return 0
}
func readYes() bool {
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// triage collects unit status + recent journal for a dead engine so a
// failed restore tells the user WHY instead of just that it failed.
func triage(distro string) string {
	status, _ := wsl.Exec(distro, 15*time.Second, "systemctl", "status", "docker.service", "--no-pager")
	journal, _ := wsl.ExecScript(distro, 15*time.Second, "journalctl -u docker.service --no-pager -n 15 || true")
	return "--- systemctl status docker ---\n" + wsl.Tail(status, 1500) +
		"\n--- journal (docker.service, last 15) ---\n" + wsl.Tail(journal, 2500)
}

func mustConfig() userconfig.Config {
	cfg, _ := userconfig.Ensure()
	return cfg.WithDefaults()
}
