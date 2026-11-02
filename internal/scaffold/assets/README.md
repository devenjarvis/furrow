# The spec standard

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
id: {{PREFIX}}-0007              # prefix from .furrow.yml, then the file's 4-digit number
title: Export a report as CSV
status: accepted          # draft | accepted | paused | done (done lives in archive/)
area: reports             # the living spec this changes: specs/areas/reports.md
priority: p1              # p0 now | p1 next | p2 later
human_review: required    # required | optional
invariants: [INV-1]       # invariants from docs/product.md this touches (makes review required)
context:                  # read these before starting
  - docs/product.md
steps:
  - n: 1
    title: The CSV writer
    after: [{{PREFIX}}-0002.2, 3] # {{PREFIX}}-NNNN.N, a step of this spec (N), or {{PREFIX}}-NNNN (all of it)
    touches: [src/reports/**] # globs from the repo root, or an alias from .furrow.yml
    covers: [AC-1, AC-2]  # the criteria this step makes true
    size: m               # s (an hour or two) | m (half a day) | l (split it)
    merged: false         # set to true by the step's own PR (`furrow done`)
    blocked: ""           # optional: why it can't start, whatever else says
---
```

- Every criterion is covered by at least one step.
- `merged: true` is set by the step's own PR with `furrow done {{PREFIX}}-NNNN.N`. Merging the PR makes
  it true, and steps that wait on it unblock immediately. Nobody waits for sign-off.
- A step that a person must judge still merges. Its `human:` criteria wait in `furrow review`.

## Status

```
draft ──merge──► accepted ──every step merged──► (review) ──signed off──► done, archived
                    │
                    └──► paused (say why under "## Open questions")
```

| Status | Means | Who sets it |
| --- | --- | --- |
| `draft` | Being written, or has questions that change the work. Never offered. | `furrow new` |
| `accepted` | Decided. Its open steps are offered. | The owner, by merging |
| `paused` | Decided, but waiting on something outside the code. | The owner |
| `done` | Every step merged and every box ticked. Moved to `archive/`. | `furrow archive` |

"Building" and "review" aren't written down. `furrow list` works them out: a spec is building
once a step is merged, and in review once every step is merged but `human:` boxes are open.

In progress isn't written down either. A step is in progress while its claim branch exists on the
remote. `furrow next` reads the remote's branches, so no two agents get the same step.

## Body sections

| Section | Required | What goes in it |
| --- | --- | --- |
| `## Why` | yes | Two to four sentences: the reason this exists, for a user. |
| `## What` | yes | The behaviour, precisely. Commands, flags, output, files, failure modes. |
| `## Acceptance criteria` | yes | `- [ ] AC-1: … (test: …)`. Observable, each with its proof. |
| `## Out of scope` | yes | What a reasonable agent might do and must not. |
| `## Open questions` | if any | Undecided things. An accepted spec only has ones an agent may decide. |
| `## Decisions` | grows | `- 2026-11-01: Chose X because Y.` Written while building. |
| `## Notes for the agent` | optional | Gotchas, prior art, fixtures. |

A ticked `human:` criterion needs a line in `## Decisions` that `furrow signoff` writes:
`- 2026-11-01: AC-4 signed off by Deven.`

A spec that changes how parts connect or how something flows (a new component, a changed call
path, a state machine) draws it in `## What` as a ```` ```mermaid ```` diagram. It shows only what
changes, with new parts marked by `classDef added`, and every node names a real file, package, type
or function. A spec that doesn't change structure has no diagram.

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
3. `furrow claim {{PREFIX}}-NNNN.N`. It creates the branch on the remote and checks it out. If someone
   beat you to it, run `furrow next` again.
4. Open a draft PR titled `[{{PREFIX}}-NNNN.N] <step title>` whose body lists the criteria it covers.
5. Write the tests first. Stay inside `touches`, and run `furrow territory` to prove it.
6. Run the repo's check (`check:` in `.furrow.yml`). Tick each criterion you covered, with evidence,
   except `human:` ones. Run `furrow done {{PREFIX}}-NNNN.N`. If this was the last step, fold the change into
   the area spec.
7. Mark the PR ready.

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
- **Keep territory small.** Steps that list the same file can't run together. Code split into
  small files and folders keeps them apart.
