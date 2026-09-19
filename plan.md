# dockup — plan

## 1. Goal
A single static Windows CLI (`dockup.exe`, Go) that gives the user full manual
control over a Docker Engine running inside the user's own existing WSL2
distro(s). No services, no scheduled tasks, no tray apps, no autostart, no
extra distros. Everything runs only when the user invokes it; after a Windows
reboot the system must be provably idle (`dockup ps` shows all stopped).

Windows-only. No Linux/macOS support. `GOOS=windows GOARCH=amd64` only.
Builds on other OSes are not supported and must fail fast in scripts/CI.

## 2. Non-goals
- No Docker Desktop / Rancher / containerd-only flows. Docker Engine only.
- No Kubernetes, no registry management, no image pruning automation.
- No Linux/macOS support. Windows 10/11 + WSL2 only.
- No GUI. No installer (single exe on PATH, e.g. scoop dir or %USERPROFILE%\tools).

## 3. Prerequisites — Windows Docker CLI (user-installed)
- `dockup` does NOT bundle or install the Windows Docker CLI.
- User installs it themselves, e.g.:
  `scoop install docker docker-compose`
  (provides `docker.exe` + `docker compose` plugin).
- `dockup` must check for Docker CLI presence before any command that needs
  to talk to the engine. Check = `where docker.exe` (or `Get-Command`) +
  `docker version` through the relay where applicable, plus
  `docker compose version` for compose users.
- Warning/error policy:
  - `setup`, `list`, `default`, bare run, `daemon start/restart/stop/status`,
    `ps`, `shutdown`, `env`: verify CLI first. Print:
    `docker CLI not found — install with: scoop install docker docker-compose`
    `setup`/`list`/`default` warn-and-continue (engine install in WSL does
    not strictly need the Windows CLI yet); run/daemon/env/ps warn-and-error
    (non-zero exit, one line) because they are useless without it.
  - Exempt: `revert`, `cleanup`, `doctor`, `version` — these must run WITHOUT
    requiring docker CLI (they repair/remove/inspect).
  - `doctor` always reports CLI status explicitly (found version or missing).

## 4. Architecture (runtime data path)
Windows `docker.exe --host npipe:////./pipe/docker_engine`
  -> Named pipe `\\.\pipe\docker_engine` served by dockup itself (go-winio)
    -> per-connection `wsl -d <distro> -u root -- socat STDIO UNIX-CONNECT:/var/run/docker.sock`
      -> `dockerd -H unix:///var/run/docker.sock` (+ `containerd`, started explicitly first)

Rules:
- SINGLE ACTIVE DISTRO AT A TIME. Only one relay+daemon set may be up.
  Starting a second distro while one is active errors with a one-liner
  (`stop/shutdown first`). No multiplexing, no per-distro pipes.
- NO TCP HOP. An earlier design proxied pipe -> 127.0.0.1:<port> -> socat
  TCP-LISTEN in WSL; it failed empirically because WSL2 localhost-forwarding
  does not reliably reach distro-bound 127.0.0.1 listeners
  (`relay not reachable` while dockerd itself was healthy). STDIO bridges are
  immune to WSL networking modes (NAT vs mirrored), need no ports/firewall,
  and expose nothing to the LAN. Accepted cost: one short-lived wsl.exe per
  API connection (~0.2-0.5s), fine for manual use.
- There is NO `--port` flag. If a `--port` is ever reintroduced, it must be
  127.0.0.1-only, dynamic by default — never `0.0.0.0` (LAN exposure).
