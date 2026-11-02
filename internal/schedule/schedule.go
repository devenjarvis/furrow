// Package schedule works out which open steps can start, ranks them by how much work they unblock,
// and picks a set that can run side by side because their touches don't overlap.
package schedule

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/spec"
)

// States of an open step.
const (
	Ready   = "ready"
	Waiting = "waiting"
	Blocked = "blocked"
	Claimed = "claimed"
)

// Claim is a step someone holds: a branch on the remote.
type Claim struct {
	Ref    spec.Ref
	Branch string
	SHA    string
}

// Item is one open step and what the schedule knows about it.
type Item struct {
	Step       *spec.Step
	State      string
	WaitsOn    []*spec.Step // unmerged steps it's after
	Claim      *Claim
	Dependents int // open steps that wait on it, directly or through others
	Chain      int // the longest chain of open steps that waits on it
}

// Ref is the item's step ref.
func (it *Item) Ref() string { return it.Step.Ref().String() }

// Hold says why a ready step isn't in the together set.
type Hold struct {
	Item      *Item
	ClashWith *Item
	Paths     string
}

// Plan is the answer to "what can start now?".
type Plan struct {
	ClaimsChecked bool
	Agents        int     // 0 means no limit
	Ready         []*Item // ranked
	Together      []*Item // ranked, at most Agents when set
	Held          []Hold
	Claimed       []*Item
	Waiting       []*Item
	Blocked       []*Item
	repo          *spec.Repo
}

// Build works out every open step's state and the plan. Claims for unknown or merged steps are
// ignored. agents limits the together set; 0 means no limit.
func Build(repo *spec.Repo, claims []Claim, claimsChecked bool, agents int) *Plan {
	items := analyse(repo, claims)
	p := &Plan{ClaimsChecked: claimsChecked, Agents: agents, repo: repo}
	for _, it := range items {
		switch it.State {
		case Ready:
			p.Ready = append(p.Ready, it)
		case Claimed:
			p.Claimed = append(p.Claimed, it)
		case Waiting:
			p.Waiting = append(p.Waiting, it)
		case Blocked:
			p.Blocked = append(p.Blocked, it)
		}
	}
	rank(p.Ready)
	aliases := repo.Config.Aliases
	for _, it := range p.Ready {
		var clash *Item
		var pair string
		for _, other := range append(append([]*Item{}, p.Together...), p.Claimed...) {
			if pr, ok := paths.Overlap(it.Step.Touches, other.Step.Touches, aliases); ok {
				clash, pair = other, pr
				break
			}
		}
		if clash != nil {
			p.Held = append(p.Held, Hold{Item: it, ClashWith: clash, Paths: pair})
			continue
		}
		if agents > 0 && len(p.Together) >= agents {
			continue
		}
		p.Together = append(p.Together, it)
	}
	return p
}

// analyse returns every open step with its state, dependents and chain, in spec then step order.
func analyse(repo *spec.Repo, claims []Claim) []*Item {
	open := repo.OpenSteps()
	byStep := map[*spec.Step]*Item{}
	var items []*Item
	for _, st := range open {
		it := &Item{Step: st}
		byStep[st] = it
		items = append(items, it)
	}
	for i := range claims {
		c := claims[i]
		if st := repo.Step(c.Ref); st != nil {
			if it, ok := byStep[st]; ok {
				it.Claim = &c
			}
		}
	}
	// Edges from each open step to the unmerged steps it waits on.
	dependents := map[*spec.Step][]*Item{}
	for _, it := range items {
		it.WaitsOn = WaitsOn(repo, it.Step)
		for _, dep := range it.WaitsOn {
			dependents[dep] = append(dependents[dep], it)
		}
		switch {
		case it.Claim != nil:
			it.State = Claimed
		case it.Step.Blocked != "":
			it.State = Blocked
		case len(it.WaitsOn) > 0:
			it.State = Waiting
		default:
			it.State = Ready
		}
	}
	chain := map[*spec.Step]int{}
	var longest func(st *spec.Step, seen map[*spec.Step]bool) int
	longest = func(st *spec.Step, seen map[*spec.Step]bool) int {
		if v, ok := chain[st]; ok {
			return v
		}
		if seen[st] { // a cycle; lint reports it
			return 0
		}
		seen[st] = true
		best := 0
		for _, d := range dependents[st] {
			best = max(best, 1+longest(d.Step, seen))
		}
		delete(seen, st)
		chain[st] = best
		return best
	}
	for _, it := range items {
		reach := map[*spec.Step]bool{}
		stack := []*spec.Step{it.Step}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, d := range dependents[cur] {
				if !reach[d.Step] && d.Step != it.Step {
					reach[d.Step] = true
					stack = append(stack, d.Step)
				}
			}
		}
		it.Dependents = len(reach)
		it.Chain = longest(it.Step, map[*spec.Step]bool{})
	}
	return items
}

