# Project Layout

Each `internal/` package maps 1:1 to a product concept; `cmd/dockup/main.go`
is dispatch only (foreground, setup, uninstall, restore, ps, daemon,
shutdown, doctor, upgrade, version, help, hidden `__serve`).

| Package | Owns |
|---|---|
| `config` | Constants (`dockup` distro, pipe name, rootfs URLs); `Version` var for ldflags stamping; Windows path helpers |
| `state` | `%APPDATA%\dockup\state.json` (`installed/arch/setupAt/daemon/snapshot`) + file lock |
| `userconfig` | `~/.dockup/config.json` (paths, port, TCP, pipe, color, autostart) + validation |
| `wsl` | `wsl.exe` wrappers, UTF-16 decode, quoteless `Exec` vs stdin-script `ExecScript`, retries, import/unregister/terminate |
| `download` | Progress fetch with 30-min timeout, HEAD size probes |
| `docker` | In-distro scripts: install / configure / upgrade / repair / test / wait, engine package set |
| `setup` | Full setup flow (+ `--path`/`--dry-run`), uninstall |
| `relay` | `ServeEx` pipe (+ optional TCP) bridge, message-mode EOF, byte-tracked teardown, loud-500 |
| `daemon` | Detached `__serve` lifecycle (PID liveness, pipe waits) |
| `doctor` | Checks + stale-state repair + `--fix` engine reinstall |
| `restore` | Lightweight delta restore (snapshot diff, engine reinstall, config rescue) + `--full` reinstall |
| `autostart` | Startup `.lnk` via `WScript.Shell` (opt-in login boot) |
| `pstable` | `ps` table render + pure `Classify` |
| `sysinfo` | Distro disk size, Windows + in-distro RSS |
| `logx` / `ui` | Logging/rotation, CLI colors (auto-off under pipe/`NO_COLOR`/`CI`) |

Supporting tree:

```text
test/unit/        # pure tests: no WSL, no network
test/e2e/doc.go   # placeholder — real e2e runs in GitHub Actions
scripts/build.ps1 # windows build · test.ps1 · smoke.ps1 (version/doctor/ps/status, no mutations)
scripts/ops/      # go-bugcheck.sh (7 lint gates) · go-seccheck.sh (gosec)
cmd/dockup/winres/winres.json + rsrc_windows_*.syso  # exe icon (assets/appicon.png)
assets/           # icon variants (exe uses appicon.png)
```
