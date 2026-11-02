package brief_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/brief"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
	"github.com/devenjarvis/furrow/internal/territory"
)

// config adds an alias ui and the visual globs to the fixture config.
const config = spectest.Config + "  ui: web/ui\nvisual: [web/css/**, ui]\n"

// criteria for spec 7: two the step proves, one a person signs off, and one for step 2.
var criteria = []string{
	"- [ ] AC-1: It works (test: TestWorks)",
	"- [ ] AC-2: It is checked (check: true)",
	"- [ ] AC-3: It reads well (human: read the output)",
	"- [ ] AC-4: Step two works (test: TestTwo)",
}

const specFile = "specs/changes/0007-thing.md"

// spec7 is spec 7 with a body of our own, so a test can put a diagram in ## What.
func spec7(what, decisions string, crit []string) spectest.Spec {
	body := "# FUR-0007: Thing\n\n## Why\n\nBecause.\n\n## What\n\n" + what + "\n\n## Acceptance criteria\n\n" +
		strings.Join(crit, "\n") + "\n\n## Out of scope\n\n- That.\n\n## Decisions\n\n" + decisions
	return spectest.Spec{
		Num: 7, Slug: "thing", Title: "Thing", Invariants: []string{"INV-2"},
		Steps: []spectest.Step{
			{Title: "Build it", Touches: []string{"internal/thing/**", "cli/thing.go", "web/**", "tests/**", "src/**", "foo.spec.ts"}, Covers: []string{"AC-1", "AC-2", "AC-3"}},
			{Title: "Step two", Touches: []string{"internal/two/**"}, Covers: []string{"AC-4"}},
		},
		Criteria: crit,
		Body:     body,
	}
}

// setup commits a repo with spec 7 on main with a bare remote, and switches to step/0007-1.
func setup(t *testing.T, s spectest.Spec) *spectest.Repo {
	t.Helper()
	r := spectest.New(t)
	r.Write(".furrow.yml", config)
	r.Write("internal/.keep", "")
	r.Write("README.md", "hi\n")
	r.Spec(s)
	gittest.Init(t, r.Root)
	gittest.Must(t, r.Root, "switch", "-q", "-c", "step/0007-1")
	return r
}

func build(t *testing.T, r *spectest.Repo, opt brief.Options) *brief.Brief {
	t.Helper()
	b, err := brief.Build(gittest.Git(r.Root), r.Load(), opt)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func headings(b *brief.Brief) []string {
	out := []string{}
	for _, s := range b.Sections {
		out = append(out, s.Heading)
	}
	return out
}

func section(t *testing.T, b *brief.Brief, heading string) string {
	t.Helper()
	for _, s := range b.Sections {
		if s.Heading == heading {
			return s.Markdown
		}
	}
	t.Fatalf("no ## %s in %v", heading, headings(b))
	return ""
}

const mermaid = "```mermaid\nflowchart LR\n  A --> B\n```"

const (
	behaviourRaw = "**Before:** <!-- furrow: what a user or caller saw, at most 20 words -->\n" +
		"**After:** <!-- furrow: what they see now, at most 20 words -->\n\n" +
		"<!-- furrow: show it: real command output, a screenshot, or a short usage example. \"No visible effect.\" is an answer. -->"
	reviewRaw = "<!-- furrow: at most 3 numbered items, each \"`file` `name`: the question a reviewer should ask about it\" -->"
	invRaw    = " <!-- furrow: how this diff touches it, or \"Not touched.\" -->"
)

func TestBriefBehaviourBeforeAfter(t *testing.T) {
	r := setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	b := build(t, r, brief.Options{})
	if got := section(t, b, "Behaviour"); got != behaviourRaw {
		t.Errorf("Behaviour =\n%s\nwant\n%s", got, behaviourRaw)
	}
	if !strings.HasPrefix(b.Markdown(), "## Behaviour\n\n**Before:** <!-- furrow:") {
		t.Errorf("the brief should open with Behaviour's Before line:\n%s", b.Markdown())
	}
}

func TestBriefReviewThis(t *testing.T) {
	r := setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "x\n")
	r.Write("internal/thing/a_test.go", "x\n")
	r.Write("web/css/site.css", "x\n")
	r.Write("specs/areas/core.md", "x\n")
	got := section(t, build(t, r, brief.Options{}), "Review this")
	want := reviewRaw + "\n\n" +
		"<details><summary>All 4 changed files</summary>\n\n" +
		"**Code**\n\n- `internal/thing/a.go`\n\n" +
		"**Visual**\n\n- `web/css/site.css`\n\n" +
		"**Tests**\n\n- `internal/thing/a_test.go`\n\n" +
		"**Specs**\n\n- `specs/areas/core.md`\n\n" +
		"</details>"
	if got != want {
		t.Errorf("Review this =\n%s\nwant\n%s", got, want)
	}

	// One file: the summary says so in the singular.
	r = setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "x\n")
	got = section(t, build(t, r, brief.Options{}), "Review this")
	want = reviewRaw + "\n\n<details><summary>1 changed file</summary>\n\n**Code**\n\n- `internal/thing/a.go`\n\n</details>"
	if got != want {
		t.Errorf("Review this, one file =\n%s\nwant\n%s", got, want)
	}

	// No changes: no list to fold.
	r = setup(t, spec7("This.", "", criteria))
	if got := section(t, build(t, r, brief.Options{}), "Review this"); got != reviewRaw+"\n\nNo files changed." {
		t.Errorf("Review this, no changes = %q", got)
	}
}

