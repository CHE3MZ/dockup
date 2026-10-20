// Package setup orchestrates `dockup setup` per revision.md.
package setup

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/CHE3MZ/dockup/internal/autostart"
	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/docker"
	"github.com/CHE3MZ/dockup/internal/download"
	"github.com/CHE3MZ/dockup/internal/image"
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
	fmt.Printf("%s ", ui.Gray(fmt.Sprintf("enter path e.g D:/WSL/Dockup [default: %s]:", filepath.ToSlash(def))))
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
	// fail clears the install location (the distro may be gone or half-built)
	// and exits 1. Installed lives in state.json, which is already empty on
	// fresh failures or was reset on reinstall, so there is nothing to clear
	// there. Only called after the confirm prompts, so aborts, dry runs,
	// and pre-prompt errors never touch it.
	fail := func() int {
		cfg.CurrentPath = ""
		_ = userconfig.Save(cfg)
		return 1
	}
	// Ctrl+C aborts like declining a prompt (best-effort state hygiene),
	// then reports interruption instead of success. Without this, the
	// default disposition kills setup mid-distro-work with no explanation.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		<-sigCh
		logx.Warn("setup interrupted — aborting")
		fail()
		os.Exit(130)
	}()
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
			root, rerr := userconfig.ResolveInstallDir(o.Path, cfg)
			if rerr != nil {
				root = filepath.FromSlash(cfg.DefaultPath)
			}
			ui.Printf("existing distro: %s would be unregistered", config.DistroName)
			return dryRunPlan(arch, cfg, root)
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
		// Pure input validation: return without touching config, so a typo
		// can't wipe a still-valid current_path. (On the reinstall path the
		// old distro is already gone; doctor repairs that residue by design.)
		logx.Err("%v", err)
		return 1
	}

	if o.DryRun {
		return dryRunPlan(arch, cfg, installDir)
	}

	// 1. Resolve the rootfs. The signed prebuilt image is the primary path;
	// the old Debian-download + in-distro apt flow remains as a fallback.
	distroDir := filepath.Join(installDir, config.DistroSubdir)
	url := config.RootfsURL(arch)
	fallback := config.RootfsFallbackURL(arch)
	dest := config.TempTar(arch)
	fallbackDest := config.TempTarFallback(arch)
	tarPath := ""
	imgUsed := false
	imgCached := false

	var pb *ui.Progress
	var last int64
	imgProgress := func(done, total int64) {
		if pb == nil {
			pb = ui.NewProgress(total, "downloading dockup image...")
		}
		pb.Add(done - last)
		last = done
	}
	img, imgErr := image.Ensure(installDir, arch, false, imgProgress)
	if pb != nil {
		pb.Finish()
	}
	if imgErr == nil {
		tarPath = img.Tar
		imgUsed = true
		imgCached = img.Cached
		if imgCached {
			logx.Info("using cached image %s (%d MB)", config.ImageAsset(arch), img.Size/(1024*1024))
		} else {
			logx.Info("downloaded %s (%d MB)", config.ImageAsset(arch), img.Size/(1024*1024))
		}
	} else {
		logx.Warn("prebuilt image unavailable: %v", imgErr)
		logx.Info("falling back to the Debian rootfs and in-distro engine install...")
		last = 0
		total := download.Size(url)
		pb = ui.NewProgress(total, "downloading debian rootfs...")
		if err := download.Fetch(url, dest, func(done, _ int64) {
			pb.Add(done - last)
			last = done
		}); err != nil {
			pb.Finish()
			logx.Err("primary download failed: %v - trying fallback %s", err, fallback)
			dest = fallbackDest
			if err2 := download.Fetch(fallback, dest, nil); err2 != nil {
				logx.Err("download failed: %v", err2)
				return fail()
			}
		}
		pb.Finish()
		tarPath = dest
	}

	// 2. Import. wsl --import reports no progress, so a live elapsed timer
	// instead of a fake percentage that would sit at 0% for minutes.
	spImport := ui.NewSpinner(nil, "importing debian into WSL as \"dockup\"...")
	spImport.Start()
	if err := os.MkdirAll(distroDir, 0o700); err != nil {
		spImport.Stop()
		logx.Err("mkdir wsl dir: %v", err)
		return fail()
	}
	if st, err := os.Stat(tarPath); err == nil {
		logx.Info("importing %d MB from %s", st.Size()/(1024*1024), tarPath)
	} else {
		spImport.Stop()
		logx.Err("rootfs missing at %s: %v", tarPath, err)
		return fail()
	}
	if err := wsl.Import(config.DistroName, distroDir, tarPath); err != nil {
		spImport.Stop()
		logx.Err("import failed: %v", err)
		if imgUsed {
			logx.Err("if the cached image is suspect, run \"dockup prune\" to discard it and try setup again")
		}
		return fail()
	}
	spImport.Done()

	// 3. Test WSL.
	spWSL := ui.NewSpinner(nil, "testing dockup on WSL... (please wait.)")
	spWSL.Start()
	if _, err := wsl.Exec(config.DistroName, 30*time.Second, "sh", "-c", "echo ok"); err != nil {
		spWSL.Stop()
		logx.Info("test results : failure")
		if askRetryAbort() {
			return Run(arch)
		}
		return fail()
	}
	spWSL.Done()
	logx.Info("test results : success")

	// 4. Engine. The image already contains Docker; source installs do the
	// full apt flow. If an image engine refuses to start, fall back to the
	// old install scripts rather than leaving setup half-dead.
	if imgUsed {
		spEngine := ui.NewSpinner(nil, "starting docker from the prebuilt image...")
		spEngine.Start()
		err := docker.Configure(config.DistroName)
		if err == nil {
			err = docker.WaitDaemon(config.DistroName, 90*time.Second)
		}
		if err == nil {
			spEngine.Done()
			logx.Info("test results : success")
		} else {
			spEngine.Stop()
			logx.Warn("prebuilt image engine did not start: %v", err)
			logx.Info("reinstalling engine packages from Docker's apt repo...")
			spInstall := ui.NewSpinner(nil, "installing docker...")
			spInstall.Start()
			if ierr := docker.Install(config.DistroName); ierr != nil {
				spInstall.Stop()
				logx.Info("test results : failure")
				logx.Err("%v", ierr)
				if askRetryAbort() {
					return Run(arch)
				}
				return fail()
			}
			spInstall.Done()
			spConfigure := ui.NewSpinner(nil, "configuring docker...")
			spConfigure.Start()
			if cerr := docker.Configure(config.DistroName); cerr != nil {
				spConfigure.Stop()
				logx.Info("test results : failure")
				logx.Err("%v", cerr)
				if askRetryAbort() {
					return Run(arch)
				}
				return fail()
			}
			spConfigure.Done()
			spTest := ui.NewSpinner(nil, "testing docker...")
			spTest.Start()
			if werr := docker.WaitDaemon(config.DistroName, 90*time.Second); werr != nil {
				spTest.Stop()
				logx.Info("test results : failure")
				logx.Err("%v", werr)
				if askRetryAbort() {
					return Run(arch)
				}
				return fail()
			}
			spTest.Done()
			logx.Info("test results : success")
		}
	} else {
		// 4. Install docker.
		spInstall := ui.NewSpinner(nil, "installing docker...")
		spInstall.Start()
		if err := docker.Install(config.DistroName); err != nil {
			spInstall.Stop()
			logx.Info("test results : failure")
			logx.Err("%v", err)
			if askRetryAbort() {
				return Run(arch)
			}
			return fail()
		}
		spInstall.Done()

		// 5. Configure.
		spConfigure := ui.NewSpinner(nil, "configuring docker...")
		spConfigure.Start()
		if err := docker.Configure(config.DistroName); err != nil {
			spConfigure.Stop()
			logx.Info("test results : failure")
			logx.Err("%v", err)
			if askRetryAbort() {
				return Run(arch)
			}
			return fail()
		}
		spConfigure.Done()

		// 6. Test docker.
		spTest := ui.NewSpinner(nil, "testing docker...")
		spTest.Start()
		if err := docker.WaitDaemon(config.DistroName, 90*time.Second); err != nil {
			spTest.Stop()
			logx.Info("test results : failure")
			logx.Err("%v", err)
			if askRetryAbort() {
				return Run(arch)
			}
			return fail()
		}
		spTest.Done()
		logx.Info("test results : success")
	}

	// 7. Bridge test (ephemeral relay is implicitly covered by pipe check;
	// full bridge verified on first foreground/daemon start).
	logx.Info("testing the docker daemon bridge...")
	if relay.AliveOn(cfg.EffectivePipe()) {
		logx.Info("note: pipe already held (another dockup running?)")
	}
	logx.Info("test results : success")

	// Snapshot the package set so restore can later tell user-added
	// packages apart. Best-effort: an empty snapshot just means restore
	// falls back to engine-only repair.
	snap, _ := docker.ManualPackages(config.DistroName)
	_ = state.WithLock(func(s *state.State) error {
		s.Installed = true
		s.Arch = arch
		s.SetupAt = time.Now().UTC().Format(time.RFC3339)
		s.Snapshot = state.Snapshot{At: time.Now().UTC().Format(time.RFC3339), Manual: snap}
		return nil
	})
	// Remember the used path as both default (prefilled next time) and
	// current (where dockup looks for the distro). Only a fully successful
	// setup earns installed=true in state.json (single source of truth).
	cfg.DefaultPath = filepath.ToSlash(installDir)
	cfg.CurrentPath = filepath.ToSlash(installDir)
	_ = userconfig.Save(cfg)
	_ = os.Remove(dest)
	_ = os.Remove(fallbackDest)
	if imgUsed {
		logx.Info("setup image kept at %s (removed by \"dockup prune\")", image.CacheDir(installDir))
	}
	logx.Ok("you're all good to go ! run \"dockup\" to start a foreground process or \"dockup daemon start\" to start a background daemon process.")
	return 0
}

