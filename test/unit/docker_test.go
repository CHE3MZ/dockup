package unit

import (
	"strings"
	"testing"
	"time"

	"github.com/CHE3MZ/dockup/internal/docker"
)

func TestWaitDaemonTimeout(t *testing.T) {
	start := time.Now()
	err := docker.WaitDaemon("definitely-not-a-distro-xyz", 6*time.Second)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed < 5*time.Second {
		t.Fatalf("returned too fast (%v), not actually waiting", elapsed)
	}
}

func TestRepairScriptsResetFailed(t *testing.T) {
	scripts := map[string]string{
		"configure": docker.ConfigureScript,
		"repair":    docker.RepairScript,
	}
	for name, s := range scripts {
		if !strings.Contains(s, "reset-failed") {
			t.Fatalf("%s script lost its reset-failed guard", name)
		}
	}
}