func TestBriefLayoutAndProof(t *testing.T) {
	// Without a diagram or a visual change, Diagram and Visual proof are left out.
	r := setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	b := build(t, r, brief.Options{})
	want := []string{"Behaviour", "Review this", "Judgment calls", "Needs you", "Not done"}
	if got := headings(b); !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %v\nwant       %v", got, want)
	}

	// With both, all seven, in order.
	crit := append([]string{}, criteria...)
	crit[1] = "- [x] AC-2: It is checked (check: true)"
	r = setup(t, spec7("This.\n\n"+mermaid, "", crit))
	r.Write("internal/thing/a.go", "package thing\n")
	r.Write("web/css/site.css", "body {}\n")
	b = build(t, r, brief.Options{})
	want = []string{"Behaviour", "Diagram", "Review this", "Visual proof", "Judgment calls", "Needs you", "Not done"}
	if got := headings(b); !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %v\nwant       %v", got, want)
	}
	for _, h := range []string{"Behaviour", "Diagram", "Review this", "Visual proof", "Needs you", "Not done"} {
		if !strings.Contains(section(t, b, h), "<!-- furrow:") {
			t.Errorf("## %s has no <!-- furrow: placeholder:\n%s", h, section(t, b, h))
		}
	}
	if strings.Contains(section(t, b, "Judgment calls"), "<!--") {
		t.Errorf("## Judgment calls is facts, but has a placeholder:\n%s", section(t, b, "Judgment calls"))
	}

	// The Markdown is the sections, as "## " headings, in the same order, then Proof, folded: one
	// line per covered criterion that isn't human, and no placeholder.
	md := b.Markdown()
	last := -1
	for _, h := range want {
		i := strings.Index(md, "\n## "+h+"\n")
		if h == "Behaviour" {
			i = strings.Index(md, "## Behaviour\n")
		}
		if i <= last {
			t.Fatalf("## %s out of place in:\n%s", h, md)
		}
		last = i
	}
	proof := "<details><summary>Proof: AC-1 ✗ AC-2 ✓</summary>\n\n" +
		"- AC-1 (test: TestWorks) not ticked\n" +
		"- AC-2 (check: true)\n\n" +
		"</details>\n"
	notDone := "## Not done\n\n<!-- furrow: what this step leaves out, and its risks. \"Nothing.\" is an answer. -->\n\n"
	if !strings.HasSuffix(md, notDone+proof) {
		t.Errorf("markdown should end with Not done, then Proof:\n%s", md)
	}
	if strings.Contains(md, "## Proof") || strings.Contains(md, "## Proven") || strings.Contains(md, "AC-3 (") {
		t.Errorf("Proof is folded, not a section, and leaves out human criteria:\n%s", md)
	}

	// --json: judgment_calls and proof, in criterion order.
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		JudgmentCalls []string `json:"judgment_calls"`
		Proof         []struct {
			ID     string `json:"id"`
			Proof  string `json:"proof"`
			Ticked bool   `json:"ticked"`
		} `json:"proof"`
		DecisionsAdded any `json:"decisions_added"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.JudgmentCalls == nil || len(got.JudgmentCalls) != 0 || got.DecisionsAdded != nil {
		t.Errorf("judgment_calls should be [] and decisions_added gone: %s", data)
	}
	if len(got.Proof) != 2 || got.Proof[0].ID != "AC-1" || got.Proof[0].Proof != "test: TestWorks" || got.Proof[0].Ticked ||
		got.Proof[1].ID != "AC-2" || got.Proof[1].Proof != "check: true" || !got.Proof[1].Ticked {
		t.Errorf("proof = %+v", got.Proof)
	}

	// A step whose criteria all need a person still closes with Proof, saying so.
	s := spec7("This.", "", criteria)
	s.Steps[1].Covers = []string{"AC-3"}
	s.Steps[0].Covers = []string{"AC-1", "AC-2", "AC-4"}
	r = setup(t, s)
	b = build(t, r, brief.Options{Step: "FUR-0007.2"})
	if md := b.Markdown(); !strings.HasSuffix(md, "<details><summary>Proof: none</summary>\n\nEvery criterion this step covers needs a person.\n\n</details>\n") {
		t.Errorf("all-human step's Proof:\n%s", md)
	}
	if len(b.Proof) != 0 || b.Proof == nil {
		t.Errorf("proof = %#v, want []", b.Proof)
	}
}

func TestBriefGroupsFiles(t *testing.T) {
	r := setup(t, spec7("This.", "", criteria))
	for _, f := range []string{
		"internal/thing/b.go",            // code
		"internal/thing/a.go",            // code
		"internal/thing/a_test.go",       // tests
		"internal/thing/testdata/in.txt", // tests
		"tests/e2e.sh",                   // tests
		"src/__tests__/a.js",             // tests
		"foo.spec.ts",                    // tests
		"web/css/site.css",               // visual
		"web/ui/button.tsx",              // visual, through the alias
		"web/css/site.test.css",          // tests and visual: tests
		"web/js/app.js",                  // code: not a visual glob
		"specs/areas/core.md",            // specs
		"specs/testdata/x.md",            // specs and tests: specs
	} {
		r.Write(f, "x\n")
	}
	b := build(t, r, brief.Options{})
	want := brief.Files{
		Code:   []string{"internal/thing/a.go", "internal/thing/b.go", "web/js/app.js"},
		Visual: []string{"web/css/site.css", "web/ui/button.tsx"},
		Tests:  []string{"foo.spec.ts", "internal/thing/a_test.go", "internal/thing/testdata/in.txt", "src/__tests__/a.js", "tests/e2e.sh", "web/css/site.test.css"},
		Specs:  []string{"specs/areas/core.md", "specs/testdata/x.md"},
	}
	if !reflect.DeepEqual(b.Files, want) {
		t.Errorf("files = %+v\nwant    %+v", b.Files, want)
	}

	where := section(t, b, "Review this")
	order := []string{"**Code**", "`internal/thing/a.go`", "`internal/thing/b.go`", "`web/js/app.js`",
		"**Visual**", "`web/css/site.css`", "**Tests**", "`foo.spec.ts`", "**Specs**", "`specs/areas/core.md`"}
	last := -1
	for _, s := range order {
		i := strings.Index(where, s)
		if i <= last {
			t.Fatalf("%s out of place in:\n%s", s, where)
		}
		last = i
	}

	// Empty groups are left out.
	r = setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	where = section(t, build(t, r, brief.Options{}), "Review this")
	for _, g := range []string{"**Visual**", "**Tests**", "**Specs**"} {
		if strings.Contains(where, g) {
			t.Errorf("empty group %s shown:\n%s", g, where)
		}
	}
}

func TestBriefCopiesSpecDiagram(t *testing.T) {
	second := "```mermaid\nsequenceDiagram\n    A->>B: hi\n\n    B-->>A: ok\n```"
	what := "Words.\n\n" + mermaid + "\n\n```yaml\nnot: a diagram\n```\n\nMore.\n\n" + second
	s := spec7(what, "", criteria)
	s.Body = strings.Replace(s.Body, "Because.", "Because.\n\n```mermaid\nflowchart TD\n  Why --> Not\n```", 1)
	r := setup(t, s)
	b := build(t, r, brief.Options{})
	d := section(t, b, "Diagram")
	if !strings.HasPrefix(d, "<!-- furrow: one sentence saying what this diagram answers.") {
		t.Errorf("Diagram should open with its placeholder:\n%s", d)
	}
	if !strings.Contains(d, mermaid+"\n\n"+second) {
		t.Errorf("Diagram should hold both blocks from ## What, as is, in order:\n%s", d)
	}
	if strings.Contains(d, "not: a diagram") || strings.Contains(d, "Why --> Not") {
		t.Errorf("Diagram copied something that isn't a mermaid block in ## What:\n%s", d)
	}
}

func TestBriefDiagramAsksForLegend(t *testing.T) {
	r := setup(t, spec7("This.\n\n"+mermaid, "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	b := build(t, r, brief.Options{})
	d := section(t, b, "Diagram")
	caption := "<!-- furrow: one sentence saying what this diagram answers. Mark what this step adds or changes with classDef added, or say it matches the spec as drawn. End with a \"Legend:\" line. -->"
	legend := "Legend: <!-- furrow: what a box, an arrow and each colour mean -->"
	if want := caption + "\n\n" + mermaid + "\n\n" + legend; d != want {
		t.Errorf("Diagram =\n%s\nwant\n%s", d, want)
	}

	// The raw brief has a Legend: line, so --check asks only for the placeholders to be written.
	var problems []string
	for _, p := range brief.Check(b, b.Markdown()).Problems {
		if strings.Contains(p, "Diagram") {
			problems = append(problems, p)
		}
	}
	if want := []string{"`## Diagram` still has a placeholder. Write it."}; !reflect.DeepEqual(problems, want) {
		t.Errorf("Diagram problems = %q, want %q", problems, want)
	}
}

