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
  PR `[REF] title`, and plan one test per covered criterion.
- **furrow-implement**: tests first, stay inside `touches` (`furrow territory`), run the repo's
  check, tick covered criteria with evidence, `furrow done`, and on a spec's last step fold the change
  into the area's Behaviour. Problems go under "Spec problem", never into the criteria.
- **furrow-verify**: a skeptic's review: every covered criterion's proof, territory with the PR
  body, invariants, spec hygiene, the repo's check, and a verdict table.
- **furrow-write**: `furrow new`, then steps with small non-overlapping touches, honest `after`
  and invariants, a proof for every criterion, lint and graph. It stays a draft for the owner.
- **furrow-swarm**: `furrow next --agents N --json`, one worktree and background agent per
  assignment, verify what returns, merge what passes, hand out newly unblocked steps as slots free
  up, archive finished specs, and report what waits on the owner.

No skill tells an agent to sign off, tick a `human:` box, accept a spec, or change a criterion's
text.

## History
