package main

import (
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/status"
)

func init() {
	register("status", "Say where everything stands: claims, hand-backs, ready steps, the owner's queue", runStatus)
}

func runStatus(env *Env, args []string) int {
	fs := newFlags(env, "status", "[--json] [--offline]")
	asJSON := fs.Bool("json", false, "print JSON")
	offline := fs.Bool("offline", false, "don't ask the remote for claims")
	if _, err := parseFlags(fs, args); err != nil {
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	g := gitx.Git{Dir: repo.Root, Env: gitEnv(env)}
	rep := status.Read(g, repo, env.Now(), *offline)
	if *asJSON {
		return printJSON(env, rep)
	}
	rep.Text(env.Stdout)
	return 0
}
