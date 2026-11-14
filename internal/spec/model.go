// Package spec reads a repo's specs, areas and invariants into one model, and lints them.
package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/devenjarvis/furrow/internal/config"
	"gopkg.in/yaml.v3"
)

// Statuses a spec may have in specs/changes/. Archived specs are always "done".
const (
	Draft    = "draft"
	Accepted = "accepted"
	Paused   = "paused"
	Done     = "done"
)

// Repo is everything Furrow knows about one repo, read from its files.
type Repo struct {
	Root       string
	Config     config.Config
	Specs      []*Spec // changes and archive, sorted by number
	Areas      map[string]*Area
	Invariants []Invariant
	Problems   []Problem // files that couldn't be read at all
	byID       map[string]*Spec
}

// Spec is one file in specs/changes/ or specs/archive/.
type Spec struct {
	ID          string   `yaml:"id"`
	Title       string   `yaml:"title"`
	Status      string   `yaml:"status"`
	Area        string   `yaml:"area"`
	Priority    string   `yaml:"priority"`
	HumanReview string   `yaml:"human_review"`
	Imported    string   `yaml:"imported"` // fetch or edition: written by furrow import
	Invariants  []string `yaml:"invariants"`
	Context     []string `yaml:"context"`
	Steps       []*Step  `yaml:"steps"`

	Num      int          `yaml:"-"` // from the file name
	File     string       `yaml:"-"` // repo-relative, with forward slashes
	Archived bool         `yaml:"-"`
	Body     string       `yaml:"-"`
	Sections []string     `yaml:"-"`
	Criteria []*Criterion `yaml:"-"`
}

// Step is one PR's worth of a spec.
type Step struct {
	N       int      `yaml:"n"`
	Title   string   `yaml:"title"`
	After   []string `yaml:"after"`
	Touches []string `yaml:"touches"`
	Covers  []string `yaml:"covers"`
	Size    string   `yaml:"size"`
	Merged  bool     `yaml:"merged"`
	Blocked string   `yaml:"blocked"`

	Spec *Spec `yaml:"-"`
}

// Criterion is one "- [ ] AC-n: text (proof: detail)" line.
type Criterion struct {
	ID     string // "AC-3"
	Num    int
	Ticked bool
	Text   string // everything after "AC-n: "
	Proof  string // test, check or human; "" when missing
	Detail string
}

// Area is a living spec in specs/areas/.
type Area struct {
	Name     string `yaml:"area"`
	Title    string `yaml:"title"`
	File     string `yaml:"-"`
	Stem     string `yaml:"-"`
	Sections []string
}

// Invariant is one **INV-n: name.** rule in the product file.
type Invariant struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Problem is one lint finding.
type Problem struct {
	File    string
	Message string
	Warning bool
}

func (p Problem) String() string {
	kind := "error"
	if p.Warning {
		kind = "warning"
	}
	if p.File == "" {
		return fmt.Sprintf("%s: %s", kind, p.Message)
	}
	return fmt.Sprintf("%s: %s: %s", kind, p.File, p.Message)
}

// Ref names a step (Step > 0) or a whole spec (Step == 0).
type Ref struct {
	Spec string
	Step int
}

func (r Ref) String() string {
	if r.Step == 0 {
		return r.Spec
	}
	return fmt.Sprintf("%s.%d", r.Spec, r.Step)
}

// Ref is the step's own ref, such as FUR-0002.1.
func (s *Step) Ref() Ref { return Ref{Spec: s.Spec.ID, Step: s.N} }

var specFile = regexp.MustCompile(`^(\d{4})-[a-z0-9-]+\.md$`)

// Load reads the whole repo described by cfg at root.
func Load(root string, cfg config.Config) (*Repo, error) {
	r := &Repo{Root: root, Config: cfg, Areas: map[string]*Area{}, byID: map[string]*Spec{}}
	for _, dir := range []string{"changes", "archive"} {
		if err := r.loadSpecs(dir); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(r.Specs, func(i, j int) bool {
		if r.Specs[i].Num != r.Specs[j].Num {
			return r.Specs[i].Num < r.Specs[j].Num
		}
		return r.Specs[i].File < r.Specs[j].File
	})
	for _, s := range r.Specs {
		if _, dup := r.byID[s.ID]; !dup {
			r.byID[s.ID] = s
		}
	}
	if err := r.loadAreas(); err != nil {
		return nil, err
	}
	r.loadInvariants()
	return r, nil
}

func (r *Repo) rel(parts ...string) string {
	return filepath.ToSlash(filepath.Join(append([]string{r.Config.Specs}, parts...)...))
}

func (r *Repo) loadSpecs(dir string) error {
	full := filepath.Join(r.Root, r.Config.Specs, dir)
	entries, err := os.ReadDir(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		m := specFile.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		rel := r.rel(dir, e.Name())
		data, err := os.ReadFile(filepath.Join(full, e.Name()))
		if err != nil {
			return err
		}
		s, err := ParseSpec(data)
		if err != nil {
			r.Problems = append(r.Problems, Problem{File: rel, Message: err.Error()})
			continue
		}
		s.Num, _ = strconv.Atoi(m[1])
		s.File = rel
		s.Archived = dir == "archive"
		r.Specs = append(r.Specs, s)
	}
	return nil
}

// ParseSpec reads one spec file's front matter and body.
func ParseSpec(data []byte) (*Spec, error) {
	front, body, err := splitFrontMatter(string(data))
	if err != nil {
		return nil, err
	}
	s := &Spec{}
	if err := yaml.Unmarshal([]byte(front), s); err != nil {
		return nil, fmt.Errorf("can't read front matter: %v", err)
	}
	for _, st := range s.Steps {
		if st == nil {
			return nil, fmt.Errorf("can't read front matter: an empty step")
		}
		st.Spec = s
	}
	s.Body = body
	s.Sections = sections(body)
	s.Criteria = criteria(SectionText(body, "Acceptance criteria"))
	return s, nil
}

func splitFrontMatter(text string) (string, string, error) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return "", "", fmt.Errorf("missing front matter (the file must start with ---)")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		if strings.HasSuffix(text, "\n---") {
			return text[4 : len(text)-4], "", nil
		}
		return "", "", fmt.Errorf("front matter isn't closed with ---")
	}
	return text[4 : 4+end], text[4+end+5:], nil
}

