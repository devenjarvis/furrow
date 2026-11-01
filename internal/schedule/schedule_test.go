package schedule_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

type S = spectest.Spec
type St = spectest.Step

func refsOf(items []*schedule.Item) string {
	var out []string
	for _, it := range items {
		out = append(out, it.Ref())
	}
	return strings.Join(out, " ")
}

func TestStates(t *testing.T) {
	r := spectest.New(t)
	r.Spec(S{Num: 1, Steps: []St{{Merged: true}, {}}})            // FUR-0001.2 ready
	r.Spec(S{Num: 2, Steps: []St{{After: []string{"FUR-0001"}}}}) // waits on all of FUR-0001
	r.Spec(S{Num: 3, Steps: []St{{After: []string{"FUR-0001.1"}}, {Blocked: "needs a key"}}})
	r.Spec(S{Num: 4, Status: "draft"})
	r.Spec(S{Num: 5, Archived: true})
	r.Spec(S{Num: 6, Steps: []St{{After: []string{"FUR-0005"}}}})
	p := schedule.Build(r.Load(), nil, false, 0)

	if got := refsOf(p.Ready); got != "FUR-0001.2 FUR-0003.1 FUR-0006.1" {
		t.Errorf("ready = %q", got)
	}
	if got := refsOf(p.Waiting); got != "FUR-0002.1" {
		t.Errorf("waiting = %q", got)
	}
	if got := refsOf(p.Blocked); got != "FUR-0003.2" {
		t.Errorf("blocked = %q", got)
	}
	if w := p.Waiting[0].WaitsOn; len(w) != 1 || w[0].Ref().String() != "FUR-0001.2" {
		t.Errorf("FUR-0002.1 should wait only on the unmerged FUR-0001.2, got %v", w)
	}
}

func TestRanking(t *testing.T) {
	r := spectest.New(t)
	// FUR-0001.1 unblocks two steps; FUR-0002.1 unblocks a chain of two; FUR-0003.1 nothing (p0);
	// FUR-0004.1 nothing (p1). Ranking: most dependents, then chain, then priority, then number.
	r.Spec(S{Num: 1})
	r.Spec(S{Num: 2})
	r.Spec(S{Num: 3, Priority: "p0"})
	r.Spec(S{Num: 4})
	r.Spec(S{Num: 5, Steps: []St{{After: []string{"FUR-0001.1"}}}})
	r.Spec(S{Num: 6, Steps: []St{{After: []string{"FUR-0001.1"}}}})
	r.Spec(S{Num: 7, Steps: []St{{After: []string{"FUR-0002.1"}}, {After: []string{"1"}}}})
	r.Spec(S{Num: 8})
	r.Spec(S{Num: 9, Steps: []St{{After: []string{"FUR-0008.1"}}, {After: []string{"FUR-0008.1"}}}})
	p := schedule.Build(r.Load(), nil, false, 0)
	// 0001 and 0002 and 0008 all have 2 dependents; 0002 has the longer chain (2); 0001 and 0008 tie
	// on chain 1 and priority, so number decides.
	if got := refsOf(p.Ready); got != "FUR-0002.1 FUR-0001.1 FUR-0008.1 FUR-0003.1 FUR-0004.1" {
		t.Errorf("ranked = %q", got)
	}
}

func TestPickTogether(t *testing.T) {
	r := spectest.New(t)
	r.Spec(S{Num: 1, Steps: []St{{Touches: []string{"internal/**"}}}})
	r.Spec(S{Num: 2, Steps: []St{{Touches: []string{"internal/spec/x.go"}}}})
	r.Spec(S{Num: 3, Steps: []St{{Touches: []string{"cli/next.go"}}}})
	r.Spec(S{Num: 4, Steps: []St{{Touches: []string{"cmd/furrow/*.go"}}}})
	r.Spec(S{Num: 5, Steps: []St{{Touches: []string{"docs/a.md"}}}})
	repo := r.Load()

	p := schedule.Build(repo, nil, false, 0)
	if got := refsOf(p.Together); got != "FUR-0001.1 FUR-0003.1 FUR-0005.1" {
		t.Errorf("together = %q", got)
	}
	var held []string
	for _, h := range p.Held {
		held = append(held, h.Item.Ref()+"|"+h.ClashWith.Ref()+"|"+h.Paths)
	}
	want := "FUR-0002.1|FUR-0001.1|internal/spec/x.go and internal/** FUR-0004.1|FUR-0003.1|cmd/furrow/*.go and cli/next.go"
	if strings.Join(held, " ") != want {
		t.Errorf("held = %v", held)
	}
	for i, a := range p.Together {
		for _, b := range p.Together[i+1:] {
			if pair, ok := overlap(repo, a, b); ok {
				t.Errorf("%s and %s overlap on %s", a.Ref(), b.Ref(), pair)
			}
		}
	}

	p = schedule.Build(repo, nil, false, 2)
	if got := refsOf(p.Together); got != "FUR-0001.1 FUR-0003.1" {
		t.Errorf("--agents 2 = %q", got)
	}
}

