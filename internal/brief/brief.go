// Package brief writes the reviewer's brief for a step's PR: what Furrow already knows from the
// spec and git, with HTML-comment placeholders for the parts that need the agent's judgment.
package brief

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/territory"
)

// Placeholder opens every part of a brief the agent must write. It's an HTML comment, so it doesn't
// render if forgotten.
const Placeholder = "<!-- furrow:"

// Section headings, in the order a brief has them.
const (
	Behaviour   = "Behaviour"
	Diagram     = "Diagram"
	WhereToLook = "Where to look"
	VisualProof = "Visual proof"
	Departures  = "Departures from the spec"
	Proven      = "Proven"
	NeedsYou    = "Needs you"
	NotDone     = "Not done"
)

// testGlobs are the files grouped as Tests.
var testGlobs = []string{"**/*_test.*", "**/*.test.*", "**/*.spec.*", "**/test/**", "**/tests/**", "**/__tests__/**", "**/testdata/**"}

// Options say which step's brief to write. Empty fields are worked out as furrow territory does.
type Options struct {
	Step string
	Base string
}

// Section is one "## Heading" of the brief and the Markdown under it.
type Section struct {
	Heading  string `json:"heading"`
	Markdown string `json:"markdown"`
}

// Files are the changed files in their Where to look groups. Each file is in exactly one.
type Files struct {
	Code   []string `json:"code"`
	Visual []string `json:"visual"`
	Tests  []string `json:"tests"`
	Specs  []string `json:"specs"`
}

// Brief is a step's brief: its sections, and the facts they were built from.
type Brief struct {
	Step            string    `json:"step"`
	Base            string    `json:"base"`
	Sections        []Section `json:"sections"`
	Files           Files     `json:"files"`
	DecisionsAdded  []string  `json:"decisions_added"`
	CriteriaChanged []string  `json:"criteria_changed"`
	Outside         []string  `json:"outside"`
	NeedsYou        []string  `json:"needs_you"`
}

// Build works out the step and base, reads the spec and the diff, and writes the brief. It reads
// only the working tree and git.
func Build(g gitx.Git, repo *spec.Repo, opt Options) (*Brief, error) {
	st, err := territory.StepFromBranch(g, repo, opt.Step)
	if err != nil {
		return nil, err
	}
	base, err := ResolveBase(g, repo, opt.Base)
	if err != nil {
		return nil, err
	}
	mb, err := g.Run("merge-base", base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("couldn't find where HEAD left %s: %v", base, err)
	}
	changed, err := territory.ChangedFiles(g, base)
	if err != nil {
		return nil, err
	}
	b := &Brief{
		Step:            st.Ref().String(),
		Base:            base,
		Files:           Group(repo, changed),
		DecisionsAdded:  []string{},
		CriteriaChanged: []string{},
		Outside:         []string{},
		NeedsYou:        []string{},
	}
	for _, f := range changed {
		if !territory.Inside(repo, st, f) {
			b.Outside = append(b.Outside, f)
		}
	}
	if old, ok := fileAt(g, mb, st.Spec.File); ok {
		b.DecisionsAdded = added(decisions(old), decisions(st.Spec.Body))
		b.CriteriaChanged = changedCriteria(old, st.Spec)
	} else {
		b.DecisionsAdded = added(nil, decisions(st.Spec.Body))
	}

	b.add(Behaviour, Placeholder+" before → after, in at most 80 words. Behaviour, not code. -->")
	if blocks := MermaidBlocks(spec.SectionText(st.Spec.Body, "What")); len(blocks) > 0 {
		b.add(Diagram, Placeholder+` one sentence saying what this diagram answers. Mark what this step adds or changes with classDef added, or say it matches the spec as drawn. End with a "Legend:" line. -->`+"\n\n"+
			strings.Join(blocks, "\n\n")+"\n\n"+
			"Legend: "+Placeholder+" what a box, an arrow and each colour mean -->")
	}
	b.add(WhereToLook, b.whereToLook())
	if len(b.Files.Visual) > 0 {
		b.add(VisualProof, list(b.Files.Visual)+"\n\n"+
			"| Screen | Before | After |\n| --- | --- | --- |\n"+
			"| "+Placeholder+" screen --> | "+Placeholder+" image, or \"new\" --> | "+Placeholder+" image --> |")
	}
	b.add(Departures, b.departures())
	b.add(Proven, proven(st))
	b.add(NeedsYou, b.needsYou(repo, st))
	b.add(NotDone, Placeholder+` what this step leaves out, and its risks. "Nothing." is an answer. -->`)
	return b, nil
}

