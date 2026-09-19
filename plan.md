# dockup — Implementation Plan (based on `revision.md`)

> Source of truth for behavior: `revision.md` (118 lines, `github.com/CHE3MZ/dockup`).
> This file is the build order. Implement top-to-bottom. Do not skip verification.
> Host-machine rule (from `revision.md`): host is ONLY for (1) compile check
> (`go build` / `go vet`) and (2) `scripts/ops/go-bugcheck.sh`. NEVER run
> `dockup setup/run/daemon` locally, never touch WSL locally, never download
> the Debian image locally. ALL functional testing goes through GitHub Actions
> via `gh` CLI, and every test run must be preceded by `git commit` + `git push`.

## 0. Ground rules (non-negotiable)

1. **Windows-only.** `GOOS=windows GOARCH=amd64` (+ `arm64` download variant for
   the *WSL distro payload only* — the Windows binary itself stays `amd64`;
   see §1.1). All Windows-specific files get `//go:build windows`.
2. **No host mutation.** The implementation must never, when run on the dev
   host, be executed for manual verification. Only `go build ./...`,
   `go vet ./...`, and `scripts/ops/go-bugcheck.sh` may run on the host.
3. **gh-first testing.** Workflows live in `.github/workflows/`. Run with
   `gh workflow run <NAME>`, list with `gh run list`, inspect with
   `gh run view <ID>` and `gh run view <ID> --log`. Commit + push BEFORE each
   remote run. Short commit messages.
4. **Single dedicated distro.** The WSL distro is ALWAYS named `dockup`.
   No multi-distro picker, no `--distro` flag (old plan had it — revision
   removes it). The only arch flags are `dockup setup [--amd|--arm]`.
5. **Single-active process.** At most one of {foreground `dockup`, daemon}
   may run. The second starter must refuse with a one-line message.
6. **Helper lifecycle is slaved.** The named-pipe→WSL bridge (“helper”/relay)
   only runs while `dockup` (foreground or daemon) runs. If the helper fails,
   the parent must exit/error with code + reason. No orphan pipes.
7. **Stdlib-first, tiny deps.** Only `github.com/Microsoft/go-winio` for named
   pipes. No cobra/bubbletea. Plain `os.Args` parsing + numbered `y/n` prompts
   on stdin.

---

## 1. Research findings (what we verified before designing)

### 1.1 Debian rootfs — exact URLs (CORRECTED after GH e2e proof)

> Correction (2026-09-19, proven by GH `e2e-wsl` run 35456171649):
> `https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-nocloud-*.tar.xz`
> is NOT WSL-importable — it contains only a single `disk.raw` partitioned
> image, so `wsl --import` fails with
> `Wsl/Service/RegisterDistro/WSL_E_NOT_A_LINUX_DISTRO`.
> The implementation therefore uses the official Debian rootfs tarballs
> (same bookworm content, WSL-compatible) from debuerreotype:
>
> ```text
> amd64 primary:  https://raw.githubusercontent.com/debuerreotype/docker-debian-artifacts/dist-amd64/bookworm/oci/blobs/rootfs.tar.gz
> amd64 fallback: https://raw.githubusercontent.com/debuerreotype/docker-debian-artifacts/dist-amd64/bookworm/rootfs.tar.xz
> arm64 primary:  https://raw.githubusercontent.com/debuerreotype/docker-debian-artifacts/dist-arm64v8/bookworm/oci/blobs/rootfs.tar.gz
> arm64 fallback: https://raw.githubusercontent.com/debuerreotype/docker-debian-artifacts/dist-arm64v8/bookworm/rootfs.tar.xz
> ```
>
> Branch mapping: `amd64 → dist-amd64`, `arm64 → dist-arm64v8`.
> This keeps revision.md's UX (default amd64, `--amd`/`--arm`, `DOWNLOADSIZE_INMB`
> banner, `%TEMP%` transient file) while making setup actually work.
> Original nocloud URLs are documented here for reference only:
> `https://cloud.debian.org/images/cloud/bookworm/latest/` (~252 MB amd64 /
> ~238 MB arm64 `.tar.xz`) + `cdimage.debian.org` mirror.

