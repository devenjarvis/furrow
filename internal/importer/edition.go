package importer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/paths"
	"github.com/devenjarvis/furrow/internal/spec"
)

// EditionSpecs is where Edition keeps its specs before the import: one docs/specs/<feature>.md each.
const EditionSpecs = "docs/specs"

// EditionHelp describes what furrow import edition reads, for furrow import --help.
const EditionHelp = `furrow import edition reads docs/specs/<feature>.md, numbered in file-name order. A spec's area
is its entry under specs: (by id or by its old slug), else the first word of its **Area:** line when
that names an area, else its touches.`

// editionSkip are the files in docs/specs that aren't specs. The import removes them too: the
// standard moves to specs/README.md and the template to specs/_template.md.
var editionSkip = map[string]bool{"README.md": true, "TEMPLATE.md": true}

var editionStatus = map[string]string{
	"Shipped": spec.Done, "Building": spec.Accepted, "Accepted": spec.Accepted,
	"Draft": spec.Draft, "Paused": spec.Paused,
}

var (
	edFile      = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*\.md$`)
	edMeta      = regexp.MustCompile(`^- \*\*([A-Za-z ]+):\*\*\s*(.*)$`)
	edCriterion = regexp.MustCompile(`^- \*\*([A-Z])(\d+)\.\*\*\s*(.*)$`)
	edStep      = regexp.MustCompile(`^(\d+)\. \[( |x)\] (.*)$`)
	edStepMeta  = regexp.MustCompile(`^\s+- \*\*(After|Touches|Blocked):\*\*`)
	edField     = regexp.MustCompile(`\*\*(After|Touches|Blocked):\*\*\s*(.*?)\s*(?:·\s*|$)(?:\*\*|$)`)
	edRef       = regexp.MustCompile(`^(?:([a-z0-9-]+) )?(\d+)$`)
	edADR       = regexp.MustCompile(`\bADRs? ((?:\d{4}(?:,? (?:and )?)?)+)`)
	edFour      = regexp.MustCompile(`\d{4}`)
	edSpecLink  = regexp.MustCompile("(^|[^A-Za-z0-9_./-])((?:docs/specs/|\\./)?)([a-z0-9]+(?:-[a-z0-9]+)*)\\.md\\b")
)

// edSource is one Edition spec as read.
type edSource struct {
	file, slug, rel string
	num             int
	id              string
	title           string
	statusLine      string
	meta            []edMetaLine
	sections        []edSection // in file order, "" for text before the first ##
	letter          string
	crits           []edCrit
	steps           []edStepSrc
	dir             string
	plan            PlannedSpec
}

type edMetaLine struct {
	key   string
	lines []string // the bullet and its wrapped lines, as written
}

type edSection struct {
	name  string
	lines []string // body lines, without the heading
}

type edCrit struct {
	label string // "S5"
	n     int    // 5
	text  string // joined onto one line
	line  int    // index in Behavior
	span  int    // lines it took
}

type edStepSrc struct {
	n       int
	done    bool
	text    string   // the step's text, joined
	lines   []string // the step's text lines as written, without its After line
	after   []string // raw refs; nil when no After line
	touches []string
	blocked string
}

func (s *edSource) section(name string) *edSection {
	for i := range s.sections {
		if s.sections[i].name == name {
			return &s.sections[i]
		}
	}
	return nil
}

// EditionDir plans an import of the Edition repo at root, whose .furrow.yml must already exist.
func EditionDir(root, areas, by string, now time.Time) (*Plan, error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf(NoConfigFmt, "EDN")
	}
	if err != nil {
		return nil, err
	}
	return Edition(Options{Root: root, Config: cfg, AreasFile: areas, By: by, Now: now})
}

// Edition plans the import of every docs/specs/<feature>.md in Edition's format. Specs are
// numbered in file-name order. A refusal comes back as an error; anything else wrong is in the
// plan's Problems.
func Edition(o Options) (*Plan, error) {
	if strings.TrimSpace(o.AreasFile) == "" {
		return nil, errors.New(NoAreas)
	}
	af, err := readAreas(areasPath(o.Root, o.AreasFile))
	if err != nil {
		return nil, err
	}
	specsDir := filepath.ToSlash(filepath.Clean(o.Config.Specs))
	p := &Plan{Specs: []PlannedSpec{}, Areas: []PlannedArea{}, Problems: []string{}, root: o.Root, specs: specsDir, source: "Edition"}
	problem := func(format string, args ...any) { p.Problems = append(p.Problems, fmt.Sprintf(format, args...)) }

	entries, err := os.ReadDir(filepath.Join(o.Root, filepath.FromSlash(EditionSpecs)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("No %s/ here to import. Run it at the root of an Edition clone.", EditionSpecs)
	}
	if err != nil {
		return nil, err
	}
	var srcs []*edSource
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		rel := EditionSpecs + "/" + e.Name()
		if editionSkip[e.Name()] {
			p.removes = append(p.removes, rel)
			continue
		}
		if !edFile.MatchString(e.Name()) {
			problem("%s: Furrow reads only names like lower-case-slug.md. Rename it in Edition first.", rel)
			continue
		}
		data, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		s, errs := parseEdition(strings.ReplaceAll(string(data), "\r\n", "\n"))
		for _, e := range errs {
			problem("%s: %s", rel, e)
		}
		if s == nil {
			continue
		}
		s.file, s.rel, s.slug = e.Name(), rel, strings.TrimSuffix(e.Name(), ".md")
		srcs = append(srcs, s)
	}
	if len(srcs) == 0 && len(p.Problems) == 0 {
		return nil, fmt.Errorf("No %s/*.md specs here to import.", EditionSpecs)
	}
	sort.SliceStable(srcs, func(i, j int) bool { return srcs[i].file < srcs[j].file })

	bySlug := map[string]*edSource{}
	moved := map[string]string{} // slug -> new path, repo-relative
	for i, s := range srcs {
		s.num = i + 1
		s.id = fmt.Sprintf("%s-%04d", o.Config.Prefix, s.num)
		status, ok := editionStatus[s.statusLine]
		if !ok {
			problem("%s: the '- **Status:**' line must start with Draft, Accepted, Building, Paused or Shipped, found %q", s.rel, s.statusLine)
			status = spec.Draft
		}
		s.dir = "changes"
		if status == spec.Done {
			s.dir = "archive"
		}
		to := fmt.Sprintf("%s/%s/%04d-%s.md", specsDir, s.dir, s.num, s.slug)
		s.plan = PlannedSpec{ID: s.id, From: s.rel, To: to, Status: status, Signoffs: []string{}}
		bySlug[s.slug] = s
		moved[s.slug] = to
	}

	areaTitles, areaOrder := map[string]string{}, []string{}
	for _, a := range af.Areas {
		areaTitles[a.Area] = a.Title
		areaOrder = append(areaOrder, a.Area)
	}

	var texts []fileWrite
	for _, s := range srcs {
		pl := &s.plan
		steps, stepProblems := edPlanSteps(s, bySlug, o)
		for _, sp := range stepProblems {
			problem("%s (%s): %s", s.id, s.rel, sp)
		}
		pl.Merged = len(steps) > 0
		for _, st := range steps {
			if !st.merged {
				pl.Merged = false
			}
		}
		if pl.Status == spec.Done {
			for _, st := range s.steps {
				if !st.done {
					problem("%s (%s) is Shipped but step %d isn't ticked. Tick it, or set the spec back to Building, in Edition's format.", s.id, s.rel, st.n)
				}
			}
		}
		// Area.
		if name, ok := af.Specs[s.id]; ok {
			pl.Area = name
		} else if name, ok := af.Specs[s.slug]; ok {
			pl.Area = name
		} else if name := edAreaLine(s); areaTitles[name] != "" {
			pl.Area = name
		} else {
			var touches []string
			for _, st := range s.steps {
				touches = append(touches, st.touches...)
			}
			pl.Area = pickArea(af, touches, o.Config.Aliases)
		}
		if pl.Area == "" {
			problem("%s (%s) matches no area. Name its area on the **Area:** line, add one of its touches to an area's paths, or list it under specs:", s.id, s.rel)
		} else if _, known := areaTitles[pl.Area]; !known && !exists(o.Root, specsDir+"/areas/"+pl.Area+".md") {
			problem("%s is listed under specs: as %q, which isn't under areas: and has no %s/areas/%s.md.", s.id, pl.Area, specsDir, pl.Area)
		}
		if exists(o.Root, pl.To) {
			problem("%s already exists. Move it out of the way before importing %s.", pl.To, s.id)
		}
		texts = append(texts, fileWrite{rel: pl.To, text: edText(s, steps, moved, o)})
		p.Specs = append(p.Specs, *pl)
		p.removes = append(p.removes, s.rel)
	}
	sort.Strings(p.removes)

	history := map[string][]string{}
	for _, s := range srcs {
		if s.dir == "archive" && s.plan.Area != "" {
			history[s.plan.Area] = append(history[s.plan.Area], fmt.Sprintf("- %s: %s (archived %s, imported)", s.id, s.title, o.Now.Format(dateLayout)))
		}
	}
	areaWrites, areas, err := planAreas(o.Root, specsDir, areaOrder, areaTitles, p.Specs, history)
	if err != nil {
		return nil, err
	}
	texts = append(texts, areaWrites...)
	p.Areas = areas

	rest, err := standardTree(o.Root, specsDir, o.Config.Prefix)
	if err != nil {
		return nil, err
	}
	p.writes = append(texts, rest...)

	lintProblems, err := p.lint(o.Config)
	if err != nil {
		return nil, err
	}
	p.Problems = append(p.Problems, lintProblems...)
	return p, nil
}

// parseEdition reads the title, the meta bullets, the sections, the criteria and the steps.
func parseEdition(text string) (*edSource, []string) {
	var errs []string
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		return nil, []string{"the first line must be the title, \"# <Feature>\""}
	}
	s := &edSource{title: strings.TrimSpace(lines[0][2:])}
	cur := &edSection{}
	i := 1
	// Meta bullets, each with its wrapped lines, up to the first blank line or heading.
	for ; i < len(lines) && strings.TrimSpace(lines[i]) == ""; i++ {
	}
	for ; i < len(lines); i++ {
		l := lines[i]
		if m := edMeta.FindStringSubmatch(l); m != nil {
			s.meta = append(s.meta, edMetaLine{key: m[1], lines: []string{l}})
			continue
		}
		if len(s.meta) > 0 && strings.HasPrefix(l, " ") && strings.TrimSpace(l) != "" {
			last := &s.meta[len(s.meta)-1]
			last.lines = append(last.lines, l)
			continue
		}
		break
	}
	for _, m := range s.meta {
		if m.key == "Status" {
			v := strings.TrimSpace(strings.TrimPrefix(m.lines[0], "- **Status:**"))
			s.statusLine = regexp.MustCompile(`^\w+`).FindString(v)
		}
	}
	if s.statusLine == "" {
		errs = append(errs, "no '- **Status:**' line under the title")
	}
	for ; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "## ") {
			s.sections = append(s.sections, *cur)
			cur = &edSection{name: strings.TrimSpace(l[3:])}
			continue
		}
		cur.lines = append(cur.lines, l)
	}
	s.sections = append(s.sections, *cur)

	// Criteria, each joined onto one line.
	if b := s.section("Behavior"); b != nil {
		for j := 0; j < len(b.lines); j++ {
			m := edCriterion.FindStringSubmatch(b.lines[j])
			if m == nil {
				continue
			}
			n, _ := strconv.Atoi(m[2])
			c := edCrit{label: m[1] + m[2], n: n, text: strings.TrimSpace(m[3]), line: j, span: 1}
			for j+1 < len(b.lines) && strings.HasPrefix(b.lines[j+1], " ") && strings.TrimSpace(b.lines[j+1]) != "" {
				j++
				c.text += " " + strings.TrimSpace(b.lines[j])
				c.span++
			}
			if s.letter == "" {
				s.letter = m[1]
			} else if m[1] != s.letter {
				errs = append(errs, fmt.Sprintf("criterion %s doesn't use the spec's letter %s", c.label, s.letter))
			}
			s.crits = append(s.crits, c)
		}
	}
	// Steps.
	if st := s.section("Steps"); st != nil {
		var step *edStepSrc
		metaSeen := false
		for _, l := range st.lines {
			if m := edStep.FindStringSubmatch(l); m != nil {
				n, _ := strconv.Atoi(m[1])
				s.steps = append(s.steps, edStepSrc{n: n, done: m[2] == "x", text: m[3], lines: []string{l}})
				step, metaSeen = &s.steps[len(s.steps)-1], false
				continue
			}
			if step == nil || !strings.HasPrefix(l, " ") {
				step = nil
				continue
			}
			if edStepMeta.MatchString(l) {
				metaSeen = true
				for _, f := range splitFields(l) {
					switch f[0] {
					case "After":
						step.after = []string{}
						if v := strings.Trim(f[1], "` "); strings.ToLower(v) != "nothing" && v != "" {
							for _, r := range strings.Split(v, ",") {
								step.after = append(step.after, strings.Trim(strings.TrimSpace(r), "`"))
							}
						}
					case "Touches":
						for _, t := range strings.Split(f[1], ",") {
							if t = strings.Trim(strings.TrimSpace(t), "`"); t != "" {
								step.touches = append(step.touches, t)
							}
						}
					case "Blocked":
						step.blocked = f[1]
						if step.blocked == "" {
							step.blocked = "no reason given"
						}
					}
				}
				continue
			}
			if !metaSeen {
				step.text += " " + strings.TrimSpace(l)
				step.lines = append(step.lines, l)
			}
		}
	}
	return s, errs
}

