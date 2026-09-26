# Contributing to dockup

Thanks for helping out. Full guide:
<https://github.com/CHE3MZ/dockup/tree/main/docs/docs/contributing.md>

Short version:

- **Host is compile/lint only.** Never run `dockup setup/run/daemon` locally,
  never touch WSL, never download distro payloads. Allowed: `go build`,
  `go vet`, `gofmt`, `go test` (unit), `scripts/ops/go-bugcheck.sh`,
  `scripts/ops/go-seccheck.sh`. Everything functional runs on GitHub Actions
  via `gh`, preceded by `git commit` + `git push`.
- **Keep it tiny.** Stdlib-first (`go-winio` only at runtime), one package per
  concept, `main.go` stays dispatch-only.
- **Short commit messages** in repo style
  (e.g. `move installed flag from config to state`).
- **Tests:** pure logic → `test/unit/`; user flows → CI workflow asserts.
- **Docs:** user-facing changes update `docs/docs/` (must pass
  `mkdocs build --strict` from `docs/`).
- **Frozen strings** (CI greps them): setup banner flow,
  `dockup has not been setup yet run "dockup setup" to set it up.`,
  `starting...`, `test results :`, `autostart: on/off`,
  `STATUS/AUTOSTART/INSTALLED/SIZE/MEMORY`, `read-only`, `ok: autostart on`,
  `dry run complete` — never reword.
- **Green before review:** all workflows green, full logs read
  (`gh run view <ID> --log`), fix-forward.
