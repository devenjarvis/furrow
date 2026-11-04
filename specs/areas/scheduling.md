---
area: scheduling
title: Scheduling
---

# Scheduling

Which steps can start now, which can run together, and the order they unblock in: `furrow next` and `furrow graph`.

## Behaviour

An **open** step is an unmerged step of an accepted spec. Each open step is in one state, checked
in this order:

- **claimed**: its claim branch is on the remote;
- **blocked**: it has a `blocked` reason;
- **waiting**: something in its `after` isn't merged (a whole-spec ref waits for every step);
- **ready**: otherwise.

**Stacking.** `stack: N` in `.furrow.yml` (a whole number from 0 to 9; 0, or the key left out, is
off) lets a step start on top of steps that are waiting for review. A claimed step is **handed
back** when its claim branch's tip has the step's spec file with `merged: true` on that step, which
is what `furrow done` writes. Furrow fetches the claim branches into `refs/furrow/claims/` and reads
the spec file at each tip with `git show`; a tip without the file, or one that doesn't parse, isn't
handed back. It also reads each claim commit's `furrow-stack:` line, the branch it was stacked on.
With `N > 0`, a waiting step is **ready, stacked** when every unmerged step it waits on is handed
back, those steps sit on one chain (the top one, the **parent**, has the others below it), and the
parent's chain, counting the parent, is at most N unmerged steps long. The chain is the parent, the
step its claim is stacked on, and so on, until a step that isn't stacked, is merged or has no
claim. Its **depth** is that chain's length. A waiting step that can't stack says why:
`(FUR-0090.1 isn't handed back)`, `(on different stacks: FUR-0090.1, FUR-0095.1)` or `(the stack
would be 4 deep, over stack: 3)`. When claims aren't checked, nothing stacks.

**Ranking.** Plain ready steps come before stacked ones. Then ready steps are ordered by how many
open steps wait on them, directly or not (more first), then by the longest chain of open steps
behind them, then priority, then spec and step number. The step that unblocks the most work goes
first.

**Picking.** Walking the ranked list, a step joins the "together" set when its `touches` overlap
neither a step already in the set nor any claimed step. A stacked step may overlap the claimed
steps of its parent's chain, since it builds on top of them. Every step left out is listed with its
clash: `not FUR-0006.1: shares cmd/furrow/x.go with FUR-0004.1`.

**Overlap.** Two `touches` entries overlap when they can name the same file, after aliases are
resolved. Two exact files overlap only when they're equal. An exact file against a glob: the glob
decides (a file shallower than the glob's literal folders overlaps when those folders agree). Two
globs are walked segment by segment from the root: a `**` in either overlaps anything below it;
two literal segments must be equal; otherwise each segment's literal prefix and suffix (the text
around `*`, `?`, `[...]` and `\` escapes) must agree: one prefix starts the other, and one suffix ends the other.
Globs of different depths with no `**` left don't overlap. So `internal/brief/brief*` and
`internal/brief/check*` don't overlap, and `internal/**/*.go` and `internal/**/*.md` do. When in
doubt it overlaps, so two agents never share a file.

**`furrow next [--agents N] [--json] [--offline]`** lists ready steps with their file and touches,
the together set, held steps, claimed steps with their branches, waiting steps with what they wait
on, and blocked steps with why. With `--agents N` it lists only the N picked steps under
`Assignments:`, followed by the held lines. When steps are ready but none can be assigned, it
prints `Assignments: none. Every ready step shares files with a claimed step.` and then the held
lines. Claims come from `git ls-remote` on the configured remote. When the remote can't be
reached, it says "(Couldn't reach origin, so claimed steps aren't left out.)", sets
`claims_checked: false`, and still answers. `--json` gives the same sections as stable,
machine-readable JSON. A stacked ready step has a third line, `FUR-0091.1  stacked on step/0090-1
(FUR-0090.1)`, and a waiting step that can't stack ends its line with why. Every step in `--json`
has `"stack_on"` (`{"ref", "branch"}` of the parent, or null) and `"depth"` (0 when not stacked);
a waiting step that can't stack adds `"why_waiting"`.

**`furrow graph [--json] [--offline]`** puts every open step in a wave: wave 1 waits on nothing
open, and each later wave waits on the one before. Each step shows its state and what it waits on.
A stacked step stays in the wave its waits put it in, and its state reads `ready (stacked)`.

**`furrow status [--json] [--offline]`** says where everything stands in one call, from the specs
and the remote's branches: In progress, Handed back, Ready now (at most 5, ranked as `furrow next`
ranks them, then `+N more: furrow next`), Waiting on the owner (human checks, specs ready to
archive), Stale or leftover claims, and one `Next:` line, the first that applies of: release
leftover claims, start the ready steps, review the handed-back PRs, check what `furrow review`
lists, or write a spec. Each section is sorted by ref. `--json` gives the same facts, with every
ready step. Offline, or with the remote unreachable, the claim sections say `(remote not checked)`
and it exits 0. It never asks a forge.

## History

- FUR-0002: Say what can start now, and what can run together (archived 2026-11-01)
- FUR-0013: furrow next tells patterns apart and says why a step is held (archived 2026-11-02)
