package main

import (
	"github.com/devenjarvis/furrow/internal/schedule"
)

func init() {
	register("graph", "Show every open step in waves: what unblocks what", runGraph)
}

func runGraph(env *Env, args []string) int {
	fs := newFlags(env, "graph", "[--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parseFlags(fs, args); err != nil {
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	waves := schedule.Waves(repo, nil)
	if *asJSON {
		return printJSON(env, schedule.GraphJSON(waves))
	}
	schedule.GraphText(env.Stdout, waves)
	return 0
}