### 1.2 `wsl.exe --import` contract

```powershell
wsl.exe --import <Distro> <InstallLocation> <TarFile> [--version 2]
```

- `<Distro>` = `dockup` (hardcoded constant).
- `<InstallLocation>` = `%LOCALAPPDATA%\dockup\wsl` (e.g.
  `C:\Users\<u>\AppData\Local\dockup\wsl`). Contains `ext4.vhdx` after import.
  Must `mkdir -p` first.
- Always pass `--version 2` (WSL2 required for systemd + docker).
- Import runs as `root` by default; no default-user setup needed (docker-only
  distro, always `wsl -d dockup -u root`).
- Other commands used:
  - `wsl --list --quiet` (UTF-16LE decode!) → existence check.
  - `wsl --list --running --quiet` → liveness.
  - `wsl -d dockup -u root -- <cmd>` → all in-distro work.
  - `wsl -d dockup -u root -- sh -s` (stdin script) for ANYTHING with quotes,
    `$()`, redirects — `wsl.exe` argv corrupts `"`/`$()` (proven in old
    codebase: `ARCH=$(...)` arrived empty via argv, worked via stdin).
  - `wsl --terminate dockup` → stop distro (only on uninstall/shutdown where
    appropriate; the dockup distro is disposable so terminate is safe, unlike
    the old “never kill user distro” rule).
  - `wsl --unregister dockup` → delete distro (only on `uninstall` + `setup`
    reinstall-after-confirm).
  - `wsl --shutdown` → NEVER call globally (kills user distros). Only
    `--terminate dockup`.
- `wsl.exe` stdout is UTF-16LE with BOM. Must decode before parsing
  (reuse proven `decodeWslOutput` pattern from pre-refactor code).

### 1.3 systemd in WSL

- Requires WSL Store version ≥0.67.6 (`wsl --version` works). Enable via
  `/etc/wsl.conf`:
  ```ini
  [boot]
  systemd=true
  ```
- After writing `wsl.conf`, must `wsl --terminate dockup` once, then next
  `wsl -d dockup` boots under systemd (`ps -p 1 -o comm=` → `systemd`,
  `systemctl is-system-running`).
- `doctor` must check `wsl --version` exists, else print “update WSL:
  `wsl --update`”.
- Because this is a docker-only distro, `systemctl enable --now docker.service
  containerd.service` is CORRECT (revision explicitly says so; old plan forbade
  autostart because it used user distros — that constraint is gone).

### 1.4 Docker on Debian bookworm (official repo, not `docker.io`)

Debian’s `docker.io` is stale — must use `download.docker.com`:

```bash
apt-get update && apt-get install -y ca-certificates curl gnupg
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg \
  -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) \
  signed-by=/etc/apt/keyrings/docker.asc] \
  https://download.docker.com/linux/debian \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | tee /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io \
  docker-buildx-plugin docker-compose-plugin socat
systemctl enable --now containerd docker
```

- `socat` is still needed INSIDE the distro for the per-connection bridge
  (`socat STDIO UNIX-CONNECT:/var/run/docker.sock`).
- Health: `docker version --format '{{.Server.Version}}'` + `docker run
  --rm hello-world` (only in setup test + doctor, not on every start).
- Kernel preflight: `iptables -L -n`, overlayfs (`grep overlay /proc/filesystems`).
  Stock WSL2 kernel is fine; custom kernels fail with a clear message.

### 1.5 Named-pipe → WSL bridge (the “helper”)

- Docker clients speak **HTTP over named pipe** (`npipe:////./pipe/docker_engine`,
  same as Unix socket on Linux). It is NOT plain TCP.
- Docker Desktop’s trick: listen on `\\.\pipe\docker_engine`, forward each
  connection byte-for-byte to the Engine socket. HTTP Upgrade (“hijack” for
  `run -it`, `exec -it`, `logs -f`) works ONLY if the proxy is a **raw
  byte-copy** (`io.Copy` both directions). Any HTTP parsing breaks hijack.
  Old codebase proved this: `pipe → wsl socat STDIO` with `io.Copy` handles
  `-it` natively; `pipe → 127.0.0.1:2375 → socat TCP-LISTEN` FAILED because
  WSL2 localhost-forwarding doesn’t reliably reach distro-bound 127.0.0.1.
