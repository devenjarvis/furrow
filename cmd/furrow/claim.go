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
	register("release", "Give a step back: delete its branch on the remote", runRelease)
	register("claims", "List claimed steps with their age: active, stale or leftover", runClaims)
}

func runClaim(env *Env, args []string) int {
	fs := newFlags(env, "claim", "<ref> [--no-checkout] [--force]")
	noCheckout := fs.Bool("no-checkout", false, "create the branch on the remote but don't switch to it")
	force := fs.Bool("force", false, "claim even if its touches overlap a claimed step")
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
	fmt.Fprintf(env.Stdout, "Claimed %s on %s.\n", res.Ref, res.Branch)
	if *noCheckout {
		fmt.Fprintf(env.Stdout, "Switch to it with: git switch -c %s %s/%s\n", res.Branch, repo.Config.Remote, res.Branch)
	}
	fmt.Fprintf(env.Stdout, "Open a draft PR titled: %s\n", res.PRTitle)
	return 0
}

func runRelease(env *Env, args []string) int {
	fs := newFlags(env, "release", "<ref>")
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
	branch, existed, err := claim.Release(gitx.Git{Dir: repo.Root}, repo, pos[0])
	if err != nil {
		return fail(env, "%v", err)
	}
	if !existed {
		fmt.Fprintf(env.Stdout, "%s isn't claimed (no %s on %s).\n", pos[0], branch, repo.Config.Remote)
		return 0
	}
	fmt.Fprintf(env.Stdout, "Released %s: deleted %s on %s.\n", pos[0], branch, repo.Config.Remote)
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
	statuses, err := claim.Statuses(gitx.Git{Dir: repo.Root}, repo, env.Now(), *stale)
	if err != nil {
		return fail(env, "%v", err)
	}
	if *asJSON {
		return printJSON(env, statuses)
	}
	claim.StatusText(env.Stdout, statuses)
	return 0
}
