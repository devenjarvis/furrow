package main

import (
	"fmt"

	"github.com/devenjarvis/furrow/internal/spec"
)

func init() {
	register("lint", "Check every spec, area and invariant against the standard", runLint)
}

func runLint(env *Env, args []string) int {
	fs := newFlags(env, "lint", "")
	if _, err := parseFlags(fs, args); err != nil {
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	problems := spec.Lint(repo)
	for _, p := range problems {
		fmt.Fprintln(env.Stderr, p)
	}
	line, clean := spec.Summary(repo, problems)
	if !clean {
		fmt.Fprintln(env.Stderr)
		fmt.Fprintln(env.Stderr, line)
		return 1
	}
	fmt.Fprintln(env.Stdout, line)
	return 0
}
