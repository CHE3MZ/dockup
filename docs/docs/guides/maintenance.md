# Maintenance

## Daemon lifecycle

```powershell
dockup daemon start     # refuses if already running (foreground or daemon)
dockup daemon stop
dockup daemon restart
dockup daemon status    # running (daemon pid ..., pipe ok) / stopped; exit code matches
dockup daemon log       # follow the log, read-only, Ctrl+C to exit
```

## Full shutdown and uninstall

```powershell
dockup shutdown    # stop everything (daemon + any foreground) + terminate the distro
dockup uninstall   # confirm [y/n], unregister the distro, clear state
```

Uninstall deletes the distro data under `<root>\distro` only. It keeps the
install root, the cached setup image under `<root>\cache\images`, and
`default_path`, so the next setup re-imports without downloading. It removes the
Startup entry and is idempotent - running it twice still exits 0. `--force`
skips the confirmation (same for `restore`).

## Reclaiming disk space

The distro disk only ever grows. Reclaim Docker objects without reinstalling:

```powershell
dockup prune                 # in-distro Docker prune + delete the cached image
dockup prune --all           # plus all unused images
dockup prune --volumes       # plus unused volumes (destroys their data)
dockup prune --keep-image    # Docker prune only, keep the cached image
```

`prune` deletes the cached setup image by default and works even after
`uninstall`, when only the cache is left. Every form confirms first (`--force`
skips it).

## Health and repair

```powershell
dockup doctor          # checks WSL, distro, systemd, socket, pipe, config; repairs stale PIDs/paths
dockup doctor --fix    # also reinstalls + reconfigures a broken in-distro engine
dockup upgrade         # confirm [y/n], update the engine to latest, restart services, verify
dockup update          # alias for upgrade
```

`upgrade` also refreshes the cached setup image when one exists: it fetches the
current checksum and signature and only re-downloads the tarball if the release
changed. A refresh failure is a warning, not a failed upgrade.

`doctor --fix` is engine-only repair. For tampering *around* the engine
(extra packages, broken configs), use `restore`:

```powershell
dockup restore         # confirm [y/n]: remove packages added since setup,
                       # reinstall engine set, rewrite managed configs,
                       # restart services. Images, containers, volumes survive.
dockup restore --full  # confirm [y/n]: delete + reinstall the whole distro.
                       # Guaranteed pristine; containers, images, volumes are destroyed.
                       # Re-imports the cached image; accepts --amd/--arm/--path overrides.
```

Rule of thumb: `doctor --fix` when the engine won't answer, `restore` when
the distro was fiddled with, `restore --full` when you want factory-fresh.