// splitFields reads "**After:** 1 · **Touches:** a, b · **Blocked:** why" as key, value pairs.
func splitFields(line string) [][2]string {
	var out [][2]string
	for _, part := range regexp.MustCompile(`\s*·\s*(?:\*\*)`).Split(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")), -1) {
		part = strings.TrimPrefix(part, "**")
		key, value, ok := strings.Cut(part, ":**")
		if !ok {
			continue
		}
		out = append(out, [2]string{key, strings.TrimSpace(value)})
	}
	return out
}

// edStatusRest is the Status line after its first word: "after step 1 stage 1 (needs …)".
func edStatusRest(s *edSource) string {
	for _, m := range s.meta {
		if m.key == "Status" {
			v := strings.TrimSpace(strings.TrimPrefix(m.lines[0], "- **Status:**"))
			for _, l := range m.lines[1:] {
				v += " " + strings.TrimSpace(l)
			}
			v = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(v, s.statusLine), ".·:, "))
			if v == "" {
				return "no reason given"
			}
			return strings.TrimSuffix(v, ".")
		}
	}
	return "no reason given"
}

func edAreaLine(s *edSource) string {
	for _, m := range s.meta {
		if m.key == "Area" {
			v := strings.TrimSpace(strings.TrimPrefix(m.lines[0], "- **Area:**"))
			return strings.ToLower(regexp.MustCompile(`^[A-Za-z]+`).FindString(v))
		}
	}
	return ""
}

