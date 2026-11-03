package importer

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/lifecycle"
	"github.com/devenjarvis/furrow/internal/spec"
)

var importDay = time.Date(2026, 11, 3, 15, 4, 0, 0, time.UTC)

// fixture copies testdata/fetch, a small Fetch repo, into a temp dir and returns its root.
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("testdata", "fetch")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func opts(t *testing.T, root string) Options {
	t.Helper()
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return Options{Root: root, Config: cfg, AreasFile: "areas.yml", By: "Deven Jarvis", Now: importDay}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// snapshot reads every file under root, so a test can prove nothing changed.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func importAndWrite(t *testing.T, root string) *Plan {
	t.Helper()
	plan, err := Fetch(opts(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Problems) > 0 {
		t.Fatalf("problems:\n%s", strings.Join(plan.Problems, "\n"))
	}
	if err := plan.Write(); err != nil {
		t.Fatal(err)
	}
	return plan
}

func load(t *testing.T, root string) *spec.Repo {
	t.Helper()
	repo, err := spec.LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func lintErrors(repo *spec.Repo) []string {
	var out []string
	for _, p := range spec.Lint(repo) {
		if !p.Warning {
			out = append(out, p.String())
		}
	}
	return out
}

func TestImportFetch(t *testing.T) {
	root := fixture(t)
	plan := importAndWrite(t, root)

	want := []PlannedSpec{
		{ID: "FETCH-0001", From: "specs/0001-app-shell.md", To: "specs/archive/0001-app-shell.md", Status: "done", Area: "app", Merged: true, Signoffs: []string{"AC-2"}},
		{ID: "FETCH-0002", From: "specs/0002-gh-bridge.md", To: "specs/archive/0002-gh-bridge.md", Status: "done", Area: "github", Merged: true, Signoffs: []string{"AC-1", "AC-3"}},
		{ID: "FETCH-0003", From: "specs/0003-rule-engine.md", To: "specs/changes/0003-rule-engine.md", Status: "accepted", Area: "triage", Merged: true, Signoffs: []string{}},
		{ID: "FETCH-0004", From: "specs/0004-needs-you-screen.md", To: "specs/changes/0004-needs-you-screen.md", Status: "accepted", Area: "needs-you", Merged: false, Signoffs: []string{}},
		{ID: "FETCH-0005", From: "specs/0005-profiles.md", To: "specs/changes/0005-profiles.md", Status: "draft", Area: "profiles", Merged: false, Signoffs: []string{}},
		{ID: "FETCH-0006", From: "specs/0006-trust-presets.md", To: "specs/changes/0006-trust-presets.md", Status: "paused", Area: "profiles", Merged: false, Signoffs: []string{}},
	}
	if !reflect.DeepEqual(plan.Specs, want) {
		t.Fatalf("plan specs:\n got %+v\nwant %+v", plan.Specs, want)
	}

	repo := load(t, root)
	if errs := lintErrors(repo); len(errs) > 0 {
		t.Fatalf("lint on the imported tree:\n%s", strings.Join(errs, "\n"))
	}
	for _, old := range []string{"specs/0001-app-shell.md", "specs/0003-rule-engine.md"} {
		if exists(root, old) {
			t.Errorf("%s should have moved", old)
		}
	}
	for _, keep := range []string{"specs/changes/.gitkeep", "specs/archive/.gitkeep", "specs/areas/.gitkeep"} {
		if !exists(root, keep) {
			t.Errorf("missing %s", keep)
		}
	}
	if got := read(t, root, "specs/README.md"); !strings.Contains(got, "# The spec standard") || !strings.Contains(got, "FETCH-0007") {
		t.Errorf("specs/README.md should be Furrow's, with the prefix filled in:\n%.200s", got)
	}
	if got := read(t, root, "specs/_template.md"); !strings.Contains(got, "id: FETCH-NNNN") || strings.Contains(got, "{{PREFIX}}") {
		t.Errorf("specs/_template.md should be Furrow's with FETCH:\n%.200s", got)
	}

	s1 := repo.Spec("FETCH-0001")
	if !s1.Archived || s1.Status != spec.Done || !s1.Steps[0].Merged {
		t.Errorf("FETCH-0001 should be archived, done and merged: %+v", s1)
	}
	// The dependency chain: each step waits on the whole spec before it.
	for id, after := range map[string][]string{
		"FETCH-0001": nil, "FETCH-0002": {"FETCH-0001"}, "FETCH-0003": {"FETCH-0002"},
		"FETCH-0004": {"FETCH-0003"}, "FETCH-0006": {"FETCH-0004", "FETCH-0005"},
	} {
		if got := repo.Spec(id).Steps[0].After; !reflect.DeepEqual(got, after) && !(len(got) == 0 && len(after) == 0) {
			t.Errorf("%s after = %v, want %v", id, got, after)
		}
	}
	s4 := repo.Spec("FETCH-0004")
	st := s4.Steps[0]
	if st.N != 1 || st.Title != "Needs you screen" || st.Size != "m" || !reflect.DeepEqual(st.Covers, []string{"AC-1", "AC-2"}) {
		t.Errorf("FETCH-0004's step = %+v", st)
	}
	if s4.Priority != "p1" || s4.HumanReview != "required" || s4.Invariants == nil || len(s4.Invariants) != 0 {
		t.Errorf("FETCH-0004 kept fields wrong: priority %q, review %q, invariants %#v", s4.Priority, s4.HumanReview, s4.Invariants)
	}
	if !strings.Contains(read(t, root, "specs/changes/0004-needs-you-screen.md"), "\nkind: chore\n") {
		t.Error("FETCH-0004 should keep kind: chore")
	}
	if !strings.Contains(read(t, root, "specs/changes/0005-profiles.md"), "\nkind: spike\n") {
		t.Error("FETCH-0005 should keep kind: spike")
	}
	// A title that needed quotes stays one value.
	if got := repo.Spec("FETCH-0002").Title; got != "gh bridge: notifications" {
		t.Errorf("FETCH-0002 title = %q", got)
	}
}

func TestImportFetchSignoffQueue(t *testing.T) {
	root := fixture(t)
	importAndWrite(t, root)
	repo := load(t, root)
	s := repo.Spec("FETCH-0003")
	if s.Status != spec.Accepted || !s.Steps[0].Merged || s.DerivedStatus() != "review" {
		t.Fatalf("FETCH-0003: status %s, merged %v, derived %s", s.Status, s.Steps[0].Merged, s.DerivedStatus())
	}
	rep := lifecycle.Review(repo)
	var got []string
	for _, rs := range rep.Specs {
		for _, c := range rs.Criteria {
			got = append(got, rs.ID+" "+c.ID)
		}
	}
	if want := []string{"FETCH-0003 AC-3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("furrow review lists %v, want %v", got, want)
	}
	// A ready spec with an open non-human box stays unmerged.
	if repo.Spec("FETCH-0004").Steps[0].Merged {
		t.Error("FETCH-0004 has an open test: box, so it shouldn't be merged")
	}
}

func TestImportFetchRewritesPaths(t *testing.T) {
	root := fixture(t)
	before := snapshot(t, root)
	importAndWrite(t, root)
	repo := load(t, root)

	// Touches: its own file is dropped, another spec's follows it, everything else stays.
	if got, want := repo.Spec("FETCH-0003").Steps[0].Touches, []string{
		"Packages/AcmeCore/Sources/AcmeCore/Rules/**", "specs/archive/0001-app-shell.md", "specs/README.md",
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("FETCH-0003 touches = %v, want %v", got, want)
	}
	// Context: spec entries point to the new path, anchors kept.
	if got, want := repo.Spec("FETCH-0003").Context, []string{"specs/archive/0002-gh-bridge.md#decisions", "docs/architecture.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FETCH-0003 context = %v, want %v", got, want)
	}
	if got, want := repo.Spec("FETCH-0006").Context, []string{"specs/changes/0005-profiles.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FETCH-0006 context = %v, want %v", got, want)
	}
	if got, want := repo.Spec("FETCH-0002").Context, []string{"docs/product.md#scoring", "specs/archive/0001-app-shell.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FETCH-0002 context = %v, want %v", got, want)
	}

	// Bodies: links follow their spec, and nothing else changes.
	body := func(id string) string { return repo.Spec(id).Body }
	oldBody := func(rel string) string {
		_, b, _ := strings.Cut(before[rel][4:], "\n---\n")
		return b
	}
	cases := []struct {
		id, from string
		repl     []string
	}{
		{"FETCH-0001", "specs/0001-app-shell.md", []string{"in specs/0002-gh-bridge.md,", "in specs/archive/0002-gh-bridge.md,"}},
		{"FETCH-0003", "specs/0003-rule-engine.md", []string{
			"(./0001-app-shell.md)", "(../archive/0001-app-shell.md)",
			"`specs/0002-gh-bridge.md`", "`specs/archive/0002-gh-bridge.md`",
		}},
		{"FETCH-0006", "specs/0006-trust-presets.md", nil},
		{"FETCH-0004", "specs/0004-needs-you-screen.md", nil},
	}
	for _, c := range cases {
		want := strings.NewReplacer(c.repl...).Replace(oldBody(c.from))
		if c.id == "FETCH-0001" {
			want += "\n## Decisions\n\n- 2026-11-03: AC-2 signed off by Deven Jarvis (imported).\n"
		}
		if got := body(c.id); got != want {
			t.Errorf("%s body:\n got %q\nwant %q", c.id, got, want)
		}
	}
	b3 := body("FETCH-0003")
	for _, keep := range []string{
		"[Needs you](0004-needs-you-screen.md)",                            // same folder: unchanged
		"specs/0099-not-a-spec.md",                                         // not a spec: unchanged
		"AC-2: The pile names match specs/0001-app-shell.md (test: Rule", // criteria text: never changed
	} {
		if !strings.Contains(b3, keep) {
			t.Errorf("FETCH-0003 body should keep %q", keep)
		}
	}
	if b1 := body("FETCH-0001"); !strings.Contains(b1, "docs/decisions/0012-your-line.md") {
		t.Error("a path under docs/ that looks like a spec name must stay")
	}
	if b6 := body("FETCH-0006"); !strings.Contains(b6, "from 0005-profiles.md.") {
		t.Error("a bare link to a spec in the same folder stays as written")
	}
}