func TestBriefVisualProof(t *testing.T) {
	r := setup(t, spec7("This.", "", criteria))
	r.Write("web/ui/button.tsx", "x\n")
	r.Write("web/css/site.css", "x\n")
	r.Write("internal/thing/a.go", "x\n")
	v := section(t, build(t, r, brief.Options{}), "Visual proof")
	want := "- `web/css/site.css`\n- `web/ui/button.tsx`\n\n" +
		"| Screen | Before | After |\n| --- | --- | --- |\n" +
		"| <!-- furrow: screen --> | <!-- furrow: image, or \"new\" --> | <!-- furrow: image --> |"
	if v != want {
		t.Errorf("Visual proof =\n%s\nwant\n%s", v, want)
	}

	// No visual file changed: no section.
	r = setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "x\n")
	if h := headings(build(t, r, brief.Options{})); strings.Contains(strings.Join(h, ","), "Visual proof") {
		t.Errorf("no visual change, but headings = %v", h)
	}

	// visual empty: no section, and nothing is grouped as Visual.
	r = setup(t, spec7("This.", "", criteria))
	r.Write(".furrow.yml", spectest.Config)
	r.Write("web/css/site.css", "x\n")
	b := build(t, r, brief.Options{})
	if h := headings(b); strings.Contains(strings.Join(h, ","), "Visual proof") {
		t.Errorf("visual is empty, but headings = %v", h)
	}
	if len(b.Files.Visual) != 0 || !strings.Contains(strings.Join(b.Files.Code, ","), "web/css/site.css") {
		t.Errorf("with visual empty, a css file is code: %+v", b.Files)
	}
}

