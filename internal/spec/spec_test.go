package spec_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

func TestLoadRepo(t *testing.T) {
	r := spectest.New(t)
	r.Spec(spectest.Spec{Num: 1, Archived: true})
	r.Spec(spectest.Spec{Num: 2, Steps: []spectest.Step{{Merged: true}, {After: []string{"1"}}}})
	r.Spec(spectest.Spec{Num: 3, Steps: []spectest.Step{{After: []string{"FUR-0001", "FUR-0002.2"}}},
		Criteria:  []string{"- [ ] AC-1: Runs `x (y)` well (check: test \"$(pwd)\" = /)", "- [x] AC-2: Looks right (human: look at it)"},
		Decisions: "- 2026-11-01: AC-2 signed off by Deven."})
	repo := r.Load()

	if len(repo.Specs) != 3 || len(repo.Areas) != 1 || len(repo.Invariants) != 2 {
		t.Fatalf("loaded %d specs, %d areas, %d invariants", len(repo.Specs), len(repo.Areas), len(repo.Invariants))
	}
	if repo.Invariants[1] != (spec.Invariant{ID: "INV-2", Text: "Git is enough"}) {
		t.Errorf("invariant = %+v", repo.Invariants[1])
	}
	s1, s2, s3 := repo.Spec("FUR-0001"), repo.Spec("FUR-0002"), repo.Spec("FUR-0003")
	if !s1.Archived || !repo.RefMerged(spec.Ref{Spec: "FUR-0001"}) {
		t.Error("an archived spec counts as merged")
	}
	refs, errs := repo.AfterRefs(s2.Steps[1])
	if len(errs) > 0 || refs[0] != (spec.Ref{Spec: "FUR-0002", Step: 1}) || !repo.RefMerged(refs[0]) {
		t.Errorf("bare step ref: %v %v", refs, errs)
	}
	refs, _ = repo.AfterRefs(s3.Steps[0])
	if refs[0] != (spec.Ref{Spec: "FUR-0001"}) || refs[1] != (spec.Ref{Spec: "FUR-0002", Step: 2}) || repo.RefMerged(refs[1]) {
		t.Errorf("full refs: %v", refs)
	}
	if repo.RefMerged(spec.Ref{Spec: "FUR-0002"}) {
		t.Error("a spec with an unmerged step isn't merged as a whole")
	}
	c := s3.Criteria[0]
	if c.Proof != "check" || c.Detail != `test "$(pwd)" = /` || c.Ticked {
		t.Errorf("criterion = %+v", c)
	}
	if !s3.Criteria[1].Ticked || s3.Criteria[1].Proof != "human" {
		t.Errorf("criterion 2 = %+v", s3.Criteria[1])
	}
	if got := spec.Lint(repo); len(got) != 0 {
		t.Errorf("clean repo has problems: %v", got)
	}
}

func lintMessages(repo *spec.Repo) []string {
	var out []string
	for _, p := range spec.Lint(repo) {
		out = append(out, p.String())
	}
	return out
}

