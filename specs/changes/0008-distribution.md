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
    merged: false
  - n: 2
    title: Tagged releases with GoReleaser and a Homebrew tap
    after: [1]
    touches:
      - .goreleaser.yaml
      - .github/**
    covers: [AC-3]
    size: s
    merged: false
  - n: 3
    title: A pinned shim for repos that use Furrow
    after: [2]
    touches:
      - internal/scaffold/**
      - cmd/furrow/scaffold.go
    covers: [AC-4, AC-5]
    size: m
    merged: false
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

- [ ] AC-1: CI runs `make check` on every PR (check: a PR shows the job)
- [ ] AC-2: CI fails a PR whose diff leaves its step's territory unexplained (human: a test PR)
- [ ] AC-3: Tagging `v0.1.0` produces four binaries, checksums and a formula (human: the release page)
- [ ] AC-4: `bin/furrow` refuses a binary whose checksum doesn't match (test: TestShimChecksum)
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
