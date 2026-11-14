package spec_test

import (
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

const fence = "```"

// mock is a Before and after section with a code block on each side.
func mock(before, after string) string {
	return "**Before:** " + before + "\n\n" + fence + "\n$ furrow next\nFUR-0030.1  Add the flag (s)\n" + fence + "\n\n" +
		"**After:** " + after + "\n\n" + fence + "\n$ furrow next\nFUR-0030.1  Add the flag (s)  ready\n" + fence + "\n"
}

// withSection is a fixture spec's body with a ## Before and after section of text, or none when
// text is "-".
func withSection(text string) string {
	b := "# FUR-0001: x\n\n## Why\n\nBecause.\n\n## What\n\nThis.\n\n"
	if text != "-" {
		b += "## Before and after\n\n" + text + "\n"
	}
	return b + "## Acceptance criteria\n\n- [ ] AC-1: It works (test: TestWorks)\n\n## Out of scope\n\n- That.\n\n## Decisions\n"
}

const missingBA = `missing section "## Before and after"`

// beforeAfterProblems lints a one-spec repo and returns the problems that mention the section.
func beforeAfterProblems(t *testing.T, s spectest.Spec) []string {
	t.Helper()
	r := spectest.New(t)
	r.Spec(s)
	var out []string
	for _, m := range lintMessages(r.Load()) {
		if strings.Contains(m, "Before and after") {
			out = append(out, m)
		}
	}
	return out
}

// AC-1
func TestLintBeforeAfterRequired(t *testing.T) {
	cases := []struct {
		name string
		spec spectest.Spec
		want bool
	}{
		{"accepted, unmerged step", spectest.Spec{Num: 1, Body: withSection("-")}, true},
		{"paused, unmerged step", spectest.Spec{Num: 1, Status: "paused", Body: withSection("-")}, true},
		{"accepted, one of two merged", spectest.Spec{Num: 1, Steps: []spectest.Step{{Merged: true}, {}}, Body: withSection("-")}, true},
		{"draft", spectest.Spec{Num: 1, Status: "draft", Body: withSection("-")}, false},
		{"every step merged", spectest.Spec{Num: 1, Steps: []spectest.Step{{Merged: true}}, Criteria: []string{"- [x] AC-1: It works (test: TestWorks)"},
			Body: strings.Replace(withSection("-"), "- [ ]", "- [x]", 1)}, false},
		{"archived", spectest.Spec{Num: 1, Archived: true, Body: strings.Replace(withSection("-"), "- [ ]", "- [x]", 1)}, false},
		{"has it", spectest.Spec{Num: 1, Body: withSection(mock("Steps one per line.", "Each step says why it's ready."))}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := beforeAfterProblems(t, c.spec)
			missing := len(got) == 1 && strings.HasSuffix(got[0], missingBA) && strings.HasPrefix(got[0], "error: specs/")
			if c.want != missing || (!c.want && len(got) != 0) {
				t.Errorf("problems = %q, want missing section: %v", got, c.want)
			}
		})
	}

	// An imported spec gets a warning instead, and is checked like any other once it has one.
	imported := map[string]string{
		"fetch":   "warning: specs/changes/0001-spec-1.md: " + missingBA,
		"edition": "warning: specs/changes/0001-spec-1.md: " + missingBA,
	}
	for kind, want := range imported {
		got := beforeAfterProblems(t, spectest.Spec{Num: 1, Imported: kind, Body: withSection("-")})
		if len(got) != 1 || got[0] != want {
			t.Errorf("imported: %s: problems = %q, want %q", kind, got, want)
		}
	}
	got := beforeAfterProblems(t, spectest.Spec{Num: 1, Imported: "fetch", Body: withSection("**Before:** a\n\n**After:** b\n")})
	if len(got) != 2 || !strings.HasPrefix(got[0], "error: ") || !strings.HasPrefix(got[1], "error: ") {
		t.Errorf("imported spec with a section: %q", got)
	}
	r0 := spectest.New(t)
	r0.Spec(spectest.Spec{Num: 1, Imported: "jira"})
	if msgs := lintMessages(r0.Load()); len(msgs) != 1 || msgs[0] != `error: specs/changes/0001-spec-1.md: imported must be fetch or edition, found "jira"` {
		t.Errorf("unknown imported: %q", msgs)
	}

	// An area is never asked for one.
	r := spectest.New(t)
	for _, m := range lintMessages(r.Load()) {
		if strings.Contains(m, "Before and after") {
			t.Errorf("area: %s", m)
		}
	}

	// A heading inside a code block doesn't count, and one there doesn't end the section.
	got = beforeAfterProblems(t, spectest.Spec{Num: 1, Body: withSection("-") + "\n" + fence + "\n## Before and after\n" + fence + "\n"})
	if len(got) != 1 || !strings.HasSuffix(got[0], missingBA) {
		t.Errorf("heading in a code block: %q", got)
	}
}

