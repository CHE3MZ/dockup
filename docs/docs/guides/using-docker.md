# Using Docker

Any Docker client that speaks `npipe://` works: the `docker.exe` from
[docker.com](https://www.docker.com/), `scoop install docker`, VS Code's
Docker extension (set its host), and Compose.

## Connection strings

| Bridge | Address |
|---|---|
| Named pipe (always on) | `npipe:////./pipe/dockup_engine` |
| TCP (opt-in, see [Configuration](configuration.md)) | `tcp://127.0.0.1:2375` (or your custom port) |

!!! note
    The pipe is `dockup_engine`, not `docker_engine` — that name belongs to
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
docker compose down
```

The classic `docker-compose` binary works too — just export `DOCKER_HOST`
first so it finds the bridge.

## What "native" means here

The bridge is a raw byte-copy per connection straight into the in-distro
engine socket, so HTTP-hijacked streams (`run -it`, `exec`, `logs -f`,
`attach`) behave exactly like a local daemon. There is no protocol parsing
in the middle to break them — including terminal resize, which is just
another API call multiplexed over the same stream.
