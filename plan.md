# dockup — plan

## 1. Goal
A single static Windows CLI (`dockup.exe`, Go) that gives the user full manual
control over a Docker Engine running inside the user's own existing WSL2
distro(s). No services, no scheduled tasks, no tray apps, no autostart, no
extra distros. Everything runs only when the user invokes it; after a Windows
reboot the system must be provably idle (`dockup ps` shows all stopped).

## 2. Non-goals
- No Docker Desktop / Rancher / containerd-only flows. Docker Engine only.
- No Kubernetes, no registry management, no image pruning automation.
- No Linux/macOS support. Windows 10/11 + WSL2 only.
- No GUI. No installer (single exe on PATH, e.g. scoop dir or %USERPROFILE%\tools).

## 3. Architecture (runtime data path)
Windows `docker.exe --host npipe:////./pipe/docker_engine`
  -> Named pipe `\\.\pipe\docker_engine` served by dockup itself (go-winio)
    -> TCP dial to 127.0.0.1:<port> (WSL localhost relay, host-local, no LAN exposure)
      -> `socat TCP-LISTEN:<port>,bind=0.0.0.0,fork,reuseaddr UNIX-CONNECT:/var/run/docker.sock` inside target distro
        -> `dockerd -H unix:///var/run/docker.sock` (+ `containerd`, started explicitly first)
Default relay port: 2375. Flag-overridable (`--port`). One relay+daemon set per
distro; guard against double-start (check pipe liveness + socket health first).

## 4. State model
- Single JSON state file, e.g. `%APPDATA%\dockup\state.json`
  (create dir on first run; this is the ONLY thing dockup ever persists):
  `{ "default": "<distro>", "distros": { "<distro>": { "relayPort": 2375, "daemon": { "pid": 0, "startedAt": "" } } } }`
- Liveness is NEVER trusted from the file. `ps` / `daemon status` verify:
  process alive? named pipe exists? `docker version` answers through the chain?
  Stale entries after reboot must report `stopped` (and may be pruned by `cleanup`).
- `dockup` must never write outside: its state dir, the target distro (setup only),
  and stdout. Never touch Windows autostart locations, services, tasks, registry,
  or machine/user env vars. `DOCKER_HOST` is communicated via `dockup env` only.

## 5. Distro support
- Discover via `wsl --list --quiet` (parse UTF-16 output correctly).
- `setup` must work on at least: Debian/Ubuntu (apt) and Alpine (apk).
  Do NOT assume systemd. dockerd/containerd/socat are started manually by
  dockup over `wsl -d <distro> -- ...`; this works identically on systemd and
  OpenRC distros.
- Debian/Ubuntu setup: add Docker's official apt repo, install
  `docker-ce`, `docker-ce-cli` (harmless, handy for in-distro debugging),
  `containerd.io`, `socat`. Never `systemctl enable` anything.
- Alpine setup: `apk add docker containerd socat` (plus `docker-cli-compose`
  optionally). Same no-autostart rule (no rc-update).
- `setup` is idempotent: re-running on a configured distro verifies/repairs
  instead of duplicating. Detect pre-existing dockerd installs and ask before
  touching them.

## 6. Command spec
- `dockup setup [name]` — no name => interactive distro picker (only distros
  NOT yet configured are selectable for fresh setup; show configured ones
  marked). Installs engine + socat per §5, records distro in state. First
  configured distro becomes `default` automatically.
- `dockup revert [name]` — no name => interactive picker among CONFIGURED
  distros. Mandatory `ARE YOU SURE (y/N)` confirm. Stops processes, removes
  what setup added (packages), NEVER deletes volumes, images, the vhdx, or the
  distro itself (`wsl --unregister` is forbidden). Removes distro from state;
  if it was default, default resets to another configured distro or empty.
- `dockup default [name]` — no name => interactive picker among configured
  distros. Sets default.
- `dockup list | ls` — table of configured distros: name, WSL version,
  in-WSL engine version, default marker. Static inventory, no health checks.
- `dockup [-d|--distro <name>]` (bare run, FOREGROUND) — resolves distro
  (flag > default; error if none). Ensures distro booted, starts
  containerd->dockerd->socat chain in WSL, serves the named-pipe relay in the
  foreground with logs streaming. Ctrl-C (SIGINT) must gracefully tear down:
  stop relay listener, kill socat/dockerd/containerd it started, terminate the
  distro (`wsl --terminate`, data persists in vhdx), exit 0. If already up,
  attach/print status instead of double-starting.
