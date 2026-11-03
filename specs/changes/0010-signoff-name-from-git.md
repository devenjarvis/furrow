---
id: FUR-0010
title: furrow signoff takes the name from git when --by is left out
status: accepted
area: lifecycle
priority: p2
human_review: required
invariants: [INV-3]
context:
  - docs/product.md
  - specs/archive/0005-lifecycle.md
  - specs/areas/lifecycle.md
steps:
  - n: 1
    title: Default --by to git config user.name
    after: [FUR-0005]
    touches:
      - cmd/furrow/lifecycle.go
      - internal/lifecycle/**
    covers: [AC-1, AC-2]
    size: s
    merged: true
---

# FUR-0010: furrow signoff takes the name from git when --by is left out

## Why

FUR-0005 said `--by` defaults to `git config user.name`, but the step was built before
`internal/gitx` was on main, so the agent made `--by` required and logged a spec problem. The owner
shouldn't have to type their name for every sign-off.

## What

When `--by` is left out, `furrow signoff` reads `git config user.name` through `internal/gitx` and
passes it as `SignoffRequest.By`. When that's empty too, it refuses as it does now: "Say who
checked it: --by NAME." The agent check (INV-3) still runs first, before git is asked anything.

## Acceptance criteria

- [x] AC-1: With no `--by`, the sign-off line names git's `user.name`, and with neither it refuses with the message (test: TestSignoffNameFromGit)
- [x] AC-2: Inside an agent session it refuses before running git (test: TestSignoffRefusesAgentsBeforeGit)

## Out of scope

- Any other identity source, such as the forge's API.

## Open questions

- Is this worth a step at all, or is typing `--by` fine? Proposed: yes, it's small, and it keeps the
  sign-off line honest without friction.

## Decisions

- 2026-11-03: Chose a `SignoffRequest.Name` lookup, defaulting to `git config user.name` in the repo root, so tests can prove the agent check runs before git.
- 2026-11-03: Chose to treat a git failure (no repo, no user.name) as no name, so it ends in the same "Say who checked it" refusal.
- 2026-11-03: Used the existing `gitx.Git.Run`, so `internal/gitx` needed no change.
