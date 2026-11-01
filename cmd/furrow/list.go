package main

import (
	"encoding/json"
	"fmt"

	"github.com/devenjarvis/furrow/internal/spec"
)

func init() {
	register("list", "List every spec with its status and steps merged", runList)
}

func runList(env *Env, args []string) int {
	fs := newFlags(env, "list", "[--json]")
	asJSON := fs.Bool("json", false, "print JSON")
	if _, err := parseFlags(fs, args); err != nil {
		return 2
	}
	repo, ok := loadRepo(env)
	if !ok {
		return 1
	}
	entries := spec.List(repo)
	if *asJSON {
		return printJSON(env, entries)
	}
	for _, e := range entries {
		fmt.Fprintf(env.Stdout, "%s  %-8s  %d/%d  %s\n", e.ID, e.Status, e.StepsMerged, e.StepsTotal, e.Title)
	}
	return 0
}

// printJSON prints v as indented JSON with a trailing newline.
func printJSON(env *Env, v any) int {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fail(env, "%v", err)
	}
	fmt.Fprintln(env.Stdout, string(data))
	return 0
}
