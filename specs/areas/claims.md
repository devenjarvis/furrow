---
area: claims
title: Claims
---

# Claims

How an agent takes a step so no one else does: `furrow claim`, `furrow release` and `furrow claims`.

## Behaviour

A claim is a branch on the remote, named by the config's `branch` pattern (`step/{num}-{step}`,
so FUR-0003.1 is `step/0003-1`). Only `internal/gitx` runs a process, and the process is `git`.

**`furrow claim <ref>`** fetches base and reads the claims on the remote. It refuses a step that
isn't ready and says why: it's merged, its spec isn't accepted, it's blocked, it waits on named
steps, or someone holds it. It also refuses a step whose `touches` overlap a claimed step, unless
you pass `--force`, and a dirty working tree. Then it makes a claim commit on base. The commit
changes nothing and its message is unique, and furrow pushes it to the new branch with an empty
lease (`--force-with-lease=refs/heads/<branch>:`), so the push only succeeds if the branch doesn't
exist. Exactly one agent wins. The loser exits 3: "FUR-0003.1 is already claimed (step/0003-1).
Run `furrow next` for another step." The winner is switched to the branch, with its upstream set,
unless `--no-checkout`, and furrow prints the PR title `[REF] step title`.

**`furrow release <ref>`** deletes the claim branch. Releasing an unclaimed step says so and exits 0.

**`furrow claims [--stale 24h] [--json]`** lists every claim with the age of its branch's last
commit. It fetches the claim branches into `refs/furrow/claims/` to read them. A claim is `leftover`
when its step is merged (the branch outlived its PR), `stale` when it has had no commit for longer
than `--stale`, and `active` otherwise.

When the remote can't be reached, the claim functions return `claim.ErrRemote` so callers can say
so and carry on.

## History

- FUR-0003: Claim a step with a branch, atomically (archived 2026-11-03)
