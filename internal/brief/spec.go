package brief

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
)

// Section headings of a spec's brief, in the order it has them.
const (
	Decision        = "Decision"
	YourCalls       = "Your calls"
	Answered        = "Answered"
	Plan            = "Plan"
	Collisions      = "Collisions"
	CostsLater      = "What it costs later"
	ChangedCriteria = "Changed criteria"
)

// MaxDecisionWords is the most words ## Decision may have in all, a backstop for prose beside its
// two lines.
const MaxDecisionWords = 80

// The two lines of ## Decision, and the most words each may have after its label.
const (
	CommitsUsTo          = "**Commits us to:**"
	InsteadOf            = "**Instead of:**"
	MaxDecisionLineWords = 25
)

// decisionPlaceholder is ## Decision as furrow brief --spec writes it.
var decisionPlaceholder = CommitsUsTo + " " + Placeholder + " what accepting this means we build, at most 25 words -->\n" +
	InsteadOf + " " + Placeholder + " the main alternative it rejects, and why, at most 25 words -->"

// PlanLegend follows the Plan's diagram, word for word.
const PlanLegend = "Legend: each box is a step, with its size (s or m). An arrow points from a step to one that waits for it. Steps in the same wave can run at the same time unless they share files."

// YourCallsIntro opens ## Your calls.
const YourCallsIntro = "Merging accepts each proposed answer. To overrule one, reply to this PR with its number, like \"Q2: no, keep it\", or comment on its line. One reply can answer several."

// ErrSpecAndStep means --spec and --step were both given. furrow brief exits 2 on it.
var ErrSpecAndStep = errors.New("Pass `--spec` or `--step`, not both.")

// SpecOptions say which spec's brief to write. An empty Base is <remote>/<base>.
type SpecOptions struct {
	Spec    string
	Step    string // must be empty; set only to report ErrSpecAndStep
	PR      int    // the PR's number, for line links; 0 for none
	Base    string
	Offline bool // don't ask the remote which steps are claimed
}

// PlanWave is one wave of the Plan.
type PlanWave struct {
	Wave  int        `json:"wave"`
	Steps []PlanStep `json:"steps"`
}

// PlanStep is one unmerged step of the spec in the Plan.
type PlanStep struct {
	Ref   string   `json:"ref"`
	Title string   `json:"title"`
	Size  string   `json:"size"`
	After []string `json:"after"`
}

// Collision is a step of this spec and a step of another accepted spec whose touches overlap.
type Collision struct {
	Step    string `json:"step"`
	Other   string `json:"other"`
	Paths   string `json:"paths"`
	Claimed bool   `json:"claimed"`
}

// CriterionChange is a criterion added, removed or reworded since the merge base.
type CriterionChange struct {
	ID     string `json:"id"`
	Change string `json:"change"` // added, removed or reworded
	Old    string `json:"old"`
	New    string `json:"new"`
}

// Question is one top-level item of ## Open questions.
type Question struct {
	N        int
	Text     string
	Proposed string // "" when it has none
	Line     int    // the item's first line in the working tree's spec file
}

// SpecBrief is the brief for a PR that adds or amends a spec.
type SpecBrief struct {
	Spec            string            `json:"spec"`
	Base            string            `json:"base"`
	Sections        []Section         `json:"sections"`
	Plan            []PlanWave        `json:"plan"`
	Collisions      []Collision       `json:"collisions"`
	ChangedCriteria []CriterionChange `json:"changed_criteria"`
}

// BuildSpec writes the brief for a PR that adds or amends spec opt.Spec. It reads the working tree
// and git, and the remote's branches for claims unless opt.Offline. No forge is asked anything.
func BuildSpec(g gitx.Git, repo *spec.Repo, opt SpecOptions) (*SpecBrief, error) {
	if opt.Step != "" {
		return nil, ErrSpecAndStep
	}
	s, err := findSpec(repo, opt.Spec)
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
	raw, err := os.ReadFile(filepath.Join(repo.Root, filepath.FromSlash(s.File)))
	if err != nil {
		return nil, err
	}
	old, existed := fileAt(g, mb, s.File)

	b := &SpecBrief{Spec: s.ID, Base: base, Plan: []PlanWave{}, Collisions: []Collision{}, ChangedCriteria: []CriterionChange{}}
	b.add(Decision, decisionPlaceholder)
	b.add(YourCalls, yourCalls(OpenQuestions(string(raw)), s.File, githubRepo(g, repo, opt.PR), opt.PR))

	var oldDecisions []string
	if existed {
		oldDecisions = decisions(old)
	}
	var chose []string
	for _, d := range added(oldDecisions, decisions(s.Body)) {
		if strings.Contains(d, "Owner chose") {
			chose = append(chose, "- "+d)
		}
	}
	if len(chose) > 0 {
		b.add(Answered, strings.Join(chose, "\n"))
	}

	b.Plan = planWaves(repo, s)
	b.add(Plan, planMarkdown(repo, s, b.Plan))

	var claims []schedule.Claim
	checked := false
	if !opt.Offline {
		if c, err := claim.List(g, repo.Config); err == nil {
			claims, checked = c, true
		}
	}
	b.Collisions = collisions(repo, s, claims)
	b.add(Collisions, collisionsMarkdown(b.Collisions, checked))

	b.add(CostsLater, costsLater(repo, s))

	if existed {
		b.ChangedCriteria = criteriaChanges(old, s)
		b.add(ChangedCriteria, changesMarkdown(b.ChangedCriteria))
	}
	return b, nil
}

