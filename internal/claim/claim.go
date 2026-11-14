// Package claim takes, lists and releases steps. A claim is a branch on the remote named after
// the step. Creating it is atomic, so at most one agent holds a step (INV-5), and it needs nothing
// but git (INV-2).
package claim

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
)

// ErrRemote means the remote couldn't be reached. Callers report it and carry on.
var ErrRemote = errors.New("couldn't reach the remote")

// ClaimedError means someone else holds the step. furrow claim exits 3 on it.
type ClaimedError struct {
	Ref    string
	Branch string
}

func (e *ClaimedError) Error() string {
	return fmt.Sprintf("%s is already claimed (%s). Run `furrow next` for another step.", e.Ref, e.Branch)
}

// Branch fills the config's branch pattern for a step: step/{num}-{step} and FUR-0003.1 make
// step/0003-1.
func Branch(cfg config.Config, ref spec.Ref) string {
	num := ref.Spec
	if i := strings.LastIndex(num, "-"); i >= 0 {
		num = num[i+1:]
	}
	b := strings.ReplaceAll(cfg.Branch, "{num}", num)
	return strings.ReplaceAll(b, "{step}", strconv.Itoa(ref.Step))
}

// Parse reads a branch name back into a step ref, if it matches the pattern.
func Parse(cfg config.Config, branch string) (spec.Ref, bool) {
	quoted := regexp.QuoteMeta(cfg.Branch)
	quoted = strings.Replace(quoted, regexp.QuoteMeta("{num}"), `(?P<num>\d{4})`, 1)
	quoted = strings.Replace(quoted, regexp.QuoteMeta("{step}"), `(?P<step>\d+)`, 1)
	re := regexp.MustCompile("^" + quoted + "$")
	m := re.FindStringSubmatch(branch)
	if m == nil {
		return spec.Ref{}, false
	}
	n, _ := strconv.Atoi(m[re.SubexpIndex("step")])
	return spec.Ref{Spec: cfg.Prefix + "-" + m[re.SubexpIndex("num")], Step: n}, true
}

// Listed is a claim branch on the remote. Released means its tip is a release commit: the step is
// free, exactly as if the branch were gone.
type Listed struct {
	schedule.Claim
	Released bool
}

// List reads the claims on the remote, sorted by ref: every claim branch but the released ones.
// It fetches the claim branches into refs/furrow/claims/ to read their tips.
func List(g gitx.Git, cfg config.Config) ([]schedule.Claim, error) {
	all, err := ListAll(g, cfg)
	if err != nil {
		return nil, err
	}
	return held(all), nil
}

// held drops the released branches.
func held(all []Listed) []schedule.Claim {
	var out []schedule.Claim
	for _, l := range all {
		if !l.Released {
			out = append(out, l.Claim)
		}
	}
	return out
}

// ListAll reads every claim branch on the remote, released ones included, sorted by ref. It
// fetches them into refs/furrow/claims/ (one fetch, and none when there are none) and marks a
// branch Released when its tip is a release commit (IsRelease).
func ListAll(g gitx.Git, cfg config.Config) ([]Listed, error) {
	heads, err := lsClaims(g, cfg)
	if err != nil {
		return nil, err
	}
	out := []Listed{}
	if len(heads) == 0 {
		return out, nil
	}
	if err := FetchClaims(g, cfg, heads); err != nil {
		return nil, err
	}
	msgs, err := tipMessages(g)
	if err != nil {
		return nil, err
	}
	for _, c := range heads {
		released := false
		// The batched messages rule out most tips; only a release-shaped one costs a git call.
		if releaseMessage(msgs[ClaimsRef(c.Branch)], c.Ref) {
			if released, err = IsRelease(g, ClaimsRef(c.Branch), c.Ref); err != nil {
				return nil, err
			}
		}
		out = append(out, Listed{Claim: c, Released: released})
	}
	return out, nil
}

// tipMessages reads the message of every fetched claim tip, by ref, in one git process.
func tipMessages(g gitx.Git) (map[string]string, error) {
	out, err := g.Run("for-each-ref", "--format=%(refname)%00%(contents)%1e", "refs/furrow/claims/")
	if err != nil {
		return nil, err
	}
	msgs := map[string]string{}
	for _, rec := range strings.Split(out, "\x1e") {
		name, msg, ok := strings.Cut(strings.TrimSpace(rec), "\x00")
		if ok {
			msgs[name] = msg
		}
	}
	return msgs, nil
}

