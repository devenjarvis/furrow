// Package paths matches touches globs against files and decides whether two steps' touches can
// name the same file.
package paths

import (
	"path"
	"strings"
)

// Resolve turns a leading alias into its path: with cli: cmd/furrow, "cli/next.go" becomes
// "cmd/furrow/next.go". A bare alias stands for its whole folder: "cli" becomes "cmd/furrow/**".
func Resolve(entry string, aliases map[string]string) string {
	entry = strings.Trim(strings.TrimSpace(entry), "/")
	head, rest, _ := strings.Cut(entry, "/")
	if target, ok := aliases[head]; ok {
		target = strings.Trim(target, "/")
		if rest == "" {
			return target + "/**"
		}
		return target + "/" + rest
	}
	return entry
}

// Match reports whether a /-separated path matches glob. "*" and "?" stay within a segment, and
// "**" matches zero or more whole segments.
func Match(glob, file string) bool {
	return matchSegments(split(glob), split(file))
}

func split(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func matchSegments(glob, file []string) bool {
	for len(glob) > 0 {
		if glob[0] == "**" {
			rest := glob[1:]
			for i := 0; i <= len(file); i++ {
				if matchSegments(rest, file[i:]) {
					return true
				}
			}
			return false
		}
		if len(file) == 0 {
			return false
		}
		ok, err := path.Match(glob[0], file[0])
		if err != nil || !ok {
			return false
		}
		glob, file = glob[1:], file[1:]
	}
	return len(file) == 0
}

// Literal returns the segments of entry before the first one holding a glob character.
func Literal(entry string) []string {
	var out []string
	for _, seg := range split(entry) {
		if strings.ContainsAny(seg, "*?[") {
			break
		}
		out = append(out, seg)
	}
	return out
}

// IsGlob reports whether entry holds any glob character.
func IsGlob(entry string) bool { return strings.ContainsAny(entry, "*?[") }

// Overlap reports whether two touches lists can name the same file, and the first clashing pair
// ("x" when both are the same entry, "x and y" otherwise). It never says no when some file could
// match both: two globs are walked segment by segment, and anything it can't rule out overlaps.
func Overlap(a, b []string, aliases map[string]string) (string, bool) {
	for _, x := range a {
		for _, y := range b {
			if entriesOverlap(Resolve(x, aliases), Resolve(y, aliases)) {
				if x == y {
					return x, true
				}
				return x + " and " + y, true
			}
		}
	}
	return "", false
}

func entriesOverlap(x, y string) bool {
	px, py := Literal(x), Literal(y)
	// Two exact files overlap only when they're the same file.
	if !IsGlob(x) && !IsGlob(y) {
		return strings.Join(px, "/") == strings.Join(py, "/")
	}
	// An exact file against a glob: the glob decides, when the file is deep enough to judge.
	if !IsGlob(x) && len(px) >= len(py) {
		return Match(y, x)
	}
	if !IsGlob(y) && len(py) >= len(px) {
		return Match(x, y)
	}
	if !IsGlob(x) || !IsGlob(y) {
		// An exact file shallower than the glob's literal folders: they overlap when those agree.
		n := min(len(px), len(py))
		for i := 0; i < n; i++ {
			if px[i] != py[i] {
				return false
			}
		}
		return true
	}
	return globsOverlap(split(x), split(y))
}

// globsOverlap walks two globs segment by segment from the root. A "**" in either can stand for any
// depth, so it overlaps. Two segments that can't match the same name don't, and neither do two
// globs of different depths with no "**" left.
func globsOverlap(x, y []string) bool {
	for len(x) > 0 && len(y) > 0 {
		if x[0] == "**" || y[0] == "**" {
			return true
		}
		if !segmentsOverlap(x[0], y[0]) {
			return false
		}
		x, y = x[1:], y[1:]
	}
	for _, seg := range append(x, y...) {
		if seg == "**" {
			return true
		}
	}
	return len(x) == len(y)
}

// segmentsOverlap reports whether two segments can match the same name, judged by their literal
// prefixes and suffixes: a name matching both starts with both prefixes and ends with both suffixes.
func segmentsOverlap(a, b string) bool {
	pa, sa := affixes(a)
	pb, sb := affixes(b)
	if !strings.HasPrefix(pa, pb) && !strings.HasPrefix(pb, pa) {
		return false
	}
	return strings.HasSuffix(sa, sb) || strings.HasSuffix(sb, sa)
}

// affixes splits a segment into the literal text before its first glob token and after its last.
// Tokens are "*", "?", a "\" escape with the character after it, and a whole "[...]" class. A
// segment with no token is all prefix and all suffix. A class with no closing "]" runs to the end.
func affixes(seg string) (prefix, suffix string) {
	first, last := -1, -1
	for i := 0; i < len(seg); i++ {
		start := i
		switch seg[i] {
		case '*', '?':
		case '\\':
			i++
		case '[':
			i = classEnd(seg, i)
		default:
			continue
		}
		if first < 0 {
			first = start
		}
		last = min(i, len(seg)-1)
	}
	if first < 0 {
		return seg, seg
	}
	return seg[:first], seg[last+1:]
}

// classEnd returns the index of the "]" closing the class that opens at seg[open], or the last
// index when it never closes. A "]" right after "[" or "[^" is part of the class, as is an escaped
// one, so the end found is never earlier than the real one.
func classEnd(seg string, open int) int {
	i := open + 1
	if i < len(seg) && (seg[i] == '^' || seg[i] == '!') {
		i++
	}
	if i < len(seg) && seg[i] == ']' {
		i++
	}
	for ; i < len(seg); i++ {
		switch seg[i] {
		case '\\':
			i++
		case ']':
			return i
		}
	}
	return len(seg) - 1
}
