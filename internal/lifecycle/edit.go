package lifecycle

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The edits below change a spec or area file's text in place. Each one changes the line it means to
// and leaves every other byte alone, so comments and formatting survive. None of them ever touches
// a criterion's text: tickBox changes only the character inside "[ ]" (INV-4).

// frontMatter parses the front matter's top-level mapping. YAML line N is file line index N,
// because file line index 0 is the opening "---".
func frontMatter(text string) (*yaml.Node, int, error) {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return nil, 0, fmt.Errorf("missing front matter (the file must start with ---)")
	}
	var front string
	if end := strings.Index(norm[4:], "\n---\n"); end >= 0 {
		front = norm[4 : 4+end]
	} else if strings.HasSuffix(norm, "\n---") {
		front = norm[4 : len(norm)-4]
	} else {
		return nil, 0, fmt.Errorf("front matter isn't closed with ---")
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		return nil, 0, fmt.Errorf("can't read front matter: %v", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, 0, fmt.Errorf("front matter isn't a mapping")
	}
	bodyStart := strings.Count(front, "\n") + 2 // the line after the closing ---
	return doc.Content[0], bodyStart, nil
}

// lookup returns a mapping's key and value nodes for key, or nils.
func lookup(m *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i], m.Content[i+1]
		}
	}
	return nil, nil
}

// setScalar replaces one scalar value where it's written, keeping everything around it.
func setScalar(text string, v *yaml.Node, value string) (string, error) {
	lines := strings.Split(text, "\n")
	if v.Kind != yaml.ScalarNode || v.Line < 1 || v.Line >= len(lines) {
		return "", fmt.Errorf("can't find the value to change on line %d", v.Line+1)
	}
	line := lines[v.Line]
	col := v.Column - 1
	if col < 0 || col > len(line) {
		return "", fmt.Errorf("can't find the value to change on line %d", v.Line+1)
	}
	rest := line[col:]
	n := -1
	switch {
	case v.Style == yaml.DoubleQuotedStyle || v.Style == yaml.SingleQuotedStyle:
		if len(rest) > 1 {
			if end := strings.IndexByte(rest[1:], rest[0]); end >= 0 {
				n = end + 2
			}
		}
	case strings.HasPrefix(rest, v.Value):
		n = len(v.Value)
	}
	if n < 0 {
		return "", fmt.Errorf("can't change line %d: write it as a plain value", v.Line+1)
	}
	lines[v.Line] = line[:col] + value + rest[n:]
	return strings.Join(lines, "\n"), nil
}

// setStatus sets the spec's top-level status.
func setStatus(text, status string) (string, error) {
	root, _, err := frontMatter(text)
	if err != nil {
		return "", err
	}
	_, v := lookup(root, "status")
	if v == nil {
		return "", fmt.Errorf("the front matter has no status line")
	}
	return setScalar(text, v, status)
}

// setMerged sets merged: true on step n. A block-style step with no merged key gets one, right
// after its n line.
func setMerged(text string, n int) (string, error) {
	root, _, err := frontMatter(text)
	if err != nil {
		return "", err
	}
	_, steps := lookup(root, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return "", fmt.Errorf("the front matter has no steps")
	}
	for _, st := range steps.Content {
		if st.Kind != yaml.MappingNode {
			continue
		}
		nk, nv := lookup(st, "n")
		if nv == nil || nv.Value != fmt.Sprint(n) {
			continue
		}
		if _, mv := lookup(st, "merged"); mv != nil {
			return setScalar(text, mv, "true")
		}
		if st.Style&yaml.FlowStyle != 0 {
			return "", fmt.Errorf("step %d has no merged line; add \"merged: false\" to it first", n)
		}
		lines := strings.Split(text, "\n")
		at := nv.Line + 1
		indent := strings.Repeat(" ", nk.Column-1)
		lines = append(lines[:at], append([]string{indent + "merged: true"}, lines[at:]...)...)
		return strings.Join(lines, "\n"), nil
	}
	return "", fmt.Errorf("the front matter has no step %d", n)
}

var criterionLine = regexp.MustCompile(`^- \[([ xX])\] (AC-\d+): `)

// tickBox ticks one criterion's box under "## Acceptance criteria". Only the character between the
// brackets changes.
func tickBox(text, id string) (string, error) {
	_, bodyStart, err := frontMatter(text)
	if err != nil {
		return "", err
	}
	lines := strings.Split(text, "\n")
	in := false
	for i := bodyStart; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "## ") {
			in = strings.TrimSpace(l[3:]) == "Acceptance criteria"
			continue
		}
		if !in {
			continue
		}
		m := criterionLine.FindStringSubmatch(l)
		if m == nil || m[2] != id {
			continue
		}
		if m[1] != " " {
			return "", fmt.Errorf("%s is already ticked", id)
		}
		lines[i] = l[:3] + "x" + l[4:]
		return strings.Join(lines, "\n"), nil
	}
	return "", fmt.Errorf("can't find %s under \"## Acceptance criteria\"", id)
}

// appendToSection adds a line at the end of "## name": after its last line, or after the heading
// and a blank line when it's empty. With no such section, it adds one at the end of the file.
func appendToSection(text, name, line string) string {
	newline := strings.HasSuffix(text, "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	start := 0
	if _, bodyStart, err := frontMatter(text); err == nil {
		start = bodyStart
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
	case last < 0 && h+1 < end: // empty, with a blank line under the heading: use it
		at, add = h+2, []string{line}
	case last < 0:
		at, add = h+1, []string{"", line}
	case strings.HasPrefix(strings.TrimSpace(lines[last]), "- "):
		add = []string{line}
	default: // after a paragraph: start a list
		add = []string{"", line}
	}
	if last < 0 && at == end && end < len(lines) { // keep a blank line before the next heading
		add = append(add, "")
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
