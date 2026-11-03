---
name: furrow-next
description: Pick the next Furrow spec step, claim it, open its draft PR, and plan the work. Use at the start of a session with no task in a repo that has a .furrow.yml, when asked "what's next?" or "what can I work on?", or when handed a step ref like FUR-0007.2.
---

# Pick and claim a step

Furrow repos are built one **step** per PR. A step belongs to a spec in `specs/changes/`, waits on
the steps in its `after`, and may change only the paths in its `touches`. `furrow` (or the repo's
pinned `bin/furrow`) knows what's free.

1. **Start from an up-to-date base.** `git fetch origin` and work from the base branch.
2. **Pick.** Run `furrow next`. Take the first step under "Ready to start" (they're ranked by how much
   work they unblock), or the step you were given if `next` lists it as ready. If nothing is ready,
   say so and stop. Don't start a waiting, blocked or draft step.
3. **Read before you claim.** Read, in this order:
   - the spec file, all of it;
   - every file in its `context`;
   - its area's living spec (`specs/areas/<area>.md`);
   - the `## Decisions` of the specs in the step's `after`. Earlier steps record what they built
     differently;
   - `AGENTS.md` or `CLAUDE.md`, and `docs/product.md`'s **Invariants**.
4. **Claim.** `furrow claim <REF>`. It creates the step's branch on the remote and switches to it.
   Exit 3 means someone beat you to it: run `furrow next` again and pick another. Never work on a
   step you didn't claim.
5. **Open a draft PR** with the forge's tools, titled exactly as `claim` printed: `[REF] <step
   title>`. Its body is `furrow brief`'s output, as printed: the reviewer's brief, which lists
   the criteria the step `covers`, each unticked, and leaves `<!-- furrow:` placeholders for
   `furrow-implement` to fill in. The brief is the template; don't use another.
6. **Plan.** Write down the files you'll change (all inside `touches`), one test per covered
   criterion named as its `(test: …)` says, and anything unclear. If something unclear changes the
   work, write it under "Spec problem" in the PR and stop. Otherwise continue with
   `furrow-implement`.

Never tick a `human:` criterion by hand, never sign off unless the owner asks (then use
`furrow-signoff`), and never edit a criterion's text.
