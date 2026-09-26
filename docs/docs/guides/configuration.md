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
| `default_path` | Prefilled install location for the next setup; rewritten to whatever path you used on every successful setup |
| `current_path` | Where the live distro sits (cleared on uninstall) |
| `port` / `use_tcp` | Optional TCP bridge (default off). When on, it listens on `127.0.0.1` only — never LAN-exposed |
| `pipe_name` | Named pipe to serve (default `\\.\pipe\dockup_engine`) |
| `color` | Master switch for CLI colors (also auto-off when piped, `NO_COLOR`, `TERM=dumb`, or CI) |
| `autostart` | Start at Windows login (managed via `dockup daemon autostart on\|off`) |

!!! note
    Whether dockup is installed lives in `%APPDATA%\dockup\state.json`
    (`installed`, plus `arch`, `setupAt`, daemon PID, and a package snapshot)
    — the single source of truth. `config.json` holds only your intent, so it
    is safe to delete: any dockup command recreates it with defaults.

!!! warning
    `state.json` is machine-managed (locked read-modify-write). Don't
    hand-edit it — `dockup doctor` repairs it if it ever goes stale.
