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

**Ranking.** Ready steps are ordered by how many open steps wait on them, directly or not (more
first), then by the longest chain of open steps behind them, then priority, then spec and step
number. The step that unblocks the most work goes first.

**Picking.** Walking the ranked list, a step joins the "together" set when its `touches` overlap
neither a step already in the set nor any claimed step. Every step left out is listed with its
clash: `not FUR-0006.1: shares cmd/furrow/x.go with FUR-0004.1`.

**`furrow next [--agents N] [--json] [--offline]`** lists ready steps with their file and touches,
the together set, held steps, claimed steps with their branches, waiting steps with what they wait
on, and blocked steps with why. With `--agents N` it lists only the N picked steps under
`Assignments:`. Claims come from `git ls-remote` on the configured remote. When the remote can't be
reached, it says "(Couldn't reach origin, so claimed steps aren't left out.)", sets
`claims_checked: false`, and still answers. `--json` gives the same sections as stable,
machine-readable JSON.

**`furrow graph [--json] [--offline]`** puts every open step in a wave: wave 1 waits on nothing
open, and each later wave waits on the one before. Each step shows its state and what it waits on.

## History
