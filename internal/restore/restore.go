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

// Run restores the distro; full selects the unregister+reinstall path.
func Run(full bool) int {
	distro := config.DistroName
	if !wsl.Exists(distro) {
		logx.Err("nothing to restore — run dockup setup first")
		return 1
	}
	if full {
		return runFull()
	}
	return runLight()
}

func runFull() int {
	st, _ := state.Load()
	arch := st.Arch
	if arch != "amd64" && arch != "arm64" {
		arch = "amd64"
	}
	installDir := userconfig.InstallDir()
	fmt.Printf("%s\n", ui.Yellow("This deletes the distro and reinstalls it from scratch."))
	fmt.Printf("%s\n", ui.Yellow("Containers, images, and volumes will be destroyed."))
	fmt.Printf("%s [y/n]\n", ui.White("Really restore with a full reinstall?"))
	if !readYes() {
		logx.Info("aborted")
		return 0
	}
	return setup.RunEx(setup.Options{Arch: arch, Path: installDir, Cfg: mustConfig()})
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
	fmt.Printf("%s [y/n]\n", ui.White("Restore the distro as described above?"))
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
	if err := docker.TestDaemon(distro); err != nil {
		logx.Info("engine unhealthy, reinstalling engine packages...")
		if err := docker.Install(distro); err != nil {
			logx.Err("%v", err)
			return 1
		}
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

func mustConfig() userconfig.Config {
	cfg, _ := userconfig.Ensure()
	return cfg.WithDefaults()
}