func TestLintRules(t *testing.T) {
	cases := []struct {
		name  string
		setup func(r *spectest.Repo)
		want  string // a substring of exactly one problem
	}{
		{"1 front matter", func(r *spectest.Repo) { r.Write("specs/changes/0001-x.md", "# no front matter\n") }, "missing front matter"},
		{"1 id", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, ID: "FUR-0009"}) }, "id should be FUR-0001"},
		{"1 duplicate", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1})
			r.Spec(spectest.Spec{Num: 1, Slug: "other", Archived: true})
		}, "duplicate id FUR-0001"},
		{"2 status", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Status: "ready"}) }, "status must be draft, accepted or paused"},
		{"2 archived status", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Archived: true, Status: "accepted"}) }, "archived specs must be status done"},
		{"2 priority", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Priority: "high"}) }, "priority must be"},
		{"2 review", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, HumanReview: "maybe"}) }, "human_review must be"},
		{"3 area", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Area: "nowhere"}) }, `area "nowhere" has no`},
		{"4 sections", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Body: "## Why\n\n## What\n\n## Acceptance criteria\n\n- [ ] AC-1: x (test: T)\n"})
		}, `missing section "## Out of scope"`},
		{"5 no criteria", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Criteria: []string{}}) }, "no acceptance criteria"},
		{"5 numbering", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Criteria: []string{"- [ ] AC-2: x (test: T)"}})
		}, "AC-2 should be numbered AC-1"},
		{"5 proof", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Criteria: []string{"- [ ] AC-1: it's fast"}}) }, "doesn't say how it's proven"},
		{"6 no steps", func(r *spectest.Repo) {
			r.Write("specs/changes/0001-x.md", noSteps())
		}, "needs at least one step"},
		{"6 touches", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Touches: []string{}}}})
		}, "needs touches"},
		{"6 covers unknown", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Covers: []string{"AC-1", "AC-7"}}}})
		}, "covers AC-7, which isn't a criterion"},
		{"6 uncovered", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Covers: []string{}}}}) }, "AC-1 isn't covered"},
		{"6 size l", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Size: "l"}}}) }, "is size l"},
		{"6 TBD", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Criteria: []string{"- [ ] AC-1: TBD (test: T)", "- [ ] AC-2: Mentions `TBD` in code, which is fine (test: T)"}})
		}, "can't contain TBD"},
		{"7 unknown after", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{After: []string{"FUR-0042.1"}}}})
		}, "after FUR-0042.1, which doesn't exist"},
		{"7 self", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{After: []string{"1"}}}}) }, "can't be after itself"},
		{"7 cycle", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{After: []string{"FUR-0002.1"}}}})
			r.Spec(spectest.Spec{Num: 2, Steps: []spectest.Step{{After: []string{"FUR-0001.1"}}}})
		}, "steps wait on each other: FUR-0001.1 -> FUR-0002.1 -> FUR-0001.1"},
		{"7 merged early", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1})
			r.Spec(spectest.Spec{Num: 2, Steps: []spectest.Step{{After: []string{"FUR-0001.1"}, Merged: true}}})
		}, "step 1 is merged but FUR-0001.1"},
		{"8 folder", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Touches: []string{"nope/x/**"}}}})
		}, `the folder nope doesn't exist`},
		{"9 unknown invariant", func(r *spectest.Repo) { r.Spec(spectest.Spec{Num: 1, Invariants: []string{"INV-9"}}) }, "invariant INV-9 isn't defined"},
		{"9 review", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Invariants: []string{"INV-1"}, HumanReview: "optional"})
		}, "human_review must be required"},
		{"10 signoff", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Criteria: []string{"- [x] AC-1: Looks right (human: look)"}})
		}, "AC-1 is a human check and is ticked"},
		{"11 done", func(r *spectest.Repo) {
			r.Spec(spectest.Spec{Num: 1, Archived: true, Criteria: []string{"- [ ] AC-1: x (test: T)"}})
		}, "status is done but"},
		{"12 area name", func(r *spectest.Repo) {
			r.Write("specs/areas/core.md", strings.Replace(spectest.AreaText("core"), "area: core", "area: other", 1))
		}, `area should be "core"`},
		{"12 area sections", func(r *spectest.Repo) {
			r.Write("specs/areas/core.md", strings.Replace(spectest.AreaText("core"), "## History", "## Past", 1))
		}, `missing section "## History"`},
		{"12 duplicate invariant", func(r *spectest.Repo) {
			r.Write("docs/product.md", spectest.Product+"- **INV-2: Again.** Text.\n")
		}, "duplicate invariant INV-2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := spectest.New(t)
			c.setup(r)
			msgs := lintMessages(r.Load())
			hits := 0
			for _, m := range msgs {
				if strings.Contains(m, c.want) {
					hits++
				}
			}
			if hits != 1 {
				t.Errorf("want exactly one problem containing %q, got:\n%s", c.want, strings.Join(msgs, "\n"))
			}
		})
	}

	t.Run("clean", func(t *testing.T) {
		r := spectest.New(t)
		r.Spec(spectest.Spec{Num: 1})
		repo := r.Load()
		if msgs := lintMessages(repo); len(msgs) != 0 {
			t.Errorf("clean repo: %v", msgs)
		}
		line, ok := spec.Summary(repo, nil)
		if !ok || line != "1 specs OK, 1 open steps." {
			t.Errorf("summary = %q", line)
		}
	})

	t.Run("archive warning", func(t *testing.T) {
		r := spectest.New(t)
		r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Merged: true}}, Criteria: []string{"- [x] AC-1: x (test: T)"}})
		problems := spec.Lint(r.Load())
		if len(problems) != 1 || !problems[0].Warning || !strings.Contains(problems[0].Message, "furrow archive FUR-0001") {
			t.Errorf("problems = %v", problems)
		}
	})
}

func noSteps() string {
	return "---\nid: FUR-0001\ntitle: x\nstatus: accepted\narea: core\npriority: p1\nhuman_review: optional\nsteps: []\n---\n\n## Why\n\n## What\n\n## Acceptance criteria\n\n- [ ] AC-1: x (test: T)\n\n## Out of scope\n"
}

