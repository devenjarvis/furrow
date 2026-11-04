// Package status answers "where does everything stand?" in one call, for furrow status. It puts
// together what furrow claims, furrow next and furrow review each know: the claims on the remote,
// the ready steps ranked as furrow next ranks them, and what waits on the owner.
package status

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/lifecycle"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
)

// Stale is how long a claim goes without a commit before it counts as stale, as furrow claims'
// default.
const Stale = 24 * time.Hour

// ReadyShown is how many ready steps the text shows; the rest are counted.
const ReadyShown = 5

// notChecked is what a claim section says when the remote wasn't asked.
const notChecked = "(remote not checked)"

// Claim is one claim branch as furrow status sees it: its status from furrow claims, and whether
// its tip has the step handed back.
type Claim struct {
	claim.Status
	HandedBack bool
}

// InProgress is a claimed step that isn't handed back yet.
type InProgress struct {
	Ref    string `json:"ref"`
	Branch string `json:"branch"`
	State  string `json:"state"`
	Age    string `json:"age"`
}

// HandedBack is a claimed step whose branch has it marked merged: true, waiting for review.
// StackOn is the branch it's stacked on, or nil when it's built on the base.
type HandedBack struct {
	Ref     string  `json:"ref"`
	Branch  string  `json:"branch"`
	StackOn *string `json:"stack_on"`
}

// Ready is a step that can start now.
type Ready struct {
	Ref     string                `json:"ref"`
	Title   string                `json:"title"`
	StackOn *schedule.StackOnJSON `json:"stack_on"`
}

// Report is the state of play. Its JSON is furrow status --json.
type Report struct {
	ClaimsChecked  bool         `json:"claims_checked"`
	InProgress     []InProgress `json:"in_progress"`
	HandedBack     []HandedBack `json:"handed_back"`
	Ready          []Ready      `json:"ready"`
	ReadyTotal     int          `json:"ready_total"`
	HumanChecks    int          `json:"human_checks"`
	ReadyToArchive []string     `json:"ready_to_archive"`
	LeftoverClaims int          `json:"leftover_claims"`
	Next           string       `json:"next"`
	base           string
}

// Read asks the remote for the claims, unless offline, and builds the report. A remote that
// can't be reached is treated as offline: the claim sections aren't checked (INV-2).
func Read(g gitx.Git, repo *spec.Repo, now time.Time, offline bool) *Report {
	if offline {
		return Build(repo, nil, false)
	}
	statuses, err := claim.Statuses(g, repo, now, Stale)
	if err != nil {
		return Build(repo, nil, false)
	}
	claims := make([]Claim, 0, len(statuses))
	for _, s := range statuses {
		c := Claim{Status: s}
		if ref, ok := claim.Parse(repo.Config, s.Branch); ok {
			if st := repo.Step(ref); st != nil {
				// Statuses fetched every claim branch, so its tip can be read here.
				c.HandedBack = claim.HandedBack(g, st, claim.ClaimsRef(s.Branch))
			}
		}
		claims = append(claims, c)
	}
	return Build(repo, claims, true)
}

// Build works out the report from the specs and the claims. Ages come from the claims' statuses,
// which Read works out against the injected clock.
func Build(repo *spec.Repo, claims []Claim, checked bool) *Report {
	r := &Report{
		ClaimsChecked: checked, InProgress: []InProgress{}, HandedBack: []HandedBack{}, Ready: []Ready{},
		ReadyToArchive: []string{}, base: repo.Config.Base,
	}
	var sched []schedule.Claim
	for _, c := range claims {
		ref, ok := claim.Parse(repo.Config, c.Branch)
		if !ok {
			continue
		}
		sched = append(sched, schedule.Claim{Ref: ref, Branch: c.Branch, SHA: c.SHA, HandedBack: c.HandedBack, StackOn: c.StackOn})
		st := repo.Step(ref)
		leftover := st == nil || spec.StepMerged(st) || c.State == "leftover"
		if leftover || c.State == "stale" || c.Orphaned {
			r.LeftoverClaims++
		}
		switch {
		case leftover:
		case c.HandedBack:
			h := HandedBack{Ref: c.Ref, Branch: c.Branch}
			if c.StackOn != "" {
				on := c.StackOn
				h.StackOn = &on
			}
			r.HandedBack = append(r.HandedBack, h)
		default:
			r.InProgress = append(r.InProgress, InProgress{Ref: c.Ref, Branch: c.Branch, State: c.State, Age: age(time.Duration(c.AgeSeconds) * time.Second)})
		}
	}
	sort.SliceStable(r.InProgress, func(i, j int) bool { return lessRef(r.InProgress[i].Ref, r.InProgress[j].Ref) })
	sort.SliceStable(r.HandedBack, func(i, j int) bool { return lessRef(r.HandedBack[i].Ref, r.HandedBack[j].Ref) })

	plan := schedule.Build(repo, sched, checked, 0)
	for _, it := range plan.Ready {
		rd := Ready{Ref: it.Ref(), Title: it.Step.Title}
		if it.Stacked() {
			rd.StackOn = &schedule.StackOnJSON{Ref: it.StackOn.Ref.String(), Branch: it.StackOn.Branch}
		}
		r.Ready = append(r.Ready, rd)
	}
	r.ReadyTotal = len(r.Ready)

	for _, s := range lifecycle.Review(repo).Specs {
		r.HumanChecks += len(s.Criteria)
	}
	for _, s := range repo.Specs {
		// The same test as furrow lint's "ready to archive" warning.
		if !s.Archived && s.Status == spec.Accepted && len(s.Steps) > 0 && s.Complete() {
			r.ReadyToArchive = append(r.ReadyToArchive, s.ID)
		}
	}
	sort.Strings(r.ReadyToArchive)
	r.Next = r.next()
	return r
}