- So: **no TCP hop, no `--port`.** Per connection:
  `winio.ListenPipe → Accept → exec wsl -d dockup -u root -- socat STDIO
  UNIX-CONNECT:/var/run/docker.sock → io.Copy(pipe↔proc)`.
  One short-lived `wsl.exe` per API connection (~0.2–0.5 s) is fine.
- In Go: `github.com/Microsoft/go-winio`:
  `winio.ListenPipe(path, nil)` server, `winio.DialPipe(path, &timeout)` for
  health checks. `PipeName = \\.\pipe\dockup_engine` (NOT `docker_engine`:
  GH runners + user machines already hold Docker Desktop's `docker_engine`
  pipe, proven in e2e — hijacking it would be a false pass. Revision allows
  either name, "e.g. \\.\pipe\dockup_engine or mirroring...". Clients use
  `docker -H npipe:////./pipe/dockup_engine`).
- TTY resize: no special code needed — resize is just another Engine API POST
  (`/containers/.../resize?h=&w=`) multiplexed over the same hijacked stream;
  raw copy forwards it. Do NOT attempt to parse it.
- The “helper” is NOT a separate `.exe`. It is the `relay.Serve(ctx)` loop
  running in-process (foreground) or in the daemon child process. This satisfies
  revision’s “companion helper binary (in Go…)” with the simplest correct
  option: same binary, `dockup __serve` hidden subcommand holding the pipe.
  Rationale: zero installer, zero version skew, lifecycle is trivial
  (parent death = pipe death). Documented as “helper mode” in code.

### 1.6 Windows daemon pattern (no services/tasks)

Revision forbids autostart persistence; daemon = plain background process:

- `dockup daemon start` spawns detached `dockup __serve` with
  `CREATE_NO_WINDOW` (`syscall.SysProcAttr{HideWindow:true, CreationFlags:
  0x08000000}`), stdout/stderr → `%APPDATA%\dockup\logs\dockup.log`
  (size-rotated, ~5 MB cap, keep 3).
- PID + `startedAt` in state file. `daemon stop` kills PID (graceful
  `SIGTERM`-equivalent via `Process.Kill` + pipe-close wait, then
  `wsl --terminate dockup`? NO — do NOT terminate the distro on daemon stop;
  dockerd stays under systemd so restart is instant. Only `uninstall`/`shutdown`
  terminates).
- Actually `shutdown` = stop pipe holder + leave distro stopped-but-imported
  (terminate) so `ps` = stopped. `daemon stop` = stop pipe holder, leave distro
  running (fast restart). This distinction is documented in `daemon stop` vs
  `shutdown` help text.
- Mutual exclusion: file-lock `%APPDATA%\dockup\dockup.lock` (Windows
  `LockFileEx`) around every state read-modify-write + pipe-`Alive()` probe.
  Two terminals double-starting is safe.

---

## 2. Architecture

```text
Windows side (dockup.exe, Go, single static binary)
─────────────────────────────────────────────────────
docker.exe (user-installed, NOT bundled)
   │  npipe:////./pipe/docker_engine (HTTP-over-pipe, hijack-capable)
   ▼
dockup foreground (`dockup`)  OR  dockup daemon child (`dockup __serve`)
   │  go-winio ListenPipe(__pipeName__), per-conn goroutine
   ▼  wsl -d dockup -u root -- socat STDIO UNIX-CONNECT:/var/run/docker.sock
WSL side (distro "dockup", Debian 12 nocloud, systemd=true)
─────────────────────────────────────────────────────
systemd (PID 1) → containerd.service → docker.service
   │  unix:///var/run/docker.sock
   ▼  socat bridge (one per client conn, spawned by Windows side)
containers
```

Data path is ALWAYS raw bytes. No ports, no firewall, no LAN exposure.

State on Windows ONLY (besides the distro):

```text
%APPDATA%\dockup\state.json      # installed/arch/setupAt
%APPDATA%\dockup\dockup.lock     # file lock
%APPDATA%\dockup\logs\dockup.log # daemon log (+ .1/.2 rotation)
%LOCALAPPDATA%\dockup\wsl\       # wsl --import target (ext4.vhdx)
%TEMP%\dockup-debian-<arch>.tar.xz  # setup download (transient)
```

