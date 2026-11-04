---
area: scaffolding
title: Scaffolding
---

# Scaffolding

Starting Furrow in a repo and starting new specs: `furrow init`, `furrow import`, `furrow new`, `furrow area` and `furrow invariants`.

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

**`furrow import fetch|edition --areas FILE [--by NAME] [--write] [--json]`** moves a repo's specs
onto Furrow from the format it used before, at the root of a repo whose `.furrow.yml` exists.
Without `--write` it's a dry run: it prints each spec's id, status, area and new path, the area
files it creates or adds `## History` lines to, and its problems, writes nothing, and exits 1 when
there are problems. `--write` refuses while there are any. Specs that are done go to `archive/`
with every step merged; the rest go to `changes/`. The areas file names each area, its title and
the paths it covers, and may map specs to areas under `specs:`; a spec no area matches is a
problem. It writes Furrow's `specs/README.md` and `specs/_template.md`, and moves files with plain
file operations, never git.

- `fetch` reads `specs/NNNN-*.md` with Fetch's front matter. Each spec becomes one step, its
  `depends_on` the step's `after`. A ticked `human:` box gets an "(imported)" sign-off line from
  `--by`.
- `edition` reads `docs/specs/<feature>.md`, numbered in file-name order. The `**Status:**` line
  becomes front matter (Shipped is done; Building and Accepted are accepted; a Draft "after 1.0"
  is p2), and the area is the `specs:` entry, else the `**Area:**` line's first word, else the
  touches. Goal becomes `## Why`; Screens, Data and the step list go under `## What`; Behavior's
  `**S5.**` criteria become `- [x] AC-5: S5. … (test: s5…)`, ticked when every step naming them is
  merged; answered open questions and "Changes since accepted" become `## Decisions`, with a line
  mapping the letters to AC numbers. Each step line's `After` (`N`, `<spec> N`) becomes `after`,
  `Touches` a package alias or a path (a folder gets `/**`), and `Blocked` stays; a paused spec's
  step with no step line is blocked by the pause.

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