// lsClaims asks the remote for its claim branches, sorted by ref, without reading their tips.
func lsClaims(g gitx.Git, cfg config.Config) ([]schedule.Claim, error) {
	lines, err := g.Lines("ls-remote", "--heads", cfg.Remote)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrRemote, cfg.Remote, err)
	}
	var out []schedule.Claim
	for _, l := range lines {
		sha, ref, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		branch := strings.TrimPrefix(ref, "refs/heads/")
		if r, ok := Parse(cfg, branch); ok {
			out = append(out, schedule.Claim{Ref: r, Branch: branch, SHA: sha})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref.Spec != out[j].Ref.Spec {
			return out[i].Ref.Spec < out[j].Ref.Spec
		}
		return out[i].Ref.Step < out[j].Ref.Step
	})
	return out, nil
}

// Options change how Take behaves.
type Options struct {
	NoCheckout bool // don't switch to the branch
	Force      bool // skip the overlap check
}

// Result is a successful claim. BaseBranch is the branch to open the PR against: the parent's
// branch for a stacked claim, else the config's base. StackOn is the parent's branch, or "".
type Result struct {
	Ref        string `json:"ref"`
	Branch     string `json:"branch"`
	SHA        string `json:"sha"`
	PRTitle    string `json:"pr_title"`
	BaseBranch string `json:"base_branch"`
	StackOn    string `json:"-"`
}

// Text writes a claim as furrow claim prints it. remote and noCheckout fill in how to switch to
// the branch when furrow didn't.
func (r *Result) Text(w io.Writer, remote string, noCheckout bool) {
	fmt.Fprintf(w, "Claimed %s on %s.\n", r.Ref, r.Branch)
	if r.StackOn != "" {
		fmt.Fprintf(w, "Stacked on %s. Open the PR against that branch.\n", r.StackOn)
	}
	if noCheckout {
		fmt.Fprintf(w, "Switch to it with: git switch -c %s %s/%s\n", r.Branch, remote, r.Branch)
	}
	fmt.Fprintf(w, "Open a draft PR titled: %s\n", r.PRTitle)
}

// Take claims a step: it must be ready, its touches mustn't overlap a claimed step, and the tree
// must be clean. It pushes a claim commit, unique to this claim, to a new branch on the remote
// with an empty lease, so the push fails if the branch exists. With the config's stack above 0 it
// reads the claim tips, and a step that stacks on a handed-back step gets its claim commit on the
// tip of the parent's branch, with a furrow-stack: trailer naming it. A step whose branch is
// released is claimed again: the claim commit gets the released tip as its second parent, and the
// push leases on that tip, so it's a fast-forward and exactly one agent wins.
func Take(g gitx.Git, repo *spec.Repo, text string, opt Options) (*Result, error) {
	cfg := repo.Config
	st, err := repo.StepRef(text)
	if err != nil {
		return nil, err
	}
	ref := st.Ref().String()
	branch := Branch(cfg, st.Ref())

	if _, err := g.Run("fetch", "-q", cfg.Remote, cfg.Base); err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrRemote, cfg.Remote, err)
	}
	all, err := ListAll(g, cfg)
	if err != nil {
		return nil, err
	}
	claims, released := held(all), ""
	for _, l := range all {
		if l.Released && l.Branch == branch {
			released = l.SHA
		}
	}
	if cfg.Stack > 0 {
		if claims, err = ReadStack(g, repo, claims); err != nil {
			return nil, err
		}
	}
	plan := schedule.Build(repo, claims, true, 0)
	item, err := readiness(repo, plan, claims, st, opt.Force)
	if err != nil {
		return nil, err
	}
	if !opt.NoCheckout {
		dirty, err := g.Run("status", "--porcelain")
		if err != nil {
			return nil, err
		}
		if dirty != "" {
			return nil, fmt.Errorf("You have uncommitted changes. Commit or stash them, then claim %s.", ref)
		}
	}

	onto, baseBranch, stackOn := cfg.Remote+"/"+cfg.Base, cfg.Base, ""
	if item != nil && item.Stacked() {
		// ListAll fetched the parent's tip just now, so a branch that moved since next is
		// built on where it is.
		stackOn = item.StackOn.Branch
		onto, baseBranch = ClaimsRef(stackOn), stackOn
	}
	base, err := g.Run("rev-parse", onto)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Claim %s: %s\n\n%s: %s", ref, st.Title, ClaimTrailer, hex.EncodeToString(nonce))
	if stackOn != "" {
		msg += fmt.Sprintf("\n%s: %s", StackTrailer, stackOn)
	}
	parents := []string{"-p", base}
	if released != "" {
		// The released tip is the second parent, so the push is a fast-forward; the tree is
		// still base's, so the branch starts where a fresh claim would.
		parents = append(parents, "-p", released)
	}
	sha, err := g.Run(append(append([]string{"commit-tree", base + "^{tree}"}, parents...), "-m", msg)...)
	if err != nil {
		return nil, err
	}
	_, err = g.Run("push", "-q", cfg.Remote, sha+":refs/heads/"+branch, "--force-with-lease=refs/heads/"+branch+":"+released)
	if err != nil {
		var gerr *gitx.Error
		if errors.As(err, &gerr) && (strings.Contains(gerr.Stderr, "stale info") || strings.Contains(gerr.Stderr, "already exists") || strings.Contains(gerr.Stderr, "rejected")) {
			return nil, &ClaimedError{Ref: ref, Branch: branch}
		}
		return nil, fmt.Errorf("%w %s: %v", ErrRemote, cfg.Remote, err)
	}
	if !opt.NoCheckout {
		if _, err := g.Run("switch", "-q", "-c", branch, sha); err != nil {
			return nil, err
		}
		if _, err := g.Run("fetch", "-q", cfg.Remote, branch); err == nil {
			_, _ = g.Run("branch", "-q", "--set-upstream-to="+cfg.Remote+"/"+branch, branch)
		}
	}
	return &Result{Ref: ref, Branch: branch, SHA: sha, PRTitle: fmt.Sprintf("[%s] %s", ref, st.Title),
		BaseBranch: baseBranch, StackOn: stackOn}, nil
}

