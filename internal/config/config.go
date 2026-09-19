// Package config holds shared constants and Windows path helpers.
package config

import (
	"os"
	"path/filepath"
)

const (
	// DistroName is the only WSL distro dockup manages.
	DistroName = "dockup"
	// PipeName is the Windows named pipe docker.exe talks to.
	// Mirrors Docker Desktop so stock docker.exe works with no -H flag.
	PipeName = `\\.\pipe\docker_engine`

	// DebianBookwormBase is the primary nocloud rootfs index.
	DebianBookwormBase = "https://cloud.debian.org/images/cloud/bookworm/latest"
	// DebianFallbackBase mirrors the same files when the primary is down.
	DebianFallbackBase = "https://cdimage.debian.org/cdimage/cloud/bookworm/latest"

	// Version is overridden at build time via -ldflags -X.
	Version = "dev"

	// MaxLogBytes caps the daemon log before rotation.
	MaxLogBytes = 5 << 20 // 5 MB
)

// ArchTarName maps "amd64"/"arm64" to the nocloud tarball filename.
func ArchTarName(arch string) string {
	return "debian-12-nocloud-" + arch + ".tar.xz"
}

// DebianURL returns the primary download URL for an arch.
func DebianURL(arch string) string {
	return DebianBookwormBase + "/" + ArchTarName(arch)
}

// FallbackURL returns the mirror download URL for an arch.
func FallbackURL(arch string) string {
	return DebianFallbackBase + "/" + ArchTarName(arch)
}

// NormalizeArch maps user flags to debian arch names.
// Empty means default (amd64). Returns error string empty on success.
func NormalizeArch(amdf, armf bool) (arch string, errMsg string) {
	if amdf && armf {
		return "", "pass only one of --amd or --arm"
	}
	if armf {
		return "arm64", ""
	}
	return "amd64", ""
}

// AppDataDir is %APPDATA%\dockup (state + logs).
func AppDataDir() string {
	if v := os.Getenv("APPDATA"); v != "" {
		return filepath.Join(v, "dockup")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Roaming", "dockup")
}

// StateFile is %APPDATA%\dockup\state.json.
func StateFile() string { return filepath.Join(AppDataDir(), "state.json") }

// LockFile guards state read-modify-write.
func LockFile() string { return filepath.Join(AppDataDir(), "dockup.lock") }

// LogDir is %APPDATA%\dockup\logs.
func LogDir() string { return filepath.Join(AppDataDir(), "logs") }

// LogFile is the daemon log.
func LogFile() string { return filepath.Join(LogDir(), "dockup.log") }

// WslInstallDir is %LOCALAPPDATA%\dockup\wsl (wsl --import target).
func WslInstallDir() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return filepath.Join(v, "dockup", "wsl")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "dockup", "wsl")
}

// TempTar is the transient download path for an arch.
func TempTar(arch string) string {
	return filepath.Join(os.TempDir(), "dockup-debian-"+arch+".tar.xz")
}