func (b *SpecBrief) add(heading, markdown string) {
	b.Sections = append(b.Sections, Section{Heading: heading, Markdown: markdown})
}

// Markdown is the spec's brief as it goes in the PR description.
func (b *SpecBrief) Markdown() string {
	return (&Brief{Sections: b.Sections}).Markdown()
}

// findSpec returns the spec id names, if it's in changes/.
func findSpec(repo *spec.Repo, id string) (*spec.Spec, error) {
	ref, err := repo.ParseRef(id, nil)
	if err != nil {
		return nil, err
	}
	if ref.Step != 0 {
		return nil, fmt.Errorf("%s names a step. Pass its spec, such as `--spec %s`.", ref, ref.Spec)
	}
	s := repo.Spec(ref.Spec)
	if s == nil {
		return nil, fmt.Errorf("no spec %s", ref.Spec)
	}
	if s.Archived {
		return nil, fmt.Errorf("%s is archived. Change it with a new spec.", s.ID)
	}
	return s, nil
}

// OpenQuestions reads the top-level items of "## Open questions" in a spec file's raw text,
// numbered from 1 in file order. An item's wrapped and nested lines are joined by a space.
func OpenQuestions(raw string) []Question {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	var out []Question
	var texts []string
	in, fence, front := false, "", false
	lazy := false // the previous line belongs to the current item, so a plain line continues it
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && trimmed == "---" {
			front = true
			continue
		}
		if front {
			if trimmed == "---" {
				front = false
			}
			continue
		}
		if fence == "" && strings.HasPrefix(line, "## ") {
			if in {
				break
			}
			in = strings.TrimSpace(line[3:]) == "Open questions"
			continue
		}
		if !in {
			continue
		}
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
		} else if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:3]
		}
		switch {
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			out = append(out, Question{N: len(out) + 1, Line: i + 1})
			texts = append(texts, strings.TrimSpace(line[2:]))
			lazy = true
		case trimmed == "":
			lazy = false
		case len(out) > 0 && (line != trimmed || lazy):
			texts[len(texts)-1] += " " + trimmed
			lazy = true
		}
	}
	for i := range out {
		q, p, ok := strings.Cut(texts[i], "Proposed:")
		out[i].Text = strings.TrimSpace(q)
		if ok {
			out[i].Proposed = strings.TrimSpace(p)
		}
	}
	return out
}

var githubURL = []*regexp.Regexp{
	regexp.MustCompile(`^https://(?:[^@/]+@)?github\.com/([^/]+)/([^/]+?)(?:\.git)?/?$`),
	regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?/?$`),
	regexp.MustCompile(`^ssh://git@github\.com(?::\d+)?/([^/]+)/([^/]+?)(?:\.git)?/?$`),
}

// GitHubRepo reads "owner/repo" from a GitHub remote URL in https or ssh form, or "" for any other.
func GitHubRepo(url string) string {
	for _, re := range githubURL {
		if m := re.FindStringSubmatch(strings.TrimSpace(url)); m != nil {
			return m[1] + "/" + m[2]
		}
	}
	return ""
}

// githubRepo is the remote's owner/repo when there's a PR to link into, else "".
func githubRepo(g gitx.Git, repo *spec.Repo, pr int) string {
	if pr <= 0 {
		return ""
	}
	url, err := g.Run("remote", "get-url", repo.Config.Remote)
	if err != nil {
		return ""
	}
	return GitHubRepo(url)
}

// LineLink opens a line of a file in a GitHub PR's diff.
func LineLink(ownerRepo string, pr int, file string, line int) string {
	sum := sha256.Sum256([]byte(file))
	return fmt.Sprintf("https://github.com/%s/pull/%d/files#diff-%sR%d", ownerRepo, pr, hex.EncodeToString(sum[:]), line)
}

