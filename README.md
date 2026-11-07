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

## Use Furrow in your repos

Use the Claude Code plugin from `main`, and the CLI built from `main`, so the skills and the CLI
always match.

1. **Cloud sessions.** Add this to the cloud environment's setup script. It runs in each fresh
   container, so every session gets `main`'s skills, agents and CLI together. (Cloud sessions
   ignore plugins a repo declares in its settings, so the setup script is how they get it.)

   ```sh
   claude plugin marketplace add devenjarvis/furrow || true
   claude plugin install furrow@furrow || true
   GOBIN=/usr/local/bin go install github.com/devenjarvis/furrow/cmd/furrow@main || true
   ```

2. **Local sessions.** `furrow init` writes the plugin into `.claude/settings.json`. Claude Code
   installs it once the folder is trusted, and auto-update keeps it current: the plugin has no
   `version`, so every commit to `main` is an update.

   ```json
   {
     "extraKnownMarketplaces": {
       "furrow": {
         "source": { "source": "github", "repo": "devenjarvis/furrow" },
         "autoUpdate": true
       }
     },
     "enabledPlugins": { "furrow@furrow": true }
   }
   ```

3. **Don't copy the skills or agents into the repo.** A project's `.claude/agents/furrow-*` take
   priority over the plugin's, so a copy hides its fixes, model pins included, and copied skills
   show up twice, as `/furrow-swarm` and `/furrow:furrow-swarm`. `furrow doctor` lists any copies.

Read `docs/product.md` for the idea and its invariants, and `specs/README.md` for the standard.
Furrow is built with Furrow: see `specs/`.
