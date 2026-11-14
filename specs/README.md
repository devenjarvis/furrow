# The Furrow spec standard

**No spec, no code.** Every change implements a step of an accepted spec. A step is sized so one
agent can finish it in one session, alone, and prove it's done.

```
specs/
  README.md            this standard
  _template.md         copy it, or run `furrow new <slug>`
  changes/NNNN-slug.md specs being decided or built
  areas/<area>.md      living specs: how each part works now
  archive/NNNN-slug.md finished specs, the record of why
```

`furrow lint` enforces everything below. CI runs it.

## The rules

1. **The spec is the source of truth.** If behaviour changes, a spec says so first.
2. **One step, one PR, one session.** A step that's too big (`size: l`) can't be accepted. Split it.
3. **Self-contained.** An agent holding the repo, the spec and its `context` files has everything
   it needs. If it would have to ask, the spec isn't ready.
4. **Every criterion is checkable.** Each one names its proof: `(test: Name)`, `(check: command)`
   or `(human: what to look at)`.
5. **Declared territory.** Each step lists the paths it may change in `touches`. Steps with
   overlapping paths never run at the same time.

## Front matter

```yaml
---
id: FUR-0007              # prefix from .furrow.yml, then the file's 4-digit number
title: Claude Code plugin and skills
status: accepted          # draft | accepted | paused | done (done lives in archive/)
area: agents              # the living spec this changes: specs/areas/agents.md
priority: p1              # p0 now | p1 next | p2 later
human_review: required    # required | optional
invariants: [INV-3]       # invariants from docs/product.md this touches (makes review required)
context:                  # read these before starting
  - docs/product.md
steps:
  - n: 1
    title: The skills
    after: [FUR-0002.2, 3] # FUR-NNNN.N, a step of this spec (N), or FUR-NNNN (all of it)
    touches: [skills/**]  # globs from the repo root, or an alias from .furrow.yml
    covers: [AC-1, AC-2]  # the criteria this step makes true
    size: m               # s (an hour or two) | m (half a day) | l (split it)
    merged: false         # set to true by the step's own PR (`furrow done`)
    blocked: ""           # optional: why it can't start, whatever else says
---
```

- Every criterion is covered by at least one step.
- `merged: true` is set by the step's own PR with `furrow done FUR-NNNN.N`. Merging the PR makes
  it true, and steps that wait on it unblock immediately. Nobody waits for sign-off.
- A step that a person must judge still merges. Its `human:` criteria wait in `furrow review`.

## Status

```
draft (parked) · PR opened as accepted ──merge──► accepted ──every step merged──► (review) ──signed off──► done, archived
                                                  │
                                                  └──► paused (say why under "## Open questions")
```

| Status | Means | Who sets it |
| --- | --- | --- |
| `draft` | Parked: being written, or kept but not built yet. Never offered. | `furrow new` |
| `accepted` | Set in the spec's PR, and merging it is the decision. Its open steps are offered. | The writer, in the PR. The owner merges it, or closes it to say no. |
| `paused` | Decided, but waiting on something outside the code. | The owner |
| `done` | Every step merged and every box ticked. Moved to `archive/`. | `furrow archive` |

"Building" and "review" aren't written down. `furrow list` works them out: a spec is building
once a step is merged, and in review once every step is merged but `human:` boxes are open.

In progress isn't written down either. A step is in progress while its claim branch exists on the
remote. `furrow next` reads the remote's branches, so no two agents get the same step.

A claimed step is **handed back** once `furrow done` is pushed to its branch: the branch's copy of
the spec says `merged: true` for it, and its PR waits for review.

### Stacking

With `stack: N` in `.furrow.yml` (0, or left out, is off), a step doesn't have to wait for what it
comes `after` to merge. When every unmerged step it waits on is handed back, and they sit on one
chain at most N deep, it's ready **stacked** on the top one: `furrow claim` starts its branch from
that step's branch, and its PR is based on it. The owner reviews a stack from the bottom, and
merging the bottom PR moves the next one onto the base. Plain ready steps are offered first.

## Body sections

| Section | Required | What goes in it |
| --- | --- | --- |
| `## Why` | yes | Two to four sentences: the reason this exists, for a user. |
| `## What` | yes | The behaviour, precisely. Commands, flags, output, files, failure modes. |
| `## Before and after` | while any step is unmerged | A mock of what a user sees now and will see: a `**Before:**` and an `**After:**` line, at most 20 words each, each with a code block or image under it. |
| `## Acceptance criteria` | yes | `- [ ] AC-1: … (test: …)`. Observable, each with its proof. |
| `## Out of scope` | yes | What a reasonable agent might do and must not. |
| `## Open questions` | if any | Questions for the owner. In a decided spec each one says `Proposed: …`, and that stands unless the owner overrules it before merging. |
| `## Decisions` | grows | `- 2026-11-01: Chose X because Y.` Written while building. |
| `## Notes for the agent` | optional | Gotchas, prior art, fixtures. |

