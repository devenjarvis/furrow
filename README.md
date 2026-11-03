# furrow

Specs in, parallel pull requests out. Furrow lets a swarm of coding agents build one repo at once
without stepping on each other.

- Specs are plain Markdown in `specs/`. Each one is split into **steps**, and each step is one PR
  with `after` (what it waits for) and `touches` (the paths it may change).
- `furrow next --agents 4` hands out four steps that are unblocked and don't overlap.
- `furrow claim` takes a step by creating its branch on the remote. That's atomic, so one agent wins.
- `furrow territory` proves a diff stayed inside its step.
- Work unblocks on merge. What only a person can judge waits in `furrow review`. `furrow signoff`
  ticks it in a sign-off PR, from any session, and the owner merging that PR is the sign-off.
- Finished specs are archived. Each area keeps a **living spec** of how it works now.

It's one Go binary that needs only `git`, plus a Claude Code plugin with skills for single agents
and for swarms.

```sh
go install github.com/devenjarvis/furrow/cmd/furrow@latest
furrow init
```

Read `docs/product.md` for the idea and its invariants, and `specs/README.md` for the standard.
Furrow is built with Furrow: see `specs/`.