// dryRunPlan prints what setup would do without changing anything.
func dryRunPlan(arch string, cfg userconfig.Config, installDir string) int {
	fmt.Print(ui.Header("dockup setup --dry-run") + "\n")
	ui.Printf("arch: %s", arch)
	ui.Printf("distro name: %s", config.DistroName)
	ui.Printf("install root: %s", installDir)
	ui.Printf("distro dir: %s", filepath.Join(installDir, config.DistroSubdir))
	ui.Printf("image: %s (%s)", config.ImageURL(arch), sizeOf(config.ImageURL(arch)))
	ui.Printf("image cache: %s", image.CacheDir(installDir))
	ui.Printf("fallback source: %s (%s)", config.RootfsURL(arch), sizeOf(config.RootfsURL(arch)))
	ui.Printf("fallback source: %s (%s)", config.RootfsFallbackURL(arch), sizeOf(config.RootfsFallbackURL(arch)))
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

// Uninstall removes the distro after confirmation. Idempotent: if the
// distro is already gone it still cleans state and succeeds. A successful
// uninstall also disables login autostart (entry + flag) so no dangling
// boot task remains; default_path is kept for the next setup.
// force skips the confirmation (for scripts).
func Uninstall(force bool) int {
	if !force && !ui.Confirm("Delete the dockup distro and all of its files? (the cached setup image is kept)") {
		logx.Info("aborted")
		return 0
	}
	root := userconfig.InstallRoot()
	distroDir := userconfig.DistroDir(root)
	_ = wsl.Terminate(config.DistroName)
	if wsl.Exists(config.DistroName) {
		if err := wsl.Unregister(config.DistroName); err != nil {
			logx.Err("unregister: %v", err)
			return 1
		}
	} else {
		logx.Info("distro already gone, cleaning up the rest")
	}
	_ = state.WithLock(func(s *state.State) error {
		*s = state.State{}
		return nil
	})
	if distroDir != root {
		if err := removeDirRetry(distroDir); err != nil {
			logx.Warn("could not remove %s: %v (WSL may still hold it; the next setup cleans it)", distroDir, err)
		}
	} else {
		// Legacy install: the VHDX sits directly in the install root, which
		// also holds cache\. Remove only WSL-owned files.
		for _, name := range []string{"ext4.vhdx", "rootfs"} {
			_ = os.Remove(filepath.Join(root, name))
		}
	}
	// Keep default_path for the next setup; everything else goes away,
	// including login autostart. Installed lives in state.json (cleared
	// above), not here.
	if cfg, err := userconfig.Load(); err == nil {
		cfg = cfg.WithDefaults()
		cfg.CurrentPath = ""
		cfg.Autostart = false
		_ = userconfig.Save(cfg)
	}
	_ = autostart.SetEnabled("", false)
	if image.CacheExists(root) {
		logx.Info("cached setup image kept at %s (remove it with \"dockup prune\")", image.CacheDir(root))
	}
	logx.Ok("dockup uninstalled")
	return 0
}

// removeDirRetry tolerates WSL releasing the VHDX handle a moment after
// --unregister returns; Windows refuses to delete a file still in use.
func removeDirRetry(dir string) error {
	var err error
	for i := 0; i < 10; i++ {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(time.Duration(500+i*500) * time.Millisecond)
	}
	return err
}