// readiness refuses a step that can't be claimed and says why. Otherwise it returns the step's
// ready item, which says what it stacks on, if anything.
func readiness(repo *spec.Repo, plan *schedule.Plan, claims []schedule.Claim, st *spec.Step, force bool) (*schedule.Item, error) {
	ref := st.Ref().String()
	switch {
	case st.Spec.Archived || spec.StepMerged(st):
		return nil, fmt.Errorf("%s is already merged.", ref)
	case st.Spec.Status != spec.Accepted:
		return nil, fmt.Errorf("%s isn't open: its spec is %s.", ref, st.Spec.Status)
	}
	for _, it := range plan.Claimed {
		if it.Step == st {
			return nil, &ClaimedError{Ref: ref, Branch: it.Claim.Branch}
		}
	}
	for _, it := range plan.Blocked {
		if it.Step == st {
			return nil, fmt.Errorf("%s is blocked: %s", ref, st.Blocked)
		}
	}
	for _, it := range plan.Waiting {
		if it.Step == st {
			return nil, waiting(repo, claims, it)
		}
	}
	var item *schedule.Item
	for _, it := range plan.Ready {
		if it.Step == st {
			item = it
		}
	}
	if force {
		return item, nil
	}
	for _, it := range plan.Claimed {
		if item != nil && onStack(item, it.Step) {
			continue // a stacked step builds on top of its parent's chain, not beside it
		}
		if pair, ok := paths.Overlap(st.Touches, it.Step.Touches, repo.Config.Aliases); ok {
			return nil, fmt.Errorf("%s shares %s with %s, which is claimed (%s). Pass --force to claim it anyway.", ref, pair, it.Ref(), it.Claim.Branch)
		}
	}
	return item, nil
}

func onStack(it *schedule.Item, st *spec.Step) bool {
	for _, s := range it.Stack {
		if s == st {
			return true
		}
	}
	return false
}

// waiting says why a waiting step can't be claimed. With stacking off it names what the step
// waits on. With it on, a wait that isn't handed back (its branch is gone, or its tip doesn't say
// merged: true) is named as no longer handed back; any other reason is the schedule's own.
func waiting(repo *spec.Repo, claims []schedule.Claim, it *schedule.Item) error {
	ref := it.Ref()
	var names []string
	for _, d := range it.WaitsOn {
		names = append(names, d.Ref().String())
	}
	if repo.Config.Stack == 0 {
		return fmt.Errorf("%s waits on %s.", ref, strings.Join(names, ", "))
	}
	back := map[spec.Ref]bool{}
	for _, c := range claims {
		back[c.Ref] = c.HandedBack
	}
	var gone []string
	for _, d := range it.WaitsOn {
		if !back[d.Ref()] {
			gone = append(gone, d.Ref().String())
		}
	}
	switch {
	case len(gone) == 1:
		return fmt.Errorf("%s waits on %s, which is no longer handed back. Run furrow next again.", ref, gone[0])
	case len(gone) > 1:
		return fmt.Errorf("%s waits on %s, which are no longer handed back. Run furrow next again.", ref, strings.Join(gone, ", "))
	case it.WhyWaiting != "":
		return fmt.Errorf("%s waits on %s (%s). Run furrow next again.", ref, strings.Join(names, ", "), it.WhyWaiting)
	}
	return fmt.Errorf("%s waits on %s.", ref, strings.Join(names, ", "))
}

