---
area: specs
title: Specs and lint
---

# Specs and lint

How Furrow reads the repo: `.furrow.yml`, spec and area files, invariants, and the rules `furrow lint` enforces.

## Behaviour

**Config.** Furrow finds `.furrow.yml` by walking up from the working directory. Keys it leaves out
take defaults: `prefix: SPEC`, `specs: specs`, `product: docs/product.md`, `remote: origin`,
`base: main`, `branch: step/{num}-{step}`, no `check`, no `aliases`, no `visual`. With no
file it says to run `furrow init`.

**Files.** Specs are `specs/changes/NNNN-slug.md` and `specs/archive/NNNN-slug.md`: YAML front
matter, then Markdown. Acceptance criteria are `- [ ] AC-n: text (proof: detail)` lines under
`## Acceptance criteria`, and the proof is the last `(test:`, `(check:` or `(human:` on the line.
Areas are `specs/areas/<area>.md`. Invariants are the `**INV-n: name.**` marks under
`## Invariants` in the product file.

**Refs.** `FUR-0003.1` is a step, `FUR-0003` is every step of a spec, and in `after` a bare `2` is a
step of the same spec. A step is merged when `merged: true`, or when its spec is archived.

**Paths.** `touches` entries are globs from the repo root: `*` and `?` within a segment, `**` across
segments. A leading alias from `.furrow.yml` expands to its path, and a bare alias means its whole
folder. Two entries overlap when their literal prefixes agree up to the shorter one, so
`internal/**` overlaps `internal/spec/x.go`, and `cmd/furrow/next.go` doesn't overlap
`cmd/furrow/claim.go`.

**`furrow lint`** enforces the twelve rules in `specs/README.md`: identity, enums, the area,
required sections, numbered criteria with proofs, steps for decided specs (numbered, with touches,
covering every criterion, never size `l`, no `TBD` outside code), `after` refs that exist with no
cycles, `touches` whose top-level folder exists (deeper folders may be new), invariants that
exist and force human review, sign-off lines for ticked `human:` boxes, done meaning done, and
well-formed areas. It warns when an accepted spec is complete and ready to archive. It exits 1
on any error.

**Before and after.** An `accepted` or `paused` spec with an unmerged step has a
`## Before and after` section: a mock, written by hand, of what a user sees now and will see. It
needs a `**Before:**` and an `**After:**` line, at most 20 words each after the label on its line
(inline code counted, HTML comments not), and a fenced code block or an image under each, up to
the next label. A Before may say `Nothing yet.` instead, and `No visible effect.` under After
excuses both sides. Labels and headings inside fenced code don't count. Lint reports
`missing section "## Before and after"`, `"## Before and after" needs a **Before:** and an
**After:** line`, `**After:** in "## Before and after" is 27 words. Keep it to 20.`, and
`"## Before and after" doesn't show the change. Add a code block or image under **After:**, or
say "No visible effect."` (Before's says "Nothing yet."). A draft, a spec with every step merged
and an area never need one. A spec with `imported: fetch` or `imported: edition` gets the missing
section as a warning; any other `imported:` value is an error.

**Deciding a spec.** A spec's PR is opened with `status: accepted`. The unmerged PR is the
proposal, and merging it is the decision: `furrow next` runs from the base branch and never sees
an unmerged spec. Closing the PR is the owner saying no. `draft` means parked: kept but never
offered. `furrow new` starts specs as `draft`, so lint stays relaxed while one is written. In an
`accepted` or `paused` spec, each top-level item under `## Open questions` carries `Proposed: …`,
and that proposal is the answer unless the owner overrules it before merging. Lint reports an item
without one by its line in the file, and ignores headings and items inside fenced code. Agents
never merge a spec PR.

**Diagrams.** A spec that changes how parts connect or how something flows draws it in
`## What` as a ```` ```mermaid ```` diagram of only what changes, new parts marked with
`classDef added`. An area may keep one in `## Behaviour`, kept current by the fold. Each has a
one-sentence caption above, a `Legend:` line below, at most 8 nodes drawn `flowchart TD`, and
shows behaviour, not a call graph, with every node a real file, package, type or function. Lint
doesn't check them; `furrow brief` copies a spec's into the step's brief.

**`furrow list [--json]`** shows each spec's worked-out status: draft, accepted, building, review,
paused or done.

**The CLI.** Each command is a file in `cmd/furrow/` that registers itself, so adding one never
touches `main.go`. `furrow help` lists them.

## History

- FUR-0016: A step may touch folders it creates (archived 2026-11-03)
- FUR-0012: Merging a spec's PR accepts it (archived 2026-11-03)
