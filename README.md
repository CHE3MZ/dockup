<img src="assets/icon.png" alt="Dockup Logo" width="320" />

# Dockup

Docker Engine in a dedicated WSL distro, driven from Windows. No Docker
Desktop, no services, no autostart unless you ask for it: one static
`dockup.exe` installs Debian into WSL as `dockup`, runs dockerd there under
systemd, and bridges the Windows Docker CLI to it over a named pipe
(plus an optional 127.0.0.1 TCP bridge).

## Requirements

- Windows 10/11 with WSL2 (`wsl --install`)
- A Windows Docker CLI, e.g. `scoop install docker docker-compose`
- `dockup.exe` on your PATH (Windows amd64 only)

## Quickstart

```powershell
dockup setup                  # install the dockup distro (asks where + which arch)
dockup daemon start           # run in the background
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
dockup daemon stop            # stop when done
```

Or run in the foreground with `dockup` (Ctrl+C stops it).

## Commands

| Command | What it does |
|---|---|
| `dockup` | Foreground run, logs inline, Ctrl+C stops |
| `dockup setup [--amd\|--arm] [--path=DIR] [--dry-run]` | Install the distro, Docker, systemd config; verify |
| `dockup uninstall` | Remove the distro and its state (keeps your default path) |
| `dockup ps` | `STATUS` (`running`/`starting...`/`stopped`) + `AUTOSTART` table |
| `dockup daemon start\|stop\|restart\|status\|log` | Background process management + log follow |
| `dockup daemon autostart [on\|off]` | Show or set starting at Windows login (default off) |
| `dockup shutdown` | Stop everything and terminate the distro |
| `dockup doctor [--fix]` | Health checks; `--fix` reinstalls/repairs a broken engine |
| `dockup upgrade` | Update the in-distro engine to the latest versions |
| `dockup version` | Show the version |
| `dockup help [command]` | Help (`-h`/`--help` work on every command) |

## Configuration

First run creates `~/.dockup/config.json`:

```json
{
  "default_path": "C:/WSL",
  "current_path": "C:/WSL",
  "port": 2375,
  "use_tcp": false,
  "pipe_name": "\\\\.\\pipe\\dockup_engine",
  "color": true,
  "autostart": false
}
```

`default_path` is prefilled at the next setup (whatever you used last
becomes the default); `current_path` is where the live distro sits.
Point any Docker client at the bridge with
`docker -H npipe:////./pipe/dockup_engine ...` or
`$env:DOCKER_HOST = 'npipe:////./pipe/dockup_engine'`
(`tcp://127.0.0.1:2375` when `use_tcp` is on).

## How it works

Windows `docker.exe` → named pipe `\\.\pipe\dockup_engine` (raw byte
copy, so `run -it`, `logs -f`, `exec`, and Compose all behave natively)
→ per-connection `wsl -d dockup socat STDIO UNIX-CONNECT:/var/run/docker.sock`
→ `dockerd` under systemd. No TCP ports or firewall rules by default,
nothing runs unless you start it.

## Development

Host machine is compile/lint only: `go build ./...`, `go vet ./...`,
`scripts/ops/go-bugcheck.sh` (run it with Git bash). Everything
functional runs on GitHub Actions via `gh`: `ci`, `e2e-wsl`,
`full-test`, `scoop-test`, `inspect-wsl`. See `plan.md` and
`revision.md` for the architecture and build order.

The exe icon is `assets/appicon.png`, embedded via the checked-in
`cmd/dockup/rsrc_windows_*.syso` (picked up automatically by `go build`;
regenerate with `go-winres make --arch amd64,arm64` inside `cmd/dockup`).
