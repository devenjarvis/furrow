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
	"Behaviour": "**Before:** The thing did nothing.\n**After:** It does the thing.\n\n```\n$ thing\ndone\n```",
	"Diagram":   "How the thing flows.\n\n" + mermaid + "\n\nLegend: a box is a package, an arrow is a call.",
	"Review this": "1. `internal/thing/a.go` `Do`: is the loop bounded?\n\n" +
		"<details><summary>2 changed files</summary>\n\n**Code**\n\n- `internal/thing/a.go`\n\n</details>",
	"Visual proof": "- `web/css/site.css`\n\n| Screen | Before | After |\n| --- | --- | --- |\n" +
		"| Home | ![before](https://example.com/b.png) | ![after](https://example.com/a.png) |\n" +
		"| Settings | new | [after](https://example.com/s.png) |",
	"Judgment calls": "None.",
	"Needs you":      "- AC-3: It reads well\n  Look at: read the output\n- INV-2: Git is enough. Not touched.",
	"Not done": "Nothing.\n\n<details><summary>Proof: AC-1 ✓ AC-2 ✓</summary>\n\n" +
		"- AC-1 (test: TestWorks)\n- AC-2 (check: true)\n\n</details>",
}

var order = []string{"Behaviour", "Diagram", "Review this", "Visual proof", "Judgment calls", "Needs you", "Not done"}

// words is n words, for testing word limits.
func words(n int) string { return strings.TrimSpace(strings.Repeat("word ", n)) }

