// Package importer moves a repo's specs onto Furrow from the format it used before: furrow import.
// It reads and writes plain files and never runs git.
package importer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/scaffold"
	"github.com/devenjarvis/furrow/internal/spec"
	"gopkg.in/yaml.v3"
)

const dateLayout = "2006-01-02"

// Refusals, word for word from the spec.
const (
	NoConfigFmt = "No .furrow.yml here. Write one first: `furrow init --prefix %s`."
	NoAreas     = "Name the areas: --areas FILE (see `furrow import --help`)"
	NoBy        = "Say who signed off the imported human: checks: --by NAME."
)

// AreasHelp describes the --areas file, for furrow import --help.
const AreasHelp = `The --areas file is YAML:

  areas:
    - area: line
      title: Your line
      paths: [Packages/FetchCore/Sources/FetchCore/Line/**, App/Sources/Features/Line/**]
  specs:            # optional: spec ids whose area the paths don't decide
    FETCH-0089: line

A spec's area is its entry under specs:, else the first area, in file order, covering its first
touches entry that any area covers. An area covers an entry when one of its paths' literal prefix
(the folders before the first wildcard) is a prefix of the entry.`

// Options is one import. Root holds .furrow.yml; AreasFile is relative to Root.
type Options struct {
	Root      string
	Config    config.Config
	AreasFile string
	By        string
	Now       time.Time
}

// Plan is what an import would do. Nothing is written until Write.
type Plan struct {
	Specs    []PlannedSpec `json:"specs"`
	Areas    []PlannedArea `json:"areas"`
	Problems []string      `json:"problems"`

	root    string
	specs   string // the specs folder, repo-relative
	writes  []fileWrite
	removes []string
}

// PlannedSpec is one Fetch spec and where it goes.
type PlannedSpec struct {
	ID       string   `json:"id"`
	From     string   `json:"from"`
	To       string   `json:"to"`
	Status   string   `json:"status"`
	Area     string   `json:"area"`
	Merged   bool     `json:"merged"`
	Signoffs []string `json:"signoffs"`
}

// PlannedArea is one area file: created, updated with History lines, or kept as it is.
type PlannedArea struct {
	Area    string   `json:"area"`
	Title   string   `json:"title"`
	File    string   `json:"file"`
	Action  string   `json:"action"` // create | update | keep
	History []string `json:"history"`
}

type fileWrite struct {
	rel, text string
}

func (p *Plan) spec(id string) *PlannedSpec {
	for i := range p.Specs {
		if p.Specs[i].ID == id {
			return &p.Specs[i]
		}
	}
	return nil
}

type areasFile struct {
	Areas []struct {
		Area  string   `yaml:"area"`
		Title string   `yaml:"title"`
		Paths []string `yaml:"paths"`
	} `yaml:"areas"`
	Specs map[string]string `yaml:"specs"`
}

// source is one Fetch spec as read, before it's planned.
type source struct {
	file  string // base name
	num   int
	id    string
	front fetchFront
	body  string
	crit  []*spec.Criterion
	dir   string // "archive" or "changes"
	plan  PlannedSpec
}

var (
	fetchFile  = regexp.MustCompile(`^(\d{4})-.+\.md$`)
	furrowFile = regexp.MustCompile(`^\d{4}-[a-z0-9-]+\.md$`)
	slugRe     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	criterion  = regexp.MustCompile(`^- \[[ xX]\] AC-\d+:`)
)

var statusMap = map[string]string{"done": spec.Done, "ready": spec.Accepted, "draft": spec.Draft, "blocked": spec.Paused}

// FetchDir plans an import of the repo at root, whose .furrow.yml must already exist.
func FetchDir(root, areas, by string, now time.Time) (*Plan, error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf(NoConfigFmt, "FETCH")
	}
	if err != nil {
		return nil, err
	}
	return Fetch(Options{Root: root, Config: cfg, AreasFile: areas, By: by, Now: now})
}