func TestListStatus(t *testing.T) {
	r := spectest.New(t)
	r.Spec(spectest.Spec{Num: 1, Status: "draft"})
	r.Spec(spectest.Spec{Num: 2})
	r.Spec(spectest.Spec{Num: 3, Steps: []spectest.Step{{Merged: true}, {}}})
	r.Spec(spectest.Spec{Num: 4, Steps: []spectest.Step{{Merged: true}}, Criteria: []string{"- [ ] AC-1: x (human: look)"}})
	r.Spec(spectest.Spec{Num: 5, Status: "paused"})
	r.Spec(spectest.Spec{Num: 6, Archived: true})
	entries := spec.List(r.Load())
	var got []string
	for _, e := range entries {
		got = append(got, e.Status)
	}
	if want := "draft accepted building review paused done"; strings.Join(got, " ") != want {
		t.Errorf("statuses = %v, want %s", got, want)
	}
	data, err := json.Marshal(entries[2])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"FUR-0003","title":"Spec 3","status":"building","area":"core","file":"specs/changes/0003-spec-3.md","steps_merged":1,"steps_total":2}`
	if string(data) != want {
		t.Errorf("json = %s", data)
	}
}

// questionsBody is a fixture body with an open-questions section after the given text.
func questionsBody(questions string) string {
	return "# FUR-0001: Spec 1\n\n## Why\n\nBecause.\n\n## What\n\nThis.\n\n## Acceptance criteria\n\n- [ ] AC-1: It works (test: TestWorks)\n\n## Out of scope\n\n- That.\n\n## Open questions\n\n" + questions + "\n## Decisions\n"
}

// lineOf is the 1-based line of the first line in text that starts with prefix.
func lineOf(t *testing.T, text, prefix string) int {
	t.Helper()
	for i, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, prefix) {
			return i + 1
		}
	}
	t.Fatalf("no line starts with %q", prefix)
	return 0
}

func openQuestionProblems(repo *spec.Repo) []string {
	var out []string
	for _, m := range lintMessages(repo) {
		if strings.Contains(m, "Open question") {
			out = append(out, m)
		}
	}
	return out
}

func TestLintOpenQuestionsNeedProposals(t *testing.T) {
	questions := "- Is it fast? Proposed: yes.\n" +
		"- Should it wrap? It might\n  read better. Proposed: no, it\n  stays on one line.\n" +
		"- Is the name right?\n  Nobody has said.\n" +
		"- Does a nested item count? Proposed: no.\n" +
		"  - Nested items don't count on their own.\n" +
		"- What about this one?\n"
	for _, status := range []string{"accepted", "paused"} {
		t.Run(status, func(t *testing.T) {
			r := spectest.New(t)
			path := r.Spec(spectest.Spec{Num: 1, Status: status, Body: questionsBody(questions)})
			text := r.Read(path)
			got := openQuestionProblems(r.Load())
			want := []string{
				fmt.Sprintf(`error: %s: Open question on line %d has no proposed answer. Add "Proposed: …", or answer it in ## Decisions.`, path, lineOf(t, text, "- Is the name right?")),
				fmt.Sprintf(`error: %s: Open question on line %d has no proposed answer. Add "Proposed: …", or answer it in ## Decisions.`, path, lineOf(t, text, "- What about this one?")),
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
	t.Run("draft", func(t *testing.T) {
		r := spectest.New(t)
		r.Spec(spectest.Spec{Num: 1, Status: "draft", Body: questionsBody(questions)})
		if got := lintMessages(r.Load()); len(got) != 0 {
			t.Errorf("a draft spec may have questions without proposals: %v", got)
		}
	})
	t.Run("all proposed", func(t *testing.T) {
		r := spectest.New(t)
		r.Spec(spectest.Spec{Num: 1, Body: questionsBody("- One? Proposed: yes.\n- Two?\n  Proposed: no.\n")})
		if got := lintMessages(r.Load()); len(got) != 0 {
			t.Errorf("every question has a proposal: %v", got)
		}
	})
}

func TestLintOpenQuestionsIgnoreFences(t *testing.T) {
	r := spectest.New(t)
	fencedHeading := "Write it like this:\n\n```md\n## Open questions\n\n- Fenced, with no proposal?\n```\n"
	body := questionsBody("- Real one? Proposed: yes.\n\n````md\n```\n- Inside a four-backtick fence?\n```\n- Still inside?\n````\n\n- Real, unanswered?\n")
	body = strings.Replace(body, "This.\n", "This.\n\n"+fencedHeading, 1)
	path := r.Spec(spectest.Spec{Num: 1, Body: body})
	text := r.Read(path)
	got := openQuestionProblems(r.Load())
	want := fmt.Sprintf(`error: %s: Open question on line %d has no proposed answer. Add "Proposed: …", or answer it in ## Decisions.`, path, lineOf(t, text, "- Real, unanswered?"))
	if len(got) != 1 || got[0] != want {
		t.Errorf("got:\n%s\nwant only:\n%s", strings.Join(got, "\n"), want)
	}

	// A fenced heading alone doesn't open the section.
	r2 := spectest.New(t)
	r2.Spec(spectest.Spec{Num: 1, Body: strings.Replace(questionsBody("- Real? Proposed: yes.\n"), "This.\n", "This.\n\n"+fencedHeading, 1)})
	if got := lintMessages(r2.Load()); len(got) != 0 {
		t.Errorf("fenced questions count: %v", got)
	}
}
