---
id: FUR-0009
title: Move Fetch and Edition onto Furrow
status: accepted
area: scaffolding
priority: p1
human_review: required
invariants: [INV-3, INV-4]
context:
  - docs/product.md
  - specs/README.md
  - specs/changes/0015-signoff-from-anywhere.md
  - specs/changes/0016-new-folders-in-touches.md
steps:
  - n: 1
    title: furrow import fetch
    after: [FUR-0006.1, FUR-0016.1]
    touches:
      - internal/importer/**
      - cmd/furrow/import.go
    covers: [AC-1, AC-2, AC-5, AC-6, AC-7, AC-8, AC-9]
    size: m
    merged: true
  - n: 2
    title: furrow import edition
    after: [1]
    touches:
      - internal/importer/**
      - cmd/furrow/import.go
    covers: [AC-3, AC-4]
    size: m
    merged: false
    blocked: "Edition's move isn't planned yet. The owner unblocks it."
---

# FUR-0009: Move Fetch and Edition onto Furrow

## Why

Furrow came out of these two repos. Moving them over proves it works on real, messy history, and
it removes two tools that have drifted apart: Fetch's `scripts/specs.mjs` and Edition's
`scripts/specs`. Fetch goes first. It has 119 specs, ready ones waiting on each other, and 40 done
specs with `human:` boxes a person already checked.

## What

**`furrow import fetch [--areas FILE] [--by NAME] [--write] [--json]`** runs at the root of a repo
whose `.furrow.yml` already exists (else: "No .furrow.yml here. Write one first: `furrow init
--prefix FETCH`."). It reads every `specs/NNNN-*.md` in Fetch's format, the files `scripts/specs.mjs
lint` checks, and plans the Furrow tree from them. Without `--write` it's a dry run: it prints the
plan and writes nothing. `--json` prints the plan as
`{"specs": [{id, from, to, status, area, merged, signoffs}], "areas": [...], "problems": [...]}`.

Each spec becomes a one-step spec:

| Fetch | Furrow |
| --- | --- |
| `status: done` | `status: done`, moved to `specs/archive/`, step `merged: true` |
| `status: ready` | `status: accepted` in `specs/changes/`. Step `merged: true` when every criterion but the `human:` ones is ticked, else `false` |
| `status: draft` | `status: draft` in `specs/changes/`, step `merged: false` |
| `status: blocked` | `status: paused` in `specs/changes/`, step `merged: false` |
| `depends_on: [FETCH-0005]` | the step's `after: [FETCH-0005]` (the whole spec) |
| `size` | the step's `size` |
| `touches` | the step's `touches`, rewritten (below) |
| every `AC-n` | the step's `covers` |
| `title` | the spec's and the step's title |
| `id`, `priority`, `human_review` | kept |
| `context` | kept, with each `specs/NNNN-slug.md` entry (and any `#anchor` after it) pointing to that spec's new path |
| `kind` | kept as it is. Furrow ignores keys it doesn't know |
| (none) | `area` from the areas file, `invariants: []` |

The body is kept byte for byte, `## Verification` and `## Design` included, with three exceptions:

- **Links.** `specs/NNNN-slug.md`, and `NNNN-slug.md` or `./NNNN-slug.md` inside a spec, become the
  file's new path (`specs/archive/0005-gh-bridge-and-notifications.md`, or the relative
  `../archive/…` from inside `changes/`).
- **Imported sign-offs.** Each ticked `human:` criterion gets
  `- YYYY-MM-DD: AC-n signed off by NAME (imported).` appended to `## Decisions`, in AC order, with
  the import's date (from the injected clock) and NAME from `--by`. A spec with no `## Decisions`
  gets one at the end. With a ticked `human:` box anywhere and no `--by`, it refuses: "Say who
  signed off the imported human: checks: --by NAME."
- Nothing else. Criteria text is never changed (INV-4).

**Touches.** An entry that matches the spec's own file is dropped, since `furrow territory`
already allows a step's own spec. An entry that names another spec's file points to where that
spec is moving. Every other entry, `specs/README.md` and globs like `specs/**` included, is kept.

**Areas.** `--areas FILE` is YAML:

```yaml
areas:
  - area: line
    title: Your line
    paths: [Packages/FetchCore/Sources/FetchCore/Line/**, App/Sources/Features/Line/**]
specs:            # optional: spec ids whose area the paths don't decide
  FETCH-0089: line
```

A spec's area is its entry under `specs:`, else the area that covers its first `touches` entry
that any area covers, trying areas in file order. An area covers an entry when one of its paths'
literal prefix (the segments before the first wildcard) is a prefix of the entry's:
`Packages/FetchCore/Sources/FetchCore/Line/**` covers `…/Line/Queue/**`, and no area covers a
broad `Packages/FetchCore/**`, so a later, narrower entry decides. The importer refuses to
guess: with no `--areas` it says "Name the areas: --areas FILE (see `furrow import --help`)", and
each spec that matches no area is a problem, `FETCH-0033 matches no area. Add one of its touches
to an area's paths, or list it under specs:`. For each area it appends
`- FETCH-NNNN: <title> (archived YYYY-MM-DD, imported)` to `specs/areas/<area>.md`'s `## History`
for each archived spec in that area, in number order. An area file that exists is kept, and only
those lines are added. A missing one is written with its front matter and title, a `## Behaviour`
that says `To write: how <title> works now.`, and the `## History`.

**The rest of the tree.** It writes `specs/README.md` and `specs/_template.md` from Furrow's own
(as `furrow init` does, with the prefix filled in), replacing Fetch's, and a `.gitkeep` in
`changes/`, `archive/` and `areas/`. Old files are moved with plain file operations, never by
running git.

**Problems.** A `done` spec with an unticked box, a spec whose front matter doesn't parse, an
unmatched area, and anything `furrow lint` would report on the planned tree are listed as
problems. The dry run prints them and exits 1 when there are any. `--write` refuses, writing
nothing, while there are any: fix the source specs, in Fetch's format, first.

**Edition** (step 2, blocked until the owner plans it). Each `docs/specs/<feature>.md` keeps its
prose. The `- **Status:**` line becomes front matter, and the numbered steps with their
`**After:** · **Touches:**` lines become `steps:`. Criteria `X1…` are renamed `AC-1…`, with a map
in the spec's Decisions so tests named after them can still be found. Shipped specs go to
`archive/`. It has the same dry run, `--write` and problems as Fetch.

## Acceptance criteria

- [x] AC-1: Importing a fixture copy of Fetch's specs (each status, a dependency chain, a `kind`) gives a tree `furrow lint` passes, with done specs in `archive/`, merged (test: TestImportFetch)
- [x] AC-2: A ready spec whose only open boxes are `human:` ones comes out accepted and merged, and its boxes are listed by `furrow review` (test: TestImportFetchSignoffQueue)
- [ ] AC-3: Importing a copy of Edition's specs gives a lint-clean tree, and its step lines become steps (test: TestImportEdition)
- [ ] AC-4: `furrow next` on the imported Edition tree offers the same steps as `scripts/next --offline` (check: compare the two outputs)
- [x] AC-5: An entry in touches naming the spec's own file is dropped, one naming another spec follows it, and `context` entries and links in bodies point to the new paths (test: TestImportFetchRewritesPaths)
- [x] AC-6: Missing `--areas`, or a spec no area matches, is refused with the message naming the spec, and nothing is written. An existing area file keeps its text and gains only History lines (test: TestImportFetchAreas)
- [x] AC-7: Each ticked `human:` box gets one "signed off by NAME (imported)" line, and without `--by` the import refuses (test: TestImportFetchSignoffLines)
- [x] AC-8: The dry run writes nothing and prints the plan, and `--write` refuses while the planned tree has problems (test: TestImportFetchDryRun)
- [x] AC-9: A dry run on devenjarvis/fetch reads right to the owner (human: run `furrow import fetch --areas … --by …` in a Fetch clone and skim the plan)

## Out of scope

- Opening PRs on Fetch or Edition. A session in each repo does that, with a spec there (Fetch:
  FETCH-0130).
- Writing the areas' `## Behaviour`. That's the repo's own work, after the import.
- Seeing open PRs or branches. Furrow reads files and git only (INV-2), so the repo merges or
  closes its spec PRs before it imports.
- Splitting imported specs into several steps.

## Open questions

- Should Edition's criteria keep their letters (`E12`) instead of `AC-n`? Proposed: let the config
  allow a criterion pattern, and keep the letters. Decided when step 2 is unblocked.
- Should the importer live in the binary or be a one-off script? Proposed: the binary, behind
  `furrow import`, because other repos will want the same.
- Are "(imported)" sign-off lines allowed under INV-3? Proposed: yes. The import's PR is a
  sign-off PR in the sense of FUR-0015: a person checked these boxes under Fetch's old process,
  and merging the import PR records that. `--by` names that person.
- Should the importer guess `invariants` for open specs? Proposed: no. It leaves `[]`, and the
  repo adds them in a spec PR, because guessing an invariant is exactly what a person should do.
- Should a ready spec with an open PR still come out merged when its non-human boxes are ticked?
  Proposed: yes. The importer can't see PRs (INV-2), so the repo finishes or closes those PRs
  before importing.

## Decisions

- 2026-11-03: Fetch goes first and Edition's step waits, blocked, because the owner is moving
  Fetch now and hasn't planned Edition's move.
- 2026-11-03: `context` entries that name a spec are rewritten too: Fetch's specs list other specs
  there (FETCH-0121 lists three), and lint doesn't check that context paths exist, so they'd
  break silently.
- 2026-11-03: Link rewriting skips criterion lines, since INV-4 outranks the links rule; a criterion naming a spec path keeps its old text.
- 2026-11-03: Relative links to a spec in the same folder stay as written; ones that cross folders become `../archive/…` or `../changes/…`.
- 2026-11-03: Lint of the planned tree runs in a temp folder holding the repo's top-level folders and product file, so touches checks still see them. Only errors count as problems.
- 2026-11-03: Furrow's README and template come from `scaffold.Init` run in a temp folder, because the assets aren't exported and scaffold is outside this step.
- 2026-11-03: Every area in the areas file gets a file, even one with no archived specs yet, so lint finds each area a spec names.
- 2026-11-03: AC-9 signed off by Deven Jarvis.
