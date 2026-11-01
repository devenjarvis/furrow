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
    merged: true
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

- [x] AC-1: Works out the step from the branch name, and fails with the message off a step branch (test: TestStepFromBranch)
- [x] AC-2: Counts committed, uncommitted and untracked changes since the merge base (test: TestChangedFiles)
- [x] AC-3: The spec file and the area file are always inside, and touches with aliases and `**` match (test: TestInside)
- [x] AC-4: Paths listed under "## Territory" in the body file are explained, and the exit code follows the rule (test: TestExplained)
- [x] AC-5: `--json` has the documented keys (test: TestTerritoryJSON)

## Out of scope

- Wiring it into CI. FUR-0008 adds the workflow.

## Decisions

- 2026-11-01: Now after FUR-0003.1, not FUR-0001. Reading the diff runs `git`, and only
  `internal/gitx` (built in FUR-0003.1) may run a process (INV-7). `furrow next` showed this step
  as ready to run beside FUR-0003.1, which would have meant two copies of the git runner.
- 2026-11-01: The base isn't fetched first. `furrow territory` compares against the local
  `<remote>/<base>` ref, so it works offline; CI fetches before it runs.
- 2026-11-01: Changed files come from `git diff --name-only --no-renames --relative` and
  `git ls-files --others --exclude-standard`, run at the repo root, so paths are relative to the
  folder holding `.furrow.yml`, like `touches`.
- 2026-11-01: The inside count is only files inside. Explained files are listed under
  "Outside, explained in the PR:" so a reviewer sees them; it reads "1 file" for one.
- 2026-11-01: Under `## Territory`, a path counts when it's a backtick span or a whole word, with
  list markers and surrounding punctuation (quotes, brackets, `,;:!?*`, trailing `.`) trimmed.
  `## Territory` matches case-insensitively and the section ends at the next `## ` heading.
- 2026-11-01: The off-branch message uses the config's prefix (`--step FUR-NNNN.N` here). A
  detached HEAD counts as off a step branch. A step branch for a step that doesn't exist is an
  error that names it.
- 2026-11-01: `--body-file` is read relative to the working directory. An error (no step, no
  merge base, unreadable body file) exits 1 with the message on stderr.
