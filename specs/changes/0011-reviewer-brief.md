---
id: FUR-0011
title: A reviewer's brief for every step's PR
status: draft
area: brief
priority: p1
human_review: required
invariants: [INV-2, INV-3, INV-6]
context:
  - docs/product.md
  - specs/README.md
  - specs/areas/territory.md
  - specs/areas/lifecycle.md
  - specs/areas/agents.md
  - internal/territory/territory.go
steps:
  - n: 1
    title: furrow brief writes the reviewer's brief
    after: []
    touches:
      - cmd/furrow/brief.go
      - internal/brief/**
      - internal/config/**
    covers: [AC-1, AC-2, AC-3, AC-4, AC-5, AC-6, AC-7, AC-8, AC-9]
    size: m
    merged: false
  - n: 2
    title: furrow brief --check reads a filled-in brief
    after: [1]
    touches:
      - cmd/furrow/brief.go
      - internal/brief/check.go
      - internal/brief/check_test.go
    covers: [AC-10, AC-11]
    size: s
    merged: false
  - n: 3
    title: furrow-next and furrow-implement write the brief
    after: [1]
    touches:
      - skills/furrow-next/**
      - skills/furrow-implement/**
    covers: [AC-12, AC-13]
    size: s
    merged: false
  - n: 4
    title: furrow-verify checks the brief against the diff
    after: [2]
    touches:
      - skills/furrow-verify/**
    covers: [AC-14]
    size: s
    merged: false
  - n: 5
    title: Specs that change structure draw a diagram
    after: []
    touches:
      - skills/furrow-write/**
      - specs/README.md
      - internal/scaffold/assets/README.md
    covers: [AC-15]
    size: s
    merged: false
  - n: 6
    title: furrow init offers the visual key
    after: [1]
    touches:
      - internal/scaffold/scaffold.go
      - internal/scaffold/scaffold_test.go
    covers: [AC-16]
    size: s
    merged: false
  - n: 7
    title: Fold the brief into the areas
    after: [2, 3, 4, 5, 6]
    touches:
      - specs/areas/agents.md
      - specs/areas/specs.md
      - specs/areas/scaffolding.md
    covers: [AC-17, AC-18]
    size: s
    merged: false
---

# FUR-0011: A reviewer's brief for every step's PR

## Why

Agents write most of the code now, so a PR description that retells the diff is wasted on its
reviewer. A person reviewing a step needs to know what behaviour changed, where to look first,
what the step did that the spec didn't say, what it looks like, and what only they can judge.
Furrow already knows most of that from the spec and git. `furrow brief` writes it down, and leaves
the agent only the parts that need judgment.

## What

### `furrow brief [--step REF] [--base REF] [--json]`

Prints a Markdown brief for one step, ready to paste as the PR description. The step and base
are found as `furrow territory` finds them: `--step`, or the current branch read through the
config's `branch` pattern, and `--base`, or `<remote>/<base>`, not fetched. "Changed files" means
what `territory` means: the merge base to the working tree, untracked files included, sorted.

The brief has these sections, always in this order. A section marked *when* is left out
otherwise. Parts the agent must write are HTML comments that start `<!-- furrow:`, so they don't
render if forgotten, and `--check` finds them.

1. **`## Behaviour`**: `<!-- furrow: before → after, in at most 80 words. Behaviour, not code. -->`
2. **`## Diagram`** *when the spec's `## What` holds a ```` ```mermaid ```` block*: each such
   block, copied as is, under `<!-- furrow: mark what this step adds or changes with classDef
   added, or say it matches the spec as drawn -->`.
3. **`## Where to look`**: changed files in four groups, each sorted, empty groups left out:
   - **Code**: everything not in another group, under `<!-- furrow: name the files or functions
     that carry the decision, most important first -->`.
   - **Visual**: files matching the config's `visual` globs.
   - **Tests**: files matching `**/*_test.*`, `**/*.test.*`, `**/*.spec.*`, `**/test/**`,
     `**/tests/**`, `**/__tests__/**` or `**/testdata/**`.
   - **Specs**: files under the config's `specs` folder.

   A file in more than one group goes in the first that matches, in the order Specs, Tests,
   Visual, Code.
4. **`## Visual proof`** *when a changed file matches `visual`*: the matching files, then a table
   to fill in:

   ```
   | Screen | Before | After |
   | --- | --- | --- |
   | <!-- furrow: screen --> | <!-- furrow: image, or "new" --> | <!-- furrow: image --> |
   ```
5. **`## Departures from the spec`**: facts, no placeholders.
   - Each line added to the spec's `## Decisions` since the merge base (all of them if the spec
     file is new).
   - Each criterion whose text, ignoring its box, differs from the merge base: `AC-n's text
     changed. Only a person changes a criterion (INV-4).`
   - Each changed file outside the step's territory, with: `Explain each under "## Territory".`
     The brief never writes these paths under a `## Territory` heading itself, because that would
     make `furrow territory --body-file` accept them unexplained.
   - With none of these: `None.`
6. **`## Proven`**: each criterion the step covers that isn't `human:`, as `- [x] AC-n: <text>`
   or `- [ ] AC-n: <text>` from the spec's box, then `  Evidence: <!-- furrow: test name, command
   and output, or screenshot -->`.
7. **`## Needs you`**:
   - each `human:` criterion the step covers, with its proof text as what to look at;
   - each invariant in the spec's `invariants:`, as `INV-n: <name>` read from the product file;
   - `human_review: required` when the spec says so.

   With none of these: `Nothing. Every criterion has a test or a check.`
8. **`## Not done`**: `<!-- furrow: what this step leaves out, and its risks. "Nothing." is an
   answer. -->`

The output is the same for the same repo state (INV-6). It has no dates. With `--json` it prints
`{"step", "base", "sections": [{"heading", "markdown"}], "files": {"code", "visual", "tests",
"specs"}, "decisions_added", "criteria_changed", "outside", "needs_you"}`, every list sorted and
never `null`.

It reads only the working tree and git (INV-2), and posts nothing anywhere. Off a step branch with
no `--step` it exits 1: "Not on a step branch. Pass `--step FUR-NNNN.N`." An unknown step exits 1
and names it. A base that doesn't resolve exits 1: "Can't find origin/main. Run `git fetch origin`,
or pass `--base`."

### `visual` in `.furrow.yml`

```yaml
visual: [web/src/**, ui]   # globs or aliases; a diff touching these needs visual proof
```

A list of globs, resolved through `aliases` like `touches`. It defaults to empty, and then no brief
has a Visual proof section.

### `furrow brief --check FILE [--step REF] [--base REF] [--json]`

Reads a filled-in brief (the PR description) and exits 0 if it's complete, else 1 with one line
per problem, in section order, each saying what to do:

- A section the brief for this step would have is missing: "No `## Visual proof` section. Run
  `furrow brief` and paste it."
- A `<!-- furrow:` placeholder is left anywhere: "`## Behaviour` still has a placeholder. Write it."
- `## Behaviour` is over 80 words: "`## Behaviour` is 112 words. Keep it to 80."
- `## Diagram` is required and has no ```` ```mermaid ```` block.
- `## Visual proof` has no table row, or a row whose After cell has no image (`![…](…)`) or link
  (`[…](…)`), or whose Before cell has neither nor the word `new`: "Row 2 of `## Visual proof`
  has no after image."

It only reads. It never ticks a box or edits the spec. `--json` prints `{"ok", "problems"}`.

### Skills

- **furrow-next** opens the draft PR with `furrow brief`'s output as its body.
- **furrow-implement** fills in every placeholder before handing back, and runs
  `furrow brief --check` next to `furrow territory --body-file`. The Behaviour is what a user or
  caller sees, not which files changed. It captures visual proof with the repo's own tools, as its
  `AGENTS.md` says, before and after, and never ticks a `human:` criterion because a screenshot
  shows it (INV-3). It keeps a spec's diagram current, and on a spec's last step folds the diagram
  into the area. Every node in a diagram names a real file, package, type or function.
- **furrow-verify** runs `furrow brief --check`, then reads the brief against the diff: the
  Behaviour describes what the code does; every diagram node and edge exists in the code; each
  image shows the screen named. A claim the diff doesn't bear out is a finding, as a failed test
  would be.
- **furrow-write** draws a ```` ```mermaid ```` diagram in `## What` for a spec that changes how
  parts connect or how something flows (a new component, a changed call path, a state machine),
  showing only the parts that change, with new ones marked by `classDef added`. A spec that
  doesn't change structure has none.

### The standard

`specs/README.md`, and the copy `furrow init` writes, say: a spec that changes structure draws it
in `## What` as a ```` ```mermaid ```` diagram, and an area may keep one in `## Behaviour`, kept
current by the fold. `furrow init` writes `# visual: []` with a one-line comment into
`.furrow.yml`.

## Acceptance criteria

- [ ] AC-1: The brief has the eight sections in order, leaves out Diagram and Visual proof when they don't apply, and puts every agent-written part in a `<!-- furrow:` comment (test: TestBriefSections)
- [ ] AC-2: Changed files are grouped Specs, Tests, Visual, Code, each file in the first group it matches, sorted (test: TestBriefGroupsFiles)
- [ ] AC-3: A spec with mermaid blocks in `## What` gets a Diagram section with each block copied as is (test: TestBriefCopiesSpecDiagram)
- [ ] AC-4: A diff touching a `visual` glob or alias gets a Visual proof section with the matching files and the table, and with `visual` empty there is none (test: TestBriefVisualProof)
- [ ] AC-5: Departures lists Decisions lines added since the merge base, criteria whose text changed, and files outside territory, or `None.` (test: TestBriefDepartures)
- [ ] AC-6: Files outside territory never appear under a `## Territory` heading, so `furrow territory --body-file` on the raw brief still reports them (test: TestBriefDoesNotExplainTerritory)
- [ ] AC-7: Proven and Needs you list the covered criteria by kind, the spec's invariants by name, and required human review (test: TestBriefProvenAndNeedsYou)
- [ ] AC-8: The same repo state gives byte-identical output, and `--json` has every documented key with lists never null (test: TestBriefDeterministicAndJSON)
- [ ] AC-9: Off a step branch, an unknown step, and a missing base each exit 1 with their message (test: TestBriefFailures)
- [ ] AC-10: `--check` reports a missing section, a leftover placeholder, a Behaviour over 80 words, a missing mermaid block, and a visual row without images, each on its own line, and exits 1 (test: TestBriefCheckProblems)
- [ ] AC-11: `--check` exits 0 on a complete brief, changes no file, and `--json` prints `ok` and `problems` (test: TestBriefCheckComplete)
- [ ] AC-12: furrow-next opens the PR with the brief, and furrow-implement runs `furrow brief --check` before `furrow done` (check: grep -q "furrow brief" skills/furrow-next/SKILL.md && grep -q "furrow brief --check" skills/furrow-implement/SKILL.md)
- [ ] AC-13: furrow-implement says a screenshot is evidence for the owner and never a reason to tick a `human:` criterion (check: grep -n -i "screenshot" skills/furrow-implement/SKILL.md | grep -q -i "human")
- [ ] AC-14: furrow-verify runs `furrow brief --check` and reads the Behaviour, diagram and images against the diff (check: grep -q "furrow brief --check" skills/furrow-verify/SKILL.md && grep -q -i "mermaid\|diagram" skills/furrow-verify/SKILL.md)
- [ ] AC-15: The standard and furrow-write say when a spec draws a mermaid diagram, and the standard's two copies agree (check: grep -q -i mermaid specs/README.md && grep -q -i mermaid skills/furrow-write/SKILL.md && grep -q -i mermaid internal/scaffold/assets/README.md)
- [ ] AC-16: `furrow init` writes a commented `visual` line into `.furrow.yml`, and the file still parses (test: TestInitWritesVisualKey)
- [ ] AC-17: A brief for a real step of this repo tells a reviewer what changed and where to look faster than its diff does (human: run `furrow brief` on a merged step of this spec and read it before the diff)
- [ ] AC-18: The skills' rules for the Behaviour, diagrams and visual proof match how the owner wants PRs reviewed (human: read the diffs to furrow-next, furrow-implement, furrow-verify and furrow-write)

## Out of scope

- Posting, updating or reading a PR through a forge's API, or uploading images. Furrow prints
  Markdown; the agent puts it in the PR with the forge's tools (INV-2).
- Taking screenshots or recordings. That's each repo's tooling, named in its `AGENTS.md`.
- Rendering or validating mermaid syntax. That needs a dependency (INV-7).
- Writing a forge PR template, such as `.github/pull_request_template.md`. The brief is the
  template.
- Making `furrow done` or `furrow lint` require a complete brief. `--check` is advice the skills
  run, for now.
- A configurable list of test globs.

## Open questions

- Is `brief` the right name? Proposed: yes. `guide` reads like docs, and `pr` suggests it talks to
  a forge.
- Should `furrow done` refuse when `--check` fails? Proposed: not yet. See how agents do with
  the skills first, then decide in its own spec.
- Is 80 words the right cap for Behaviour? Proposed: yes, about three sentences.
- Does required visual proof belong in config globs, or should a spec opt in per step? Proposed:
  globs. The diff decides, so an agent can't skip it by leaving a flag off.

## Decisions