---

## 3. Repo layout (revision-mandated)

```text
dockup/
  cmd/dockup/main.go            # CLI entry: parse args, dispatch, exit codes
  internal/
    config/config.go            # DistroName, PipeName, Debian URLs, dir helpers
    state/state.go              # state.json load/save + file lock
    wsl/wsl.go                  # wsl.exe wrappers, UTF-16 decode, Exec/Script
    download/download.go        # HTTP download with MB progress
    setup/setup.go              # setup orchestrator (all setup steps)
    docker/docker.go            # in-distro docker install/config/test scripts
    relay/relay.go              # go-winio pipe + socat bridge (helper core)
    daemon/daemon.go            # start/stop/restart/status/log, spawn __serve
    doctor/doctor.go            # preflight + repair stale state
    logx/logx.go                # foreground print + daemon file log + rotation
  test/
    unit/                       # pure tests: config URLs, wsl parse, state
    e2e/                        # doc.go only (real e2e runs in GH Actions)
  scripts/
    ops/go-bugcheck.sh          # (existing) lint gate — host-runnable
    build.ps1                   # GOOS=windows go build -o dockup.exe
    test.ps1                    # go vet + go test
  .github/workflows/
    ci.yml                      # windows-latest: vet + test + build + bugcheck
    e2e-wsl.yml                 # windows-latest: WSL + build + smoke (version/doctor/ps)
  plan.md  revision.md  README.md  go.mod  go.sum
```

Why `internal/` split this way: each folder maps 1:1 to a revision concept
(distro name, download, setup %, docker+systemd, bridge/helper, daemon
lifecycle, doctor). `cmd/dockup/main.go` stays thin (dispatch only).

---

## 4. Component design (per package)

### 4.1 `internal/config`

- Consts: `DistroName="dockup"`, `PipeName=`\\.\pipe\docker_engine``,
  `DebianBase="https://cloud.debian.org/images/cloud/bookworm/latest"`,
  `DebianFallbackBase="https://cdimage.debian.org/cdimage/cloud/bookworm/latest"`,
  `Version="dev"` (overridden by `-ldflags -X main.version=...` in CI).
- Funcs: `ArchTarName(arch)`, `DebianURL(arch)`, `FallbackURL(arch)`,
  `StateDir()`, `StateFile()`, `LockFile()`, `LogDir()`, `LogFile()`,
  `WslInstallDir()`, `TempTar(arch)` — all via `%APPDATA%/%LOCALAPPDATA%/%TEMP%`.
- Pure + unit-testable (no `wsl.exe`, no network).

### 4.2 `internal/state`

- Schema:
  ```json
  {"installed":true,"arch":"amd64","setupAt":"2026-09-19T...Z",
   "daemon":{"pid":1234,"startedAt":"..."}}
  ```
  Absent file = not setup. `daemon.pid==0` = no daemon.
- `Load()`, `Save()`, `WithLock(fn)` using `LockFileEx` on Windows
  (`//go:build windows`; non-Windows stub returns error — Windows-only).
- Liveness is NEVER trusted from file alone: callers verify pipe `Alive()`
  + process alive + `wsl --list` before reporting running.

### 4.3 `internal/wsl`

- `decodeWslOutput`, `ParseListOutput` (UTF-16LE/UTF-8 tolerant).
- `List()`, `Exists(name)`, `Running(name)`.
- `Exec(distro, timeout, args...)` — quoteless one-liners only.
- `ExecScript(distro, timeout, script)` — via `sh -s` stdin (mandatory for
  apt/docker/systemd scripts).
- `RunRetry(distro, what, timeout, tries, script)` — apt mirror flake guard.
- `Import(distro, dir, tar)`, `Unregister(distro)`, `Terminate(distro)`.
- Every exec has a timeout; stderr is attached to errors (diagnosable) but
  NEVER parsed for control flow.

### 4.4 `internal/download`

