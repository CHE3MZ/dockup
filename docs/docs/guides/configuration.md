# Configuration

First run creates `~/.dockup/config.json`:

```json
{
  "default_path": "C:/WSL",
  "current_path": "C:/WSL",
  "port": 2375,
  "use_tcp": false,
  "pipe_name": "\\\\.\\pipe\\dockup_engine",
  "color": true,
  "autostart": false
}
```

| Key | Meaning |
|---|---|
| `default_path` | Prefilled install root for the next setup (default `%LOCALAPPDATA%\dockup`); rewritten to whatever path you used on every successful setup |
| `current_path` | Install root of the live distro; holds `distro\` and `cache\images` (cleared on uninstall) |
| `port` / `use_tcp` | Optional TCP bridge (default off). When on, it listens on `127.0.0.1` only, never on the LAN |
| `pipe_name` | Named pipe to serve (default `\\.\pipe\dockup_engine`) |
| `color` | Master switch for CLI colors (also auto-off when piped, `NO_COLOR`, `TERM=dumb`, or CI) |
| `autostart` | Start at Windows login (managed via `dockup daemon autostart on\|off`) |

!!! note
    Whether dockup is installed lives in `%APPDATA%\dockup\state.json`
    (`installed`, plus `arch`, `setupAt`, daemon PID, and a package snapshot):
    the single source of truth. `config.json` holds only your intent, so it
    is safe to delete. Any dockup command recreates it with defaults.

!!! warning
    The TCP bridge has no authentication. While it is on, any local process
    that can reach `127.0.0.1` can control the daemon, which is equivalent to
    root inside the distro. Leave it off unless you need it, and never forward
    the port through a firewall or proxy.

!!! warning
    `state.json` is machine-managed (locked read-modify-write). Don't
    hand-edit it. `dockup doctor` repairs it if it ever goes stale.
