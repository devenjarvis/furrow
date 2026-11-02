---
area: scaffolding
title: Scaffolding
---

# Scaffolding

Starting Furrow in a repo and starting new specs: `furrow init`, `furrow new`, `furrow area` and `furrow invariants`.

## Behaviour

**`furrow init [--prefix P]`** starts Furrow in the current folder. It writes `.furrow.yml` (every
key with its default, the prefix, and a commented `# visual: []` line saying what it does),
`specs/README.md` (the spec standard, with its diagram rules, and the prefix filled in),
`specs/_template.md`, a `.gitkeep` in `specs/changes/`, `specs/areas/` and `specs/archive/`,
and `docs/product.md` (`# <folder>`, a placeholder and an `## Invariants` section with one example
`**INV-1: …**` line). It never overwrites anything: each path prints as `created <path>` or
`kept <path>`, always in that order. The default prefix is the folder's name upper-cased, letters
only, at most 6, else `SPEC`. If `.furrow.yml` exists, its settings win over `--prefix`. If the
product file exists without an `## Invariants` section, init says to add one. The files come from
`go:embed` in `internal/scaffold/assets/`.

**`furrow new <slug> --title T --area A [--json]`** writes `specs/changes/NNNN-<slug>.md`, where NNNN
is one more than the highest number among file names in changes and archive. It starts from the
repo's `specs/_template.md`, or the built-in one, and fills in `id`, `title`, `area`,
`status: draft` and the `# <id>: <title>` heading. It refuses a slug that isn't lowercase letters,
digits and dashes, and an unknown area: "No area 'x'. Create it with `furrow area x --title …`." It
prints the new path, or `{"id", "title", "area", "file"}` with `--json`.

**`furrow area <name> --title T`** writes `specs/areas/<name>.md` with its front matter, a one-line
placeholder, `## Behaviour` and `## History`, and prints the path. It refuses an area that exists.

**`furrow invariants [--json]`** prints each invariant in the product file as `INV-n  <name>`, or
`[{"id", "text"}]` with `--json`.

Every file these commands write passes `furrow lint`.

## History

- FUR-0006: Start Furrow in a repo, and start new specs and areas (archived 2026-11-01)
