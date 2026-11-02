package brief_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/brief"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
)

// filled is a complete brief for spec 7's step 1, with a diagram and visual proof. Each test
// breaks it in one place.
var filled = map[string]string{
	"Behaviour":     "Before, the thing did nothing. After, it does the thing.",
	"Diagram":       "How the thing flows.\n\n" + mermaid + "\n\nLegend: a box is a package, an arrow is a call.",
	"Where to look": "**Code**\n\n`internal/thing/a.go` carries it.\n\n- `internal/thing/a.go`",
	"Visual proof": "- `web/css/site.css`\n\n| Screen | Before | After |\n| --- | --- | --- |\n" +
		"| Home | ![before](https://example.com/b.png) | ![after](https://example.com/a.png) |\n" +
		"| Settings | new | [after](https://example.com/s.png) |",
	"Departures from the spec": "None.",
	"Proven":                   "- [x] AC-1: It works (test: TestWorks)\n  Evidence: TestWorks passes.",
	"Needs you":                "- AC-3: It reads well\n  Look at: read the output",
	"Not done":                 "Nothing.",
}

var order = []string{"Behaviour", "Diagram", "Where to look", "Visual proof", "Departures from the spec", "Proven", "Needs you", "Not done"}

// body writes the brief with sections in order, replacing any in change, and leaving out any
// whose replacement is "-".
func body(change map[string]string) string {
	var parts []string
	for _, h := range order {
		md := filled[h]
		if c, ok := change[h]; ok {
			if c == "-" {
				continue
			}
			md = c
		}
		parts = append(parts, "## "+h+"\n\n"+md+"\n")
	}
	return strings.Join(parts, "\n")
}

