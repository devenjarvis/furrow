package main

import (
	"fmt"

	"github.com/devenjarvis/furrow/internal/schedule"
)

func init() {
	register("next", "Say which steps can start now, and which can run together", runNext)
}

func runNext(env *Env, args []string) int {
	fs := newFlags(env, "next", "[--agents N] [--json] [--offline]")
	agents := fs.Int("agents", 0, "pick at most N steps that can run together")
	asJSON := fs.Bool("json", false, "print JSON")
	fs.Bool("offline", false, "don't ask the remote which steps are claimed")
	if _, err := parseFlags(fs, args); err != nil {
		return 2
	}
	if *agents < 0 {
		return fail(env, "--agents must be 0 or more")
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	// Claims come from the remote in FUR-0002.2. Until then, next always runs offline.
	plan := schedule.Build(repo, nil, false, *agents)
	if *asJSON {
		return printJSON(env, plan.JSON())
	}
	fmt.Fprintln(env.Stdout, "(Claims not checked: offline.)")
	plan.Text(env.Stdout)
	return 0
}