// ResolveBase returns base, or else <remote>/<base> from the config, once git can find it. It
// isn't fetched.
func ResolveBase(g gitx.Git, repo *spec.Repo, base string) (string, error) {
	if base == "" {
		base = repo.Config.Remote + "/" + repo.Config.Base
	}
	if _, err := g.Run("rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return "", fmt.Errorf("Can't find %s. Run `git fetch %s`, or pass `--base`.", base, repo.Config.Remote)
	}
	return base, nil
}

// Group sorts changed files into Specs, Tests, Visual and Code, each file in the first group that
// matches, in that order. Each group is sorted.
func Group(repo *spec.Repo, files []string) Files {
	out := Files{Code: []string{}, Visual: []string{}, Tests: []string{}, Specs: []string{}}
	specs := strings.Trim(repo.Config.Specs, "/") + "/"
	var visual []string
	for _, v := range repo.Config.Visual {
		visual = append(visual, paths.Resolve(v, repo.Config.Aliases))
	}
	for _, f := range files {
		switch {
		case strings.HasPrefix(f, specs):
			out.Specs = append(out.Specs, f)
		case matchAny(testGlobs, f):
			out.Tests = append(out.Tests, f)
		case matchAny(visual, f):
			out.Visual = append(out.Visual, f)
		default:
			out.Code = append(out.Code, f)
		}
	}
	for _, l := range [][]string{out.Code, out.Visual, out.Tests, out.Specs} {
		sort.Strings(l)
	}
	return out
}

func matchAny(globs []string, file string) bool {
	for _, g := range globs {
		if paths.Match(g, file) {
			return true
		}
	}
	return false
}

// MermaidBlocks returns each ```mermaid block in text, fences included, copied as is.
func MermaidBlocks(text string) []string {
	var out []string
	var cur []string
	fence := "" // the fence we're inside, if any
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence == "" {
			if strings.HasPrefix(trimmed, "```") {
				fence = "```"
				if strings.TrimSpace(strings.TrimPrefix(trimmed, "```")) == "mermaid" {
					cur = []string{line}
				}
			}
			continue
		}
		if cur != nil {
			cur = append(cur, line)
		}
		if trimmed == fence {
			if cur != nil {
				out = append(out, strings.Join(cur, "\n"))
			}
			fence, cur = "", nil
		}
	}
	return out
}

func (b *Brief) add(heading, markdown string) {
	b.Sections = append(b.Sections, Section{Heading: heading, Markdown: markdown})
}

// Markdown is the brief as it goes in the PR description.
func (b *Brief) Markdown() string {
	var parts []string
	for _, s := range b.Sections {
		parts = append(parts, "## "+s.Heading+"\n\n"+s.Markdown+"\n")
	}
	return strings.Join(parts, "\n")
}

func list(files []string) string {
	lines := make([]string, len(files))
	for i, f := range files {
		lines[i] = "- `" + f + "`"
	}
	return strings.Join(lines, "\n")
}

func (b *Brief) whereToLook() string {
	var parts []string
	if len(b.Files.Code) > 0 {
		parts = append(parts, "**Code**\n\n"+Placeholder+" name the files or functions that carry the decision, most important first -->\n\n"+list(b.Files.Code))
	}
	for _, g := range []struct {
		name  string
		files []string
	}{{"Visual", b.Files.Visual}, {"Tests", b.Files.Tests}, {"Specs", b.Files.Specs}} {
		if len(g.files) > 0 {
			parts = append(parts, "**"+g.name+"**\n\n"+list(g.files))
		}
	}
	if len(parts) == 0 {
		return "No files changed."
	}
	return strings.Join(parts, "\n\n")
}