func TestBriefJudgmentCalls(t *testing.T) {
	// Nothing to judge: None.
	r := setup(t, spec7("This.", "- 2026-01-01: Chose A because B.\n", criteria))
	r.Write("internal/thing/a.go", "x\n")
	b := build(t, r, brief.Options{})
	if d := section(t, b, "Judgment calls"); d != "None." {
		t.Errorf("Judgment calls = %q, want None.", d)
	}

	// Three new decisions (one wrapped over two lines, one without a date), a ticked box (not a
	// judgment call), a reworded criterion, and two files outside the territory.
	crit := append([]string{}, criteria...)
	crit[0] = "- [x] AC-1: It works (test: TestWorks)"
	crit[1] = "- [ ] AC-2: It is checked well (check: true)"
	r.Spec(spec7("This.", "- 2026-01-01: Chose A because B.\n- 2026-02-02: Chose C\n  because D.\n- 2026-01-15: Chose E because F.\n- Undated: chose G.\n", crit))
	r.Write("go.mod", "module x\n")
	r.Write("docs/notes.md", "x\n")
	gittest.Commit(t, r.Root, "work")
	b = build(t, r, brief.Options{})
	want := strings.Join([]string{
		"- Chose E because F.",
		"- Chose C because D.",
		"- Undated: chose G.",
		"",
		"AC-2's text changed. Only a person changes a criterion (INV-4).",
		"",
		"Outside the step's territory. Explain each under \"## Territory\".",
		"",
		"- `docs/notes.md`",
		"- `go.mod`",
	}, "\n")
	if d := section(t, b, "Judgment calls"); d != want {
		t.Errorf("Judgment calls =\n%s\nwant\n%s", d, want)
	}
	if !reflect.DeepEqual(b.JudgmentCalls, []string{"Chose E because F.", "Chose C because D.", "Undated: chose G."}) {
		t.Errorf("judgment_calls = %q", b.JudgmentCalls)
	}
	if !reflect.DeepEqual(b.CriteriaChanged, []string{"AC-2"}) {
		t.Errorf("criteria_changed = %q", b.CriteriaChanged)
	}
	if !reflect.DeepEqual(b.Outside, []string{"docs/notes.md", "go.mod"}) {
		t.Errorf("outside = %q", b.Outside)
	}

	// A spec file that's new on the branch: every Decisions line counts, and no criterion has
	// changed, because there's nothing to compare it with.
	r = setup(t, spec7("This.", "", criteria))
	r.Spec(spectest.Spec{Num: 9, Slug: "fresh", Decisions: "- 2026-03-03: Chose G because H.\n"})
	b = build(t, r, brief.Options{Step: "FUR-0009.1"})
	if !reflect.DeepEqual(b.JudgmentCalls, []string{"Chose G because H."}) || len(b.CriteriaChanged) != 0 {
		t.Errorf("new spec: judgment calls %q, criteria %q", b.JudgmentCalls, b.CriteriaChanged)
	}
	if d := section(t, b, "Judgment calls"); d != "- Chose G because H." {
		t.Errorf("new spec: Judgment calls = %q", d)
	}
}