var linkText = strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`)

func yourCalls(qs []Question, file, ownerRepo string, pr int) string {
	if len(qs) == 0 {
		return "None. Ready to accept."
	}
	item := func(q Question) string {
		text := fmt.Sprintf("**Q%d** %s (%s:%d)", q.N, q.Text, file, q.Line)
		if ownerRepo != "" {
			text = fmt.Sprintf("**Q%d** [%s](%s)", q.N, linkText.Replace(q.Text), LineLink(ownerRepo, pr, file, q.Line))
		}
		if q.Proposed != "" {
			text += "\nProposed: " + q.Proposed
		}
		return text
	}
	parts := []string{YourCallsIntro}
	var open, proposed []string
	for _, q := range qs {
		if q.Proposed == "" {
			open = append(open, item(q))
		} else {
			proposed = append(proposed, item(q))
		}
	}
	if len(open) > 0 {
		parts = append(parts, "Needs an answer before merging:")
		parts = append(parts, open...)
	}
	parts = append(parts, proposed...)
	return strings.Join(parts, "\n\n")
}

// planWaves puts the spec's unmerged steps in waves, worked out as if the spec were accepted.
func planWaves(repo *spec.Repo, s *spec.Spec) []PlanWave {
	status := s.Status
	s.Status = spec.Accepted
	waves := schedule.Waves(repo, nil)
	s.Status = status

	out := []PlanWave{}
	for _, w := range waves {
		var steps []PlanStep
		for _, it := range w.Items {
			if it.Step.Spec != s {
				continue
			}
			ps := PlanStep{Ref: it.Ref(), Title: it.Step.Title, Size: it.Step.Size, After: []string{}}
			for _, dep := range waitsOn(repo, it.Step) {
				ps.After = append(ps.After, dep.String())
			}
			steps = append(steps, ps)
		}
		if len(steps) > 0 {
			sort.SliceStable(steps, func(i, j int) bool { return refLess(steps[i].Ref, steps[j].Ref) })
			out = append(out, PlanWave{Wave: w.Number, Steps: steps})
		}
	}
	return out
}

// waitsOn returns the unmerged refs a step is after, as written: a step of this spec, another
// spec's step, or another whole spec. They're sorted by spec and step number.
func waitsOn(repo *spec.Repo, st *spec.Step) []spec.Ref {
	refs, _ := repo.AfterRefs(st)
	seen := map[spec.Ref]bool{}
	var out []spec.Ref
	for _, ref := range refs {
		if seen[ref] || repo.RefMerged(ref) || (ref.Spec == st.Spec.ID && ref.Step == st.N) {
			continue
		}
		if ref.Spec == st.Spec.ID && repo.Step(ref) == nil {
			continue // lint reports an unknown step
		}
		seen[ref] = true
		out = append(out, ref)
	}
	sort.SliceStable(out, func(i, j int) bool { return refLess(out[i].String(), out[j].String()) })
	return out
}

// refLess orders refs by spec, then step number, so FUR-0001.2 comes before FUR-0001.10.
func refLess(a, b string) bool {
	as, an := splitRef(a)
	bs, bn := splitRef(b)
	if as != bs {
		return as < bs
	}
	return an < bn
}

func splitRef(ref string) (string, int) {
	s, n, ok := strings.Cut(ref, ".")
	if !ok {
		return s, 0
	}
	v, _ := strconv.Atoi(n)
	return s, v
}

var nodeID = regexp.MustCompile(`[^A-Za-z0-9]+`)

func label(text string) string {
	return strings.NewReplacer(`"`, "#quot;").Replace(text)
}

