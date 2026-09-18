// Package distro detects WSL distro family via /etc/os-release.
package distro

import (
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/wsl"
)

// Family is debian | alpine | unknown.
type Family string

const (
	Debian  Family = "debian"
	Alpine  Family = "alpine"
	Unknown Family = "unknown"
)

// Detect reads ID/ID_LIKE from /etc/os-release inside the distro.
func Detect(distro string) Family {
	out, err := wsl.Exec(distro, 15*time.Second, "sh", "-c", "cat /etc/os-release")
	if err != nil {
		return Unknown
	}
	text := strings.ToLower(string(out))
	if strings.Contains(text, "id=alpine") {
		return Alpine
	}
	if strings.Contains(text, "id=debian") || strings.Contains(text, "id=ubuntu") {
		return Debian
	}
	if strings.Contains(text, "debian") || strings.Contains(text, "ubuntu") {
		return Debian
	}
	return Unknown
}