`## Before and after` is required in an `accepted` or `paused` spec while any step is unmerged;
a draft, a spec with every step merged, and an area never need one. It is a mock, written by
hand, not real output: a terminal session for a CLI, an ASCII wireframe for a screen, or a call
and its result for a library, in a fenced code block or an image under each line. A Before for
something new may say `Nothing yet.` instead, and a change nobody can see says
`No visible effect.` after `**After:**` and needs no block on either side. A spec written by
`furrow import` (`imported: fetch` or `imported: edition`) gets a warning, not an error, until it
has one. The spec's brief copies it for the owner, and agents build and verify against it.

A ticked `human:` criterion needs a line in `## Decisions`, written by `furrow signoff` in a
sign-off PR: `- 2026-11-01: AC-4 signed off by Deven.` Merging that PR is the sign-off.

A spec that changes how parts connect or how something flows (a new component, a changed call
path, a state machine) draws it in `## What` as a ```` ```mermaid ```` diagram. It shows only what
changes, with new parts marked by `classDef added`, and every node names a real file, package, type
or function. A spec that doesn't change structure has no diagram.

Every diagram, in a spec, an area or a PR, is drawn for a reviewer who didn't draw it, often on a
phone:

- **A caption.** One sentence above it says what question it answers.
- **A legend.** A line under it, starting `Legend:`, says what a box, an arrow and each colour mean.
  Every arrow means one thing, or each carries a label (`-->|reads|`).
- **Small and narrow.** At most 8 nodes, drawn `flowchart TD`. Split a bigger picture, or leave it
  out.
- **Behaviour, not a call graph.** It shows what flows where, or what changes. Every node still
  names a real file, package, type or function.

## Areas: living specs

An area says how one part of the product works **now**, in present tense, with no history of how
it got there. Each change names its area. Before a spec is archived, the PR that merges its
last step folds what changed into the area's `## Behaviour`. `furrow archive` adds a line to the
area's `## History`.

```yaml
---
area: scheduling
title: Scheduling
---
```

Required sections: `## Behaviour` and `## History`. An area may keep a ```` ```mermaid ````
diagram in `## Behaviour`, and the fold on a spec's last step keeps it current.

## How an agent works a step

1. `furrow next` lists steps that are free now. With `--agents N` it picks N that can run together.
2. Read the spec, its `context`, `AGENTS.md`, the area spec, and the `## Decisions` of the specs
   it comes after.
3. `furrow claim FUR-NNNN.N`. It creates the branch on the remote and checks it out. If someone
   beat you to it, run `furrow next` again.
4. Open a draft PR titled `[FUR-NNNN.N] <step title>` whose body lists the criteria it covers.
5. Write the tests first. Stay inside `touches`, and run `furrow territory` to prove it.
6. Run the repo's check (`check:` in `.furrow.yml`). Tick each criterion you covered, with evidence,
   except `human:` ones. Run `furrow done FUR-NNNN.N`. If this was the last step, fold the change into
   the area spec.
7. Push, then mark the PR ready: `furrow done` goes in before ready, never after, so the step is
   handed back and nothing that waits on it is held up. A stacked step's PR is based on the branch
   `furrow claim` said it's stacked on.

### Signing off

A `human:` criterion is signed off in a sign-off PR, never in a step's PR, and only when the owner
asks.

1. `furrow review` lists what waits. Ask the owner which criteria they've checked, and how.
2. On a branch from the base, `furrow signoff FUR-0005 AC-8 FUR-0012 AC-5 --by <owner>` ticks
   exactly those and writes each sign-off line. `--by` names the person who checked, never the agent.
3. `furrow territory --signoff` proves the diff changes only specs.
4. Open one PR titled `Sign off: FUR-0005 AC-8, FUR-0012 AC-5`.
5. Merge it only when the owner says to, in their own words. Merging it is the sign-off. Closing it
   says "not yet".

### When the spec is wrong

- **Small gap** inside your territory: decide, log it in `## Decisions`, carry on.
- **Real problem**: a criterion can't be met, it contradicts an invariant, or it needs another
  step's paths. Stop, write "Spec problem" in the PR, and propose the change. Don't edit the
  criteria. A person decides.

## Writing a good spec

- **Write for a smart stranger.** Name files, types, commands and flags.
- **Examples beat adjectives.** "Two steps touching `internal/**` and `internal/spec/x.go` overlap"
  beats "detects conflicts".
- **Name the failure modes.** No remote, no network, a dirty tree, a claim that already exists.
- **Keep territory small.** Steps that list the same file can't run together. A command each in its
  own file under `cmd/furrow/` keeps them apart.
