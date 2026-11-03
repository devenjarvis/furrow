package main

import (
	"fmt"

	"github.com/devenjarvis/furrow/internal/lifecycle"
)

func init() {
	register("done", "Hand a step back: mark it merged, in its own PR", runDone)
	register("review", "List the human checks waiting for sign-off", runReview)
	register("signoff", "Sign off human checks a person made, on one spec or several", runSignoff)
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
	fs := newFlags(env, "signoff", "<id> <AC-n>... [<id> <AC-n>...]... [--by NAME]")
	// With no --by, lifecycle.Signoff asks git for user.name, except in an agent session.
	by := fs.String("by", "", "the person who checked it (default outside an agent session: git config user.name)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return 2
	}
	if len(pos) == 0 {
		fs.Usage()
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	msg, err := lifecycle.Signoff(repo, lifecycle.SignoffRequest{
		Args: pos, By: *by, Getenv: env.Getenv, Now: env.Now(),
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