// Fetch plans the import of every specs/NNNN-*.md in Fetch's format. A refusal comes back as an
// error; anything else wrong is in the plan's Problems.
func Fetch(o Options) (*Plan, error) {
	if strings.TrimSpace(o.AreasFile) == "" {
		return nil, errors.New(NoAreas)
	}
	af, err := readAreas(filepath.Join(o.Root, o.AreasFile))
	if err != nil {
		return nil, err
	}
	specsDir := filepath.ToSlash(filepath.Clean(o.Config.Specs))
	p := &Plan{Specs: []PlannedSpec{}, Areas: []PlannedArea{}, Problems: []string{}, root: o.Root, specs: specsDir}
	problem := func(format string, args ...any) { p.Problems = append(p.Problems, fmt.Sprintf(format, args...)) }

	entries, err := os.ReadDir(filepath.Join(o.Root, specsDir))
	if err != nil {
		return nil, err
	}
	var srcs []*source
	for _, e := range entries {
		m := fetchFile.FindStringSubmatch(e.Name())
		if e.IsDir() || m == nil {
			continue
		}
		rel := specsDir + "/" + e.Name()
		data, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		front, body, err := parseFetch(string(data))
		if err != nil {
			problem("%s: %v", rel, err)
			continue
		}
		s := &source{file: e.Name(), id: front.scalar("id"), front: front, body: body}
		s.num, _ = strconv.Atoi(m[1])
		status, ok := statusMap[front.scalar("status")]
		if !ok {
			problem("%s: status must be draft, ready, blocked or done, found %q", rel, front.scalar("status"))
			continue
		}
		if !furrowFile.MatchString(e.Name()) {
			problem("%s: Furrow reads only names like NNNN-lower-case-slug.md. Rename it in Fetch first.", rel)
		}
		parsed, err := spec.ParseSpec([]byte("---\n{}\n---\n" + body))
		if err != nil {
			problem("%s: %v", rel, err)
			continue
		}
		s.crit = parsed.Criteria
		s.dir = "changes"
		if status == spec.Done {
			s.dir = "archive"
		}
		s.plan = PlannedSpec{ID: s.id, From: rel, To: specsDir + "/" + s.dir + "/" + e.Name(), Status: status, Signoffs: []string{}}
		srcs = append(srcs, s)
	}
	if len(srcs) == 0 && len(p.Problems) == 0 {
		return nil, fmt.Errorf("No %s/NNNN-*.md specs here to import.", specsDir)
	}
	sort.SliceStable(srcs, func(i, j int) bool { return srcs[i].file < srcs[j].file })

	// Sign-offs, and the --by refusal before anything else is planned.
	for _, s := range srcs {
		for _, c := range sortedCriteria(s.crit) {
			if c.Proof == "human" && c.Ticked {
				s.plan.Signoffs = append(s.plan.Signoffs, c.ID)
			}
		}
		if len(s.plan.Signoffs) > 0 && strings.TrimSpace(o.By) == "" {
			return nil, errors.New(NoBy)
		}
	}
	by := strings.TrimSpace(o.By)
	date := o.Now.Format(dateLayout)

	moved := map[string]string{} // base name -> "archive" or "changes"
	for _, s := range srcs {
		moved[s.file] = s.dir
	}

	areaTitles, areaOrder := map[string]string{}, []string{}
	for _, a := range af.Areas {
		areaTitles[a.Area] = a.Title
		areaOrder = append(areaOrder, a.Area)
	}

	var texts []fileWrite
	for _, s := range srcs {
		pl := &s.plan
		// Status checks.
		if pl.Status == spec.Done {
			for _, c := range sortedCriteria(s.crit) {
				if !c.Ticked {
					problem("%s is done but %s isn't ticked. Tick it, or set the spec back to ready, in Fetch's format.", s.id, c.ID)
				}
			}
		}
		switch pl.Status {
		case spec.Done:
			pl.Merged = true
		case spec.Accepted:
			pl.Merged = true
			for _, c := range s.crit {
				if c.Proof != "human" && !c.Ticked {
					pl.Merged = false
				}
			}
		}
		// Area.
		if name, ok := af.Specs[s.id]; ok {
			pl.Area = name
			if _, known := areaTitles[name]; !known && !exists(o.Root, specsDir+"/areas/"+name+".md") {
				problem("%s is listed under specs: as %q, which isn't under areas: and has no %s/areas/%s.md.", s.id, name, specsDir, name)
			}
		} else {
			pl.Area = pickArea(af, s.front.list("touches"), o.Config.Aliases)
			if pl.Area == "" {
				problem("%s matches no area. Add one of its touches to an area's paths, or list it under specs:", s.id)
			}
		}
		if exists(o.Root, pl.To) {
			problem("%s already exists. Move it out of the way before importing %s.", pl.To, s.id)
		}
		texts = append(texts, fileWrite{rel: pl.To, text: furrowText(s, moved, specsDir, by, date)})
		p.Specs = append(p.Specs, *pl)
		p.removes = append(p.removes, pl.From)
	}

	// Areas: History lines for archived specs, in number order.
	history := map[string][]string{}
	for _, s := range srcs {
		if s.dir == "archive" && s.plan.Area != "" {
			history[s.plan.Area] = append(history[s.plan.Area], fmt.Sprintf("- %s: %s (archived %s, imported)", s.id, title(s), date))
		}
	}
	seenArea := map[string]bool{}
	var extra []string
	for _, s := range srcs {
		if a := s.plan.Area; a != "" && !seenArea[a] {
			seenArea[a] = true
			if _, listed := areaTitles[a]; !listed {
				extra = append(extra, a)
			}
		}
	}
	sort.Strings(extra)
	for _, name := range append(areaOrder, extra...) {
		rel := specsDir + "/areas/" + name + ".md"
		pa := PlannedArea{Area: name, Title: areaTitles[name], File: rel, History: history[name]}
		if pa.History == nil {
			pa.History = []string{}
		}
		data, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(rel)))
		switch {
		case err == nil:
			text := string(data)
			for _, l := range pa.History {
				text = appendToSection(text, "History", l)
			}
			pa.Action = "keep"
			if len(pa.History) > 0 {
				pa.Action = "update"
				texts = append(texts, fileWrite{rel: rel, text: text})
			}
		case errors.Is(err, fs.ErrNotExist):
			if pa.Title == "" {
				continue // reported above
			}
			pa.Action = "create"
			texts = append(texts, fileWrite{rel: rel, text: newArea(name, pa.Title, pa.History)})
		default:
			return nil, err
		}
		p.Areas = append(p.Areas, pa)
	}

	// The rest of the tree: Furrow's README and template, and the folders.
	readme, tmpl, err := furrowStandard(o.Config.Prefix)
	if err != nil {
		return nil, err
	}
	texts = append(texts,
		fileWrite{rel: specsDir + "/README.md", text: readme},
		fileWrite{rel: specsDir + "/_template.md", text: tmpl},
	)
	for _, d := range []string{"changes", "archive", "areas"} {
		rel := specsDir + "/" + d + "/.gitkeep"
		if !exists(o.Root, rel) {
			texts = append(texts, fileWrite{rel: rel})
		}
	}
	p.writes = texts

	lintProblems, err := p.lint(o.Config)
	if err != nil {
		return nil, err
	}
	p.Problems = append(p.Problems, lintProblems...)
	return p, nil
}

