# Maintenance

## Daemon lifecycle

```powershell
dockup daemon start     # refuses if already running (foreground or daemon)
dockup daemon stop
dockup daemon restart
dockup daemon status    # running (daemon pid ..., pipe ok) / stopped — exit code matches
dockup daemon log       # follow the log, read-only, Ctrl+C to exit
```

## Full shutdown and uninstall

```powershell
dockup shutdown    # stop everything + terminate the distro
dockup uninstall   # confirm [y/n], unregister the distro, clear state
```

Uninstall keeps `default_path` for the next setup, removes the Startup entry,
and is idempotent — running it twice still exits 0.

## Health and repair

```powershell
dockup doctor          # checks WSL, distro, systemd, socket, pipe, config; repairs stale PIDs/paths
dockup doctor --fix    # also reinstalls + reconfigures a broken in-distro engine
dockup upgrade         # confirm [y/n], update the engine to latest, restart services, verify
```

`doctor --fix` is engine-only repair. For tampering *around* the engine
(extra packages, broken configs), use `restore`:

```powershell
dockup restore         # confirm [y/n]: remove packages added since setup,
                       # reinstall engine set, rewrite managed configs,
                       # restart services. Images, containers, volumes survive.
dockup restore --full  # confirm [y/n]: delete + reinstall the whole distro.
                       # Guaranteed pristine; containers, images, volumes are destroyed.
                       # Accepts --amd/--arm/--path overrides.
```

Rule of thumb: `doctor --fix` when the engine won't answer, `restore` when
the distro was fiddled with, `restore --full` when you want factory-fresh.
