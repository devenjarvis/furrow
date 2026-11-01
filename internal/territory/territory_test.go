package territory_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

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
