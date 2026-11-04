package status

import (
	"testing"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

func held(num int, state string, back bool) Claim {
	ref := "FUR-000" + string(rune('0'+num)) + ".1"
	branch := "step/000" + string(rune('0'+num)) + "-1"
	return Claim{Status: claim.Status{Ref: ref, Branch: branch, State: state}, HandedBack: back}
}

func TestStatusNextLine(t *testing.T) {
	const (
		rule1 = "Run furrow claims, then furrow release what's left over."
		rule2 = "Run furrow next --agents N for the 1 ready steps."
		rule3 = "Review the handed-back PRs, the bottom of each stack first."
		rule4 = "Check what furrow review lists, then ask for a sign-off PR."
		rule5 = "Nothing to do. Write a spec with furrow-write."
	)
	human := spectest.Spec{Num: 9, Steps: []spectest.Step{{Merged: true}}, Criteria: []string{"- [ ] AC-1: Looks right (human: look)"}}
	cases := []struct {
		name    string
		specs   []spectest.Spec
		claims  []Claim
		checked bool
		want    string
	}{
		{"stale claim beats everything", []spectest.Spec{{Num: 1}, {Num: 2}, human}, []Claim{held(1, "stale", false)}, true, rule1},
		{"leftover claim", []spectest.Spec{{Num: 1, Steps: []spectest.Step{{Merged: true}}}, {Num: 2}}, []Claim{held(1, "active", false)}, true, rule1},
		{"ready beats handed back", []spectest.Spec{{Num: 1}, {Num: 2}, human}, []Claim{held(1, "active", true)}, true, rule2},
		{"handed back beats human checks", []spectest.Spec{{Num: 1}, human}, []Claim{held(1, "active", true)}, true, rule3},
		{"human checks", []spectest.Spec{{Num: 1}, human}, []Claim{held(1, "active", false)}, true, rule4},
		{"unchecked claims are skipped", []spectest.Spec{human}, nil, false, rule4},
		{"nothing", []spectest.Spec{{Num: 1}}, []Claim{held(1, "active", false)}, true, rule5},
	}
	for _, c := range cases {
		r := spectest.New(t)
		for _, s := range c.specs {
			r.Spec(s)
		}
		rep := Build(r.Load(), c.claims, c.checked)
		if rep.Next != c.want {
			t.Errorf("%s: next = %q, want %q", c.name, rep.Next, c.want)
		}
	}
}