func readAreas(path string) (areasFile, error) {
	var af areasFile
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return af, fmt.Errorf("No areas file at %s. Name the areas: --areas FILE (see `furrow import --help`)", path)
		}
		return af, err
	}
	if err := yaml.Unmarshal(data, &af); err != nil {
		return af, fmt.Errorf("%s: %v", filepath.Base(path), err)
	}
	seen := map[string]bool{}
	for i, a := range af.Areas {
		switch {
		case !slugRe.MatchString(a.Area):
			return af, fmt.Errorf("%s: area %d's name %q must be lowercase letters, digits and dashes.", filepath.Base(path), i+1, a.Area)
		case strings.TrimSpace(a.Title) == "":
			return af, fmt.Errorf("%s: area %q needs a title.", filepath.Base(path), a.Area)
		case seen[a.Area]:
			return af, fmt.Errorf("%s: area %q is listed twice.", filepath.Base(path), a.Area)
		}
		seen[a.Area] = true
	}
	return af, nil
}

// pickArea is the first area, in file order, covering the first touches entry any area covers.
func pickArea(af areasFile, touches []string, aliases map[string]string) string {
	for _, t := range touches {
		entry := paths.Literal(paths.Resolve(t, aliases))
		for _, a := range af.Areas {
			for _, ap := range a.Paths {
				if isPrefix(paths.Literal(paths.Resolve(ap, aliases)), entry) {
					return a.Area
				}
			}
		}
	}
	return ""
}

func isPrefix(prefix, of []string) bool {
	if len(prefix) > len(of) {
		return false
	}
	for i := range prefix {
		if prefix[i] != of[i] {
			return false
		}
	}
	return true
}

