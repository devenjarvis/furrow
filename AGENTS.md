# Working on Furrow

Furrow is built with Furrow. Read `docs/product.md` once: its **invariants** are rules you never
break. Then read `specs/README.md`, the standard every change follows.

## The one rule

**No spec, no code.** Every change implements a step of an accepted spec in `specs/changes/`.

```sh
go run ./cmd/furrow next              # what you can start now
go run ./cmd/furrow next --agents 4   # four steps that can run side by side
go run ./cmd/furrow claim FUR-0004.1  # take one: creates step/0004-1 on origin and checks it out
go run ./cmd/furrow territory         # prove your diff stayed inside the step's touches
go run ./cmd/furrow done FUR-0004.1   # hand it back, in the same PR
```

PR title: `[FUR-NNNN.N] <step title>`. Finish by ticking each covered criterion with evidence
(never a `human:` one), running `make check`, and `furrow done`. On a spec's last step, fold the
change into `specs/areas/<area>.md`.

If the spec is wrong, don't bend the criteria to fit your code. Say so under "Spec problem" in the
PR.

## Repo map

```
cmd/furrow/        the CLI: main.go dispatches, and each command registers itself from its own file
internal/config/   .furrow.yml
internal/paths/    globs, aliases and overlap
internal/spec/     the model: changes, areas, archive, invariants; lint
internal/schedule/ ready, waiting, ranking, picking steps that can run together, waves
internal/gitx/     the only place that runs git
internal/claim/    claim branches on the remote
internal/territory/ diff against touches
internal/lifecycle/ done, review, signoff, archive (text edits that touch one line)
internal/scaffold/ init, new, area, invariants
skills/            the Claude Code plugin's skills (.claude/skills links here)
specs/             the work
```

## How we write code

- **Go 1.24, standard library, plus `gopkg.in/yaml.v3`.** Nothing else (INV-7). Only `internal/gitx`
  runs a process, and the only process is `git`.
- **One command, one file** in `cmd/furrow/`, registered in `init()`. That keeps steps from
  overlapping.
- **Tests use real git in temp dirs** (`t.TempDir()`, a bare repo as the remote). No network.
- **Output is deterministic.** Sort everything. Every listing has `--json`. Dates come from an
  injected clock.
- **Messages are short and say what to do next.** "FUR-0003.1 is already claimed (step/0003-1). Run
  `furrow next` for another step."
- **Edit spec files surgically.** Change the one line you mean to and nothing else.

## Commands

| Command | What it does |
| --- | --- |
| `make check` | vet, test, build, and `furrow lint` on this repo |
| `make test` | `go test ./...` |
| `make build` | builds `bin/furrow` |

## Never

- Sign off a `human:` criterion, or edit a criterion's text to make it pass (INV-3, INV-4).
- Add a dependency, or shell out to anything but `git`.
- Work outside your step's `touches` without explaining it under "## Territory" in the PR.