- `Size(url) (int64, error)` — HEAD for `Content-Length` → `DOWNLOADSIZE_INMB`.
- `Fetch(url, dest, onProgress) error` — GET stream, `io.Copy` with ticker
  calling `onProgress(downloaded, total)` every 500 ms; prints
  `installing debian... (X/Y MB)` from the caller.
- Fallback: caller tries primary, on error tries fallback URL.

### 4.5 `internal/setup` (+ `internal/docker`)

- Signature: `Run(arch string) int` — prints EXACTLY the revision banner flow:
  1. `dockup already installed?` → `Warning: An installation of dockup already
     exists on WSL, do you wish to delete that installation and let dockup
     re-install a new dockup instance on WSL? [y/n]` → `n`=abort 0, `y`=
     `wsl --unregister dockup`.
     Else `Setting up dockup: dockup will install and configure a new instance
     on WSL under the name "dockup" — proceed? [y/n]` → `n`=abort.
  2. `installing debian... (0/<SIZE> mb)` — download with progress.
  3. `importing debian into WSL as "dockup"... (0%)` — `wsl --import`
     (progress is indeterminate → spinner/%; we print staged % as import runs).
  4. `testing dockup on WSL... (please wait.)` → `wsl -d dockup echo ok` —
     `test results: success/failure` → failure asks `something went wrong,
     retry or abort? [retry/abort]`.
  5. `installing docker... (0%)` → apt repo + `docker-ce* + socat` via
     `ExecScript` (retry 3×).
  6. `configuring docker... (0%)` → write `/etc/wsl.conf(systemd=true)`,
     `systemctl enable --now containerd docker`, terminate+reboot once.
  7. `testing docker... (0%)` → `docker version` + `hello-world` in-distro.
  8. `testing the docker daemon bridge... (0%)` → start ephemeral relay,
     `docker -H npipe:////./pipe/docker_engine version` from Windows.
  9. `you're all good to go! run "dockup" ... or "dockup daemon start"...`.
- Any `failure` step loops on `[retry/abort]`. `abort` exits 1 with state kept
  (doctor can repair).
- Arch: `setup` (no flag)=amd64; `--amd`/`--arm` override; both = error.

### 4.6 `internal/relay` (the helper core)

- `const PipeName` re-exported from config.
- `Alive() bool` — `winio.DialPipe` 2 s timeout.
- `Serve(ctx, distro) error` — `ListenPipe`, accept loop, `go bridgeConn`
  per client. Closes listener on `ctx.Done()`.
- `bridgeConn(pipe, distro)` — spawn
  `wsl -d dockup -u root -- socat STDIO UNIX-CONNECT:/var/run/docker.sock`,
  `io.Copy` both ways, `Wait()`. Stderr goes to log, NEVER to the pipe.
- Startup contract: caller prints `starting helper...`, waits for `Alive()==true`
  (5 s poll), prints `helper up and running` or `helper failed (err)` + exit 1.
- Shutdown: `ctx` cancel closes listener; in-flight bridges drain.

### 4.7 `internal/daemon`

- `Start()`: refuse if foreground pipe alive with no daemon PID (tell user to
  Ctrl-C foreground); refuse if daemon PID alive (“already running as a daemon
  process, run dockup restart…”); require `installed`; spawn
  `dockup __serve` detached → log file; poll `Alive()`; print brief health.
- `Stop()`: kill PID, wait pipe death (10 s), clear state PID, print health.
- `Restart()`: stop+start.
- `Status()`: `running` + pipe health + `docker version` via pipe? Brief
  `running, pipe ok, errors: none` style. Exit 0 running / 1 stopped.
- `Log()`: interactive tail-follow of log file, read-only, Ctrl-C exits
  (scan + sleep poll, no truncate).
- `__serve` (hidden): the daemon child — `relay.Serve` + signal-forward; holds
  the pipe until killed. Spawned ONLY by `daemon start`.

### 4.8 `internal/doctor`, `internal/logx`

- `doctor`: checks WSL present/version, `dockup` distro exists/running,
  systemd PID 1, `/var/run/docker.sock` + `docker version`, pipe alive,
  state consistency; REPAIRS stale `daemon.pid` (dead PID → clear) and reports
  `fixed vs needs-attention`. Exit non-zero on critical fail, one-line reasons.
