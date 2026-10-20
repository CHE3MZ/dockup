# Troubleshooting

| Symptom | Meaning / fix |
|---|---|
| `dockup has not been setup yet run "dockup setup" to set it up.` | Nothing installed — run `dockup setup` |
| `distro "dockup" is missing from WSL` | Removed outside dockup (e.g. `wsl --unregister`) — state was cleared automatically; run `dockup doctor`, then `dockup setup` |
| `distro "dockup" is present but not responding` | Distro broken — run `dockup doctor --fix` to repair it |
| `pipe is held by another program` / `pipe held by another program, not dockup` | Something else holds `dockup_engine` (a second dockup foreground/daemon, never Docker Desktop — that owns `docker_engine`) — stop it first |
| Bare `docker` talks to the wrong daemon | Docker Desktop holds the default pipe — stop it, or point at dockup explicitly with `-H npipe:////./pipe/dockup_engine` |
| `ps` shows `starting...` | Bridge is up, engine still booting — wait; it flips to `running` via systemd self-heal |
| `dockup is not installed — autostart skipped` | Login boot with nothing installed — run `dockup setup` first |
| `stopped (stale daemon pid ... — run dockup doctor)` | Crash/reboot leftover — `dockup doctor` clears it |
| Ctrl+C doesn't stop the foreground | Press it again — the second interrupt forces exit. Or run `dockup shutdown` from another terminal |
| Corrupt `config.json` | Tolerated (warns, continues with defaults); delete it and any command recreates it |
| `wsl --version` fails | Update WSL (`wsl --update`); systemd needs a recent Store WSL |
| `prebuilt image unavailable` warning | Setup could not download or verify the image and fell back to the source install - usually a transient network/GitHub issue |
| Setup download stalls | Needs internet; the image lands under `<root>\cache\images` (`.part` while in flight, removed by `dockup prune`) and source fallback uses `%TEMP%\dockup-rootfs-*.tar.gz` |
| Image checksum/signature mismatch | Tampered or truncated download - run `dockup prune` to drop the cache, then `dockup setup` again |
| Setup import fails from a cached image | `dockup prune` discards the cache and retries source; report the release if it keeps failing |

If the engine answers inside WSL (`wsl -d dockup -u root -- docker version`)
but not through the pipe, check `dockup daemon log` and `dockup doctor` —
and if the distro itself was modified, `dockup restore` before anything drastic.

For hands-on debugging, open a shell right inside the distro — no `wsl.exe`
flags to remember:

```powershell
dockup ssh                                            # interactive shell
dockup ssh -- journalctl -u docker.service -n 30      # one remote command
```
