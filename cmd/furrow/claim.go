package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx"
)

func init() {
	register("claim", "Take a step: create its branch on the remote and check it out", runClaim)
	register("release", "Give a step back: delete its branch on the remote, or mark it released", runRelease)
	register("claims", "List claimed steps with their age: active, stale, leftover or released", runClaims)
}

func runClaim(env *Env, args []string) int {
	fs := newFlags(env, "claim", "<ref> [--no-checkout] [--force] [--json]")
	noCheckout := fs.Bool("no-checkout", false, "create the branch on the remote but don't switch to it")
	force := fs.Bool("force", false, "claim even if its touches overlap a claimed step")
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
	res, err := claim.Take(gitx.Git{Dir: repo.Root}, repo, pos[0], claim.Options{NoCheckout: *noCheckout, Force: *force})
	var claimed *claim.ClaimedError
	if errors.As(err, &claimed) {
		fmt.Fprintln(env.Stderr, err)
		return 3
	}
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, res)
	}
	res.Text(env.Stdout, repo.Config.Remote, *noCheckout)
	return 0
}

func runRelease(env *Env, args []string) int {
	fs := newFlags(env, "release", "<ref> [--json]")
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
	res, err := claim.Release(gitx.Git{Dir: repo.Root}, repo, pos[0])
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, res)
	}
	res.Text(env.Stdout, repo.Config.Remote)
	return 0
}

func runClaims(env *Env, args []string) int {
	fs := newFlags(env, "claims", "[--stale DURATION] [--json]")
	stale := fs.Duration("stale", 24*time.Hour, "call a claim stale after this long without a commit")
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parseFlags(fs, args); err != nil {
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	statuses, err := claim.StatusesAll(gitx.Git{Dir: repo.Root}, repo, env.Now(), *stale)
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, statuses)
	}
	claim.StatusText(env.Stdout, statuses)
	return 0
}
