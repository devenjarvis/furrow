---
id: FUR-0014
title: Briefs a person can review on a phone
status: accepted
area: brief
priority: p1
human_review: required
invariants: [INV-3, INV-6]
context:
  - docs/product.md
  - specs/areas/brief.md
  - specs/changes/0011-reviewer-brief.md
  - internal/brief/brief.go
  - internal/brief/check.go
  - internal/brief/spec.go
steps:
  - n: 1
    title: The step brief leads with what a reviewer judges
    after: []
    touches:
      - internal/brief/brief*
      - internal/brief/check*
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6]
    size: m
    merged: true
  - n: 2
    title: The spec brief's Decision is two lines
    after: []
    touches:
      - internal/brief/spec*
    covers: [AC-7]
    size: s
    merged: true
  - n: 3
    title: The skills write and verify the new brief
    after: [1, 2, 4]
    touches:
      - internal/brief/check*
      - skills/furrow-implement/SKILL.md
      - skills/furrow-verify/SKILL.md
      - skills/furrow-write/SKILL.md
      - specs/areas/agents.md
    covers: [AC-8, AC-9, AC-10, AC-11]
    size: m
    merged: false
  - n: 4
    title: The spec brief has no Proof and asks only new questions
    after: [1, 2]
    touches:
      - internal/brief/spec*
    covers: [AC-12, AC-13]
    size: s
    merged: false
---

# FUR-0014: Briefs a person can review on a phone

## Why

FUR-0011's briefs carry the right facts in the wrong shape. Behaviour is one run-on paragraph, the
best evidence (real before and after output) is buried under Proven, "Where to look" is a file
list, decisions are pasted in full, Proven restates every criterion, and invariants are named
without saying what to check. The owner reads these on a phone. The brief should lead with what
needs a person's judgment and fold away what doesn't.

## What

### The step brief

`furrow brief` prints these sections, in this order. *When* means the section is left out
otherwise, as before.

1. **`## Behaviour`**:

   ```
   **Before:** <!-- furrow: what a user or caller saw, at most 20 words -->
   **After:** <!-- furrow: what they see now, at most 20 words -->

   <!-- furrow: show it: real command output, a screenshot, or a short usage example. "No visible effect." is an answer. -->
   ```
2. **`## Diagram`** *when*: as now.
3. **`## Review this`**: `<!-- furrow: at most 3 numbered items, each "`file` `name`: the question a reviewer should ask about it" -->`,
   then every changed file, grouped as now (Code, Visual, Tests, Specs), inside
   `<details><summary>All N changed files</summary> … </details>`.
4. **`## Visual proof`** *when*: as now.
5. **`## Judgment calls`** (was Departures from the spec): one `- ` line per Decisions line the
   diff adds, with its `YYYY-MM-DD: ` prefix dropped, then criteria whose text changed and files
   outside territory, as now. With none: `None.`
6. **`## Needs you`**: each covered `human:` criterion with what to look at, as now. Each
   invariant the spec lists, as `- INV-n: <name>. <!-- furrow: how this diff touches it, or "Not touched." -->`.
   `human_review: required` is no longer listed: the invariants and criteria already say why. With
   nothing: `Nothing.`
7. **`## Not done`**: as now.
8. **Proof**, last and folded, not a `## ` section:

   ```
   <details><summary>Proof: AC-1 ✓ AC-2 ✓ AC-3 ✗</summary>

   - AC-1 (test: TestBriefSections)
   - AC-3 (check: make lint) not ticked
   </details>
   ```

   One line per covered non-human criterion: its ID and proof text, then ` not ticked` if its box
   is open. No placeholder: verify checks the proof, not the description.

`--json` keeps every key it has, renames `decisions_added` to `judgment_calls` (dates dropped),
and adds `proof: [{"id", "proof", "ticked"}]`.

### `--check` for a step brief

On top of today's checks, it reports, in section order:

- no line starting `**Before:**`, or none starting `**After:**`: "`## Behaviour` needs a
  `**Before:**` and an `**After:**` line."
- either one over 20 words: "`**After:**` is 27 words. Keep it to 20."
- Behaviour without a fenced code block, an image, or the words `No visible effect.`: "`## Behaviour`
  doesn't show the change. Add output, a screenshot, or a usage example."
- `## Review this` with more than 3 numbered items: "`## Review this` has 5 items. Keep the 3
  that need a person."
- a `## Judgment calls` line over 30 words: "A judgment call is 41 words. Shorten it in the spec's
  `## Decisions` and run `furrow brief` again."

The Behaviour word cap (80) counts only text outside code blocks and images.

A `<!-- furrow:` placeholder inside a fenced code block is ignored only in `## Behaviour`, after
its `**After:**` line, so "show it" can paste real `furrow brief` output. Anywhere else, a
placeholder inside a fence is still a placeholder, so an unfilled section can't hide in a code
block.

### The spec brief

`furrow brief --spec`'s Decision placeholder becomes two lines, each at most 25 words:

```
**Commits us to:** <!-- furrow: what accepting this means we build, at most 25 words -->
**Instead of:** <!-- furrow: the main alternative it rejects, and why, at most 25 words -->
```

