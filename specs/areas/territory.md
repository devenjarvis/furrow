---
area: territory
title: Territory
---

# Territory

Proving a step's diff stayed inside its `touches`: `furrow territory`.

## Behaviour

`furrow territory [--step REF] [--base REF] [--body-file FILE] [--json]` checks that a step's
diff stays inside its `touches`.

- The step is `--step`, or else the current branch read back through the config's `branch`
  pattern. Off a step branch (or detached) it exits 1: "Not on a step branch. Pass
  `--step FUR-NNNN.N`."
- The base is `--base`, or else `<remote>/<base>`. It isn't fetched.
- Changed files are everything that differs between the merge base of base and `HEAD` and the
  working tree: committed, staged, unstaged, and untracked files that aren't ignored. A rename
  counts as both paths. Paths are relative to the repo root, sorted.
- A file is **inside** when it matches one of the step's `touches` (after aliases, with `**`), or
  it's the step's own spec file in `changes/` or `archive/`, or its area's file.
- With `--body-file` (the PR description), a file outside is **explained** when its whole path
  appears under a `## Territory` heading, up to the next `## ` heading: bare, in backticks, or as
  a list item.

```
FUR-0004.1: 6 files inside its territory.
Outside, explained in the PR:
  Makefile
Outside, not explained:
  go.mod
Explain each one under "## Territory" in the PR, or move the change to its own step.
```

It exits 0 when nothing is outside and unexplained, and 1 otherwise. `--json` prints
`{"step", "base", "inside": [...], "explained": [...], "outside": [...]}`, with empty lists as
`[]`.

## History
