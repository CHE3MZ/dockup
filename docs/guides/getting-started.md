# Getting Started

## Foreground or daemon?

| Mode | Command | Stops with |
|---|---|---|
| Foreground | `dockup` | Ctrl+C |
| Background | `dockup daemon start` | `dockup daemon stop` |

Only one may run at a time; the second starter refuses with a one-line
message telling you what is already holding the bridge.

## Your first containers

Setup creates a Docker CLI context named `dockup` that points at the
bridge. Activate it once and every Docker client uses dockup:

```powershell
dockup context use
docker version
docker run --rm hello-world
```

`dockup context off` switches Docker back to the context that was active
before. Prefer to leave your default alone? Pick the context per command,
or point at the pipe:

```powershell
docker --context dockup run --rm hello-world
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
```

The sticky environment-variable way (current shell only; it overrides any
context):

```powershell
$env:DOCKER_HOST = 'npipe:////./pipe/dockup_engine'
docker run --rm hello-world
docker run -it --rm debian:bookworm bash
```

Interactive sessions, `exec`, and `logs -f` all work natively; see
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
- `dockup ps --json` prints the same row machine-readable for scripts.

## Stopping

```powershell
dockup daemon stop   # stop the background bridge (distro stays warm for fast restart)
dockup shutdown      # stop everything, foreground included, and terminate the distro (ps => stopped)
```

Ctrl+C in a foreground stops the bridge the same way `daemon stop` does.
The distro itself stays warm in both cases; `shutdown` is the only thing
that stops it. Ctrl+C not working at all? Run `dockup shutdown` from
another terminal. It stops the stuck foreground too.

## Starting at Windows login (opt-in)

```powershell
dockup daemon autostart on    # create the Startup entry (default off)
dockup daemon autostart       # query: autostart: on|off
dockup daemon autostart off   # remove the Startup entry
```
