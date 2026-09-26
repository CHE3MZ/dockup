<img src="assets/icon.png" alt="Dockup Logo" width="240" />

# Dockup

Docker without Docker Desktop. One small exe sets up its own Debian distro
inside WSL, runs the Docker engine there, and lets your Windows Docker CLI
talk to it. Nothing runs in the background unless you say so.

**Full documentation lives here: <https://che3mz.github.io/dockup/>**

## Try it

You need Windows 10/11 with WSL2, a Windows Docker CLI
(`scoop install docker`), and `dockup.exe` on your PATH.

```powershell
dockup setup                  # one-time install, asks where to put things
dockup daemon start           # run quietly in the background
docker -H npipe:////./pipe/dockup_engine run --rm hello-world
dockup daemon stop            # stop when you're done
```

Prefer the foreground? Just run `dockup` and Ctrl+C to stop.

## Everyday commands

- `dockup ps` — is it running?
- `dockup daemon autostart on` — start with Windows (off by default)
- `dockup doctor` — something off? start here (`--fix` rebuilds the engine)
- `dockup restore` — undo tampering, keeps your images
- `dockup shutdown` / `dockup uninstall` — stop everything / remove it all

That's the gist — guides, configuration reference, and contributor notes
are all on [the website](https://che3mz.github.io/dockup/).
