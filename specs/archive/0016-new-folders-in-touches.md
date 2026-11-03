---
id: FUR-0016
title: A step may touch folders it creates
status: done
area: specs
priority: p1
human_review: optional
invariants: []
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/specs.md
  - internal/spec/lint.go
steps:
  - n: 1
    title: Lint checks only the top-level folder of each touches entry
    after: []
    touches:
      - internal/spec/lint.go
      - internal/spec/spec_test.go
      - internal/spec/spectest/**
      - specs/areas/specs.md
    covers: [AC-1, AC-2, AC-3]
    size: s
    merged: true
---

# FUR-0016: A step may touch folders it creates

## Why

Lint rule 8 says every folder a step of an accepted spec touches must already exist. That catches
typos, but it also refuses the most common kind of step: one that creates a new module. Fetch has
28 `touches` entries in ready specs like `Packages/FetchCore/Sources/FetchCore/Line/Queue/**`,
where `Line/` doesn't exist until the step builds it, so Fetch can't be imported (FUR-0009) while
the rule stands.

## What

**Rule 8 becomes "the top-level folder exists".** For each open step of an accepted spec, and each
`touches` entry after aliases are resolved, lint takes the entry's literal segments (those before
the first one with `*`, `?`, `[` or `{`). When there are at least two, the first segment must be a
folder in the repo. Deeper folders may be new. An entry with one literal segment (`README.md`,
`Makefile`, or `**` alone) isn't checked, as today.

| Entry | Repo has | Today | After |
| --- | --- | --- | --- |
| `Packages/FetchCore/Sources/FetchCore/Line/Queue/**` | `Packages/` but no `Line/` | error | passes |
| `Pakages/FetchCore/**` | no `Pakages/` | error | error |
| `skills/furrow-signoff/SKILL.md` | `skills/` | error | passes |

The message stays: `step 1 touches "Pakages/FetchCore/**", but the folder Pakages doesn't exist`.

**`specs/areas/specs.md`** says "`touches` whose top-level folder exists" where it now says
"`touches` folders that exist", in the same commit (this is the spec's only step).

## Acceptance criteria

- [x] AC-1: An accepted spec whose step touches a new folder under an existing top-level folder passes lint (test: TestLintTouchesNewFolder)
- [x] AC-2: A missing top-level folder still fails lint with the same message (test: TestLintRules)
- [x] AC-3: The specs area describes the rule as it now works (check: grep -q "top-level folder" specs/areas/specs.md)

## Out of scope

- Checking that a step's new folders are created by the time it's merged. `furrow territory` and
  review already see the diff.
- Warnings for new folders. A new folder is normal, not a smell.

## Open questions

- Should a step be able to create a new top-level folder? Proposed: not without a trace. The spec
  PR adds the folder with a `.gitkeep`, which keeps the typo check meaningful.

## Decisions

- 2026-11-03: Chose to cut literal segments at `{` inside lint.go, because `paths.Literal` stops only at `*?[` and `internal/paths` is outside this step.
- 2026-11-03: Chose to quote `**`, `{…}` and comma items in spectest's YAML lists, because unquoted they aren't valid flow-list YAML.
