package brief_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/brief"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

const draftFile = "specs/changes/0007-draft.md"

// questions are spec 7's open questions: Q2 has no proposal, and Q3 wraps.
const questions = "- Is it called thing? Proposed: yes.\n" +
	"- Should it be fast?\n" +
	"- Does it wrap? Proposed: yes, over\n  two lines.\n"

// draft is spec 7 as a writer proposes it: two steps, a human criterion, an invariant.
func draft(open, decisions string, crit []string) spectest.Spec {
	if crit == nil {
		crit = []string{
			"- [ ] AC-1: It works (test: TestWorks)",
			"- [ ] AC-2: It reads well (human: read the output)",
		}
	}
	body := "# FUR-0007: Draft\n\n## Why\n\nBecause.\n\n## What\n\nThis.\n\n## Acceptance criteria\n\n" +
		strings.Join(crit, "\n") + "\n\n## Out of scope\n\n- That.\n\n"
	if open != "" {
		body += "## Open questions\n\n" + open + "\n"
	}
	body += "## Decisions\n\n" + decisions
	return spectest.Spec{
		Num: 7, Slug: "draft", Title: "Draft", Status: "accepted", Invariants: []string{"INV-2"},
		Steps: []spectest.Step{
			{Title: "Build it", Size: "m", Touches: []string{"internal/thing/**"}, Covers: []string{"AC-1", "AC-2"}},
			{Title: "Use it", After: []string{"1"}, Touches: []string{"cmd/furrow/thing.go"}, Covers: []string{}},
		},
		Criteria: crit,
		Body:     body,
	}
}

// specRepo commits a repo with the given specs on main with a bare remote, and switches to a
// spec branch. It returns the repo and the remote's path.
func specRepo(t *testing.T, specs ...spectest.Spec) (*spectest.Repo, string) {
	t.Helper()
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	for _, s := range specs {
		r.Spec(s)
	}
	remote := gittest.Init(t, r.Root)
	gittest.Must(t, r.Root, "switch", "-q", "-c", "spec/0007")
	return r, remote
}

