---
name: furrow-implement
description: Build a claimed Furrow spec step end to end, with tests first, inside its territory, and hand it back with furrow done. Use after furrow-next, or when told to implement a step like FUR-0007.2 on its claimed branch.
---

# Build a claimed step

You're on the step's branch (`furrow claim` put you there). The spec's `## What` is the contract.
Its acceptance criteria are what you prove. Its `## Before and after`, if it has one, is the
target: build so the part of the mock your step delivers is what a user really sees.

1. **Tests first.** For each criterion the step `covers` that has a `(test: Name)`, write that test,
   with that name, and watch it fail. Use the repo's fakes and fixtures. Tests never touch the
   network or real services.
2. **Build until they pass.** Follow the repo's `AGENTS.md`/`CLAUDE.md`. Keep logic where it can be
   tested.
3. **Stay inside `touches`.** Run `furrow territory` as you go. It lists every changed file outside the
   step's paths. If you truly must go outside, keep it small and explain each path under a
   `## Territory` heading in the PR. `furrow territory --body-file <PR body>` then accepts it. If it's
   big, stop and write a "Spec problem" instead.
4. **Run the repo's check**: `check:` in `.furrow.yml`, or `make check`. Fix everything it reports.
5. **Tick with evidence.** In the spec, tick each covered criterion you proved: `- [ ]` to `- [x]`.
   Put the evidence in the PR next to each one: the test name, a command and its output, or a
   screenshot. **Never tick a `human:` criterion by hand**, even if you checked it yourself. It waits
   for the owner in `furrow review`. Never sign off unless the owner asks, and then use
   `furrow-signoff`, in a sign-off PR of its own.
6. **Log decisions.** Add `- YYYY-MM-DD: Chose X because Y.` to the spec's `## Decisions` for
   anything the spec didn't say, in at most 30 words. Each one is a judgment call in the brief.
7. **Keep the diagram current.** If the spec's `## What` has a ```` ```mermaid ```` diagram and
   your step changes what it shows, update it. Draw to the standard: a one-sentence caption above
   saying what it answers, a `Legend:` line below saying what a box, an arrow and each colour
   mean, at most 8 nodes, `flowchart TD`, and behaviour (what flows where), not a call graph.
   Every node names a real file, package, type or function.
8. **Visual proof.** If `furrow brief` has a `## Visual proof` section, capture each screen before
   and after with the repo's own tools, as its `AGENTS.md` says, and put the images in the table.
   A screenshot is evidence for the owner, never a reason to tick a `human:` criterion (INV-3).
9. **Fill in the brief.** The PR description is `furrow brief`'s output. Regenerate it if the
   diff changed, then fill every `<!-- furrow:` placeholder, for an owner reading on a phone:
   - `**Before:**` and `**After:**`: what a user or caller saw and sees now, at most 20 words
     each, not which files changed.
   - One "show it" under the After line: for a CLI, the command and its output; for a UI, the
     screenshot; for a library, a call and its result. Paste real output, as an excerpt of at most
     15 lines showing only the part that changed. With nothing to show: `No visible effect.`
     It should look like the part of the spec's `## Before and after` this step delivers. Where
     the code can't or shouldn't match the mock, say how and why under "Spec problem" in the PR;
     never differ quietly.
   - `## Review this`: at most 3 numbered items, each a place in the code where a decision was
     made (`` `file` `name` ``) and the question to ask about it. The file list stays folded.
   - `## Needs you`: each invariant's line says how this diff touches it, or `Not touched.`

   Save it to a file, then run `furrow brief --check <file>` and
   `furrow territory --body-file <file>`. Both must exit 0.
10. **Hand it back.** `furrow done <REF>`, committed and pushed **before marking the PR ready**,
    never after. It sets the step's `merged: true` in this PR, so merging the PR unblocks whatever
    waits on it, and from the moment it's pushed the step is handed back: with `stack:` on, other
    steps can start on top of it. A PR merged before `done` leaves the step unmerged on the base and
    blocks everything after it. `done` refuses if a covered non-human criterion is unticked. Do all
    of steps 4 to 12 in one push where you can, so the repo's CI runs once.
11. **Last step of the spec?** Fold what the spec changed into `specs/areas/<area>.md`'s
    `## Behaviour`, in present tense, as if it had always worked this way, with the spec's diagram
    if it has one. Don't archive: the owner's sign-off, or `furrow archive`, does that once every
    box is ticked.
12. Run `furrow lint`, commit, push, put the filled-in brief in the PR description, and mark the
    PR ready.

## A stacked step

If `furrow claim` said `Stacked on <branch>`, your branch sits on the parent step's branch.

- The PR's base is that branch (see `furrow-next`); with `gh stack` installed, `gh stack link`
  joins it to the parent's PR as a GitHub stack. `furrow territory` diffs against the parent's
  branch on its own. Pass `--base origin/<parent branch>` to other commands that compare with the base, such as
  `furrow brief`.
- If the parent's branch changes after review, rebase onto it (`git rebase origin/<parent>`) and
  push with `--force-with-lease`, unless GitHub's stack has already rebased yours.
- If the parent merged and GitHub didn't rebase yours, rebase onto the base:
  `git rebase --onto origin/<base> <old parent tip>`. `furrow territory` listing the parent's files
  as outside is the sign.
- If the parent's PR was closed without merging, stop and report it under "Spec problem": your
  step was built on work that isn't coming. In a swarm, the hand-back carries it to the
  orchestrator, which parks it.

## When the spec is wrong

- A small gap inside your territory, such as a name or an edge case: decide, log it in
  `## Decisions`, carry on.
- A criterion that can't be met as written, a contradiction with an invariant or another spec, or
  work in another step's paths: **don't bend the criterion to fit your code**, and don't edit its
  text. Write "Spec problem" in the PR with what you'd change, and stop.