// Status is one claim with its age, for furrow claims. StackOn is the branch a stacked claim is
// built on; Orphaned means that branch is gone and its step isn't merged.
type Status struct {
	Ref        string `json:"ref"`
	Branch     string `json:"branch"`
	SHA        string `json:"sha"`
	AgeSeconds int64  `json:"age_seconds"`
	State      string `json:"state"` // active, stale, leftover or released
	StackOn    string `json:"stack_on,omitempty"`
	Orphaned   bool   `json:"orphaned,omitempty"`
}

// Statuses lists every claim with its age: the time since its branch's last commit, and the
// branch it's stacked on, read from its claim commit. It fetches the claim branches into
// refs/furrow/claims/ to read them. Released branches aren't claims, so they're left out.
func Statuses(g gitx.Git, repo *spec.Repo, now time.Time, stale time.Duration) ([]Status, error) {
	return statuses(g, repo, now, stale, false)
}

// StatusesAll is Statuses with the released branches too, in state released, so furrow claims
// shows the owner what to delete. A released claim is never stale or leftover.
func StatusesAll(g gitx.Git, repo *spec.Repo, now time.Time, stale time.Duration) ([]Status, error) {
	return statuses(g, repo, now, stale, true)
}

func statuses(g gitx.Git, repo *spec.Repo, now time.Time, stale time.Duration, withReleased bool) ([]Status, error) {
	all, err := ListAll(g, repo.Config)
	if err != nil {
		return nil, err
	}
	out := []Status{}
	onRemote := map[string]bool{}
	for _, l := range all {
		// A released branch counts as gone, so a claim stacked on it is orphaned.
		onRemote[l.Branch] = !l.Released
	}
	for _, l := range all {
		if l.Released && !withReleased {
			continue
		}
		c := l.Claim
		tip := ClaimsRef(c.Branch)
		ct, err := g.Run("log", "-1", "--format=%ct", tip)
		if err != nil {
			return nil, err
		}
		secs, _ := strconv.ParseInt(ct, 10, 64)
		age := now.Sub(time.Unix(secs, 0))
		s := Status{Ref: c.Ref.String(), Branch: c.Branch, SHA: c.SHA, AgeSeconds: int64(age / time.Second), State: "active"}
		if l.Released {
			s.State = "released"
			out = append(out, s)
			continue
		}
		st := repo.Step(c.Ref)
		switch {
		case st != nil && spec.StepMerged(st):
			s.State = "leftover"
		case age > stale:
			s.State = "stale"
		}
		if _, s.StackOn, err = ClaimCommit(g, tip, c.Ref); err != nil {
			return nil, err
		}
		if s.StackOn != "" && !onRemote[s.StackOn] {
			s.Orphaned = !parentMerged(repo, s.StackOn)
		}
		out = append(out, s)
	}
	return out, nil
}

// parentMerged reports whether the step whose claim branch is branch is merged in repo.
func parentMerged(repo *spec.Repo, branch string) bool {
	ref, ok := Parse(repo.Config, branch)
	if !ok {
		return false
	}
	st := repo.Step(ref)
	return st != nil && (st.Spec.Archived || spec.StepMerged(st))
}

// StatusText writes claims as furrow claims prints them. A stacked claim ends "on <branch>", or
// "orphaned (<branch> is gone)" when its parent's branch is gone and the parent isn't merged.
func StatusText(w io.Writer, statuses []Status) {
	if len(statuses) == 0 {
		fmt.Fprintln(w, "No claims.")
		return
	}
	rw, bw := 0, 0
	for _, s := range statuses {
		rw, bw = max(rw, len(s.Ref)), max(bw, len(s.Branch))
	}
	for _, s := range statuses {
		stack := ""
		switch {
		case s.Orphaned:
			stack = fmt.Sprintf("  orphaned (%s is gone)", s.StackOn)
		case s.StackOn != "":
			stack = "  on " + s.StackOn
		}
		note := ""
		switch s.State {
		case "leftover":
			note = "  (merged: release it)"
		case "stale":
			note = "  (no commits lately: check on it, or release it)"
		case "released":
			note = "  (released: delete the branch when you can)"
		}
		fmt.Fprintf(w, "%-*s  %-*s  %-8s  %s%s%s\n", rw, s.Ref, bw, s.Branch, s.State, age(time.Duration(s.AgeSeconds)*time.Second), stack, note)
	}
}

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
