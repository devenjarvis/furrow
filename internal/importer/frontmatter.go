package importer

import (
	"fmt"
	"regexp"
	"strings"
)

// fetchFront is a Fetch spec's front matter, read the way scripts/specs.mjs reads it: scalars,
// [inline, lists] and indented "- item" lists, with " # comments" dropped.
type fetchFront struct {
	keys   []string            // in file order
	values map[string][]string // a scalar is a one-item list
	lists  map[string]bool     // keys written as lists
}

func (f fetchFront) scalar(key string) string {
	if v := f.values[key]; len(v) > 0 && !f.lists[key] {
		return v[0]
	}
	return ""
}

func (f fetchFront) list(key string) []string {
	return f.values[key]
}

var (
	frontRe   = regexp.MustCompile(`^---\n([\s\S]*?)\n---\n`)
	commentRe = regexp.MustCompile(`\s+#.*$`)
	itemRe    = regexp.MustCompile(`^\s+-\s+(.*)$`)
	keyRe     = regexp.MustCompile(`^([A-Za-z_][\w-]*):\s*(.*)$`)
	quotedRe  = regexp.MustCompile(`^(["']).*["']$`)
)

func unquote(s string) string {
	if len(s) >= 2 && quotedRe.MatchString(s) && s[0] == s[len(s)-1] {
		return s[1 : len(s)-1]
	}
	return s
}

// parseFetch splits a Fetch spec into its front matter and body, as specs.mjs does.
func parseFetch(text string) (fetchFront, string, error) {
	f := fetchFront{values: map[string][]string{}, lists: map[string]bool{}}
	m := frontRe.FindStringSubmatchIndex(text)
	if m == nil {
		return f, "", fmt.Errorf("missing front matter")
	}
	listKey := ""
	for _, raw := range strings.Split(text[m[2]:m[3]], "\n") {
		line := commentRe.ReplaceAllString(raw, "")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if item := itemRe.FindStringSubmatch(line); item != nil && listKey != "" {
			f.values[listKey] = append(f.values[listKey], unquote(strings.TrimSpace(item[1])))
			continue
		}
		kv := keyRe.FindStringSubmatch(line)
		if kv == nil {
			return f, "", fmt.Errorf("can't read front matter line: %s", raw)
		}
		key, value := kv[1], kv[2]
		if _, seen := f.values[key]; !seen {
			f.keys = append(f.keys, key)
		}
		if value == "" {
			f.values[key], f.lists[key], listKey = []string{}, true, key
			continue
		}
		listKey = ""
		if strings.HasPrefix(value, "[") {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
			items := []string{}
			if inner != "" {
				for _, s := range strings.Split(inner, ",") {
					items = append(items, unquote(strings.TrimSpace(s)))
				}
			}
			f.values[key], f.lists[key] = items, true
			continue
		}
		f.values[key], f.lists[key] = []string{unquote(strings.TrimSpace(value))}, false
	}
	return f, text[m[1]:], nil
}
