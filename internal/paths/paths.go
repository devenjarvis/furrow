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
// ("x" when both are the same entry, "x and y" otherwise). Two entries overlap when their literal
// prefixes agree up to the shorter one.
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
	n := min(len(px), len(py))
	for i := 0; i < n; i++ {
		if px[i] != py[i] {
			return false
		}
	}
	return true
}
