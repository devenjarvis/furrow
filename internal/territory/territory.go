// Package territory proves that a step's diff stays inside its touches: every changed file is
// inside the step's territory, or explained under "## Territory" in the PR description.
package territory

import (
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/spec"
)

// ErrNotOnStep means no --step was given and the current branch isn't a step branch.
var ErrNotOnStep = errors.New("not on a step branch")

type notOnStepError struct{ prefix string }

func (e notOnStepError) Error() string {
	return fmt.Sprintf("Not on a step branch. Pass `--step %s-NNNN.N`.", e.prefix)
}

func (e notOnStepError) Is(target error) bool { return target == ErrNotOnStep }

// StepFromBranch returns the step named by ref, or else the step whose branch is checked out.
func StepFromBranch(g gitx.Git, repo *spec.Repo, ref string) (*spec.Step, error) {
	if ref != "" {
		return repo.StepRef(ref)
	}
	branch, err := g.Run("branch", "--show-current")
	if err != nil {
		return nil, err
	}
	r, ok := claim.Parse(repo.Config, branch)
	if !ok {
		return nil, notOnStepError{prefix: repo.Config.Prefix}
	}
	st := repo.Step(r)
	if st == nil {
		return nil, fmt.Errorf("%s is the branch for %s, but there's no such step.", branch, r)
	}
	return st, nil
}

// ChangedFiles lists every file that differs between the merge base of base and HEAD and the
// working tree: committed, staged, unstaged, and untracked files not ignored. A rename counts as
// both paths. Paths are relative to g.Dir, sorted, without duplicates.
func ChangedFiles(g gitx.Git, base string) ([]string, error) {
	mb, err := g.Run("merge-base", base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("couldn't find where HEAD left %s: %v", base, err)
	}
	diff, err := g.Lines("-c", "core.quotePath=false", "diff", "--name-only", "--no-renames", "--relative", mb)
	if err != nil {
		return nil, err
	}
	untracked, err := g.Lines("-c", "core.quotePath=false", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, f := range append(diff, untracked...) {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Inside reports whether file is in the step's territory: it matches one of the step's touches
// (after aliases), or it's the step's own spec file in changes/ or archive/, or its area's file.
func Inside(repo *spec.Repo, st *spec.Step, file string) bool {
	specs := strings.Trim(repo.Config.Specs, "/")
	name := path.Base(st.Spec.File)
	always := []string{
		st.Spec.File,
		path.Join(specs, "changes", name),
		path.Join(specs, "archive", name),
		path.Join(specs, "areas", st.Spec.Area+".md"),
	}
	for _, a := range always {
		if file == a {
			return true
		}
	}
	for _, t := range st.Touches {
		if paths.Match(paths.Resolve(t, repo.Config.Aliases), file) {
			return true
		}
	}
	return false
}

// Explained returns the paths named under a "## Territory" heading in body, up to the next "## "
// heading: bare, in backticks, or as list items. Each is a whole path, matched exactly.
func Explained(body string) map[string]bool {
	out := map[string]bool{}
	in := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || trimmed == "##" {
			in = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(trimmed, "##")), "Territory")
			continue
		}
		if !in {
			continue
		}
		for _, tok := range tokens(trimmed) {
			out[tok] = true
		}
	}
	return out
}

// tokens splits a line into candidate paths: each backtick span whole, and each word of the rest
// with list markers and surrounding punctuation trimmed.
func tokens(line string) []string {
	var out []string
	parts := strings.Split(line, "`")
	for i, p := range parts {
		if i%2 == 1 && i < len(parts)-1 {
			out = append(out, clean(p))
			continue
		}
		for _, w := range strings.Fields(p) {
			out = append(out, clean(w))
		}
	}
	return out
}

func clean(w string) string {
	w = strings.TrimSpace(w)
	w = strings.Trim(w, `"'()[]{}<>,;:!?*`)
	w = strings.TrimRight(w, ".")
	w = strings.TrimPrefix(w, "./")
	return w
}

// ErrSignoffWithStep means Check was asked for a sign-off PR and a step at once.
var ErrSignoffWithStep = errors.New("--signoff checks a sign-off PR, not a step: drop --step")

// Options say what to check. Body is the PR description's text, if any. Signoff checks a
// sign-off PR instead of a step: every changed file must be under the specs folder's changes/,
// archive/ or areas/, and Body explains nothing.
type Options struct {
	Step    string
	Base    string
	Body    string
	Signoff bool
}

// InSpecs reports whether file is somewhere a sign-off PR may change: under the specs folder's
// changes/, archive/ or areas/.
func InSpecs(repo *spec.Repo, file string) bool {
	specs := strings.Trim(repo.Config.Specs, "/")
	for _, dir := range []string{"changes", "archive", "areas"} {
		if strings.HasPrefix(file, path.Join(specs, dir)+"/") {
			return true
		}
	}
	return false
}

// Report is the result of a check. Every changed file is in exactly one list. StackOn is the
// parent's branch when the base was found from a stacked claim commit.
type Report struct {
	Step      string   `json:"step"`
	Base      string   `json:"base"`
	StackOn   string   `json:"stack_on,omitempty"`
	Inside    []string `json:"inside"`
	Explained []string `json:"explained"`
	Outside   []string `json:"outside"`

	signoff bool
	specs   string
}

