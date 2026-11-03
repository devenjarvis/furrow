package lifecycle_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devenjarvis/furrow/internal/lifecycle"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

type S = spectest.Spec
type St = spectest.Step

var day = time.Date(2026, 11, 1, 15, 4, 5, 0, time.UTC)

// person is an environment with no agent variables set.
func person(string) string { return "" }

func envOf(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

// diffLines returns the lines that differ between a and b, which must have the same line count.
func diffLines(t *testing.T, a, b string) []string {
	t.Helper()
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	if len(al) != len(bl) {
		t.Fatalf("line count changed: %d -> %d\n%s", len(al), len(bl), b)
	}
	var out []string
	for i := range al {
		if al[i] != bl[i] {
			out = append(out, al[i]+" => "+bl[i])
		}
	}
	return out
}

func lintErrors(t *testing.T, r *spectest.Repo) []string {
	t.Helper()
	var out []string
	for _, p := range spec.Lint(r.Load()) {
		if !p.Warning {
			out = append(out, p.String())
		}
	}
	return out
}

func exists(r *spectest.Repo, rel string) bool {
	_, err := os.Stat(filepath.Join(r.Root, rel))
	return err == nil
}

// handWritten has comments, odd spacing and two steps, so done must leave formatting alone.
const handWritten = `---
id: FUR-0001
title: Hand written
status: accepted   # decided in review
area: core
priority: p1
human_review: optional
invariants: []
context: []
steps:
  # the first step
  - n: 1
    title: First
    after: []
    touches: [internal/a/**]
    covers: [AC-1]
    size: s
    merged: false   # not yet
  - title: Second
    n: 2
    after: [1]
    touches:
      - internal/b/**
    covers: [AC-2]
    merged:   false
---

# FUR-0001: Hand written

## Why

Because.

## What

This.

## Acceptance criteria

- [x] AC-1: The first thing works (test: TestFirst)
- [x] AC-2: The second thing works (test: TestSecond)

## Out of scope

- That.

## Decisions
`

func TestDoneEditsOneLine(t *testing.T) {
	r := spectest.New(t)
	path := "specs/changes/0001-hand.md"
	r.Write(path, handWritten)

	msg, err := lifecycle.Done(r.Load(), "FUR-0001.1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "FUR-0001.1") {
		t.Errorf("message should name the step: %q", msg)
	}
	got := r.Read(path)
	want := strings.Replace(handWritten, "    merged: false   # not yet", "    merged: true   # not yet", 1)
	if got != want {
		t.Fatalf("done changed more than one line:\n%v", diffLines(t, handWritten, got))
	}

	// Step 2 has n after title and odd spacing before its value.
	if _, err := lifecycle.Done(r.Load(), "FUR-0001.2"); err != nil {
		t.Fatal(err)
	}
	got2 := r.Read(path)
	if d := diffLines(t, got, got2); len(d) != 1 || d[0] != "    merged:   false =>     merged:   true" {
		t.Fatalf("second done: %v", d)
	}
	repo := r.Load()
	for _, st := range repo.Spec("FUR-0001").Steps {
		if !st.Merged {
			t.Errorf("step %d not merged after done", st.N)
		}
	}

	// A generated spec: only that step's line changes, never another step's.
	r.Spec(S{Num: 2, Criteria: []string{"- [x] AC-1: It works (test: TestWorks)"}, Steps: []St{{}, {}, {}}})
	before := r.Read(S{Num: 2}.Path())
	if _, err := lifecycle.Done(r.Load(), "FUR-0002.2"); err != nil {
		t.Fatal(err)
	}
	after := r.Read(S{Num: 2}.Path())
	d := diffLines(t, before, after)
	if len(d) != 1 {
		t.Fatalf("want one changed line, got %v", d)
	}
	st := r.Load().Spec("FUR-0002").Steps
	if st[0].Merged || !st[1].Merged || st[2].Merged {
		t.Errorf("wrong step merged: %v %v %v", st[0].Merged, st[1].Merged, st[2].Merged)
	}

	// Done twice is harmless and changes nothing.
	if _, err := lifecycle.Done(r.Load(), "FUR-0002.2"); err != nil {
		t.Fatal(err)
	}
	if r.Read(S{Num: 2}.Path()) != after {
		t.Error("done on a merged step changed the file")
	}
}

func TestDoneRefusals(t *testing.T) {
	r := spectest.New(t)
	r.Spec(S{Num: 1}) // one unmerged step
	r.Spec(S{Num: 2, Criteria: []string{"- [x] AC-1: It works (test: TestWorks)"}, Steps: []St{{After: []string{"FUR-0001"}}}})
	r.Spec(S{Num: 3, Criteria: []string{
		"- [x] AC-1: One (test: TestOne)",
		"- [ ] AC-2: Two (test: TestTwo)",
		"- [ ] AC-3: Three (check: go vet)",
	}})
	r.Spec(S{Num: 4, Criteria: []string{
		"- [x] AC-1: One (test: TestOne)",
		"- [ ] AC-2: Looks right (human: look at it)",
	}})
	r.Spec(S{Num: 5, Criteria: []string{"- [x] AC-1: It works (test: TestWorks)"}, Steps: []St{{Covers: []string{"AC-1"}}, {After: []string{"1"}, Covers: []string{}}}})

	cases := []struct {
		ref, path, want string
	}{
		{"FUR-0002.1", S{Num: 2}.Path(), "FUR-0002.1 is after FUR-0001, which isn't merged."},
		{"FUR-0003.1", S{Num: 3}.Path(), "Tick AC-2 and AC-3 with their evidence first."},
		{"FUR-0005.2", S{Num: 5}.Path(), "FUR-0005.2 is after FUR-0005.1, which isn't merged."},
		{"FUR-0009.1", "", "no step FUR-0009.1"},
		{"FUR-0003", "", "names a whole spec"},
	}
	for _, c := range cases {
		before := ""
		if c.path != "" {
			before = r.Read(c.path)
		}
		_, err := lifecycle.Done(r.Load(), c.ref)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("done %s: want error containing %q, got %v", c.ref, c.want, err)
		}
		if c.path != "" && r.Read(c.path) != before {
			t.Errorf("done %s changed the file after refusing", c.ref)
		}
	}

	// One unticked criterion reads in the singular.
	r.Spec(S{Num: 6})
	if _, err := lifecycle.Done(r.Load(), "FUR-0006.1"); err == nil || err.Error() != "Tick AC-1 with its evidence first." {
		t.Errorf("got %v", err)
	}

	// An unticked human criterion doesn't block done: it waits for sign-off.
	msg, err := lifecycle.Done(r.Load(), "FUR-0004.1")
	if err != nil {
		t.Fatalf("human criterion blocked done: %v", err)
	}
	if !strings.Contains(msg, "AC-2") || !strings.Contains(msg, "furrow review") {
		t.Errorf("done should say what waits for sign-off: %q", msg)
	}
}

