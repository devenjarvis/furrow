---
id: FUR-0009
title: Move Fetch and Edition onto Furrow
status: draft
area: scaffolding
priority: p1
human_review: required
invariants: [INV-4]
context:
  - docs/product.md
  - specs/README.md
steps:
  - n: 1
    title: furrow import fetch
    after: [FUR-0006.1]
    touches:
      - internal/importer/**
      - cmd/furrow/import.go
    covers: [AC-1, AC-2]
    size: m
    merged: false
  - n: 2
    title: furrow import edition
    after: [1]
    touches:
      - internal/importer/**
      - cmd/furrow/import.go
    covers: [AC-3, AC-4]
    size: m
    merged: false
---

# FUR-0009: Move Fetch and Edition onto Furrow

## Why

Furrow came out of these two repos. Moving them over proves it works on real, messy history, and
it removes two diverging tools: Fetch's `scripts/specs.mjs` and Edition's `scripts/specs`.

## What

**Fetch.** Each `specs/NNNN-*.md` becomes a one-step spec. `status: done` goes to `archive/`,
`ready` becomes `accepted`, and draft and blocked map to draft and paused. `depends_on` becomes
the step's `after`, `touches` stays as it is, and every criterion is covered by step 1. Every done
spec's step is `merged: true`, and ready specs with every non-human box ticked are merged too.
That frees the eleven specs that waited on sign-off. Ticked `human:` criteria get a sign-off line
"signed off by Deven (imported)". Areas come from the top-level `touches` folders. The owner names
them in a mapping file, and the importer refuses to guess.

**Edition.** Each `docs/specs/<feature>.md` keeps its prose. The `- **Status:**` line becomes
front matter, and the numbered steps with their `**After:** · **Touches:**` lines become `steps:`.
Criteria `X1…` are renamed `AC-1…`, with a map in the spec's Decisions so tests named after them
can still be found. Shipped specs go to `archive/`.

Both are dry runs by default: they print what they'd write. `--write` writes it, and the result
passes `furrow lint`.

## Acceptance criteria

- [ ] AC-1: Importing a copy of Fetch's specs gives a lint-clean tree, with done specs archived (test: TestImportFetch)
- [ ] AC-2: Specs that only waited on sign-off come out merged, with their human boxes in `furrow review` (test: TestImportFetchSignoffQueue)
- [ ] AC-3: Importing a copy of Edition's specs gives a lint-clean tree, and its step lines become steps (test: TestImportEdition)
- [ ] AC-4: `furrow next` on the imported Edition tree offers the same steps as `scripts/next --offline` (check: compare the two outputs)

## Out of scope

- Opening PRs on Fetch or Edition. A separate session in each repo does that, with a spec there.

## Open questions

- Should Edition's criteria keep their letters (`E12`) instead of `AC-n`? Proposed: let the
  config allow a criterion pattern, and keep the letters.
- Should the importer live in the binary or be a one-off script? Proposed: the binary, behind
  `furrow import`, because other repos will want the same.

## Decisions