type edPlannedStep struct {
	n       int
	title   string
	after   []string
	touches []string
	covers  []string
	merged  bool
	blocked string
}

var edTitleTail = regexp.MustCompile(`\s*\([A-Z]\d[^()]*\)\.?\s*$`)

// edPlanSteps turns the step list into Furrow steps: After refs become step refs, Touches become
// globs, and the criteria a step names become its covers.
func edPlanSteps(s *edSource, bySlug map[string]*edSource, o Options) ([]edPlannedStep, []string) {
	var out []edPlannedStep
	var problems []string
	acOf := map[int]string{}
	for i, c := range s.crits {
		acOf[c.n] = fmt.Sprintf("AC-%d", i+1)
	}
	for _, st := range s.steps {
		ps := edPlannedStep{n: st.n, merged: st.done, blocked: st.blocked, after: []string{}, touches: []string{}, covers: []string{}}
		if s.plan.Status == spec.Paused && !st.done && st.after == nil && st.blocked == "" {
			// Edition never asked a paused spec for step lines; the spec's pause is why it waits.
			ps.blocked = "The spec is paused: " + edStatusRest(s) + ". Write its After and Touches when it's unpaused."
		}
		title := strings.TrimSpace(strings.SplitN(st.text, ". ", 2)[0])
		ps.title = strings.TrimSuffix(strings.TrimSpace(edTitleTail.ReplaceAllString(title, "")), ".")
		for _, r := range st.after {
			m := edRef.FindStringSubmatch(r)
			if m == nil {
				problems = append(problems, fmt.Sprintf("step %d: can't read After %q: use `N`, `<spec> N` or `nothing`", st.n, r))
				continue
			}
			if m[1] == "" || m[1] == s.slug {
				ps.after = append(ps.after, m[2])
				continue
			}
			other, ok := bySlug[m[1]]
			if !ok {
				problems = append(problems, fmt.Sprintf("step %d is after %s %s, and there's no %s/%s.md", st.n, m[1], m[2], EditionSpecs, m[1]))
				continue
			}
			ps.after = append(ps.after, other.id+"."+m[2])
		}
		for _, t := range st.touches {
			g, err := edGlob(o.Root, t, o.Config.Aliases)
			if err != "" {
				problems = append(problems, fmt.Sprintf("step %d: %s", st.n, err))
				continue
			}
			ps.touches = append(ps.touches, g)
		}
		seen := map[string]bool{}
		for _, n := range edCovers(st.text, s.letter) {
			ac, ok := acOf[n]
			if !ok {
				problems = append(problems, fmt.Sprintf("step %d names %s%d, which isn't a criterion", st.n, s.letter, n))
				continue
			}
			if !seen[ac] {
				seen[ac] = true
				ps.covers = append(ps.covers, ac)
			}
		}
		sort.SliceStable(ps.covers, func(i, j int) bool { return acNum(ps.covers[i]) < acNum(ps.covers[j]) })
		out = append(out, ps)
	}
	return out, problems
}

