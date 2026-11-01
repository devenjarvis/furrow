---
id: FUR-0007
title: A Claude Code plugin that teaches agents, and swarms, to work with Furrow
status: accepted
area: agents
priority: p1
human_review: required
invariants: [INV-3]
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/scheduling.md
  - specs/areas/claims.md
  - specs/areas/lifecycle.md
steps:
  - n: 1
    title: Plugin manifest and the five skills
    after: [FUR-0002.2, FUR-0003.1, FUR-0004.1, FUR-0005.1, FUR-0006.1]
    touches:
      - .claude-plugin/**
      - skills/**
      - .claude/**
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6]
    size: m
    merged: false
---

# FUR-0007: A Claude Code plugin that teaches agents, and swarms, to work with Furrow

## Why

The binary says *what* to work on. Agents also need to know *how*: read the context, claim, test
first, stay in territory, hand back, and never sign off for a person. Fetch's four slash commands
and Edition's `next-step` skill did this per repo. One plugin does it for every repo, and adds what
neither had: an orchestrator skill that runs a swarm.

## What

A plugin at the repo root, installable with `/plugin marketplace add devenjarvis/furrow`:
`.claude-plugin/plugin.json` (`name: furrow`) and `.claude-plugin/marketplace.json` listing it with
source `./`.

Skills in `skills/<name>/SKILL.md`, each with `name` and `description` front matter. The
description says when to use it.

- **`furrow-next`**: start of a session with no task. Run `furrow next`, take the first assignment
  (or the one given), read the spec, its context, its area and the Decisions of what it comes
  after, `furrow claim` it (on exit 3, run `next` again), open a draft PR `[REF] title` listing the
  covered criteria, and write the plan.
- **`furrow-implement`**: build a claimed step. Write tests first, one per covered criterion. Stay
  inside `touches` and run `furrow territory`. Run the repo's `check`. Tick each covered criterion
  with its evidence, never a `human:` one. Run `furrow done`. On the spec's last step, fold the
  change into the area's Behaviour. Write any "Spec problem" in the PR instead of editing criteria.
- **`furrow-verify`**: review a step's PR as a skeptic. For each covered criterion, check the proof
  exists and proves what it says. Run `furrow territory --body-file` and `furrow lint`, check the
  invariants the spec lists, and report a table.
- **`furrow-write`**: turn an idea into a draft spec with `furrow new`. Pick the area, split it into
  steps with small, non-overlapping `touches`, list invariants honestly, give every criterion a
  proof, and run `furrow lint`. Leave it `draft` with the open questions for the owner.
- **`furrow-swarm`**: run N agents. Run `furrow next --agents N --json`. For each assignment, start
  an agent in its own worktree whose prompt is the `furrow-next` and `furrow-implement` flow for
  that ref. As each finishes, run `furrow-verify` on it. When steps merge, run `next` again. Stop
  when nothing is ready, and report what waits on the owner (`furrow review`, drafts, blocked steps).

`.claude/skills` in this repo is a symlink to `../skills`, so Furrow's own agents use the same
skills it ships.

No skill tells an agent to run `furrow signoff`, tick a `human:` box, or change a criterion's text.

## Acceptance criteria

- [ ] AC-1: Both manifests are valid JSON with the documented fields (check: jq -e '.name == "furrow"' .claude-plugin/plugin.json && jq -e '.plugins[0].source == "./"' .claude-plugin/marketplace.json)
- [ ] AC-2: Each of the five skills has `name` and `description` front matter (check: for s in next implement verify write swarm; do head -4 skills/furrow-$s/SKILL.md | grep -q "^name: furrow-$s"; done)
- [ ] AC-3: No skill tells an agent to sign off or tick a human box (check: ! grep -rn "furrow signoff" skills | grep -v -i "never\|don't\|not")
- [ ] AC-4: `.claude/skills` points at `skills` (check: test "$(readlink .claude/skills)" = ../skills)
- [ ] AC-5: The skills read clearly and match how the owner wants agents to work (human: read the five SKILL.md files)
- [ ] AC-6: A swarm run builds real steps of this repo in parallel without collisions (human: read the swarm's report in the PR)

## Out of scope

- Skills specific to one product (minirun proof, voice). Those stay in each repo.

## Decisions
