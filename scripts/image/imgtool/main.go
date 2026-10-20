// Command imgtool signs and verifies dockup image checksums.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen()
	case "pubkey":
		err = pubkey(os.Args[2:])
	case "sign":
		err = sign(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "help", "-h", "--help":
		usage()
		return
	default:
		_, _ = fmt.Fprintf(os.Stderr, "imgtool: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "imgtool:", err)
		os.Exit(1)
	}
}

func usage() {
	_, _ = fmt.Fprint(os.Stderr, `usage: imgtool <command> [flags]

  keygen                       print a new base64 seed and public key
  pubkey -key <seed>           print the public key for a seed
  sign   -key <seed> -in <file> -out <file>
                               write a base64 signature over <file>
  verify -pub <key> -in <file> -sig <file>
                               verify a signature over <file>
`)
}

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("%s: unexpected arguments: %s", fs.Name(), strings.Join(fs.Args(), " "))
	}
	return nil
}

func decode(s, name string, size int) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("%s is not base64: %w", name, err)
	}
	if len(b) != size {
		return nil, fmt.Errorf("%s must be %d bytes, got %d", name, size, len(b))
	}
	return b, nil
}

func keygen() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	_, _ = fmt.Printf("seed=%s\n", base64.StdEncoding.EncodeToString(priv.Seed()))
	_, _ = fmt.Printf("pub=%s\n", base64.StdEncoding.EncodeToString(pub))
	return nil
}

func publicKey(seed string) (string, error) {
	raw, err := decode(seed, "-key", ed25519.SeedSize)
	if err != nil {
		return "", err
	}
	pub := ed25519.NewKeyFromSeed(raw).Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(pub), nil
}

func pubkey(args []string) error {
	fs := newFlags("pubkey")
	key := fs.String("key", "", "base64 ed25519 seed (32 bytes)")
	if err := parse(fs, args); err != nil {
		return err
	}
	pub, err := publicKey(*key)
	if err != nil {
		return err
	}
	_, _ = fmt.Println(pub)
	return nil
}

func sign(args []string) error {
	fs := newFlags("sign")
	key := fs.String("key", "", "base64 ed25519 seed (32 bytes)")
	in := fs.String("in", "", "file to sign")
	out := fs.String("out", "", "signature output file")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *key == "" || *in == "" || *out == "" {
		return fmt.Errorf("sign needs -key, -in and -out")
	}
	seed, err := decode(*key, "-key", ed25519.SeedSize)
	if err != nil {
		return err
	}
	msg, err := os.ReadFile(*in) // #nosec G304 -- operator-supplied path
	if err != nil {
		return err
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), msg)
	body := base64.StdEncoding.EncodeToString(sig) + "\n"
	if err := os.WriteFile(*out, []byte(body), 0o644); err != nil { // #nosec G306 G703 -- signature is public, path is operator-supplied
		return err
	}
	return nil
}

func verify(args []string) error {
	fs := newFlags("verify")
	key := fs.String("pub", "", "base64 ed25519 public key (32 bytes)")
	in := fs.String("in", "", "signed file")
	sig := fs.String("sig", "", "base64 signature file")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *key == "" || *in == "" || *sig == "" {
		return fmt.Errorf("verify needs -pub, -in and -sig")
	}
	pub, err := decode(*key, "-pub", ed25519.PublicKeySize)
	if err != nil {
		return err
	}
	msg, err := os.ReadFile(*in) // #nosec G304 -- operator-supplied path
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*sig) // #nosec G304 -- operator-supplied path
	if err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return fmt.Errorf("-sig is not base64: %w", err)
	}
	if len(decoded) != ed25519.SignatureSize {
		return fmt.Errorf("-sig must be %d bytes, got %d", ed25519.SignatureSize, len(decoded))
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), msg, decoded) {
		return fmt.Errorf("signature does not match %s", *in)
	}
	return nil
}