// Check works out the step and base, lists the changed files, and sorts each one into inside,
// explained or outside. With opt.Signoff there's no step: a file is inside when it's under the
// specs folder's changes/, archive/ or areas/, and outside otherwise.
func Check(g gitx.Git, repo *spec.Repo, opt Options) (*Report, error) {
	base := opt.Base
	if base == "" {
		base = repo.Config.Remote + "/" + repo.Config.Base
	}
	if opt.Signoff {
		if opt.Step != "" {
			return nil, ErrSignoffWithStep
		}
		files, err := ChangedFiles(g, base)
		if err != nil {
			return nil, err
		}
		rep := &Report{Base: base, Inside: []string{}, Explained: []string{}, Outside: []string{},
			signoff: true, specs: strings.Trim(repo.Config.Specs, "/")}
		for _, f := range files {
			if InSpecs(repo, f) {
				rep.Inside = append(rep.Inside, f)
			} else {
				rep.Outside = append(rep.Outside, f)
			}
		}
		return rep, nil
	}
	st, err := StepFromBranch(g, repo, opt.Step)
	if err != nil {
		return nil, err
	}
	stackOn := ""
	if opt.Base == "" {
		if parent := StackBase(g, repo, st); parent != "" {
			stackOn, base = parent, repo.Config.Remote+"/"+parent
		}
	}
	files, err := ChangedFiles(g, base)
	if err != nil {
		return nil, err
	}
	explained := Explained(opt.Body)
	rep := &Report{Step: st.Ref().String(), Base: base, StackOn: stackOn, Inside: []string{}, Explained: []string{}, Outside: []string{}}
	for _, f := range files {
		switch {
		case Inside(repo, st, f):
			rep.Inside = append(rep.Inside, f)
		case explained[f]:
			rep.Explained = append(rep.Explained, f)
		default:
			rep.Outside = append(rep.Outside, f)
		}
	}
	return rep, nil
}

// StackBase returns the branch a stacked step's diff is measured from: the branch named by the
// furrow-stack: trailer of the step's claim commit on HEAD, while that branch is on the remote and
// its step isn't merged on <remote>/<base>. It fetches that branch into <remote>/<branch>. It
// returns "" when the step isn't stacked, or its parent's branch is gone or merged: then the base
// is <remote>/<base>, as for any step. Offline, it uses <remote>/<branch> if git already has it.
func StackBase(g gitx.Git, repo *spec.Repo, st *spec.Step) string {
	_, parent, err := claim.ClaimCommit(g, "HEAD", st.Ref())
	if err != nil || parent == "" {
		return ""
	}
	cfg := repo.Config
	if ref, ok := claim.Parse(cfg, parent); ok {
		if pst := repo.Step(ref); pst != nil && claim.HandedBack(g, pst, cfg.Remote+"/"+cfg.Base) {
			return "" // merged on base: the parent's work is in base now
		}
	}
	tracking := "refs/remotes/" + cfg.Remote + "/" + parent
	out, err := g.Run("ls-remote", "--heads", cfg.Remote, "refs/heads/"+parent)
	switch {
	case err != nil:
		if _, err := g.Run("rev-parse", "--verify", "-q", tracking); err != nil {
			return ""
		}
		return parent
	case out == "":
		return "" // the parent's branch is gone
	}
	if _, err := g.Run("fetch", "-q", "--no-tags", cfg.Remote, "+refs/heads/"+parent+":"+tracking); err != nil {
		return ""
	}
	return parent
}

// ExitCode is 0 when nothing is outside and unexplained, and 1 otherwise.
func (r *Report) ExitCode() int {
	if len(r.Outside) > 0 {
		return 1
	}
	return 0
}

// Text writes the report as furrow territory prints it.
func (r *Report) Text(w io.Writer) {
	noun := "files"
	if len(r.Inside) == 1 {
		noun = "file"
	}
	if r.signoff {
		if len(r.Outside) == 0 {
			fmt.Fprintf(w, "Sign-off PR: %d %s, all under %s/.\n", len(r.Inside), noun, r.specs)
			return
		}
		fmt.Fprintf(w, "Sign-off PR: %d %s under %s/.\n", len(r.Inside), noun, r.specs)
		for _, f := range r.Outside {
			fmt.Fprintf(w, "A sign-off PR changes only specs. Move %s to its own PR.\n", f)
		}
		return
	}
	step := r.Step
	if r.StackOn != "" {
		step += " (stacked on " + r.StackOn + ")"
	}
	fmt.Fprintf(w, "%s: %d %s inside its territory.\n", step, len(r.Inside), noun)
	if len(r.Explained) > 0 {
		fmt.Fprintln(w, "Outside, explained in the PR:")
		for _, f := range r.Explained {
			fmt.Fprintf(w, "  %s\n", f)
		}
	}
	if len(r.Outside) > 0 {
		fmt.Fprintln(w, "Outside, not explained:")
		for _, f := range r.Outside {
			fmt.Fprintf(w, "  %s\n", f)
		}
		fmt.Fprintln(w, `Explain each one under "## Territory" in the PR, or move the change to its own step.`)
	}
}
