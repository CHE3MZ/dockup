package unit

import (
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
