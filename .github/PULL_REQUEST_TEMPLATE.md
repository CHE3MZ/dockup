## What

Short description of the change.

## Checklist

- [ ] Host untouched: only `go build` / `go vet` / `gofmt` / `go test` (unit) / lint scripts ran locally — no `setup`/`run`/`daemon`, no WSL changes, no downloads
- [ ] `git status` / `git diff` reviewed (no stray or parallel edits swept in unnoticed)
- [ ] Commit message(s) short, in repo style
- [ ] All workflows green, full logs read (`gh run view <ID> --log`), no blind green/red
- [ ] Frozen strings unchanged (setup banner, `starting...`, `test results :`, `STATUS/AUTOSTART/INSTALLED/SIZE/MEMORY`, `autostart: on/off`, `read-only`, `ok: autostart on`, `dry run complete`)
- [ ] Docs updated if user-facing (`docs/docs/`, `mkdocs build --strict` clean from `docs/`)
- [ ] Pure logic covered by `test/unit/`; user flows covered by CI asserts
