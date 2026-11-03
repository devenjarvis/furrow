@AGENTS.md

## Claude Code specifics

- The skills in `skills/` (linked from `.claude/skills/`) are the ones Furrow ships: `furrow-next`, `furrow-implement`, `furrow-verify`, `furrow-write`, `furrow-swarm`, `furrow-signoff`.
- When you run several agents, give each a different assignment from `furrow next --agents N`, each in its own worktree.
- Claude Code sets `CLAUDECODE=1`, so `furrow signoff` needs `--by NAME`, the person who checked: it won't take a name from git in your session. Sign off only when the owner asks, with the `furrow-signoff` skill, and merge the sign-off PR only on their own words (INV-3).
