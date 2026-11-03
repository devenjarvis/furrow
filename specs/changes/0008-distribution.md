---
id: FUR-0008
title: Releases, Homebrew, CI, and a pinned copy for repos that use Furrow
status: accepted
area: distribution
priority: p1
human_review: required
invariants: [INV-7]
context:
  - docs/product.md
steps:
  - n: 1
    title: CI for this repo
    after: []
    touches:
      - .github/**
    covers: [AC-1, AC-2]
    size: s
    merged: true
  - n: 2
    title: Tagged releases with GoReleaser and a Homebrew tap
    after: [1]
    touches:
      - .goreleaser.yaml
      - .github/**
    covers: [AC-3]
    size: s
    merged: true
  - n: 3
    title: A pinned shim for repos that use Furrow
    after: [2]
    touches:
      - internal/scaffold/**
      - cmd/furrow/scaffold.go
    covers: [AC-4, AC-5]
    size: m
    merged: true
---

# FUR-0008: Releases, Homebrew, CI, and a pinned copy for repos that use Furrow

## Why

Fetch and Edition run CI on a self-hosted Mac and agents work in cloud containers. Neither should
have to install Go, or fetch "latest" and get a different answer from yesterday. A repo pins a
version, and the shim fetches exactly that binary and checks its checksum.

## What

1. `.github/workflows/ci.yml` runs `make check` and `furrow territory --body-file` on PRs.
2. A `v*` tag builds macOS and Linux binaries for arm64 and amd64 with GoReleaser, publishes
   checksums, and updates a Homebrew tap formula.
3. `furrow init --pin` writes `bin/furrow`, a POSIX shell script holding a version and the
   checksums. On first run it downloads that release into `.furrow/bin/` (ignored by git), checks
   it, and runs it. `furrow doctor` says when the pinned version is behind the latest one.

## Acceptance criteria

- [x] AC-1: CI runs `make check` on every PR (check: a PR shows the job)
- [ ] AC-2: CI fails a PR whose diff leaves its step's territory unexplained (human: a test PR)
- [ ] AC-3: Tagging `v0.1.0` produces four binaries, checksums and a formula (human: the release page)
- [x] AC-4: `bin/furrow` refuses a binary whose checksum doesn't match (test: TestShimChecksum)
- [ ] AC-5: The shim works in a cloud session through its proxy (human: run it in a session)

## Out of scope

- Windows.

## Open questions

- Is the Homebrew tap worth it, or is `go install` plus the shim enough? Proposed: shim first, tap
  when someone asks.

## Decisions

- 2026-11-03: The `devenjarvis/furrow` repo is on GitHub, so the open question about needing it is settled.
- 2026-11-03: Chose to skip territory in CI off step branches because `furrow territory` exits 1 there, the same as a real violation.
- 2026-11-03: Chose to recreate the head branch at CI's merge commit because territory reads the step from the branch, and checkout leaves HEAD detached.
- 2026-11-03: Chose to rerun CI when a PR description is edited, so a `## Territory` explanation added later can clear the check.
- 2026-11-03: Chose `actions/checkout@v5` and `actions/setup-go@v6` because the v4/v5 majors warned that Node.js 20 is deprecated on the first CI run.
- 2026-11-03: Chose the tap `devenjarvis/homebrew-tap` and a `HOMEBREW_TAP_GITHUB_TOKEN` secret because the release's own token can't push to another repo.
- 2026-11-03: Chose a Homebrew cask over a formula because GoReleaser v2 deprecated `brews`; `goreleaser check` fails on it. The cask installs the same binary.
- 2026-11-03: Chose `go run goreleaser@v2.19.0` in the workflow over its action, so the version is pinned and nothing joins go.mod (INV-7).
- 2026-11-03: Chose `init --pin --checksums FILE` (the release's checksums.txt) because the binary can't download (INV-7) or know its own archive's checksum.
- 2026-11-03: Chose `--version`, defaulting to the running furrow's version, so a release build pins itself and a dev build must name a tag.
- 2026-11-03: Chose to write `.furrow/.gitignore` over editing the repo's `.gitignore`, because init never overwrites a file.
- 2026-11-03: Chose to keep an existing `bin/furrow`, like every file init writes; moving the pin is delete it and rerun `furrow init --pin`.
- 2026-11-03: Chose `git ls-remote --tags` for `furrow doctor`'s latest release, so it needs only git (INV-2, INV-7); pre-releases count only when nothing else is tagged.
- 2026-11-03: Chose a `FURROW_DOWNLOAD_URL` override in the shim for another release host; TestShimChecksum uses it with a file:// URL, so no test touches the network.
- 2026-11-03: Chose to exit 0 when the pin is behind, so CI doesn't start failing the day a release comes out.
- 2026-11-03: Chose to describe `init --pin` and `doctor` in the distribution area only, because `specs/areas/scaffolding.md` is outside this step's territory.