func overlap(repo *spec.Repo, a, b *schedule.Item) (string, bool) {
	return paths.Overlap(a.Step.Touches, b.Step.Touches, repo.Config.Aliases)
}

func fixture(t *testing.T) *spectest.Repo {
	r := spectest.New(t)
	r.Spec(S{Num: 1, Title: "Base", Steps: []St{{Title: "Territory check", Touches: []string{"internal/territory/**", "cli/territory.go"}, Size: "m"}}})
	r.Spec(S{Num: 2, Priority: "p0", Steps: []St{{Title: "Lifecycle", Touches: []string{"internal/lifecycle/**"}}}})
	r.Spec(S{Num: 3, Steps: []St{{Title: "Scaffold", Touches: []string{"cmd/furrow/territory.go"}}}})
	r.Spec(S{Num: 4, Steps: []St{{Title: "Plugin", After: []string{"FUR-0001.1", "FUR-0002.1"}}}})
	r.Spec(S{Num: 5, Steps: []St{{Title: "Release", Blocked: "needs the GitHub repo"}}})
	return r
}

func TestNextText(t *testing.T) {
	r := fixture(t)
	var b bytes.Buffer
	schedule.Build(r.Load(), nil, false, 0).Text(&b)
	want := `Ready to start:
  FUR-0002.1  p0  s  Lifecycle
              specs/changes/0002-spec-2.md · touches internal/lifecycle/**
  FUR-0001.1  p1  m  Territory check
              specs/changes/0001-spec-1.md · touches internal/territory/**, cli/territory.go
  FUR-0003.1  p1  s  Scaffold
              specs/changes/0003-spec-3.md · touches cmd/furrow/territory.go
Can run together: FUR-0002.1, FUR-0001.1
  not FUR-0003.1: shares cmd/furrow/territory.go and cli/territory.go with FUR-0001.1
Waiting:
  FUR-0004.1  after FUR-0001.1, FUR-0002.1
Blocked:
  FUR-0005.1  needs the GitHub repo
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}

	b.Reset()
	schedule.Build(r.Load(), nil, false, 1).Text(&b)
	if !strings.HasPrefix(b.String(), "Assignments:\n  FUR-0002.1  p0  s  Lifecycle\n              specs/changes/0002-spec-2.md · touches internal/lifecycle/**\nWaiting:") {
		t.Errorf("agents mode:\n%s", b.String())
	}

	empty := spectest.New(t)
	empty.Spec(S{Num: 1, Status: "draft"})
	b.Reset()
	schedule.Build(empty.Load(), nil, false, 0).Text(&b)
	if b.String() != "Nothing is ready to start.\n" {
		t.Errorf("nothing ready = %q", b.String())
	}
}

func TestNextJSON(t *testing.T) {
	r := fixture(t)
	one, err := json.Marshal(schedule.Build(r.Load(), nil, false, 0))
	if err != nil {
		t.Fatal(err)
	}
	two, _ := json.Marshal(schedule.Build(r.Load(), nil, false, 0))
	if !bytes.Equal(one, two) {
		t.Error("two runs differ")
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(one, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"claims_checked", "ready", "together", "held", "claimed", "waiting", "blocked"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	var steps []map[string]any
	if err := json.Unmarshal(got["waiting"], &steps); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ref", "spec", "title", "priority", "size", "file", "touches", "after", "branch"} {
		if _, ok := steps[0][k]; !ok {
			t.Errorf("step missing key %q", k)
		}
	}
	if string(got["held"]) != `[{"step":"FUR-0003.1","clash_with":"FUR-0001.1","paths":"cmd/furrow/territory.go and cli/territory.go"}]` {
		t.Errorf("held = %s", got["held"])
	}
}

func TestGraphWaves(t *testing.T) {
	r := spectest.New(t)
	r.Spec(S{Num: 1, Steps: []St{{Merged: true}, {}}})
	r.Spec(S{Num: 2, Steps: []St{{After: []string{"FUR-0001.2"}}, {After: []string{"1"}}, {After: []string{"FUR-0001.1"}}}})
	r.Spec(S{Num: 3, Steps: []St{{After: []string{"FUR-0002.2", "FUR-0001.2"}}}})
	waves := schedule.Waves(r.Load(), nil)
	var got []string
	for _, w := range waves {
		got = append(got, refsOf(w.Items))
	}
	if strings.Join(got, " | ") != "FUR-0001.2 FUR-0002.3 | FUR-0002.1 | FUR-0002.2 | FUR-0003.1" {
		t.Errorf("waves = %v", got)
	}
	var b bytes.Buffer
	schedule.GraphText(&b, waves)
	if !strings.Contains(b.String(), "Wave 4\n  FUR-0003.1  [waiting]  Step 1  after FUR-0001.2, FUR-0002.2\n") {
		t.Errorf("graph text:\n%s", b.String())
	}
}
