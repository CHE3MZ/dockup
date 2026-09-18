// Package doctor implements `dockup doctor` preflight + health checks.
package doctor

import (
	"fmt"
	"runtime"

	"dockup/internal/dockercli"
	"dockup/internal/state"
	"dockup/internal/wsl"
)

// Run prints checks, returns non-zero-worthy error if critical fails.
func Run() error {
	fmt.Printf("dockup doctor (%s/%s)\n", runtime.GOOS, runtime.GOARCH)
	fails := 0
	check := func(name string, ok bool, detail string) {
		status := "ok"
		if !ok {
			status = "FAIL"
			fails++
		}
		fmt.Printf("  [%s] %s %s\n", status, name, detail)
	}

	cliPath, err := dockercli.Path()
	check("docker CLI", err == nil, firstNonEmpty(cliPath, dockercli.InstallHint))
	check("docker compose", dockercli.ComposeCheck(), "")

	distros, err := wsl.List()
	check("wsl --list", err == nil, fmt.Sprintf("(%d distros)", len(distros)))

	s, err := state.Load()
	if err != nil {
		check("state file", false, err.Error())
	} else {
		check("state file", true, fmt.Sprintf("(default=%q active=%q)", s.Default, s.ActiveDistro))
	}
	fmt.Printf("  [info] wsl distros: %v\n", distros)

	if fails > 0 {
		return fmt.Errorf("%d check(s) failed", fails)
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
