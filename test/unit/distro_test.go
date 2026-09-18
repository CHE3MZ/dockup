package unit

import (
	"testing"

	"github.com/CHE3MZ/dockup/internal/distro"
)

func TestParseOSRelease(t *testing.T) {
	cases := map[distro.Family][]string{
		distro.Debian: {
			"ID=debian\nVERSION_CODENAME=bookworm\n",
			"ID=ubuntu\nVERSION_CODENAME=noble\n",
			"ID=linuxmint\nID_LIKE=ubuntu debian\n",
		},
		distro.Alpine: {
			"ID=alpine\nVERSION_ID=3.20\n",
		},
		distro.Unknown: {
			"ID=fedora\n",
			"",
		},
	}
	for want, texts := range cases {
		for _, text := range texts {
			if got := distro.ParseOSRelease(text); got != want {
				t.Fatalf("ParseOSRelease(%q) = %q, want %q", text, got, want)
			}
		}
	}
}
