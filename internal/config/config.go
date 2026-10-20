// Package config holds shared constants and Windows path helpers.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	// DistroName is the only WSL distro dockup manages.
	DistroName = "dockup"

	// ReleaseBase is the public release host for binaries and images.
	ReleaseBase = "https://github.com/CHE3MZ/dockup/releases"
	// ImageLatestTag is the rolling image release used by dev builds.
	ImageLatestTag = "image-latest"
	// DistroSubdir holds the WSL import target under the install root.
	DistroSubdir = "distro"
	// CacheSubdir holds persistent setup artifacts under the install root.
	CacheSubdir = "cache"
	// ImagesSubdir holds cached rootfs images under CacheSubdir.
	ImagesSubdir = "images"
	// PipeName is the Windows named pipe docker.exe talks to.
	// Uses dockup_engine (not docker_engine) to avoid hijacking Docker
	// Desktop's same-named pipe (GH runners already hold docker_engine).
	// Users point the CLI via `docker -H npipe:////./pipe/dockup_engine`
	// or $env:DOCKER_HOST. Revision allows either name ("e.g.").
	PipeName = `\\.\pipe\dockup_engine`

	// RootfsBase hosts the official Debian rootfs tarballs (debuerreotype).
	// NOTE: cloud nocloud tarballs contain only disk.raw and fail
	// `wsl --import` with WSL_E_NOT_A_LINUX_DISTRO (proven in GH e2e). The
	// debuerreotype rootfs tarballs below are the official Debian root
	// filesystems (same bookworm content) and import cleanly.
	RootfsBase = "https://raw.githubusercontent.com/debuerreotype/docker-debian-artifacts"

	// MaxLogBytes caps the daemon log before rotation.
	MaxLogBytes = 5 << 20 // 5 MB
)

var (
	// Version is stamped at build time via -ldflags -X (release tag). Without
	// a stamp it falls back to the module version, then the VCS revision.
	Version = "dev"
)

func init() { Version = resolveVersion(Version) }

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

// ImageTag is the image release matching this binary. Release builds and
// `go install pkg@version` use their own v* tag (images are attached to the
// same release); dev, CI, and pseudo-version builds use the rolling
// image-latest release. DOCKUP_IMAGE_TAG overrides it for tests and manual
// pinning.
func ImageTag() string {
	if v := strings.TrimSpace(os.Getenv("DOCKUP_IMAGE_TAG")); v != "" {
		return v
	}
	if v := strings.TrimSpace(Version); imageReleaseTag.MatchString(v) {
		return v
	}
	return ImageLatestTag
}

// ImageAsset is the rootfs asset name for arch.
func ImageAsset(arch string) string { return "dockup-" + arch + ".tar.gz" }

// ImageBaseURL is the directory containing this build's image assets.
// DOCKUP_IMAGE_BASE_URL overrides it for tests and mirrors.
func ImageBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("DOCKUP_IMAGE_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return ReleaseBase + "/download/" + ImageTag()
}

// ImageURL is the prebuilt rootfs tarball for arch.
func ImageURL(arch string) string { return ImageBaseURL() + "/" + ImageAsset(arch) }

// ImageChecksumURL is the sha256 file for arch's image.
func ImageChecksumURL(arch string) string { return ImageURL(arch) + ".sha256" }

// ImageSignatureURL is the ed25519 signature over the checksum file.
func ImageSignatureURL(arch string) string { return ImageURL(arch) + ".sig" }

// DefaultInstallRoot is %LOCALAPPDATA%\dockup: the install root new setups
// suggest. The root contains distro\ (WSL) and cache\ (images).
func DefaultInstallRoot() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return filepath.Join(v, "dockup")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "dockup")
}

// WslInstallDir is the legacy %LOCALAPPDATA%\dockup\wsl import target.
// It is kept as the fallback for installs that predate config.json.
func WslInstallDir() string { return filepath.Join(DefaultInstallRoot(), "wsl") }

// TempTar is the transient download path for an arch (primary .tar.gz).
func TempTar(arch string) string {
	return filepath.Join(os.TempDir(), "dockup-rootfs-"+arch+".tar.gz")
}

// TempTarFallback is the transient path for the legacy .tar.xz fallback.
func TempTarFallback(arch string) string {
	return filepath.Join(os.TempDir(), "dockup-rootfs-"+arch+".tar.xz")
}
