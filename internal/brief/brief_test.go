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

func TestBriefSections(t *testing.T) {
	// Without a diagram or a visual change, Diagram and Visual proof are left out.
	r := setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	b := build(t, r, brief.Options{})
	want := []string{"Behaviour", "Where to look", "Departures from the spec", "Proven", "Needs you", "Not done"}
	if got := headings(b); !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %v\nwant       %v", got, want)
	}

	// With both, all eight, in order.
	r = setup(t, spec7("This.\n\n"+mermaid, "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	r.Write("web/css/site.css", "body {}\n")
	b = build(t, r, brief.Options{})
	want = []string{"Behaviour", "Diagram", "Where to look", "Visual proof", "Departures from the spec", "Proven", "Needs you", "Not done"}
	if got := headings(b); !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %v\nwant       %v", got, want)
	}

	// Every part the agent writes is a <!-- furrow: comment.
	for _, h := range []string{"Behaviour", "Diagram", "Where to look", "Visual proof", "Proven", "Not done"} {
		if !strings.Contains(section(t, b, h), "<!-- furrow:") {
			t.Errorf("## %s has no <!-- furrow: placeholder:\n%s", h, section(t, b, h))
		}
	}
	for _, h := range []string{"Departures from the spec", "Needs you"} {
		if strings.Contains(section(t, b, h), "<!--") {
			t.Errorf("## %s is facts, but has a placeholder:\n%s", h, section(t, b, h))
		}
	}
	if !strings.Contains(section(t, b, "Behaviour"), "80 words") {
		t.Error("Behaviour's placeholder should give the 80-word limit")
	}

	// The Markdown is the sections, as "## " headings, in the same order.
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
	if !strings.HasPrefix(md, "## Behaviour\n") || !strings.HasSuffix(md, "-->\n") {
		t.Errorf("markdown should start with ## Behaviour and end with Not done's placeholder:\n%s", md)
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

	where := section(t, b, "Where to look")
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
	where = section(t, build(t, r, brief.Options{}), "Where to look")
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
	if !strings.HasPrefix(d, "<!-- furrow: mark what this step adds or changes with classDef added") {
		t.Errorf("Diagram should open with its placeholder:\n%s", d)
	}
	if !strings.Contains(d, mermaid+"\n\n"+second) {
		t.Errorf("Diagram should hold both blocks from ## What, as is, in order:\n%s", d)
	}
	if strings.Contains(d, "not: a diagram") || strings.Contains(d, "Why --> Not") {
		t.Errorf("Diagram copied something that isn't a mermaid block in ## What:\n%s", d)
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

func TestBriefDepartures(t *testing.T) {
	// Nothing departs: None.
	r := setup(t, spec7("This.", "- 2026-01-01: Chose A because B.\n", criteria))
	r.Write("internal/thing/a.go", "x\n")
	b := build(t, r, brief.Options{})
	if d := section(t, b, "Departures from the spec"); d != "None." {
		t.Errorf("Departures = %q, want None.", d)
	}

	// Two new decisions (one wrapped over two lines), a ticked box (not a departure), a reworded
	// criterion, and two files outside the territory.
	crit := append([]string{}, criteria...)
	crit[0] = "- [x] AC-1: It works (test: TestWorks)"
	crit[1] = "- [ ] AC-2: It is checked well (check: true)"
	r.Spec(spec7("This.", "- 2026-01-01: Chose A because B.\n- 2026-02-02: Chose C\n  because D.\n- 2026-01-15: Chose E because F.\n", crit))
	r.Write("go.mod", "module x\n")
	r.Write("docs/notes.md", "x\n")
	gittest.Commit(t, r.Root, "work")
	b = build(t, r, brief.Options{})
	want := strings.Join([]string{
		"Added to `## Decisions`:",
		"",
		"- 2026-01-15: Chose E because F.",
		"- 2026-02-02: Chose C because D.",
		"",
		"AC-2's text changed. Only a person changes a criterion (INV-4).",
		"",
		"Outside the step's territory. Explain each under \"## Territory\".",
		"",
		"- `docs/notes.md`",
		"- `go.mod`",
	}, "\n")
	if d := section(t, b, "Departures from the spec"); d != want {
		t.Errorf("Departures =\n%s\nwant\n%s", d, want)
	}
	if !reflect.DeepEqual(b.DecisionsAdded, []string{"2026-01-15: Chose E because F.", "2026-02-02: Chose C because D."}) {
		t.Errorf("decisions_added = %q", b.DecisionsAdded)
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
	if !reflect.DeepEqual(b.DecisionsAdded, []string{"2026-03-03: Chose G because H."}) || len(b.CriteriaChanged) != 0 {
		t.Errorf("new spec: decisions %q, criteria %q", b.DecisionsAdded, b.CriteriaChanged)
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

func TestBriefProvenAndNeedsYou(t *testing.T) {
	crit := append([]string{}, criteria...)
	crit[1] = "- [x] AC-2: It is checked (check: true)"
	r := setup(t, spec7("This.", "", crit))
	r.Write("docs/product.md", spectest.Product)
	b := build(t, r, brief.Options{})
	ev := "  Evidence: <!-- furrow: test name, command and output, or screenshot -->"
	want := strings.Join([]string{
		"- [ ] AC-1: It works (test: TestWorks)", ev,
		"- [x] AC-2: It is checked (check: true)", ev,
	}, "\n")
	if p := section(t, b, "Proven"); p != want {
		t.Errorf("Proven =\n%s\nwant\n%s", p, want)
	}
	want = strings.Join([]string{
		"- AC-3: It reads well",
		"  Look at: read the output",
		"- INV-2: Git is enough",
		"- `human_review: required`",
	}, "\n")
	if n := section(t, b, "Needs you"); n != want {
		t.Errorf("Needs you =\n%s\nwant\n%s", n, want)
	}
	if !reflect.DeepEqual(b.NeedsYou, []string{"AC-3: It reads well. Look at: read the output", "INV-2: Git is enough", "human_review: required"}) {
		t.Errorf("needs_you = %q", b.NeedsYou)
	}

	// Step 2 covers one test criterion; without the spec's invariants and review, nothing needs you.
	s := spec7("This.", "", criteria)
	s.Invariants, s.HumanReview = nil, "optional"
	r = setup(t, s)
	b = build(t, r, brief.Options{Step: "FUR-0007.2"})
	if p := section(t, b, "Proven"); !strings.HasPrefix(p, "- [ ] AC-4: Step two works (test: TestTwo)\n") || strings.Contains(p, "AC-1") {
		t.Errorf("step 2's Proven =\n%s", p)
	}
	if n := section(t, b, "Needs you"); n != "Nothing. Every criterion has a test or a check." {
		t.Errorf("Needs you = %q", n)
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
	keys := []string{"step", "base", "sections", "files", "decisions_added", "criteria_changed", "outside", "needs_you"}
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
