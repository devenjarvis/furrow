package schedule_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

// stackRepo is a fixture repo with stack: n in its config.
func stackRepo(t *testing.T, n int) *spectest.Repo {
	r := spectest.New(t)
	r.Write(".furrow.yml", spectest.Config+fmt.Sprintf("stack: %d\n", n))
	r.Write("internal/.keep", "")
	return r
}

// held is a claim on step 1 of spec num: handed back or not, stacked on another step's branch or not.
func held(num int, back bool, on int) schedule.Claim {
	c := schedule.Claim{Ref: spec.Ref{Spec: fmt.Sprintf("FUR-%04d", num), Step: 1}, Branch: fmt.Sprintf("step/%04d-1", num), HandedBack: back}
	if on > 0 {
		c.StackOn = fmt.Sprintf("step/%04d-1", on)
	}
	return c
}

func find(p *schedule.Plan, ref string) *schedule.Item {
	for _, group := range [][]*schedule.Item{p.Ready, p.Claimed, p.Waiting, p.Blocked} {
		for _, it := range group {
			if it.Ref() == ref {
				return it
			}
		}
	}
	return nil
}

func after(num int, refs ...string) S {
	return S{Num: num, Steps: []St{{After: refs}}}
}

func TestStackDepth(t *testing.T) {
	r := stackRepo(t, 3)
	r.Spec(S{Num: 90})
	r.Spec(after(91, "FUR-0090.1"))
	r.Spec(after(92, "FUR-0091.1"))
	r.Spec(after(93, "FUR-0092.1"))
	r.Spec(after(94, "FUR-0093.1"))
	r.Spec(after(95, "FUR-0090.1"))
	r.Spec(after(96, "FUR-0091.1"))
	r.Spec(after(97, "FUR-0092.1"))
	r.Spec(S{Num: 98})
	r.Spec(after(99, "FUR-0098.1"))
	repo := r.Load()
	// A chain of four handed-back steps: 0090.1 on main, 0091.1 on it, and so on. 0098.1 is
	// claimed but still being built.
	claims := []schedule.Claim{held(90, true, 0), held(91, true, 90), held(92, true, 91), held(93, true, 92), held(98, false, 0)}
	p := schedule.Build(repo, claims, true, 0)

	for ref, want := range map[string]struct {
		parent string
		depth  int
	}{
		"FUR-0095.1": {"step/0090-1", 1},
		"FUR-0096.1": {"step/0091-1", 2},
		"FUR-0097.1": {"step/0092-1", 3},
	} {
		it := find(p, ref)
		if it == nil || it.State != schedule.Ready || !it.Stacked() || it.StackOn.Branch != want.parent || it.Depth() != want.depth {
			t.Errorf("%s: %+v, want ready, stacked on %s at depth %d", ref, it, want.parent, want.depth)
		}
	}
	if got := refsOf(p.Ready); got != "FUR-0095.1 FUR-0096.1 FUR-0097.1" {
		t.Errorf("ready = %q", got)
	}
	wantWhy := map[string]string{
		"FUR-0094.1": "the stack would be 4 deep, over stack: 3",
		"FUR-0099.1": "FUR-0098.1 isn't handed back",
	}
	if got := refsOf(p.Waiting); got != "FUR-0094.1 FUR-0099.1" {
		t.Errorf("waiting = %q", got)
	}
	for _, it := range p.Waiting {
		if it.WhyWaiting != wantWhy[it.Ref()] {
			t.Errorf("%s: why = %q, want %q", it.Ref(), it.WhyWaiting, wantWhy[it.Ref()])
		}
	}
	var b bytes.Buffer
	p.Text(&b)
	for _, line := range []string{
		"  FUR-0095.1  stacked on step/0090-1 (FUR-0090.1)\n",
		"  FUR-0094.1  after FUR-0093.1 (the stack would be 4 deep, over stack: 3)\n",
		"  FUR-0099.1  after FUR-0098.1 (FUR-0098.1 isn't handed back)\n",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("text missing %q:\n%s", line, b.String())
		}
	}

	// Unchecked claims stack nothing, and say nothing about it.
	p = schedule.Build(repo, claims, false, 0)
	if len(p.Ready) != 0 {
		t.Errorf("unchecked claims: ready = %q", refsOf(p.Ready))
	}
	for _, it := range p.Waiting {
		if it.WhyWaiting != "" {
			t.Errorf("unchecked claims: %s says %q", it.Ref(), it.WhyWaiting)
		}
	}

	// furrow graph keeps the waves and says which ready steps are stacked.
	waves := schedule.Waves(repo, claims)
	b.Reset()
	schedule.GraphText(&b, waves)
	if !strings.Contains(b.String(), "  FUR-0095.1  [ready (stacked)]  Step 1  after FUR-0090.1\n") {
		t.Errorf("graph:\n%s", b.String())
	}
	for _, w := range schedule.GraphJSON(waves) {
		for _, s := range w.Steps {
			if s.Ref == "FUR-0095.1" && (w.Wave != 2 || s.State != "ready (stacked)") {
				t.Errorf("graph json: wave %d %+v", w.Wave, s)
			}
		}
	}
}

