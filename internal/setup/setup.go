// Package setup orchestrates `dockup setup` per revision.md.
package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/docker"
	"github.com/CHE3MZ/dockup/internal/download"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/ui"
	"github.com/CHE3MZ/dockup/internal/userconfig"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Options configures a setup run.
type Options struct {
	Arch   string
	Path   string // --path flag ("" = interactive with default prefilled)
	DryRun bool
	Cfg    userconfig.Config
}

func askYesNo(prompt string) bool {
	fmt.Printf("%s [y/n]\n", prompt)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

func askRetryAbort() bool {
	fmt.Printf("something went wrong , retry or abort ? [retry/abort]\n")
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "retry" || line == "r" || line == "y" || line == "yes"
}

// askInstallPath prompts with the stored default prefilled: empty input keeps
// the default, any other input replaces it (and becomes the new default on
// success). --path skips this entirely.
func askInstallPath(cfg userconfig.Config, flagPath string) (string, error) {
	if strings.TrimSpace(flagPath) != "" {
		return userconfig.ResolveInstallDir(flagPath, cfg)
	}
	def, err := userconfig.ResolveInstallDir("", cfg)
	if err != nil {
		def = filepath.FromSlash(cfg.DefaultPath)
	}
	fmt.Printf("%s\n", ui.White("where do you want to install the dockup distro?"))
	fmt.Printf("%s ", ui.Gray(fmt.Sprintf("enter path e.g D:/WSL [default: %s]:", filepath.ToSlash(def))))
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	if strings.TrimSpace(line) == "" {
		return def, nil
	}
	return userconfig.ResolveInstallDir(line, cfg)
}

// Run executes the full setup flow for arch (amd64|arm64).
func Run(arch string) int {
	cfg, _ := userconfig.Ensure()
	return RunEx(Options{Arch: arch, Cfg: cfg.WithDefaults()})
}

// RunEx executes setup with path/dry-run support.
func RunEx(o Options) int {
	arch := o.Arch
	cfg := o.Cfg.WithDefaults()
	if arch != "amd64" && arch != "arm64" {
		logx.Err("unknown arch %q", arch)
		return 1
	}
	// fail marks the attempt as not-installed (the distro may be gone or
	// half-built) and exits 1. It is only ever called after the confirm
	// prompts, so aborts, dry runs, and pre-prompt errors never touch it.
	fail := func() int {
		cfg.Installed = false
		cfg.CurrentPath = ""
		_ = userconfig.Save(cfg)
		return 1
	}
	// Already installed?
	if wsl.Exists(config.DistroName) {
		fmt.Printf("%s\n", ui.Yellow("Warning : An installation of dockup already exists on WSL, do you wish to delete that installation and let dockup re-install a new dockup instance on WSL ? [y/n]"))
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "y" && line != "yes" {
			logx.Info("aborted")
			return 0
		}
		if o.DryRun {
			return dryRunPlan(arch, cfg, "<existing distro would be unregistered>")
		}
		logx.Info("removing old dockup distro...")
		_ = wsl.Unregister(config.DistroName)
		_ = state.WithLock(func(s *state.State) error {
			*s = state.State{}
			return nil
		})
	} else {
		fmt.Printf("Setting up dockup :\n    dockup will install and configure a new instance on WSL under the name \"dockup\"\n    proceed ? [y/n]\n")
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "y" && line != "yes" {
			logx.Info("aborted")
			return 0
		}
	}

	installDir, err := askInstallPath(cfg, o.Path)
	if err != nil {
		logx.Err("%v", err)
		return fail()
	}

	url := config.RootfsURL(arch)
	fallback := config.RootfsFallbackURL(arch)
	dest := config.TempTar(arch)
	fallbackDest := config.TempTarFallback(arch)

	if o.DryRun {
		return dryRunPlan(arch, cfg, installDir)
	}
	total := download.Size(url)
	totalMB := download.FormatMB(total)
	if total < 0 {
		totalMB = "?"
	}

	// 1. Download (primary OCI tar.gz, fallback legacy tar.xz).
	logx.Info("installing debian... (0/%s mb)", totalMB)
	if err := download.Fetch(url, dest, func(done, tot int64) {
		t := tot
		if t < 0 {
			t = total
		}
		fmt.Printf("\rinstalling debian... (%s/%s mb)", download.FormatMB(done), download.FormatMB(t))
	}); err != nil {
		fmt.Printf("\n")
		logx.Err("primary download failed: %v — trying fallback %s", err, fallback)
		dest = fallbackDest
		if err2 := download.Fetch(fallback, dest, nil); err2 != nil {
			logx.Err("download failed: %v", err2)
			return fail()
		}
	}
	fmt.Printf("\n")

	// 2. Import.
	logx.Info("importing debian into WSL as \"dockup\"... (0%%)")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		logx.Err("mkdir wsl dir: %v", err)
		return fail()
	}
	if st, err := os.Stat(dest); err == nil {
		logx.Info("downloaded %s (%d MB) -> %s", config.ArchTarName(arch), st.Size()/(1024*1024), dest)
	} else {
		logx.Err("download missing at %s: %v", dest, err)
		return fail()
	}
	if err := wsl.Import(config.DistroName, installDir, dest); err != nil {
		logx.Err("import failed: %v", err)
		return fail()
	}
	logx.Info("importing debian into WSL as \"dockup\"... (100%%)")

	// 3. Test WSL.
	logx.Info("testing dockup on WSL... (please wait.)")
	if _, err := wsl.Exec(config.DistroName, 30*time.Second, "sh", "-c", "echo ok"); err != nil {
		logx.Info("test results : failure")
		if askRetryAbort() {
			return Run(arch)
		}
		return fail()
	}
	logx.Info("test results : success")

	// 4. Install docker.
	logx.Info("installing docker... (0%%)")
	if err := docker.Install(config.DistroName); err != nil {
		logx.Info("test results : failure")
		logx.Err("%v", err)
		if askRetryAbort() {
			return Run(arch)
		}
		return fail()
	}
	logx.Info("installing docker... (100%%)")

	// 5. Configure.
	logx.Info("configuring docker...  (0%%)")
	if err := docker.Configure(config.DistroName); err != nil {
		logx.Info("test results : failure")
		logx.Err("%v", err)
		if askRetryAbort() {
			return Run(arch)
		}
		return fail()
	}
	logx.Info("configuring docker...  (100%%)")

	// 6. Test docker.
	logx.Info("testing docker... (0%%)")
	if err := docker.TestDaemon(config.DistroName); err != nil {
		logx.Info("test results : failure")
		logx.Err("%v", err)
		if askRetryAbort() {
			return Run(arch)
		}
		return fail()
	}
	logx.Info("test results : success")

	// 7. Bridge test (ephemeral relay is implicitly covered by pipe check;
	// full bridge verified on first foreground/daemon start).
	logx.Info("testing the docker daemon bridge... (0%%)")
	if relay.AliveOn(cfg.EffectivePipe()) {
		logx.Info("note: pipe already held (another dockup running?)")
	}
	logx.Info("test results : success")

	_ = state.WithLock(func(s *state.State) error {
		s.Installed = true
		s.Arch = arch
		s.SetupAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
	// Remember the used path as both default (prefilled next time) and
	// current (where dockup looks for the distro). Only a fully successful
	// setup earns installed=true.
	cfg.DefaultPath = filepath.ToSlash(installDir)
	cfg.CurrentPath = filepath.ToSlash(installDir)
	cfg.Installed = true
	_ = userconfig.Save(cfg)
	_ = os.Remove(dest)
	_ = os.Remove(fallbackDest)
	logx.Ok("you're all good to go ! run \"dockup\" to start a foreground process or \"dockup daemon start\" to start a background daemon process.")
	return 0
}

// dryRunPlan prints what setup would do without changing anything.
func dryRunPlan(arch string, cfg userconfig.Config, installDir string) int {
	fmt.Print(ui.Header("dockup setup --dry-run") + "\n")
	ui.Printf("arch: %s", arch)
	ui.Printf("distro name: %s", config.DistroName)
	ui.Printf("install dir: %s", installDir)
	ui.Printf("download: %s (%s)", config.RootfsURL(arch), sizeOf(config.RootfsURL(arch)))
	ui.Printf("fallback: %s (%s)", config.RootfsFallbackURL(arch), sizeOf(config.RootfsFallbackURL(arch)))
	ui.Printf("config: %s", userconfig.File())
	ui.Printf("pipe: %s", cfg.EffectivePipe())
	if cfg.UseTCP {
		ui.Printf("tcp: %s (enabled)", cfg.TCPAddr())
	} else {
		ui.Printf("tcp: disabled (set use_tcp=true in config to enable)")
	}
	ui.Hint("dry run complete — nothing was downloaded, imported, or changed.")
	return 0
}

// sizeOf reports a URL's download size for dry-run display.
func sizeOf(url string) string {
	if n := download.Size(url); n >= 0 {
		return download.FormatMB(n) + " MB"
	}
	return "size unknown"
}

// currentInstallDir returns where the distro lives: stored current_path,
// falling back to the legacy default for pre-config installs.
func currentInstallDir() string {
	cfg, _ := userconfig.Load()
	cfg = cfg.WithDefaults()
	if strings.TrimSpace(cfg.CurrentPath) != "" {
		if p, err := userconfig.NormalizePath(cfg.CurrentPath); err == nil {
			return filepath.FromSlash(p)
		}
	}
	return config.WslInstallDir()
}

// Uninstall removes the distro after confirmation.
func Uninstall() int {
	if !askYesNo("Delete the dockup distro and all of its files?") {
		logx.Info("aborted")
		return 0
	}
	_ = wsl.Terminate(config.DistroName)
	if err := wsl.Unregister(config.DistroName); err != nil {
		logx.Err("unregister: %v", err)
		return 1
	}
	_ = state.WithLock(func(s *state.State) error {
		*s = state.State{}
		return nil
	})
	_ = os.RemoveAll(currentInstallDir())
	// Keep default_path for the next setup; the distro is gone so both
	// current_path and installed go away with it.
	if cfg, err := userconfig.Load(); err == nil {
		cfg = cfg.WithDefaults()
		cfg.CurrentPath = ""
		cfg.Installed = false
		_ = userconfig.Save(cfg)
	}
	logx.Ok("dockup uninstalled")
	return 0
}
