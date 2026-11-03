---
area: lifecycle
title: Lifecycle
---

# Lifecycle

How a step is handed back and a spec is finished: `furrow done`, `furrow review`, `furrow signoff` and `furrow archive`.

## Behaviour

**Edits.** Every lifecycle command edits the spec file's text in place. It changes only the line
it means to (the step's `merged:` value, the character inside a criterion's `[ ]`, the `status:`
value) and appends lines to `## Decisions` or the area's `## History`, so comments and formatting
survive. It never changes a criterion's text (INV-4).

**`furrow done <ref>`** sets `merged: true` on one step. It refuses, changing nothing, while any
`after` isn't merged ("FUR-0005.1 is after FUR-0001, which isn't merged. Merge it first.") or a
criterion the step covers, other than a `human:` one, is unticked ("Tick AC-2 with its evidence
first."). Then it says what's next: the `human:` checks now waiting for sign-off, or that the spec
can be archived. On a step that's already merged it says so and changes nothing.

**`furrow review [--json]`** lists every unticked `human:` criterion in `changes/` whose covering
steps are all merged, grouped by spec, with its proof text as what to look at. With none it prints
`Nothing waiting for sign-off.`

**`furrow signoff <id> <AC-n>... [--by NAME]`** is for people. Inside an agent session (`CLAUDECODE`
or `FURROW_AGENT` set) it exits 1 with "Sign-off is for a person (INV-3). Ask the owner to run
this." before doing anything else. Each criterion must be one `review` lists, or it refuses and
changes nothing. It ticks each box and appends `- YYYY-MM-DD: AC-n signed off by NAME.` to
`## Decisions`, in AC order. When that completes the spec, it archives it and says so. With no
`--by`, NAME is `git config user.name` in the repo, read through `internal/gitx` only after the
agent check passes. With neither, it refuses: "Say who checked it: --by NAME."

**`furrow archive <id>`** refuses, naming the unmerged steps and unticked boxes, unless the spec is
complete. Then it sets `status: done`, moves the file from `changes/` to `archive/` with the same
name, appends `- <id>: <title> (archived YYYY-MM-DD)` to the area's `## History`, and reminds you
that the area's `## Behaviour` should already say what the spec changed.

Dates come from an injected clock.

## History

- FUR-0010: furrow signoff takes the name from git when --by is left out (archived 2026-11-03)
