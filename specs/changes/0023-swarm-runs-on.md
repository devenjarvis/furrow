---
id: FUR-0023
title: Swarms keep going until only the owner can unblock them
status: accepted
area: agents
priority: p1
human_review: required
invariants: [INV-3]
context:
  - docs/product.md
  - specs/areas/agents.md
  - specs/changes/0018-lean-sessions.md
  - specs/changes/0019-orchestrator-models.md
  - skills/furrow-swarm/SKILL.md
steps:
  - n: 1
    title: furrow-swarm runs wave after wave, parks what needs the owner, and stops only when stuck
    after: []
    touches:
      - skills/furrow-swarm/**
      - skills/furrow-implement/**
      - specs/areas/agents.md
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6, AC-7, AC-8, AC-9]
    size: s
    merged: true
---

# FUR-0023: Swarms keep going until only the owner can unblock them

## Why

FUR-0018 made the swarm orchestrator stop after every wave, and past about 200K tokens, because an
Opus orchestrator re-reading a long history was most of a swarm's cost. Since FUR-0019 the
orchestrator runs on Haiku, so a long session is cheap, and stopping after each wave now just means
the owner has to start the next session by hand. A swarm should keep building, wave after wave,
without asking the owner anything, until nothing is left that it can do alone.

## What

All of this is in `skills/furrow-swarm/SKILL.md` unless it names another file. No command changes.

**No wave or token stop.** The `## Hand off` section loses two of its three triggers: "a wave has
merged and `furrow next` offers new work" and "the conversation is over about 200K tokens". After a
merge or a hand-back the orchestrator runs `furrow next --agents <free slots> --json` and hands out
the next work in the same session, across as many waves as the queue has.

**Compaction.** When the conversation is compacted (Claude Code summarises it as it gets long),
the orchestrator runs `furrow status` before it hands out anything else, and rebuilds the state of
play from it and from the agents still out. Everything it needs is in the repo and the remote
(INV-1), so nothing is lost but chat.

**Park, don't ask.** While any work can still move, the orchestrator never stops to ask the owner
a question and never waits for an answer. What only the owner can settle is **parked**: the
orchestrator notes it, in one line, in a parked list it keeps in the conversation, leaves that step
alone, and carries on with everything else. Parked are:

- a spec problem a builder reports;
- a step whose parent's PR was closed without merging;
- a step that has come back `ready to merge: no` three times (its builder has had three fixes);
- a CI failure a `furrow-chores` agent can't explain, or that fails again after its one re-run;
- anything blocked outside Furrow: a secret, a sign-in, a broken tool.

A parked step's claim stays, so no one else picks it up, unless the problem is the step itself
(a spec problem, a closed parent); then the orchestrator runs `furrow release <ref>` so it reads
as free once the owner fixes it. The "tell the owner once" model line under `## Models` stays: it's
a line in the report, not a question.

**`skills/furrow-implement/SKILL.md`.** "If the parent's PR was closed without merging, stop and
tell the owner" becomes "stop and report it under Spec problem": in a swarm the hand-back carries it
to the orchestrator, which parks it.

**When it stops.** `## Hand off` and `## Stop when` become one rule. The orchestrator stops only
when every one of these holds:

- `furrow next` offers nothing (or nothing that doesn't overlap a parked step's claim);
- no builder, verifier or chores agent is out;
- no step PR is waiting on its CI or on a verifier.

It then writes the handoff (at most 10 lines, as today, ending `Start a new session with: Run
furrow status, then furrow-swarm.`) and the report, which gains a first section: **Parked**, one
line each, with the ref and what the owner needs to do. The rest of the report stays as it is.

**Unchanged.** The 15-line hand-back, 25-line verdict and 10-line chore reports; the models; the
merge gate (a fresh `yes` and green CI on the head commit); and every rule about `human:` criteria,
sign-offs and spec PRs (INV-3).

`specs/areas/agents.md` says the same in its furrow-swarm entry.

## Before and after

**Before:** The orchestrator stops after every wave, and the owner restarts it by hand.

```
Wave 1 merged: FUR-0022.1, FUR-0022.2. furrow next offers 3 more.
Start a new session with: Run furrow status, then furrow-swarm.
```

**After:** It runs on through every wave, and stops once only the owner can unblock it.

```
Merged: FUR-0022.1, FUR-0022.2, FUR-0024.1, FUR-0024.2, FUR-0025.1
Parked:
  FUR-0025.2  spec problem: AC-4 contradicts INV-6 (PR #70)
Start a new session with: Run furrow status, then furrow-swarm.
```

## Acceptance criteria

- [x] AC-1: furrow-swarm no longer stops at a wave boundary or at a token count. (check: ! grep -Eq "200K|wave has merged" skills/furrow-swarm/SKILL.md)
- [x] AC-2: furrow-swarm says to run `furrow status` after the conversation is compacted, before handing out more work. (check: grep -Eq "compact.*furrow status|furrow status.*compact" skills/furrow-swarm/SKILL.md)
- [x] AC-3: furrow-swarm says the orchestrator never stops to ask the owner while work can move, and parks what only the owner can settle. (check: grep -q "Park, don't ask" skills/furrow-swarm/SKILL.md)
- [x] AC-4: furrow-swarm lists the five kinds of parked work, including a step that came back `no` three times. (check: grep -q "three times" skills/furrow-swarm/SKILL.md)
- [x] AC-5: furrow-swarm releases a parked step whose problem is the step itself, and keeps the claim otherwise. (check: grep -q "furrow release <ref>" skills/furrow-swarm/SKILL.md)
- [x] AC-6: furrow-swarm stops only when nothing is offered, no agent is out and no PR waits on CI or a verifier, and its report starts with a Parked section. (check: grep -q "^- no step PR is waiting on its CI or on a verifier" skills/furrow-swarm/SKILL.md && grep -q "Parked" skills/furrow-swarm/SKILL.md)
- [x] AC-7: furrow-implement reports a closed parent PR under Spec problem instead of telling the owner. (check: ! grep -q "stop and tell the owner" skills/furrow-implement/SKILL.md)
- [x] AC-8: The agents area describes the new stop rule and no longer mentions a wave boundary or 200K tokens. (check: ! grep -Eq "200K|wave boundary" specs/areas/agents.md)
- [ ] AC-9: A real swarm ran at least two waves in one session without asking the owner anything, and its report listed what it parked. (human: the orchestrator's transcript, from the first `furrow next` to the report)

## Out of scope

- Waking the orchestrator when the owner merges a PR or answers a question (PR subscriptions,
  scheduled check-ins). It stops; the owner starts it again.
- Changing the report line limits, the models or the merge gate.
- Any CLI change. `furrow status` and `furrow next` already give what a compacted session needs.
- Letting the orchestrator decide anything that's the owner's: specs, sign-offs, criteria (INV-3,
  INV-4).
- Editing FUR-0018's or FUR-0019's criteria.

## Open questions

- Keep the 15-, 25- and 10-line limits on what agents report back?
  Proposed: yes. They don't interrupt the owner, and short reports make each compaction cheap and
  keep a long Haiku session sharp.
- When the only thing left is waiting on the owner (PRs to review, a full stack), should the
  orchestrator stop, or stay up and wait for merges?
  Proposed: stop with the report. Waiting needs a wake-up mechanism, which is out of scope; a
  later spec can add it.
- How many `ready to merge: no` round trips before a step is parked?
  Proposed: three, so a builder gets three fixes before the owner is pulled in.
- FUR-0018 AC-15 and FUR-0019 AC-7 ask to see a swarm stop with a handoff after one wave. How do
  they get signed off once this lands?
  Proposed: sign them off from a transcript of a wave run before this merges, or from this
  spec's own AC-9 run, which still ends with a handoff; their text stays as it is (INV-4).

## Decisions

- 2026-11-14: Chose to hold the parked list, not unanswered questions, in the handoff, since the orchestrator no longer asks the owner anything.
- 2026-11-14: Chose to keep "spec problems" in the report's last line, as the spec says the rest of the report stays; Parked comes first.
- 2026-11-14: Chose to claim with `--force`: step/0022-2 is a leftover branch of FUR-0022.2, already merged in PR #62.
