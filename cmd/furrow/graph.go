package main

import (
	"fmt"

	"github.com/devenjarvis/furrow/internal/schedule"
)

func init() {
	register("graph", "Show every open step in waves: what unblocks what", runGraph)
}

func runGraph(env *Env, args []string) int {
	fs := newFlags(env, "graph", "[--json] [--offline]")
	asJSON := fs.Bool("json", false, "print JSON")
	offline := fs.Bool("offline", false, "don't ask the remote which steps are claimed")
	if _, err := parseFlags(fs, args); err != nil {
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	claims, _, note := readClaims(env, repo, *offline)
	waves := schedule.Waves(repo, claims)
	if *asJSON {
		return printJSON(env, schedule.GraphJSON(waves))
	}
	if note != "" {
		fmt.Fprintln(env.Stdout, note)
	}
	schedule.GraphText(env.Stdout, waves)
	return 0
}
