package scaffold

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

// lintErrors loads the repo at root and returns its lint errors (warnings are left out).
func lintErrors(t *testing.T, root string) []string {
	t.Helper()
	repo, err := spec.LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range spec.Lint(repo) {
		if !p.Warning {
			out = append(out, p.String())
		}
	}
	return out
}

func load(t *testing.T, root string) *spec.Repo {
	t.Helper()
	repo, err := spec.LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

var initPaths = []string{
	".furrow.yml",
	"specs/README.md",
	"specs/_template.md",
	"specs/changes/.gitkeep",
	"specs/areas/.gitkeep",
	"specs/archive/.gitkeep",
	"docs/product.md",
}

// AC-1
func TestInit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my-app2")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Init(root, "")
	if err != nil {
		t.Fatal(err)
	}
	var first bytes.Buffer
	res.Text(&first)
	var want strings.Builder
	for _, p := range initPaths {
		want.WriteString("created " + p + "\n")
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	if first.String() != want.String() {
		t.Errorf("first run printed\n%s\nwant\n%s", first.String(), want.String())
	}

	cfg, err := config.Load(filepath.Join(root, ".furrow.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Prefix != "MYAPP" {
		t.Errorf("prefix = %q, want MYAPP", cfg.Prefix)
	}
	readme, _ := os.ReadFile(filepath.Join(root, "specs/README.md"))
	if !strings.Contains(string(readme), "id: MYAPP-0007") || strings.Contains(string(readme), "{{") || strings.Contains(string(readme), "FUR-") {
		t.Errorf("README doesn't have the prefix filled in")
	}
	product, _ := os.ReadFile(filepath.Join(root, "docs/product.md"))
	if !strings.HasPrefix(string(product), "# my-app2\n") {
		t.Errorf("product.md should start with the repo name, got %q", strings.SplitN(string(product), "\n", 2)[0])
	}
	if invs := spec.ParseInvariants(string(product)); len(invs) != 1 || invs[0].ID != "INV-1" {
		t.Errorf("product.md should hold one example invariant, got %v", invs)
	}
	if errs := lintErrors(t, root); len(errs) != 0 {
		t.Errorf("a fresh init should lint clean: %v", errs)
	}

	// A second run keeps everything and changes nothing.
	if err := os.WriteFile(filepath.Join(root, "docs/product.md"), []byte("# Mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = Init(root, "OTHER")
	if err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	res.Text(&second)
	want.Reset()
	for _, p := range initPaths {
		want.WriteString("kept " + p + "\n")
	}
	if !strings.HasPrefix(second.String(), want.String()) {
		t.Errorf("second run printed\n%s\nwant it to start with\n%s", second.String(), want.String())
	}
	if !strings.Contains(second.String(), `no "## Invariants" section`) {
		t.Errorf("second run should say to add an Invariants section, printed\n%s", second.String())
	}
	if got, _ := os.ReadFile(filepath.Join(root, "docs/product.md")); string(got) != "# Mine\n" {
		t.Errorf("init overwrote docs/product.md")
	}
	if cfg, _ := config.Load(filepath.Join(root, ".furrow.yml")); cfg.Prefix != "MYAPP" {
		t.Errorf("init overwrote .furrow.yml")
	}
}

// AC-16 (FUR-0011)
func TestInitWritesVisualKey(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root, "APP"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".furrow.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var line string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "# visual:") {
			if line != "" {
				t.Fatalf(".furrow.yml has more than one visual line:\n%s", data)
			}
			line = l
		}
	}
	if line == "" {
		t.Fatalf(".furrow.yml has no commented visual line:\n%s", data)
	}
	if !strings.HasPrefix(line, "# visual: []   # ") || !strings.Contains(line, "visual proof") {
		t.Errorf("visual line = %q, want `# visual: []   # <what it does>` mentioning visual proof", line)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf(".furrow.yml doesn't parse: %v", err)
	}
	if cfg.Visual == nil || len(cfg.Visual) != 0 {
		t.Errorf("visual = %#v, want an empty list", cfg.Visual)
	}
	if cfg.Prefix != "APP" {
		t.Errorf("prefix = %q, want APP", cfg.Prefix)
	}
}

func TestInitPrefix(t *testing.T) {
	cases := map[string]string{"furrow": "FURROW", "my-app2": "MYAPP", "edition": "EDITIO", "123": "SPEC", "a_b": "AB"}
	for dir, want := range cases {
		if got := DefaultPrefix(dir); got != want {
			t.Errorf("DefaultPrefix(%q) = %q, want %q", dir, got, want)
		}
	}
	if _, err := Init(t.TempDir(), "bad-prefix"); err == nil {
		t.Errorf("Init should refuse a prefix with a dash")
	}
	root := t.TempDir()
	if _, err := Init(root, "ACME"); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := config.Load(filepath.Join(root, ".furrow.yml")); cfg.Prefix != "ACME" {
		t.Errorf("prefix = %q, want ACME", cfg.Prefix)
	}
}

// AC-2
func TestNew(t *testing.T) {
	r := spectest.New(t)
	r.Spec(spectest.Spec{Num: 3})
	r.Spec(spectest.Spec{Num: 7, Archived: true})

	if _, err := Area(r.Load(), "widgets", "Widgets"); err != nil {
		t.Fatal(err)
	}
	got, err := New(r.Load(), "add-widgets", "Add widgets: the first ones", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if got.File != "specs/changes/0008-add-widgets.md" || got.ID != "FUR-0008" {
		t.Errorf("New = %+v, want FUR-0008 in specs/changes/0008-add-widgets.md", got)
	}
	repo := r.Load()
	s := repo.Spec("FUR-0008")
	if s == nil {
		t.Fatalf("FUR-0008 not loaded; problems: %v", repo.Problems)
	}
	if s.Title != "Add widgets: the first ones" || s.Area != "widgets" || s.Status != spec.Draft {
		t.Errorf("spec = %q %q %q", s.Title, s.Area, s.Status)
	}
	if !strings.Contains(s.Body, "# FUR-0008: Add widgets: the first ones\n") {
		t.Errorf("heading not filled in:\n%s", s.Body)
	}
	if errs := lintErrors(t, r.Root); len(errs) != 0 {
		t.Errorf("lint after new: %v", errs)
	}

	// The repo's own template is used when present.
	tmpl, err := os.ReadFile("../../specs/_template.md")
	if err != nil {
		t.Fatal(err)
	}
	r.Write("specs/_template.md", strings.Replace(string(tmpl), "## Why", "<!-- repo template -->\n\n## Why", 1))
	got, err = New(r.Load(), "second", "Second", "core")
	if err != nil {
		t.Fatal(err)
	}
	if got.File != "specs/changes/0009-second.md" {
		t.Errorf("second file = %s", got.File)
	}
	text := r.Read(got.File)
	if !strings.Contains(text, "<!-- repo template -->") || !strings.Contains(text, "id: FUR-0009\n") {
		t.Errorf("repo template not used or not filled in:\n%s", text)
	}
	if errs := lintErrors(t, r.Root); len(errs) != 0 {
		t.Errorf("lint after second new: %v", errs)
	}

	var js bytes.Buffer
	if err := json.NewEncoder(&js).Encode(got); err != nil || !strings.Contains(js.String(), `"file":"specs/changes/0009-second.md"`) {
		t.Errorf("JSON = %s (%v)", js.String(), err)
	}
}

// AC-3
func TestNewRefusals(t *testing.T) {
	r := spectest.New(t)
	for _, slug := range []string{"Bad", "has space", "under_score", "-lead", "trail-", "a--b", ""} {
		_, err := New(r.Load(), slug, "T", "core")
		if err == nil || !strings.Contains(err.Error(), "lowercase letters, digits and dashes") {
			t.Errorf("slug %q: err = %v", slug, err)
		}
	}
	_, err := New(r.Load(), "fine", "T", "x")
	if err == nil || err.Error() != "No area 'x'. Create it with `furrow area x --title …`." {
		t.Errorf("unknown area: err = %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(r.Root, "specs/changes")); len(entries) != 0 {
		t.Errorf("refusals wrote files: %v", entries)
	}
}

// AC-4
func TestArea(t *testing.T) {
	r := spectest.New(t)
	file, err := Area(r.Load(), "billing", "Billing and invoices")
	if err != nil {
		t.Fatal(err)
	}
	if file != "specs/areas/billing.md" {
		t.Errorf("file = %s", file)
	}
	repo := r.Load()
	a := repo.Areas["billing"]
	if a == nil || a.Title != "Billing and invoices" {
		t.Fatalf("area not loaded: %+v %v", a, repo.Problems)
	}
	if errs := lintErrors(t, r.Root); len(errs) != 0 {
		t.Errorf("lint after area: %v", errs)
	}
	before := r.Read(file)
	if _, err := Area(r.Load(), "billing", "Again"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("duplicate: err = %v", err)
	}
	if r.Read(file) != before {
		t.Errorf("duplicate area overwrote the file")
	}
	if _, err := Area(r.Load(), "Bad Name", "X"); err == nil {
		t.Errorf("bad name accepted")
	}
}

// AC-5
func TestInvariants(t *testing.T) {
	invs, err := Invariants("../../docs/product.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(invs) != 7 {
		t.Fatalf("got %d invariants, want 7: %v", len(invs), invs)
	}
	for i, inv := range invs {
		if want := "INV-" + string(rune('1'+i)); inv.ID != want {
			t.Errorf("invariant %d is %s, want %s", i, inv.ID, want)
		}
	}
	if invs[0].Text != "Plain files are the truth" {
		t.Errorf("INV-1 = %q", invs[0].Text)
	}
	var b bytes.Buffer
	WriteInvariants(&b, invs)
	if !strings.HasPrefix(b.String(), "INV-1  Plain files are the truth\nINV-2  Git is enough\n") || strings.Count(b.String(), "\n") != 7 {
		t.Errorf("text output:\n%s", b.String())
	}
	data, _ := json.Marshal(invs)
	if !strings.HasPrefix(string(data), `[{"id":"INV-1","text":"Plain files are the truth"}`) {
		t.Errorf("JSON = %s", data)
	}
	if _, err := Invariants(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Errorf("a missing product file should be an error")
	}
}