func planMarkdown(repo *spec.Repo, s *spec.Spec, waves []PlanWave) string {
	if len(waves) == 0 {
		return "Every step is merged."
	}
	id := func(ref string) string {
		sp, n := splitRef(ref)
		if sp == s.ID && n > 0 {
			return "S" + strconv.Itoa(n)
		}
		return nodeID.ReplaceAllString(ref, "_")
	}
	var external []string
	seenExt := map[string]bool{}
	type edge struct{ from, to string }
	var edges []edge
	lines := []string{"```mermaid", "flowchart TD"}
	var body []string
	for _, w := range waves {
		body = append(body, fmt.Sprintf("  subgraph W%d[\"Wave %d\"]", w.Wave, w.Wave))
		for _, st := range w.Steps {
			_, n := splitRef(st.Ref)
			size := st.Size
			if size == "" {
				size = "-"
			}
			body = append(body, fmt.Sprintf("    %s[\"%d. %s (%s)\"]", id(st.Ref), n, label(st.Title), size))
			for _, a := range st.After {
				if sp, _ := splitRef(a); sp != s.ID && !seenExt[a] {
					seenExt[a] = true
					external = append(external, a)
				}
				edges = append(edges, edge{a, st.Ref})
			}
		}
		body = append(body, "  end")
	}
	sort.SliceStable(external, func(i, j int) bool { return refLess(external[i], external[j]) })
	for _, e := range external {
		lines = append(lines, fmt.Sprintf("  %s[\"%s\"]", id(e), e))
	}
	lines = append(lines, body...)
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].to != edges[j].to {
			return refLess(edges[i].to, edges[j].to)
		}
		return refLess(edges[i].from, edges[j].from)
	})
	for _, e := range edges {
		lines = append(lines, fmt.Sprintf("  %s --> %s", id(e.from), id(e.to)))
	}
	lines = append(lines, "```", "", PlanLegend)
	return strings.Join(lines, "\n")
}

// collisions pairs each unmerged step of s with each unmerged step of another accepted spec whose
// touches overlap, in step order.
func collisions(repo *spec.Repo, s *spec.Spec, claims []schedule.Claim) []Collision {
	claimed := map[spec.Ref]bool{}
	for _, c := range claims {
		claimed[c.Ref] = true
	}
	out := []Collision{}
	for _, st := range s.Steps {
		if spec.StepMerged(st) {
			continue
		}
		for _, other := range repo.OpenSteps() {
			if other.Spec == s || other.Spec.ID == s.ID {
				continue
			}
			if pair, ok := paths.Overlap(st.Touches, other.Touches, repo.Config.Aliases); ok {
				out = append(out, Collision{Step: st.Ref().String(), Other: other.Ref().String(), Paths: pair, Claimed: claimed[other.Ref()]})
			}
		}
	}
	return out
}

func collisionsMarkdown(cs []Collision, checked bool) string {
	if len(cs) == 0 {
		return "None."
	}
	var lines []string
	if !checked {
		lines = append(lines, "(claims not checked)", "")
	}
	for _, c := range cs {
		other := c.Other
		if c.Claimed {
			other += " (claimed)"
		}
		lines = append(lines, fmt.Sprintf("- %s and %s both touch %s.", c.Step, other, c.Paths))
	}
	return strings.Join(lines, "\n")
}

// costsLater lists the invariants, required human review, the human criteria still to sign off,
// and the unmerged steps by size.
func costsLater(repo *spec.Repo, s *spec.Spec) string {
	var lines []string
	names := map[string]string{}
	for _, inv := range repo.Invariants {
		names[inv.ID] = inv.Text
	}
	invs := append([]string{}, s.Invariants...)
	sort.SliceStable(invs, func(i, j int) bool { return invNum(invs[i]) < invNum(invs[j]) })
	for _, id := range invs {
		id = strings.TrimSpace(id)
		if name := names[id]; name != "" {
			id += ": " + name
		}
		lines = append(lines, "- "+id)
	}
	if s.HumanReview == "required" {
		lines = append(lines, "- `human_review: required`")
	}
	for _, c := range s.Criteria {
		if c.Proof != "human" || c.Ticked {
			continue
		}
		text := c.Text
		if i := strings.LastIndex(text, "(human:"); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", c.ID, text), "  Sign off by: "+c.Detail)
	}
	sizes := map[string]int{}
	n := 0
	for _, st := range s.Steps {
		if spec.StepMerged(st) {
			continue
		}
		size := st.Size
		if size == "" {
			size = "unsized"
		}
		sizes[size]++
		n++
	}
	var keys []string
	for k := range sizes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var counts []string
	for _, k := range keys {
		counts = append(counts, fmt.Sprintf("%d %s", sizes[k], k))
	}
	noun := "steps"
	if n == 1 {
		noun = "step"
	}
	count := fmt.Sprintf("%d %s to build.", n, noun)
	if n > 0 {
		count = fmt.Sprintf("%d %s: %s.", n, noun, strings.Join(counts, ", "))
	}
	if len(lines) == 0 {
		return count
	}
	return strings.Join(lines, "\n") + "\n\n" + count
}

