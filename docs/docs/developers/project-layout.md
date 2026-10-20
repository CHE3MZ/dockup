# Project Layout

Each `internal/` package maps 1:1 to a product concept; `cmd/dockup/main.go`
is dispatch only (foreground, setup, uninstall, restore, ps, daemon,
shutdown, doctor, upgrade/update, version, help, hidden `__serve`).

| Package | Owns |
|---|---|
| `config` | Constants (`dockup` distro, pipe name, rootfs + release image URLs); `Version` var for ldflags stamping; install-root/subdir helpers; Windows path helpers |
| `state` | `%APPDATA%\dockup\state.json` (`installed/arch/setupAt/daemon/snapshot`) + file lock |
| `userconfig` | `~/.dockup/config.json` (paths, port, TCP, pipe, color, autostart) + validation, install root / `distro\` resolution, legacy VHDX detection |
| `wsl` | `wsl.exe` wrappers, UTF-16 decode, quoteless `Exec` vs stdin-script `ExecScript`, retries, import/unregister/terminate, `WslIsDistributionRegistered` fast path |
| `download` | Progress fetch with 30-min timeout, HEAD size probes |
| `image` | Signed prebuilt-image client: URL/tag resolution, ed25519 verify (embedded pubkey), SHA256 verify, `cache\images` download/reuse/refresh/clear |
| `docker` | In-distro scripts: install / configure / upgrade / repair / test / wait, engine package set |
| `setup` | Full setup flow (image download/verify/import with source fallback, + `--path`/`--dry-run`), uninstall |
| `relay` | `ServeEx` pipe (+ `docker_engine` mirror when free, + optional TCP) bridge, message-mode EOF, byte-tracked teardown, loud-500, ctx-cancelled bridges |
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
test/e2e/doc.go   # placeholder; real e2e runs in GitHub Actions
.goreleaser.yaml  # windows binaries, checksums, tag release
scripts/build.ps1 # windows build · test.ps1 · smoke.ps1 (version/doctor/ps/status, no mutations)
scripts/ops/      # go-bugcheck.sh (7 lint gates) · go-seccheck.sh (gosec)
scripts/image/    # build.sh (prebuilt WSL rootfs) · imgtool (ed25519 sign/verify)
cmd/dockup/winres/winres.json + rsrc_windows_*.syso  # exe icon (assets/icon.png)
assets/           # icon variants (exe uses appicon.png)
```
