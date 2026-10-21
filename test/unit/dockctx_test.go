package unit

import (
	"testing"

	"github.com/CHE3MZ/dockup/internal/dockctx"
)

func TestContextHostFromPipe(t *testing.T) {
	cases := []struct{ pipe, want string }{
		{`\\.\pipe\dockup_engine`, "npipe:////./pipe/dockup_engine"},
		{`\\.\pipe\custom`, "npipe:////./pipe/custom"},
		{"npipe:////./pipe/already", "npipe:////./pipe/already"},
	}
	for _, c := range cases {
		if got := dockctx.Host(c.pipe); got != c.want {
			t.Fatalf("Host(%q) = %q, want %q", c.pipe, got, c.want)
		}
	}
}
