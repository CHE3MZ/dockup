# Troubleshooting

| Symptom | Meaning / fix |
|---|---|
| `dockup has not been setup yet run "dockup setup" to set it up.` | Nothing installed. Run `dockup setup` |
| `distro "dockup" is missing from WSL` | Removed outside dockup (e.g. `wsl --unregister`). State was cleared automatically. Run `dockup doctor`, then `dockup setup` |
| `distro "dockup" is present but not responding` | Distro broken. Run `dockup doctor --fix` to repair it |
| `pipe is held by another program` / `pipe held by another program, not dockup` | Something else holds `dockup_engine` (a second dockup foreground/daemon, or Docker Desktop, which owns `docker_engine`). Stop it first |
| Bare `docker` talks to the wrong daemon | Docker Desktop holds the default pipe. Stop it, or point at dockup explicitly with `-H npipe:////./pipe/dockup_engine` |
| `ps` shows `starting...` | Bridge is up, engine still booting. Wait; it flips to `running` via systemd self-heal |
| `dockup is not installed — autostart skipped` | Login boot with nothing installed. Run `dockup setup` first |
| `stopped (stale daemon pid ... — run dockup doctor)` | Crash or reboot leftover. `dockup doctor` clears it |
| Ctrl+C doesn't stop the foreground | Press it again; the second interrupt forces exit. Or run `dockup shutdown` from another terminal |
| Corrupt `config.json` | Tolerated (warns, continues with defaults); delete it and any command recreates it |
| `wsl --version` fails | Update WSL (`wsl --update`); systemd needs a recent Store WSL |
| `prebuilt image unavailable` warning | Setup could not download or verify the image and fell back to the source install. Usually a transient network or GitHub issue |
| Setup download stalls | Needs internet; the image lands under `<root>\cache\images` (`.part` while in flight, removed by `dockup prune`) and source fallback uses `%TEMP%\dockup-rootfs-*.tar.gz` |
| Image checksum/signature mismatch | Tampered or truncated download. Run `dockup prune` to drop the cache, then `dockup setup` again |
| Setup import fails from a cached image | `dockup prune` discards the cache and retries source. Report the release if it keeps failing |

If the engine answers inside WSL (`wsl -d dockup -u root -- docker version`)
but not through the pipe, check `dockup daemon log` and `dockup doctor`. If
the distro itself was modified, run `dockup restore` before anything drastic.

For hands-on debugging, open a shell right inside the distro, with no
`wsl.exe` flags to remember:

```powershell
dockup ssh                                            # interactive shell
dockup ssh -- journalctl -u docker.service -n 30      # one remote command
```
