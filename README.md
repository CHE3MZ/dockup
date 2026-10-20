<img src="assets/icon.png" alt="Dockup Logo" width="256" />

# Dockup

Docker without Docker Desktop. One small exe sets up its own Debian distro
inside WSL, runs the Docker engine there, and lets your Windows Docker CLI
talk to it. Nothing runs in the background unless you say so.

Full documentation: <https://che3mz.github.io/dockup/>

## Installing

#### Via the Go package manager

```
go install github.com/CHE3MZ/dockup/cmd/dockup@latest
```

#### Manually from releases

* Download `dockup-windows-amd64.exe` (or `dockup-windows-arm64.exe`) from the [latest release](https://github.com/CHE3MZ/dockup/releases/latest).
* Check it against `dockup_checksums.txt` in the same release.
* Put it in a directory that is on your **PATH**. If you need one, create it, add it to **PATH**, and put the binary there. `~/.local/bin` and `~/.local/tools` work well.

## Getting started

### Prerequisites

* Windows 10/11 with **WSL2** installed (`wsl --install`).
* A Windows Docker CLI (`scoop install docker`).
* Dockup installed and on your PATH ([see installing dockup](#installing)).


```powershell
dockup setup                  # one-time install, asks where to put things
dockup daemon start           # run quietly in the background
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
dockup daemon stop            # stop when you're done
```

Prefer the foreground? Run `dockup` and press Ctrl+C to stop. The bridge
stops; the distro stays warm until `dockup shutdown`.

No Docker Desktop around? While dockup runs, plain `docker ...` (no `-H`)
works too. The default pipe is mirrored automatically.

## Everyday commands

- `dockup ps`: is it running?
- `dockup daemon autostart on`: start with Windows (off by default)
- `dockup doctor`: something off? start here (`--fix` rebuilds the engine)
- `dockup restore`: undo tampering, keeps your images
- `dockup ssh`: a shell inside the distro for debugging
- `dockup prune`: reclaim distro disk space
- `dockup shutdown` / `dockup uninstall`: stop everything / remove it all

### That's the gist

Guides, the configuration reference, and contributor notes are on [the website](https://che3mz.github.io/dockup/).

## License

Dockup is licensed under the [MIT License](LICENSE).

The Docker Project is also currently open source; see the [component licenses](https://www.docker.com/legal/components-licenses/).

## Contributing

Any kind of support or contribution is welcome. See the [contribution guidelines](.github/CONTRIBUTING.md).