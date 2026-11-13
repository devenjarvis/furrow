---
id: FUR-0021
title: Spec PRs show a mocked before and after
status: accepted
area: brief
priority: p1
human_review: required
invariants: [INV-6]
context:
  - docs/product.md
  - specs/areas/brief.md
  - specs/areas/specs.md
  - internal/brief/spec.go
  - internal/spec/lint.go
steps:
  - n: 1
    title: Specs carry a Before and after, and lint checks it
    after: []
    touches:
      - internal/spec/**
      - internal/scaffold/assets/_template.md
      - internal/scaffold/assets/README.md
      - internal/scaffold/scaffold_test.go
      - specs/_template.md
      - specs/README.md
      - specs/areas/specs.md
      - internal/importer/**
      - specs/areas/scaffolding.md
    covers: [AC-1, AC-2, AC-3, AC-4, AC-10]
    size: m
    merged: false
  - n: 2
    title: The spec brief shows the mock
    after: []
    touches:
      - internal/brief/spec.go
      - internal/brief/spec_test.go
      - specs/areas/brief.md
    covers: [AC-5, AC-6, AC-9]
    size: s
    merged: true
  - n: 3
    title: Skills write the mock, build to it, and check against it
    after: []
    touches:
      - skills/furrow-write/**
      - skills/furrow-implement/**
      - skills/furrow-verify/**
    covers: [AC-7, AC-8]
    size: s
    merged: false
---

# FUR-0021: Spec PRs show a mocked before and after

## Why

A spec's PR asks the owner to accept a plan, but today its brief shows the plan's shape (steps,
collisions, costs) and never what a user will see. A mocked before and after (the terminal
session, the screen, the call and its result) is the fastest thing for a person to judge on a
phone, and it gives every agent building a step the same concrete target to build to and verify
against.

## What

**A new spec section, `## Before and after`.** It mocks what a user or caller sees today and what
they will see once every step is merged. It is written by the spec's author, is a mock and not
real output, and lives in the spec file, so agents read it as part of the spec (INV-1). It holds:

- a line starting `**Before:**`, at most 20 words after the label, then what it looks like now;
- a line starting `**After:**`, at most 20 words after the label, then what it will look like;
- under each line, a fenced code block (a terminal session for a CLI, an ASCII wireframe for a
  screen, a call and its result for a library) or an image. A Before for something new may be
  `Nothing yet.` instead. A change nobody can see says `No visible effect.` after the `**After:**`
  line and needs no block on either side.

For this spec, it is the section below.

**Lint.** `## Before and after` joins the required sections only for a spec whose `status` is
`accepted` or `paused` and that has at least one step with `merged: false`. A spec with every step
merged, a draft, and an area are never asked for one, so every spec in the repo today still
passes. When it is required, lint reports, each with the file:

- `missing section "## Before and after"`;
- `"## Before and after" needs a **Before:** and an **After:** line`;
- `**After:** in "## Before and after" is 27 words. Keep it to 20.` (and the same for Before);
- `"## Before and after" doesn't show the change. Add a code block or image under **After:**, or
  say "No visible effect."`; and the same for Before, which also accepts `Nothing yet.`.

**Imported specs.** `furrow import fetch|edition` writes `imported: fetch` or `imported: edition`
in each spec's front matter, after `human_review`. A spec with an `imported:` key that would get
`missing section "## Before and after"` gets it as a warning instead of an error, so `import
--write` still writes and every file it writes still passes `furrow lint`. Once such a spec has the
section, it is checked like any other. An `imported:` value other than `fetch` or `edition` is an
error.

Words are counted as `furrow brief --check` counts them for `## Behaviour`: after the label, on
its line, with inline code counted and HTML comments not.

**Template and standard.** `specs/_template.md`, the copy `furrow init` writes
(`internal/scaffold/assets/_template.md`), the standard (`specs/README.md` and its scaffold copy)
gain the section between `## What` and `## Acceptance criteria`, with a placeholder Before and
After. The README's body-sections table gets a row: required while any step is unmerged.

**The spec brief.** `furrow brief --spec ID` writes a `## Before and after` section right after
`## Decision`: the spec's section copied byte for byte, then the line
`A mock from the spec, not real output.` It has no placeholder, so the author has nothing more
to fill in, and `--check --spec` only checks it's there. A spec with no such section (an
amendment to an older spec) gets none in its brief. On an amendment whose section changed since
the merge base, the copy is preceded by `Changed in this PR.` `--json` lists it in `sections`
like any other.

```
$ furrow brief --spec FUR-0021 --offline
## Decision

**Commits us to:** <!-- furrow: … -->
**Instead of:** <!-- furrow: … -->

## Before and after

**Before:** A spec PR's brief shows the plan, never what a user sees.
...
A mock from the spec, not real output.

## Your calls
...
```

**Skills.** `furrow-write` asks for the section, written before the steps, with the advice above
on what to mock. `furrow-implement` reads it as the target: a step's `## Behaviour` "show it"
should look like the part of the mock that step delivers, and where the code can't or shouldn't
match, the PR says so under "Spec problem" instead of quietly differing. `furrow-verify` compares
the step's real output with the mock and reports an unexplained difference as a finding.

## Before and after

**Before:** The spec PR's brief opens with Decision, then Your calls; nothing shows what a user
will see.

```
## Decision
**Commits us to:** Spec PRs show a mocked before and after.
**Instead of:** …

## Your calls
Merging accepts each proposed answer…
```

**After:** Right under Decision, the spec's own mock of what a user sees now and will see.

```
## Decision
**Commits us to:** Spec PRs show a mocked before and after.
**Instead of:** …

## Before and after
**Before:** `furrow next` lists steps one per line.
  $ furrow next
  FUR-0030.1  Add the flag (s)
**After:** each step also says why it's ready.
  $ furrow next
  FUR-0030.1  Add the flag (s)  ready: nothing to wait on
A mock from the spec, not real output.

## Your calls
…
```

## Acceptance criteria

- [ ] AC-1: Lint requires `## Before and after` in an accepted or paused spec with an unmerged step, and not in a draft, a spec with every step merged, or an area; a spec with `imported:` gets a warning instead (test: TestLintBeforeAfterRequired)
- [ ] AC-2: Lint reports a missing `**Before:**` or `**After:**` line, and either line over 20 words after its label (test: TestLintBeforeAfterLines)
- [ ] AC-3: Lint reports a side with no code block or image, accepting `Nothing yet.` for Before and `No visible effect.` for After, which also excuses Before (test: TestLintBeforeAfterShowsIt)
- [ ] AC-4: The template, its scaffold copy and both copies of the standard have the section, and a spec made by `furrow new` then set to accepted lints clean once its placeholders are filled (test: TestNewSpecHasBeforeAfter)
- [x] AC-5: `furrow brief --spec` copies the spec's section byte for byte right after `## Decision`, ends it with `A mock from the spec, not real output.`, and leaves it out when the spec has none (test: TestSpecBriefBeforeAfter)
- [x] AC-6: On an amendment whose section changed since the merge base, the copy starts with `Changed in this PR.` (test: TestSpecBriefBeforeAfterChanged)
- [ ] AC-7: `furrow-write` asks for the mock, and `furrow-implement` and `furrow-verify` treat it as the target, with a difference explained under "Spec problem" or reported as a finding (check: grep -q "Before and after" skills/furrow-write/SKILL.md && grep -q "Before and after" skills/furrow-implement/SKILL.md && grep -q "Before and after" skills/furrow-verify/SKILL.md)
- [ ] AC-8: A spec written with the skills has a mock the owner can judge on a phone without opening the spec (human: open the next spec PR written after this merges, and read its `## Before and after` on a phone)
- [x] AC-9: `furrow brief --check --spec` reports a missing `## Before and after` when the spec has one, and nothing for its contents (test: TestCheckSpecBeforeAfter)
- [ ] AC-10: `furrow import fetch|edition` writes `imported: fetch` or `imported: edition`, and its output, with no `## Before and after`, passes `furrow lint` with a warning for each spec that has an unmerged step (test: TestImportMarksSpecs)

## Out of scope

- Changing the step brief's `## Behaviour` or `--check` for step briefs. The skills tie a step's
  real output to the mock; the step brief stays as it is.
- Rendering, screenshotting or generating the mock. The author writes it by hand.
- Requiring the section in specs already in the repo, or adding it to them.
- Checking that a step's output matches the mock in code. That's a judgment for the verifier.

## Open questions

- Should the mock be required in every new spec, or only suggested? Proposed: required while a decided spec has an unmerged step, so every new spec has one and no finished spec needs one added.
- Should the step brief also copy the spec's mock into `## Behaviour`? Proposed: no; it would blow the 80-word budget, and the skills already point builders and verifiers at the spec.
- Should Before and After allow ASCII wireframes for UI, or require images? Proposed: allow both; a fenced wireframe is reviewable in plain text and needs no tooling before the code exists.

## Decisions

- 2026-11-13: Imported specs have no mock, so the rule broke `furrow import --write`. Owner chose an `imported:` marker that turns the missing section into a warning, over dropping the requirement.
- 2026-11-13: Chose to copy the mock without its surrounding blank lines and to count a mock added by an amendment as changed, since both read as the author meant.