func acNum(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "AC-"))
	return n
}

// edCovers finds the criteria of the spec's letter a step's text names: "S4", or a range "J1–J7".
func edCovers(text, letter string) []int {
	if letter == "" {
		return nil
	}
	re := regexp.MustCompile(`\b` + letter + `(\d+)(?:\s*[–-]\s*` + letter + `?(\d+))?\b`)
	var out []int
	for _, m := range re.FindAllStringSubmatch(text, -1) {
		from, _ := strconv.Atoi(m[1])
		to := from
		if m[2] != "" {
			to, _ = strconv.Atoi(m[2])
		}
		for n := from; n <= to; n++ {
			out = append(out, n)
		}
	}
	return out
}

// edGlob turns a Touches entry into a touches glob. A package or app name stays as it is (an alias
// stands for its folder). A path to a folder, existing or not, gets "/**"; a file stays a file.
// A folder that doesn't exist yet is one whose last name has no extension.
func edGlob(root, entry string, aliases map[string]string) (string, string) {
	entry = strings.Trim(strings.TrimSpace(entry), "/")
	head, _, hasRest := strings.Cut(entry, "/")
	if _, ok := aliases[head]; ok && !hasRest {
		return entry, ""
	}
	if _, ok := aliases[head]; !ok && !hasRest {
		return "", fmt.Sprintf("Touches %q isn't a package or app name (an alias in .furrow.yml); give a path", entry)
	}
	if paths.IsGlob(entry) {
		return entry, ""
	}
	full := filepath.Join(root, filepath.FromSlash(paths.Resolve(entry, aliases)))
	if info, err := os.Stat(full); err == nil {
		if info.IsDir() {
			return entry + "/**", ""
		}
		return entry, ""
	}
	if !strings.Contains(filepath.Base(entry), ".") {
		return entry + "/**", ""
	}
	return entry, ""
}

