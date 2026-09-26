# Installation

## Requirements

- Windows 10/11 with WSL2 (`wsl --install`)
- A Windows Docker CLI, e.g. `scoop install docker docker-compose`
- `dockup.exe` on your PATH (Windows amd64 or arm64)

## Getting dockup.exe

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

By default, setup asks two things: **where** to install the distro (any absolute path,
e.g. `D:/WSL` — empty input keeps the shown default, which is whatever you
used last; `--path` skips this), and confirms before touching anything:

```text
Setting up dockup :
    dockup will install and configure a new instance on WSL under the name "dockup"
    proceed ? [y/n]
```

It then downloads Debian, imports it as the `dockup` distro, installs Docker
from Docker's official apt repo, enables systemd + the engine services, and
verifies each stage (`test results : success`). If a stage fails you get
`something went wrong , retry or abort ? [retry/abort]`.

Finishing prints:

```text
you're all good to go ! run "dockup" to start a foreground process or "dockup daemon start" to start a background daemon process.
```

### Useful flags

| Flag | Effect |
|---|---|
| `--amd` / `--arm` | Distro architecture (default `--amd`) |
| `--path=DIR` | Install here without asking (becomes the new default on success) |
| `--dry-run` | Print distro name, install dir, URLs + sizes, pipe/TCP plan — changes nothing |

### Reinstalling

If a `dockup` distro already exists, setup asks:

```text
Warning : An installation of dockup already exists on WSL, do you wish to delete that installation and let dockup re-install a new dockup instance on WSL ? [y/n]
```

Answering `n` aborts cleanly with exit code 0 and changes nothing.

## Verify it works

```powershell
dockup daemon start
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
```
