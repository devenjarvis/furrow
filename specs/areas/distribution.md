---
area: distribution
title: Distribution
---

# Distribution

How people get Furrow: releases, Homebrew, and a pinned copy that CI can run without the network.

## Behaviour

**CI.** `.github/workflows/ci.yml` runs `make check` on every PR, and `furrow territory
--body-file` on PRs from step branches, rerunning when the PR description is edited.

**Releases.** A `v*` tag runs `.github/workflows/release.yml`: `make check`, then GoReleaser
(`.goreleaser.yaml`, pinned with `go run`) builds one static binary each for macOS and Linux on
amd64 and arm64, publishes `furrow_<version>_<os>_<arch>.tar.gz` archives and `checksums.txt`
(sha256) on the release page, and updates the Homebrew cask in `devenjarvis/homebrew-tap`.

**`furrow init --pin --checksums FILE [--version V]`** does what `furrow init` does, then writes
`bin/furrow` (executable) and `.furrow/.gitignore` (ignoring `.furrow/bin/`), never overwriting
either. `V` is a release tag and defaults to the running furrow's version; `FILE` is that
release's `checksums.txt`, and it must hold all four archives. Without `--checksums` it says where
to download them. It checks the pin before writing anything.

`bin/furrow` is a POSIX shell script holding the version and the four archives' sha256 lines. On
first run it picks the archive for `uname`, downloads it with curl or wget from the release page
(or `$FURROW_DOWNLOAD_URL/<tag>/`), checks it with `sha256sum` or `shasum -a 256`, installs the
binary at `.furrow/bin/<tag>/furrow`, and runs it with the same arguments. An archive whose
checksum doesn't match is refused and nothing is installed. Later runs use the installed copy.

**`furrow doctor [--repo URL] [--json]`** reads the version `bin/furrow` pins and the latest
release, the highest `v*` tag at `--repo` (default `https://github.com/devenjarvis/furrow.git`) read
with `git ls-remote`, and says whether the pin is behind and how to move it. It exits 0 whether or
not the pin is behind, and 1 when the tags can't be read. With `--json` it prints
`{"shim", "pinned", "latest", "behind"}`.

## History
