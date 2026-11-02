---
id: FUR-0013
title: furrow next tells patterns apart and says why a step is held
status: accepted
area: scheduling
priority: p1
human_review: required
invariants: [INV-6]
context:
  - docs/product.md
  - specs/areas/scheduling.md
  - internal/paths/paths.go
  - internal/schedule/schedule.go
steps:
  - n: 1
    title: Overlap compares patterns segment by segment
    after: []
    touches:
      - internal/paths/paths.go
      - internal/paths/paths_test.go
    covers: [AC-1, AC-2]
    size: s
    merged: true
  - n: 2
    title: furrow next --agents lists held steps and why
    after: []
    touches:
      - internal/schedule/schedule.go
      - internal/schedule/schedule_test.go
      - cmd/furrow/next_test.go
    covers: [AC-3, AC-4]
    size: s
    merged: true
  - n: 3
    title: Fold into the scheduling area
    after: [1, 2]
    touches:
      - specs/areas/scheduling.md
    covers: [AC-5]
    size: s
    merged: false
---

# FUR-0013: furrow next tells patterns apart and says why a step is held

## Why

During FUR-0011's swarm, `furrow next` held FUR-0011.9 back because `internal/brief/brief*` and
`internal/brief/check*` "overlap". No file can match both, so the run lost parallelism. Worse, the
text output with `--agents` printed an empty `Assignments:` and nothing else, so the ready steps
looked missing. Only `--json` showed they were held, and by what.

## What

### Overlap, segment by segment

`paths.Overlap` keeps its contract: it says two `touches` lists **can** name the same file, and it
never says they can't when they can. A false "no overlap" would let two agents edit one file, so
when in doubt it still says they overlap.

What changes is how two globs are compared. Today they overlap when their literal folders agree up
to the shorter one, so anything below a shared folder clashes. Instead, after aliases are resolved,
the two patterns are walked segment by segment from the root:

- If either current segment is `**`, they overlap. (`**` can stand for any depth.)
- Two literal segments that differ: no overlap.
- Otherwise, compare the two segments' **literal prefixes** (the text before the first `*`, `?` or
  `[`) and **literal suffixes** (the text after the last of them; a segment with no glob character
  is all prefix and all suffix). If one prefix doesn't start with the other, or one suffix doesn't
  end with the other, they can't match the same name: no overlap. Else move to the next segment.
- If one pattern runs out while the other has segments left, and neither has a `**` left, they
  can't match a file at the same depth: no overlap. If both run out together, they overlap.

The existing rules for an exact file against a glob, and two exact files, stay as they are.

Examples:

| a | b | Overlap |
| --- | --- | --- |
| `internal/brief/brief*` | `internal/brief/check*` | no: prefixes `brief`, `check` |
| `internal/**/*.go` | `internal/**/*.md` | yes: `**` comes first |
| `internal/*.go` | `internal/*.md` | no: suffixes `.go`, `.md` |
| `internal/brief/**` | `internal/brief/check*` | yes: `**` |
| `skills/*` | `skills/furrow-write/**` | yes: `*` and `furrow-write` agree, then `**` |
| `cmd/*/main.go` | `cmd/furrow/*.go` | yes |
| `internal/a*/x` | `internal/b*/x` | no: prefixes `a`, `b` |

### `furrow next --agents N` says what it held

With `--agents N`, the text output lists the held steps after the assignments, in the same form
as without `--agents`: `  not FUR-0011.9: shares internal/brief/brief* and internal/brief/check*
with FUR-0011.2`. When ready steps exist but none can be assigned, it prints `Assignments: none.
Every ready step shares files with a claimed step.` and then the held lines, instead of an empty
`Assignments:` heading. `--json` doesn't change: it already has `held`.

```
Assignments: none. Every ready step shares files with a claimed step.
  not FUR-0011.8: shares cmd/furrow/brief.go with FUR-0011.2
  not FUR-0011.9: shares internal/brief/brief* and internal/brief/check* with FUR-0011.2
Claimed:
  FUR-0011.2  step/0011-2
```

## Acceptance criteria

- [x] AC-1: Each row of the table in `## What` gives the overlap it says, and a `**` in either pattern before the first disagreement still overlaps (test: TestOverlapSegments)
- [x] AC-2: Every existing overlap case gives the same answer as before (check: go test ./internal/paths -run 'TestOverlap$')
- [x] AC-3: With `--agents N`, the text lists each held step with its clash after the assignments (test: TestNextAgentsShowsHeld)
- [x] AC-4: With ready steps but none assignable, the text says `Assignments: none.` and why, not an empty heading (test: TestNextAgentsNoneAssignable)
- [ ] AC-5: The scheduling area describes overlap segment by segment and the held lines under `--agents` (check: grep -q "segment by segment" specs/areas/scheduling.md && grep -q "Assignments: none" specs/areas/scheduling.md)

## Out of scope

- Comparing the inside of `[...]` character classes. A segment holding `[` is judged only by its
  literal prefix and suffix.
- Listing steps left out only because `--agents N` was reached. Plain `furrow next` lists every
  ready step.
- Any change to `--json`.

## Open questions

- Should the held lines under `--agents` also name ready steps left out only because N was
  reached? Proposed: no. They aren't held by anything, and `furrow next` without `--agents` shows
  them.
- Should Overlap compare inside `[...]` character classes? Proposed: not yet. Prefix and suffix
  catch the cases seen so far, and staying conservative there can't cause a collision.

## Decisions
- 2026-11-02: Chose to print the held lines under `--agents` with the same helper plain `furrow next` uses because the spec asks for the same form, and one function keeps the two in step.
- 2026-11-02: Chose to print `Assignments: none.` only when the together set is empty because then no step was picked, so every hold is against a claimed step and the message is true; when some steps are picked, the heading stays `Assignments:` and the held lines may name a picked step.
- 2026-11-02: Chose to count a `\` escape (with the character after it) as a glob token, and a whole `[...]` class as one token ending at its closing `]`, because a prefix or suffix cut too short only makes Overlap more cautious, while a suffix taken from inside a class could miss a shared file.
- 2026-11-02: Chose to leave an exact file shallower than a glob's literal folders on the old rule, because the spec keeps the exact-against-glob rules as they are; only two globs are walked segment by segment.
