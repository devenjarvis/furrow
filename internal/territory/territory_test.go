package territory_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
	"github.com/devenjarvis/furrow/internal/territory"
)

type S = spectest.Spec
type St = spectest.Step

// setup writes a repo with spec 7 (area "core", one step touching internal/thing/** and cli),
// commits it on main with a bare remote, and switches to the step's branch.
func setup(t *testing.T) *spectest.Repo {
	t.Helper()
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	r.Write("cmd/furrow/.keep", "")
	r.Write("README.md", "hi\n")
	r.Write(".gitignore", "bin/\n")
	r.Spec(S{Num: 7, Slug: "thing", Steps: []St{{Touches: []string{"internal/thing/**", "cli/thing.go"}}}})
	r.Spec(S{Num: 8, Slug: "other"})
	gittest.Init(t, r.Root)
	gittest.Must(t, r.Root, "switch", "-q", "-c", "step/0007-1")
	return r
}

func load(t *testing.T, r *spectest.Repo) *spec.Repo {
	t.Helper()
	return r.Load()
}

func TestStepFromBranch(t *testing.T) {
	r := setup(t)
	g := gittest.Git(r.Root)
	repo := load(t, r)

	st, err := territory.StepFromBranch(g, repo, "")
	if err != nil || st.Ref().String() != "FUR-0007.1" {
		t.Fatalf("on step/0007-1: %v %v", st, err)
	}
	// --step wins over the branch.
	st, err = territory.StepFromBranch(g, repo, "FUR-0008.1")
	if err != nil || st.Ref().String() != "FUR-0008.1" {
		t.Fatalf("--step: %v %v", st, err)
	}
	// Off a step branch, and detached, it fails with the message.
	want := "Not on a step branch. Pass `--step FUR-NNNN.N`."
	gittest.Must(t, r.Root, "switch", "-q", "main")
	_, err = territory.StepFromBranch(g, repo, "")
	if !errors.Is(err, territory.ErrNotOnStep) || err.Error() != want {
		t.Errorf("on main: err = %v", err)
	}
	gittest.Must(t, r.Root, "switch", "-q", "--detach", "HEAD")
	if _, err = territory.StepFromBranch(g, repo, ""); err == nil || err.Error() != want {
		t.Errorf("detached: err = %v", err)
	}
	// A step branch for a step that doesn't exist is an error too.
	gittest.Must(t, r.Root, "switch", "-q", "-c", "step/0099-1")
	if _, err = territory.StepFromBranch(g, repo, ""); err == nil {
		t.Error("step/0099-1 should fail: no such step")
	}
}

func TestChangedFiles(t *testing.T) {
	r := setup(t)
	g := gittest.Git(r.Root)
	// Main moves on after the branch point: its changes aren't ours.
	gittest.Must(t, r.Root, "switch", "-q", "main")
	r.Write("main-only.txt", "x")
	gittest.Commit(t, r.Root, "main moves")
	gittest.Must(t, r.Root, "push", "-q", "origin", "main")
	gittest.Must(t, r.Root, "switch", "-q", "step/0007-1")

	r.Write("internal/thing/committed.go", "package thing\n")
	gittest.Must(t, r.Root, "mv", "README.md", "docs/README.md")
	gittest.Commit(t, r.Root, "work")
	r.Write("internal/thing/staged.go", "package thing\n")
	gittest.Must(t, r.Root, "add", "internal/thing/staged.go")
	r.Write(".gitignore", "bin/\n# changed\n") // unstaged
	r.Write("untracked.txt", "u")
	r.Write("bin/furrow", "ignored")

	got, err := territory.ChangedFiles(g, "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "README.md", "docs/README.md", "internal/thing/committed.go", "internal/thing/staged.go", "untracked.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("changed = %v\nwant      %v", got, want)
	}
	if !sort.StringsAreSorted(got) {
		t.Error("not sorted")
	}

	if _, err := territory.ChangedFiles(g, "origin/nope"); err == nil {
		t.Error("a missing base should fail")
	}
}

