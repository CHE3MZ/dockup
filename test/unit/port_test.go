package unit

import (
	"testing"

	"github.com/CHE3MZ/dockup/internal/engine"
)

func TestPickFreePort(t *testing.T) {
	p, err := engine.PickFreePort()
	if err != nil {
		t.Fatal(err)
	}
	if p <= 0 || p > 65535 {
		t.Fatalf("bad port %d", p)
	}
}
