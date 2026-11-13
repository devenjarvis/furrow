package spec

import (
	"fmt"
	"regexp"
	"strings"
)

// BeforeAndAfter is the heading of a spec's mock of what a user sees now and will see.
const BeforeAndAfter = "Before and after"

// Labels, limits and stand-ins for a missing mock in "## Before and after".
const (
	BeforeLabel         = "**Before:**"
	AfterLabel          = "**After:**"
	MaxBeforeAfterWords = 20
	NothingYet          = "Nothing yet."
	NoVisibleEffect     = "No visible effect."
)

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	imageRef    = regexp.MustCompile(`(?s)!\[[^\]]*\]\([^)]*\)|<img\b[^>]*>`)
)

// needsBeforeAfter reports whether lint asks s for "## Before and after": a decided spec with a
// step that isn't merged.
func needsBeforeAfter(s *Spec) bool {
	if s.Archived || (s.Status != Accepted && s.Status != Paused) {
		return false
	}
	return s.MergedSteps() < len(s.Steps)
}

// side is what one label of "## Before and after" says: from its line to the next label's.
type side struct {
	found bool
	words int    // on the label's line, after the label
	shown bool   // a code block or an image
	prose string // the text outside code blocks, comments removed
}

// beforeAfterProblems checks the text of "## Before and after" and returns lint's messages, in a
// fixed order: the labels, Before's words, After's words, then Before and After shown.
func beforeAfterProblems(section string) []string {
	lines := strings.Split(htmlComment.ReplaceAllString(section, " "), "\n")
	code := codeLines(lines)
	labels := []string{BeforeLabel, AfterLabel}
	var sides [2]side
	cur := -1
	for i, l := range lines {
		if code[i] {
			if cur >= 0 {
				sides[cur].shown = true
			}
			continue
		}
		t := strings.TrimSpace(l)
		for k, label := range labels {
			if !sides[k].found && strings.HasPrefix(t, label) {
				cur = k
				sides[k].found = true
				sides[k].words = len(strings.Fields(strings.TrimPrefix(t, label)))
			}
		}
		if cur < 0 {
			continue
		}
		sides[cur].prose += l + "\n"
		if imageRef.MatchString(l) {
			sides[cur].shown = true
		}
	}
	quoted := fmt.Sprintf("%q", "## "+BeforeAndAfter)
	var out []string
	if !sides[0].found || !sides[1].found {
		out = append(out, fmt.Sprintf("%s needs a %s and an %s line", quoted, BeforeLabel, AfterLabel))
	}
	for k, label := range labels {
		if n := sides[k].words; n > MaxBeforeAfterWords {
			out = append(out, fmt.Sprintf("%s in %s is %d words. Keep it to %d.", label, quoted, n, MaxBeforeAfterWords))
		}
	}
	before, after := sides[0], sides[1]
	noEffect := after.found && strings.Contains(after.prose, NoVisibleEffect)
	if before.found && !before.shown && !noEffect && !strings.Contains(before.prose, NothingYet) {
		out = append(out, fmt.Sprintf("%s doesn't show the change. Add a code block or image under %s, or say %q", quoted, BeforeLabel, NothingYet))
	}
	if after.found && !after.shown && !noEffect {
		out = append(out, fmt.Sprintf("%s doesn't show the change. Add a code block or image under %s, or say %q", quoted, AfterLabel, NoVisibleEffect))
	}
	return out
}
