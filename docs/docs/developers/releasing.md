# Releasing

Releases use `v*` tags. A tag starts two workflows:

- `release.yml`: GoReleaser builds the Windows binaries and creates the GitHub
  release with `GITHUB_TOKEN`.
- `image.yml`: builds, signs, and smoke-tests both rootfs tarballs, then
  attaches them to the same release.

The release is only usable once both finish. The image job fails if the release
does not exist, so it cannot silently publish an image-only release.

## Assets

| Asset | What it is |
|---|---|
| `dockup-windows-amd64.exe` | Windows amd64 binary, stamped with the tag |
| `dockup-windows-arm64.exe` | Windows arm64 binary, stamped with the tag |
| `dockup_checksums.txt` | SHA-256 for both binaries |
| `dockup-amd64.tar.gz` | Prebuilt WSL rootfs, amd64 |
| `dockup-arm64.tar.gz` | Prebuilt WSL rootfs, arm64 |
| `*.tar.gz.sha256` | SHA-256 of the tarball |
| `*.tar.gz.sig` | ed25519 signature over the `.sha256` bytes |

The rolling `image-latest` release still exists for weekly rebuilds; versioned
releases are the immutable reference.

## Release notes

Notes come from `cliff.toml` via git-cliff. `release.yml` renders the commits
in the tag's range, then hands the file to GoReleaser with `--release-notes`.
The prebuilt-image footer is added by the `release` section of
`.goreleaser.yaml`.

Preview them locally:

```powershell
task cliff           # unreleased commits
task release-notes   # latest tag, writes RELEASE_NOTES.md
```

## Cutting a release

From a clean `main` with all workflows green:

```powershell
git tag -a v1.1.0 -m "dockup v1.1.0"
git push origin v1.1.0
```

Watch both runs and inspect the completed release:

```powershell
gh run list --workflow release.yml --limit 1
gh run list --workflow image.yml --limit 1
gh release view v1.1.0
```

If a run fails, fix it on `main`, delete the tag, and retag the fixed commit;
do not move a tag after assets have been published.

## Manual dispatch

Both workflows can be started without pushing a tag.

```powershell
# Build binaries without publishing; uploads dist/ as a workflow artifact.
gh workflow run release.yml -f tag=v1.1.0 -f dry_run=true

# Build and publish images for an existing tag release.
gh workflow run image.yml -f tag=v1.1.0 -f publish=true

# Refresh the rolling release instead.
gh workflow run image.yml -f publish=true
```

`release.yml` requires the tag to exist and checks it out. `image.yml` with an
empty `tag` input publishes to `image-latest`.

## Verifying a release

```bash
sha256sum -c dockup_checksums.txt
gh release download v1.1.0 -p 'dockup-amd64.tar.gz*'
go run ./scripts/image/imgtool verify \
  -pub "$(cat scripts/image/pubkey.txt)" \
  -in dockup-amd64.tar.gz.sha256 \
  -sig dockup-amd64.tar.gz.sig
```

`dockup version` prints the tag in release builds and `go install` builds. Local
`go build` builds print the short commit SHA (with `-dirty` when the tree has
uncommitted changes).
