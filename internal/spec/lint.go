package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/devenjarvis/furrow/internal/paths"
)

var (
	priorities    = []string{"p0", "p1", "p2"}
	reviews       = []string{"required", "optional"}
	sizes         = []string{"", "s", "m", "l"}
	requiredHeads = []string{"Why", "What", "Acceptance criteria", "Out of scope"}
	tbd           = regexp.MustCompile(`\bTBD\b`)
	fenced        = regexp.MustCompile("(?s)```.*?```")
	inlineCode    = regexp.MustCompile("`[^`\n]*`")
)

// prose is the body without code, so a spec can mention `TBD` without containing it.
func prose(body string) string {
	return inlineCode.ReplaceAllString(fenced.ReplaceAllString(body, ""), "")
}

func has(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// literal is the entry's segments before the first one with a glob character, `{` included.
func literal(entry string) []string {
	lit := paths.Literal(entry)
	for i, seg := range lit {
		if strings.Contains(seg, "{") {
			return lit[:i]
		}
	}
	return lit
}

// Lint checks every rule in specs/README.md. Problems come back in a fixed order: load problems,
// then specs by number, then areas by name, then repo-wide ones.
func Lint(r *Repo) []Problem {
	var out []Problem
	out = append(out, r.Problems...)
	add := func(file, format string, args ...any) {
		out = append(out, Problem{File: file, Message: fmt.Sprintf(format, args...)})
	}
	warn := func(file, format string, args ...any) {
		out = append(out, Problem{File: file, Message: fmt.Sprintf(format, args...), Warning: true})
	}
	invariants := map[string]bool{}
	for _, inv := range r.Invariants {
		invariants[inv.ID] = true
	}

	seen := map[string]string{}
	for _, s := range r.Specs {
		f := s.File
		// 1. Identity.
		if want := fmt.Sprintf("%s-%04d", r.Config.Prefix, s.Num); s.ID != want {
			add(f, "id should be %s, found %q", want, s.ID)
		}
		if other, dup := seen[s.ID]; dup {
			add(f, "duplicate id %s (also in %s)", s.ID, other)
		} else {
			seen[s.ID] = f
		}
		// 2. Enums.
		if s.Archived && s.Status != Done {
			add(f, "archived specs must be status done, found %q", s.Status)
		}
		if !s.Archived && !has([]string{Draft, Accepted, Paused}, s.Status) {
			add(f, "status must be draft, accepted or paused, found %q (done specs live in archive/)", s.Status)
		}
		if !has(priorities, s.Priority) {
			add(f, "priority must be p0, p1 or p2, found %q", s.Priority)
		}
		if !has(reviews, s.HumanReview) {
			add(f, "human_review must be required or optional, found %q", s.HumanReview)
		}
		// 3. Area.
		if strings.TrimSpace(s.Area) == "" {
			add(f, "area is empty: name the living spec this changes")
		} else if _, ok := r.Areas[s.Area]; !ok {
			add(f, "area %q has no %s", s.Area, r.rel("areas", s.Area+".md"))
		}
		// 4. Sections.
		for _, h := range requiredHeads {
			if !has(s.Sections, h) {
				add(f, "missing section \"## %s\"", h)
			}
		}
		if needsBeforeAfter(s) {
			if !has(s.Sections, BeforeAndAfter) {
				add(f, "missing section \"## %s\"", BeforeAndAfter)
			} else {
				for _, p := range beforeAfterProblems(SectionText(s.Body, BeforeAndAfter)) {
					add(f, "%s", p)
				}
			}
		}
		// 5. Criteria.
		if len(s.Criteria) == 0 {
			add(f, "no acceptance criteria (lines like \"- [ ] AC-1: … (test: …)\")")
		}
		for i, c := range s.Criteria {
			if c.Num != i+1 {
				add(f, "%s should be numbered AC-%d", c.ID, i+1)
			}
			if c.Proof == "" {
				add(f, "%s doesn't say how it's proven: add (test: …), (check: …) or (human: …)", c.ID)
			}
		}
		// 6. Steps of decided specs.
		decided := !s.Archived && (s.Status == Accepted || s.Status == Paused)
		if decided {
			lintSteps(r, s, add)
			if tbd.MatchString(prose(s.Body)) {
				add(f, "an %s spec can't contain TBD", s.Status)
			}
			for _, n := range unproposed(s.Body, bodyOffset(r, s)) {
				add(f, "Open question on line %d has no proposed answer. Add \"Proposed: …\", or answer it in ## Decisions.", n)
			}
		}
		// 7. After refs (every status) and merged-before-dependencies.
		for _, st := range s.Steps {
			refs, errs := r.AfterRefs(st)
			for _, err := range errs {
				add(f, "step %d: %v", st.N, err)
			}
			for _, ref := range refs {
				switch {
				case r.Spec(ref.Spec) == nil || (ref.Step != 0 && r.Step(ref) == nil):
					add(f, "step %d is after %s, which doesn't exist", st.N, ref)
				case ref == st.Ref() || (ref.Step == 0 && ref.Spec == s.ID):
					add(f, "step %d can't be after itself", st.N)
				case StepMerged(st) && !r.RefMerged(ref):
					add(f, "step %d is merged but %s, which it's after, isn't", st.N, ref)
				}
			}
		}
		// 8. Touches' top-level folders exist. Deeper folders may be ones the step creates.
		if !s.Archived && s.Status == Accepted {
			for _, st := range s.Steps {
				if StepMerged(st) {
					continue
				}
				for _, t := range st.Touches {
					lit := literal(paths.Resolve(t, r.Config.Aliases))
					if len(lit) < 2 {
						continue
					}
					if info, err := os.Stat(filepath.Join(r.Root, lit[0])); err != nil || !info.IsDir() {
						add(f, "step %d touches %q, but the folder %s doesn't exist", st.N, t, lit[0])
					}
				}
			}
		}
		// 9. Invariants.
		for _, id := range s.Invariants {
			if !invariants[id] {
				add(f, "invariant %s isn't defined in %s", id, r.Config.Product)
			}
		}
		if len(s.Invariants) > 0 && s.HumanReview != "required" {
			add(f, "touches invariants %s, so human_review must be required", strings.Join(s.Invariants, ", "))
		}
		// 10. Human sign-off lines.
		decisions := SectionText(s.Body, "Decisions")
		for _, c := range s.Criteria {
			if c.Proof == "human" && c.Ticked && !signedOff(decisions, c.ID) {
				add(f, "%s is a human check and is ticked, but \"## Decisions\" has no \"%s signed off by …\" line (use furrow signoff)", c.ID, c.ID)
			}
		}
		// 11. Done means done.
		if s.Status == Done && !s.Complete() {
			add(f, "status is done but some steps aren't merged or some boxes aren't ticked")
		}
		if !s.Archived && s.Status == Accepted && len(s.Steps) > 0 && s.Complete() {
			warn(f, "ready to archive: run `furrow archive %s`", s.ID)
		}
	}
	out = append(out, cycles(r)...)

	// 12. Areas and invariants.
	for _, name := range sortedKeys(r.Areas) {
		a := r.Areas[name]
		if a.Name != a.Stem {
			add(a.File, "area should be %q (the file name), found %q", a.Stem, a.Name)
		}
		for _, h := range []string{"Behaviour", "History"} {
			if !has(a.Sections, h) {
				add(a.File, "missing section \"## %s\"", h)
			}
		}
	}
	ids := map[string]bool{}
	for _, inv := range r.Invariants {
		if ids[inv.ID] {
			add(r.Config.Product, "duplicate invariant %s", inv.ID)
		}
		ids[inv.ID] = true
	}
	return out
}

func lintSteps(r *Repo, s *Spec, add func(string, string, ...any)) {
	f := s.File
	if len(s.Steps) == 0 {
		add(f, "an %s spec needs at least one step", s.Status)
	}
	covered := map[string]bool{}
	for i, st := range s.Steps {
		if st.N != i+1 {
			add(f, "step %d should be numbered %d", st.N, i+1)
		}
		if strings.TrimSpace(st.Title) == "" {
			add(f, "step %d has no title", st.N)
		}
		if !StepMerged(st) && st.Blocked == "" && len(st.Touches) == 0 {
			add(f, "step %d needs touches: the paths it may change", st.N)
		}
		if !has(sizes, st.Size) {
			add(f, "step %d: size must be s, m or l, found %q", st.N, st.Size)
		}
		if st.Size == "l" {
			add(f, "step %d is size l: split it before the spec is %s", st.N, s.Status)
		}
		for _, c := range st.Covers {
			if s.Criterion(c) == nil {
				add(f, "step %d covers %s, which isn't a criterion", st.N, c)
			}
			covered[c] = true
		}
	}
	for _, c := range s.Criteria {
		if !covered[c.ID] {
			add(f, "%s isn't covered by any step", c.ID)
		}
	}
}

// bodyOffset is how many lines of the spec's file come before its body, so line numbers match the
// file. It's 0 if the file can't be read again.
func bodyOffset(r *Repo, s *Spec) int {
	data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(s.File)))
	if err != nil {
		return 0
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasSuffix(text, s.Body) {
		return 0
	}
	return strings.Count(text[:len(text)-len(s.Body)], "\n")
}