- `logx`: `Info/Err` to stdout/stderr (foreground) + `Append(logfile)` with
  rotation (`>5 MB → .1/.2/.3`). Daemon child logs there; foreground logs inline.

### 4.9 `cmd/dockup/main.go`

Thin dispatch (no logic):

```text
dockup                  → foreground: require installed, refuse if daemon/pipe alive, start relay inline, Ctrl-C teardown
dockup setup [--amd|--arm]
dockup uninstall        → confirm [y/n], stop procs, --unregister, clear state
dockup ps               → stopped/running (pipe+distro+daemon aware)
dockup daemon start|stop|restart|status|log
dockup shutdown         → stop foreground-hint + daemon + terminate distro
dockup doctor | version | --help
dockup __serve          → hidden helper holder (not for humans)
```

All failures: one line on stderr + non-zero exit. No stack traces.

---

## 5. Build order (do in this order, commit after each)

- **Step 0 — scaffolding.** `go.mod` (`go 1.24+`, `go-winio`),
  `internal/config`, `internal/state`, `internal/wsl` (parse+decode only),
  `cmd/dockup/main.go` with `version` + `--help` only. Verify: `go vet`, `go build`.
- **Step 1 — pipe core.** `internal/relay` (Alive/Serve/bridge) + `ps` +
  foreground `dockup` skeleton (no docker yet). Unit: pipe alive false when
  closed. CI: build only.
- **Step 2 — download+setup skeleton.** `internal/download` (Size/Fetch) +
  `setup` prompts + arch flags + `wsl --import` wiring. No apt yet. Unit:
  URL builder, arch parse.
- **Step 3 — docker+systemd.** `internal/docker` scripts + `setup` steps
  5–8 + `uninstall`. E2E (GH): full setup on `windows-latest` (250 MB
  download tolerated, 20 min timeout).
- **Step 4 — daemon lifecycle.** `internal/daemon` + `__serve` + `shutdown` +
  `daemon log/status`. E2E: start→status→stop→restart cycle + double-start refusal.
- **Step 5 — doctor+polish.** `doctor` repair, `logx` rotation, `scripts/*.ps1`,
  README/docs touch-up. Final gates: `ci.yml` + `e2e-wsl.yml` green.

Each step = code + unit test + `go vet/build` host check + commit/push + GH run.

---

## 6. Testing strategy (all remote except compile/lint)

| layer | where | cmd |
|-------|-------|-----|
| compile | host (allowed) | `go build ./...`, `go vet ./...` |
| lint | host (allowed) | `bash scripts/ops/go-bugcheck.sh` (WSL-git-bash ok, no dockup run) |
| unit | GH `ci.yml` | `go test ./...` on `windows-latest` (pure: no wsl/network) |
| smoke/e2e | GH `e2e-wsl.yml` | `windows-latest`: `wsl --version`, build exe, `dockup version/doctor/ps`, full `setup` + `daemon start/status/stop` + `uninstall` on `workflow_dispatch` or main push |

- NEVER `wsl --terminate/unregister`, never `dockup setup`, never download
  tarballs on the dev host.
- After every push: `gh run list --limit 5` → `gh run view <ID> --log`,
  fix-forward. Do not stack pushes on red.

Workflows must set `GOOS=windows GOARCH=amd64`, fail fast on non-Windows,
and use `gh workflow run` friendly names (`ci`, `e2e-wsl`).

---

## 7. Risks & decisions log

1. **tar.xz vs raw:** chose `tar.xz` (250 MB) — only format both small AND
   directly importable. qcow2/raw are for QEMU, useless for `wsl --import`.
2. **TCP removed:** old code proved WSL2 NAT loopback can’t reach distro
   `127.0.0.1` binds; STDIO bridge is the only design. No `--port` flag ever.
3. **Pipe name:** `\\.\pipe\dockup_engine` to avoid hijacking Docker
   Desktop's `docker_engine` pipe (GH runners prove the collision is real).
   Clients use `docker -H npipe:////./pipe/dockup_engine`; `dockup ps`/
   `doctor` report foreign `docker_engine` holders instead of hijacking.