func sortedCriteria(cs []*spec.Criterion) []*spec.Criterion {
	out := append([]*spec.Criterion{}, cs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out
}

func title(s *source) string { return s.front.scalar("title") }

func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// rewriteEntry points "specs/NNNN-slug.md" (and any #anchor after it) at the spec's new path.
func rewriteEntry(entry, specsDir string, moved map[string]string) string {
	file, anchor, _ := strings.Cut(entry, "#")
	if dir, base := filepath.ToSlash(filepath.Dir(file)), filepath.Base(file); dir == specsDir {
		if to, ok := moved[base]; ok {
			out := specsDir + "/" + to + "/" + base
			if strings.Contains(entry, "#") {
				out += "#" + anchor
			}
			return out
		}
	}
	return entry
}

// furrowText is the spec in Furrow's format: new front matter, the body with links and sign-offs.
func furrowText(s *source, moved map[string]string, specsDir, by, date string) string {
	f := s.front
	own := specsDir + "/" + s.file
	var touches []string
	for _, t := range f.list("touches") {
		if strings.TrimSpace(t) == own {
			continue
		}
		touches = append(touches, rewriteEntry(t, specsDir, moved))
	}
	var context []string
	for _, c := range f.list("context") {
		context = append(context, rewriteEntry(c, specsDir, moved))
	}
	var covers []string
	for _, c := range sortedCriteria(s.crit) {
		covers = append(covers, c.ID)
	}

	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("---")
	line("id: %s", yamlString(s.id))
	line("title: %s", yamlString(title(s)))
	line("status: %s", s.plan.Status)
	if _, ok := f.values["kind"]; ok {
		line("kind: %s", yamlString(f.scalar("kind")))
	}
	line("area: %s", yamlString(s.plan.Area))
	line("priority: %s", yamlString(f.scalar("priority")))
	line("human_review: %s", yamlString(f.scalar("human_review")))
	line("invariants: []")
	blockList(&b, "context", context, "")
	known := map[string]bool{"id": true, "title": true, "status": true, "kind": true, "priority": true, "size": true,
		"depends_on": true, "touches": true, "context": true, "human_review": true, "area": true, "invariants": true, "steps": true}
	for _, k := range f.keys {
		if known[k] {
			continue
		}
		if f.lists[k] {
			line("%s: %s", k, inlineList(f.list(k)))
		} else {
			line("%s: %s", k, yamlString(f.scalar(k)))
		}
	}
	line("steps:")
	line("  - n: 1")
	line("    title: %s", yamlString(title(s)))
	line("    after: %s", inlineList(f.list("depends_on")))
	blockList(&b, "touches", touches, "    ")
	line("    covers: %s", inlineList(covers))
	line("    size: %s", yamlString(f.scalar("size")))
	line("    merged: %v", s.plan.Merged)
	line("---")

	body := rewriteLinks(s.body, s.dir, specsDir, moved)
	for _, id := range s.plan.Signoffs {
		body = appendToSection(body, "Decisions", fmt.Sprintf("- %s: %s signed off by %s (imported).", date, id, by))
	}
	return b.String() + body
}

func blockList(b *strings.Builder, key string, items []string, indent string) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s%s: []\n", indent, key)
		return
	}
	fmt.Fprintf(b, "%s%s:\n", indent, key)
	for _, it := range items {
		fmt.Fprintf(b, "%s  - %s\n", indent, yamlString(it))
	}
}

func inlineList(items []string) string {
	q := make([]string, len(items))
	for i, it := range items {
		q[i] = yamlString(it)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// yamlString renders s as a YAML scalar, quoting it only when it must.
func yamlString(s string) string {
	data, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(string(data), "\n")
}

var linkRe = regexp.MustCompile(`(^|[^A-Za-z0-9_./-])((?:[A-Za-z0-9_-]+/|\./)?)(\d{4}-[A-Za-z0-9-]+\.md)`)

// rewriteLinks points links to imported specs at their new paths. "specs/NNNN-slug.md" becomes
// "specs/<dir>/NNNN-slug.md"; a relative "NNNN-slug.md" or "./NNNN-slug.md" becomes relative to
// the spec's new folder. Criterion lines are never changed (INV-4).
func rewriteLinks(body, fromDir, specsDir string, moved map[string]string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if criterion.MatchString(l) {
			continue
		}
		lines[i] = linkRe.ReplaceAllStringFunc(l, func(m string) string {
			sm := linkRe.FindStringSubmatch(m)
			lead, prefix, base := sm[1], sm[2], sm[3]
			to, ok := moved[base]
			if !ok {
				return m
			}
			switch prefix {
			case specsDir + "/":
				return lead + specsDir + "/" + to + "/" + base
			case "", "./":
				if to == fromDir {
					return m
				}
				return lead + "../" + to + "/" + base
			}
			return m
		})
	}
	return strings.Join(lines, "\n")
}

// appendToSection adds a line at the end of "## name", or a new section at the end. It mirrors
// the edit furrow signoff and archive make.
func appendToSection(text, name, line string) string {
	newline := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	start := 0
	if strings.HasPrefix(text, "---\n") {
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				start = i + 1
				break
			}
		}
	}
	h := -1
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") && strings.TrimSpace(lines[i][3:]) == name {
			h = i
			break
		}
	}
	if h < 0 {
		if !newline {
			text += "\n"
		}
		return text + "\n## " + name + "\n\n" + line + "\n"
	}
	end := len(lines)
	for i := h + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	last := -1
	for i := h + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			last = i
		}
	}
	var add []string
	at := last + 1
	switch {
	case last < 0 && h+1 < end:
		at, add = h+2, []string{line}
	case last < 0:
		at, add = h+1, []string{"", line}
	case strings.HasPrefix(strings.TrimSpace(lines[last]), "- "):
		add = []string{line}
	default:
		add = []string{"", line}
	}
	if last < 0 && at == end && end < len(lines) {
		add = append(add, "")
	}
	if at > len(lines) {
		at = len(lines)
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, add...)
	out = append(out, lines[at:]...)
	result := strings.Join(out, "\n")
	if newline {
		result += "\n"
	}
	return result
}

