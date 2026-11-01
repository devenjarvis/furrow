package main

import (
	"fmt"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
)

func init() {
	register("next", "Say which steps can start now, and which can run together", runNext)
}

func runNext(env *Env, args []string) int {
	fs := newFlags(env, "next", "[--agents N] [--json] [--offline]")
	agents := fs.Int("agents", 0, "pick at most N steps that can run together")
	asJSON := fs.Bool("json", false, "print JSON")
	offline := fs.Bool("offline", false, "don't ask the remote which steps are claimed")
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
	claims, checked, note := readClaims(env, repo, *offline)
	plan := schedule.Build(repo, claims, checked, *agents)
	if *asJSON {
		return printJSON(env, plan.JSON())
	}
	if note != "" {
		fmt.Fprintln(env.Stdout, note)
	}
	plan.Text(env.Stdout)
	return 0
}

// readClaims asks the remote which steps are claimed. When it can't, it says so and carries on
// without them (INV-2: furrow keeps working with what git can tell it).
func readClaims(env *Env, repo *spec.Repo, offline bool) ([]schedule.Claim, bool, string) {
	if offline {
		return nil, false, "(Claims not checked: offline.)"
	}
	claims, err := claim.List(gitx.Git{Dir: repo.Root, Env: gitEnv(env)}, repo.Config)
	if err != nil {
		return nil, false, fmt.Sprintf("(Couldn't reach %s, so claimed steps aren't left out.)", repo.Config.Remote)
	}
	return claims, true, ""
}

// gitEnv lets tests pin git's identity and config through the command's environment.
func gitEnv(env *Env) []string {
	if env.Getenv == nil {
		return nil
	}
	var out []string
	for _, k := range []string{"GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"} {
		if v := env.Getenv(k); v != "" {
			out = append(out, k+"="+v)
		}
	}
	return out
}
