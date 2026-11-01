package main

import (
	"fmt"

	"github.com/devenjarvis/furrow/internal/lifecycle"
)

func init() {
	register("done", "Hand a step back: mark it merged, in its own PR", runDone)
	register("review", "List the human checks waiting for sign-off", runReview)
	register("signoff", "Sign off human checks you made (people only, not agents)", runSignoff)
	register("archive", "Move a finished spec to the archive and log it in its area", runArchive)
}

func runDone(env *Env, args []string) int {
	fs := newFlags(env, "done", "<ref>")
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
	msg, err := lifecycle.Done(repo, pos[0])
	if err != nil {
		return fail(env, "%v", err)
	}
	fmt.Fprintln(env.Stdout, msg)
	return 0
}

func runReview(env *Env, args []string) int {
	fs := newFlags(env, "review", "[--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) != 0 {
		fs.Usage()
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	rep := lifecycle.Review(repo)
	if *asJSON {
		return printJSON(env, rep)
	}
	rep.Text(env.Stdout)
	return 0
}

func runSignoff(env *Env, args []string) int {
	// Refuse before anything else, so an agent learns nothing it could work around (INV-3).
	if err := lifecycle.CheckPerson(env.Getenv); err != nil {
		return fail(env, "%v", err)
	}
	fs := newFlags(env, "signoff", "<id> <AC-n>... --by NAME")
	// The default from `git config user.name` arrives with internal/gitx; until then --by is required.
	by := fs.String("by", "", "who checked it")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) < 2 {
		fs.Usage()
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	msg, err := lifecycle.Signoff(repo, lifecycle.SignoffRequest{
		ID: pos[0], Criteria: pos[1:], By: *by, Getenv: env.Getenv, Now: env.Now(),
	})
	if err != nil {
		return fail(env, "%v", err)
	}
	fmt.Fprintln(env.Stdout, msg)
	return 0
}

func runArchive(env *Env, args []string) int {
	fs := newFlags(env, "archive", "<id>")
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
	msg, err := lifecycle.Archive(repo, pos[0], env.Now())
	if err != nil {
		return fail(env, "%v", err)
	}
	fmt.Fprintln(env.Stdout, msg)
	return 0
}
