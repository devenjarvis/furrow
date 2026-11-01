@AGENTS.md

## Claude Code specifics

- The skills in `skills/` (linked from `.claude/skills/` once FUR-0007 lands) are the ones Furrow ships: `furrow-next`, `furrow-implement`, `furrow-verify`, `furrow-write`, `furrow-swarm`.
- When you run several agents, give each a different assignment from `furrow next --agents N`, each in its own worktree.
- Claude Code sets `CLAUDECODE=1`, so `furrow signoff` refuses to run in your session. That's on purpose (INV-3).