func TestImportFetchAreas(t *testing.T) {
	t.Run("no --areas", func(t *testing.T) {
		root := fixture(t)
		before := snapshot(t, root)
		o := opts(t, root)
		o.AreasFile = ""
		_, err := Fetch(o)
		if err == nil || err.Error() != "Name the areas: --areas FILE (see `furrow import --help`)" {
			t.Fatalf("err = %v", err)
		}
		if !reflect.DeepEqual(snapshot(t, root), before) {
			t.Error("a refused import wrote something")
		}
	})

	t.Run("a spec no area matches", func(t *testing.T) {
		root := fixture(t)
		writeFile(t, root, "specs/0007-models.md", strings.Replace(read(t, root, "specs/0005-profiles.md"), "FETCH-0005", "FETCH-0007", -1))
		before := snapshot(t, root)
		plan, err := Fetch(opts(t, root))
		if err != nil {
			t.Fatal(err)
		}
		want := "FETCH-0007 matches no area. Add one of its touches to an area's paths, or list it under specs:"
		if !contains(plan.Problems, want) {
			t.Fatalf("problems = %q, want one %q", plan.Problems, want)
		}
		for _, p := range plan.Problems {
			if strings.Contains(p, "area is empty") {
				t.Errorf("the unmatched area is said twice: %q", p)
			}
		}
		if err := plan.Write(); err == nil {
			t.Fatal("--write should refuse while there are problems")
		}
		if !reflect.DeepEqual(snapshot(t, root), before) {
			t.Error("a refused write wrote something")
		}
	})

	t.Run("first covered entry, in file order", func(t *testing.T) {
		root := fixture(t)
		plan, err := Fetch(opts(t, root))
		if err != nil {
			t.Fatal(err)
		}
		// FETCH-0004's first entry, Packages/AcmeCore/**, is too broad for any area, so the
		// second decides, and needs-you comes before app in the file.
		if got := plan.spec("FETCH-0004").Area; got != "needs-you" {
			t.Errorf("FETCH-0004 area = %q, want needs-you", got)
		}
		if got := plan.spec("FETCH-0005").Area; got != "profiles" {
			t.Errorf("FETCH-0005 area = %q, want profiles (from specs:)", got)
		}
	})

	t.Run("area files", func(t *testing.T) {
		root := fixture(t)
		oldApp := read(t, root, "specs/areas/app.md")
		plan := importAndWrite(t, root)

		wantApp := oldApp + "- FETCH-0001: App shell (archived 2026-11-03, imported)\n"
		if got := read(t, root, "specs/areas/app.md"); got != wantApp {
			t.Errorf("app.md:\n got %q\nwant %q", got, wantApp)
		}
		wantGitHub := "---\narea: github\ntitle: GitHub and enrichment\n---\n\n# GitHub and enrichment\n\n" +
			"## Behaviour\n\nTo write: how GitHub and enrichment works now.\n\n## History\n\n" +
			"- FETCH-0002: \"gh bridge: notifications\" (archived 2026-11-03, imported)\n"
		wantGitHub = strings.Replace(wantGitHub, `"gh bridge: notifications"`, "gh bridge: notifications", 1)
		if got := read(t, root, "specs/areas/github.md"); got != wantGitHub {
			t.Errorf("github.md:\n got %q\nwant %q", got, wantGitHub)
		}
		if got := read(t, root, "specs/areas/triage.md"); !strings.HasSuffix(got, "## History\n") {
			t.Errorf("triage.md has no archived specs, so its History is empty:\n%s", got)
		}
		var names []string
		for _, a := range plan.Areas {
			names = append(names, a.Area+":"+a.Action)
		}
		if want := []string{"needs-you:create", "github:create", "triage:create", "profiles:create", "app:update"}; !reflect.DeepEqual(names, want) {
			t.Errorf("areas = %v, want %v", names, want)
		}
	})
}