`--check --spec` reports a missing line or one over 25 words, as for Before and After.

The spec brief has no Proof block: that belongs to a step brief.

On a PR that amends a spec already on the base branch, its questions there were decided when it
merged. `## Your calls` lists only questions that aren't in the spec at the merge base, numbered
from Q1. With none: `None. Ready to accept.`

### The skills

- **furrow-implement** fills the brief this way: Before and After as a user or caller sees it;
  one "show it" (for a CLI, the command and its output; for a UI, the screenshot; for a library, a
  call and its result), as an excerpt of at most 15 lines showing only the part that changed; at most 3 "Review this" items, each naming where a decision was made and
  the question to ask; each invariant's line says how this diff touches it. It writes each
  `## Decisions` line in at most 30 words.
- **furrow-verify** re-runs a "show it" command when it's cheap and compares the output, and checks
  each "Review this" item names real code.
- **furrow-write** fills the spec brief's two Decision lines.

The last step folds this into `specs/areas/brief.md` and `specs/areas/agents.md`.

## Acceptance criteria

- [x] AC-1: Behaviour has the Before and After placeholder lines and the "show it" placeholder (test: TestBriefBehaviourBeforeAfter)
- [x] AC-2: Review this has the placeholder, then every changed file grouped inside a `<details>` whose summary counts them (test: TestBriefReviewThis)
- [x] AC-3: Judgment calls lists added decisions without their dates, then changed criteria and outside files, or `None.` (test: TestBriefJudgmentCalls)
- [x] AC-4: Needs you gives each invariant a placeholder for how the diff touches it, drops `human_review`, and says `Nothing.` when empty (test: TestBriefNeedsYouInvariants)
- [x] AC-5: The sections come in the new order, Proof is a closing `<details>` with one line per non-human criterion, and `--json` has `judgment_calls` and `proof` (test: TestBriefLayoutAndProof)
- [x] AC-6: `--check` reports a missing Before or After line, one over 20 words, a Behaviour that shows nothing, more than 3 Review items, and a judgment call over 30 words (test: TestBriefCheckReviewable)
- [x] AC-7: The spec brief's Decision is Commits us to and Instead of lines, and `--check --spec` reports a missing one or one over 25 words (test: TestSpecBriefDecisionLines)
- [ ] AC-8: The three skills describe the new brief (check: grep -q "Before:" skills/furrow-implement/SKILL.md && grep -q "Review this" skills/furrow-implement/SKILL.md && grep -q -i "show it" skills/furrow-verify/SKILL.md && grep -q "Instead of" skills/furrow-write/SKILL.md)
- [ ] AC-9: A step PR written with the new brief can be reviewed on a phone without wading through what doesn't need a person (human: open the first step PR made after this merges on your phone, and read it before the diff)
- [ ] AC-10: `--check` ignores a placeholder inside a fence after Behaviour's After line, and still reports one inside a fence in any other section (test: TestBriefCheckFencedPlaceholders)
- [ ] AC-11: furrow-implement keeps "show it" to an excerpt of at most 15 lines (check: grep -q "15 lines" skills/furrow-implement/SKILL.md)
- [ ] AC-12: The spec brief ends without a Proof block (test: TestSpecBriefHasNoProof)
- [ ] AC-13: On an amendment, Your calls lists only questions added since the merge base, and says `None. Ready to accept.` when there are none (test: TestSpecBriefAmendmentCalls)

## Out of scope

- Linting the length of `## Decisions` lines in specs. `--check` catches the ones a reviewer sees.
- Rewriting the descriptions of PRs already merged.
- The visual proof table, the diagram rules and spec-brief sections other than Decision.

## Open questions

- Should Proof fold away in a `<details>` block rather than be a section? Proposed: yes. It's the
  record, not something to judge, and verify already checks it.
- Is 20 words right for each of Before and After? Proposed: yes. One plain sentence each, and the
  "show it" carries the detail.
- Should "Departures from the spec" be renamed "Judgment calls"? Proposed: yes. It says what they
  are: choices the spec left to the agent, which the owner may disagree with.
- Should `human_review: required` stay in Needs you? Proposed: no. The invariants and `human:`
  criteria listed there already say what needs the owner.

## Decisions

- 2026-11-02: Chose to keep the 80-word Decision cap as a backstop because the two 25-word lines don't bound prose written beside them.
- 2026-11-02: Chose to report a missing line once, not per line, because the spec gives one message naming both lines.
- 2026-11-02: Chose to count a line's words after its label, comments excluded, because the label isn't the writer's words.
- 2026-11-02: Chose "1 changed file" for a single file, and "No files changed." without `<details>` for none, because "All 1 changed files" and an empty fold read badly.
- 2026-11-02: Chose to end an all-human step's brief with "Proof: none" and a one-line reason, so every step brief closes the same way.
- 2026-11-02: Chose to have `--check` ignore placeholders inside fenced code blocks, closed by a fence at least as long, so "show it" can paste raw `furrow brief` output.
- 2026-11-02: Chose to sort judgment calls by their dated lines before dropping the dates, so they stay in date order.
- 2026-11-02: Chose to count only unindented numbered lines as Review this items, and to check every Judgment calls line, list marker excluded, against the 30-word cap.
