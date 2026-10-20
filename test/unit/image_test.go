package unit

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/CHE3MZ/dockup/internal/config"
	"github.com/CHE3MZ/dockup/internal/image"
)

type releaseServer struct {
	mu       sync.Mutex
	tar      []byte
	checksum []byte
	sig      []byte
	hits     int
}

func checksumFor(data []byte) []byte {
	sum := sha256.Sum256(data)
	return []byte(hex.EncodeToString(sum[:]) + "  " + config.ImageAsset("amd64") + "\n")
}

func newReleaseServer(t *testing.T, priv ed25519.PrivateKey) (*httptest.Server, *releaseServer) {
	t.Helper()
	rs := &releaseServer{tar: []byte("prebuilt-image-payload")}
	rs.setTar(rs.tar, priv)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		defer rs.mu.Unlock()
		switch r.URL.Path {
		case "/" + config.ImageAsset("amd64") + ".sha256":
			_, _ = w.Write(rs.checksum)
		case "/" + config.ImageAsset("amd64") + ".sig":
			_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString(rs.sig) + "\n"))
		case "/" + config.ImageAsset("amd64"):
			rs.hits++
			_, _ = w.Write(rs.tar)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, rs
}

func (r *releaseServer) setTar(tar []byte, priv ed25519.PrivateKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tar = tar
	r.checksum = checksumFor(tar)
	r.sig = ed25519.Sign(priv, r.checksum)
}

func (r *releaseServer) setSignature(sig []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sig = sig
}

func (r *releaseServer) hitCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits
}

func TestImageURLs(t *testing.T) {
	t.Setenv("DOCKUP_IMAGE_TAG", "v9.9.9")
	t.Setenv("DOCKUP_IMAGE_BASE_URL", "")
	if got := config.ImageAsset("amd64"); got != "dockup-amd64.tar.gz" {
		t.Fatalf("asset = %q", got)
	}
	if got := filepath.ToSlash(image.CacheDir("Z:/root")); got != "Z:/root/cache/images" {
		t.Fatalf("cache dir = %q", got)
	}
	u := config.ImageURL("arm64")
	for _, want := range []string{"v9.9.9", "dockup-arm64.tar.gz", "/download/"} {
		if !strings.Contains(u, want) {
			t.Fatalf("image url %q missing %q", u, want)
		}
	}
	if got := config.ImageChecksumURL("amd64"); !strings.HasSuffix(got, ".sha256") {
		t.Fatalf("checksum url = %q", got)
	}
	if got := config.ImageSignatureURL("amd64"); !strings.HasSuffix(got, ".sig") {
		t.Fatalf("signature url = %q", got)
	}
}

func TestImageTag(t *testing.T) {
	old := config.Version
	defer func() { config.Version = old }()
	t.Setenv("DOCKUP_IMAGE_TAG", "")

	config.Version = "v1.2.3"
	if got := config.ImageTag(); got != "v1.2.3" {
		t.Fatalf("release tag = %q", got)
	}
	config.Version = "v1.3.0-rc1"
	if got := config.ImageTag(); got != "v1.3.0-rc1" {
		t.Fatalf("prerelease tag = %q", got)
	}
	config.Version = "v1.2.2-0.20261020155638-f53f85ea879b+dirty"
	if got := config.ImageTag(); got != config.ImageLatestTag {
		t.Fatalf("pseudo-version tag = %q", got)
	}
	config.Version = "f53f85e-dirty"
	if got := config.ImageTag(); got != config.ImageLatestTag {
		t.Fatalf("commit tag = %q", got)
	}
	config.Version = "dev"
	if got := config.ImageTag(); got != config.ImageLatestTag {
		t.Fatalf("dev tag = %q", got)
	}
	t.Setenv("DOCKUP_IMAGE_TAG", "v9.9.9")
	if got := config.ImageTag(); got != "v9.9.9" {
		t.Fatalf("override tag = %q", got)
	}
}

func TestEmbeddedPublicKeyMatchesFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "image", "pubkey.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != image.PublicKey {
		t.Fatalf("embedded key %q != scripts/image/pubkey.txt", image.PublicKey)
	}
}