// longBehaviour is filled's Behaviour (10 words outside its code block) plus 102 more: 112.
var longBehaviour = filled["Behaviour"] + "\n\n" + words(102)

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

	cases := []struct {
		name   string
		change map[string]string
		want   []string
	}{
		{"missing section", map[string]string{"Visual proof": "-"},
			[]string{"No `## Visual proof` section. Run `furrow brief` and paste it."}},
		{"missing sections, in section order", map[string]string{"Not done": "-", "Behaviour": "-"},
			[]string{"No `## Behaviour` section. Run `furrow brief` and paste it.", "No `## Not done` section. Run `furrow brief` and paste it."}},
		{"placeholder", map[string]string{"Behaviour": "**Before:** a.\n**After:** b.\n\nNo visible effect. <!-- furrow: show it -->"},
			[]string{"`## Behaviour` still has a placeholder. Write it."}},
		{"placeholder in a fact section", map[string]string{"Judgment calls": "- Chose A. <!-- furrow: why -->"},
			[]string{"`## Judgment calls` still has a placeholder. Write it."}},
		{"long Behaviour", map[string]string{"Behaviour": longBehaviour},
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
		"Behaviour":    longBehaviour + " <!-- furrow: x -->",
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
		"Behaviour": filled["Behaviour"] + "\n\n" + words(70) + " <!-- a note -->",
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

func TestBriefCheckReviewable(t *testing.T) {
	_, check := checkSetup(t)
	shows := "\n\nNo visible effect."
	bad := []struct {
		name   string
		change map[string]string
		want   []string
	}{
		{"no After line", map[string]string{"Behaviour": "**Before:** It did nothing." + shows},
			[]string{"`## Behaviour` needs a `**Before:**` and an `**After:**` line."}},
		{"no Before line", map[string]string{"Behaviour": "**After:** It does it." + shows},
			[]string{"`## Behaviour` needs a `**Before:**` and an `**After:**` line."}},
		{"Before and After only in a code block", map[string]string{"Behaviour": "```\n**Before:** a\n**After:** b\n```"},
			[]string{"`## Behaviour` needs a `**Before:**` and an `**After:**` line."}},
		{"After over 20 words", map[string]string{"Behaviour": "**Before:** It did nothing.\n**After:** " + words(27) + shows},
			[]string{"`**After:**` is 27 words. Keep it to 20."}},
		{"both over 20 words", map[string]string{"Behaviour": "**Before:** " + words(21) + "\n**After:** " + words(22) + " <!-- a note -->" + shows},
			[]string{"`**Before:**` is 21 words. Keep it to 20.", "`**After:**` is 22 words. Keep it to 20."}},
		{"shows nothing", map[string]string{"Behaviour": "**Before:** a.\n**After:** b.\n\nIt is better now."},
			[]string{"`## Behaviour` doesn't show the change. Add output, a screenshot, or a usage example."}},
		{"\"No visible effect.\" only in a comment", map[string]string{"Behaviour": "**Before:** a.\n**After:** b.\n\n<!-- \"No visible effect.\" is an answer -->"},
			[]string{"`## Behaviour` doesn't show the change. Add output, a screenshot, or a usage example."}},
		{"5 Review items", map[string]string{"Review this": "1. a\n2. b\n3. c\n4. d\n5. e\n\n<details><summary>2 changed files</summary>\n\n- `x`\n\n</details>"},
			[]string{"`## Review this` has 5 items. Keep the 3 that need a person."}},
		{"placeholder inside <details>", map[string]string{"Review this": "1. a\n\n<details><summary>2 changed files</summary>\n\n<!-- furrow: x -->\n\n</details>"},
			[]string{"`## Review this` still has a placeholder. Write it."}},
		{"a long judgment call", map[string]string{"Judgment calls": "- Chose A.\n- " + words(41)},
			[]string{"A judgment call is 41 words. Shorten it in the spec's `## Decisions` and run `furrow brief` again."}},
	}
	for _, c := range bad {
		rep := check(body(c.change))
		if rep.OK || !reflect.DeepEqual(rep.Problems, c.want) {
			t.Errorf("%s: ok=%v problems =\n%q\nwant\n%q", c.name, rep.OK, rep.Problems, c.want)
		}
	}

	// In section order: Behaviour's, then Review this's, then Judgment calls'.
	rep := check(body(map[string]string{
		"Judgment calls": "- " + words(31),
		"Review this":    "1. a\n2. b\n3. c\n4. d",
		"Behaviour":      "**After:** " + words(21),
	}))
	want := []string{
		"`## Behaviour` needs a `**Before:**` and an `**After:**` line.",
		"`**After:**` is 21 words. Keep it to 20.",
		"`## Behaviour` doesn't show the change. Add output, a screenshot, or a usage example.",
		"`## Review this` has 4 items. Keep the 3 that need a person.",
		"A judgment call is 31 words. Shorten it in the spec's `## Decisions` and run `furrow brief` again.",
	}
	if !reflect.DeepEqual(rep.Problems, want) {
		t.Errorf("problems =\n%q\nwant\n%q", rep.Problems, want)
	}

	// The raw brief: Behaviour's placeholders don't count as showing the change.
	b, _ := checkSetup(t)
	var problems []string
	for _, p := range brief.Check(b, b.Markdown()).Problems {
		if strings.Contains(p, "Behaviour") {
			problems = append(problems, p)
		}
	}
	want = []string{
		"`## Behaviour` still has a placeholder. Write it.",
		"`## Behaviour` doesn't show the change. Add output, a screenshot, or a usage example.",
	}
	if !reflect.DeepEqual(problems, want) {
		t.Errorf("raw Behaviour problems =\n%q\nwant\n%q", problems, want)
	}

	good := map[string]map[string]string{
		"20-word lines and an image": {"Behaviour": "**Before:** " + words(20) + "\n**After:** " + words(20) + "\n\n![after](https://example.com/a.png)"},
		"an <img> tag":               {"Behaviour": "**Before:** a.\n**After:** b.\n\n<img src=\"a.png\" alt=\"after\">"},
		"No visible effect.":         {"Behaviour": "**Before:** a.\n**After:** b." + shows},
		"a ~~~ fence":                {"Behaviour": "**Before:** a.\n**After:** b.\n\n~~~\n$ x\n~~~"},
		// The 80-word cap counts only text outside code blocks and images.
		"long code and image text": {"Behaviour": filled["Behaviour"] + "\n\n```\n" + words(200) + "\n```\n\n![" + words(100) + "](a.png)\n\n" + words(70)},
		// A placeholder inside a fenced code block is shown output, not a placeholder: the raw brief
		// pasted as "show it", with its inner fences, closed only by a run at least as long.
		"pasted raw brief": {"Behaviour": filled["Behaviour"] + "\n\n````markdown\n" + b.Markdown() + "\n```\n## Not a heading\n````"},
		"3 Review items":   {"Review this": "1. a\n2. b\n3. c\n\n<details><summary>2 changed files</summary>\n\n- `x`\n\n</details>"},
		"30-word judgment": {"Judgment calls": "- " + words(30) + "\n- Chose B."},
	}
	for name, change := range good {
		if rep := check(body(change)); !rep.OK {
			t.Errorf("%s: %q", name, rep.Problems)
		}
	}
	// ~~~ fences hide placeholders too.
	if rep := check(body(map[string]string{"Not done": "Nothing.\n\n~~~~\n<!-- furrow: x -->\n~~~\n~~~~"})); !rep.OK {
		t.Errorf("placeholder in a ~~~~ fence: %q", rep.Problems)
	}
}