// departures never writes a "## Territory" heading: that would make furrow territory --body-file
// accept the outside files unexplained.
func (b *Brief) departures() string {
	var parts []string
	if len(b.DecisionsAdded) > 0 {
		lines := []string{"Added to `## Decisions`:", ""}
		for _, d := range b.DecisionsAdded {
			lines = append(lines, "- "+d)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	for _, id := range b.CriteriaChanged {
		parts = append(parts, id+"'s text changed. Only a person changes a criterion (INV-4).")
	}
	if len(b.Outside) > 0 {
		parts = append(parts, "Outside the step's territory. Explain each under \"## Territory\".\n\n"+list(b.Outside))
	}
	if len(parts) == 0 {
		return "None."
	}
	return strings.Join(parts, "\n\n")
}

// covered returns the criteria the step covers, in criterion order.
func covered(st *spec.Step) []*spec.Criterion {
	var out []*spec.Criterion
	for _, id := range st.Covers {
		if c := st.Spec.Criterion(strings.TrimSpace(id)); c != nil {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out
}

func proven(st *spec.Step) string {
	var lines []string
	for _, c := range covered(st) {
		if c.Proof == "human" {
			continue
		}
		box := " "
		if c.Ticked {
			box = "x"
		}
		lines = append(lines, fmt.Sprintf("- [%s] %s: %s", box, c.ID, c.Text),
			"  Evidence: "+Placeholder+" test name, command and output, or screenshot -->")
	}
	if len(lines) == 0 {
		return "None. Every criterion this step covers needs a person."
	}
	return strings.Join(lines, "\n")
}

// needsYou lists the human criteria the step covers, then the spec's invariants, then required
// human review. It fills b.NeedsYou in the same order.
func (b *Brief) needsYou(repo *spec.Repo, st *spec.Step) string {
	var lines []string
	for _, c := range covered(st) {
		if c.Proof != "human" {
			continue
		}
		text := c.Text
		if i := strings.LastIndex(text, "(human:"); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", c.ID, text), "  Look at: "+c.Detail)
		b.NeedsYou = append(b.NeedsYou, fmt.Sprintf("%s: %s. Look at: %s", c.ID, strings.TrimSuffix(text, "."), c.Detail))
	}
	names := map[string]string{}
	for _, inv := range repo.Invariants {
		names[inv.ID] = inv.Text
	}
	invs := append([]string{}, st.Spec.Invariants...)
	sort.SliceStable(invs, func(i, j int) bool { return invNum(invs[i]) < invNum(invs[j]) })
	for _, id := range invs {
		id = strings.TrimSpace(id)
		item := id
		if name := names[id]; name != "" {
			item = id + ": " + name
		}
		lines = append(lines, "- "+item)
		b.NeedsYou = append(b.NeedsYou, item)
	}
	if st.Spec.HumanReview == "required" {
		lines = append(lines, "- `human_review: required`")
		b.NeedsYou = append(b.NeedsYou, "human_review: required")
	}
	if len(lines) == 0 {
		return "Nothing. Every criterion has a test or a check."
	}
	return strings.Join(lines, "\n")
}

func invNum(id string) int {
	var n int
	fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(id), "INV-"), "%d", &n)
	return n
}

// fileAt returns a file's content at a commit, and whether it was there. file is relative to
// g.Dir.
func fileAt(g gitx.Git, commit, file string) (string, bool) {
	rel := "./" + path.Clean(file)
	if _, err := g.Run("cat-file", "-e", commit+":"+rel); err != nil {
		return "", false
	}
	out, err := g.Run("show", commit+":"+rel)
	if err != nil {
		return "", false
	}
	return out, true
}

// decisions returns the items under "## Decisions": each list item with its wrapped lines joined
// by a space, and each other non-blank line on its own.
func decisions(body string) []string {
	var out []string
	for _, line := range strings.Split(spec.SectionText(body, "Decisions"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			out = append(out, strings.TrimSpace(line[2:]))
		case len(out) > 0 && line != trimmed:
			out[len(out)-1] += " " + trimmed
		default:
			out = append(out, trimmed)
		}
	}
	return out
}

// added returns the items in now that aren't in before, counting repeats, sorted.
func added(before, now []string) []string {
	seen := map[string]int{}
	for _, d := range before {
		seen[d]++
	}
	out := []string{}
	for _, d := range now {
		if seen[d] > 0 {
			seen[d]--
			continue
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// changedCriteria returns the ids of criteria whose text, ignoring the box, differs from old's:
// reworded, added or removed. They're sorted by number.
func changedCriteria(old string, now *spec.Spec) []string {
	out := []string{}
	before, err := spec.ParseSpec([]byte(old))
	if err != nil {
		return out
	}
	text := map[string]string{}
	num := map[string]int{}
	for _, c := range before.Criteria {
		text[c.ID], num[c.ID] = c.Text, c.Num
	}
	seen := map[string]bool{}
	for _, c := range now.Criteria {
		seen[c.ID] = true
		num[c.ID] = c.Num
		if old, ok := text[c.ID]; !ok || old != c.Text {
			out = append(out, c.ID)
		}
	}
	for _, c := range before.Criteria {
		if !seen[c.ID] {
			out = append(out, c.ID)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if num[out[i]] != num[out[j]] {
			return num[out[i]] < num[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}
