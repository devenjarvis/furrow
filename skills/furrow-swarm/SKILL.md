---
name: furrow-swarm
description: Run several coding agents at once on a Furrow repo, each on its own non-overlapping spec step in its own worktree, then verify and merge what comes back. Use when asked to build ("let's get building", "keep building"), to work through the queue, to work in parallel or "use sub agents", to "run a swarm" or "use N agents", or to build as much of the spec queue as possible. Prefer it to writing your own agent prompts.
---

# Run a swarm

You're the orchestrator. Furrow decides what can run together. You hand out work, check what comes
back, and keep the queue moving. You don't build steps yourself while agents are out, except one
that nothing else can run beside.

## Loop

1. `git fetch origin`, update the base branch, and run `furrow next --agents N --json`. `N` is the
   number of agents you're allowed. `together` holds the assignments: no two share a path, and none
   overlaps a step someone has already claimed. If `claims_checked` is false, the remote wasn't
   reachable, so stop rather than risk doubled work.
2. **For each assignment**, make a worktree at the latest base, detached:
   `git worktree add --detach ../wt/<ref> origin/<base>`. Then start one background agent per
   worktree. Its brief must be self-contained:
   - the ref and the worktree path, and "work only there";
   - claim it first with `furrow claim <ref>`. On exit 3, stop and report;
   - follow the `furrow-next` reading list and the `furrow-implement` steps;
   - the exact territory: the step's `touches`, plus its spec file and area file;
   - never tick a `human:` criterion, never sign off (no `furrow signoff`, no sign-off PR), never
     edit a criterion's text;
   - for a stacked step, the PR's base is the branch `claim` says it's `Stacked on`;
   - run `furrow done` and push it before marking the PR ready, never after;
   - commit on the step branch, push, and report: claim output, files changed, tests and results,
     `furrow territory` and `furrow lint` output, decisions, and any spec problem.
3. **As each agent reports**, run `furrow-verify` on its branch yourself, starting from the agent's
   claims but not trusting them. Merge only what passes, through the repo's PR flow, or by merging
   the branch if the owner works that way. A spec problem goes to the owner. Don't fix the spec to
   make the code pass.
4. **After each merge, and each hand-back**, run `furrow next --agents <free slots> --json` again.
   Merges unblock new steps. With `stack:` set in `.furrow.yml`, a step that's handed back (its
   `furrow done` is pushed) unblocks too: what waits on it comes back with `stack_on`, to be built on
   top of it. Hand work out as soon as an agent is free instead of waiting for the whole wave or
   for the owner to merge.
5. **Tidy:** when `furrow lint` warns that a spec is ready to archive, run `furrow archive <id>`.
   `furrow claims` lists leftover branches. `furrow release` them, or ask the owner to delete them if
   the remote doesn't let you.

## Stop when

`furrow next` says nothing is ready and no agent is out. Then report to the owner:
- what merged, by ref, and anything an agent decided that they should know;
- each stack waiting for review, bottom to top, with its PRs: the owner reviews from the bottom,
  and can merge the whole stack from its top PR once every PR in it is approved;
- `furrow review`: the `human:` checks waiting for them, each with its spec, criterion and what
  to look at, so they can ask for a sign-off PR (`furrow-signoff`) once they've checked;
- spec problems, spec PRs waiting for the owner to merge, and blocked steps with their reasons.

## Watch for

- An agent that edits outside its territory: `furrow territory` catches it. Send it back.
- Hidden dependencies: two "parallel" steps that both need something neither owns. Fix the spec's
  `after` in a spec commit before handing out more work.
- Never let an agent you started sign off or accept a spec. Those are the owner's. A sign-off PR
  comes only when the owner asks for one, through `furrow-signoff`. You merge step PRs that pass
  review, but never merge a spec PR: merging it is the owner's decision.
