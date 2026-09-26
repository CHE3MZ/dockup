# Contributing

Issues and pull requests are welcome at
[CHE3MZ/dockup](https://github.com/CHE3MZ/dockup).

## Ground rules

- **Never mutate the host in PRs.** No code path may require running dockup
  commands, touching WSL, or downloading payloads on a dev machine — CI owns
  all functional testing (see [Building & Testing](developers/building-testing.md)).
- **Keep it tiny.** Stdlib-first, minimal dependencies (`go-winio` only at
  runtime). One package per concept; `main.go` stays dispatch-only.
- **Short commit messages** in the repo's existing style
  (e.g. `move installed flag from config to state`).
- **Pure logic gets unit tests** in `test/unit/` (no WSL, no network);
  user-facing flows get CI workflow asserts.
- **User-facing changes update these docs**, and must pass
  `mkdocs build --strict` (the `docs` workflow enforces it).
- **Don't reword frozen strings** — CI greps the setup banner, `starting...`,
  `test results :`, and the `ps` headers verbatim.

## Workflow

1. Fork, branch, implement (host: `go build` / `go vet` / lint scripts only).
2. Commit short, push, then watch the workflows:
   `gh run list` → `gh run view <ID> --log`, fix-forward.
3. All workflows green (or scoped-green for docs-only changes) before review.
