# Prebuilt WSL Image

Status: the build, sign, smoke, and publish pipeline is green. `dockup setup`
still downloads the official Debian rootfs and installs Docker itself; the
client switch is the next step.

## Why

Installing Docker inside a fresh WSL distro at setup time is the slowest and
most fragile part of `dockup setup`: it depends on apt mirrors, DNS, and
proxies working on the user's machine. A prebuilt rootfs turns that into a
download and `wsl --import`.

## What's inside

- Debian 12 (`bookworm`) `minbase`
- `docker-ce`, `docker-ce-cli`, `containerd.io`, `docker-buildx-plugin`,
  `docker-compose-plugin`, `socat`
- `ca-certificates`, `curl`, `gnupg`, `procps`, `iproute2`, `systemd`, `systemd-sysv`
- `/etc/wsl.conf` with `[boot] systemd=true`
- `containerd.service`, `docker.service`, and `docker.socket` enabled
- `/etc/dockup-image` with the build id, arch, suite, and engine versions

Scrubbed before packing: `machine-id` (emptied so systemd regenerates it),
`/etc/hosts` and `/etc/resolv.conf` (WSL regenerates them), Docker and
containerd state, apt lists/cache, logs, shell history, SSH host keys,
`random-seed`, and the cross-build QEMU binary.

## Building

Linux or WSL2, as root:

```bash
sudo ./scripts/image/build.sh amd64 build
sudo ./scripts/image/build.sh arm64 build
```

Outputs:

```text
build/dockup-amd64.tar.gz
build/dockup-amd64.tar.gz.sha256
```

Cross-building `arm64` on an `amd64` host installs `qemu-user-static` and uses
binfmt, so the build takes noticeably longer.

The tarball normalizes timestamps to `SOURCE_DATE_EPOCH` (0 by default) and
uses `gzip -n`, and keeps numeric owners plus extended attributes. Package
versions still float with the Debian and Docker repos, so two builds are
reproducible only for the same package set.

## Signing

```bash
go run ./scripts/image/imgtool keygen
```

Store the printed `seed=` as the `DOCKUP_IMAGE_SIGNING_KEY` repository secret
and keep the `pub=` value for the client to embed. The current public key lives
in `scripts/image/pubkey.txt`. CI signs the checksum file:

```text
dockup-amd64.tar.gz.sha256  ->  dockup-amd64.tar.gz.sig
```

Verify locally:

```bash
go run ./scripts/image/imgtool verify \
  -pub "$PUB" \
  -in build/dockup-amd64.tar.gz.sha256 \
  -sig build/dockup-amd64.tar.gz.sig
```

The signature is ed25519 over the exact bytes of the `.sha256` file, written
as base64. Verify the signature first, then the tarball against the checksum.

## CI

`.github/workflows/image.yml` builds both arches on every Monday at 04:00 UTC,
on manual dispatch, and on every `v*` tag:

- `build` — rootfs, checksum, signature, artifacts
- `smoke` — imports the amd64 tarball on `windows-latest`, checks that systemd
  is PID 1, waits for dockerd, runs `hello-world`, then unregisters
- `publish` — checks each tarball against its `.sha256` and verifies both
  signatures with `scripts/image/pubkey.txt`. On a `v*` tag it waits for the
  release created by `release.yml` and attaches the image assets to it;
  otherwise it replaces the assets on the `image-latest` rolling release

Dispatch inputs: `publish` (default true) and `tag` (empty = rolling
`image-latest`; set it to attach to an existing versioned release).

Versioned releases carry the Windows binaries from GoReleaser plus the image
assets, and are the immutable reference. `image-latest` is explicitly not
marked latest, so normal release links keep pointing at the versioned release.

The arm64 artifact is built but not imported in CI: the Windows ARM runners
do not ship `wsl.exe` yet (see `arm-test.yml`).

## Rollout

1. Build, sign, and smoke the image in CI (done).
2. Teach `dockup setup` to download, verify, and import it, with the current
   from-source flow as an automatic fallback.
3. Drop runtime apt from setup; keep the install/configure scripts because
   `doctor`, `restore`, and `upgrade` still use them.

## Security

- Verify the signature before importing; never after.
- Rotate the signing seed only together with `scripts/image/pubkey.txt`; publish
  fails if a signature does not verify with the committed key.
- Keep the build script and workflow public so the artifact is reproducible.
- Rebuild on a schedule, otherwise the baked engine goes stale.
- Keep the from-source path as the escape hatch if an image is broken or a
  user does not want to trust the CI-built blob.