// WaitsOn returns the unmerged steps a step is after, deduplicated, in spec then step order.
func WaitsOn(repo *spec.Repo, st *spec.Step) []*spec.Step {
	refs, _ := repo.AfterRefs(st)
	seen := map[*spec.Step]bool{}
	var out []*spec.Step
	for _, ref := range refs {
		expanded := repo.Expand(ref)
		if len(expanded) == 0 && ref.Step == 0 && !repo.RefMerged(ref) {
			continue // a whole spec with no steps yet: lint's problem, not a wait we can name
		}
		for _, dep := range expanded {
			if dep != st && !spec.StepMerged(dep) && !seen[dep] {
				seen[dep] = true
				out = append(out, dep)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}

func less(a, b *spec.Step) bool {
	if a.Spec.Num != b.Spec.Num {
		return a.Spec.Num < b.Spec.Num
	}
	return a.N < b.N
}

var priorityOrder = map[string]int{"p0": 0, "p1": 1, "p2": 2}

func priority(s *spec.Spec) int {
	if v, ok := priorityOrder[s.Priority]; ok {
		return v
	}
	return 3
}

// rank orders ready steps: most dependents, longest chain, priority, then spec and step number.
func rank(items []*Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Dependents != b.Dependents {
			return a.Dependents > b.Dependents
		}
		if a.Chain != b.Chain {
			return a.Chain > b.Chain
		}
		if pa, pb := priority(a.Step.Spec), priority(b.Step.Spec); pa != pb {
			return pa < pb
		}
		return less(a.Step, b.Step)
	})
}

func refs(steps []*spec.Step) []string {
	out := []string{}
	for _, st := range steps {
		out = append(out, st.Ref().String())
	}
	return out
}

func size(st *spec.Step) string {
	if st.Size == "" {
		return "-"
	}
	return st.Size
}

func width(groups ...[]*Item) int {
	w := 0
	for _, g := range groups {
		for _, it := range g {
			w = max(w, len(it.Ref()))
		}
	}
	return w
}

// Text writes the plan as furrow next prints it.
func (p *Plan) Text(w io.Writer) {
	wd := width(p.Ready, p.Claimed, p.Waiting, p.Blocked)
	describe := func(it *Item) {
		st := it.Step
		fmt.Fprintf(w, "  %-*s  %s  %s  %s\n", wd, it.Ref(), st.Spec.Priority, size(st), st.Title)
		fmt.Fprintf(w, "  %-*s  %s · touches %s\n", wd, "", st.Spec.File, strings.Join(st.Touches, ", "))
	}
	if len(p.Ready) == 0 {
		fmt.Fprintln(w, "Nothing is ready to start.")
	} else if p.Agents > 0 {
		if len(p.Together) == 0 {
			// Nothing was picked, so every hold is against a claimed step.
			fmt.Fprintln(w, "Assignments: none. Every ready step shares files with a claimed step.")
		} else {
			fmt.Fprintln(w, "Assignments:")
		}
		for _, it := range p.Together {
			describe(it)
		}
		p.heldText(w)
	} else {
		fmt.Fprintln(w, "Ready to start:")
		for _, it := range p.Ready {
			describe(it)
		}
		var names []string
		for _, it := range p.Together {
			names = append(names, it.Ref())
		}
		fmt.Fprintf(w, "Can run together: %s\n", strings.Join(names, ", "))
		p.heldText(w)
	}
	if len(p.Claimed) > 0 {
		fmt.Fprintln(w, "Claimed:")
		for _, it := range p.Claimed {
			fmt.Fprintf(w, "  %-*s  %s\n", wd, it.Ref(), it.Claim.Branch)
		}
	}
	if len(p.Waiting) > 0 {
		fmt.Fprintln(w, "Waiting:")
		for _, it := range p.Waiting {
			fmt.Fprintf(w, "  %-*s  after %s\n", wd, it.Ref(), strings.Join(refs(it.WaitsOn), ", "))
		}
	}
	if len(p.Blocked) > 0 {
		fmt.Fprintln(w, "Blocked:")
		for _, it := range p.Blocked {
			fmt.Fprintf(w, "  %-*s  %s\n", wd, it.Ref(), it.Step.Blocked)
		}
	}
}

// heldText writes one line per held step: what it shares, and with which step.
func (p *Plan) heldText(w io.Writer) {
	for _, h := range p.Held {
		fmt.Fprintf(w, "  not %s: shares %s with %s\n", h.Item.Ref(), h.Paths, h.ClashWith.Ref())
	}
}

// StepJSON is how a step appears in --json output.
type StepJSON struct {
	Ref      string   `json:"ref"`
	Spec     string   `json:"spec"`
	Title    string   `json:"title"`
	Priority string   `json:"priority"`
	Size     string   `json:"size"`
	File     string   `json:"file"`
	Touches  []string `json:"touches"`
	After    []string `json:"after"`
	Branch   string   `json:"branch"`
}

// HoldJSON is how a held step appears in --json output.
type HoldJSON struct {
	Step      string `json:"step"`
	ClashWith string `json:"clash_with"`
	Paths     string `json:"paths"`
}

// PlanJSON is furrow next --json.
type PlanJSON struct {
	ClaimsChecked bool       `json:"claims_checked"`
	Ready         []StepJSON `json:"ready"`
	Together      []StepJSON `json:"together"`
	Held          []HoldJSON `json:"held"`
	Claimed       []StepJSON `json:"claimed"`
	Waiting       []StepJSON `json:"waiting"`
	Blocked       []StepJSON `json:"blocked"`
}

func toJSON(items []*Item) []StepJSON {
	out := []StepJSON{}
	for _, it := range items {
		st := it.Step
		touches := append([]string{}, st.Touches...)
		branch := ""
		if it.Claim != nil {
			branch = it.Claim.Branch
		}
		out = append(out, StepJSON{
			Ref: it.Ref(), Spec: st.Spec.ID, Title: st.Title, Priority: st.Spec.Priority, Size: st.Size,
			File: st.Spec.File, Touches: touches, After: refs(it.WaitsOn), Branch: branch,
		})
	}
	return out
}

// JSON is the plan as furrow next --json prints it.
func (p *Plan) JSON() PlanJSON {
	held := []HoldJSON{}
	for _, h := range p.Held {
		held = append(held, HoldJSON{Step: h.Item.Ref(), ClashWith: h.ClashWith.Ref(), Paths: h.Paths})
	}
	return PlanJSON{
		ClaimsChecked: p.ClaimsChecked, Ready: toJSON(p.Ready), Together: toJSON(p.Together), Held: held,
		Claimed: toJSON(p.Claimed), Waiting: toJSON(p.Waiting), Blocked: toJSON(p.Blocked),
	}
}

// MarshalJSON lets a plan be printed directly.
func (p *Plan) MarshalJSON() ([]byte, error) { return json.Marshal(p.JSON()) }

// Wave is one level of furrow graph.
type Wave struct {
	Number int
	Items  []*Item
}

// Waves puts every open step in a wave: 1 when it waits on nothing open, else one more than the
// deepest open step it waits on.
func Waves(repo *spec.Repo, claims []Claim) []Wave {
	items := analyse(repo, claims)
	byStep := map[*spec.Step]*Item{}
	for _, it := range items {
		byStep[it.Step] = it
	}
	depth := map[*Item]int{}
	var visit func(it *Item, seen map[*Item]bool) int
	visit = func(it *Item, seen map[*Item]bool) int {
		if d, ok := depth[it]; ok {
			return d
		}
		if seen[it] {
			return 1
		}
		seen[it] = true
		d := 1
		for _, dep := range it.WaitsOn {
			if di, ok := byStep[dep]; ok {
				d = max(d, 1+visit(di, seen))
			}
		}
		delete(seen, it)
		depth[it] = d
		return d
	}
	var waves []Wave
	for _, it := range items {
		d := visit(it, map[*Item]bool{})
		for len(waves) < d {
			waves = append(waves, Wave{Number: len(waves) + 1})
		}
		waves[d-1].Items = append(waves[d-1].Items, it)
	}
	return waves
}

// GraphText writes waves as furrow graph prints them.
func GraphText(w io.Writer, waves []Wave) {
	if len(waves) == 0 {
		fmt.Fprintln(w, "No open steps.")
		return
	}
	wd := 0
	for _, wave := range waves {
		wd = max(wd, width(wave.Items))
	}
	for _, wave := range waves {
		label := ""
		if wave.Number == 1 {
			label = " (waits on nothing open)"
		}
		fmt.Fprintf(w, "Wave %d%s\n", wave.Number, label)
		for _, it := range wave.Items {
			after := ""
			if len(it.WaitsOn) > 0 {
				after = "  after " + strings.Join(refs(it.WaitsOn), ", ")
			}
			fmt.Fprintf(w, "  %-*s  %-9s  %s%s\n", wd, it.Ref(), "["+it.State+"]", it.Step.Title, after)
		}
	}
}

// WaveJSON is one wave in furrow graph --json.
type WaveJSON struct {
	Wave  int            `json:"wave"`
	Steps []WaveStepJSON `json:"steps"`
}

// WaveStepJSON is one step in a wave.
type WaveStepJSON struct {
	Ref   string   `json:"ref"`
	State string   `json:"state"`
	Title string   `json:"title"`
	After []string `json:"after"`
}

// GraphJSON is furrow graph --json.
func GraphJSON(waves []Wave) []WaveJSON {
	out := []WaveJSON{}
	for _, wave := range waves {
		wj := WaveJSON{Wave: wave.Number, Steps: []WaveStepJSON{}}
		for _, it := range wave.Items {
			wj.Steps = append(wj.Steps, WaveStepJSON{Ref: it.Ref(), State: it.State, Title: it.Step.Title, After: refs(it.WaitsOn)})
		}
		out = append(out, wj)
	}
	return out
}
