package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/devenjarvis/furrow/internal/brief"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/spec"
)

func init() {
	register("brief", "Print the reviewer's brief for a step's PR", runBrief)
}

// briefFlags are every flag furrow brief takes. Each mode (writing or checking a step's brief, or
// a spec's) reads the ones it needs.
type briefFlags struct {
	step    string
	base    string
	check   string
	asJSON  bool
	spec    string
	pr      int
	offline bool
}

func (f briefFlags) specOptions() brief.SpecOptions {
	return brief.SpecOptions{Spec: f.spec, Step: f.step, PR: f.pr, Base: f.base, Offline: f.offline}
}

func runBrief(env *Env, args []string) int {
	fs := newFlags(env, "brief", "[--check FILE] [--step REF | --spec ID [--pr N] [--offline]] [--base REF] [--json]")
	var f briefFlags
	fs.StringVar(&f.step, "step", "", "the step to brief (default: read from the current branch)")
	fs.StringVar(&f.base, "base", "", "compare against this ref (default: <remote>/<base>)")
	fs.StringVar(&f.check, "check", "", "check a filled-in brief, such as the PR description, instead of writing one")
	fs.BoolVar(&f.asJSON, "json", false, "print JSON")
	fs.StringVar(&f.spec, "spec", "", "brief a PR that adds or amends this spec, such as FUR-0011")
	fs.IntVar(&f.pr, "pr", 0, "with --spec: the PR's number, so each open question links to its line")
	fs.BoolVar(&f.offline, "offline", false, "with --spec: don't ask the remote which steps are claimed")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 0 {
		fs.Usage()
		return 2
	}
	if f.spec != "" && f.step != "" {
		fmt.Fprintln(env.Stderr, brief.ErrSpecAndStep)
		return 2
	}
	if f.spec == "" && (f.pr != 0 || f.offline) {
		fmt.Fprintln(env.Stderr, "`--pr` and `--offline` go with `--spec`.")
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	g := gitx.Git{Dir: repo.Root}
	if f.check != "" {
		return runBriefCheck(env, g, repo, f)
	}
	if f.spec != "" {
		return runSpecBrief(env, g, repo, f)
	}
	b, err := brief.Build(g, repo, brief.Options{Step: f.step, Base: f.base})
	if err != nil {
		return fail(env, "%v", err)
	}
	if f.asJSON {
		return printJSON(env, b)
	}
	fmt.Fprint(env.Stdout, b.Markdown())
	return 0
}

// runBriefCheck reads a filled-in brief and reports what's missing from it. It only reads.
func runBriefCheck(env *Env, g gitx.Git, repo *spec.Repo, f briefFlags) int {
	file := f.check
	if !filepath.IsAbs(file) {
		file = filepath.Join(env.Dir, file)
	}
	var rep *brief.Report
	var err error
	if f.spec != "" {
		rep, err = brief.CheckSpecFile(gitx.Git{Dir: g.Dir, Env: gitEnv(env)}, repo, f.specOptions(), file)
	} else {
		rep, err = brief.CheckFile(g, repo, brief.Options{Step: f.step, Base: f.base}, file)
	}
	if errors.Is(err, brief.ErrSpecAndStep) {
		fmt.Fprintln(env.Stderr, err)
		return 2
	}
	if err != nil {
		return fail(env, "%v", err)
	}
	if f.asJSON {
		if code := printJSON(env, rep); code != 0 {
			return code
		}
	} else {
		rep.Text(env.Stdout)
	}
	return rep.ExitCode()
}

// runSpecBrief prints the brief for a PR that adds or amends a spec.
func runSpecBrief(env *Env, g gitx.Git, repo *spec.Repo, f briefFlags) int {
	b, err := brief.BuildSpec(gitx.Git{Dir: g.Dir, Env: gitEnv(env)}, repo, f.specOptions())
	if errors.Is(err, brief.ErrSpecAndStep) {
		fmt.Fprintln(env.Stderr, err)
		return 2
	}
	if err != nil {
		return fail(env, "%v", err)
	}
	if f.asJSON {
		return printJSON(env, b)
	}
	fmt.Fprint(env.Stdout, b.Markdown())
	return 0
}
