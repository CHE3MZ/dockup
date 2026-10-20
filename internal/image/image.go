// Package image downloads, verifies, and caches prebuilt WSL rootfs images.
//
// The release assets are public: no GitHub API, token, or registry auth is
// involved. The checksum file is signed with an ed25519 key whose public half
// is embedded in the binary, so a corrupted or swapped download is rejected
// before it ever reaches wsl --import.
package image

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/download"
)

// CacheDir is <install root>/cache/images.
func CacheDir(root string) string {
	return filepath.Join(root, config.CacheSubdir, config.ImagesSubdir)
}

// Paths are the cache files for one architecture.
type Paths struct {
	Dir       string
	Tar       string
	Checksum  string
	Signature string
	Meta      string
}

// PathsFor returns the cache paths for arch under root.
func PathsFor(root, arch string) Paths {
	dir := CacheDir(root)
	tar := filepath.Join(dir, config.ImageAsset(arch))
	return Paths{
		Dir:       dir,
		Tar:       tar,
		Checksum:  tar + ".sha256",
		Signature: tar + ".sig",
		Meta:      filepath.Join(dir, "image-"+arch+".json"),
	}
}

// Meta describes a cached image. It is written last, so its presence means
// the three payload files were fully downloaded and verified.
type Meta struct {
	Tag          string `json:"tag"`
	Arch         string `json:"arch"`
	SHA256       string `json:"sha256"`
	Source       string `json:"source"`
	DownloadedAt string `json:"downloaded_at"`
}

// Result reports which image was selected.
type Result struct {
	Tar    string
	Cached bool
	Size   int64
	SHA256 string
}

// Ensure returns a verified image for arch, downloading it if needed.
func Ensure(root, arch string, refresh bool, progress download.Progress) (Result, error) {
	return EnsureWithKey(root, arch, refresh, PublicKey, progress)
}

// EnsureWithKey is Ensure with an explicit public key, for tests and key
// rotation. refresh re-checks the release even when a valid cache exists;
// it only downloads the tarball when the release checksum changed.
func EnsureWithKey(root, arch string, refresh bool, pubKey string, progress download.Progress) (Result, error) {
	if arch != "amd64" && arch != "arm64" {
		return Result{}, fmt.Errorf("unknown image architecture %q", arch)
	}
	p := PathsFor(root, arch)
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create image cache: %w", err)
	}

	cached, meta, _ := verifyCache(p, arch, pubKey)
	if cached && !refresh {
		return Result{Tar: p.Tar, Cached: true, Size: fileSize(p.Tar), SHA256: meta.SHA256}, nil
	}

	checksumTmp, sigTmp, remoteSHA, err := fetchVerifiedChecksum(p, arch, pubKey)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.Remove(checksumTmp) }()
	defer func() { _ = os.Remove(sigTmp) }()

	if cached && refresh && strings.EqualFold(remoteSHA, meta.SHA256) {
		return Result{Tar: p.Tar, Cached: true, Size: fileSize(p.Tar), SHA256: meta.SHA256}, nil
	}

	tmpTar := p.Tar + ".part"
	defer func() { _ = os.Remove(tmpTar) }()
	if err := download.Fetch(config.ImageURL(arch), tmpTar, progress); err != nil {
		return Result{}, fmt.Errorf("download image: %w", err)
	}
	if err := VerifyChecksumFile(tmpTar, checksumTmp); err != nil {
		return Result{}, err
	}
	if err := installFile(tmpTar, p.Tar); err != nil {
		return Result{}, fmt.Errorf("store image: %w", err)
	}
	if err := installFile(checksumTmp, p.Checksum); err != nil {
		return Result{}, fmt.Errorf("store checksum: %w", err)
	}
	if err := installFile(sigTmp, p.Signature); err != nil {
		return Result{}, fmt.Errorf("store signature: %w", err)
	}
	meta = Meta{
		Tag:          config.ImageTag(),
		Arch:         arch,
		SHA256:       remoteSHA,
		Source:       config.ImageURL(arch),
		DownloadedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := writeMeta(p.Meta, meta); err != nil {
		return Result{}, fmt.Errorf("store image metadata: %w", err)
	}
	return Result{Tar: p.Tar, Cached: false, Size: fileSize(p.Tar), SHA256: meta.SHA256}, nil
}

