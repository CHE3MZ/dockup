# Handoff: dockup — Windows-only Docker Engine in a dedicated WSL distro
<!-- Last updated: 2026-09-20 -->

## Summary
`dockup.exe` (Go, Windows amd64 + arm64 builds) installs Debian into WSL as a dedicated `dockup` distro, runs dockerd under systemd, and bridges the Windows Docker CLI to it via named pipe `\\.\pipe\dockup_engine` (plus optional 127.0.0.1 TCP). Tree is clean, all 7 GH workflows green, all 8 local lint gates green. Nothing is in flight — wait for the user's next task.

## Objective
No active task. Next session: wait for the user to assign work. Deferred-by-user list lives in `TODO.txt` history (file since deleted by user): `dockup restore` follow-ups are DONE; still deferred are web UI (Vue 3/Vite/Vapor, localhost:8060), full docs site, exe icon from assets, CD auto-tag/release.

## Standing operating rules (do not violate)
- **NEVER modify the user's system.** Never run `dockup setup/run/daemon`, never touch WSL, never download distro payloads, never install software on the host. Host is ONLY for: `go build`, `go vet`, `gofmt`, `scripts/ops/go-bugcheck.sh`, `scripts/ops/go-seccheck.sh`, `go test` (unit only, part of bugcheck).
- **All functional testing via GitHub Actions with the `gh` CLI.** Commit with a short message + push BEFORE every test run. Never ask the user for confirmation; implement step by step.
- Every push triggers all 7 workflows (~10 min wall). Always read full logs (`gh run view <ID> --log`), never trust green/red blindly — several "failures" were test-harness bugs, not product bugs.
- `logs/` is gitignored; `dockup.exe` in repo root is gitignored.
- The user edits files directly in parallel (help text, TODO.txt). `git add -A` will sweep their uncommitted edits into your commit — always run `git status`/`git diff` first and report what rode along.

## Repo map
- `cmd/dockup/main.go` — dispatch only: foreground, setup, uninstall, restore, ps, daemon (start/stop/restart/status/log/autostart), shutdown, doctor [--fix], upgrade, version, help, hidden `__serve`.
- `internal/`: `config` (constants; `Version` is a **var** for ldflags stamping), `state` (%APPDATA% state.json: installed/arch/setupAt/daemon/snapshot), `userconfig` (~/.dockup/config.json: default_path/current_path/port/use_tcp/pipe_name/color/autostart/installed + `InstallDir()` + `ReconcileInstalled`), `wsl` (wsl.exe wrappers, UTF-16 decode, stdin-script pattern), `download` (progress fetch, 30-min timeout), `docker` (apt scripts, Install/Configure/Upgrade/Repair/TestDaemon/WaitDaemon/ManualPackages), `setup` (full flow + --path/--dry-run), `relay` (ServeEx pipe+TCP, message-mode, byte-tracked teardown, loud-500), `daemon` (detached `__serve`, PID liveness), `doctor` (checks + RunEx fix), `autostart` (Startup .lnk via WScript.Shell), `logx`/`ui` (colors), `pstable` (ps table), `sysinfo` (vhdx size, RSS), `restore` (RemovalList + light/full).
- `test/unit/` (pure tests), `test/e2e/doc.go` (remote only), `scripts/ops/go-bugcheck.sh` (7 checks), `scripts/ops/go-seccheck.sh` (gosec), `plan.md` (architecture + build log §§10.x), `revision.md` (behavior source of truth), `README.md` (user doc).

## Architecture (one paragraph)
Windows `docker.exe` → `\\.\pipe\dockup_engine` in **message mode** (so client stdin CloseWrite arrives as EOF) → per-connection `wsl -d dockup socat STDIO UNIX-CONNECT:/var/run/docker.sock` → dockerd under systemd. Raw byte-copy (HTTP hijack works: `-it`, `logs -f`, `exec`, compose). Teardown: stdin-EOF first → close socat stdin + 60s grace for flush/exit; daemon-side first → reap immediately; daemon bytes tracked — backend death with zero bytes serves HTTP 500 + daemon-log entry instead of silent EOF. No TCP ports/firewall by default; TCP (`use_tcp`) is 127.0.0.1-only.

## CI map (all on push + dispatch)
- `ci`: vet, `go test -race`, build (version-stamped), version smoke.
- `e2e-wsl` smoke: wsl version/list, build, version/doctor/ps, exit-code asserts (no `continue-on-error` — assert explicitly + `exit 0`).
- `full-test` (~7 min): vet, unit, build, help matrix, dry-run, config checks, custom-path setup, reinstall-over-existing, doctor, `--fix`-healthy, daemon lifecycle incl. refusal cases, autostart on/off/login-simulation, pipe version/hello/compose/stdin/tty/exec/logs-f, definitive `cat` EOF test, TCP custom port, upgrade, restore (break→repair→data-preserved), stop, foreground serve, shutdown, uninstall (abort + real, default retained).
- `scoop-test`: scoop docker + docker-compose shims, bridge via `DOCKER_HOST`.
- `inspect-wsl`: deep distro audit, provenance artifact, upgrade, break→`doctor --fix`, `--full` wipe+health, legacy recovery, A/B reinstall-compare (must match exactly).
- `multi-distro`: two plain neighbor distros — setup/daemon/shutdown scoping, distro switching, shutdown→`starting...`→self-heal, uninstall removes only dockup.
- `arm-test`: `windows-11-arm` has NO WSL (verified on both arm images) — `arm-build` always proves arm64 vet/unit/build; `arm-install` auto-skips until Microsoft ships WSL (gated on a WSL probe output).

