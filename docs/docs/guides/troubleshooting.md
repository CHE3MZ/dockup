# Troubleshooting

| Symptom | Meaning / fix |
|---|---|
| `dockup has not been setup yet run "dockup setup" to set it up.` | Nothing installed — run `dockup setup` |
| `pipe is held by another program` / `pipe held by another program, not dockup` | Something else holds `dockup_engine` (a second dockup foreground/daemon, never Docker Desktop — that owns `docker_engine`) — stop it first |
| `ps` shows `starting...` | Bridge is up, engine still booting — wait; it flips to `running` via systemd self-heal |
| `dockup is not installed — autostart skipped` | Login boot with nothing installed — run `dockup setup` first |
| `stopped (stale daemon pid ... — run dockup doctor)` | Crash/reboot leftover — `dockup doctor` clears it |
| Corrupt `config.json` | Tolerated (warns, continues with defaults); delete it and any command recreates it |
| `wsl --version` fails | Update WSL (`wsl --update`); systemd needs a recent Store WSL |
| Setup download stalls | Needs internet; transient files stay in `%TEMP%` (`dockup-rootfs-*.tar.gz`) for forensics |

If the engine answers inside WSL (`wsl -d dockup -u root -- docker version`)
but not through the pipe, check `dockup daemon log` and `dockup doctor` —
and if the distro itself was modified, `dockup restore` before anything drastic.
