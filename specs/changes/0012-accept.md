---
id: FUR-0012
title: furrow accept records the owner's answers and accepts a spec
status: draft
area: lifecycle
priority: p1
human_review: required
invariants: [INV-1, INV-4]
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/lifecycle.md
  - internal/lifecycle/lifecycle.go
  - internal/lifecycle/edit.go
steps:
  - n: 1
    title: furrow accept
    after: []
    touches:
      - cmd/furrow/accept.go
      - internal/lifecycle/accept.go
      - internal/lifecycle/accept_test.go
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6]
    size: s
    merged: false
  - n: 2
    title: The standard and the skills say how a spec is accepted
    after: [1]
    touches:
      - specs/README.md
      - internal/scaffold/assets/README.md
      - skills/furrow-write/SKILL.md
      - skills/furrow-swarm/SKILL.md
    covers: [AC-7, AC-8]
    size: s
    merged: false
---

# FUR-0012: furrow accept records the owner's answers and accepts a spec

## Why

A draft spec's open questions each carry a proposed answer, and merging the spec is meant to
accept them. Today nothing records that: after the merge the questions still sit under
`## Open questions`, looking undecided, though the standard says an accepted spec has none an agent
must ask about. `furrow accept` turns the owner's merge into what the file says: every proposed
answer becomes a decision, with a date, and the status changes in the same commit.

## What

`furrow accept <id> [--json]` is for the owner, run on the spec's PR branch before merging.

1. Inside an agent session (`CLAUDECODE` or `FURROW_AGENT` set) it exits 1 with "Accepting a spec
   is the owner's decision. Ask the owner to run this." before reading anything, using the same
   check as `furrow signoff`.
2. It refuses, exit 1 and changing nothing, when:
   - the spec isn't in `changes/`: "No spec FUR-0042 in changes/.";
   - `furrow lint` reports a problem in that spec: it prints them, then "Fix these first.";
   - an item of `## Open questions` has no `Proposed:`: "The open question on line 241 has no
     proposed answer. Add `Proposed: …`, or answer it in `## Decisions`." It names every such line.
3. On an accepted spec it says "FUR-0012 is already accepted." and changes nothing, exit 0. On a
   `paused` spec it refuses: "FUR-0012 is paused. Set it to accepted by hand when it's unpaused."
4. Otherwise it edits the spec file in place:
   - `status: draft` becomes `status: accepted`;
   - each top-level item of `## Open questions`, in order, is removed, and
     `- YYYY-MM-DD: <question> Accepted as proposed: <answer>` is appended to `## Decisions`,
     where the question is the text before `Proposed:` and the answer the text after it, each
     joined onto one line with runs of whitespace made single;
   - when no item is left, the `## Open questions` heading and its blank lines go too.

   Headings and list items inside fenced code blocks don't count, so a spec can show an example
   `## Open questions` section, as this one does.

   Nothing else changes: not the criteria (INV-4), not the steps, not other sections, not other
   lines in Decisions. The date comes from the injected clock.
5. It prints "FUR-0012 accepted. 5 proposed answers moved to ## Decisions. Commit it, and merge the
   PR.", or with `--json` `{"id", "status", "decisions": [...]}`. It never commits or pushes.

Before running it, the owner overrules any proposal they disagree with, in the PR or by asking the
agent. That answer is already in `## Decisions` and gone from `## Open questions`, so `accept` only
sees the proposals the owner let stand.

The standard (`specs/README.md` and the copy `furrow init` writes) says the owner accepts a spec by
running `furrow accept` on its PR and merging. `furrow-write` and `furrow-swarm` say agents never
run it.

Before, for a spec with one question:

```
status: draft
…
## Open questions

- Is `brief` the right name? Proposed: yes. `guide` reads like docs.

## Decisions
```

After `furrow accept FUR-0011` on 2026-11-03:

```
status: accepted
…
## Decisions

- 2026-11-03: Is `brief` the right name? Accepted as proposed: yes. `guide` reads like docs.
```

## Acceptance criteria

- [ ] AC-1: Inside an agent session it refuses with its message before reading the repo (test: TestAcceptRefusesAgents)
- [ ] AC-2: It moves each proposed answer to `## Decisions` in order, joined onto one dated line, removes the emptied `## Open questions` section, and sets `status: accepted` (test: TestAcceptMovesProposals)
- [ ] AC-3: Only those lines change: the criteria, steps, other sections and existing Decisions are byte-identical, and an `## Open questions` heading inside a fenced code block is left alone (test: TestAcceptEditsOnlyItsLines)
- [ ] AC-4: A question without `Proposed:`, a lint problem, or an unknown spec refuses with its message, naming every bad line, and changes nothing (test: TestAcceptRefusals)
- [ ] AC-5: An accepted spec says so and changes nothing; a paused spec refuses (test: TestAcceptAlreadyAcceptedOrPaused)
- [ ] AC-6: It prints the count and what to do next, and `--json` has `id`, `status` and `decisions` (test: TestAcceptOutput)
- [ ] AC-7: The standard's two copies say the owner accepts with `furrow accept`, and furrow-write and furrow-swarm say agents never run it (check: grep -q "furrow accept" specs/README.md && grep -q "furrow accept" internal/scaffold/assets/README.md && grep -q "furrow accept" skills/furrow-write/SKILL.md && grep -q "furrow accept" skills/furrow-swarm/SKILL.md)
- [ ] AC-8: Accepting a real spec with `furrow accept` reads right in the file and the PR (human: run it on the next spec PR and read the diff)

## Out of scope

- Committing, pushing or merging. The owner does that.
- Telling whether the PR was merged, or reading PR comments. That needs a forge (INV-2).
- Accepting a spec whose questions are only partly proposed. Every question gets an answer
  first.

## Open questions

- Should "agents never accept a spec" become an invariant (INV-8) in `docs/product.md`, like
  sign-off? Proposed: yes, in its own small PR by the owner, since invariants are the owner's.
- Should `accept` also work on a `paused` spec, to unpause it? Proposed: no. Unpausing means
  something outside the code changed, and the owner should say what in `## Decisions`.

## Decisions
