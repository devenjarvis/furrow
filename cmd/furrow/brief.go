package main

import (
	"fmt"
	"path/filepath"

	"github.com/devenjarvis/furrow/internal/brief"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/spec"
)

func init() {
	register("brief", "Print the reviewer's brief for a step's PR", runBrief)
}

// briefFlags are every flag furrow brief takes. Each mode (writing a step's brief, and later
// checking one or writing a spec's) reads the ones it needs.
type briefFlags struct {
	step   string
	base   string
	check  string
	asJSON bool
}

func runBrief(env *Env, args []string) int {
	fs := newFlags(env, "brief", "[--check FILE] [--step REF] [--base REF] [--json]")
	var f briefFlags
	fs.StringVar(&f.step, "step", "", "the step to brief (default: read from the current branch)")
	fs.StringVar(&f.base, "base", "", "compare against this ref (default: <remote>/<base>)")
	fs.StringVar(&f.check, "check", "", "check a filled-in brief, such as the PR description, instead of writing one")
	fs.BoolVar(&f.asJSON, "json", false, "print JSON")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 0 {
		fs.Usage()
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
	rep, err := brief.CheckFile(g, repo, brief.Options{Step: f.step, Base: f.base}, file)
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
