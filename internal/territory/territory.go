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

// Options say what to check. Body is the PR description's text, if any.
type Options struct {
	Step string
	Base string
	Body string
}

// Report is the result of a check. Every changed file is in exactly one list.
type Report struct {
	Step      string   `json:"step"`
	Base      string   `json:"base"`
	Inside    []string `json:"inside"`
	Explained []string `json:"explained"`
	Outside   []string `json:"outside"`
}

// Check works out the step and base, lists the changed files, and sorts each one into inside,
// explained or outside.
func Check(g gitx.Git, repo *spec.Repo, opt Options) (*Report, error) {
	st, err := StepFromBranch(g, repo, opt.Step)
	if err != nil {
		return nil, err
	}
	base := opt.Base
	if base == "" {
		base = repo.Config.Remote + "/" + repo.Config.Base
	}
	files, err := ChangedFiles(g, base)
	if err != nil {
		return nil, err
	}
	explained := Explained(opt.Body)
	rep := &Report{Step: st.Ref().String(), Base: base, Inside: []string{}, Explained: []string{}, Outside: []string{}}
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
	fmt.Fprintf(w, "%s: %d %s inside its territory.\n", r.Step, len(r.Inside), noun)
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
