---
id: FUR-0002
title: Say what can start now, and what can run together
status: done
area: scheduling
priority: p0
human_review: optional
invariants: []
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/specs.md
steps:
  - n: 1
    title: Ready steps, ranking, picking N that don't overlap, and graph
    after: [FUR-0001]
    touches:
      - internal/schedule/**
      - cmd/furrow/next.go
      - cmd/furrow/graph.go
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6]
    size: m
    merged: true
  - n: 2
    title: Leave out steps claimed on the remote
    after: [1, FUR-0003.1]
    touches:
      - cmd/furrow/next.go
      - cmd/furrow/graph.go
      - cmd/furrow/next_test.go
    covers: [AC-7, AC-8]
    size: s
    merged: true
---

# FUR-0002: Say what can start now, and what can run together

## Why

This is the command that makes a swarm possible: an agent with no task runs `furrow next` and knows
where to go. Fetch's `make next` ignored `touches`, so two agents could pick steps that collide.
Edition's `scripts/next` checked overlap, but ordered steps by how the specs were listed. With many
agents, the step that unblocks the most work should go first.

## What

`internal/schedule` takes the loaded repo plus a set of claimed steps (each with its branch), and
works out each open step's state. A step is **open** when its spec is accepted and the step isn't
merged. An open step is:

- **claimed**, when it's in the claimed set;
- otherwise **blocked**, when it has a non-empty `blocked`;
- otherwise **waiting**, when any of its `after` isn't merged;
- otherwise **ready**.

**Ranking.** Ready steps are ordered by, in turn:
1. how many open steps wait on it, directly or through others (more first);
2. the longest chain of open steps that waits on it (longer first);
3. priority (p0 first);
4. spec number, then step number.

**Picking.** Walking the ranked ready steps, a step joins the "together" set when its `touches`
don't overlap (`paths.Overlap`) any step already in the set or any claimed step. `--agents N` stops
after N. Every ready step left out says why: `not FUR-0006.1: shares cmd/furrow/x.go with
FUR-0004.1`.

**`furrow next [--agents N] [--json] [--offline]`.** Text output, with sections only when they have
something in them:

```
Ready to start:
  FUR-0004.1  p1  m  Territory check
              specs/changes/0004-territory.md · touches internal/territory/**, cmd/furrow/territory.go
Can run together: FUR-0004.1, FUR-0005.1
  not FUR-0006.1: shares cmd/furrow with FUR-0004.1
Claimed:
  FUR-0003.1  step/0003-1
Waiting:
  FUR-0007.1  after FUR-0004.1, FUR-0005.1
Blocked:
  FUR-0008.1  needs the GitHub repo
```

With `--agents N` the first section is `Assignments:` and lists only the picked steps, ranked. With
nothing ready it prints `Nothing is ready to start.` and exits 0. `--json` prints
`{"claims_checked": bool, "ready": [...], "together": [...], "held": [{"step", "clash_with",
"paths"}], "claimed": [...], "waiting": [...], "blocked": [...]}`, where each step is
`{"ref", "spec", "title", "priority", "size", "file", "touches", "after", "branch"}`.

**`furrow graph [--json]`.** Every open step in waves: wave 1 waits on nothing open, wave N waits
on something in wave N−1. Each line is `ref  [ready|claimed|blocked|waiting]  title  after …`.

**Step 1** works with no claims at all and prints `(Claims not checked: offline.)` above the
output. **Step 2** reads the claims from the remote through `internal/claim` (FUR-0003.1). When the
remote can't be reached it prints `(Couldn't reach <remote>, so claimed steps aren't left out.)`, sets
`claims_checked: false`, and still answers. `--offline` skips the remote.

## Acceptance criteria

- [x] AC-1: Open steps are sorted into ready, waiting and blocked as described, and a step after a whole spec waits until every step of it is merged (test: TestStates)
- [x] AC-2: Ranking puts the step with the most open steps waiting on it first, then the longest chain, priority, and number (test: TestRanking)
- [x] AC-3: Picking never puts two overlapping steps together, respects `--agents N`, and gives each held step the clash (test: TestPickTogether)
- [x] AC-4: Text output matches the documented layout, and nothing ready prints the documented line (test: TestNextText)
- [x] AC-5: `--json` has the documented keys and is byte-identical across two runs (test: TestNextJSON)
- [x] AC-6: `furrow graph` puts each open step in the wave after the deepest open step it waits on (test: TestGraphWaves)
- [x] AC-7: A claimed step isn't offered, and a ready step overlapping a claimed one is held with that clash (test: TestNextLeavesOutClaims)
- [x] AC-8: With the remote unreachable, `next` still answers and says claims weren't checked (test: TestNextRemoteDown)

## Out of scope

- Claiming. That's FUR-0003.
- Waiting on a step's sign-off instead of its merge. If a spec needs that, say so in "Open
  questions" for a later spec.

## Decisions

- 2026-11-01: Step 1 built by hand: no `furrow claim` yet (FUR-0003.1 waits on this step).
- 2026-11-01: Rendering (`Plan.Text`, `GraphText`) and JSON live in `internal/schedule`, so they're
  tested there and the command files stay thin.
- 2026-11-01: Step 2's touches now include `cmd/furrow/next_test.go`. Its criteria are tests
  (AC-7, AC-8), and without a test file in its territory the step couldn't prove them. No
  criterion changed.
- 2026-11-01: `branch` in `--json` is the claim branch for claimed steps and empty otherwise.
  Working out the branch to claim is `internal/claim`'s job (FUR-0003).
- 2026-11-01: Step 2 claimed with `furrow claim FUR-0002.2`, the first step Furrow claimed for itself.
- 2026-11-01: `graph` reads claims too, so claimed steps show as `[claimed]` there. It also takes
  `--offline`.
