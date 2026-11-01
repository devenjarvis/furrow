---
id: FUR-0001
title: Read the repo's specs and lint them
status: accepted
area: specs
priority: p0
human_review: optional
invariants: []
context:
  - docs/product.md
  - specs/README.md
steps:
  - n: 1
    title: Config, spec model, glob paths, lint and list
    after: []
    touches:
      - go.mod
      - go.sum
      - Makefile
      - cmd/furrow/main.go
      - cmd/furrow/lint.go
      - cmd/furrow/list.go
      - internal/config/**
      - internal/paths/**
      - internal/spec/**
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6, AC-7]
    size: m
    merged: false
---

# FUR-0001: Read the repo's specs and lint them

## Why

Everything else Furrow does stands on reading the specs correctly and refusing ones that would
mislead an agent. Fetch and Edition each grew a linter, and the bugs they caught (unknown
dependencies, done specs with open boxes, `touches` paths that don't exist) are the ones a swarm
trips on. This is the first piece, and it is built by hand: there's no `furrow next` to pick it yet.

## What

A Go module `github.com/devenjarvis/furrow` with one dependency, `gopkg.in/yaml.v3`.

**Config** (`internal/config`). `config.Find(dir)` walks up from `dir` to the first `.furrow.yml`
and returns the repo root and the config. Missing keys take defaults:

| Key | Default |
| --- | --- |
| `prefix` | `SPEC` |
| `specs` | `specs` |
| `product` | `docs/product.md` |
| `check` | `""` |
| `remote` | `origin` |
| `base` | `main` |
| `branch` | `step/{num}-{step}` |
| `aliases` | `{}` |

With no `.furrow.yml` anywhere above, it returns `ErrNoConfig`, which prints "No .furrow.yml here
or above. Run `furrow init` to start." The branch pattern must contain `{num}` and `{step}`.

**Paths** (`internal/paths`).
- `Match(glob, path)` matches `/`-separated paths. `*` matches within a segment, `**` matches zero
  or more segments, `?` matches one character.
- `Resolve(entry, aliases)` turns a leading alias into its path (`cli/next.go` with `cli: cmd/furrow`
  → `cmd/furrow/next.go`).
- `Overlap(a, b, aliases)` reports whether two touches lists can name the same file. Two entries
  overlap when their literal prefixes (segments before the first glob character) agree up to the
  shorter one. `internal/**` overlaps `internal/spec/x.go`. `internal/spec/**` doesn't overlap
  `internal/schedule/**`. It returns the first clashing pair for the message.

**Model** (`internal/spec`). `spec.Load(root, cfg)` reads:
- `specs/changes/NNNN-*.md` and `specs/archive/NNNN-*.md`: YAML front matter (fields in
  `specs/README.md`) and the body: its `## ` sections and its acceptance criteria, which are lines
  `- [ ] AC-n: text (proof: detail)` where proof is `test`, `check` or `human` (the last such
  parenthesis on the line).
- `specs/areas/*.md`: front matter `area` and `title`, and the sections.
- the product file's invariants: every `**INV-n` under `## Invariants`.

A step ref is `PREFIX-NNNN.N`. In `after`, a bare `N` means a step of the same spec, and
`PREFIX-NNNN` means every step of that spec. A step is merged when `merged: true`. Every step of an
archived spec counts as merged.

**Lint** (`furrow lint`). It prints one `error: <file>: <message>` line per problem to stderr, then
`N problem(s) in M specs.` and exits 1. Warnings print `warning: …` and don't fail. With no errors
it prints `M specs OK, K open steps.` and exits 0. Errors:

1. Front matter missing or unreadable; `id` isn't `PREFIX-` plus the file's number; duplicate ids
   across changes and archive.
2. `status` isn't `draft`, `accepted` or `paused` in `changes/`, or isn't `done` in `archive/`.
   `priority` isn't p0, p1 or p2. `human_review` isn't required or optional.
3. `area` is empty or has no `specs/areas/<area>.md`.
4. A required section is missing: Why, What, Acceptance criteria, Out of scope.
5. No criteria; criteria not numbered AC-1, AC-2, … in order; a criterion without a proof.
6. Accepted or paused: no steps; steps not numbered 1, 2, … in order; an unmerged, unblocked step
   with no `touches`; a step that covers an unknown criterion; a criterion no step covers; `size: l`;
   the word `TBD` in the body.
7. An `after` ref that doesn't exist, points at itself, or forms a cycle (with the cycle printed).
   A merged step whose `after` isn't merged.
8. An unmerged step of an accepted spec has a `touches` entry whose folder doesn't exist (the
   literal prefix minus its last segment, after aliases). New files in existing folders are fine.
9. An invariant in `invariants:` that the product file doesn't define. A spec with invariants that
   isn't `human_review: required`.
10. A ticked `human:` criterion without a `## Decisions` line containing `<AC-n> signed off by`.
11. A `done` spec with an unmerged step or an unticked criterion.
12. An area file whose `area` isn't its file name, or that lacks `## Behaviour` or `## History`.
    Duplicate invariant ids.

Warning: an accepted spec with every step merged and every box ticked: "ready to archive: run
`furrow archive <id>`".

**List** (`furrow list [--json]`). One line per spec in changes and archive, in id order:
`id  status  merged/total steps  title`. The status is worked out: `draft`, `accepted` (nothing
merged), `building` (some merged), `review` (every step merged, `human:` boxes open), `paused`,
`done`. `--json` prints an array of `{id, title, status, area, file, steps_merged, steps_total}`.

**CLI** (`cmd/furrow/main.go`). Each command lives in its own file and registers itself in an
`init()` function with its name, a one-line summary and a run function. So later specs add commands
without touching `main.go`. `furrow` and `furrow help` list the commands. An unknown command exits 2.
`furrow version` prints the version.

## Acceptance criteria

- [ ] AC-1: Config is found by walking up, missing keys take the defaults, and with no file the error says to run `furrow init` (test: TestFindConfig)
- [ ] AC-2: Changes, archive, areas and invariants load, including `after` refs in all three forms (test: TestLoadRepo)
- [ ] AC-3: Each lint rule 1–12 has a case that triggers exactly its error, and the clean repo passes (test: TestLintRules)
- [ ] AC-4: `Match` handles `*`, `**` and `?`, and `Overlap` follows the literal-prefix rule with aliases (test: TestMatch, TestOverlap)
- [ ] AC-5: `furrow list` works out building and review, and `--json` has the listed fields (test: TestListStatus)
- [ ] AC-6: Furrow's own specs pass (check: go run ./cmd/furrow lint)
- [ ] AC-7: The only module dependency is yaml.v3 (check: go list -m all)

## Out of scope

- Scheduling, claims and every other command. Each has its own spec.
- Line numbers in lint messages.

## Decisions
