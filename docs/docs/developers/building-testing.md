# Building & Testing

## Host rule (non-negotiable)

The dev machine is **compile/lint only**. Never run `dockup setup/run/daemon`
locally, never touch WSL, never download distro payloads, never install
software on the host. All functional testing goes through GitHub Actions via
the `gh` CLI, and every test run is preceded by `git commit` + `git push`.

Allowed on host:

```powershell
go build ./...
go vet ./...
gofmt -l .
go test ./...            # unit only (part of bugcheck)
sh scripts/ops/go-bugcheck.sh    # busybox sh OK (skips the gopls step)
sh scripts/ops/go-seccheck.sh
```

Version stamping (as CI does it):

```powershell
go build -ldflags "-X github.com/CHE3MZ/dockup/internal/config.Version=$(git rev-parse --short HEAD)" -o dockup.exe ./cmd/dockup
```

Local unstamped builds report `dev`.

## gh runbook

```powershell
git add -A; git commit -m "<short>"; git push
gh run list --limit 7
gh run view <ID> --log   # full text — never trust green/red blindly
gh workflow run <name> [-f full-setup=true]
```

`git add -A` sweeps parallel uncommitted edits into your commit — always run
`git status` / `git diff` first and report what rode along.

## CI map (all on push + dispatch)

| Workflow | Proves |
|---|---|
| `ci` | vet, `go test -race`, version-stamped build + smoke |
| `e2e-wsl` | WSL smoke: version/doctor/ps, refusal cases, foreign-pipe branch |
| `full-test` (~7 min) | End-to-end: setup → daemon → pipe/TCP/compose/stdin/TTY/exec/logs → upgrade → restore → uninstall |
| `scoop-test` | Real scoop-installed `docker` + `docker-compose` via `DOCKER_HOST` |
| `inspect-wsl` | Distro audit, provenance manifest, upgrade, break → `doctor --fix`, `--full`, reinstall reproducibility |
| `multi-distro` | Neighbor distros untouched; shutdown → `starting...` → self-heal; scoped uninstall |
| `arm-test` | arm64 vet/unit/build always; `--arm` install gated on a WSL probe (arm images currently lack WSL) |
| `docs` | `mkdocs build --strict` for this site |

## PowerShell-in-CI pitfalls (learned the hard way)

- `$LASTEXITCODE` persists: steps tolerating nonzero native exits must end `exit 0`.
- `-match` on arrays returns elements (truthy!): `(cmd) -join "`n"` first.
- `wsl.exe` emits raw UTF-16LE intermittently: strip NULs before matching; Go side already decodes.
- Single-element arrays, `echo x |` pipeline stdin closing, `wsl -- <quoted args>`
  mangling (quoteless one-liners; scripts via stdin in Go) — follow existing patterns.
- `Start-Job` stdin piping is unreliable; prefer main-shell commands with timeouts.

## Frozen strings

CI greps these verbatim — never reword: the setup banner flow,
`dockup has not been setup yet run "dockup setup" to set it up.`,
`starting...`, `test results :`, `autostart: on/off`,
`STATUS/AUTOSTART/INSTALLED/SIZE/MEMORY`, `read-only`, `ok: autostart on`,
`dry run complete`.

## Exe icon

`cmd/dockup/winres/winres.json` points at `assets/appicon.png`; the
checked-in `rsrc_windows_*.syso` files are picked up automatically by
`go build`. Regenerate after changing the art:

```powershell
cd cmd/dockup
go run github.com/tc-hib/go-winres@latest make --arch amd64,arm64
```

## Docs

This site: `docs/mkdocs.yml` + `docs/docs/`. Build strictly before pushing:

```powershell
cd docs
mkdocs build --strict
```
