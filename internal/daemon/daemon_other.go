//go:build !windows

package daemon

import (
	"errors"

	"github.com/CHE3MZ/dockup/internal/state"
)

var errWindowsOnly = errors.New("dockup is Windows-only")

func processAlive(_ int) bool { return false }

func DaemonAlive(_ state.State) bool { return false }

func Start() error { return errWindowsOnly }

func Stop() error { return errWindowsOnly }

func Status() int { return 1 }