// fence returns the backtick run that opens or closes a fenced code block on this line, or "".
func fence(line string) string {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return ""
	}
	n := 0
	for n < len(t) && t[n] == '`' {
		n++
	}
	if n < 3 {
		return ""
	}
	return t[:n]
}

// unproposed returns the file line of each top-level item under "## Open questions" that doesn't
// say "Proposed:" anywhere in it. Lines inside fenced code blocks are skipped, headings included.
// offset is the number of file lines before body.
func unproposed(body string, offset int) []int {
	var out []int
	open := ""  // the fence we're inside, or ""
	in := false // inside ## Open questions
	start := 0  // file line of the current item, or 0
	proposed := false
	finish := func() {
		if start > 0 && !proposed {
			out = append(out, start)
		}
		start, proposed = 0, false
	}
	for i, l := range strings.Split(body, "\n") {
		if f := fence(l); f != "" {
			switch {
			case open == "":
				open = f
				continue
			case strings.HasPrefix(f, open) && strings.TrimSpace(l) == f:
				open = ""
				continue
			}
		}
		if open != "" {
			continue
		}
		if strings.HasPrefix(l, "# ") || strings.HasPrefix(l, "## ") {
			finish()
			in = strings.HasPrefix(l, "## ") && strings.TrimSpace(l[3:]) == "Open questions"
			continue
		}
		if !in {
			continue
		}
		if strings.HasPrefix(l, "- ") {
			finish()
			start = offset + i + 1
		}
		if start > 0 && strings.Contains(l, "Proposed:") {
			proposed = true
		}
	}
	finish()
	return out
}

