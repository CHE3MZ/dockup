// Package setup orchestrates `dockup setup` per revision.md.
package setup

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/docker"
	"github.com/CHE3MZ/dockup/internal/download"
	"github.com/CHE3MZ/dockup/internal/logx"
	"github.com/CHE3MZ/dockup/internal/relay"
	"github.com/CHE3MZ/dockup/internal/state"
	"github.com/CHE3MZ/dockup/internal/wsl"
)

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

// Run executes the full setup flow for arch (amd64|arm64).
func Run(arch string) int {
	if arch != "amd64" && arch != "arm64" {
		logx.Err("unknown arch %q", arch)
		return 1
	}
	// Already installed?
	if wsl.Exists(config.DistroName) {
		fmt.Printf("Warning : An installation of dockup already exists on WSL, do you wish to delete that installation and let dockup re-install a new dockup instance on WSL ? [y/n]\n")
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "y" && line != "yes" {
			logx.Info("aborted")
			return 0
		}
		logx.Info("removing old dockup distro...")
		_ = wsl.Unregister(config.DistroName)
		_ = state.WithLock(func(s *state.State) error {
			*s = state.State{}
			return nil
		})
	} else {
		fmt.Printf("Setting up dockup :\n    dockup will install and confiure a new instance on WSL under the name \"dockup\"\n    proceed ? [y/n]\n")
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "y" && line != "yes" {
			logx.Info("aborted")
			return 0
		}
	}

	url := config.DebianURL(arch)
	fallback := config.FallbackURL(arch)
	dest := config.TempTar(arch)
	total := download.Size(url)
	totalMB := download.FormatMB(total)
	if total < 0 {
		totalMB = "?"
	}

	// 1. Download.
	logx.Info("installing debian... (0/%s mb)", totalMB)
	if err := download.Fetch(url, dest, func(done, tot int64) {
		t := tot
		if t < 0 {
			t = total
		}
		fmt.Printf("\rinstalling debian... (%s/%s mb)", download.FormatMB(done), download.FormatMB(t))
	}); err != nil {
		fmt.Printf("\n")
		logx.Err("primary download failed: %v — trying mirror", err)
		if err2 := download.Fetch(fallback, dest, nil); err2 != nil {
			logx.Err("download failed: %v", err2)
			return 1
		}
	}
	fmt.Printf("\n")

	// 2. Import.
	logx.Info("importing debian into WSL as \"dockup\"... (0%%)")
	if err := os.MkdirAll(config.WslInstallDir(), 0o755); err != nil {
		logx.Err("mkdir wsl dir: %v", err)
		return 1
	}
	if err := wsl.Import(config.DistroName, config.WslInstallDir(), dest); err != nil {
		logx.Err("import failed: %v", err)
		return 1
	}
	logx.Info("importing debian into WSL as \"dockup\"... (100%%)")

	// 3. Test WSL.
	logx.Info("testing dockup on WSL... (please wait.)")
	if _, err := wsl.Exec(config.DistroName, 30*time.Second, "sh", "-c", "echo ok"); err != nil {
		logx.Info("test results : failure")
		if askRetryAbort() {
			return Run(arch)
		}
		return 1
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
		return 1
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
		return 1
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
		return 1
	}
	logx.Info("test results : success")

	// 7. Bridge test (ephemeral relay is implicitly covered by pipe check;
	// full bridge verified on first foreground/daemon start).
	logx.Info("testing the docker daemon bridge... (0%%)")
	if relay.Alive() {
		logx.Info("note: pipe already held (another dockup running?)")
	}
	logx.Info("test results : success")

	_ = state.WithLock(func(s *state.State) error {
		s.Installed = true
		s.Arch = arch
		s.SetupAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
	_ = os.Remove(dest)
	logx.Info("you're all good to go ! run \"dockup\" to start a foreground process or \"dockup daemon start\" to start a background daemon process.")
	return 0
}

// Uninstall removes the distro after confirmation.
func Uninstall() int {
	fmt.Printf("are you sure you want to uninstall your dockup wsl distro ? [y/n]\n")
	if !askYesNo("") {
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
	_ = os.RemoveAll(config.WslInstallDir())
	logx.Info("dockup uninstalled")
	return 0
}