## Runbook
```powershell
git add -A; git commit -m "<short>"; git push
gh run list --limit 7
gh run view <ID> --log   # full text; distinguish script ECHO lines (^[[36;1m) from real output
gh workflow run <name> [-f full-setup=true]  # e2e full-setup is dispatch-gated
```

## PowerShell-in-CI pitfalls (learned the hard way)
- `$LASTEXITCODE` persists: any step tolerating a nonzero native exit MUST end `exit 0`.
- `-match` on arrays returns elements (truthy!): always `(cmd) -join "`n"` first.
- `wsl.exe` emits raw UTF-16LE intermittently: strip NULs (``-replace "`0",""``) before matching; Go side already decodes.
- `$o = ... 2>&1` merges stderr reliably (proven); for decisive forensics prefer separate `> file 2> file` + dump both.
- Single-element arrays, `echo x |` pipeline stdin closing, `wsl -- <quoted args>` mangling (pass quoteless one-liners; scripts via stdin in Go) — keep patterns as in existing workflows.
- `Start-Job` stdin piping is unreliable for probes; prefer main-shell commands with step timeouts.
- Minbase rootfs has NO `pgrep`/`ps` (only after docker install pulls procps): use PID files, `tasklist`, or coreutils builtins in neighbor distros.

## Tool-use lessons
- `edit` needs byte-exact `oldString` (read the region first); batched parallel edits can report success/failure crossed — verify with grep/read after.
- Multi-line heredocs through the shell tool get swallowed — use `edit` for file appends.
- Go vet/build catch x/sys API mistakes fast (`GetProcessImageFileName`, `PROCESS_MEMORY_COUNTERS` don't exist; `QueryFullProcessImageName` does).
- gosec G115 accepts guarded-variable conversions but flags direct-call conversions; `nosec` needs per-line justification comments.
- `go test` cache can mislead after edits — use `-count=1` when it matters.

## Frozen strings (never reword)
Revision-mandated setup flow verbatim (incl. `confiure`→`configure` fix applied to code+revision together); `dockup has not been setup yet run "dockup setup" to set it up.`; all CI-grepped tokens (`autostart: on/off`, `STATUS/AUTOSTART/INSTALLED/SIZE/MEMORY`, `starting...`, `test results :`, `read-only`, `ok: autostart on`, `dry run complete`, version sentence).

## Decisions (rationale compressed)
Debuereotype rootfs over nocloud tarballs (nocloud = disk.raw, fails import); `dockup_engine` pipe (Desktop holds `docker_engine`); no TCP hop (WSL NAT loopback unreliable); stdio-bridge per connection; message-mode + CloseWrite for stdin EOF; byte-tracked teardown + loud 500; `reset-failed` before every unit restart (start-limit throttling); `WaitDaemon` after every restart (boot race); engine set ALWAYS reinstalled on restore; `apt-get clean` (VHDX only grows); `installed` flag with safe transitions (set true only on success, cleared only post-prompt-fail/uninstall/confirmed-absent; reconcile adopts but never clears); uninstall removes Startup entry + autostart, keeps `default_path`, idempotent; `doctor --fix` ≠ `restore` (engine-only vs delta+data-preserved) ≠ `--full` (wipe).

## Open issues / watch items
- `wsl.exe` UTF-16 transient family (garbage bytes / slow-empty spawn / NUL-garbled list): rare, always transient, mitigated (retries, NUL-strip, loud-500, forensics). No action unless it recurs with the new daemon-log evidence.
- Untestable in CI: graceful-SIGINT teardown (kill path tested), `--arm` install (build gate only), manual provenance diffing.
- Failed-setup temp tarballs stay in TEMP by design (debugging).
- Node.js 20 deprecation warnings come from checkout/setup-go themselves.

## References
- Behavior: `revision.md` — Architecture/log: `plan.md` (§§10.x) — User doc: `README.md`
- Lint: `scripts/ops/go-bugcheck.sh`, `scripts/ops/go-seccheck.sh` (run with Git bash; busybox `sh` skips the gopls step)
- Key files: `internal/relay/relay.go`, `internal/setup/setup.go`, `internal/docker/docker.go`, `internal/restore/restore.go`, `internal/state/state.go`, `internal/userconfig/userconfig.go`, `cmd/dockup/main.go`
