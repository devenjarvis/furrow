---
id: FUR-0006
title: Start Furrow in a repo, and start new specs and areas
status: done
area: scaffolding
priority: p1
human_review: optional
invariants: []
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/specs.md
steps:
  - n: 1
    title: init, new, area and invariants
    after: [FUR-0001]
    touches:
      - internal/scaffold/**
      - cmd/furrow/scaffold.go
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5]
    size: m
    merged: true
---

# FUR-0006: Start Furrow in a repo, and start new specs and areas

## Why

Furrow is meant for any repo the owner builds with agents. Starting it should take one command, and
starting a spec shouldn't involve copying a template and guessing the next number.

## What

**`furrow init [--prefix P]`** writes, without overwriting anything that exists:
- `.furrow.yml` with the defaults and the prefix (default: the repo folder's name, upper-cased,
  letters only, at most 6, else `SPEC`);
- `specs/README.md` (the standard, as in this repo, with the prefix filled in) and
  `specs/_template.md`;
- `specs/changes/`, `specs/areas/` and `specs/archive/`, each with a `.gitkeep`;
- `docs/product.md` with `# <Repo>`, a short placeholder, and an `## Invariants` section holding
  one example `**INV-1: …**` line, only if the file doesn't exist. If it exists without an
  `## Invariants` section, it says to add one.

It prints each path as `created` or `kept`. The files come from `go:embed`.

**`furrow new <slug> --title T --area A [--json]`** creates `specs/changes/NNNN-<slug>.md`. NNNN
is one more than the highest number in changes and archive. It starts from the repo's
`specs/_template.md`, or the built-in one, filling in the id, title and area, with `status: draft`.
It refuses a slug that isn't lowercase letters, digits and dashes, and an area that doesn't exist:
"No area 'x'. Create it with `furrow area x --title …`." It prints the new path.

**`furrow area <name> --title T`** creates `specs/areas/<name>.md` with the front matter, a
one-line description placeholder, `## Behaviour` and `## History`. It refuses an existing area.

**`furrow invariants [--json]`** prints each invariant from the product file as
`INV-n  <name>`. `--json` prints `[{"id", "text"}]`.

Every file it writes passes `furrow lint`.

## Acceptance criteria

- [x] AC-1: `init` in an empty git repo creates every listed path, and a second run keeps them all (test: TestInit)
- [x] AC-2: `new` picks the next number across changes and archive, and the file passes lint once its area exists (test: TestNew)
- [x] AC-3: `new` refuses a bad slug and an unknown area with the messages (test: TestNewRefusals)
- [x] AC-4: `area` creates a lint-clean area and refuses a duplicate (test: TestArea)
- [x] AC-5: `invariants` lists this repo's seven invariants (test: TestInvariants)

## Out of scope

- Converting another repo's specs. That's FUR-0009.

## Decisions

- 2026-11-01: `init` treats a folder name with no ASCII letters as having none, so the prefix is `SPEC`; `--prefix` must be uppercase letters and digits starting with a letter.
- 2026-11-01: When `.furrow.yml` already exists, `init` keeps it and uses its prefix, `specs` and `product` paths for the other files; `--prefix` then has no effect.
- 2026-11-01: `init` writes every `.furrow.yml` key with its default (`check: ""` with a comment) so the owner sees what can be set.
- 2026-11-01: `new` numbers from file names (`NNNN-*.md`) in changes and archive, so a spec that fails to parse still holds its number. It fills the top-level `id`, `title`, `status` and `area` lines and the first `# ` heading of the template, adding any key the template lacks; titles are YAML-quoted when needed.
- 2026-11-01: `new` and `area` require `--title`; `area` refuses a name that isn't lowercase letters, digits and dashes, like a slug.
- 2026-11-01: `invariants` fails when the product file is missing, and prints a hint (exit 0) when it has no invariants. AC-1 is tested in a temp folder with a `.git` folder, since only `internal/gitx` may run git.