4. **arm64 Windows binary:** out of scope — only the *payload* has an arm
   variant (`--arm` downloads arm64 rootfs). Windows exe stays amd64.
5. **systemd reboot:** one `wsl --terminate dockup` is required after writing
   `wsl.conf`; setup does it automatically and re-waits for systemd.
6. **250 MB CI download:** acceptable on `windows-latest` (gigabit); cache the
   tarball per-arch with `actions/cache` keyed by URL to keep reruns fast.

---

## 8. Acceptance (revision verbatim, mapped)

- `setup` fresh → `docker run hello-world` from Windows `docker.exe` with no
  Windows-persistent changes except state/log/vhdx dirs. ✅ §4.5+4.6
- `run -it`, `logs -f`, `compose up` native (raw copy). ✅ §1.5/4.6
- Double-start refused (foreground vs daemon). ✅ §1.6/4.7
- Helper failure kills parent with code+reason. ✅ §4.6
- `uninstall` confirms `[y/n]`, removes distro+state. ✅ §4.5
- `ps`/`daemon status` truthful after reboot (stale PID repaired). ✅ §4.8
- `shutdown` stops everything; `doctor` repairs stale state. ✅ §4.7/4.8
- `version` prints `you're running the latest/dev version.` for now. ✅ §4.9

---

## 9. `gh` runbook (copy-paste)

```powershell
# after each implementation step:
git add -A; git commit -m "<short msg>"; git push
gh run list --limit 5
gh run view <ID> --log   # read fully; fix-forward on failure
gh workflow run ci
gh workflow run e2e-wsl
```

---

## 10. User config, paths, TCP, styling, help, new test suites

### 10.1 `~/.dockup/config.json` (`internal/userconfig`)

Created on first dockup run (`Ensure`), bare minimum:

```json
{
  "default_path": "C:/Users/<u>/AppData/Local/dockup/wsl",
  "current_path": "",
  "port": 2375,
  "use_tcp": false,
  "pipe_name": "\\\\.\\pipe\\dockup_engine",
  "color": true
}
```

- `default_path`: prefilled install location for the next setup; rewritten
  to whatever path was used on EVERY successful setup.
- `current_path`: where the installed distro actually lives; set on setup,
  cleared on uninstall/doctor-repair; dockup reads it to find the distro dir.
- `port` + `use_tcp`: optional 127.0.0.1-only TCP bridge (named pipe is
  always served; default off = no LAN exposure). Validated 1-65535.
- `pipe_name`: default `\\.\pipe\dockup_engine`, overridable.
- `color`: master switch for CLI styling.

### 10.2 Install location UX (`dockup setup [--path=DIR]`)

- `--path="D:/WSL"` (or `--path DIR`) skips the prompt, is validated
  absolute, and becomes the new default on success.
- Interactive: `where do you want to install the dockup distro?`
  `enter path e.g D:/WSL [default: <last used>]:` — empty input keeps the
  default (emulates a prefilled box); anything else replaces it.
- Uninstall keeps `default_path`, clears `current_path`, removes the
  current install dir. Doctor clears a stale `current_path` when the distro
  is gone and validates both paths + port.

### 10.3 Optional TCP bridge (`internal/relay.ServeEx`)

`ServeEx(ctx, distro, pipe, useTCP, port)` serves the pipe plus, when
enabled, `net.Listen("tcp", "127.0.0.1:port")`; every accepted socket
(pipe or TCP) reuses the same socat-STDIO byte-copy bridge. `doctor` and
`daemon status`/`ps` probe `TCPAlive(127.0.0.1:port)` when enabled.

### 10.4 CLI styling (`internal/ui` + `internal/logx`)

Palette: white normal, bold headers, light blue (94, never dark blue) for
help accents, gray hints, yellow warnings, red errors, green success.
Auto-disabled when piped / `TERM=dumb` / `NO_COLOR` / `CI` /
`GITHUB_ACTIONS` so CI logs stay grep-able (substrings unchanged).

### 10.5 Help everywhere + `--dry-run`

