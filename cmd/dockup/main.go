// Command dockup is a Windows-only CLI that manages a Docker Engine
// inside the user's own WSL2 distro(s). See plan.md.
package main

import (
	"fmt"
	"os"
	"runtime"

	"dockup/internal/cli"
)

var (
	version = "v0.1.0-dev"
	commit  = "none"
)

func main() {
	if runtime.GOOS != "windows" {
		fmt.Fprintln(os.Stderr, "dockup: Windows only (GOOS=windows GOARCH=amd64)")
		os.Exit(1)
	}
	os.Exit(cli.Run(os.Args[1:], version, commit))
}