func TestStackOneChain(t *testing.T) {
	r := stackRepo(t, 3)
	r.Spec(S{Num: 90})
	r.Spec(after(91, "FUR-0090.1"))
	r.Spec(after(93, "FUR-0090.1", "FUR-0095.1"))
	r.Spec(after(94, "FUR-0090.1", "FUR-0091.1"))
	r.Spec(S{Num: 95})
	r.Spec(after(96, "FUR-0090.1", "FUR-0097.1"))
	r.Spec(S{Num: 97})
	claims := []schedule.Claim{held(90, true, 0), held(91, true, 90), held(95, true, 0)}
	p := schedule.Build(r.Load(), claims, true, 0)

	if it := find(p, "FUR-0093.1"); it == nil || it.State != schedule.Waiting || it.WhyWaiting != "on different stacks: FUR-0090.1, FUR-0095.1" {
		t.Errorf("FUR-0093.1 = %+v", it)
	}
	if it := find(p, "FUR-0094.1"); it == nil || it.State != schedule.Ready || it.StackOn.Branch != "step/0091-1" || it.Depth() != 2 {
		t.Errorf("FUR-0094.1 = %+v, want stacked on step/0091-1 at depth 2", it)
	}
	if it := find(p, "FUR-0096.1"); it == nil || it.State != schedule.Waiting || it.WhyWaiting != "FUR-0097.1 isn't handed back" {
		t.Errorf("FUR-0096.1 = %+v", it)
	}
}

func TestStackOverlapAndRank(t *testing.T) {
	r := stackRepo(t, 2)
	r.Spec(S{Num: 90, Steps: []St{{Touches: []string{"internal/a/**"}}}})
	r.Spec(S{Num: 91, Steps: []St{{After: []string{"FUR-0090.1"}, Touches: []string{"internal/a/x.go"}}}})
	r.Spec(S{Num: 92, Steps: []St{{Touches: []string{"internal/b/**"}}}})
	r.Spec(S{Num: 93, Steps: []St{{After: []string{"FUR-0090.1"}, Touches: []string{"internal/b/y.go"}}}})
	r.Spec(S{Num: 94, Steps: []St{{Touches: []string{"internal/c/**"}}}})
	r.Spec(after(96, "FUR-0091.1"))
	r.Spec(S{Num: 97, Steps: []St{{After: []string{"FUR-0090.1"}, Touches: []string{"internal/a/x.go"}}}})
	// 0090.1 is handed back; 0092.1 is claimed and still being built.
	claims := []schedule.Claim{held(90, true, 0), held(92, false, 0)}
	p := schedule.Build(r.Load(), claims, true, 0)

	// 0091.1 unblocks more than 0094.1, but plain ready steps rank first.
	if got := refsOf(p.Ready); got != "FUR-0094.1 FUR-0091.1 FUR-0093.1 FUR-0097.1" {
		t.Errorf("ready = %q", got)
	}
	// 0091.1 shares internal/a with its parent, which is fine; 0093.1 shares internal/b with
	// 0092.1, which isn't its parent; 0097.1 shares internal/a/x.go with 0091.1, already picked.
	if got := refsOf(p.Together); got != "FUR-0094.1 FUR-0091.1" {
		t.Errorf("together = %q", got)
	}
	var holds []string
	for _, h := range p.Held {
		holds = append(holds, h.Item.Ref()+"/"+h.ClashWith.Ref())
	}
	if got := strings.Join(holds, " "); got != "FUR-0093.1/FUR-0092.1 FUR-0097.1/FUR-0091.1" {
		t.Errorf("held = %q", got)
	}

	// With stack: 0 the same claims stack nothing.
	r.Write(".furrow.yml", spectest.Config)
	p = schedule.Build(r.Load(), claims, true, 0)
	if got := refsOf(p.Ready); got != "FUR-0094.1" {
		t.Errorf("stack: 0, ready = %q", got)
	}
	for _, it := range p.Waiting {
		if it.WhyWaiting != "" || it.Stacked() {
			t.Errorf("stack: 0, %s = %+v", it.Ref(), it)
		}
	}
}
