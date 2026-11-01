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

// List reads every claim branch on the remote, sorted by ref.
func List(g gitx.Git, cfg config.Config) ([]schedule.Claim, error) {
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

// Result is a successful claim.
type Result struct {
	Ref     string
	Branch  string
	SHA     string
	PRTitle string
}

// Take claims a step: it must be ready, its touches mustn't overlap a claimed step, and the tree
// must be clean. It pushes a claim commit, unique to this claim, to a new branch on the remote
// with an empty lease, so the push fails if the branch exists.
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
	claims, err := List(g, cfg)
	if err != nil {
		return nil, err
	}
	plan := schedule.Build(repo, claims, true, 0)
	if err := readiness(repo, plan, st, branch, opt.Force); err != nil {
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

	base, err := g.Run("rev-parse", cfg.Remote+"/"+cfg.Base)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Claim %s: %s\n\nfurrow-claim: %s", ref, st.Title, hex.EncodeToString(nonce))
	sha, err := g.Run("commit-tree", base+"^{tree}", "-p", base, "-m", msg)
	if err != nil {
		return nil, err
	}
	_, err = g.Run("push", "-q", cfg.Remote, sha+":refs/heads/"+branch, "--force-with-lease=refs/heads/"+branch+":")
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
	return &Result{Ref: ref, Branch: branch, SHA: sha, PRTitle: fmt.Sprintf("[%s] %s", ref, st.Title)}, nil
}

func readiness(repo *spec.Repo, plan *schedule.Plan, st *spec.Step, branch string, force bool) error {
	ref := st.Ref().String()
	switch {
	case st.Spec.Archived || spec.StepMerged(st):
		return fmt.Errorf("%s is already merged.", ref)
	case st.Spec.Status != spec.Accepted:
		return fmt.Errorf("%s isn't open: its spec is %s.", ref, st.Spec.Status)
	}
	for _, it := range plan.Claimed {
		if it.Step == st {
			return &ClaimedError{Ref: ref, Branch: it.Claim.Branch}
		}
	}
	for _, it := range plan.Blocked {
		if it.Step == st {
			return fmt.Errorf("%s is blocked: %s", ref, st.Blocked)
		}
	}
	for _, it := range plan.Waiting {
		if it.Step == st {
			var names []string
			for _, d := range it.WaitsOn {
				names = append(names, d.Ref().String())
			}
			return fmt.Errorf("%s waits on %s.", ref, strings.Join(names, ", "))
		}
	}
	if force {
		return nil
	}
	for _, it := range plan.Claimed {
		if pair, ok := paths.Overlap(st.Touches, it.Step.Touches, repo.Config.Aliases); ok {
			return fmt.Errorf("%s shares %s with %s, which is claimed (%s). Pass --force to claim it anyway.", ref, pair, it.Ref(), it.Claim.Branch)
		}
	}
	return nil
}

// Release deletes a step's claim branch. It reports whether there was one.
func Release(g gitx.Git, repo *spec.Repo, text string) (string, bool, error) {
	st, err := repo.StepRef(text)
	if err != nil {
		return "", false, err
	}
	branch := Branch(repo.Config, st.Ref())
	out, err := g.Run("ls-remote", "--heads", repo.Config.Remote, "refs/heads/"+branch)
	if err != nil {
		return branch, false, fmt.Errorf("%w %s: %v", ErrRemote, repo.Config.Remote, err)
	}
	if out == "" {
		return branch, false, nil
	}
	if _, err := g.Run("push", "-q", repo.Config.Remote, "--delete", "refs/heads/"+branch); err != nil {
		return branch, false, err
	}
	return branch, true, nil
}

// Status is one claim with its age, for furrow claims.
type Status struct {
	Ref        string `json:"ref"`
	Branch     string `json:"branch"`
	SHA        string `json:"sha"`
	AgeSeconds int64  `json:"age_seconds"`
	State      string `json:"state"` // active, stale or leftover
}

// Statuses lists every claim with its age: the time since its branch's last commit. It fetches
// the claim branches into refs/furrow/claims/ to read them.
func Statuses(g gitx.Git, repo *spec.Repo, now time.Time, stale time.Duration) ([]Status, error) {
	claims, err := List(g, repo.Config)
	if err != nil {
		return nil, err
	}
	out := []Status{}
	if len(claims) == 0 {
		return out, nil
	}
	args := []string{"fetch", "-q", "--no-tags", repo.Config.Remote}
	for _, c := range claims {
		args = append(args, "+refs/heads/"+c.Branch+":refs/furrow/claims/"+c.Branch)
	}
	if _, err := g.Run(args...); err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrRemote, repo.Config.Remote, err)
	}
	for _, c := range claims {
		ct, err := g.Run("log", "-1", "--format=%ct", "refs/furrow/claims/"+c.Branch)
		if err != nil {
			return nil, err
		}
		secs, _ := strconv.ParseInt(ct, 10, 64)
		age := now.Sub(time.Unix(secs, 0))
		s := Status{Ref: c.Ref.String(), Branch: c.Branch, SHA: c.SHA, AgeSeconds: int64(age / time.Second), State: "active"}
		st := repo.Step(c.Ref)
		switch {
		case st != nil && spec.StepMerged(st):
			s.State = "leftover"
		case age > stale:
			s.State = "stale"
		}
		out = append(out, s)
	}
	return out, nil
}

// StatusText writes claims as furrow claims prints them.
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
		note := ""
		switch s.State {
		case "leftover":
			note = "  (merged: release it)"
		case "stale":
			note = "  (no commits lately: check on it, or release it)"
		}
		fmt.Fprintf(w, "%-*s  %-*s  %-8s  %s%s\n", rw, s.Ref, bw, s.Branch, s.State, age(time.Duration(s.AgeSeconds)*time.Second), note)
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
