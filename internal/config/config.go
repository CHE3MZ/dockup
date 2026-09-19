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
	// Uses dockup_engine (not docker_engine) to avoid hijacking Docker
	// Desktop's same-named pipe (GH runners already hold docker_engine).
	// Users point the CLI via `docker -H npipe:////./pipe/dockup_engine`
	// or $env:DOCKER_HOST. Revision allows either name ("e.g.").
	PipeName = `\\.\pipe\dockup_engine`

	// RootfsBase hosts the official Debian rootfs tarballs (debuerreotype).
	// NOTE: revision.md names cloud nocloud tarballs, but those contain only
	// disk.raw and fail `wsl --import` with WSL_E_NOT_A_LINUX_DISTRO
	// (proven in GH e2e). The debuerreotype rootfs tarballs below are the
	// official Debian root filesystems (same bookworm content) and import cleanly.
	RootfsBase = "https://raw.githubusercontent.com/debuerreotype/docker-debian-artifacts"

	// Version is overridden at build time via -ldflags -X.
	Version = "dev"

	// MaxLogBytes caps the daemon log before rotation.
	MaxLogBytes = 5 << 20 // 5 MB
)

// DistBranch maps amd64/arm64 to debuerreotype branch names.
func DistBranch(arch string) string {
	if arch == "arm64" {
		return "dist-arm64v8"
	}
	return "dist-amd64"
}

// RootfsURL returns the primary WSL-importable rootfs URL (OCI tar.gz layout).
func RootfsURL(arch string) string {
	return RootfsBase + "/" + DistBranch(arch) + "/bookworm/oci/blobs/rootfs.tar.gz"
}

// RootfsFallbackURL returns the legacy top-level rootfs tarball URL.
func RootfsFallbackURL(arch string) string {
	return RootfsBase + "/" + DistBranch(arch) + "/bookworm/rootfs.tar.xz"
}

// ArchTarName is kept for display purposes.
func ArchTarName(arch string) string {
	if arch == "arm64" {
		return "rootfs-arm64v8-bookworm.tar.gz"
	}
	return "rootfs-amd64-bookworm.tar.gz"
}

// DebianURL kept for backward-compat; returns the primary rootfs URL.
func DebianURL(arch string) string { return RootfsURL(arch) }

// FallbackURL kept for backward-compat; returns the legacy rootfs URL.
func FallbackURL(arch string) string { return RootfsFallbackURL(arch) }

// NormalizeArch maps user flags to debian arch names.
// Empty means default (amd64). Returns error string empty on success.
func NormalizeArch(amdf, armf bool) (arch string, errMsg string) {
	if amdf && armf {
		return "", "pick one: --amd or --arm, not both"
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

// TempTar is the transient download path for an arch (primary .tar.gz).
func TempTar(arch string) string {
	return filepath.Join(os.TempDir(), "dockup-rootfs-"+arch+".tar.gz")
}

// TempTarFallback is the transient path for the legacy .tar.xz fallback.
func TempTarFallback(arch string) string {
	return filepath.Join(os.TempDir(), "dockup-rootfs-"+arch+".tar.xz")
}