func signedOff(decisions, id string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(id) + ` signed off by \S`).MatchString(decisions)
}

func cycles(r *Repo) []Problem {
	var out []Problem
	state := map[*Step]int{} // 1 visiting, 2 done
	var path []*Step
	var visit func(st *Step)
	visit = func(st *Step) {
		switch state[st] {
		case 2:
			return
		case 1:
			start := 0
			for i, p := range path {
				if p == st {
					start = i
				}
			}
			var names []string
			for _, p := range path[start:] {
				names = append(names, p.Ref().String())
			}
			names = append(names, st.Ref().String())
			out = append(out, Problem{File: st.Spec.File, Message: "steps wait on each other: " + strings.Join(names, " -> ")})
			return
		}
		state[st] = 1
		path = append(path, st)
		refs, _ := r.AfterRefs(st)
		for _, ref := range refs {
			for _, dep := range r.Expand(ref) {
				if dep != st {
					visit(dep)
				}
			}
		}
		path = path[:len(path)-1]
		state[st] = 2
	}
	for _, s := range r.Specs {
		for _, st := range s.Steps {
			visit(st)
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// Summary is the last line lint prints.
func Summary(r *Repo, problems []Problem) (string, bool) {
	errs := 0
	for _, p := range problems {
		if !p.Warning {
			errs++
		}
	}
	if errs > 0 {
		return fmt.Sprintf("%d problem(s) in %d specs.", errs, len(r.Specs)), false
	}
	return fmt.Sprintf("%d specs OK, %s open steps.", len(r.Specs), strconv.Itoa(len(r.OpenSteps()))), true
}

// ListEntry is one row of furrow list.
type ListEntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Area        string `json:"area"`
	File        string `json:"file"`
	StepsMerged int    `json:"steps_merged"`
	StepsTotal  int    `json:"steps_total"`
}

// List returns one entry per spec, in number order.
func List(r *Repo) []ListEntry {
	out := []ListEntry{}
	for _, s := range r.Specs {
		out = append(out, ListEntry{
			ID: s.ID, Title: s.Title, Status: s.DerivedStatus(), Area: s.Area, File: s.File,
			StepsMerged: s.MergedSteps(), StepsTotal: len(s.Steps),
		})
	}
	return out
}