func TestReview(t *testing.T) {
	r := spectest.New(t)
	empty := lifecycle.Review(r.Load())
	var buf bytes.Buffer
	empty.Text(&buf)
	if buf.String() != "Nothing waiting for sign-off.\n" {
		t.Errorf("empty review: %q", buf.String())
	}
	data, _ := json.Marshal(empty)
	if string(data) != `{"specs":[]}` {
		t.Errorf("empty review JSON: %s", data)
	}

	// FUR-0001: step 1 merged covers AC-2 (listed); step 2 not merged covers AC-3 (not listed).
	r.Spec(S{Num: 1, Title: "First", Criteria: []string{
		"- [x] AC-1: Works (test: TestWorks)",
		"- [ ] AC-2: Reads well (human: read `furrow help` aloud)",
		"- [ ] AC-3: Feels fast (human: time it)",
	}, Steps: []St{
		{Merged: true, Covers: []string{"AC-1", "AC-2"}},
		{Covers: []string{"AC-3"}},
	}})
	// FUR-0002: human criterion already signed off.
	r.Spec(S{Num: 2, Criteria: []string{"- [x] AC-1: Clear (human: look)"}, Steps: []St{{Merged: true}},
		Decisions: "- 2026-10-01: AC-1 signed off by Ann."})
	// FUR-0003: covered by a merged step and an unmerged one.
	r.Spec(S{Num: 3, Criteria: []string{"- [ ] AC-1: Clear (human: look)"}, Steps: []St{
		{Merged: true, Covers: []string{"AC-1"}}, {Covers: []string{"AC-1"}},
	}})
	// FUR-0004: unticked test criterion on a merged step is not for review.
	r.Spec(S{Num: 4, Steps: []St{{Merged: true}}})
	// FUR-0005: two waiting, in criterion order.
	r.Spec(S{Num: 5, Title: "Fifth", Criteria: []string{
		"- [ ] AC-1: Pretty (human: look at the screen)",
		"- [ ] AC-2: Calm (human: use it for a day)",
	}, Steps: []St{{Merged: true}}})

	rep := lifecycle.Review(r.Load())
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"specs":[` +
		`{"id":"FUR-0001","title":"First","file":"specs/changes/0001-spec-1.md","criteria":[{"id":"AC-2","text":"Reads well","look_at":"read ` + "`furrow help`" + ` aloud"}]},` +
		`{"id":"FUR-0005","title":"Fifth","file":"specs/changes/0005-spec-5.md","criteria":[{"id":"AC-1","text":"Pretty","look_at":"look at the screen"},{"id":"AC-2","text":"Calm","look_at":"use it for a day"}]}]}`
	if string(data) != want {
		t.Errorf("review JSON:\n got %s\nwant %s", data, want)
	}
	buf.Reset()
	rep.Text(&buf)
	wantText := "FUR-0001: First\n" +
		"  AC-2: Reads well\n" +
		"        Look at: read `furrow help` aloud\n" +
		"\n" +
		"FUR-0005: Fifth\n" +
		"  AC-1: Pretty\n" +
		"        Look at: look at the screen\n" +
		"  AC-2: Calm\n" +
		"        Look at: use it for a day\n" +
		"\n" +
		"When you've checked one, run `furrow signoff <id> <AC-n>...`.\n"
	if buf.String() != wantText {
		t.Errorf("review text:\n%s\nwant:\n%s", buf.String(), wantText)
	}
}

func TestSignoffInAgentSession(t *testing.T) {
	for _, k := range []string{"CLAUDECODE", "FURROW_AGENT"} {
		r := spectest.New(t)
		path := r.Spec(S{Num: 1, Criteria: []string{
			"- [ ] AC-1: Clear (human: look)",
			"- [ ] AC-2: Calm (human: use it)",
		}, Steps: []St{{Merged: true}}})
		before := r.Read(path)
		msg, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{
			ID: "FUR-0001", Criteria: []string{"AC-1"}, By: "Ann", Getenv: envOf(map[string]string{k: "1"}), Now: day,
		})
		if err != nil {
			t.Fatalf("%s set: %v", k, err)
		}
		if msg != "Signed off AC-1 on FUR-0001 by Ann." {
			t.Errorf("%s set: message %q", k, msg)
		}
		want := strings.Replace(before, "- [ ] AC-1:", "- [x] AC-1:", 1)
		want = strings.TrimRight(want, "\n") + "\n\n- 2026-11-01: AC-1 signed off by Ann.\n"
		if got := r.Read(path); got != want {
			t.Errorf("%s set: file changed wrongly: %q", k, diffLines(t, want, got))
		}
		if errs := lintErrors(t, r); len(errs) > 0 {
			t.Errorf("%s set: lint after signoff: %v", k, errs)
		}
	}
}

func TestSignoffSeveralSpecs(t *testing.T) {
	r := spectest.New(t)
	p1 := r.Spec(S{Num: 1, Slug: "one", Title: "One", Criteria: []string{
		"- [x] AC-1: Works (test: TestWorks)",
		"- [ ] AC-2: Clear (human: look)",
	}, Steps: []St{{Merged: true}}})
	p2 := r.Spec(S{Num: 2, Slug: "two", Title: "Two", Criteria: []string{
		"- [ ] AC-1: A (human: a)",
		"- [ ] AC-2: B (human: b)",
		"- [ ] AC-3: C (human: c)",
	}, Steps: []St{{Merged: true}}})
	p3 := r.Spec(S{Num: 3, Slug: "three", Title: "Three", Criteria: []string{
		"- [ ] AC-1: Pretty (human: look)",
	}, Steps: []St{{Merged: true}}})
	before2 := r.Read(p2)

	// FUR-0002 is named twice: its groups merge. FUR-0001 and FUR-0003 complete and are archived.
	msg, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{
		Args: []string{"FUR-0003", "AC-1", "FUR-0002", "AC-3", "FUR-0001", "AC-2", "FUR-0002", "AC-1"},
		By:   "Deven", Getenv: person, Now: day,
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(msg, "\n")
	want := []string{
		"Signed off AC-1 on FUR-0003 by Deven.",
		"Archived FUR-0003 to specs/archive/0003-three.md and added it to specs/areas/core.md's ## History.",
		"Check that the area's ## Behaviour already says what FUR-0003 changed.",
		"Signed off AC-1 and AC-3 on FUR-0002 by Deven.",
		"Signed off AC-2 on FUR-0001 by Deven.",
		"Archived FUR-0001 to specs/archive/0001-one.md and added it to specs/areas/core.md's ## History.",
		"Check that the area's ## Behaviour already says what FUR-0001 changed.",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("message:\n%s\nwant:\n%s", msg, strings.Join(want, "\n"))
	}

	if exists(r, p1) || exists(r, p3) || !exists(r, "specs/archive/0001-one.md") || !exists(r, "specs/archive/0003-three.md") {
		t.Fatal("completed specs weren't archived")
	}
	got2 := r.Read(p2)
	want2 := strings.Replace(before2, "- [ ] AC-1:", "- [x] AC-1:", 1)
	want2 = strings.Replace(want2, "- [ ] AC-3:", "- [x] AC-3:", 1)
	want2 = strings.TrimRight(want2, "\n") + "\n\n- 2026-11-01: AC-1 signed off by Deven.\n- 2026-11-01: AC-3 signed off by Deven.\n"
	if got2 != want2 {
		t.Errorf("FUR-0002 changed wrongly: %q", diffLines(t, want2, got2))
	}
	if !strings.Contains(r.Read("specs/archive/0001-one.md"), "- 2026-11-01: AC-2 signed off by Deven.\n") {
		t.Error("FUR-0001's sign-off line is missing from its archived file")
	}
	// Both archives land in the one area's History, in the order given.
	if !strings.HasSuffix(r.Read("specs/areas/core.md"), "## History\n\n- FUR-0003: Three (archived 2026-11-01)\n- FUR-0001: One (archived 2026-11-01)\n") {
		t.Errorf("area history:\n%s", r.Read("specs/areas/core.md"))
	}
	if errs := lintErrors(t, r); len(errs) > 0 {
		t.Fatalf("lint after signoff: %v", errs)
	}
}

func TestSignoffSeveralSpecsAllOrNothing(t *testing.T) {
	r := spectest.New(t)
	p1 := r.Spec(S{Num: 1, Criteria: []string{
		"- [x] AC-1: Works (test: TestWorks)",
		"- [ ] AC-2: Clear (human: look)",
	}, Steps: []St{{Merged: true}}})
	p2 := r.Spec(S{Num: 2, Criteria: []string{
		"- [ ] AC-1: A (human: a)",
		"- [ ] AC-2: B (test: TestB)",
		"- [ ] AC-3: Later (human: later)",
		"- [x] AC-4: Done (human: done)",
	}, Steps: []St{{Merged: true, Covers: []string{"AC-1", "AC-2", "AC-4"}}, {Covers: []string{"AC-3"}}},
		Decisions: "- 2026-10-01: AC-4 signed off by Ann."})
	snapshot := func() map[string]string {
		out := map[string]string{}
		for _, p := range []string{p1, p2, "specs/areas/core.md"} {
			out[p] = r.Read(p)
		}
		return out
	}
	before := snapshot()

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"FUR-0001", "AC-2", "FUR-0002", "AC-9"}, "FUR-0002 has no AC-9."},
		{[]string{"FUR-0001", "AC-2", "FUR-0002", "AC-2"}, "AC-2 isn't a human: criterion."},
		{[]string{"FUR-0001", "AC-2", "FUR-0002", "AC-4"}, "AC-4 is already ticked."},
		{[]string{"FUR-0001", "AC-2", "FUR-0002", "AC-3"}, "AC-3 isn't waiting for sign-off: step 2 isn't merged."},
		{[]string{"FUR-0001", "AC-2", "FUR-0002"}, "Name at least one criterion after FUR-0002."},
		{[]string{"FUR-0001", "FUR-0002", "AC-1"}, "Name at least one criterion after FUR-0001."},
		{[]string{"FUR-0001", "AC-2", "FUR-0009", "AC-1"}, "no spec FUR-0009"},
		{[]string{"AC-2", "FUR-0001", "AC-2"}, "Start with a spec id, such as `furrow signoff FUR-0005 AC-8`."},
		{nil, "Start with a spec id, such as `furrow signoff FUR-0005 AC-8`."},
	}
	for _, c := range cases {
		_, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{Args: c.args, By: "Ann", Getenv: person, Now: day})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("signoff %v: want %q, got %v", c.args, c.want, err)
		}
		after := snapshot()
		for p := range before {
			if after[p] != before[p] {
				t.Errorf("signoff %v changed %s after refusing", c.args, p)
			}
		}
		if exists(r, "specs/archive/0001-spec-1.md") {
			t.Errorf("signoff %v archived FUR-0001 after refusing", c.args)
		}
	}
}

func TestSignoffRefusals(t *testing.T) {
	isolateGit(t) // with no --by, signoff asks git, so the user's own name mustn't leak in
	r := spectest.New(t)
	path := r.Spec(S{Num: 1, Criteria: []string{
		"- [x] AC-1: Works (test: TestWorks)",
		"- [ ] AC-2: Clear (human: look)",
		"- [ ] AC-3: Later (human: look later)",
	}, Steps: []St{{Merged: true, Covers: []string{"AC-1", "AC-2"}}, {Covers: []string{"AC-3"}}}})
	before := r.Read(path)
	cases := []struct {
		acs  []string
		by   string
		want string
	}{
		{[]string{"AC-2"}, "", "Say who checked it: --by NAME."},
		{[]string{"AC-2"}, "Ann\nEvil", "--by must be one line."},
		{nil, "Ann", "Name at least one criterion"},
		{[]string{"AC-2", "AC-1"}, "Ann", "AC-1 isn't a human: criterion."},
		{[]string{"AC-2", "AC-3"}, "Ann", "AC-3 isn't waiting for sign-off: step 2 isn't merged."},
		{[]string{"AC-9"}, "Ann", "FUR-0001 has no AC-9."},
	}
	for _, c := range cases {
		_, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: c.acs, By: c.by, Getenv: person, Now: day})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("signoff %v by %q: want %q, got %v", c.acs, c.by, c.want, err)
		}
		if r.Read(path) != before {
			t.Errorf("signoff %v changed the file after refusing", c.acs)
		}
	}
	if _, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0007", Criteria: []string{"AC-1"}, By: "Ann", Getenv: person, Now: day}); err == nil {
		t.Error("unknown spec was allowed")
	}
	// Signed off already: not waiting any more.
	if _, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-2"}, By: "Ann", Getenv: person, Now: day}); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-2"}, By: "Ann", Getenv: person, Now: day}); err == nil || !strings.Contains(err.Error(), "already ticked") {
		t.Errorf("second signoff: %v", err)
	}
}

func TestSignoffArchives(t *testing.T) {
	r := spectest.New(t)
	path := r.Spec(S{Num: 1, Slug: "thing", Title: "A thing", Criteria: []string{
		"- [x] AC-1: Works (test: TestWorks)",
		"- [ ] AC-2: Reads well (human: read it)",
		"- [ ] AC-3: Feels calm (human: use it)",
	}, Steps: []St{{Merged: true}}, Decisions: "- 2026-10-30: Chose the short name."})
	before := r.Read(path)

	msg, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-2"}, By: "Ann Lee", Getenv: person, Now: day})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "AC-2") || strings.Contains(msg, "Archived") {
		t.Errorf("message: %q", msg)
	}
	mid := r.Read(path)
	wantMid := strings.Replace(before, "- [ ] AC-2:", "- [x] AC-2:", 1)
	wantMid = strings.Replace(wantMid, "- 2026-10-30: Chose the short name.\n", "- 2026-10-30: Chose the short name.\n- 2026-11-01: AC-2 signed off by Ann Lee.\n", 1)
	if mid != wantMid {
		t.Fatalf("after first signoff:\n%s\nwant:\n%s", mid, wantMid)
	}
	if errs := lintErrors(t, r); len(errs) > 0 {
		t.Fatalf("lint after signoff: %v", errs)
	}

	msg, err = lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-3"}, By: "Ann Lee", Getenv: person, Now: day})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "Archived FUR-0001") {
		t.Errorf("signoff should say it archived: %q", msg)
	}
	if exists(r, path) || !exists(r, "specs/archive/0001-thing.md") {
		t.Fatal("complete spec wasn't moved to the archive")
	}
	arch := r.Read("specs/archive/0001-thing.md")
	if !strings.Contains(arch, "- 2026-11-01: AC-3 signed off by Ann Lee.\n") || !strings.Contains(arch, "\nstatus: done\n") {
		t.Errorf("archived file:\n%s", arch)
	}
	if !strings.HasSuffix(r.Read("specs/areas/core.md"), "## History\n\n- FUR-0001: A thing (archived 2026-11-01)\n") {
		t.Errorf("area history:\n%s", r.Read("specs/areas/core.md"))
	}
	if errs := lintErrors(t, r); len(errs) > 0 {
		t.Fatalf("lint after archive: %v", errs)
	}

	// Several at once, out of order: ticked and logged in criterion order.
	r2 := spectest.New(t)
	p2 := r2.Spec(S{Num: 1, Criteria: []string{
		"- [ ] AC-1: A (human: a)",
		"- [ ] AC-2: B (human: b)",
		"- [ ] AC-3: C (test: TestC)",
	}, Steps: []St{{Merged: true}}})
	if _, err := lifecycle.Signoff(r2.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-2", "AC-1", "AC-2"}, By: "Bo", Getenv: person, Now: day}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(r2.Read(p2), "## Decisions\n\n- 2026-11-01: AC-1 signed off by Bo.\n- 2026-11-01: AC-2 signed off by Bo.\n") {
		t.Errorf("decisions:\n%s", r2.Read(p2))
	}
}

func TestArchive(t *testing.T) {
	r := spectest.New(t)
	if err := os.RemoveAll(filepath.Join(r.Root, "specs/archive")); err != nil {
		t.Fatal(err)
	}
	incomplete := r.Spec(S{Num: 1, Criteria: []string{
		"- [x] AC-1: A (test: TestA)",
		"- [ ] AC-2: B (human: look)",
	}, Steps: []St{{Merged: true, Covers: []string{"AC-1", "AC-2"}}, {Covers: []string{}}}})
	before := r.Read(incomplete)
	areaBefore := r.Read("specs/areas/core.md")
	_, err := lifecycle.Archive(r.Load(), "FUR-0001", day)
	if err == nil || !strings.Contains(err.Error(), "step 2 isn't merged") || !strings.Contains(err.Error(), "AC-2 isn't ticked") {
		t.Errorf("incomplete archive: %v", err)
	}
	if r.Read(incomplete) != before || r.Read("specs/areas/core.md") != areaBefore || exists(r, "specs/archive") {
		t.Error("refused archive changed something")
	}

	done := r.Spec(S{Num: 2, Slug: "done-one", Title: "Done one", Criteria: []string{"- [x] AC-1: A (test: TestA)"}, Steps: []St{{Merged: true}}})
	r.Spec(S{Num: 3, Title: "Third", Criteria: []string{"- [x] AC-1: A (test: TestA)"}, Steps: []St{{Merged: true}}})
	src := r.Read(done)
	msg, err := lifecycle.Archive(r.Load(), "FUR-0002", day)
	if err != nil {
		t.Fatal(err)
	}
	if exists(r, done) {
		t.Error("file still in changes/")
	}
	got := r.Read("specs/archive/0002-done-one.md")
	if want := strings.Replace(src, "\nstatus: accepted\n", "\nstatus: done\n", 1); got != want {
		t.Errorf("archived file changed more than status: %v", diffLines(t, src, got))
	}
	if r.Read("specs/areas/core.md") != areaBefore+"\n- FUR-0002: Done one (archived 2026-11-01)\n" {
		t.Errorf("area:\n%s", r.Read("specs/areas/core.md"))
	}
	for _, want := range []string{"specs/archive/0002-done-one.md", "## History", "## Behaviour"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message should mention %q: %q", want, msg)
		}
	}

	if _, err := lifecycle.Archive(r.Load(), "FUR-0003", day); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(r.Read("specs/areas/core.md"), "## History\n\n- FUR-0002: Done one (archived 2026-11-01)\n- FUR-0003: Third (archived 2026-11-01)\n") {
		t.Errorf("second history line:\n%s", r.Read("specs/areas/core.md"))
	}
	if _, err := lifecycle.Archive(r.Load(), "FUR-0002", day); err == nil || !strings.Contains(err.Error(), "already archived") {
		t.Errorf("archive twice: %v", err)
	}
	if _, err := lifecycle.Archive(r.Load(), "FUR-0002.1", day); err == nil {
		t.Error("a step ref was accepted")
	}

	// A spec whose area file is missing is refused before anything moves.
	r.Spec(S{Num: 4, Area: "ghost", Criteria: []string{"- [x] AC-1: A (test: TestA)"}, Steps: []St{{Merged: true}}})
	if _, err := lifecycle.Archive(r.Load(), "FUR-0004", day); err == nil || !exists(r, S{Num: 4}.Path()) {
		t.Errorf("missing area: %v", err)
	}
}

// criteriaTexts returns every criterion's text, keyed by spec id.
func criteriaTexts(repo *spec.Repo) map[string][]string {
	out := map[string][]string{}
	for _, s := range repo.Specs {
		for _, c := range s.Criteria {
			out[s.ID] = append(out[s.ID], c.ID+": "+c.Text)
		}
	}
	return out
}

func TestCriteriaTextUntouched(t *testing.T) {
	r := spectest.New(t)
	tricky := []string{
		"- [x] AC-1: `merged: false` stays as written, even with - [ ] inside (test: TestOne)",
		"- [ ] AC-2: status: accepted (and (nested) parens) look fine (human: read `status: accepted`)",
		"- [ ] AC-3: AC-2 signed off by nobody (human: check AC-3 text)",
	}
	path := r.Spec(S{Num: 1, Criteria: tricky, Steps: []St{{Covers: []string{"AC-1", "AC-2"}}, {After: []string{"1"}, Covers: []string{"AC-3"}}}})
	start := criteriaTexts(r.Load())

	check := func(stage string) {
		t.Helper()
		got := criteriaTexts(r.Load())
		if len(got["FUR-0001"]) != 3 {
			t.Fatalf("%s: lost criteria: %v", stage, got)
		}
		for i := range start["FUR-0001"] {
			if got["FUR-0001"][i] != start["FUR-0001"][i] {
				t.Errorf("%s changed a criterion:\n got %q\nwant %q", stage, got["FUR-0001"][i], start["FUR-0001"][i])
			}
		}
	}
	mustOK := func(_ string, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	mustOK(lifecycle.Done(r.Load(), "FUR-0001.1"))
	check("done 1")
	mustOK(lifecycle.Done(r.Load(), "FUR-0001.2"))
	check("done 2")
	mustOK(lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-2"}, By: "Ann", Getenv: person, Now: day}))
	check("signoff AC-2")
	mustOK(lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-3"}, By: "Ann", Getenv: person, Now: day}))
	check("signoff AC-3 and archive")
	if exists(r, path) {
		t.Fatal("spec should be archived")
	}
	// Only the boxes differ in the criteria lines themselves.
	arch := r.Read("specs/archive/0001-spec-1.md")
	for _, line := range tricky {
		ticked := "- [x]" + line[5:]
		if !strings.Contains(arch, ticked+"\n") {
			t.Errorf("archived file lacks %q", ticked)
		}
	}
}