// AC-2
func TestLintBeforeAfterLines(t *testing.T) {
	words := func(n int) string { return strings.TrimSpace(strings.Repeat("word ", n)) }
	const needs = `"## Before and after" needs a **Before:** and an **After:** line`
	cases := []struct {
		name, section string
		want          []string
	}{
		{"twenty words is fine", mock(words(20), words(18)+" `code span`"), nil},
		{"after too long", mock(words(5), words(27)), []string{`**After:** in "## Before and after" is 27 words. Keep it to 20.`}},
		{"before too long", mock(words(21), words(3)), []string{`**Before:** in "## Before and after" is 21 words. Keep it to 20.`}},
		{"both too long", mock(words(22), words(23)), []string{
			`**Before:** in "## Before and after" is 22 words. Keep it to 20.`,
			`**After:** in "## Before and after" is 23 words. Keep it to 20.`}},
		{"inline code counts", mock(words(19)+" `two words`", "x"), []string{`**Before:** in "## Before and after" is 21 words. Keep it to 20.`}},
		{"comments don't", mock(words(20)+" <!-- a comment of many many words -->", "x"), nil},
		{"only its own line", mock(words(10)+"\n"+words(30), "x"), nil},
		{"no before", strings.Replace(mock("a", "b"), "**Before:**", "Before:", 1), []string{needs}},
		{"no after", strings.Replace(mock("a", "b"), "**After:**", "After", 1), []string{needs}},
		{"labels in a code block don't count", "Some text.\n\n" + fence + "\n**Before:** a\n**After:** b\n" + fence + "\n", []string{needs}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := beforeAfterProblems(t, spectest.Spec{Num: 1, Body: withSection(c.section)})
			want := make([]string, len(c.want))
			for i, w := range c.want {
				want[i] = "error: specs/changes/0001-spec-1.md: " + w
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// AC-3
func TestLintBeforeAfterShowsIt(t *testing.T) {
	const (
		noBefore = `"## Before and after" doesn't show the change. Add a code block or image under **Before:**, or say "Nothing yet."`
		noAfter  = `"## Before and after" doesn't show the change. Add a code block or image under **After:**, or say "No visible effect."`
		block    = fence + "\n$ furrow x\nout\n" + fence + "\n"
	)
	cases := []struct {
		name, section string
		want          []string
	}{
		{"blocks on both sides", mock("a", "b"), nil},
		{"no blocks", "**Before:** a\n\n**After:** b\n", []string{noBefore, noAfter}},
		{"block only under after", "**Before:** a\n\n**After:** b\n\n" + block, []string{noBefore}},
		{"block only under before", "**Before:** a\n\n" + block + "\n**After:** b\n", []string{noAfter}},
		{"before first block above the label", block + "\n**Before:** a\n\n**After:** b\n\n" + block, []string{noBefore}},
		{"images", "**Before:** a\n\n![now](now.png)\n\n**After:** b\n\n<img src=\"after.png\">\n", nil},
		{"nothing yet", "**Before:** Nothing yet.\n\n**After:** b\n\n" + block, nil},
		{"nothing yet below the label", "**Before:** a new command\n\nNothing yet.\n\n**After:** b\n\n" + block, nil},
		{"nothing yet doesn't excuse after", "**Before:** Nothing yet.\n\n**After:** b\n", []string{noAfter}},
		{"no visible effect excuses both", "**Before:** a\n\n**After:** b. No visible effect.\n", nil},
		{"no visible effect below the label", "**Before:** a\n\n**After:** b\n\nNo visible effect.\n", nil},
		{"no visible effect under before doesn't count", "**Before:** a\n\nNo visible effect.\n\n**After:** b\n\n" + block, []string{noBefore}},
		{"only in a comment", "**Before:** <!-- Nothing yet. -->\n\n**After:** b\n\n" + block, []string{noBefore}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := beforeAfterProblems(t, spectest.Spec{Num: 1, Body: withSection(c.section)})
			want := make([]string, len(c.want))
			for i, w := range c.want {
				want[i] = "error: specs/changes/0001-spec-1.md: " + w
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}