`dockup help [command]`, `-h`/`--help` globally and on every command
(`setup`, `uninstall`, `ps`, `daemon [start|...]`, `shutdown`, `doctor`,
`version`). `dockup setup --dry-run` prints arch, distro name, install
dir, URLs, config path, pipe/TCP plan and changes nothing (no download,
no import, no unregister).

### 10.6 New workflows
- `.github/workflows/full-test.yml` (push + dispatch): vet, unit, build,
  help matrix, dry-run creates nothing, config bare-minimum check, custom
  `--path` setup, config default/current assertions, doctor, daemon
  lifecycle, pipe `version` + `hello-world`, TCP enable on port 2376 +
  `version`/`hello-world` via TCP, TCP disable, stop, shutdown, uninstall
  (distro gone, `current_path` cleared, `default_path` retained).
- `.github/workflows/scoop-test.yml` (push + dispatch): install scoop,
  `scoop install docker`, assert the resolved `docker` is the scoop shim,
  build, setup, daemon start, `docker version` + `hello-world` through the
  scoop-installed CLI via `DOCKER_HOST=npipe:////./pipe/dockup_engine`,
  stop, shutdown.

### 10.7 Login autostart + `ps` table

- `~/.dockup/config.json` gains `"autostart": false` (default off).
- `dockup daemon autostart` prints `autostart: on|off`;
  `dockup daemon autostart on|off` writes the config AND creates/removes
  `%APPDATA%\...\Startup\dockup.lnk` (`<exe> daemon start`, minimized).
  Implemented in `internal/autostart` (PowerShell `WScript.Shell`, no new
  Go deps); `doctor` reconciles entry vs setting at info level.
- `dockup ps` renders a bold-header/plain-value table (`internal/pstable`):
  `STATUS` = `running` (pipe up + `GET /_ping` answers `200 OK` via
  `relay.EngineReady`), `starting...` (pipe up, engine not answering yet),
  `stopped`; `AUTOSTART` = `on`/`off` from config. Decision logic is pure
  (`Classify`) and unit-tested; TCP state (when enabled) follows as a gray
  line.   `full-test.yml` asserts query/set/config/shortcut/`ps`/doctor for
  the whole cycle.

### 10.8 Compose, autostart proof, inspection, upgrade, self-heal

- `full-test.yml` additionally runs `docker compose` (plugin) `up/down`
  with a hello-world service over the pipe, executes the Startup `.lnk`
  target after `daemon stop` to prove a login boot works (polls `daemon
  status`), and runs `dockup upgrade` + pipe `version`.
- `scoop-test.yml` additionally installs `docker-compose` (classic) and
  runs `docker-compose up/down` through `DOCKER_HOST`.
- `inspect-wsl.yml` (push + dispatch) audits the generated distro:
  systemd PID 1, unit states, socket, package set, `docker info`
  (overlay2, cgroup v2), `wsl.conf`, apt repo/key, iptables/overlay;
  uploads a `provenance.txt` manifest artifact (package versions +
  engine) for cross-run reproducibility comparison; runs `dockup
  upgrade`; breaks the engine (`systemctl stop docker`), asserts it is
  down, runs `dockup doctor --fix` (reinstalls/reconfigures via
  `InstallScript` + `ConfigureScript` when `TestDaemon` fails) and
  asserts recovery; verifies legacy recovery (deleted config.json is
  recreated with defaults by any command).
- `dockup upgrade` (`internal/docker.Upgrade`: apt update + latest
  engine/plugin packages + unit restarts + `TestDaemon`) and `dockup
  doctor --fix` share the same idempotent scripts as setup.

### 10.9 Bridge I/O coverage + known stdin-EOF limitation

`full-test.yml` now covers: piped stdin bytes (`run -i ... read`),
pseudo-TTY (`run -t`), `exec` ± stdin, `logs -f`, `compose up/down`,
foreground serve, `daemon log` follow, and live `starting...`.
Known limitation (proven by a non-failing probe step): a container that
blocks waiting for stdin EOF (e.g. bare `cat` with piped stdin) hangs,
because the byte-mode named-pipe bridge has no half-close propagation
(TCP gets this for free). Fixing it means message-mode pipe +
CloseWrite propagation — real Docker Desktop parity work, tracked as
future work, not a regression: output without `-i` (the revision's
requirement) is unaffected.

