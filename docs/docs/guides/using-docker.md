# Using Docker

Any Docker client that speaks `npipe://` works: the `docker.exe` from
[docker.com](https://www.docker.com/), `scoop install docker`, VS Code's
Docker extension (set its host), and Compose.

No flags needed on most machines: while dockup runs, it also serves the
default `docker_engine` pipe, so plain `docker ps` just works. (If Docker
Desktop is running and holds that name, keep using `-H` / `DOCKER_HOST`
below.)

## Docker contexts

Setup creates a Docker CLI context named `dockup` that points at the
bridge. This is the cleanest way to select it, and it composes with
Compose and IDE Docker integrations:

```powershell
dockup context use          # make dockup Docker's current context
docker ps
dockup context off          # restore the context used before
```

`dockup context` on its own shows the context's host and which context is
active. For a single command, skip activation:

```powershell
docker --context dockup ps
```

An active context only applies when `DOCKER_HOST` is unset: the
environment variable always wins. Uninstall removes the `dockup` context
when it still points at the configured pipe.

## Connection strings

| Bridge | Address |
|---|---|
| Named pipe (always on) | `npipe:////./pipe/dockup_engine` |
| TCP (opt-in, see [Configuration](configuration.md)) | `tcp://127.0.0.1:2375` (or your custom port) |

!!! note
    The pipe is `dockup_engine`, not `docker_engine`; that name belongs to
    Docker Desktop, and dockup deliberately avoids hijacking it.

## Everyday commands

```powershell
$env:DOCKER_HOST = 'npipe:////./pipe/dockup_engine'

docker run --rm hello-world
docker run -it --rm debian:bookworm bash
docker exec -it <container> sh
docker logs -f <container>
```

## Compose

With the Compose plugin:

```powershell
docker compose up --abort-on-container-exit
docker compose build
docker compose down
```

The classic `docker-compose` binary works too. Just export `DOCKER_HOST`
first so it finds the bridge. Builds work as well: the client streams the
build context to the daemon over the same pipe.

### Bind mounts

The daemon runs on Linux inside the distro, so a Windows host path does not
exist for it:

```powershell
docker run -v C:\proj:/app ...        # fails: no such path in the distro
docker run -v ./data:/data ...        # Compose resolves this to a Windows path too
```

Use named volumes instead; they live in the distro and work everywhere:

```yaml
services:
  app:
    image: alpine
    volumes:
      - data:/var/lib/app
volumes:
  data:
```

If you need to edit bind-mounted files from Windows, keep the project inside
the distro instead and open it through `\\wsl$\dockup\...`.

## What "native" means here

The bridge is a raw byte-copy per connection straight into the in-distro
engine socket, so HTTP-hijacked streams (`run -it`, `exec`, `logs -f`,
`attach`) behave exactly like a local daemon. There is no protocol parsing
in the middle to break them, including terminal resize, which is just
another API call multiplexed over the same stream.
