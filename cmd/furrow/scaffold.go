package main

import (
	"fmt"
	"path/filepath"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/scaffold"
)

func init() {
	register("init", "Start Furrow in this repo, keeping any files that exist", runInit)
	register("new", "Start a draft spec with the next number", runNew)
	register("area", "Start a living spec for one part of the product", runArea)
	register("invariants", "List the product's invariants", runInvariants)
}

func runInit(env *Env, args []string) int {
	fs := newFlags(env, "init", "[--prefix P]")
	prefix := fs.String("prefix", "", "spec id prefix (default: the folder's name, upper-cased)")
	if pos, err := parseFlags(fs, args); err != nil {
		return 2
	} else if len(pos) > 0 {
		fs.Usage()
		return 2
	}
	res, err := scaffold.Init(env.Dir, *prefix)
	res.Text(env.Stdout)
	if err != nil {
		return fail(env, "%v", err)
	}
	return 0
}

func runNew(env *Env, args []string) int {
	fs := newFlags(env, "new", "<slug> --title T --area A [--json]")
	title := fs.String("title", "", "the spec's title")
	area := fs.String("area", "", "the area it changes (specs/areas/<area>.md)")
	asJSON := fs.Bool("json", false, "print JSON")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fs.Usage()
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	c, err := scaffold.New(repo, pos[0], *title, *area)
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, c)
	}
	fmt.Fprintln(env.Stdout, c.File)
	return 0
}

func runArea(env *Env, args []string) int {
	fs := newFlags(env, "area", "<name> --title T")
	title := fs.String("title", "", "the area's title")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 1 {
		fs.Usage()
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	file, err := scaffold.Area(repo, pos[0], *title)
	if err != nil {
		return fail(env, "%v", err)
	}
	fmt.Fprintln(env.Stdout, file)
	return 0
}

func runInvariants(env *Env, args []string) int {
	fs := newFlags(env, "invariants", "[--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if pos, err := parseFlags(fs, args); err != nil {
		return 2
	} else if len(pos) > 0 {
		fs.Usage()
		return 2
	}
	root, cfg, err := config.Find(env.Dir)
	if err != nil {
		return fail(env, "%v", err)
	}
	invs, err := scaffold.Invariants(filepath.Join(root, cfg.Product))
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, invs)
	}
	if len(invs) == 0 {
		fmt.Fprintf(env.Stdout, "No invariants in %s. Add an \"## Invariants\" section with lines like **INV-1: <rule>.**\n", cfg.Product)
		return 0
	}
	scaffold.WriteInvariants(env.Stdout, invs)
	return 0
}
