---
name: furrow-swarm
description: Run several coding agents at once on a Furrow repo, each on its own non-overlapping spec step in its own worktree, then verify and merge what comes back. Use when asked to build ("let's get building", "keep building"), to work through the queue, to work in parallel or "use sub agents", to "run a swarm" or "use N agents", or to build as much of the spec queue as possible. Prefer it to writing your own agent prompts.
---

# Run a swarm

You're the orchestrator. Furrow decides what can run together. You hand out work, read what comes
back, and keep the queue moving. Keep your own conversation small: agents build, verify and do
chores, and report to you in a few lines. You don't build or verify steps yourself.

The plugin ships the agents you start: `furrow-builder`, `furrow-verifier` and `furrow-chores`.
Start them by name; don't write your own agent prompts.

## Models

The orchestrator, this session, runs on Haiku: it hands out work and reads short reports, so it
doesn't need more. The agents set their own models in their front matter: `furrow-builder` and
`furrow-verifier` run on Opus and `furrow-chores` on Sonnet, whatever this session runs on. If this
session isn't on Haiku, tell the owner once, in one line, and carry on:
`This swarm runs on <model>. /model haiku makes the orchestrator cheaper; the agents stay on Opus.`
A project overrides an agent's model by copying the agent into its own `.claude/agents/`.

## Loop

1. **Start from `furrow status`.** It says what's in progress, handed back, ready, and waiting on
   the owner, and what to do next. Then `git fetch origin`, update the base branch, and run
   `furrow next --agents N --json`. `N` is the number of agents you're allowed. `together` holds the
   assignments: no two share a path, and none overlaps a step someone has already claimed. If
   `claims_checked` is false, the remote wasn't reachable, so stop rather than risk doubled work.
2. **For each assignment**, make a worktree at the latest base, detached:
   `git worktree add --detach ../wt/<ref> origin/<base>`. Then start one background
   `furrow-builder` agent per worktree. Its brief gives:
   - the ref and the worktree path, and "work only there";
   - the exact territory: the step's `touches`, plus its spec file and area file;
   - for a stacked step, the branch it's `Stacked on`, which is its PR's base;
   - anything the owner said that bears on the step.

   The builder claims, builds, runs `furrow done` and pushes before marking the PR ready, and never
   ticks a `human:` box, signs off or edits a criterion's text. Its brief ends with this hand-back
   contract:

   ```
   FUR-0018.2  PR #51  step/0018-2  handed back
   claim: exit 0
   make check: exit 0
   furrow territory: exit 0
   furrow lint: exit 0
   furrow done: pushed
   decisions: chose internal/status over cmd-only, so the text and JSON share one model
   spec problem: none
   ```

   At most 15 lines: each command with its exit code, decisions one line each, a spec problem or
   `none`. Raw output only for a command that failed, at most 20 lines of it.
3. **As each builder hands back**, start a fresh `furrow-verifier` agent with the ref, the branch,
   the PR number and a worktree. Never run `furrow-verify` in your own conversation: read only the
   verdict. On `ready to merge: no`, send the findings back to the step's builder. A spec problem
   goes to the owner. Don't fix the spec to make the code pass. On `ready to merge: yes`, check CI
   on the PR's head commit before you merge:
   - **Every required check passed on the head commit:** merge through the repo's PR flow, or by
     merging the branch if the owner works that way.
   - **A check failed on the head commit:** don't merge. Send a `furrow-chores` agent to read the
     failure. A failure in the step's code goes back to the builder, and its next hand-back gets a
     fresh verifier. A failure the chore shows is unrelated to the diff may be re-run once on the
     same commit; merge only when that run passes.
   - **Checks still running:** wait for them. Never merge on an older commit's green run.
   - **The repo has no CI:** the verifier's run of the repo's check is the gate.

   A step whose fix wasn't re-verified never merges: every merge follows a fresh
   `ready to merge: yes` on the head commit you merge.
4. **Chores go to a `furrow-chores` agent**, with the ref, the branch, a worktree and the chore:
   rebasing a stacked step onto a parent that changed, merging the base into a step branch when
   the only conflict is in `specs/`, re-running the repo's check, and reading a CI failure. It
   reports at most 10 lines. A conflict outside `specs/`, a failure it can't explain, and anything
   else go back to the step's builder.
5. **After each merge, and each hand-back**, run `furrow next --agents <free slots> --json` again.
   Merges unblock new steps. With `stack:` set in `.furrow.yml`, a step that's handed back (its
   `furrow done` is pushed) unblocks too: what waits on it comes back with `stack_on`, to be built on
   top of it. Hand work out as soon as an agent is free, within the wave.
6. **Tidy:** when `furrow lint` warns that a spec is ready to archive, run `furrow archive <id>`.
   `furrow claims` lists leftover branches. `furrow release` them, or ask the owner to delete them if
   the remote doesn't let you.

## Hand off

Stop, instead of carrying on into the next wave, when any of these holds:

- a wave has merged and `furrow next` offers new work, so the next wave starts in a fresh session;
- nothing can start until the owner acts: a review, a sign-off, a spec to merge;
- the conversation is over about 200K tokens, when the tool shows it.

Wait for the agents that are out, then write a handoff of at most 10 lines. Hold only what the repo
doesn't: questions put to the owner in chat and not yet answered, and anything blocked outside
Furrow (a secret, a sign-in, a broken tool). End it with exactly this line:

```
Start a new session with: Run furrow status, then furrow-swarm.
```

Then give the "Stop when" report below.

## Stop when

`furrow next` says nothing is ready and no agent is out, or you've handed off. Report to the owner:
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
