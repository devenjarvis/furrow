---
id: FUR-0005
title: Hand back steps, sign off what a person must judge, archive finished specs
status: done
area: lifecycle
priority: p0
human_review: required
invariants: [INV-3, INV-4]
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/specs.md
steps:
  - n: 1
    title: done, review, signoff and archive
    after: [FUR-0001]
    touches:
      - internal/lifecycle/**
      - cmd/furrow/lifecycle.go
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6, AC-7, AC-8]
    size: m
    merged: true
---

# FUR-0005: Hand back steps, sign off what a person must judge, archive finished specs

## Why

Fetch's worst friction: an agent can't tick a `human:` criterion, so the spec stayed `ready`, the
linter refused `done` on anything after it, and the owner merged eleven separate "mark done" PRs to
unblock the queue. Here, work unblocks on merge and sign-off is its own queue. Finished specs move
to the archive, so `changes/` only holds what's still moving, and what they changed lives on in the
area's living spec.

## What

Every command edits the spec file's text in place. It changes only the line it means to change, so
the rest of the file, comments and formatting included, is untouched. It never changes a
criterion's text, only its box (INV-4).

**`furrow done <ref>`** sets `merged: true` on that step. It refuses, with the reason, when:
- any `after` isn't merged;
- a criterion the step covers, other than a `human:` one, is still unticked: "Tick AC-2 with its
  evidence first."

**`furrow review [--json]`** lists every unticked `human:` criterion whose covering steps are all
merged, grouped by spec, with the proof text: what the person should look at. With none it prints
`Nothing waiting for sign-off.`

**`furrow signoff <id> <AC-n>... [--by NAME]`**
- Refuses inside an agent session: when `CLAUDECODE` or `FURROW_AGENT` is set in the environment, it
  exits 1 with "Sign-off is for a person (INV-3). Ask the owner to run this." (INV-3)
- `--by` defaults to `git config user.name`. With neither it refuses.
- Each criterion must be a `human:` criterion that `review` would list. Otherwise it refuses and
  changes nothing.
- Ticks each box and appends `- YYYY-MM-DD: AC-n signed off by NAME.` to `## Decisions`.
- If the spec is now complete (every step merged, every box ticked), it archives it as below and
  says so.

**`furrow archive <id>`** refuses unless every step is merged and every box ticked. Then it:
1. sets `status: done`;
2. moves the file from `changes/` to `archive/`, keeping its name;
3. appends `- <id>: <title> (archived YYYY-MM-DD)` to the area file's `## History`;
4. reminds you that the area's `## Behaviour` should already say what the spec changed.

Dates come from an injectable clock, so tests are deterministic.

## Acceptance criteria

- [x] AC-1: `done` flips only that step's `merged` line and leaves every other byte of the file alone (test: TestDoneEditsOneLine)
- [x] AC-2: `done` refuses when an `after` isn't merged, or a covered non-human criterion is unticked (test: TestDoneRefusals)
- [x] AC-3: `review` lists only `human:` criteria whose covering steps are all merged (test: TestReview)
- [x] AC-4: `signoff` refuses criteria that `review` doesn't list, and changes nothing (test: TestSignoffRefusals)
- [x] AC-5: `signoff` ticks the box, writes the Decisions line that `furrow lint` accepts, and archives a now-complete spec (test: TestSignoffArchives)
- [x] AC-6: `archive` sets done, moves the file, and appends to the area's History (test: TestArchive)
- [x] AC-7: No command changes a criterion's text (test: TestCriteriaTextUntouched)
- [x] AC-8: The review and sign-off flow is clear to the owner (human: run `furrow review` then `furrow signoff` on a test repo)

## Out of scope

- Writing the area's Behaviour section. That's the agent's job in the last step's PR (the
  `furrow-implement` skill says so).

## Decisions

- 2026-11-01: `--by` has no default yet: reading `git config user.name` means running git, and only
  `internal/gitx` may (INV-7), which isn't on main. `signoff` refuses with "Say who checked it:
  --by NAME." until a follow-up passes the git name in as the default.
- 2026-11-01: `signoff` checks `CLAUDECODE` and `FURROW_AGENT` before it parses flags or loads the
  repo, and a missing environment counts as an agent.
- 2026-11-01: `done` on a step that's already merged says so, changes nothing and exits 0. When the
  step completes its spec, or leaves `human:` checks waiting, `done` says what's next.
- 2026-11-01: `review` lists only specs still in `changes/`, and a `human:` criterion no step covers
  isn't listed (lint reports it). `review --json` is `{"specs": [{id, title, file, criteria: [{id,
  text, look_at}]}]}`, with the proof text as `look_at`.
- 2026-11-01: `signoff` ticks and logs criteria in AC order, ignores repeats, and refuses the whole
  call if any one isn't waiting. A line appended to a section goes after its last line; an empty
  section gets a blank line under the heading first.
- 2026-11-01: `done` adds a `merged: true` line after a block step's `n:` line when the step has no
  `merged` key; a flow-style step without one is refused.
- 2026-11-01: `archive` works out every change (status line, destination, area History) before it
  writes anything, and refuses if the destination exists or the area file is missing.
- 2026-11-03: AC-4 changed in FUR-0015's spec PR. `signoff` no longer refuses agent sessions: merging
  a sign-off PR is the sign-off (INV-3 as FUR-0015 words it).

- 2026-11-03: AC-8 signed off by Deven Jarvis.