- Guard against double-start AND foreign listeners: pipe alive + active==name
  means attach; pipe alive + anything else means HARD ERROR (never hijack
  Docker Desktop's same-named pipe — that produced a silent false pass once).
- Relay must be a raw byte-copy proxy (support HTTP hijack for
  `run -it`, `logs -f`, `exec -it`, `compose up`). No HTTP parsing.

## 5. State model
- Single JSON state file, e.g. `%APPDATA%\dockup\state.json`
  (create dir on first run; this is the ONLY thing dockup ever persists):
  ```json
  {
    "default": "<distro>",
    "activeDistro": "<distro|empty>",
    "distros": {
      "<distro>": {
        "wslWasRunning": false,
        "bootedByDockup": false,
        "installedByDockup": {"packages": [], "repos": [], "files": []},
        "hadPriorDocker": false,
        "daemon": {"pid": 0, "startedAt": "", "mode": "foreground|daemon"}
      }
    }
  }
  ```
- `activeDistro` enforces single-active rule.
- Liveness is NEVER trusted from the file. `ps` / `daemon status` verify:
  process alive? named pipe exists? `docker version` answers through the chain?
  Stale entries after reboot must report `stopped` (and may be pruned by `cleanup`).
- Concurrency: file-lock state file (Windows file lock / lockfile) for all
  read-modify-write. Two terminals double-starting must be safe.
- `dockup` must never write outside: its state dir, the target distro (setup only),
  and stdout. Never touch Windows autostart locations, services, tasks, registry,
  or machine/user env vars. `DOCKER_HOST` is communicated via `dockup env` only.

## 6. WSL lifecycle (do not kill user distros)
- Before starting any chain, probe: `wsl --list --running` (parse UTF-16).
  Record `wslWasRunning` / `bootedByDockup`.
- All in-WSL starts use `wsl.exe -d <distro> -u root -- sh -c '...'` with
  `setsid nohup ... &` + PID capture. NEVER assume default user is root.
  Timeouts on every exec. Never parse stderr for control flow.
- Teardown policy:
  - Always kill ONLY processes dockup started (tracked PIDs + signature match:
    relay port, log paths, exact command lines — never broad `killall docker`).
  - `wsl --terminate <distro>` ONLY if `bootedByDockup == true`
    (distro was stopped before dockup started it). If distro was already
    running, leave it running.
- Foreground Ctrl-C (SIGINT) and `daemon stop` / `shutdown` all follow this rule.

## 7. Distro support
- Discover via `wsl --list --quiet` (parse UTF-16 output correctly).
- `setup` must work on at least: Debian/Ubuntu (apt) and Alpine (apk).
  Do NOT assume systemd. dockerd/containerd/socat are started manually by
  dockup over `wsl -d <distro> -u root -- ...`; this works identically on systemd and
  OpenRC distros.
- Debian/Ubuntu setup: add Docker's official apt repo, install
  `docker-ce`, `docker-ce-cli` (harmless, handy for in-distro debugging),
  `containerd.io`, `socat`. Never `systemctl enable` anything.
- Alpine setup: `apk add docker containerd socat` (plus `docker-cli-compose`
  optionally). Same no-autostart rule (no rc-update).
- `setup` is idempotent: re-running on a configured distro verifies/repairs
  instead of duplicating. SNAPSHOT before install: current packages,
  repo files added, files touched. Store in `installedByDockup`.
  Detect pre-existing dockerd installs (`hadPriorDocker=true`) and ask before
  touching them.

## 8. Command spec
- `dockup setup [name]` — no name => interactive distro picker (only distros
  NOT yet configured are selectable for fresh setup; show configured ones
  marked). Requires docker-CLI check (warn-and-continue). Installs engine +
  socat per §7, snapshots installed artifacts, records distro in state. First
  configured distro becomes `default` automatically.
- `dockup revert [name]` — NO docker-CLI check required. No name => interactive
  picker among CONFIGURED distros. Mandatory `ARE YOU SURE (y/N)` confirm.
  Stops dockup-managed processes (respecting §6 terminate rule), removes ONLY
  what setup added (snapshot in `installedByDockup`: packages, repos, files).
  NEVER deletes volumes, images, the vhdx, or the distro itself
  (`wsl --unregister` is forbidden). If user had prior docker
  (`hadPriorDocker`), warn + confirm, and only remove dockup-added delta.
  Removes distro from state; if it was default, default resets to another
  configured distro or empty. If it was active, clear `activeDistro`.
- `dockup default [name]` — no name => interactive picker among configured
  distros. Sets default.
- `dockup list | ls` — table of configured distros: name, WSL version,
  in-WSL engine version, default marker, active marker. Static inventory,
  no health checks (except optional engine version probe).
- `dockup [-d|--distro <name>]` (bare run, FOREGROUND) — resolves distro
  (flag > default; error if none). Requires docker-CLI check (error if missing).
  Enforces single-active (§4). Records `wslWasRunning`. Ensures distro booted,
  starts containerd->dockerd->socat chain in WSL (as root, nohup/setsid),
  serves the named-pipe relay in the foreground with logs streaming. Ctrl-C
  (SIGINT) must gracefully tear down per §6, clear `activeDistro`, exit 0.
  If already up, attach/print status instead of double-starting.
- `dockup daemon start [-d name]` — same as bare run but detached
  (hidden/background process via `CREATE_NO_WINDOW`, logs to state-dir log file,
  PID recorded, `mode=daemon`, sets `activeDistro`). `daemon stop [-d name]`
  tears that distro's set down per §6. `daemon restart [-d name]` = stop + start.
  `daemon status [-d name]` = health: relay alive? socket answers
  `docker version`? else `stopped` + reason. One daemon globally (single-active).
- `dockup ps` — live processes managed by dockup for the active distro
  (foreground runs it can see + daemon PID verified alive), with uptime.
  Requires docker-CLI check (warn). After reboot shows `stopped`.
- `dockup shutdown` — stop the ACTIVE set (foreground ones it owns + daemon),
  clear `activeDistro`, terminate distro ONLY if booted by dockup. Idempotent.
- `dockup env [--shell powershell|cmd]` — prints the exact snippet to point
  the current shell at the active relay, e.g.
  `$env:DOCKER_HOST='npipe:////./pipe/docker_engine'`. Errors if no relay up.
- `dockup cleanup` — NO docker-CLI check required. Repair pass over all
  configured distros: prune stale PID entries, clear stale `activeDistro`,
  kill orphan socat/dockerd/relay processes (only ones matching dockup's
  signature — never broad `killall docker`), re-verify sockets, report fixed
  vs needs-attention.
- `dockup doctor` — NO docker-CLI requirement (reports it). Preflight + health:
  Windows version, WSL version, `wsl --list` parse OK?, docker CLI found?
  (`docker version`, `docker compose version`), default distro set?, active
  distro?, relay pipe alive?, socket answers?, kernel checks inside distro
  (`iptables -L`, overlayfs), port free?. Exit non-zero if anything critical
  fails, with one-line reasons.
- `dockup version` — print `dockup vX.Y.Z (windows/amd64, commit ...)`.
  No checks.
- Global `-d|--distro` applies to run/ps-relevant/daemon subcommands.
  `--help` on everything. Exit non-zero with a
  one-line reason on all failures (no stack traces to users).

## 9. Implementation notes (Go, Windows-only)
- Module `dockup`, Go >= 1.24. `GOOS=windows GOARCH=amd64` only.
  `scripts/build.ps1` must set `$env:GOOS='windows'; $env:GOARCH='amd64'`
  and `go vet`/`go build` must fail fast on other platforms
  (`//go:build windows` guards on windows-specific files).
- Deps minimal: `go-winio` (named pipe), stdlib flags (prefer stdlib if clean,
  else cobra), picker via simple numbered stdin prompt (no TUI lib).
- WSL interop: exec `wsl.exe -d <distro> -u root -- <sh -c '...'>` for simple
  quoteless one-liners ONLY. Any script containing quotes, `$()`, or
  redirections MUST go via stdin (`sh -s` + ExecScript): wsl.exe corrupts
  double quotes/`$()` passed as argv (proven: `ARCH=$(...)` arrives empty,
  which wrote a broken docker.list). Handle `wsl.exe`
  UTF-16 stdout when parsing. Timeouts on every exec. Never parse `wsl.exe`
  stderr for control flow. Use `setsid nohup` + PID capture for daemons.
- Windows process mgmt: `os/exec` + `syscall` for hidden windows
  (`CREATE_NO_WINDOW`); signal handling (`os/signal` SIGINT) for foreground
  teardown; PID tracking in state file, liveness via process handle + active
  probes (pipe dial with timeout, `docker version` through relay).
  State file locking for all mutations.
- Docker CLI check helper: `internal/dockercli/Check()` used by all commands
  except revert/cleanup/doctor/version.
- Logging: foreground streams to stdout; daemon appends to
  `%APPDATA%\dockup\logs\<distro>.log` with rotation by size (cap ~5 MB).
- No cgo. `GOOS=windows GOARCH=amd64 go build -o dockup.exe ./...`.

## 10. Repo layout (Windows-only Go CLI)
```
dockup/
  cmd/dockup/main.go            # entrypoint, signal handling, exit codes
  internal/
    cli/                        # arg parsing, --help, dispatch
    state/                      # state.json load/save/lock, activeDistro
    wsl/                        # wsl.exe wrappers, UTF-16 parse, probes
    distro/debian/              # apt setup/teardown scripts
    distro/alpine/              # apk setup/teardown scripts
    engine/                     # containerd->dockerd->socat start/stop, health
    relay/                      # go-winio npipe -> 127.0.0.1 TCP proxy
    dockercli/                  # windows docker.exe/compose presence checks
    doctor/                     # preflight checks
    picker/                     # interactive numbered prompt
    log/                        # file logging + rotation
  test/
    e2e/                        # manual/integration (needs WSL, not in CI)
    unit/                       # state, parsing, port-pick tests
  scripts/
    build.ps1                   # GOOS=windows go build -o dockup.exe
    test.ps1                    # go vet + go test ./...
    smoke.ps1                   # doctor + list + env happy-path
  .github/workflows/ci.yml      # windows-latest: vet, test, build
  plan.md
  go.mod
  README.md (later)
```
- `src/` not used — Go convention is `cmd/` + `internal/`.
- `test/` holds unit + e2e (not `src/`).
- CI runs on `windows-latest` only.

## 11. MVP phases (build+verify in order, do not skip verification)
- P1: state mgmt + locking, distro discovery (UTF-16), docker-CLI check,
  `setup` (Debian first, with snapshot), bare foreground run (single-active,
  wslWasRunning-aware teardown) + Ctrl-C, `env`, `list`, `default`,
  `-d` override, `version`, `doctor` (basic).
- P2: Alpine `setup`, `daemon start/stop/restart/status`, `ps`, `shutdown`.
- P3: `revert` (snapshot-based delta removal +confirm), `cleanup`, log rotation.
  (STDIO relay replaced the planned `--port`/dynamic-port work.)
- Each phase ends with the acceptance checks in §12 for that phase's commands.

## 12. Acceptance criteria
- Fresh machine: P1 flow takes Debian with no docker to `docker run hello-world`
  from Windows `docker.exe` (scoop-installed) with zero Windows-persistent
  changes; `autoruns` shows no new entries; reboot leaves zero dockup processes
  and `ps` = stopped.
- Missing CLI: uninstall `docker.exe`, run `dockup setup` (warns but proceeds),
  `dockup daemon start` (errors one-line), `dockup revert/cleanup/doctor`
  (run fine without CLI).
- Single-active: start distro A, starting distro B errors; `list` shows active
  marker; `shutdown` clears it.
- WSL lifecycle: with distro already running + user sleep process, Ctrl-C kills
  dockerd/socat/relay but distro stays running + sleep survives. With stopped
  distro, Ctrl-C terminates it.
- `docker run -it`, `docker logs -f`, `docker compose up` all behave natively.
- Double-start (two terminals, `dockup` twice) is safe (second attaches/reports,
  no state corruption via lock).
- Ctrl-C tears dockup processes down: no relay, no socat/dockerd leftovers in WSL.
  `shutdown` equivalent for daemon mode.
- `revert` on a configured distro removes only dockup-added packages/repos/files
  while an unrelated distro is untouched; user data (images/volumes) verified
  intact via `docker images` after re-`setup`. Prior-docker distro prompts and
  preserves delta.
- `cleanup` after `taskkill`-simulated relay crash reports and repairs state.
- `doctor` fails clearly on custom kernel without iptables/overlayfs.

## 13. Risks / open questions for the builder
- RESOLVED (was: WSL localhost-relay for a socat TCP hop): verified empirically
  that Windows-loopback dial does NOT reliably reach distro-bound 127.0.0.1
  listeners — killed the TCP design, STDIO bridges are primary. No open item.
- `docker-ce` on Debian 13 (trixie): confirm Docker's repo serves trixie;
  fallback is Ubuntu-track or static binaries (decide at build time, note it).
- dockerd needs iptables/nat kernel modules in WSL2: stock WSL2 kernel is
  fine, but if user runs a custom kernel, `setup`/`doctor` must preflight-check
  (`iptables -L`, overlayfs) and fail with a clear message.
