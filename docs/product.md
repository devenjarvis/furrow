# Furrow

Furrow lets a swarm of coding agents build one repo at the same time without stepping on each
other. Work is written down as **specs** in plain Markdown. Each spec is split into **steps**, and
each step is one pull request. Every step declares what it waits for (`after`) and which paths it may
change (`touches`). From that, Furrow works out what can start now and which steps can run side by
side, and it hands each agent its own row to plough.

It's a small Go binary plus a Claude Code plugin. It works in any repo, in any language.

## Who it's for

One owner, many agents. The owner decides *what* gets built, by merging specs, and judges what
only a person can judge. Agents do everything else: pick a step, claim it, build it, prove it, and
hand it back.

## The loop

1. **Write** a spec (`furrow new`, or the `furrow-write` skill). It starts as `draft`.
2. **Accept** it. Merging the spec PR with `status: accepted` is the decision.
3. **Pick:** `furrow next --agents N` hands out up to N steps whose `after` is merged and whose
   `touches` don't overlap each other or any claimed step.
4. **Claim:** `furrow claim FUR-0007.2` creates the step's branch on the remote. Creating a ref is
   atomic, so exactly one agent wins.
5. **Build** inside `touches`, write a test per criterion, and run the repo's check.
   `furrow territory` proves the diff stayed inside.
6. **Hand back:** `furrow done FUR-0007.2` marks the step merged in the same PR, so merging the PR
   makes it true. Steps that waited on it unblock at once.
7. **Sign off:** criteria that a person must judge (`human:`) wait in `furrow review`. The owner
   runs `furrow signoff`. When nothing is left, the spec is archived and what it changed lives on
   in its area's living spec.

## Words

- **Spec**: one decision about a change, in `specs/changes/NNNN-slug.md`.
- **Step**: one PR's worth of a spec, with `after`, `touches` and the criteria it `covers`.
- **Area**: a living spec in `specs/areas/<area>.md`, saying how one part of the product works
  *now*. Changes are temporary. Areas are permanent.
- **Archive**: finished specs in `specs/archive/`, kept as the record of why.
- **Claim**: a branch on the remote named after the step (`step/0007-2` by default).
- **Invariant**: a rule in a product's `docs/product.md` that is never broken. Specs that touch
  one say so, and a person reviews them.

## Invariants

These are never broken. A spec that touches one lists it under `invariants:`, and that makes it
`human_review: required`.

- **INV-1: Plain files are the truth.** Everything Furrow knows is in the repo's Markdown files
  and the remote's branches. No server, no database, no hidden state, no cache that can disagree.
- **INV-2: Git is enough.** Every command works with only `git` and a remote. A forge's API (GitHub
  or another) can add detail, but nothing depends on it.
- **INV-3: Agents never sign off for a person.** A `human:` criterion is ticked only through
  `furrow signoff`, which refuses to run inside an agent session, and every ticked `human:`
  criterion has a sign-off line naming who checked it.
- **INV-4: Criteria aren't bent.** Furrow never rewrites the text of an acceptance criterion. It
  only ticks boxes. A criterion that's wrong is changed by a person, in a spec PR.
- **INV-5: One step, one owner.** At most one claim exists per step, and claiming is atomic.
- **INV-6: Same input, same answer.** Output is deterministic and every listing has `--json`, so
  agents and scripts never parse prose.
- **INV-7: Nothing to install but the binary.** One static binary with no runtime. It may shell
  out to `git` and nothing else.

## Not Furrow's job

- Writing the code, or running the agents' sessions. Skills tell agents how. Furrow only tells
  them what.
- Hosting: issues, boards and dashboards. The files are the board.
- Replacing a spec-writing workflow such as Spec Kit or OpenSpec. Furrow schedules work; a step's
  author can plan however they like.
