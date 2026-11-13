---
name: furrow-verify
description: Review a Furrow step's branch or PR against its spec, criterion by criterion, as a skeptic. Use when asked to verify or review a step like FUR-0007.2, or by furrow-swarm after an agent hands a step back.
---

# Verify a step

You're the reviewer, not the author. Assume nothing is done until you've seen the evidence.

1. Find the spec and the step from the ref. Check out the step's branch (`git fetch origin` first).
2. For each criterion the step `covers`:
   - `test:`: the named test exists, tests what the criterion says (not something weaker), and passes.
   - `check:`: run the command and read the result.
   - `human:`: it must be **unticked**. Say what the owner should look at, and whether the PR gives
     them what they need: steps, screenshots, output.
3. **Territory.** `furrow territory --base origin/<PR's base branch> --body-file <PR body>` must exit
   0. Read each explained path's reason. For a stacked step (its claim commit has a
   `furrow-stack:` trailer), check the PR is based on the parent's branch, not the base, so the
   diff is only this step's.
4. **The brief.** The PR description is the reviewer's brief. Save it to a file;
   `furrow brief --check <file> --base origin/<base>` must exit 0. Then read it against the diff:
   - **Behaviour**: Before and After say what the code does, not what the author meant it to do.
     Re-run the "show it" command when it's cheap and compare its output; it should show only
     the part that changed.
   - **Before and after**: compare the step's real output with the part of the spec's
     `## Before and after` mock it delivers. A difference the PR doesn't explain under
     "Spec problem" is a finding.
   - **Review this**: each item names code that exists in the diff, and asks a real question.
   - **Diagram**: every node and edge exists in the code. It has a caption and a `Legend:` line,
     at most 8 nodes, is a mermaid `flowchart TD`, and shows behaviour, not a call graph.
   - **Visual proof**: each image shows the screen its row names.
   - **Judgment calls** match the lines the diff adds to the spec's `## Decisions`, one for one.
   - **Needs you**: each invariant's line says truly how the diff touches it.
   - A screenshot is evidence for the owner, never a reason a `human:` criterion is ticked.

   A claim the diff doesn't bear out is a finding, as a failed test would be.
5. **Invariants.** Read the spec's `invariants:` in `docs/product.md` and check the diff keeps every
   one.
6. **Spec hygiene.** `furrow lint` passes. The step is `merged: true`, its covered non-human boxes are
   ticked, and no criterion's text changed (`git diff` the spec). On a spec's last step, the area's
   `## Behaviour` says what changed.
7. Run the repo's check.
8. Report a table: criterion, verdict (met, not met, can't tell), evidence, with each brief
   claim the diff doesn't bear out as a row of its own. Then territory, brief, invariant and
   hygiene findings with file and line. Say plainly whether it's ready to merge.

Run as a sub-agent (a `furrow-verifier` started by `furrow-swarm`), your report is only the verdict
table, the findings one line each (`findings: none` if there are none), and one last line,
`ready to merge: yes` or `ready to merge: no`. At most 25 lines in all, with no raw command output
except at most 10 lines of a check that failed.
