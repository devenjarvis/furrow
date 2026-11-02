---
id: FUR-0012
title: Merging a spec's PR accepts it
status: accepted
area: specs
priority: p1
human_review: required
invariants: [INV-1]
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/specs.md
  - internal/spec/lint.go
  - skills/furrow-write/SKILL.md
steps:
  - n: 1
    title: Lint wants a proposed answer for every open question of a decided spec
    after: []
    touches:
      - internal/spec/lint.go
      - internal/spec/spec_test.go
      - internal/spec/spectest/**
    covers: [AC-1, AC-2]
    size: s
    merged: true
  - n: 2
    title: The standard and the skills say a spec PR is proposed as accepted
    after: [1]
    touches:
      - specs/README.md
      - internal/scaffold/assets/README.md
      - skills/furrow-write/SKILL.md
      - skills/furrow-swarm/SKILL.md
    covers: [AC-3, AC-4, AC-5]
    size: s
    merged: false
---

# FUR-0012: Merging a spec's PR accepts it

## Why

A spec is proposed when its PR opens, and decided when the owner merges it. Today the owner must
also flip `status: draft` to `accepted` on the branch before merging, or the merged spec says
"undecided" and nothing is offered. The open questions have the same problem: their proposed
answers still read as undecided after the merge. Merging should be the whole decision, with
nothing to edit first and nothing to tidy up after.

## What

**A spec's PR is opened with `status: accepted`.** An unmerged PR is the proposal; the base branch
is what's decided, because `furrow next` runs from an up-to-date base and never sees an unmerged
branch's specs. Merging the PR is the owner accepting it. Closing it is the owner saying no.

**`draft` means parked.** A spec that's merged as `draft` is kept but not offered, for an idea the
owner wants written down but not built yet. `furrow new` still starts specs as `draft`, so lint
stays relaxed while one is being written; `furrow-write` sets `accepted` when it opens the PR.

**In a decided spec, a proposed answer is the answer.** Each top-level item under
`## Open questions` of an `accepted` or `paused` spec must say `Proposed: …`, and an agent building
the spec takes that proposal as decided. The owner overrules one before merging; the agent then
moves the question to `## Decisions` as `- YYYY-MM-DD: <question> Owner chose <answer> over the
proposed <proposal>.` (FUR-0011 says how the owner answers from a PR.)

**Lint.** For an `accepted` or `paused` spec in `changes/`, each top-level `- ` item under
`## Open questions` without `Proposed:` is an error: `Open question on line 241 has no proposed
answer. Add "Proposed: …", or answer it in ## Decisions.` Items and headings inside fenced code
blocks don't count. A `draft` spec may have questions without proposals.

**The standard** (`specs/README.md` and the copy `furrow init` writes):

- the status diagram becomes `draft (parked) · PR opened as accepted ──merge──► accepted ──every
  step merged──► …`;
- the status table says `accepted` is set in the spec's PR and that merging it is the decision,
  and `draft` is a parked spec, never offered;
- the `## Open questions` row says a decided spec's questions each carry a proposed answer, which
  stands unless the owner overrules it before merging.

**The skills.** `furrow-write` sets `status: accepted` and checks every question has `Proposed:`
before opening the spec's PR. It never merges a spec PR. `furrow-swarm`, which merges step PRs
that pass review, never merges a spec PR.

## Acceptance criteria

- [x] AC-1: An accepted or paused spec with an open question lacking `Proposed:` fails lint with the message and line; a draft one passes (test: TestLintOpenQuestionsNeedProposals)
- [x] AC-2: An `## Open questions` heading and items inside a fenced code block are ignored by that rule (test: TestLintOpenQuestionsIgnoreFences)
- [ ] AC-3: The standard's two copies say a spec PR is opened as accepted, merging it is the decision, and draft means parked (check: grep -q -i "parked" specs/README.md && grep -q -i "parked" internal/scaffold/assets/README.md && grep -q -i "merging it is the decision" specs/README.md && grep -q -i "merging it is the decision" internal/scaffold/assets/README.md)
- [ ] AC-4: furrow-write opens spec PRs as accepted with a proposal for every question, and furrow-write and furrow-swarm never merge a spec PR (check: grep -q "status: accepted" skills/furrow-write/SKILL.md && grep -q -i "never merge" skills/furrow-write/SKILL.md && grep -q -i "never merge.*spec" skills/furrow-swarm/SKILL.md)
- [ ] AC-5: Proposing and accepting a spec this way reads right to the owner end to end (human: merge the next spec PR without editing it, and read the spec on main)

## Out of scope

- A `furrow accept` command. Nothing needs editing at merge time any more.
- Telling from Furrow whether a PR was merged or closed. That needs a forge (INV-2).
- Changing the specs already parked as `draft` on main (FUR-0008, 0009, 0010).

## Open questions

- Should `draft` stay, now that it only means parked? Proposed: yes. Writing an idea down without
  committing to build it is worth one word.
- Should the heading stay `## Open questions` though, once merged, its proposals are decided?
  Proposed: yes. Renaming it touches every spec for little gain, and the standard says how to read
  it.
- Should agents never merging a spec PR become an invariant, like sign-off (INV-3)? Proposed: not
  yet. The forge already keeps merge rights with the owner, and Furrow can't check it.

## Decisions

- 2026-11-02: Chose to count line numbers from the top of the file, front matter included, by
  reading the spec's file again in lint, because that's the line an editor shows and `Spec` keeps
  only the body.
- 2026-11-02: Chose to treat a top-level item as running until the next top-level `- ` item or
  heading, so `Proposed:` on any continuation or nested line counts, because questions wrap.
- 2026-11-02: Chose to skip every line inside a fence of three or more backticks, closed by a run
  at least as long, because a four-backtick fence often shows a three-backtick one.