- `dockup daemon start [-d name]` — same as bare run but detached
  (hidden/background process, logs to state-dir log file, PID recorded).
  `daemon stop [-d name]` tears that distro's set down.
  `daemon restart [-d name]` = stop + start. `daemon status [-d name]` =
  per-distro health: relay alive? socket answers `docker version`? else `stopped`
  + reason. One daemon per distro max.
- `dockup ps` — live processes managed by dockup across all distros
  (foreground runs it can see + daemon PIDs verified alive), with uptime.
- `dockup shutdown` — stop ALL dockup-managed processes across all distros
  (foreground ones it owns + all daemons), terminate those distros. Idempotent.
- `dockup env [--shell powershell|cmd]` — prints the exact snippet to point
  the current shell at the active relay, e.g.
  `$env:DOCKER_HOST='npipe:////./pipe/docker_engine'`. Errors if no relay up.
- `dockup cleanup` — repair pass over all configured distros: prune stale PID
  entries, kill orphan socat/dockerd/relay processes (only ones matching
  dockup's signature: relay port, log paths, command lines — never broad
  `killall docker`), re-verify sockets, report fixed vs needs-attention.
- Global `-d|--distro` applies to run/ps-relevant/daemon subcommands.
  `--port` overrides relay port. `--help` on everything. Exit non-zero with a
  one-line reason on all failures (no stack traces to users).

## 7. Implementation notes (Go)
- Module `dockup`, Go >= 1.24. Deps minimal: `go-winio` (named pipe),
  a CLI lib (cobra or stdlib flag — prefer stdlib if it stays clean),
  picker via simple numbered stdin prompt (no TUI lib needed).
- WSL interop: exec `wsl.exe -d <distro> -- <sh -c '...'>`. Handle `wsl.exe`
  UTF-16 stdout when parsing. Timeouts on every exec. Never parse `wsl.exe`
  stderr for control flow.
- Windows process mgmt: `os/exec` + `syscall` for hidden windows
  (`CREATE_NO_WINDOW`); signal handling (`os/signal` SIGINT) for foreground
  teardown; PID tracking in state file, liveness via process handle + active
  probes (pipe dial with timeout, `docker version` through relay).
- Logging: foreground streams to stdout; daemon appends to
  `%APPDATA%\dockup\logs\<distro>.log` with rotation by size (cap ~5 MB).
- No cgo. `GOOS=windows GOARCH=amd64 go build -o dockup.exe ./...`.

## 8. MVP phases (build+verify in order, do not skip verification)
- P1: state mgmt, distro discovery, `setup` (Debian first), bare foreground
  run + Ctrl-C teardown, `env`, `list`, `default`, `-d` override.
- P2: Alpine `setup`, `daemon start/stop/restart/status`, `ps`, `shutdown`.
- P3: `revert` (+confirm), `cleanup`, `--port`, log rotation.
- Each phase ends with the acceptance checks in §9 for that phase's commands.

## 9. Acceptance criteria
- Fresh machine: P1 flow takes Debian with no docker to `docker run hello-world`
  from Windows `docker.exe` with zero Windows-persistent changes; `autoruns`
  shows no new entries; reboot leaves zero dockup processes and `ps` = stopped.
- `docker run -it`, `docker logs -f`, `docker compose up` all behave natively.
- Double-start (two terminals, `dockup` twice) is safe (second attaches/reports).
- Ctrl-C tears everything down: no relay, no socat/dockerd leftovers in WSL,
  distro terminated. `dockdown`-equivalent covered by teardown + `shutdown`.
- `revert` on a configured distro returns it to pre-setup package state while
  an unrelated distro is untouched; user data (images/volumes) verified intact
  via `docker images` after re-`setup`.
- `cleanup` after `taskkill`-simulated relay crash reports and repairs state.

## 10. Risks / open questions for the builder
- WSL localhost-relay behavior for the socat TCP hop: verify empirically that
  Windows-loopback dial reaches distro-bound 0.0.0.0:2375 on the user's box;
  fallback is `socat` VSOCK listener (more code — avoid unless needed).
- `docker-ce` on Debian 13 (trixie): confirm Docker's repo serves trixie;
  fallback is Ubuntu-track or static binaries (decide at build time, note it).
- dockerd needs iptables/nat kernel modules in WSL2: stock WSL2 kernel is
  fine, but if user runs a custom kernel, `setup` must preflight-check
  (`iptables -L`, overlayfs) and fail with a clear message.
