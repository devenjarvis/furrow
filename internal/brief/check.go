package brief

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/spec"
)

// Report is what furrow brief --check found in a filled-in brief.
type Report struct {
	OK       bool     `json:"ok"`
	Problems []string `json:"problems"`
}

// Text prints one line per problem, and nothing for a complete brief.
func (r *Report) Text(w io.Writer) {
	for _, p := range r.Problems {
		fmt.Fprintln(w, p)
	}
}

// ExitCode is 0 for a complete brief and 1 otherwise.
func (r *Report) ExitCode() int {
	if r.OK {
		return 0
	}
	return 1
}

// MaxBehaviourWords is the most words ## Behaviour may have.
const MaxBehaviourWords = 80

var (
	comment = regexp.MustCompile(`(?s)<!--.*?-->`)
	link    = regexp.MustCompile(`\[[^\]]*\]\([^)]+\)`) // also matches an image, ![…](…)
	newWord = regexp.MustCompile(`(?i)\bnew\b`)
	sepCell = regexp.MustCompile(`^:?-+:?$`)
)

// CheckFile builds the brief for the step, as furrow brief would, and checks the brief in file
// against it. It only reads.
func CheckFile(g gitx.Git, repo *spec.Repo, opt Options, file string) (*Report, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	b, err := Build(g, repo, opt)
	if err != nil {
		return nil, err
	}
	return Check(b, string(data)), nil
}

// part is one "## Heading" of a filled-in brief, or the text before the first heading.
type part struct {
	heading string
	text    string
}

// Check reads text, a filled-in brief, against b, the brief furrow brief writes for the same step.
// Problems are in b's section order, then any placeholder outside b's sections, in text order.
func Check(b *Brief, text string) *Report {
	parts := split(text)
	byHeading := map[string]string{}
	for _, p := range parts[1:] {
		if _, ok := byHeading[p.heading]; !ok {
			byHeading[p.heading] = p.text
		}
	}
	rep := &Report{Problems: []string{}}
	expected := map[string]bool{}
	for _, s := range b.Sections {
		expected[s.Heading] = true
		body, ok := byHeading[s.Heading]
		if !ok {
			rep.add("No `## %s` section. Run `furrow brief` and paste it.", s.Heading)
			continue
		}
		if strings.Contains(body, Placeholder) {
			rep.add("`## %s` still has a placeholder. Write it.", s.Heading)
		}
		switch s.Heading {
		case Behaviour:
			if n := len(strings.Fields(comment.ReplaceAllString(body, " "))); n > MaxBehaviourWords {
				rep.add("`## %s` is %d words. Keep it to %d.", Behaviour, n, MaxBehaviourWords)
			}
		case Diagram:
			checkDiagram(rep, body)
		case VisualProof:
			checkVisual(rep, body)
		}
	}
	if strings.Contains(parts[0].text, Placeholder) {
		first := Behaviour
		if len(b.Sections) > 0 {
			first = b.Sections[0].Heading
		}
		rep.add("The text before `## %s` still has a placeholder. Write it.", first)
	}
	for _, p := range parts[1:] {
		if !expected[p.heading] && strings.Contains(p.text, Placeholder) {
			rep.add("`## %s` still has a placeholder. Write it.", p.heading)
		}
	}
	rep.OK = len(rep.Problems) == 0
	return rep
}

func (r *Report) add(format string, args ...any) {
	r.Problems = append(r.Problems, fmt.Sprintf(format, args...))
}

// split cuts text at "## " headings outside code fences. The first part is the text before any
// heading, with an empty heading.
func split(text string) []part {
	parts := []part{{}}
	var lines []string
	fence := ""
	flush := func() { parts[len(parts)-1].text = strings.Join(lines, "\n") }
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == "" {
				fence = ""
			}
		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			fence = trimmed[:3]
		case strings.HasPrefix(line, "## "):
			flush()
			parts = append(parts, part{heading: strings.TrimSpace(strings.TrimPrefix(line, "## "))})
			lines = nil
			continue
		}
		lines = append(lines, line)
	}
	flush()
	return parts
}

func checkDiagram(rep *Report, body string) {
	if len(MermaidBlocks(body)) == 0 {
		rep.add("`## %s` has no ```mermaid block. Copy the spec's diagram from `furrow brief`.", Diagram)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Legend:") {
			return
		}
	}
	rep.add("`## %s` has no line starting `Legend:`. Say what a box, an arrow and each colour mean.", Diagram)
}

// checkVisual reads the table in ## Visual proof: its first row is the header, separator rows
// are skipped, and each other row needs a before image, link or "new", and an after image or link.
func checkVisual(rep *Report, body string) {
	header, n := false, 0
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if isSeparator(cells) {
			continue
		}
		if !header {
			header = true
			continue
		}
		n++
		cell := func(i int) string {
			if i < len(cells) {
				return comment.ReplaceAllString(cells[i], " ")
			}
			return ""
		}
		if before := cell(1); !link.MatchString(before) && !newWord.MatchString(before) {
			rep.add("Row %d of `## %s` has no before image, or \"new\".", n, VisualProof)
		}
		if !link.MatchString(cell(2)) {
			rep.add("Row %d of `## %s` has no after image.", n, VisualProof)
		}
	}
	if n == 0 {
		rep.add("`## %s` has no table row. Add one per screen, with before and after images.", VisualProof)
	}
}

func isSeparator(cells []string) bool {
	for _, c := range cells {
		if !sepCell.MatchString(strings.TrimSpace(c)) {
			return false
		}
	}
	return true
}