func TestImportFetchSignoffLines(t *testing.T) {
	t.Run("without --by", func(t *testing.T) {
		root := fixture(t)
		before := snapshot(t, root)
		o := opts(t, root)
		o.By = " "
		_, err := Fetch(o)
		if err == nil || err.Error() != "Say who signed off the imported human: checks: --by NAME." {
			t.Fatalf("err = %v", err)
		}
		if !reflect.DeepEqual(snapshot(t, root), before) {
			t.Error("a refused import wrote something")
		}
	})

	t.Run("one line per ticked human box", func(t *testing.T) {
		root := fixture(t)
		importAndWrite(t, root)
		text := read(t, root, "specs/archive/0002-gh-bridge.md")
		want := "## Decisions\n\n" +
			"- 2026-10-01: Chose `gh api` over the REST client because it reuses the login.\n" +
			"- 2026-11-03: AC-1 signed off by Deven Jarvis (imported).\n" +
			"- 2026-11-03: AC-3 signed off by Deven Jarvis (imported).\n" +
			"\n## Notes for the agent\n"
		if !strings.Contains(text, want) {
			t.Errorf("FETCH-0002's Decisions:\n%s", text)
		}
		if n := strings.Count(text, "signed off by"); n != 2 {
			t.Errorf("FETCH-0002 has %d sign-off lines, want 2", n)
		}
		if n := strings.Count(read(t, root, "specs/archive/0001-app-shell.md"), "signed off by"); n != 1 {
			t.Errorf("FETCH-0001 has %d sign-off lines, want 1", n)
		}
		for _, f := range []string{"specs/changes/0003-rule-engine.md", "specs/changes/0004-needs-you-screen.md"} {
			if strings.Contains(read(t, root, f), "signed off by") {
				t.Errorf("%s has no ticked human box, so no sign-off line", f)
			}
		}
	})

	t.Run("--by isn't needed without ticked human boxes", func(t *testing.T) {
		root := fixture(t)
		for _, f := range []string{"specs/0001-app-shell.md", "specs/0002-gh-bridge.md"} {
			os.Remove(filepath.Join(root, f))
		}
		writeFile(t, root, "specs/0003-rule-engine.md", strings.Replace(read(t, root, "specs/0003-rule-engine.md"), "depends_on: [FETCH-0002]", "depends_on: []", 1))
		o := opts(t, root)
		o.By = ""
		if _, err := Fetch(o); err != nil {
			t.Fatal(err)
		}
	})
}