// edText is the spec in Furrow's format.
func edText(s *edSource, steps []edPlannedStep, moved map[string]string, o Options) string {
	specsDir := filepath.ToSlash(filepath.Clean(o.Config.Specs))
	fromDir := specsDir + "/" + s.dir
	link := func(text string) string { return edLinks(text, fromDir, specsDir, moved) }

	// Which criteria are ticked: every one in a shipped spec; otherwise each one every step
	// covering it is merged, with at least one such step.
	ticked := map[string]bool{}
	coveredBy := map[string][]bool{}
	for _, st := range steps {
		for _, c := range st.covers {
			coveredBy[c] = append(coveredBy[c], st.merged)
		}
	}
	for i := range s.crits {
		ac := fmt.Sprintf("AC-%d", i+1)
		all := len(coveredBy[ac]) > 0
		for _, m := range coveredBy[ac] {
			all = all && m
		}
		ticked[ac] = s.plan.Status == spec.Done || all
	}

	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	priority := "p1"
	for _, m := range s.meta {
		if m.key == "Status" && s.plan.Status == spec.Draft && strings.Contains(strings.Join(m.lines, " "), "after 1.0") {
			priority = "p2"
		}
	}
	context := []string{config.Default().Product}
	if o.Config.Product != "" {
		context[0] = o.Config.Product
	}
	for _, m := range s.meta {
		joined := strings.Join(m.lines, " ")
		switch m.key {
		case "Decisions":
			for _, g := range edADR.FindAllStringSubmatch(joined, -1) {
				for _, n := range edFour.FindAllString(g[1], -1) {
					if f := adrFile(o.Root, n); f != "" && !slices.Contains(context, f) {
						context = append(context, f)
					}
				}
			}
		case "Depends on":
			for _, m := range edSpecLink.FindAllStringSubmatch(joined, -1) {
				if to, ok := moved[m[3]]; ok && to != moved[s.slug] && !slices.Contains(context, to) {
					context = append(context, to)
				}
			}
		}
	}

	line("---")
	line("id: %s", s.id)
	line("title: %s", yamlString(s.title))
	line("status: %s", s.plan.Status)
	line("area: %s", yamlString(s.plan.Area))
	line("priority: %s", priority)
	line("human_review: required")
	line("imported: edition")
	line("invariants: []")
	blockList(&b, "context", context, "")
	if len(steps) == 0 {
		line("steps: []")
	} else {
		line("steps:")
	}
	for _, st := range steps {
		line("  - n: %d", st.n)
		line("    title: %s", yamlString(st.title))
		line("    after: %s", edAfterList(st.after))
		blockList(&b, "touches", st.touches, "    ")
		line("    covers: %s", inlineList(st.covers))
		line("    size: m")
		line("    merged: %v", st.merged)
		if st.blocked != "" {
			line("    blocked: %s", yamlString(st.blocked))
		}
	}
	line("---")
	line("")
	line("# %s: %s", s.id, s.title)
	line("")
	for _, m := range s.meta {
		if m.key == "Status" {
			v := strings.TrimSpace(strings.TrimPrefix(m.lines[0], "- **Status:**"))
			first := append([]string{"- **Status when imported:** " + v}, m.lines[1:]...)
			b.WriteString(link(strings.Join(first, "\n")) + "\n")
			continue
		}
		b.WriteString(link(strings.Join(m.lines, "\n")) + "\n")
	}

	body := func(sec *edSection) string { return strings.Trim(strings.Join(sec.lines, "\n"), "\n") }
	write := func(head, text string) {
		line("")
		line("%s", head)
		if text != "" {
			line("")
			b.WriteString(link(text) + "\n")
		}
	}
	used := map[string]bool{"": true}
	if g := s.section("Goal"); g != nil {
		write("## Why", body(g))
		used["Goal"] = true
	} else {
		write("## Why", "")
	}
	write("## What", "")
	for _, name := range []string{"Screens", "Data", "Design changes"} {
		if sec := s.section(name); sec != nil {
			write("### "+name, body(sec))
			used[name] = true
		}
	}
	if len(s.steps) > 0 {
		var steps []string
		for _, st := range s.steps {
			first := regexp.MustCompile(`^(\d+)\. \[[ x]\] `).ReplaceAllString(st.lines[0], "$1. ")
			steps = append(steps, append([]string{first}, st.lines[1:]...)...)
		}
		write("### Steps", strings.Join(steps, "\n"))
	}
	used["Steps"] = true

	// Acceptance criteria: each Behavior criterion as "- [x] AC-n: S5. text (test: s5…)", keeping
	// the lines around them.
	if beh := s.section("Behavior"); beh != nil {
		var out []string
		ci := 0
		for j := 0; j < len(beh.lines); j++ {
			if ci < len(s.crits) && s.crits[ci].line == j {
				c := s.crits[ci]
				ac := fmt.Sprintf("AC-%d", ci+1)
				box := " "
				if ticked[ac] {
					box = "x"
				}
				out = append(out, fmt.Sprintf("- [%s] %s: %s. %s (test: %s…)", box, ac, c.label, link(c.text), strings.ToLower(c.label)))
				j += c.span - 1
				ci++
				continue
			}
			out = append(out, link(beh.lines[j]))
		}
		write("## Acceptance criteria", strings.Trim(strings.Join(out, "\n"), "\n"))
		used["Behavior"] = true
	} else {
		write("## Acceptance criteria", "")
	}
	var answered []string
	if sec := s.section("Out of scope"); sec != nil {
		write("## Out of scope", body(sec))
		used["Out of scope"] = true
	}
	if sec := s.section("Open questions"); sec != nil {
		open, ans := edSplitAnswered(sec.lines)
		answered = ans
		if text := strings.Trim(strings.Join(open, "\n"), "\n"); text != "" {
			write("## Open questions", text)
		}
		used["Open questions"] = true
	}
	for _, sec := range s.sections {
		if !used[sec.name] && sec.name != "Changes since accepted" {
			write("## "+sec.name, body(&sec))
		}
	}
	decisions := ""
	if len(answered) > 0 {
		decisions = "Answered in Open questions before the import:\n\n" + strings.Trim(strings.Join(answered, "\n"), "\n")
	}
	if sec := s.section("Changes since accepted"); sec != nil && body(sec) != "" {
		if decisions != "" {
			decisions += "\n\n"
		}
		decisions += body(sec)
	}
	write("## Decisions", decisions)
	text := b.String()
	if m := edMap(s.crits); m != "" {
		text = appendToSection(text, "Decisions", fmt.Sprintf("- %s: Imported from %s. %s", o.Now.Format(dateLayout), s.rel, m))
	}
	return text
}

