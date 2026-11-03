---
area: agents
title: Agents
---

# Agents

The Claude Code plugin: the skills that teach an agent, or a swarm of them, to work with Furrow.

## Behaviour

Furrow ships a Claude Code plugin from the repo root. `.claude-plugin/plugin.json` names it
`furrow`, and `.claude-plugin/marketplace.json` lists it with source `./`. Install it with
`/plugin marketplace add devenjarvis/furrow`. Its skills live in `skills/`, and this repo's
`.claude/skills` links there, so Furrow's own agents use what it ships.

- **furrow-next**: pick the top ready step (or the one given), read the spec, its context, its area
  and the Decisions of what it comes after, `furrow claim` it (exit 3: pick again), open the draft
  PR `[REF] title` with `furrow brief`'s output as its body (the brief is the template; no other),
  and plan one test per covered criterion.
- **furrow-implement**: tests first, stay inside `touches` (`furrow territory`), run the repo's
  check, tick covered criteria with evidence, and log decisions. Then it keeps the spec's diagram
  current, captures visual proof before and after with the repo's own tools when the brief has a
  Visual proof section (a screenshot is evidence for the owner, never a reason to tick a `human:`
  box), and fills every brief placeholder for an owner on a phone: Before and After as a user or
  caller sees them, at most 20 words each; one "show it" (a command and its output, a screenshot,
  or a call and its result) as an excerpt of at most 15 lines; at most 3 Review this items, each a
  place in the code and the question to ask; and how the diff touches each invariant. It writes
  each `## Decisions` line in at most 30 words. `furrow brief --check` and `furrow territory --body-file` on the description must
  both exit 0 before `furrow done`. On a spec's last step it folds the change, and the diagram, into
  the area's Behaviour. Problems go under "Spec problem", never into the criteria.
- **furrow-verify**: a skeptic's review: every covered criterion's proof, territory with the PR
  body, the brief (`furrow brief --check`, then Before and After, each diagram node and edge, each
  image, the Judgment calls and the invariant lines read against the diff; it re-runs a cheap
  "show it" command and compares the output, and checks each Review this item names real code; a
  claim the diff doesn't bear out is a finding),
  invariants, spec hygiene, the repo's check, and a verdict table.
- **furrow-write**: `furrow new`, then steps with small non-overlapping touches, honest `after`
  and invariants, a proof for every criterion, lint and graph. A spec that changes structure draws
  it in `## What` as a mermaid diagram to the standard's rules. Every open question gets
  `Proposed:`. It opens the spec's PR with `furrow brief --spec` (regenerated with `--pr N`), fills
  the Decision's `**Commits us to:**` and `**Instead of:**` lines, at most 25 words each, runs
  `--check`, and opens one review thread per question. When the owner overrules
  one, it moves the question to `## Decisions` as an `Owner chose … over the proposed …` line and
  regenerates the brief; if a `Qn` is ambiguous it asks. On an amendment, Your calls lists only
  new questions: ones decided when the spec merged aren't asked again. It never merges or accepts a spec.
- **furrow-swarm**: `furrow next --agents N --json`, one worktree and background agent per
  assignment, verify what returns, merge what passes, hand out newly unblocked steps as slots free
  up, archive finished specs, and report what waits on the owner.

No skill tells an agent to sign off, tick a `human:` box, accept a spec, or change a criterion's
text.

## History
