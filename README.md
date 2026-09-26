<img src="assets/icon.png" alt="Dockup Logo" width="256" />

# Dockup

Docker without Docker Desktop. One small exe sets up its own Debian distro
inside WSL, runs the Docker engine there, and lets your Windows Docker CLI
talk to it. Nothing runs in the background unless you say so.

**Full documentation lives here: <https://che3mz.github.io/dockup/>**

## Installing Dockup:

#### Via The `GO` package manager:
```
go install github.com/CHE3MZ/dockup/cmd/dockup@latest
dockup help
```
#### Manually from releases:

* download the binary manually from the [**latest release**](https://github.com/CHE3MZ/dockup/releases/latest).
* put it in your in a directory that is in your **PATH**, or create a new directory, add it to your **PATH** and put the binary there. *(recommended directories are ~/.local/bin or ~/.local/tools)*

## Getting Started

### Prerequisites:
* A Windows 10/11 with **WSL2** Installed on it. (`wsl --install`) 
* A Windows Docker CLI Installation (`scoop install docker`)
* Dockup Installed and on your PATH. ([**see installing dockup...**](#installing-dockup))


```powershell
dockup setup                  # one-time install, asks where to put things
dockup daemon start           # run quietly in the background
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
dockup daemon stop            # stop when you're done
```

#### *Prefer the foreground? Just run `dockup` and Ctrl+C to stop.*

## Everyday commands

- `dockup ps` — is it running?
- `dockup daemon autostart on` — start with Windows (off by default)
- `dockup doctor` — something off? start here (`--fix` rebuilds the engine)
- `dockup restore` — undo tampering, keeps your images
- `dockup shutdown` / `dockup uninstall` — stop everything / remove it all

### That's the gist — guides, configuration reference, and contributor notes are all on [**the website**](https://che3mz.github.io/dockup/).

## License

Dockup is licensed under the [**MIT License.**](LICENSE)

The Docker Project is also *(currently)* open source, [**more info here.**](https://www.docker.com/legal/components-licenses/)

## Contributing

I would highly appreciate any and all forms of support and contributions that anyone is willing to make to the project, see the [**contribution guidelines here.**](.github\CONTRIBUTING.md)