// CacheExists reports whether the image cache has any entries. It is true
// even after uninstall, so prune can clean a cache without a distro.
func CacheExists(root string) bool {
	dir := CacheDir(root)
	found := false
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Size() > 0 {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// Clear removes the image cache and returns how many bytes it held.
func Clear(root string) (int64, error) {
	dir := CacheDir(root)
	size := dirSize(dir)
	if err := os.RemoveAll(dir); err != nil {
		return 0, err
	}
	return size, nil
}

func verifyCache(p Paths, arch, pubKey string) (bool, Meta, error) {
	var meta Meta
	for _, name := range []string{p.Tar, p.Checksum, p.Signature, p.Meta} {
		if st, err := os.Stat(name); err != nil || st.IsDir() {
			return false, meta, fmt.Errorf("missing cache file %s", name)
		}
	}
	data, err := os.ReadFile(p.Meta) // #nosec G304 -- path is owned by the image cache
	if err != nil {
		return false, meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return false, meta, fmt.Errorf("parse image metadata: %w", err)
	}
	if meta.Tag != config.ImageTag() {
		return false, meta, fmt.Errorf("cache tag %q does not match %q", meta.Tag, config.ImageTag())
	}
	if meta.Arch != arch || meta.SHA256 == "" {
		return false, meta, fmt.Errorf("cache metadata does not describe dockup-%s", arch)
	}
	if err := VerifySignatureFile(p.Checksum, p.Signature, pubKey); err != nil {
		return false, meta, err
	}
	sum, err := ParseChecksumFile(p.Checksum)
	if err != nil {
		return false, meta, err
	}
	if !strings.EqualFold(sum, meta.SHA256) {
		return false, meta, fmt.Errorf("cached checksum changed")
	}
	if err := VerifyChecksumFile(p.Tar, p.Checksum); err != nil {
		return false, meta, err
	}
	return true, meta, nil
}

// fetchVerifiedChecksum downloads the release checksum and signature and
// verifies the signature before returning. The caller must remove both
// temporary files.
func fetchVerifiedChecksum(p Paths, arch, pubKey string) (string, string, string, error) {
	checksumTmp := p.Checksum + ".part"
	sigTmp := p.Signature + ".part"
	if err := download.Fetch(config.ImageChecksumURL(arch), checksumTmp, nil); err != nil {
		_ = os.Remove(checksumTmp)
		_ = os.Remove(sigTmp)
		return "", "", "", fmt.Errorf("download image checksum: %w", err)
	}
	if err := download.Fetch(config.ImageSignatureURL(arch), sigTmp, nil); err != nil {
		_ = os.Remove(checksumTmp)
		_ = os.Remove(sigTmp)
		return "", "", "", fmt.Errorf("download image signature: %w", err)
	}
	if err := VerifySignatureFile(checksumTmp, sigTmp, pubKey); err != nil {
		_ = os.Remove(checksumTmp)
		_ = os.Remove(sigTmp)
		return "", "", "", err
	}
	sum, err := ParseChecksumFile(checksumTmp)
	if err != nil {
		_ = os.Remove(checksumTmp)
		_ = os.Remove(sigTmp)
		return "", "", "", err
	}
	return checksumTmp, sigTmp, sum, nil
}

// VerifySignatureFile verifies a base64 ed25519 signature over a checksum file.
func VerifySignatureFile(checksumPath, sigPath, pubKeyB64 string) error {
	msg, err := os.ReadFile(checksumPath) // #nosec G304 -- cache path, never remote input
	if err != nil {
		return fmt.Errorf("read checksum: %w", err)
	}
	sigRaw, err := os.ReadFile(sigPath) // #nosec G304 -- cache path, never remote input
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}
	return VerifySignature(msg, sigRaw, pubKeyB64)
}

// VerifySignature verifies a base64 ed25519 signature over msg.
func VerifySignature(msg, sigRaw []byte, pubKeyB64 string) error {
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubKeyB64))
	if err != nil {
		return fmt.Errorf("image public key is not base64: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("image public key must be %d bytes, got %d", ed25519.PublicKeySize, len(pub))
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigRaw)))
	if err != nil {
		return fmt.Errorf("image signature is not base64: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("image signature must be %d bytes, got %d", ed25519.SignatureSize, len(sig))
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), msg, sig) {
		return fmt.Errorf("image signature does not match the checksum")
	}
	return nil
}

// ParseChecksumFile returns the hex sha256 from a sha256sum-style file.
func ParseChecksumFile(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- checksum path under our cache
	if err != nil {
		return "", fmt.Errorf("read checksum: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", fmt.Errorf("checksum file has no valid sha256")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("checksum is not hex: %w", err)
	}
	return strings.ToLower(fields[0]), nil
}

// VerifyChecksumFile checks that tarPath matches the signed checksum file.
func VerifyChecksumFile(tarPath, checksumPath string) error {
	want, err := ParseChecksumFile(checksumPath)
	if err != nil {
		return err
	}
	f, err := os.Open(tarPath) // #nosec G304 -- caller passes a cache/temp path
	if err != nil {
		return fmt.Errorf("open image: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash image: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("image checksum mismatch (want %s, got %s)", want, got)
	}
	return nil
}

func installFile(src, dst string) error {
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst)
}

func writeMeta(path string, meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func fileSize(path string) int64 {
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return st.Size()
	}
	return 0
}

func dirSize(dir string) int64 {
	var total int64
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Size() > 0 {
			total += info.Size()
		}
		return nil
	})
	return total
}
