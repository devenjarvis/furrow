package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/devenjarvis/furrow/internal/importer"
)

func init() {
	register("import", "Move a repo's specs onto Furrow from its old format (fetch, edition)", runImport)
}

const importUsage = "fetch|edition [--areas FILE] [--by NAME] [--write] [--json]"

func runImport(env *Env, args []string) int {
	fs := newFlags(env, "import", importUsage)
	fs.Usage = func() {
		fmt.Fprintf(env.Stderr, "Usage: furrow import %s\n\n", importUsage)
		fmt.Fprintln(env.Stderr, "Run it at the root of the repo. Without --write it's a dry run: it prints the plan and writes nothing.")
		fmt.Fprintln(env.Stderr)
		fs.PrintDefaults()
		fmt.Fprintln(env.Stderr)
		fmt.Fprintln(env.Stderr, importer.AreasHelp)
		fmt.Fprintln(env.Stderr)
		fmt.Fprintln(env.Stderr, importer.EditionHelp)
	}
	areas := fs.String("areas", "", "YAML file naming the areas and the paths they cover")
	by := fs.String("by", "", "who signed off the human: checks already ticked")
	write := fs.Bool("write", false, "write the plan (refused while it has problems)")
	asJSON := fs.Bool("json", false, "print the plan as JSON")
	pos, err := parseFlags(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fs.Usage()
		return 2
	}
	var plan *importer.Plan
	switch pos[0] {
	case "fetch":
		plan, err = importer.FetchDir(env.Dir, *areas, *by, env.Now())
	case "edition":
		plan, err = importer.EditionDir(env.Dir, *areas, *by, env.Now())
	default:
		return fail(env, "Can't import from %q. Furrow knows: fetch, edition.", pos[0])
	}
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		if code := printJSON(env, plan); code != 0 {
			return code
		}
	} else {
		plan.Text(env.Stdout, *write)
	}
	if !*write {
		if len(plan.Problems) > 0 {
			return 1
		}
		return 0
	}
	if err := plan.Write(); err != nil {
		return fail(env, "%v", err)
	}
	if !*asJSON {
		fmt.Fprintln(env.Stdout, "\n"+plan.Summary())
	}
	return 0
}