func TestBriefDoesNotExplainTerritory(t *testing.T) {
	r := setup(t, spec7("This.", "", criteria))
	r.Write("go.mod", "module x\n")
	r.Write("internal/thing/a.go", "x\n")
	md := build(t, r, brief.Options{}).Markdown()
	if !strings.Contains(md, "`go.mod`") {
		t.Fatalf("the brief should name go.mod:\n%s", md)
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "##")), "Territory") && strings.HasPrefix(strings.TrimSpace(line), "##") {
			t.Fatalf("the brief has a ## Territory heading:\n%s", md)
		}
	}
	rep, err := territory.Check(gittest.Git(r.Root), r.Load(), territory.Options{Body: md})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.Outside, []string{"go.mod"}) || len(rep.Explained) != 0 {
		t.Errorf("territory --body-file on the raw brief: outside %v, explained %v", rep.Outside, rep.Explained)
	}
}

func TestBriefNeedsYouInvariants(t *testing.T) {
	s := spec7("This.", "", criteria)
	s.Invariants = []string{"INV-2", "INV-1"}
	r := setup(t, s)
	r.Write("docs/product.md", spectest.Product)
	b := build(t, r, brief.Options{})
	want := strings.Join([]string{
		"- AC-3: It reads well",
		"  Look at: read the output",
		"- INV-1: Files are the truth." + invRaw,
		"- INV-2: Git is enough." + invRaw,
	}, "\n")
	if n := section(t, b, "Needs you"); n != want {
		t.Errorf("Needs you =\n%s\nwant\n%s", n, want)
	}
	if strings.Contains(b.Markdown(), "human_review") {
		t.Errorf("human_review is no longer listed:\n%s", b.Markdown())
	}
	if !reflect.DeepEqual(b.NeedsYou, []string{"AC-3: It reads well. Look at: read the output", "INV-1: Files are the truth", "INV-2: Git is enough"}) {
		t.Errorf("needs_you = %q", b.NeedsYou)
	}

	// Step 2 covers one test criterion; with no invariants, nothing needs you, even with
	// human_review: required.
	s = spec7("This.", "", criteria)
	s.Invariants, s.HumanReview = nil, "required"
	r = setup(t, s)
	b = build(t, r, brief.Options{Step: "FUR-0007.2"})
	if n := section(t, b, "Needs you"); n != "Nothing." {
		t.Errorf("Needs you = %q, want Nothing.", n)
	}
	if len(b.NeedsYou) != 0 || b.NeedsYou == nil {
		t.Errorf("needs_you = %#v, want []", b.NeedsYou)
	}
}