// checkSetup is a step branch whose brief has every section, Diagram and Visual proof included.
func checkSetup(t *testing.T) (*brief.Brief, func(string) *brief.Report) {
	t.Helper()
	r := setup(t, spec7("This.\n\n"+mermaid, "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	r.Write("web/css/site.css", "body {}\n")
	b := build(t, r, brief.Options{})
	return b, func(text string) *brief.Report { return brief.Check(b, text) }
}

func TestBriefCheckProblems(t *testing.T) {
	_, check := checkSetup(t)
	long := strings.TrimSpace(strings.Repeat("word ", 112))

	cases := []struct {
		name   string
		change map[string]string
		want   []string
	}{
		{"missing section", map[string]string{"Visual proof": "-"},
			[]string{"No `## Visual proof` section. Run `furrow brief` and paste it."}},
		{"missing sections, in section order", map[string]string{"Not done": "-", "Behaviour": "-"},
			[]string{"No `## Behaviour` section. Run `furrow brief` and paste it.", "No `## Not done` section. Run `furrow brief` and paste it."}},
		{"placeholder", map[string]string{"Behaviour": "<!-- furrow: before → after, in at most 80 words. Behaviour, not code. -->"},
			[]string{"`## Behaviour` still has a placeholder. Write it."}},
		{"placeholder in a fact section", map[string]string{"Proven": "- [x] AC-1: It works\n  Evidence: <!-- furrow: test name -->"},
			[]string{"`## Proven` still has a placeholder. Write it."}},
		{"long Behaviour", map[string]string{"Behaviour": long},
			[]string{"`## Behaviour` is 112 words. Keep it to 80."}},
		{"no mermaid block", map[string]string{"Diagram": "How it flows.\n\nLegend: a box is a package."},
			[]string{"`## Diagram` has no ```mermaid block. Copy the spec's diagram from `furrow brief`."}},
		{"no Legend line", map[string]string{"Diagram": "How it flows.\n\n" + mermaid + "\n\nA box is a package."},
			[]string{"`## Diagram` has no line starting `Legend:`. Say what a box, an arrow and each colour mean."}},
		{"no table row", map[string]string{"Visual proof": "- `web/css/site.css`\n\n| Screen | Before | After |\n| --- | --- | --- |"},
			[]string{"`## Visual proof` has no table row. Add one per screen, with before and after images."}},
		{"no after image", map[string]string{"Visual proof": "| Screen | Before | After |\n| --- | --- | --- |\n" +
			"| Home | new | ![a](a.png) |\n| Settings | ![b](b.png) | see above |"},
			[]string{"Row 2 of `## Visual proof` has no after image."}},
		{"no before image or new", map[string]string{"Visual proof": "| Screen | Before | After |\n| --- | --- | --- |\n" +
			"| Home | was blue | ![a](a.png) |"},
			[]string{"Row 1 of `## Visual proof` has no before image, or \"new\"."}},
	}
	for _, c := range cases {
		rep := check(body(c.change))
		if rep.OK || !reflect.DeepEqual(rep.Problems, c.want) {
			t.Errorf("%s: ok=%v problems =\n%q\nwant\n%q", c.name, rep.OK, rep.Problems, c.want)
		}
		if rep.ExitCode() != 1 {
			t.Errorf("%s: exit %d, want 1", c.name, rep.ExitCode())
		}
	}

	// Several problems: one line each, in section order, whatever order the body has them in.
	rep := check(body(map[string]string{
		"Behaviour":    long + " <!-- furrow: x -->",
		"Diagram":      "nothing",
		"Visual proof": "-",
		"Not done":     "<!-- furrow: what this step leaves out -->",
	}))
	want := []string{
		"`## Behaviour` still has a placeholder. Write it.",
		"`## Behaviour` is 112 words. Keep it to 80.",
		"`## Diagram` has no ```mermaid block. Copy the spec's diagram from `furrow brief`.",
		"`## Diagram` has no line starting `Legend:`. Say what a box, an arrow and each colour mean.",
		"No `## Visual proof` section. Run `furrow brief` and paste it.",
		"`## Not done` still has a placeholder. Write it.",
	}
	if !reflect.DeepEqual(rep.Problems, want) {
		t.Errorf("problems =\n%q\nwant\n%q", rep.Problems, want)
	}
	var out strings.Builder
	rep.Text(&out)
	if out.String() != strings.Join(want, "\n")+"\n" {
		t.Errorf("text should be one line per problem:\n%s", out.String())
	}

	// A placeholder outside the brief's sections counts too: anywhere means anywhere.
	rep = check("<!-- furrow: intro -->\n\n" + body(nil) + "\n## Territory\n\n<!-- furrow: explain -->\n")
	want = []string{
		"The text before `## Behaviour` still has a placeholder. Write it.",
		"`## Territory` still has a placeholder. Write it.",
	}
	if !reflect.DeepEqual(rep.Problems, want) {
		t.Errorf("stray placeholders =\n%q\nwant\n%q", rep.Problems, want)
	}

	// The raw brief, just as furrow brief prints it, is incomplete.
	b, _ := checkSetup(t)
	if rep := brief.Check(b, b.Markdown()); rep.OK || len(rep.Problems) == 0 {
		t.Errorf("the raw brief passed --check: %q", rep.Problems)
	}
}

func TestBriefCheckComplete(t *testing.T) {
	_, check := checkSetup(t)
	rep := check(body(nil))
	if !rep.OK || len(rep.Problems) != 0 || rep.ExitCode() != 0 {
		t.Fatalf("a complete brief: ok=%v problems=%q", rep.OK, rep.Problems)
	}
	var out strings.Builder
	rep.Text(&out)
	if out.String() != "" {
		t.Errorf("a complete brief prints nothing, got %q", out.String())
	}

	// Exactly 80 words is fine; headings inside a code fence don't start a section; a comment
	// that isn't a furrow placeholder is fine.
	rep = check(body(map[string]string{
		"Behaviour": strings.TrimSpace(strings.Repeat("word ", 80)) + " <!-- a note -->",
		"Not done":  "Nothing.\n\n```\n## Behaviour\n```",
	}))
	if !rep.OK {
		t.Errorf("80 words, a fenced heading and a plain comment: %q", rep.Problems)
	}

	// Without a diagram or a visual change, those sections aren't asked for.
	r := setup(t, spec7("This.", "", criteria))
	r.Write("internal/thing/a.go", "package thing\n")
	plain := build(t, r, brief.Options{})
	if rep := brief.Check(plain, body(map[string]string{"Diagram": "-", "Visual proof": "-"})); !rep.OK {
		t.Errorf("no diagram or visual change, but: %q", rep.Problems)
	}

	// CheckFile reads the brief from a file and changes nothing: not the file, the spec or the tree.
	file := filepath.Join(r.Root, "pr.md")
	text := body(map[string]string{"Diagram": "-", "Visual proof": "-"})
	r.Write("pr.md", text)
	spec := r.Read(specFile)
	status := gittest.Must(t, r.Root, "status", "--porcelain")
	rep, err := brief.CheckFile(gittest.Git(r.Root), r.Load(), brief.Options{}, file)
	if err != nil || !rep.OK {
		t.Fatalf("CheckFile: %v %+v", err, rep)
	}
	if r.Read("pr.md") != text || r.Read(specFile) != spec || gittest.Must(t, r.Root, "status", "--porcelain") != status {
		t.Error("--check changed a file")
	}
	if _, err := brief.CheckFile(gittest.Git(r.Root), r.Load(), brief.Options{}, filepath.Join(r.Root, "nope.md")); err == nil {
		t.Error("a missing file should be an error")
	}
	if _, err := os.Stat(filepath.Join(r.Root, "nope.md")); err == nil {
		t.Error("--check created a file")
	}

	// --json: ok and problems, problems never null.
	for _, rep := range []*brief.Report{check(body(nil)), check(body(map[string]string{"Behaviour": "-"}))} {
		data, err := json.Marshal(rep)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if _, ok := got["ok"].(bool); !ok || len(got) != 2 {
			t.Errorf("JSON should be {ok, problems}: %s", data)
		}
		if _, ok := got["problems"].([]any); !ok {
			t.Errorf("problems should be a list, never null: %s", data)
		}
	}
	if data, _ := json.Marshal(check(body(nil))); string(data) != `{"ok":true,"problems":[]}` {
		t.Errorf("complete JSON = %s", data)
	}
}
