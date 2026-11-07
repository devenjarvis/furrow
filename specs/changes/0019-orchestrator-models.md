---
id: FUR-0019
title: A Haiku orchestrator, Opus builders and verifiers, and merges only on green CI
status: accepted
area: agents
priority: p1
human_review: required
invariants: [INV-3]
context:
  - docs/product.md
  - specs/changes/0018-lean-sessions.md
steps:
  - n: 1
    title: Pin the agents' models, run the swarm on Haiku, and merge only on green CI
    after: []
    touches:
      - agents/**
      - skills/furrow-swarm/**
      - specs/changes/0018-lean-sessions.md
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6, AC-7]
    size: s
    merged: false
---

# FUR-0019: A Haiku orchestrator, Opus builders and verifiers, and merges only on green CI

## Why

Swarms still cost $58–100 a wave, all on Opus. Since FUR-0018, the orchestrator mostly reads short
hand-backs and verdicts, so it is 20–35% of a swarm's cost for little thinking. In a tabletop of 14
swarm moments, two Haiku runs scored 27 and 28 of 28, with no unsafe move. Opus scored 28 and Sonnet
27. So the orchestrator can run on Haiku while the builders and verifiers stay on Opus. Separately,
the Opus orchestrator merged 4 Fetch PRs while their latest CI run was red, and the skill never said
not to.

## What

### Which model runs what

Each agent names its own model, so the session's model no longer decides it. Today builders and
verifiers have no `model:` line, so a Haiku session would run them on Haiku too.

| Who | Model | Where it's set |
| --- | --- | --- |
| The orchestrator (the `furrow-swarm` session) | Haiku | The owner starts the session with `/model haiku` |
| `furrow-builder` | `model: opus` | `agents/furrow-builder.md` front matter |
| `furrow-verifier` | `model: opus` | `agents/furrow-verifier.md` front matter |
| `furrow-chores` | `model: sonnet` (unchanged) | `agents/furrow-chores.md` front matter |

`furrow-swarm` gains a short section, `## Models`, that says:

- The orchestrator session runs on Haiku. Agents set their own models in their front matter, so
  building and verifying stay on Opus whatever the session runs on.
- If the session isn't on Haiku, it says so to the owner once, in one line
  (`This swarm runs on <model>. /model haiku makes the orchestrator cheaper; the agents stay on
  Opus.`), and carries on. It never stops for this.
- A project overrides any agent's model by copying the agent into its own `.claude/agents/`, as
  FUR-0018 says.

### Merging only on green CI

Step 3 of the loop changes. On `ready to merge: yes`, the orchestrator checks CI on the PR's head
commit before merging:

- **Every required check passed on the head commit:** merge through the repo's PR flow, as today.
- **A check failed on the head commit:** don't merge. Send a `furrow-chores` agent to read the
  failure. A failure in the step's code goes back to the builder, and its next hand-back gets a
  fresh verifier. A failure the chore shows is unrelated to the diff (a known flaky test) may be
  re-run once on the same commit. Merge only when that run passes.
- **Checks still running:** wait for them. Never merge on an older commit's green run.
- **The repo has no CI:** the verifier's re-run of the repo's check is the gate, as today.

A merge after a fix always follows a fresh `ready to merge: yes` on the new head. The orchestrator
never merges a step whose fix wasn't re-verified.

### FUR-0018's AC-12

FUR-0018 AC-12 says builders and verifiers have no `model:` line, and its check will fail once this
lands. The step adds one line to FUR-0018's `## Decisions` saying FUR-0019 supersedes that part of
AC-12. It leaves the criterion's text and its tick alone (INV-4).

## Acceptance criteria

- [ ] AC-1: `agents/furrow-builder.md` and `agents/furrow-verifier.md` have `model: opus` in their front matter. (check: grep -q "^model: opus$" agents/furrow-builder.md && grep -q "^model: opus$" agents/furrow-verifier.md)
- [ ] AC-2: `agents/furrow-chores.md` still has `model: sonnet`. (check: grep -q "^model: sonnet$" agents/furrow-chores.md)
- [ ] AC-3: `furrow-swarm` has a `## Models` section saying the orchestrator runs on Haiku, the agents set their own models, and a session on another model says so once and carries on. (check: grep -q "^## Models" skills/furrow-swarm/SKILL.md && grep -q "/model haiku" skills/furrow-swarm/SKILL.md)
- [ ] AC-4: `furrow-swarm` merges a step only when the verdict is `ready to merge: yes` and every check passed on the PR's head commit; a failed or running check means no merge, and a failure goes to `furrow-chores`. (check: grep -q "head commit" skills/furrow-swarm/SKILL.md)
- [ ] AC-5: FUR-0018's `## Decisions` has a line saying FUR-0019 supersedes AC-12's "no `model:` line", and AC-12's text is unchanged. (check: grep -q "FUR-0019 supersedes" specs/changes/0018-lean-sessions.md)
- [ ] AC-6: `make check` passes. (check: make check)
- [ ] AC-7: In one real swarm wave with the orchestrator on Haiku, the briefs were right (stacked steps included), every merge followed a `yes` verdict and green CI on the head commit, no spec or sign-off PR was merged, findings went back to builders, and the session ended with a handoff a fresh session could start from. (human: the orchestrator's transcript of one wave, and its cost against a recent Opus wave)

## Out of scope

- Running builders or verifiers on a cheaper model, for any step size.
- Choosing the session's model from code or settings. The owner picks it with `/model`.
- Reading CI state in the `furrow` binary. The orchestrator reads it with the forge's tools, as it
  already merges with them (INV-2).
- How repos that use Furrow get the plugin's updates. That is a separate spec.
- Changing furrow-next, furrow-implement, furrow-verify, furrow-write or furrow-signoff.

## Open questions

- Haiku or Sonnet for the orchestrator? Proposed: Haiku, from the tabletop. If AC-7's wave shows a slip, a follow-up switches the skill's advice to Sonnet.
- Pin `opus` (the alias) or a full model ID? Proposed: the alias, so the agents follow the newest Opus without a spec change.

## Decisions