// criteriaChanges compares the criteria in old, the spec file at the merge base, with now's:
// reworded and added in now's order, then removed in old's.
func criteriaChanges(old string, now *spec.Spec) []CriterionChange {
	out := []CriterionChange{}
	before, err := spec.ParseSpec([]byte(old))
	if err != nil {
		return out
	}
	text := map[string]string{}
	for _, c := range before.Criteria {
		text[c.ID] = c.Text
	}
	seen := map[string]bool{}
	for _, c := range now.Criteria {
		seen[c.ID] = true
		was, ok := text[c.ID]
		switch {
		case !ok:
			out = append(out, CriterionChange{ID: c.ID, Change: "added", New: c.Text})
		case was != c.Text:
			out = append(out, CriterionChange{ID: c.ID, Change: "reworded", Old: was, New: c.Text})
		}
	}
	for _, c := range before.Criteria {
		if !seen[c.ID] {
			out = append(out, CriterionChange{ID: c.ID, Change: "removed", Old: c.Text})
		}
	}
	return out
}

func changesMarkdown(cs []CriterionChange) string {
	lines := []string{"Only a person changes a criterion (INV-4)."}
	if len(cs) == 0 {
		return lines[0] + "\n\nNone."
	}
	lines = append(lines, "")
	for _, c := range cs {
		switch c.Change {
		case "added":
			lines = append(lines, fmt.Sprintf("- %s added: %s", c.ID, c.New))
		case "removed":
			lines = append(lines, fmt.Sprintf("- %s removed. It was: %s", c.ID, c.Old))
		default:
			lines = append(lines, fmt.Sprintf("- %s reworded.", c.ID), "  Was: "+c.Old, "  Now: "+c.New)
		}
	}
	return strings.Join(lines, "\n")
}

// CheckSpecFile builds the spec's brief, as furrow brief --spec would, and checks the brief in
// file against it. It only reads.
func CheckSpecFile(g gitx.Git, repo *spec.Repo, opt SpecOptions, file string) (*Report, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	b, err := BuildSpec(g, repo, opt)
	if err != nil {
		return nil, err
	}
	return CheckSpec(b, string(data)), nil
}

// CheckSpec reads text, a filled-in spec brief, against b: every section present, no placeholder
// left, and ## Decision with a Commits us to and an Instead of line of at most 25 words each, and
// at most 80 words in all. Problems are in b's section order, then any placeholder
// outside b's sections, in text order.
func CheckSpec(b *SpecBrief, text string) *Report {
	parts := split(text)
	byHeading := map[string]string{}
	for _, p := range parts[1:] {
		if _, ok := byHeading[p.heading]; !ok {
			byHeading[p.heading] = p.text
		}
	}
	rep := &Report{Problems: []string{}}
	expected := map[string]bool{}
	for _, s := range b.Sections {
		expected[s.Heading] = true
		body, ok := byHeading[s.Heading]
		if !ok {
			rep.add("No `## %s` section. Run `furrow brief --spec %s` and paste it.", s.Heading, b.Spec)
			continue
		}
		if strings.Contains(body, Placeholder) {
			rep.add("`## %s` still has a placeholder. Write it.", s.Heading)
		}
		if s.Heading == Decision {
			for _, p := range decisionLines(body) {
				rep.add("%s", p)
			}
			if n := len(strings.Fields(comment.ReplaceAllString(body, " "))); n > MaxDecisionWords {
				rep.add("`## %s` is %d words. Keep it to %d.", Decision, n, MaxDecisionWords)
			}
		}
	}
	if strings.Contains(parts[0].text, Placeholder) {
		rep.add("The text before `## %s` still has a placeholder. Write it.", Decision)
	}
	for _, p := range parts[1:] {
		if !expected[p.heading] && strings.Contains(p.text, Placeholder) {
			rep.add("`## %s` still has a placeholder. Write it.", p.heading)
		}
	}
	rep.OK = len(rep.Problems) == 0
	return rep
}

// decisionLines reports a missing Commits us to or Instead of line in ## Decision's body, or one
// whose text after its label is over MaxDecisionLineWords words, comments not counted.
func decisionLines(body string) []string {
	labels := []string{CommitsUsTo, InsteadOf}
	found := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		for _, l := range labels {
			if _, ok := found[l]; !ok && strings.HasPrefix(line, l) {
				found[l] = strings.TrimPrefix(line, l)
			}
		}
	}
	if len(found) < len(labels) {
		return []string{fmt.Sprintf("`## %s` needs a `%s` and an `%s` line.", Decision, CommitsUsTo, InsteadOf)}
	}
	var out []string
	for _, l := range labels {
		if n := len(strings.Fields(comment.ReplaceAllString(found[l], " "))); n > MaxDecisionLineWords {
			out = append(out, fmt.Sprintf("`%s` is %d words. Keep it to %d.", l, n, MaxDecisionLineWords))
		}
	}
	return out
}
