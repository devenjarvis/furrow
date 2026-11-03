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

// MaxBehaviourWords is the most words ## Behaviour may have, outside code blocks and images.
const MaxBehaviourWords = 80

// Limits for the parts of a step brief a reviewer reads first.
const (
	MaxBeforeAfterWords  = 20 // each of the **Before:** and **After:** lines
	MaxReviewItems       = 3  // numbered items in ## Review this
	MaxJudgmentCallWords = 30 // each line of ## Judgment calls
)

// NoVisibleEffect is the answer to "show it" for a change with nothing to show.
const NoVisibleEffect = "No visible effect."

var (
	comment    = regexp.MustCompile(`(?s)<!--.*?-->`)
	link       = regexp.MustCompile(`\[[^\]]*\]\([^)]+\)`) // also matches an image, ![…](…)
	newWord    = regexp.MustCompile(`(?i)\bnew\b`)
	sepCell    = regexp.MustCompile(`^:?-+:?$`)
	image      = regexp.MustCompile(`(?s)!\[[^\]]*\]\([^)]*\)|<img\b[^>]*>`)
	numbered   = regexp.MustCompile(`^\d+[.)]\s`)
	listMarker = regexp.MustCompile(`^[-*+]\s+`)
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
		if hasPlaceholder(body, s.Heading == Behaviour) {
			rep.add("`## %s` still has a placeholder. Write it.", s.Heading)
		}
		switch s.Heading {
		case Behaviour:
			checkBehaviour(rep, body)
		case Diagram:
			checkDiagram(rep, body)
		case ReviewThis:
			checkReviewThis(rep, body)
		case VisualProof:
			checkVisual(rep, body)
		case JudgmentCalls:
			checkJudgmentCalls(rep, body)
		}
	}
	if hasPlaceholder(parts[0].text, false) {
		first := Behaviour
		if len(b.Sections) > 0 {
			first = b.Sections[0].Heading
		}
		rep.add("The text before `## %s` still has a placeholder. Write it.", first)
	}
	for _, p := range parts[1:] {
		if !expected[p.heading] && hasPlaceholder(p.text, false) {
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
			if closesFence(fence, trimmed) {
				fence = ""
			}
		case openFence(trimmed) != "":
			fence = openFence(trimmed)
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

// openFence returns the run of three or more ` or ~ that opens a fenced code block on the
// trimmed line, or "".
func openFence(trimmed string) string {
	for _, c := range []string{"`", "~"} {
		if strings.HasPrefix(trimmed, c+c+c) {
			return trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, c))]
		}
	}
	return ""
}

// closesFence says whether the trimmed line closes the block fence opened: a run of the same
// character, at least as long, and nothing else.
func closesFence(fence, trimmed string) bool {
	return len(trimmed) >= len(fence) && strings.Trim(trimmed, fence[:1]) == ""
}

// outsideFences returns text's lines that aren't in a fenced code block, fences dropped, and
// whether it had a fenced block at all.
func outsideFences(text string) (string, bool) {
	var out []string
	fence, found := "", false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			if closesFence(fence, trimmed) {
				fence = ""
			}
		case openFence(trimmed) != "":
			fence, found = openFence(trimmed), true
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n"), found
}

// hasPlaceholder says whether text has a <!-- furrow: placeholder. With showIt, text is
// ## Behaviour's body, and a placeholder in a fenced code block after its **After:** line is shown
// output, such as a pasted brief, so it doesn't count. Every other one counts, in a fence or
// <details> or not, so an unfilled section can't hide in a code block.
func hasPlaceholder(text string, showIt bool) bool {
	fence, after := "", false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		inside := fence != ""
		switch {
		case inside:
			if closesFence(fence, trimmed) {
				fence = ""
			}
		case openFence(trimmed) != "":
			fence = openFence(trimmed)
		case strings.HasPrefix(trimmed, AfterLabel):
			after = true
		}
		if inside && showIt && after {
			continue
		}
		if strings.Contains(line, Placeholder) {
			return true
		}
	}
	return false
}

// checkBehaviour wants a **Before:** and an **After:** line of at most 20 words each, something
// that shows the change, and at most 80 words outside code blocks, images and comments.
func checkBehaviour(rep *Report, body string) {
	prose, fenced := outsideFences(body)
	prose = comment.ReplaceAllString(prose, " ")
	if n := len(strings.Fields(image.ReplaceAllString(prose, " "))); n > MaxBehaviourWords {
		rep.add("`## %s` is %d words. Keep it to %d.", Behaviour, n, MaxBehaviourWords)
	}
	count := map[string]int{}
	for _, line := range strings.Split(prose, "\n") {
		line = strings.TrimSpace(line)
		for _, label := range []string{BeforeLabel, AfterLabel} {
			if _, seen := count[label]; !seen && strings.HasPrefix(line, label) {
				count[label] = len(strings.Fields(strings.TrimPrefix(line, label)))
			}
		}
	}
	if len(count) < 2 {
		rep.add("`## %s` needs a `%s` and an `%s` line.", Behaviour, BeforeLabel, AfterLabel)
	}
	for _, label := range []string{BeforeLabel, AfterLabel} {
		if n := count[label]; n > MaxBeforeAfterWords {
			rep.add("`%s` is %d words. Keep it to %d.", label, n, MaxBeforeAfterWords)
		}
	}
	if !fenced && !image.MatchString(prose) && !strings.Contains(prose, NoVisibleEffect) {
		rep.add("`## %s` doesn't show the change. Add output, a screenshot, or a usage example.", Behaviour)
	}
}

// checkReviewThis counts the numbered items outside code blocks.
func checkReviewThis(rep *Report, body string) {
	prose, _ := outsideFences(body)
	n := 0
	for _, line := range strings.Split(comment.ReplaceAllString(prose, " "), "\n") {
		if numbered.MatchString(line) {
			n++
		}
	}
	if n > MaxReviewItems {
		rep.add("`## %s` has %d items. Keep the %d that need a person.", ReviewThis, n, MaxReviewItems)
	}
}

// checkJudgmentCalls reports each line over 30 words, comments and list markers not counted.
func checkJudgmentCalls(rep *Report, body string) {
	prose, _ := outsideFences(body)
	for _, line := range strings.Split(comment.ReplaceAllString(prose, " "), "\n") {
		line = listMarker.ReplaceAllString(strings.TrimSpace(line), "")
		if n := len(strings.Fields(line)); n > MaxJudgmentCallWords {
			rep.add("A judgment call is %d words. Shorten it in the spec's `## Decisions` and run `furrow brief` again.", n)
		}
	}
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
