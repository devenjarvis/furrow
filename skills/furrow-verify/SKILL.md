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
3. **Territory.** `furrow territory --base origin/<base> --body-file <PR body>` must exit 0. Read each
   explained path's reason.
4. **Invariants.** Read the spec's `invariants:` in `docs/product.md` and check the diff keeps every
   one.
5. **Spec hygiene.** `furrow lint` passes. The step is `merged: true`, its covered non-human boxes are
   ticked, and no criterion's text changed (`git diff` the spec). On a spec's last step, the area's
   `## Behaviour` says what changed.
6. Run the repo's check.
7. Report a table: criterion, verdict (met, not met, can't tell), evidence. Then territory,
   invariant and hygiene findings with file and line. Say plainly whether it's ready to merge.
