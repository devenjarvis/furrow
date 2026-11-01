---
id: FUR-0004
title: Prove a step's diff stays inside its touches
status: accepted
area: territory
priority: p1
human_review: optional
invariants: []
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/specs.md
  - specs/areas/claims.md
steps:
  - n: 1
    title: furrow territory
    after: [FUR-0003.1]
    touches:
      - internal/territory/**
      - cmd/furrow/territory.go
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5]
    size: s
    merged: false
---

# FUR-0004: Prove a step's diff stays inside its touches

## Why

In Fetch, four PRs went outside their spec's `touches` because the list had drifted from the real
work. Each one was explained in the PR body, but nothing checked. Parallel agents are only safe if
territory holds, so it has to be checked by a machine, in the agent's loop and again in CI.

## What

**`furrow territory [--step REF] [--base REF] [--body-file FILE] [--json]`.**

- The step is `--step`, or else the current branch read back through the config's `branch`
  pattern. If neither gives a step it exits 1: "Not on a step branch. Pass `--step FUR-NNNN.N`."
- The base is `--base`, or else `<remote>/<base>`.
- Changed files are everything that differs between the merge base of base and `HEAD` and the
  working tree: committed, staged, unstaged, and untracked files not ignored
  (`git diff --name-only <merge-base>` plus `git ls-files --others --exclude-standard`). Renames
  count as both paths.
- A file is **inside** when it matches one of the step's `touches` (after aliases), or it's the
  step's own spec file (in `changes/` or `archive/`), or its area's file.
- With `--body-file` (the PR description), a file outside is **explained** when it appears, as a
  path, under a `## Territory` heading in that file.

Output:

```
FUR-0004.1: 6 files inside its territory.
Outside, not explained:
  go.mod
Explain each one under "## Territory" in the PR, or move the change to its own step.
```

It exits 0 when nothing is outside and unexplained, and 1 otherwise. `--json` prints
`{"step", "base", "inside": [...], "explained": [...], "outside": [...]}`.

## Acceptance criteria

- [ ] AC-1: Works out the step from the branch name, and fails with the message off a step branch (test: TestStepFromBranch)
- [ ] AC-2: Counts committed, uncommitted and untracked changes since the merge base (test: TestChangedFiles)
- [ ] AC-3: The spec file and the area file are always inside, and touches with aliases and `**` match (test: TestInside)
- [ ] AC-4: Paths listed under "## Territory" in the body file are explained, and the exit code follows the rule (test: TestExplained)
- [ ] AC-5: `--json` has the documented keys (test: TestTerritoryJSON)

## Out of scope

- Wiring it into CI. FUR-0008 adds the workflow.

## Decisions

- 2026-11-01: Now after FUR-0003.1, not FUR-0001. Reading the diff runs `git`, and only
  `internal/gitx` (built in FUR-0003.1) may run a process (INV-7). `furrow next` showed this step
  as ready to run beside FUR-0003.1, which would have meant two copies of the git runner.