func TestInside(t *testing.T) {
	r := setup(t)
	repo := load(t, r)
	st := repo.Step(spec.Ref{Spec: "FUR-0007", Step: 1})
	cases := map[string]bool{
		"internal/thing/a.go":         true,  // ** match
		"internal/thing/deep/b/c.go":  true,  // ** across folders
		"internal/thingy/a.go":        false, // whole segments only
		"cmd/furrow/thing.go":         true,  // alias cli -> cmd/furrow
		"cmd/furrow/other.go":         false,
		"specs/changes/0007-thing.md": true, // own spec file
		"specs/archive/0007-thing.md": true, // own spec file, archived
		"specs/areas/core.md":         true, // its area
		"specs/changes/0008-other.md": false,
		"specs/areas/other.md":        false,
		"go.mod":                      false,
	}
	for file, want := range cases {
		if got := territory.Inside(repo, st, file); got != want {
			t.Errorf("Inside(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestExplained(t *testing.T) {
	body := strings.Join([]string{
		"# [FUR-0007.1] Thing",
		"",
		"go.sum is mentioned here, outside the section.",
		"",
		"## Territory",
		"",
		"- go.mod: the new dependency",
		"* `Makefile` needs a target.",
		"Also docs/a.md, for the link.",
		"1. scripts/x.sh",
		"Not a match: docs/a.md.bak isn't changed, and dir/ is no file.",
		"",
		"## Testing",
		"",
		"- LICENSE",
	}, "\n")
	got := territory.Explained(body)
	for _, p := range []string{"go.mod", "Makefile", "docs/a.md", "scripts/x.sh"} {
		if !got[p] {
			t.Errorf("%s should be explained; got %v", p, got)
		}
	}
	for _, p := range []string{"go.sum", "LICENSE", "docs", "a.md", "mod"} {
		if got[p] {
			t.Errorf("%s shouldn't be explained", p)
		}
	}
	if len(territory.Explained("no heading here\n- go.mod\n")) != 0 {
		t.Error("no ## Territory section explains nothing")
	}

	// End to end: inside, explained and outside, and the exit code.
	r := setup(t)
	r.Write("internal/thing/a.go", "package thing\n")
	r.Write("go.mod", "module x\n")
	r.Write("Makefile", "all:\n")
	g := gittest.Git(r.Root)
	repo := load(t, r)

	rep, err := territory.Check(g, repo, territory.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExitCode() != 1 {
		t.Errorf("unexplained files: exit = %d, want 1", rep.ExitCode())
	}
	var b bytes.Buffer
	rep.Text(&b)
	wantText := "FUR-0007.1: 1 file inside its territory.\nOutside, not explained:\n  Makefile\n  go.mod\n" +
		"Explain each one under \"## Territory\" in the PR, or move the change to its own step.\n"
	if b.String() != wantText {
		t.Errorf("text:\n%s\nwant:\n%s", b.String(), wantText)
	}

	rep, err = territory.Check(g, repo, territory.Options{Body: "## Territory\n\n- `go.mod`\n"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExitCode() != 1 || !reflect.DeepEqual(rep.Explained, []string{"go.mod"}) || !reflect.DeepEqual(rep.Outside, []string{"Makefile"}) {
		t.Errorf("half explained: %+v exit %d", rep, rep.ExitCode())
	}

	rep, err = territory.Check(g, repo, territory.Options{Body: "## Territory\n\n- go.mod\n- Makefile\n"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExitCode() != 0 {
		t.Errorf("all explained: exit = %d, want 0 (%+v)", rep.ExitCode(), rep)
	}
	b.Reset()
	rep.Text(&b)
	if !strings.HasPrefix(b.String(), "FUR-0007.1: 1 file inside its territory.\nOutside, explained in the PR:\n  Makefile\n  go.mod\n") ||
		strings.Contains(b.String(), "not explained") {
		t.Errorf("text:\n%s", b.String())
	}

	// Nothing outside at all.
	gittest.Must(t, r.Root, "clean", "-fdq")
	r.Write("internal/thing/b.go", "package thing\n")
	rep, err = territory.Check(g, repo, territory.Options{})
	if err != nil || rep.ExitCode() != 0 {
		t.Fatalf("clean: %v %+v", err, rep)
	}
	b.Reset()
	rep.Text(&b)
	if b.String() != "FUR-0007.1: 1 file inside its territory.\n" {
		t.Errorf("text = %q", b.String())
	}
}

func TestTerritoryJSON(t *testing.T) {
	r := setup(t)
	repo := load(t, r)
	rep, err := territory.Check(gittest.Git(r.Root), repo, territory.Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"step":"FUR-0007.1","base":"origin/main","inside":[],"explained":[],"outside":[]}` {
		t.Errorf("json = %s", data)
	}

	r.Write("internal/thing/a.go", "x")
	r.Write("go.mod", "x")
	r.Write("Makefile", "x")
	rep, err = territory.Check(gittest.Git(r.Root), repo, territory.Options{Step: "FUR-0007.1", Base: "main", Body: "## Territory\n`Makefile`\n"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	data, _ = json.Marshal(rep)
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"step": "FUR-0007.1", "base": "main",
		"inside": []any{"internal/thing/a.go"}, "explained": []any{"Makefile"}, "outside": []any{"go.mod"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("json = %s", data)
	}
}

func TestTerritorySignoff(t *testing.T) {
	r := setup(t)
	g := gittest.Git(r.Root)
	repo := load(t, r)
	// Off a step branch: --signoff needs no step and ignores the branch name.
	gittest.Must(t, r.Root, "switch", "-q", "-c", "signoff/fur-0007")

	r.Write("specs/changes/0007-thing.md", "ticked\n")
	r.Write("specs/changes/0008-other.md", "ticked\n")
	r.Write("specs/archive/0009-old.md", "archived\n")
	r.Write("specs/areas/core.md", "folded\n")
	rep, err := territory.Check(g, repo, territory.Options{Signoff: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExitCode() != 0 || len(rep.Outside) != 0 || len(rep.Inside) != 4 {
		t.Errorf("only specs: exit %d %+v", rep.ExitCode(), rep)
	}
	var b bytes.Buffer
	rep.Text(&b)
	if b.String() != "Sign-off PR: 4 files, all under specs/.\n" {
		t.Errorf("text = %q", b.String())
	}

	// A code file, and a specs file outside changes/, archive/ and areas/, fail and are named.
	r.Write("internal/thing/a.go", "package thing\n")
	r.Write("specs/README.md", "changed\n")
	rep, err = territory.Check(g, repo, territory.Options{Signoff: true, Body: "## Territory\n- internal/thing/a.go\n"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExitCode() != 1 || !reflect.DeepEqual(rep.Outside, []string{"internal/thing/a.go", "specs/README.md"}) || len(rep.Explained) != 0 {
		t.Errorf("with code: exit %d %+v", rep.ExitCode(), rep)
	}
	b.Reset()
	rep.Text(&b)
	want := "Sign-off PR: 4 files under specs/.\n" +
		"A sign-off PR changes only specs. Move internal/thing/a.go to its own PR.\n" +
		"A sign-off PR changes only specs. Move specs/README.md to its own PR.\n"
	if b.String() != want {
		t.Errorf("text:\n%s\nwant:\n%s", b.String(), want)
	}

	// --json keeps today's shape.
	data, _ := json.Marshal(rep)
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	wantJSON := map[string]any{
		"step": "", "base": "origin/main",
		"inside":    []any{"specs/archive/0009-old.md", "specs/areas/core.md", "specs/changes/0007-thing.md", "specs/changes/0008-other.md"},
		"explained": []any{},
		"outside":   []any{"internal/thing/a.go", "specs/README.md"},
	}
	if !reflect.DeepEqual(got, wantJSON) {
		t.Errorf("json = %s", data)
	}

	// --signoff with --step is a usage error.
	if _, err := territory.Check(g, repo, territory.Options{Signoff: true, Step: "FUR-0007.1"}); !errors.Is(err, territory.ErrSignoffWithStep) {
		t.Errorf("--signoff --step: err = %v", err)
	}
}

func TestTerritoryStackBase(t *testing.T) {
	r := spectest.New(t)
	r.Write(".furrow.yml", spectest.Config+"stack: 3\n")
	r.Write("internal/.keep", "")
	r.Write("cmd/furrow/.keep", "")
	parent := S{Num: 90, Steps: []St{{Title: "Parent", Touches: []string{"internal/a/**"}}}}
	child := S{Num: 91, Steps: []St{{Title: "Child", After: []string{"FUR-0090.1"}, Touches: []string{"internal/b/**"}}}}
	r.Spec(parent)
	r.Spec(child)
	remote := gittest.Init(t, r.Root)
	g := gittest.Git(r.Root)
	done := parent
	done.Steps = []St{{Title: "Parent", Touches: []string{"internal/a/**"}, Merged: true}}

	// FUR-0090.1 is claimed, built and handed back.
	res, err := claim.Take(g, r.Load(), "FUR-0090.1", claim.Options{NoCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	gittest.Must(t, r.Root, "switch", "-q", "--detach", res.SHA)
	r.Write("internal/a/x.go", "x\n")
	r.Write(parent.Path(), spectest.Text(done))
	gittest.Commit(t, r.Root, "build FUR-0090.1")
	parentTip := gittest.Must(t, r.Root, "rev-parse", "HEAD")
	gittest.Must(t, r.Root, "push", "-q", "origin", "HEAD:refs/heads/step/0090-1")
	gittest.Must(t, r.Root, "switch", "-q", "main")

	// FUR-0091.1 stacks on it and changes one file of its own.
	res, err = claim.Take(g, r.Load(), "FUR-0091.1", claim.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.StackOn != "step/0090-1" {
		t.Fatalf("not stacked: %+v", res)
	}
	r.Write("internal/b/y.go", "y\n")
	gittest.Commit(t, r.Root, "build FUR-0091.1")

	check := func(base string) *territory.Report {
		t.Helper()
		rep, err := territory.Check(g, r.Load(), territory.Options{Base: base})
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	rep := check("")
	if rep.Base != "origin/step/0090-1" || rep.StackOn != "step/0090-1" ||
		strings.Join(rep.Inside, " ") != "internal/b/y.go" || len(rep.Outside) != 0 {
		t.Errorf("stacked: %+v", rep)
	}
	var b bytes.Buffer
	rep.Text(&b)
	if b.String() != "FUR-0091.1 (stacked on step/0090-1): 1 file inside its territory.\n" {
		t.Errorf("text = %q", b.String())
	}
	data, _ := json.Marshal(rep)
	if !strings.Contains(string(data), `"base":"origin/step/0090-1","stack_on":"step/0090-1"`) {
		t.Errorf("json = %s", data)
	}

	// --base wins: against base, the parent's files are outside.
	rep = check("origin/main")
	if rep.Base != "origin/main" || rep.StackOn != "" || strings.Join(rep.Outside, " ") != "internal/a/x.go "+parent.Path() {
		t.Errorf("--base: %+v", rep)
	}
	b.Reset()
	rep.Text(&b)
	if !strings.HasPrefix(b.String(), "FUR-0091.1: 1 file inside its territory.\n") {
		t.Errorf("--base text = %q", b.String())
	}

	// The parent's branch is gone: the base is <remote>/<base>.
	gittest.Must(t, remote, "branch", "-q", "-D", "step/0090-1")
	if rep = check(""); rep.Base != "origin/main" || rep.StackOn != "" {
		t.Errorf("parent gone: %+v", rep)
	}

	// The parent's branch is back, but the parent merged on base: the base is <remote>/<base>.
	gittest.Must(t, r.Root, "push", "-q", "origin", parentTip+":refs/heads/step/0090-1")
	if rep = check(""); rep.StackOn != "step/0090-1" {
		t.Fatalf("parent back: %+v", rep)
	}
	gittest.Must(t, r.Root, "push", "-q", "origin", parentTip+":refs/heads/main")
	gittest.Must(t, r.Root, "fetch", "-q", "origin", "main")
	if rep = check(""); rep.Base != "origin/main" || rep.StackOn != "" || len(rep.Outside) != 0 {
		t.Errorf("parent merged: %+v", rep)
	}
}
