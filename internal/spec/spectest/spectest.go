// Package spectest builds small Furrow repos in temp dirs for tests. Every package's tests can use
// it: specs default to valid, so a test changes only what it's about.
package spectest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/spec"
)

// Config is the .furrow.yml every fixture repo starts with.
const Config = "prefix: FUR\ncheck: true\naliases:\n  cli: cmd/furrow\n"

// Product is the product file every fixture repo starts with: two invariants.
const Product = "# Test\n\n## Invariants\n\n- **INV-1: Files are the truth.** Text.\n- **INV-2: Git is enough.** Text.\n"

// Repo is a fixture repo on disk.
type Repo struct {
	T    testing.TB
	Root string
}

// New makes a repo with the config, the product file, an area "core", and the folders
// internal/ and cmd/furrow/ so default touches pass lint.
func New(t testing.TB) *Repo {
	t.Helper()
	r := &Repo{T: t, Root: t.TempDir()}
	r.Write(".furrow.yml", Config)
	r.Write("docs/product.md", Product)
	r.Area("core")
	for _, d := range []string{"internal", "cmd/furrow", "specs/changes", "specs/archive"} {
		if err := os.MkdirAll(filepath.Join(r.Root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

// Write writes a file, making its folders.
func (r *Repo) Write(rel, content string) {
	r.T.Helper()
	full := filepath.Join(r.Root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.T.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.T.Fatal(err)
	}
}

// Read returns a file's content.
func (r *Repo) Read(rel string) string {
	r.T.Helper()
	data, err := os.ReadFile(filepath.Join(r.Root, rel))
	if err != nil {
		r.T.Fatal(err)
	}
	return string(data)
}

// Area writes a valid area file.
func (r *Repo) Area(name string) {
	r.Write("specs/areas/"+name+".md", AreaText(name))
}

// AreaText is a valid area file.
func AreaText(name string) string {
	return fmt.Sprintf("---\narea: %s\ntitle: %s\n---\n\n# %s\n\nWhat it is.\n\n## Behaviour\n\nHow it works.\n\n## History\n", name, name, name)
}

// Step describes one step of a fixture spec.
type Step struct {
	Title   string
	After   []string
	Touches []string // default: internal/s<num>-<n>/**
	Covers  []string // default: every criterion, on step 1 only
	Size    string   // default: s
	Merged  bool
	Blocked string
}

// Spec describes a fixture spec. Zero values become valid defaults.
type Spec struct {
	Num         int
	Slug        string // default: spec-<num>
	ID          string // default: FUR-<num>
	Title       string // default: Spec <num>
	Status      string // default: accepted (done when Archived)
	Area        string // default: core
	Priority    string // default: p1
	HumanReview string // default: optional, or required with Invariants
	Invariants  []string
	Steps       []Step   // default: one step
	Criteria    []string // full lines; default: one unticked test criterion
	Decisions   string   // lines under ## Decisions
	BeforeAfter string   // the ## Before and after section's text; default: a valid mock, "-" for none
	Body        string   // replaces the whole body when set
	Archived    bool
}

// BeforeAfter is the ## Before and after text a fixture spec has by default: a mock lint accepts.
const BeforeAfter = "**Before:** It doesn't.\n\n```\n$ x\nnothing\n```\n\n**After:** It does.\n\n```\n$ x\nsomething\n```\n"

// Path is where the spec is written.
func (s Spec) Path() string {
	dir := "changes"
	if s.Archived {
		dir = "archive"
	}
	slug := s.Slug
	if slug == "" {
		slug = fmt.Sprintf("spec-%d", s.Num)
	}
	return fmt.Sprintf("specs/%s/%04d-%s.md", dir, s.Num, slug)
}

// Spec writes a spec and returns its path.
func (r *Repo) Spec(s Spec) string {
	r.T.Helper()
	r.Write(s.Path(), Text(s))
	return s.Path()
}

// list renders a YAML flow list, quoting items that YAML would read as something else (`**`,
// `{a,b}`).
func list(items []string) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it
		if strings.ContainsAny(it, ",{}[]") || strings.HasPrefix(it, "*") {
			out[i] = strconv.Quote(it)
		}
	}
	return "[" + strings.Join(out, ", ") + "]"
}

// Text renders a fixture spec.
func Text(s Spec) string {
	if s.ID == "" {
		s.ID = fmt.Sprintf("FUR-%04d", s.Num)
	}
	if s.Title == "" {
		s.Title = fmt.Sprintf("Spec %d", s.Num)
	}
	if s.Status == "" {
		s.Status = "accepted"
		if s.Archived {
			s.Status = "done"
		}
	}
	if s.Area == "" {
		s.Area = "core"
	}
	if s.Priority == "" {
		s.Priority = "p1"
	}
	if s.HumanReview == "" {
		s.HumanReview = "optional"
		if len(s.Invariants) > 0 {
			s.HumanReview = "required"
		}
	}
	if s.Criteria == nil {
		box := " "
		if s.Archived {
			box = "x"
		}
		s.Criteria = []string{fmt.Sprintf("- [%s] AC-1: It works (test: TestWorks)", box)}
	}
	if s.Steps == nil {
		s.Steps = []Step{{Merged: s.Archived}}
	}
	var ids []string
	for _, c := range s.Criteria {
		if i := strings.Index(c, "AC-"); i >= 0 {
			if j := strings.Index(c[i:], ":"); j > 0 {
				ids = append(ids, c[i:i+j])
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\ntitle: %s\nstatus: %s\narea: %s\npriority: %s\nhuman_review: %s\ninvariants: %s\ncontext: []\nsteps:\n",
		s.ID, s.Title, s.Status, s.Area, s.Priority, s.HumanReview, list(s.Invariants))
	for i, st := range s.Steps {
		n := i + 1
		if st.Title == "" {
			st.Title = fmt.Sprintf("Step %d", n)
		}
		if st.Touches == nil {
			st.Touches = []string{fmt.Sprintf("internal/s%d-%d/**", s.Num, n)}
		}
		if st.Covers == nil {
			if n == 1 {
				st.Covers = ids
			} else {
				st.Covers = []string{}
			}
		}
		if st.Size == "" {
			st.Size = "s"
		}
		fmt.Fprintf(&b, "  - n: %d\n    title: %s\n    after: %s\n    touches: %s\n    covers: %s\n    size: %s\n    merged: %v\n",
			n, st.Title, list(st.After), list(st.Touches), list(st.Covers), st.Size, st.Merged)
		if st.Blocked != "" {
			fmt.Fprintf(&b, "    blocked: %q\n", st.Blocked)
		}
	}
	b.WriteString("---\n\n")
	if s.Body != "" {
		b.WriteString(s.Body)
		return b.String()
	}
	switch s.BeforeAfter {
	case "":
		s.BeforeAfter = BeforeAfter
	case "-":
		s.BeforeAfter = ""
	}
	if s.BeforeAfter != "" {
		s.BeforeAfter = "## Before and after\n\n" + strings.TrimRight(s.BeforeAfter, "\n") + "\n\n"
	}
	fmt.Fprintf(&b, "# %s: %s\n\n## Why\n\nBecause.\n\n## What\n\nThis.\n\n%s## Acceptance criteria\n\n%s\n\n## Out of scope\n\n- That.\n\n## Decisions\n\n%s",
		s.ID, s.Title, s.BeforeAfter, strings.Join(s.Criteria, "\n"), s.Decisions)
	if s.Decisions != "" && !strings.HasSuffix(s.Decisions, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// Load reads the repo the way furrow does.
func (r *Repo) Load() *spec.Repo {
	r.T.Helper()
	root, cfg, err := config.Find(r.Root)
	if err != nil {
		r.T.Fatal(err)
	}
	repo, err := spec.Load(root, cfg)
	if err != nil {
		r.T.Fatal(err)
	}
	return repo
}