func TestImportFetchDryRun(t *testing.T) {
	root := fixture(t)
	before := snapshot(t, root)
	plan, err := Fetch(opts(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot(t, root), before) {
		t.Fatal("planning wrote something")
	}
	var buf bytes.Buffer
	plan.Text(&buf, false)
	out := buf.String()
	for _, want := range []string{
		"6 Fetch specs: 2 to specs/archive/ (2 done), 4 to specs/changes/ (2 accepted, 1 draft, 1 paused).",
		"FETCH-0001  done      app        specs/0001-app-shell.md -> specs/archive/0001-app-shell.md  merged, signs off AC-2",
		"FETCH-0003  accepted  triage     specs/0003-rule-engine.md -> specs/changes/0003-rule-engine.md  merged",
		"  update  specs/areas/app.md  +1 History line",
		"  create  specs/areas/github.md  +1 History line",
		"No problems.",
		"Dry run: nothing written. Run it again with --write to write the plan.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run output lacks %q:\n%s", want, out)
		}
	}
	// Same input, same answer.
	again, _ := Fetch(opts(t, root))
	var buf2 bytes.Buffer
	again.Text(&buf2, false)
	if buf2.String() != out {
		t.Error("two dry runs print different plans")
	}

	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string][]map[string]any
	if err := json.Unmarshal([]byte(strings.Replace(string(data), `"problems":[]`, `"problems":[{}]`, 1)), &shape); err != nil {
		t.Fatalf("json: %v\n%s", err, data)
	}
	var keys []string
	for k := range shape["specs"][0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"area", "from", "id", "merged", "signoffs", "status", "to"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("spec keys = %v, want %v", keys, want)
	}

	// With a problem, --write refuses and writes nothing.
	writeFile(t, root, "specs/0002-gh-bridge.md", strings.Replace(read(t, root, "specs/0002-gh-bridge.md"), "- [x] AC-2:", "- [ ] AC-2:", 1))
	before = snapshot(t, root)
	plan, err = Fetch(opts(t, root))
	if err != nil {
		t.Fatal(err)
	}
	want := "FETCH-0002 is done but AC-2 isn't ticked. Tick it, or set the spec back to ready, in Fetch's format."
	if !contains(plan.Problems, want) {
		t.Fatalf("problems = %q, want %q", plan.Problems, want)
	}
	buf.Reset()
	plan.Text(&buf, true)
	if !strings.Contains(buf.String(), "Problems (") || !strings.Contains(buf.String(), want) {
		t.Errorf("the plan should print its problems:\n%s", buf.String())
	}
	if err := plan.Write(); err == nil || !strings.Contains(err.Error(), "Nothing written") {
		t.Fatalf("Write err = %v", err)
	}
	if !reflect.DeepEqual(snapshot(t, root), before) {
		t.Error("a refused write wrote something")
	}
}

func TestImportFetchBadFrontMatter(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "specs/0007-broken.md", "---\nid: FETCH-0007\nthis is not a key\n---\n\n# Broken\n")
	plan, err := Fetch(opts(t, root))
	if err != nil {
		t.Fatal(err)
	}
	want := "specs/0007-broken.md: can't read front matter line: this is not a key"
	if !contains(plan.Problems, want) {
		t.Fatalf("problems = %q, want %q", plan.Problems, want)
	}
}

func TestImportFetchNeedsConfig(t *testing.T) {
	root := fixture(t)
	os.Remove(filepath.Join(root, config.FileName))
	_, err := FetchDir(root, "areas.yml", "Deven", importDay)
	if err == nil || err.Error() != "No .furrow.yml here. Write one first: `furrow init --prefix FETCH`." {
		t.Fatalf("err = %v", err)
	}
}

func writeFile(t *testing.T, root, rel, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
