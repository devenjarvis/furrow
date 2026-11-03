---
id: FUR-0003
title: Claim a step with a branch, atomically
status: done
area: claims
priority: p0
human_review: required
invariants: [INV-2, INV-5]
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/specs.md
steps:
  - n: 1
    title: claim, release and claims over plain git
    after: [FUR-0002.1]
    touches:
      - internal/gitx/**
      - internal/claim/**
      - cmd/furrow/claim.go
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6, AC-7]
    size: m
    merged: true
---

# FUR-0003: Claim a step with a branch, atomically

## Why

Fetch and Edition claimed work by opening a draft PR. Two agents that ran `next` at the same moment
both picked the top step and both opened a PR. When `gh` couldn't reach GitHub, both tools said so
and carried on without checking claims. A branch on the remote is a better claim: creating a ref
either succeeds or fails, so one agent wins. And it needs only `git` (INV-2).

## What

`internal/gitx` runs `git` (and nothing else) in the repo root and returns trimmed output or an error
that includes git's stderr.

`internal/claim`:
- `Branch(cfg, ref)` fills the config's `branch` pattern: `{num}` is the spec's 4-digit number and
  `{step}` the step number. `step/{num}-{step}` and FUR-0003.1 make `step/0003-1`.
- `List(root, cfg)` runs `git ls-remote --heads <remote>` and returns every branch that matches the
  pattern, as `{ref, branch, sha}`, sorted by ref. When the remote can't be reached it returns
  `ErrRemote`, which callers report and survive.

**`furrow claim <ref> [--no-checkout] [--force]`.**
1. Fetches `<remote> <base>`.
2. Refuses a step that isn't ready (see FUR-0002 for the states) and says why: "FUR-0007.1 waits on
   FUR-0004.1." It also refuses a step whose `touches` overlap a claimed step, naming the clash.
   `--force` skips only the overlap check.
3. Makes a claim commit on `<remote>/<base>` that changes nothing and whose message is unique to
   this claim (`Claim FUR-0003.1: <title>` plus a random `furrow-claim:` line), and creates the
   branch on the remote with
   `git push <remote> <sha>:refs/heads/<branch> --force-with-lease=refs/heads/<branch>:`. The empty
   lease means "only if it doesn't exist yet". If the push is refused because the branch exists, it
   exits 3 with "FUR-0003.1 is already claimed (step/0003-1). Run `furrow next` for another step."
4. Unless `--no-checkout`, runs `git switch -c <branch> <sha>` and sets the upstream. With
   uncommitted changes in the tree, it refuses before step 3.
5. Prints the claim and a suggested PR title: `[FUR-0003.1] <step title>`.

**`furrow release <ref>`** deletes the remote branch (`git push <remote> --delete`). Releasing a step
that isn't claimed says so and exits 0.

**`furrow claims [--stale DURATION] [--json]`** lists every claim with its age, which is the time since
the branch's last commit. To read the age it fetches the claim branches into
`refs/furrow/claims/*`. A claim is:
- `leftover` when its step is merged on base (the branch outlived its PR);
- `stale` when its last commit is older than `--stale` (default `24h`);
- `active` otherwise.

Exit codes: 0 ok, 1 error, 3 already claimed.

## Acceptance criteria

- [x] AC-1: `Branch` fills the pattern, and `List` reads only matching branches from a real bare remote in a temp dir (test: TestBranchAndList)
- [x] AC-2: Claiming a ready step creates the remote branch at base and checks it out (test: TestClaimCreatesBranch)
- [x] AC-3: When two clones claim the same step, exactly one succeeds and the other exits 3 with the message (test: TestClaimRace)
- [x] AC-4: A waiting, blocked or overlapping step is refused with the reason, and a dirty tree is refused before anything is pushed (test: TestClaimRefusals)
- [x] AC-5: `release` deletes the branch, and releasing an unclaimed step exits 0 (test: TestRelease)
- [x] AC-6: `claims` marks leftover, stale and active claims (test: TestClaimsAges)
- [x] AC-7: The claim flow reads well to the owner, including the already-claimed message (human: run `furrow claim` twice on a test repo)

## Out of scope

- Opening the PR. Skills do it with the forge's own tools.
- Showing claims in `next`. That's FUR-0002.2.

## Decisions

- 2026-11-01: **Each claim pushes its own commit.** Pushing base itself fails INV-5. When the branch
  already exists at the same commit, git says "Everything up-to-date", exits 0, and the lease never
  fires, so two agents claiming from the same base would both win. That's the common race. An
  empty commit with a random line makes every claim a different value, so the second push is
  refused (`stale info`). Found while hand-claiming FUR-0005.1 twice. Squash merges drop the commit.
- 2026-11-01: Readiness is read from the working tree's specs after fetching base. An agent on a
  stale checkout could see a step as ready that base has since merged. The claim still can't be
  doubled, so that's acceptable. `furrow-next` tells agents to start from an up-to-date base.
- 2026-11-01: `internal/gitx/gittest` makes real repos with bare remotes for tests, with a fixed
  identity and `GIT_CONFIG_GLOBAL=/dev/null` so a user's git config can't change results.
- 2026-11-01: `TestClaimRace` passed 20 runs in a row under `-race`.
- 2026-11-03: AC-7 signed off by Deven Jarvis.