func buildSpec(t *testing.T, r *spectest.Repo, opt brief.SpecOptions) *brief.SpecBrief {
	t.Helper()
	if opt.Spec == "" {
		opt.Spec = "FUR-0007"
	}
	b, err := brief.BuildSpec(gittest.Git(r.Root), r.Load(), opt)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func specHeadings(b *brief.SpecBrief) []string {
	out := []string{}
	for _, s := range b.Sections {
		out = append(out, s.Heading)
	}
	return out
}

func specSection(t *testing.T, b *brief.SpecBrief, heading string) string {
	t.Helper()
	for _, s := range b.Sections {
		if s.Heading == heading {
			return s.Markdown
		}
	}
	t.Fatalf("no ## %s in %v", heading, specHeadings(b))
	return ""
}

// lineOf returns the 1-based line of the first line in file that starts with prefix.
func lineOf(t *testing.T, r *spectest.Repo, file, prefix string) int {
	t.Helper()
	for i, l := range strings.Split(r.Read(file), "\n") {
		if strings.HasPrefix(l, prefix) {
			return i + 1
		}
	}
	t.Fatalf("no line %q in %s", prefix, file)
	return 0
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestSpecBriefSections(t *testing.T) {
	// A new spec file: Decision, Your calls, Plan, Collisions, What it costs later.
	r, _ := specRepo(t)
	r.Spec(draft(questions, "", nil))
	b := buildSpec(t, r, brief.SpecOptions{Offline: true})
	want := []string{"Decision", "Your calls", "Plan", "Collisions", "What it costs later"}
	if got := specHeadings(b); !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %v\nwant       %v", got, want)
	}
	if d := specSection(t, b, "Decision"); !strings.HasPrefix(d, "**Commits us to:** <!-- furrow:") || !strings.Contains(d, "25 words") {
		t.Errorf("Decision should be placeholder lines giving the 25-word limit: %q", d)
	}

	// Your calls: merging accepts the proposals; the question without one comes first.
	calls := strings.Join([]string{
		brief.YourCallsIntro,
		"",
		"Needs an answer before merging:",
		"",
		"**Q2** Should it be fast? (" + draftFile + ":" + itoa(lineOf(t, r, draftFile, "- Should it")) + ")",
		"",
		"**Q1** Is it called thing? (" + draftFile + ":" + itoa(lineOf(t, r, draftFile, "- Is it")) + ")",
		"Proposed: yes.",
		"",
		"**Q3** Does it wrap? (" + draftFile + ":" + itoa(lineOf(t, r, draftFile, "- Does it")) + ")",
		"Proposed: yes, over two lines.",
	}, "\n")
	if got := specSection(t, b, "Your calls"); got != calls {
		t.Errorf("Your calls =\n%s\nwant\n%s", got, calls)
	}
	if !strings.Contains(brief.YourCallsIntro, "Merging accepts each proposed answer.") {
		t.Errorf("intro = %q", brief.YourCallsIntro)
	}

	// What it costs later: invariants by name, human review, human sign-offs, steps by size.
	costs := strings.Join([]string{
		"- INV-2: Git is enough",
		"- `human_review: required`",
		"- AC-2: It reads well",
		"  Sign off by: read the output",
		"",
		"2 steps: 1 m, 1 s.",
	}, "\n")
	if got := specSection(t, b, "What it costs later"); got != costs {
		t.Errorf("What it costs later =\n%s\nwant\n%s", got, costs)
	}

	// No open questions: ready to accept.
	r.Spec(draft("", "", nil))
	if got := specSection(t, buildSpec(t, r, brief.SpecOptions{Offline: true}), "Your calls"); got != "None. Ready to accept." {
		t.Errorf("Your calls with no questions = %q", got)
	}

	// An amendment whose Decisions gained an Owner chose line: Answered lists it, and only it.
	r, _ = specRepo(t, draft(questions, "- 2026-01-01: Chose A because B.\n", nil))
	r.Spec(draft(questions, "- 2026-01-01: Chose A because B.\n"+
		"- 2026-11-02: Should it be slow? Owner chose fast over the proposed slow.\n"+
		"- 2026-11-02: Chose C because D.\n", nil))
	b = buildSpec(t, r, brief.SpecOptions{Offline: true})
	want = []string{"Decision", "Your calls", "Answered", "Plan", "Collisions", "What it costs later", "Changed criteria"}
	if got := specHeadings(b); !reflect.DeepEqual(got, want) {
		t.Errorf("amendment headings = %v\nwant                 %v", got, want)
	}
	if got := specSection(t, b, "Answered"); got != "- 2026-11-02: Should it be slow? Owner chose fast over the proposed slow." {
		t.Errorf("Answered = %q", got)
	}
}

func TestSpecBriefLineLinks(t *testing.T) {
	r, _ := specRepo(t)
	r.Spec(draft(questions, "", nil))
	sum := sha256.Sum256([]byte(draftFile))
	anchor := "#diff-" + hex.EncodeToString(sum[:]) + "R"
	l1 := itoa(lineOf(t, r, draftFile, "- Is it"))
	l2 := itoa(lineOf(t, r, draftFile, "- Should it"))

	for _, url := range []string{
		"https://github.com/o/r",
		"https://github.com/o/r.git",
		"git@github.com:o/r.git",
		"git@github.com:o/r",
		"ssh://git@github.com/o/r.git",
		"ssh://git@github.com/o/r",
	} {
		gittest.Must(t, r.Root, "remote", "set-url", "origin", url)
		calls := specSection(t, buildSpec(t, r, brief.SpecOptions{PR: 42, Offline: true}), "Your calls")
		for _, want := range []string{
			"**Q1** [Is it called thing?](https://github.com/o/r/pull/42/files" + anchor + l1 + ")",
			"**Q2** [Should it be fast?](https://github.com/o/r/pull/42/files" + anchor + l2 + ")",
		} {
			if !strings.Contains(calls, want) {
				t.Errorf("%s: Your calls has no %s:\n%s", url, want, calls)
			}
		}
	}

	// Without --pr, or with a remote that isn't GitHub, the line follows the question as text.
	for _, c := range []struct {
		url string
		pr  int
	}{{"https://github.com/o/r.git", 0}, {"https://gitlab.com/o/r.git", 42}, {"/srv/git/r.git", 42}} {
		gittest.Must(t, r.Root, "remote", "set-url", "origin", c.url)
		calls := specSection(t, buildSpec(t, r, brief.SpecOptions{PR: c.pr, Offline: true}), "Your calls")
		if want := "**Q1** Is it called thing? (" + draftFile + ":" + l1 + ")"; !strings.Contains(calls, want) || strings.Contains(calls, "](") {
			t.Errorf("%s with --pr %d: want %q and no link:\n%s", c.url, c.pr, want, calls)
		}
	}

	// Numbers follow the file's order, whatever the order they're listed in.
	qs := brief.OpenQuestions(r.Read(draftFile))
	if len(qs) != 3 || qs[0].N != 1 || qs[1].N != 2 || qs[2].N != 3 || qs[1].Proposed != "" || qs[2].Proposed != "yes, over two lines." {
		t.Errorf("questions = %+v", qs)
	}
	if got := brief.GitHubRepo("https://x-access-token@github.com/o/r.git"); got != "o/r" {
		t.Errorf("GitHubRepo with a user = %q", got)
	}
}

func TestSpecBriefPlan(t *testing.T) {
	// Spec 8 is accepted and its step 1 is open. Spec 7 is a draft: step 1 is merged, 2 waits on
	// nothing open, 3 waits on 2 and FUR-0008.1, 4 waits on 3, and 5 waits on 2.
	other := spectest.Spec{Num: 8, Steps: []spectest.Step{{Title: "Other"}}}
	s := draft("", "", nil)
	s.Status = "draft"
	s.Steps = []spectest.Step{
		{Title: "Done already", Merged: true, Covers: []string{"AC-1", "AC-2"}},
		{Title: `Say "hi"`, After: []string{"1"}, Size: "m", Covers: []string{}},
		{Title: "Join", After: []string{"2", "FUR-0008.1"}, Covers: []string{}},
		{Title: "Last", After: []string{"3"}, Covers: []string{}},
		{Title: "Side", After: []string{"2"}, Covers: []string{}},
	}
	r, _ := specRepo(t, other)
	r.Spec(s)
	b := buildSpec(t, r, brief.SpecOptions{Offline: true})
	want := strings.Join([]string{
		"```mermaid",
		"flowchart TD",
		`  FUR_0008_1["FUR-0008.1"]`,
		`  subgraph W1["Wave 1"]`,
		`    S2["2. Say #quot;hi#quot; (m)"]`,
		"  end",
		`  subgraph W2["Wave 2"]`,
		`    S3["3. Join (s)"]`,
		`    S5["5. Side (s)"]`,
		"  end",
		`  subgraph W3["Wave 3"]`,
		`    S4["4. Last (s)"]`,
		"  end",
		"  S2 --> S3",
		"  FUR_0008_1 --> S3",
		"  S3 --> S4",
		"  S2 --> S5",
		"```",
		"",
		"Legend: each box is a step, with its size (s or m). An arrow points from a step to one that waits for it. Steps in the same wave can run at the same time unless they share files.",
	}, "\n")
	if got := specSection(t, b, "Plan"); got != want {
		t.Errorf("Plan =\n%s\nwant\n%s", got, want)
	}
	if b.Plan[1].Steps[0].Ref != "FUR-0007.3" || !reflect.DeepEqual(b.Plan[1].Steps[0].After, []string{"FUR-0007.2", "FUR-0008.1"}) {
		t.Errorf("plan JSON = %+v", b.Plan)
	}
	// The draft is still a draft on disk, and the same specs give the same bytes.
	if r.Load().Spec("FUR-0007").Status != "draft" {
		t.Error("the spec's status changed")
	}
	for i := 0; i < 3; i++ {
		if again := buildSpec(t, r, brief.SpecOptions{Offline: true}).Markdown(); again != b.Markdown() {
			t.Fatalf("output changed between runs:\n%s\n---\n%s", b.Markdown(), again)
		}
	}
}

func TestSpecBriefCollisions(t *testing.T) {
	// Spec 8's step 1 shares a folder with spec 7's step 1 and is claimed; spec 9's step 2 shares
	// a file with step 2. Spec 10 is a draft and spec 11's overlapping step is merged: neither counts.
	specs := []spectest.Spec{
		{Num: 8, Steps: []spectest.Step{{Touches: []string{"internal/thing/a.go"}}}},
		{Num: 9, Steps: []spectest.Step{{}, {Touches: []string{"cmd/furrow/thing.go"}}}},
		{Num: 10, Status: "draft", Steps: []spectest.Step{{Touches: []string{"internal/thing/**"}}}},
		{Num: 11, Steps: []spectest.Step{{Touches: []string{"internal/thing/**"}, Merged: true}, {}}},
	}
	r, _ := specRepo(t, specs...)
	gittest.Must(t, r.Root, "push", "-q", "origin", "main:refs/heads/step/0008-1")
	r.Spec(draft("", "", nil))

	b := buildSpec(t, r, brief.SpecOptions{})
	want := strings.Join([]string{
		"- FUR-0007.1 and FUR-0008.1 (claimed) both touch internal/thing/** and internal/thing/a.go.",
		"- FUR-0007.2 and FUR-0009.2 both touch cmd/furrow/thing.go.",
	}, "\n")
	if got := specSection(t, b, "Collisions"); got != want {
		t.Errorf("Collisions =\n%s\nwant\n%s", got, want)
	}
	if len(b.Collisions) != 2 || !b.Collisions[0].Claimed || b.Collisions[1].Claimed {
		t.Errorf("collisions JSON = %+v", b.Collisions)
	}

	// Offline: claims aren't asked for, and the section says so once.
	b = buildSpec(t, r, brief.SpecOptions{Offline: true})
	got := specSection(t, b, "Collisions")
	if !strings.HasPrefix(got, "(claims not checked)\n") || strings.Count(got, "claims not checked") != 1 || strings.Contains(got, "(claimed)") {
		t.Errorf("offline Collisions =\n%s", got)
	}

	// No overlap: None.
	r, _ = specRepo(t, spectest.Spec{Num: 8})
	r.Spec(draft("", "", nil))
	if got := specSection(t, buildSpec(t, r, brief.SpecOptions{}), "Collisions"); got != "None." {
		t.Errorf("Collisions with no overlap = %q", got)
	}
}

func TestSpecBriefChangedCriteria(t *testing.T) {
	crit := []string{
		"- [ ] AC-1: It works (test: TestWorks)",
		"- [ ] AC-2: It is gone (test: TestGone)",
		"- [ ] AC-3: It stays (test: TestStays)",
	}
	r, _ := specRepo(t, draft("", "", crit))
	// AC-1 reworded, AC-2 removed, AC-3 only ticked, AC-4 added.
	r.Spec(draft("", "", []string{
		"- [ ] AC-1: It works well (test: TestWorks)",
		"- [x] AC-3: It stays (test: TestStays)",
		"- [ ] AC-4: It is new (check: true)",
	}))
	b := buildSpec(t, r, brief.SpecOptions{Offline: true})
	want := strings.Join([]string{
		"Only a person changes a criterion (INV-4).",
		"",
		"- AC-1 reworded.",
		"  Was: It works (test: TestWorks)",
		"  Now: It works well (test: TestWorks)",
		"- AC-4 added: It is new (check: true)",
		"- AC-2 removed. It was: It is gone (test: TestGone)",
	}, "\n")
	if got := specSection(t, b, "Changed criteria"); got != want {
		t.Errorf("Changed criteria =\n%s\nwant\n%s", got, want)
	}
	wantJSON := []brief.CriterionChange{
		{ID: "AC-1", Change: "reworded", Old: "It works (test: TestWorks)", New: "It works well (test: TestWorks)"},
		{ID: "AC-4", Change: "added", New: "It is new (check: true)"},
		{ID: "AC-2", Change: "removed", Old: "It is gone (test: TestGone)"},
	}
	if !reflect.DeepEqual(b.ChangedCriteria, wantJSON) {
		t.Errorf("changed_criteria = %+v", b.ChangedCriteria)
	}

	// A spec file new on the branch has no Changed criteria section, and an empty list.
	r, _ = specRepo(t)
	r.Spec(draft("", "", crit))
	b = buildSpec(t, r, brief.SpecOptions{Offline: true})
	for _, h := range specHeadings(b) {
		if h == "Changed criteria" {
			t.Error("a new spec file has a Changed criteria section")
		}
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"spec", "base", "sections", "plan", "collisions", "changed_criteria"} {
		if v, ok := keys[k]; !ok || string(v) == "null" {
			t.Errorf("--json key %s = %s", k, v)
		}
	}
	if len(keys) != 6 {
		t.Errorf("--json has keys %v", keys)
	}
}

func TestSpecBriefCheckAndFailures(t *testing.T) {
	r, _ := specRepo(t)
	r.Spec(draft(questions, "", nil))
	b := buildSpec(t, r, brief.SpecOptions{Offline: true})

	// The raw brief has a placeholder in Decision.
	rep := brief.CheckSpec(b, b.Markdown())
	if rep.OK || !reflect.DeepEqual(rep.Problems, []string{"`## Decision` still has a placeholder. Write it."}) {
		t.Errorf("raw brief: %+v", rep)
	}

	// Filled in, it passes.
	decision := "**Commits us to:** a thing.\n**Instead of:** the other thing."
	filledIn := strings.Replace(b.Markdown(), specSection(t, b, "Decision"), decision, 1)
	if rep := brief.CheckSpec(b, filledIn); !rep.OK || len(rep.Problems) != 0 || rep.ExitCode() != 0 {
		t.Errorf("filled-in brief: %+v", rep)
	}

	// A long Decision (two short lines and prose, 81 words in all) and a missing Plan, in section order.
	long := strings.Replace(filledIn, decision, decision+"\n\n"+strings.Repeat("word ", 71), 1)
	long = strings.Replace(long, "## Plan\n", "## Plans\n", 1)
	rep = brief.CheckSpec(b, long)
	want := []string{
		"`## Decision` is 81 words. Keep it to 80.",
		"No `## Plan` section. Run `furrow brief --spec FUR-0007` and paste it.",
	}
	if rep.OK || !reflect.DeepEqual(rep.Problems, want) || rep.ExitCode() != 1 {
		t.Errorf("long Decision, no Plan: %+v", rep)
	}

	// --spec with --step is a usage error (exit 2).
	_, err := brief.BuildSpec(gittest.Git(r.Root), r.Load(), brief.SpecOptions{Spec: "FUR-0007", Step: "FUR-0007.1", Offline: true})
	if !errors.Is(err, brief.ErrSpecAndStep) {
		t.Errorf("--spec with --step: %v", err)
	}

	// An archived spec and an unknown one exit 1 with their message.
	r.Spec(spectest.Spec{Num: 5, Archived: true})
	for id, msg := range map[string]string{
		"FUR-0005": "FUR-0005 is archived. Change it with a new spec.",
		"FUR-0099": "no spec FUR-0099",
	} {
		_, err := brief.BuildSpec(gittest.Git(r.Root), r.Load(), brief.SpecOptions{Spec: id, Offline: true})
		if err == nil || err.Error() != msg || errors.Is(err, brief.ErrSpecAndStep) {
			t.Errorf("%s: %v, want %q", id, err, msg)
		}
	}
}

func TestSpecBriefDecisionLines(t *testing.T) {
	r, _ := specRepo(t)
	r.Spec(draft(questions, "", nil))
	b := buildSpec(t, r, brief.SpecOptions{Offline: true})

	// The Decision is two placeholder lines, each giving its 25-word limit.
	want := "**Commits us to:** <!-- furrow: what accepting this means we build, at most 25 words -->\n" +
		"**Instead of:** <!-- furrow: the main alternative it rejects, and why, at most 25 words -->"
	decision := specSection(t, b, "Decision")
	if decision != want {
		t.Errorf("Decision = %q\nwant       %q", decision, want)
	}

	with := func(body string) []string {
		t.Helper()
		return brief.CheckSpec(b, strings.Replace(b.Markdown(), decision, body, 1)).Problems
	}
	commits := "**Commits us to:** a two-line Decision a person reads in seconds."
	instead := "**Instead of:** one free paragraph, because it ran long and buried the choice."

	// Both lines filled in: no problems.
	if got := with(commits + "\n" + instead); len(got) != 0 {
		t.Errorf("filled in: %q", got)
	}

	// A missing line, either one, or both.
	missing := "`## Decision` needs a `**Commits us to:**` and an `**Instead of:**` line."
	for name, body := range map[string]string{
		"no Commits us to": instead,
		"no Instead of":    commits,
		"neither":          "We build a thing instead of another thing.",
	} {
		if got := with(body); !reflect.DeepEqual(got, []string{missing}) {
			t.Errorf("%s: %q", name, got)
		}
	}

	// A line over 25 words; exactly 25 is fine, and a comment doesn't count.
	long := "**Instead of:** " + strings.Repeat("word ", 31)
	if got := with(commits + "\n" + long); !reflect.DeepEqual(got, []string{"`**Instead of:**` is 31 words. Keep it to 25."}) {
		t.Errorf("long Instead of: %q", got)
	}
	both := "**Commits us to:** " + strings.Repeat("word ", 26) + "\n" + long
	if got := with(both); !reflect.DeepEqual(got, []string{
		"`**Commits us to:**` is 26 words. Keep it to 25.",
		"`**Instead of:**` is 31 words. Keep it to 25.",
	}) {
		t.Errorf("both long: %q", got)
	}
	edge := "**Commits us to:** " + strings.Repeat("word ", 25) + "<!-- a note, not counted -->\n" + instead
	if got := with(edge); len(got) != 0 {
		t.Errorf("25 words and a comment: %q", got)
	}

	// The checks are deterministic.
	if a, c := with(both), with(both); !reflect.DeepEqual(a, c) {
		t.Errorf("not deterministic: %q then %q", a, c)
	}
}

func TestSpecBriefHasNoProof(t *testing.T) {
	r, _ := specRepo(t)
	r.Spec(draft(questions, "", nil))
	md := buildSpec(t, r, brief.SpecOptions{Offline: true}).Markdown()
	if strings.Contains(md, "<details><summary>Proof") || strings.Contains(md, "Proof:") {
		t.Errorf("the spec brief has a Proof block:\n%s", md)
	}
	if !strings.HasSuffix(md, "2 steps: 1 m, 1 s.\n") {
		t.Errorf("the spec brief should end with its last section:\n%s", md)
	}
}

func TestSpecBriefAmendmentCalls(t *testing.T) {
	// The base has spec 7 with three questions. The branch rewraps one, and adds one.
	r, _ := specRepo(t, draft(questions, "", nil))
	amended := "- Is it called   thing? Proposed: no, it changed.\n" +
		"- Should it\n  be fast?\n" +
		"- Does it wrap? Proposed: yes, over\n  two lines.\n" +
		"- Is it new? Proposed: yes.\n"
	r.Spec(draft(amended, "", nil))
	b := buildSpec(t, r, brief.SpecOptions{Offline: true})
	calls := strings.Join([]string{
		brief.YourCallsIntro,
		"",
		"**Q1** Is it new? (" + draftFile + ":" + itoa(lineOf(t, r, draftFile, "- Is it new")) + ")",
		"Proposed: yes.",
	}, "\n")
	if got := specSection(t, b, "Your calls"); got != calls {
		t.Errorf("Your calls =\n%s\nwant\n%s", got, calls)
	}

	// With a PR on GitHub, the new question links to its line.
	gittest.Must(t, r.Root, "remote", "set-url", "origin", "https://github.com/o/r.git")
	sum := sha256.Sum256([]byte(draftFile))
	link := "**Q1** [Is it new?](https://github.com/o/r/pull/42/files#diff-" + hex.EncodeToString(sum[:]) + "R" +
		itoa(lineOf(t, r, draftFile, "- Is it new")) + ")"
	if got := specSection(t, buildSpec(t, r, brief.SpecOptions{PR: 42, Offline: true}), "Your calls"); !strings.Contains(got, link) || strings.Contains(got, "**Q2**") {
		t.Errorf("Your calls with --pr should hold only %s:\n%s", link, got)
	}

	// An amendment that adds no question is ready to accept, and --check still wants the section.
	r.Spec(draft(questions, "- 2026-11-03: Chose A because B.\n", nil))
	b = buildSpec(t, r, brief.SpecOptions{Offline: true})
	if got := specSection(t, b, "Your calls"); got != "None. Ready to accept." {
		t.Errorf("Your calls with no new question = %q", got)
	}
	md := b.Markdown()
	if rep := brief.CheckSpec(b, strings.Replace(md, "## Your calls\n\nNone. Ready to accept.\n\n", "", 1)); rep.OK {
		t.Error("--check --spec passed a brief with no ## Your calls")
	}

	// A spec file new on the branch asks every question, as before.
	r, _ = specRepo(t)
	r.Spec(draft(questions, "", nil))
	got := specSection(t, buildSpec(t, r, brief.SpecOptions{Offline: true}), "Your calls")
	for _, q := range []string{"**Q1** Is it called thing?", "**Q2** Should it be fast?", "**Q3** Does it wrap?"} {
		if !strings.Contains(got, q) {
			t.Errorf("a new spec's Your calls has no %s:\n%s", q, got)
		}
	}
}