func newArea(name, title string, history []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\narea: %s\ntitle: %s\n---\n\n# %s\n\n## Behaviour\n\nTo write: how %s works now.\n\n## History\n",
		name, yamlString(title), title, title)
	if len(history) > 0 {
		b.WriteString("\n" + strings.Join(history, "\n") + "\n")
	}
	return b.String()
}

// furrowStandard is the README and template furrow init writes, with the prefix filled in.
func furrowStandard(prefix string) (string, string, error) {
	dir, err := os.MkdirTemp("", "furrow-import-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(dir)
	if _, err := scaffold.Init(dir, prefix); err != nil {
		return "", "", err
	}
	readme, err := os.ReadFile(filepath.Join(dir, "specs", "README.md"))
	if err != nil {
		return "", "", err
	}
	tmpl, err := os.ReadFile(filepath.Join(dir, "specs", "_template.md"))
	if err != nil {
		return "", "", err
	}
	return string(readme), string(tmpl), nil
}

// lint builds the planned tree in a temp folder, next to the repo's top-level folders and product
// file, and returns what furrow lint would report as errors there.
func (p *Plan) lint(cfg config.Config) ([]string, error) {
	tmp, err := os.MkdirTemp("", "furrow-import-lint-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	entries, err := os.ReadDir(p.root)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != p.specs {
			if err := os.MkdirAll(filepath.Join(tmp, e.Name()), 0o755); err != nil {
				return nil, err
			}
		}
	}
	copyIn := func(rel string) error {
		data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(rel)))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		return writeAt(tmp, rel, string(data))
	}
	if err := copyIn(cfg.Product); err != nil {
		return nil, err
	}
	for _, d := range []string{"changes", "archive", "areas"} {
		es, err := os.ReadDir(filepath.Join(p.root, p.specs, d))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		for _, e := range es {
			if !e.IsDir() {
				if err := copyIn(p.specs + "/" + d + "/" + e.Name()); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, w := range p.writes {
		if err := writeAt(tmp, w.rel, w.text); err != nil {
			return nil, err
		}
	}
	repo, err := spec.Load(tmp, cfg)
	if err != nil {
		return nil, err
	}
	noArea := map[string]bool{}
	for _, s := range p.Specs {
		if s.Area == "" {
			noArea[s.To] = true
		}
	}
	var out []string
	for _, pr := range spec.Lint(repo) {
		if pr.Warning || (noArea[pr.File] && strings.HasPrefix(pr.Message, "area is empty")) {
			continue
		}
		out = append(out, strings.TrimPrefix(pr.String(), "error: "))
	}
	return out, nil
}

func writeAt(root, rel, text string) error {
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(text), 0o644)
}

// Write writes the plan: new files first, then removes the old ones. It refuses, writing nothing,
// while the plan has problems.
func (p *Plan) Write() error {
	if len(p.Problems) > 0 {
		return fmt.Errorf("Nothing written: the plan has %d problem(s). Fix the source specs, in Fetch's format, first.", len(p.Problems))
	}
	for _, w := range p.writes {
		if err := writeAt(p.root, w.rel, w.text); err != nil {
			return err
		}
	}
	for _, rel := range p.removes {
		if err := os.Remove(filepath.Join(p.root, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	return nil
}
