# Installation

## Requirements

- Windows 10/11 with WSL2 (`wsl --install`)
- A Windows Docker CLI, e.g. `scoop install docker docker-compose`
- `dockup.exe` on your PATH (Windows amd64 or arm64)

## Getting dockup.exe

### From a release

Download `dockup-windows-amd64.exe` or `dockup-windows-arm64.exe` from the
[latest release](https://github.com/CHE3MZ/dockup/releases/latest), verify it
against `dockup_checksums.txt`, rename it to `dockup.exe`, and put it on your
`PATH`.

### From source

Build from source (Go 1.24+):

```powershell
git clone https://github.com/CHE3MZ/dockup.git
cd dockup
go build -o dockup.exe ./cmd/dockup
```

## Running setup

```powershell
dockup setup
```

Setup asks where to install dockup (any absolute path, e.g. `D:/WSL/Dockup`;
empty input keeps the shown default, which is whatever you used last, and
`--path` skips the question), then confirms before touching anything:

```text
Setting up dockup:
    dockup will install and configure a new instance on WSL under the name "dockup"
    proceed? [y/n]
```

It then downloads the signed prebuilt image for your architecture from the
matching GitHub release (plain HTTPS, no GitHub account or token), verifies its
ed25519 signature and SHA256 checksum, caches it in `<install root>\cache\images`,
and imports it into `<install root>\distro`. The image already contains Debian,
Docker, systemd, and the enabled engine units, so setup only has to configure
and start them. Each stage ends with `test results : success`.

If the image cannot be fetched or verified, setup warns and falls back to the
older source path: download the official Debian rootfs and install Docker from
Docker's apt repo inside the distro. That fallback rootfs is fetched over HTTPS
only, not signature-checked; prefer the signed image and treat the fallback as
an escape hatch. If a stage fails you get
`something went wrong, retry or abort? [retry/abort]`.

The install root defaults to `%LOCALAPPDATA%\dockup`. The cached image is about
200 MB per architecture and is reused by reinstalls, `restore --full`, and
`dockup upgrade`; `dockup prune` deletes it when you want the space back.

Finishing prints:

```text
you're all good to go! run "dockup" to start a foreground process or "dockup daemon start" to start a background daemon process.
```

### Useful flags

| Flag | Effect |
|---|---|
| `--amd` / `--arm` | Distro architecture (default `--amd`) |
| `--path=DIR` | Install root (default `%LOCALAPPDATA%\dockup`; becomes the new default on success) |
| `--dry-run` | Print distro name, install root, image/cache paths, URLs + sizes, pipe/TCP plan - changes nothing |

### Folder layout

```text
<install root>\
  distro\                 # wsl --import target (ext4.vhdx, disk, etc.)
  cache\images\           # verified setup image: .tar.gz, .sha256, .sig, image-<arch>.json
```

Uninstall removes `distro\` and keeps `cache\images\`, so a later setup does not
download the image again. `dockup prune` removes the cache. Installs created by
older dockup versions kept `ext4.vhdx` directly in the install root; setup and
uninstall still recognize that layout, and the next install uses `distro\`.

### Reinstalling

If a `dockup` distro already exists, setup asks:

```text
Warning: an installation of dockup already exists on WSL. Delete it and let dockup reinstall? [y/n]
```

Answering `n` aborts cleanly with exit code 0 and changes nothing. Answering
`y` re-imports from the cached image when it still matches the running binary's
release, so reinstalling is normally fast and offline-friendly.

## Verify it works

```powershell
dockup daemon start
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
```