// next is the first of the five rules that applies. What wasn't checked is skipped.
func (r *Report) next() string {
	switch {
	case r.ClaimsChecked && r.LeftoverClaims > 0:
		return "Run furrow claims, then furrow release what's left over."
	case r.ReadyTotal > 0:
		return fmt.Sprintf("Run furrow next --agents N for the %d ready steps.", r.ReadyTotal)
	case r.ClaimsChecked && len(r.HandedBack) > 0:
		return "Review the handed-back PRs, the bottom of each stack first."
	case r.HumanChecks > 0:
		return "Check what furrow review lists, then ask for a sign-off PR."
	default:
		return "Nothing to do. Write a spec with furrow-write."
	}
}

// Text writes the report for a person: six sections in a fixed order, each sorted by ref.
func (r *Report) Text(w io.Writer) {
	wd := 0
	for _, x := range r.InProgress {
		wd = max(wd, len(x.Ref))
	}
	for _, x := range r.HandedBack {
		wd = max(wd, len(x.Ref))
	}
	for _, x := range r.Ready {
		wd = max(wd, len(x.Ref))
	}
	bw := 0
	for _, x := range r.InProgress {
		bw = max(bw, len(x.Branch))
	}
	for _, x := range r.HandedBack {
		bw = max(bw, len(x.Branch))
	}
	header := func(name string, n int) {
		if n > 0 {
			fmt.Fprintf(w, "%s (%d):\n", name, n)
		} else {
			fmt.Fprintf(w, "%s:\n", name)
		}
	}

	header("In progress", len(r.InProgress))
	switch {
	case !r.ClaimsChecked:
		fmt.Fprintln(w, "  "+notChecked)
	case len(r.InProgress) == 0:
		fmt.Fprintln(w, "  (none)")
	}
	for _, x := range r.InProgress {
		fmt.Fprintf(w, "  %-*s  %-*s  %s  %s\n", wd, x.Ref, bw, x.Branch, x.State, x.Age)
	}

	header("Handed back, waiting for review", len(r.HandedBack))
	switch {
	case !r.ClaimsChecked:
		fmt.Fprintln(w, "  "+notChecked)
	case len(r.HandedBack) == 0:
		fmt.Fprintln(w, "  (none)")
	}
	for _, x := range r.HandedBack {
		on := r.base
		if x.StackOn != nil {
			on = *x.StackOn
		}
		fmt.Fprintf(w, "  %-*s  %-*s  on %s\n", wd, x.Ref, bw, x.Branch, on)
	}

	header("Ready now", r.ReadyTotal)
	if r.ReadyTotal == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for i, x := range r.Ready {
		if i == ReadyShown {
			fmt.Fprintf(w, "  +%d more: furrow next\n", r.ReadyTotal-ReadyShown)
			break
		}
		stack := ""
		if x.StackOn != nil {
			stack = fmt.Sprintf("  stacked on %s (%s)", x.StackOn.Branch, x.StackOn.Ref)
		}
		fmt.Fprintf(w, "  %-*s  %s%s\n", wd, x.Ref, x.Title, stack)
	}

	fmt.Fprintln(w, "Waiting on the owner:")
	if r.HumanChecks == 0 && len(r.ReadyToArchive) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	if r.HumanChecks > 0 {
		fmt.Fprintf(w, "  human checks: %d (furrow review)\n", r.HumanChecks)
	}
	if len(r.ReadyToArchive) > 0 {
		fmt.Fprintf(w, "  ready to archive: %s\n", strings.Join(r.ReadyToArchive, ", "))
	}

	switch {
	case !r.ClaimsChecked:
		fmt.Fprintln(w, "Stale or leftover claims: "+notChecked)
	case r.LeftoverClaims == 0:
		fmt.Fprintln(w, "Stale or leftover claims: (none)")
	default:
		fmt.Fprintf(w, "Stale or leftover claims: %d (furrow claims)\n", r.LeftoverClaims)
	}
	fmt.Fprintf(w, "Next: %s\n", r.Next)
}

// lessRef orders refs by spec then step number, so FUR-0018.10 comes after FUR-0018.9.
func lessRef(a, b string) bool {
	sa, na := splitRef(a)
	sb, nb := splitRef(b)
	if sa != sb {
		return sa < sb
	}
	return na < nb
}

func splitRef(ref string) (string, int) {
	i := strings.LastIndex(ref, ".")
	if i < 0 {
		return ref, 0
	}
	n := 0
	fmt.Sscanf(ref[i+1:], "%d", &n)
	return ref[:i], n
}

// age prints a duration as furrow claims does: minutes under an hour, hours under two days, then
// days.
func age(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