// edSplitAnswered takes the items under an "Answered" label line out of Open questions. The
// group runs to the next label line ("Still open") or the end. The label itself goes too.
func edSplitAnswered(lines []string) (open, answered []string) {
	in := false
	for _, l := range lines {
		label := strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "- ") && !strings.HasPrefix(l, " ")
		switch {
		case label && strings.TrimSpace(l) == "Answered":
			in = true
			continue
		case label:
			in = false
		}
		if in {
			answered = append(answered, l)
		} else {
			open = append(open, l)
		}
	}
	return open, answered
}

// edMap says how the criteria were renamed, so tests named after the letters can still be found.
func edMap(crits []edCrit) string {
	if len(crits) == 0 {
		return ""
	}
	same := true
	for i, c := range crits {
		if c.n != i+1 {
			same = false
		}
	}
	first, last := crits[0], crits[len(crits)-1]
	if same {
		if len(crits) == 1 {
			return fmt.Sprintf("Criterion %s is AC-1, and its text starts with %s.", first.label, first.label)
		}
		return fmt.Sprintf("Criteria %s–%s are AC-1–AC-%d, in order, and each one's text starts with its letter.", first.label, last.label, len(crits))
	}
	var pairs []string
	for i, c := range crits {
		pairs = append(pairs, fmt.Sprintf("%s is AC-%d", c.label, i+1))
	}
	return "Criteria renamed, each one's text starting with its letter: " + strings.Join(pairs, ", ") + "."
}