func TestBriefDeterministicAndJSON(t *testing.T) {
	r := setup(t, spec7("This.\n\n"+mermaid, "", criteria))
	r.Write("web/css/site.css", "x\n")
	r.Write("internal/thing/a.go", "x\n")
	r.Write("internal/thing/a_test.go", "x\n")
	r.Write("go.mod", "x\n")
	first := build(t, r, brief.Options{}).Markdown()
	second := build(t, r, brief.Options{}).Markdown()
	if first != second {
		t.Errorf("two runs differ:\n%s\n---\n%s", first, second)
	}

	// An empty brief: every list is [], never null.
	r = setup(t, spec7("This.", "", criteria))
	b := build(t, r, brief.Options{Step: "FUR-0007.2"})
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	keys := []string{"step", "base", "sections", "files", "judgment_calls", "criteria_changed", "outside", "needs_you", "proof"}
	for _, k := range keys {
		if _, ok := got[k]; !ok {
			t.Errorf("JSON has no %q: %s", k, data)
		}
	}
	if len(got) != len(keys) {
		t.Errorf("JSON has extra keys: %s", data)
	}
	if got["step"] != "FUR-0007.2" || got["base"] != "origin/main" {
		t.Errorf("step and base: %s", data)
	}
	if strings.Contains(string(data), "null") {
		t.Errorf("JSON has a null: %s", data)
	}
	files := got["files"].(map[string]any)
	for _, k := range []string{"code", "visual", "tests", "specs"} {
		if _, ok := files[k].([]any); !ok {
			t.Errorf("files.%s isn't a list: %s", k, data)
		}
	}
	sec := got["sections"].([]any)[0].(map[string]any)
	if sec["heading"] != "Behaviour" || !strings.Contains(sec["markdown"].(string), "<!-- furrow:") {
		t.Errorf("sections[0] = %v", sec)
	}
	if strings.Contains(first, "2026") || strings.Contains(first, "1970") {
		t.Errorf("the brief has a date:\n%s", first)
	}
}

func TestBriefFailures(t *testing.T) {
	r := setup(t, spec7("This.", "", criteria))
	g := gittest.Git(r.Root)

	gittest.Must(t, r.Root, "switch", "-q", "main")
	_, err := brief.Build(g, r.Load(), brief.Options{})
	if err == nil || err.Error() != "Not on a step branch. Pass `--step FUR-NNNN.N`." {
		t.Errorf("off a step branch: %v", err)
	}

	_, err = brief.Build(g, r.Load(), brief.Options{Step: "FUR-0099.1"})
	if err == nil || !strings.Contains(err.Error(), "FUR-0099.1") {
		t.Errorf("unknown step: %v", err)
	}

	gittest.Must(t, r.Root, "switch", "-q", "step/0007-1")
	_, err = brief.Build(g, r.Load(), brief.Options{Base: "origin/nope"})
	if err == nil || err.Error() != "Can't find origin/nope. Run `git fetch origin`, or pass `--base`." {
		t.Errorf("missing base: %v", err)
	}
	// The default base, when the remote branch was never fetched.
	gittest.Must(t, r.Root, "update-ref", "-d", "refs/remotes/origin/main")
	_, err = brief.Build(g, r.Load(), brief.Options{})
	if err == nil || err.Error() != "Can't find origin/main. Run `git fetch origin`, or pass `--base`." {
		t.Errorf("missing default base: %v", err)
	}
}
