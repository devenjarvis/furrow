---
name: furrow-write
description: Turn an idea into a draft Furrow spec, split into steps that can run in parallel, that passes furrow lint. Use when asked to spec, plan or write up a feature or change in a repo with a .furrow.yml.
---

# Write a spec

1. Read `specs/README.md` (the standard), `docs/product.md` (with its invariants), and the area specs
   in `specs/areas/`. Run `furrow list` to see what exists. If an open spec already covers most of
   the idea, propose an amendment to it instead.
2. Pick the area: the living spec this changes. If none fits, create one with
   `furrow area <name> --title …`.
3. `furrow new <slug> --title "…" --area <area>`. It picks the number and starts a draft.
4. Fill it in for a smart stranger:
   - **Why**: two to four sentences, for a user.
   - **What**: behaviour at the boundaries: commands, types, files, output, and every failure mode.
     Examples beat adjectives. If the change alters how parts connect or how something flows (a new
     component, a changed call path, a state machine), draw it here as a ```` ```mermaid ````
     diagram: only the parts that change, new ones marked with `classDef added`, and every node a
     real file, package, type or function. A spec that doesn't change structure has no diagram.
   - **Acceptance criteria**: `- [ ] AC-n: … (test: Name)`, `(check: command)` or
     `(human: what to look at)`. Observable, one behaviour each.
   - **Out of scope**: what a reasonable agent might do and mustn't.
5. **Split into steps** that one agent can finish in one session (`size: s` or `m`; `l` must be split):
   - give each step the smallest `touches` that does the job. Steps with overlapping touches can't
     run together, so separate them by file, for example one command per file;
   - set `after` honestly: `N` for a step of this spec, `PREFIX-NNNN.N`, or `PREFIX-NNNN` for all of
     one. A hidden dependency, such as needing a helper another step creates, means `after` is wrong;
   - `covers` lists the criteria each step makes true, and every criterion is covered;
   - leave room in `touches` for the step's own tests.
6. **Invariants.** List every invariant the change could break under `invariants:`. That makes it
   `human_review: required`, and so do UI changes and anything that acts as the user.
7. `furrow lint` must pass. Then `furrow graph` shows how the steps fan out. Rework them if
   everything is serial.
8. Leave `status: draft`, and list the questions the owner must answer under `## Open questions`,
   each with a proposed answer. The owner accepts a spec by merging it with `status: accepted`. Never
   accept your own.
