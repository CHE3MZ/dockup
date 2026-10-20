package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSignVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed())
	key := base64.StdEncoding.EncodeToString(pub)

	in := writeFile(t, "SHA256SUMS", "abc123  dockup-amd64.tar.gz\n")
	sig := filepath.Join(t.TempDir(), "SHA256SUMS.sig")

	if err := sign([]string{"-key", seed, "-in", in, "-out", sig}); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := verify([]string{"-pub", key, "-in", in, "-sig", sig}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	if err := os.WriteFile(in, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verify([]string{"-pub", key, "-in", in, "-sig", sig}); err == nil {
		t.Fatal("verify accepted a modified file")
	}
}

func TestPubkeyMatchesSeed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	seed := base64.StdEncoding.EncodeToString(priv.Seed())
	want := base64.StdEncoding.EncodeToString(pub)

	got, err := publicKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("pubkey = %q, want %q", got, want)
	}
}

func TestDecodeRejectsWrongSize(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("too short"))
	if _, err := decode(short, "-key", ed25519.SeedSize); err == nil {
		t.Fatal("decode accepted a short key")
	}
}

func TestVerifyRejectsBadSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	in := writeFile(t, "SHA256SUMS", "abc\n")
	sig := writeFile(t, "SHA256SUMS.sig", base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))+"\n")

	err = verify([]string{
		"-pub", base64.StdEncoding.EncodeToString(pub),
		"-in", in,
		"-sig", sig,
	})
	if err == nil {
		t.Fatal("verify accepted an invalid signature")
	}
}