var heading = regexp.MustCompile(`^## (.+?)\s*$`)

// sections lists the "## " headings outside fenced code blocks, in order.
func sections(body string) []string {
	var out []string
	code := codeLines(strings.Split(body, "\n"))
	for i, l := range strings.Split(body, "\n") {
		if m := heading.FindStringSubmatch(l); m != nil && !code[i] {
			out = append(out, m[1])
		}
	}
	return out
}

// codeLines reports, for each line, whether it's inside a fenced code block, its fences included.
func codeLines(lines []string) []bool {
	out := make([]bool, len(lines))
	open := ""
	for i, l := range lines {
		f := fence(l)
		switch {
		case open == "" && f != "":
			open = f
		case open != "" && strings.HasPrefix(f, open) && strings.TrimSpace(l) == f:
			open = ""
		case open == "":
			continue
		}
		out[i] = true
	}
	return out
}

// SectionText returns the text under "## name", up to the next "## " heading. Headings inside
// fenced code blocks don't count.
func SectionText(body, name string) string {
	lines := strings.Split(body, "\n")
	code := codeLines(lines)
	var out []string
	in := false
	for i, l := range lines {
		if strings.HasPrefix(l, "## ") && !code[i] {
			if in {
				break
			}
			in = strings.TrimSpace(l[3:]) == name
			continue
		}
		if in {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

var criterionLine = regexp.MustCompile(`^- \[([ xX])\] (AC-(\d+)): (.*)$`)

func criteria(section string) []*Criterion {
	var out []*Criterion
	for _, l := range strings.Split(section, "\n") {
		m := criterionLine.FindStringSubmatch(strings.TrimRight(l, " "))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[3])
		c := &Criterion{ID: m[2], Num: n, Ticked: m[1] != " ", Text: m[4]}
		c.Proof, c.Detail = proof(m[4])
		out = append(out, c)
	}
	return out
}

// proof finds the last "(test: …)", "(check: …)" or "(human: …)" on a line. The detail runs to the
// final ")" so it may hold parentheses of its own.
func proof(text string) (string, string) {
	best, kind := -1, ""
	for _, k := range []string{"test", "check", "human"} {
		if i := strings.LastIndex(text, "("+k+":"); i > best {
			best, kind = i, k
		}
	}
	if best < 0 {
		return "", ""
	}
	detail := strings.TrimSpace(text[best+len(kind)+2:])
	detail = strings.TrimSpace(strings.TrimSuffix(detail, ")"))
	return kind, detail
}

func (r *Repo) loadAreas() error {
	full := filepath.Join(r.Root, r.Config.Specs, "areas")
	entries, err := os.ReadDir(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel := r.rel("areas", e.Name())
		data, err := os.ReadFile(filepath.Join(full, e.Name()))
		if err != nil {
			return err
		}
		front, body, err := splitFrontMatter(string(data))
		a := &Area{File: rel, Stem: strings.TrimSuffix(e.Name(), ".md")}
		if err == nil {
			err = yaml.Unmarshal([]byte(front), a)
		}
		if err != nil {
			r.Problems = append(r.Problems, Problem{File: rel, Message: err.Error()})
			continue
		}
		a.Sections = sections(body)
		r.Areas[a.Stem] = a
	}
	return nil
}

var invariantMark = regexp.MustCompile(`\*\*(INV-\d+)[:.]?\s*([^*]*?)\s*\*\*`)

func (r *Repo) loadInvariants() {
	data, err := os.ReadFile(filepath.Join(r.Root, r.Config.Product))
	if err != nil {
		return
	}
	r.Invariants = ParseInvariants(string(data))
}

// ParseInvariants reads every **INV-n: name.** under "## Invariants".
func ParseInvariants(text string) []Invariant {
	var out []Invariant
	for _, m := range invariantMark.FindAllStringSubmatch(SectionText(text, "Invariants"), -1) {
		out = append(out, Invariant{ID: m[1], Text: strings.TrimSuffix(m[2], ".")})
	}
	return out
}

// Spec returns the spec with this id, or nil.
func (r *Repo) Spec(id string) *Spec { return r.byID[id] }

// Step returns the step a ref names, or nil for a whole-spec ref or an unknown step.
func (r *Repo) Step(ref Ref) *Step {
	s := r.Spec(ref.Spec)
	if s == nil || ref.Step == 0 {
		return nil
	}
	for _, st := range s.Steps {
		if st.N == ref.Step {
			return st
		}
	}
	return nil
}

// ParseRef reads "FUR-0001.2", "FUR-0001" or, relative to spec self, "2".
func (r *Repo) ParseRef(text string, self *Spec) (Ref, error) {
	text = strings.TrimSpace(text)
	if n, err := strconv.Atoi(text); err == nil && self != nil {
		return Ref{Spec: self.ID, Step: n}, nil
	}
	re := regexp.MustCompile(`^(` + regexp.QuoteMeta(r.Config.Prefix) + `-\d{4})(?:\.(\d+))?$`)
	m := re.FindStringSubmatch(text)
	if m == nil {
		return Ref{}, fmt.Errorf("can't read ref %q: use %s-NNNN.N, %s-NNNN or a step number", text, r.Config.Prefix, r.Config.Prefix)
	}
	ref := Ref{Spec: m[1]}
	if m[2] != "" {
		ref.Step, _ = strconv.Atoi(m[2])
	}
	return ref, nil
}

// StepRef reads a ref that must name one step.
func (r *Repo) StepRef(text string) (*Step, error) {
	ref, err := r.ParseRef(text, nil)
	if err != nil {
		return nil, err
	}
	if ref.Step == 0 {
		return nil, fmt.Errorf("%s names a whole spec; name a step, such as %s.1", ref.Spec, ref.Spec)
	}
	st := r.Step(ref)
	if st == nil {
		return nil, fmt.Errorf("no step %s", ref)
	}
	return st, nil
}

// AfterRefs resolves a step's after list. Unreadable entries come back as errors.
func (r *Repo) AfterRefs(st *Step) ([]Ref, []error) {
	var refs []Ref
	var errs []error
	for _, a := range st.After {
		ref, err := r.ParseRef(a, st.Spec)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		refs = append(refs, ref)
	}
	return refs, errs
}

// StepMerged reports whether a step's work is on the base branch.
func StepMerged(st *Step) bool { return st.Merged || st.Spec.Archived }

// RefMerged reports whether a ref is merged: a step, or every step of a whole spec. A whole spec
// with no steps counts as merged only once archived.
func (r *Repo) RefMerged(ref Ref) bool {
	s := r.Spec(ref.Spec)
	if s == nil {
		return false
	}
	if ref.Step == 0 {
		if s.Archived {
			return true
		}
		if len(s.Steps) == 0 {
			return false
		}
		for _, st := range s.Steps {
			if !StepMerged(st) {
				return false
			}
		}
		return true
	}
	st := r.Step(ref)
	return st != nil && StepMerged(st)
}

// Expand turns a ref into the steps it names: one step, or every step of a spec.
func (r *Repo) Expand(ref Ref) []*Step {
	if ref.Step != 0 {
		if st := r.Step(ref); st != nil {
			return []*Step{st}
		}
		return nil
	}
	if s := r.Spec(ref.Spec); s != nil {
		return s.Steps
	}
	return nil
}

// Criterion returns the criterion with this id, or nil.
func (s *Spec) Criterion(id string) *Criterion {
	for _, c := range s.Criteria {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// MergedSteps counts the spec's merged steps.
func (s *Spec) MergedSteps() int {
	n := 0
	for _, st := range s.Steps {
		if StepMerged(st) {
			n++
		}
	}
	return n
}

// Complete reports whether every step is merged and every box ticked.
func (s *Spec) Complete() bool {
	if s.MergedSteps() != len(s.Steps) {
		return false
	}
	for _, c := range s.Criteria {
		if !c.Ticked {
			return false
		}
	}
	return true
}

// DerivedStatus works out what furrow list shows: draft, accepted, building, review, paused or done.
func (s *Spec) DerivedStatus() string {
	if s.Archived || s.Status == Done {
		return Done
	}
	if s.Status != Accepted {
		return s.Status
	}
	merged := s.MergedSteps()
	switch {
	case merged == 0:
		return Accepted
	case merged < len(s.Steps):
		return "building"
	default:
		return "review"
	}
}

// OpenSteps returns the unmerged steps of accepted specs, in spec then step order.
func (r *Repo) OpenSteps() []*Step {
	var out []*Step
	for _, s := range r.Specs {
		if s.Archived || s.Status != Accepted {
			continue
		}
		for _, st := range s.Steps {
			if !StepMerged(st) {
				out = append(out, st)
			}
		}
	}
	return out
}

// LoadDir finds the config above dir and loads the repo.
func LoadDir(dir string) (*Repo, error) {
	root, cfg, err := config.Find(dir)
	if err != nil {
		return nil, err
	}
	return Load(root, cfg)
}