func TestVerifySignatureAndChecksum(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tar := []byte("payload")
	checksum := checksumFor(tar)
	sig := ed25519.Sign(priv, checksum)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	if err := image.VerifySignature(checksum, []byte(base64.StdEncoding.EncodeToString(sig)), pubB64); err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), sig...)
	bad[0] ^= 0xff
	if err := image.VerifySignature(checksum, []byte(base64.StdEncoding.EncodeToString(bad)), pubB64); err == nil {
		t.Fatal("tampered signature should fail")
	}

	dir := t.TempDir()
	tarPath := filepath.Join(dir, "dockup-amd64.tar.gz")
	checksumPath := filepath.Join(dir, "dockup-amd64.tar.gz.sha256")
	if err := os.WriteFile(tarPath, tar, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checksumPath, checksum, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := image.VerifyChecksumFile(tarPath, checksumPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tarPath, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := image.VerifyChecksumFile(tarPath, checksumPath); err == nil {
		t.Fatal("tampered tarball should fail")
	}
}

func TestEnsureDownloadsCachesAndRefreshes(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	srv, rs := newReleaseServer(t, priv)
	t.Setenv("DOCKUP_IMAGE_BASE_URL", srv.URL)
	t.Setenv("DOCKUP_IMAGE_TAG", "test")
	root := t.TempDir()
	paths := image.PathsFor(root, "amd64")

	res, err := image.EnsureWithKey(root, "amd64", false, pubB64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached || res.Size == 0 {
		t.Fatalf("first result = %+v", res)
	}
	for _, p := range []string{paths.Tar, paths.Checksum, paths.Signature, paths.Meta} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	if !image.CacheExists(root) {
		t.Fatal("cache should exist")
	}
	if got, _ := os.ReadFile(paths.Tar); string(got) != "prebuilt-image-payload" {
		t.Fatalf("tar = %q", got)
	}

	hits := rs.hitCount()
	res, err = image.EnsureWithKey(root, "amd64", false, pubB64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cached || rs.hitCount() != hits {
		t.Fatalf("cached result = %+v, hits = %d want %d", res, rs.hitCount(), hits)
	}

	res, err = image.EnsureWithKey(root, "amd64", true, pubB64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cached || rs.hitCount() != hits {
		t.Fatalf("unchanged refresh = %+v, hits = %d want %d", res, rs.hitCount(), hits)
	}

	rs.setTar([]byte("prebuilt-image-payload-v2"), priv)
	res, err = image.EnsureWithKey(root, "amd64", true, pubB64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cached || rs.hitCount() != hits+1 {
		t.Fatalf("changed refresh = %+v, hits = %d want %d", res, rs.hitCount(), hits+1)
	}
	if got, _ := os.ReadFile(paths.Tar); string(got) != "prebuilt-image-payload-v2" {
		t.Fatalf("refreshed tar = %q", got)
	}

	if n, err := image.Clear(root); err != nil || n == 0 {
		t.Fatalf("clear = %d, %v", n, err)
	}
	if image.CacheExists(root) {
		t.Fatal("cache should be gone after Clear")
	}
}

func TestEnsureRejectsBadSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv, rs := newReleaseServer(t, priv)
	t.Setenv("DOCKUP_IMAGE_BASE_URL", srv.URL)
	t.Setenv("DOCKUP_IMAGE_TAG", "test")
	rs.setSignature([]byte("not a signature"))
	root := t.TempDir()
	if _, err := image.EnsureWithKey(root, "amd64", false, base64.StdEncoding.EncodeToString(pub), nil); err == nil {
		t.Fatal("bad signature should fail")
	}
	if image.CacheExists(root) {
		t.Fatal("failed download must not leave a cache entry")
	}
}

func TestEnsureRejectsBadChecksum(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	srv, rs := newReleaseServer(t, priv)
	t.Setenv("DOCKUP_IMAGE_BASE_URL", srv.URL)
	t.Setenv("DOCKUP_IMAGE_TAG", "test")
	// Sign a checksum for different bytes so the signature is valid but the
	// tarball does not match.
	rs.mu.Lock()
	rs.checksum = checksumFor([]byte("different"))
	rs.sig = ed25519.Sign(priv, rs.checksum)
	rs.mu.Unlock()
	root := t.TempDir()
	if _, err := image.EnsureWithKey(root, "amd64", false, base64.StdEncoding.EncodeToString(pub), nil); err == nil {
		t.Fatal("bad checksum should fail")
	}
	if image.CacheExists(root) {
		t.Fatal("failed download must not leave a cache entry")
	}
}