func edAfterList(after []string) string {
	q := make([]string, len(after))
	for i, a := range after {
		if _, err := strconv.Atoi(a); err == nil {
			q[i] = a
		} else {
			q[i] = yamlString(a)
		}
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// edLinks points links to Edition specs at their new paths: "docs/specs/slug.md" becomes the new
// repo path, and a bare "slug.md" or "./slug.md" the path relative to the spec's new folder.
func edLinks(text, fromDir, specsDir string, moved map[string]string) string {
	return edSpecLink.ReplaceAllStringFunc(text, func(m string) string {
		sm := edSpecLink.FindStringSubmatch(m)
		lead, prefix, slug := sm[1], sm[2], sm[3]
		to, ok := moved[slug]
		if !ok {
			return m
		}
		if prefix == "docs/specs/" {
			return lead + to
		}
		dir, base := filepath.ToSlash(filepath.Dir(to)), filepath.Base(to)
		if dir == fromDir {
			return lead + base
		}
		return lead + "../" + strings.TrimPrefix(dir, specsDir+"/") + "/" + base
	})
}

func adrFile(root, num string) string {
	matches, _ := filepath.Glob(filepath.Join(root, "docs", "decisions", num+"-*.md"))
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return "docs/decisions/" + filepath.Base(matches[0])
}
