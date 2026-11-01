package main

import (
	"os"
	"path/filepath"

	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/territory"
)

func init() {
	register("territory", "Prove the current step's diff stays inside its touches", runTerritory)
}

func runTerritory(env *Env, args []string) int {
	fs := newFlags(env, "territory", "[--step REF] [--base REF] [--body-file FILE] [--json]")
	step := fs.String("step", "", "the step to check (default: read from the current branch)")
	base := fs.String("base", "", "compare against this ref (default: <remote>/<base>)")
	bodyFile := fs.String("body-file", "", "the PR description; paths under \"## Territory\" are explained")
	asJSON := fs.Bool("json", false, "print JSON")
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
	opt := territory.Options{Step: *step, Base: *base}
	if *bodyFile != "" {
		file := *bodyFile
		if !filepath.IsAbs(file) {
			file = filepath.Join(env.Dir, file)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return fail(env, "%v", err)
		}
		opt.Body = string(data)
	}
	rep, err := territory.Check(gitx.Git{Dir: repo.Root}, repo, opt)
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		if code := printJSON(env, rep); code != 0 {
			return code
		}
	} else {
		rep.Text(env.Stdout)
	}
	return rep.ExitCode()
}
