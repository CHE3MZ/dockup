# Getting Started

## Foreground or daemon?

| Mode | Command | Stops with |
|---|---|---|
| Foreground | `dockup` | Ctrl+C |
| Background | `dockup daemon start` | `dockup daemon stop` |

Only one may run at a time — the second starter refuses with a one-line
message telling you what is already holding the bridge.

## Your first containers

Point any Docker client at the bridge. The per-command way:

```powershell
docker -H npipe:////./pipe/dockup_engine version
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
```

The sticky way (current shell only):

```powershell
$env:DOCKER_HOST = 'npipe:////./pipe/dockup_engine'
docker run --rm hello-world
docker run -it --rm debian:bookworm bash
```

Interactive sessions, `exec`, and `logs -f` all work natively — see
[Using Docker](using-docker.md).

## Checking status

```powershell
dockup ps
```

```text
STATUS      AUTOSTART   INSTALLED   SIZE      MEMORY
running     off         yes         452 MB    128 MB
```

- `STATUS` is `running` (engine answering), `starting...` (bridge up, engine
  still booting), or `stopped`.
- `AUTOSTART` is `on`/`off` (Windows login autostart).
- `INSTALLED` is `yes`/`no`, `SIZE` is distro disk use, `MEMORY` its live RAM use.

## Stopping

```powershell
dockup daemon stop   # stop the background bridge (distro stays warm for fast restart)
dockup shutdown      # stop everything and terminate the distro (ps => stopped)
```

## Starting at Windows login (opt-in)

```powershell
dockup daemon autostart on    # create the Startup entry (default off)
dockup daemon autostart       # query: autostart: on|off
dockup daemon autostart off   # remove the Startup entry
```
