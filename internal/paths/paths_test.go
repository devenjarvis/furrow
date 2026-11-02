package paths

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		glob, file string
		want       bool
	}{
		{"internal/**", "internal/spec/x.go", true},
		{"internal/**", "internal", true},
		{"internal/**", "cmd/x.go", false},
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "c.go", true},
		{"cmd/furrow/*.go", "cmd/furrow/next.go", true},
		{"cmd/furrow/*.go", "cmd/furrow/sub/next.go", false},
		{"cmd/furrow/n?xt.go", "cmd/furrow/next.go", true},
		{"go.mod", "go.mod", true},
		{"go.mod", "go.sum", false},
		{"a/**/z.txt", "a/z.txt", true},
		{"a/**/z.txt", "a/b/c/z.txt", true},
	}
	for _, c := range cases {
		if got := Match(c.glob, c.file); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.glob, c.file, got, c.want)
		}
	}
}

func TestOverlap(t *testing.T) {
	aliases := map[string]string{"cli": "cmd/furrow"}
	cases := []struct {
		a, b []string
		want bool
		pair string
	}{
		{[]string{"internal/**"}, []string{"internal/spec/x.go"}, true, "internal/** and internal/spec/x.go"},
		{[]string{"internal/spec/**"}, []string{"internal/schedule/**"}, false, ""},
		{[]string{"cmd/furrow/next.go"}, []string{"cmd/furrow/claim.go"}, false, ""},
		{[]string{"cmd/furrow/next.go"}, []string{"cmd/furrow/next.go"}, true, "cmd/furrow/next.go"},
		{[]string{"cli/next.go"}, []string{"cmd/furrow/*.go"}, true, "cli/next.go and cmd/furrow/*.go"},
		{[]string{"cli"}, []string{"cmd/furrow/next.go"}, true, "cli and cmd/furrow/next.go"}, // a bare alias is its folder
		{[]string{"cli/**"}, []string{"cmd/furrow/next.go"}, true, "cli/** and cmd/furrow/next.go"},
		{[]string{"go.mod", "internal/a/**"}, []string{"internal/b/**", "go.mod"}, true, "go.mod"},
		{[]string{"cmd/furrow/*.go"}, []string{"cmd/furrow/sub/x.go"}, false, ""},
	}
	for _, c := range cases {
		pair, got := Overlap(c.a, c.b, aliases)
		if got != c.want || pair != c.pair {
			t.Errorf("Overlap(%v, %v) = %q, %v; want %q, %v", c.a, c.b, pair, got, c.pair, c.want)
		}
	}
	if Resolve("cli", aliases) != "cmd/furrow/**" || Resolve("cli/x.go", aliases) != "cmd/furrow/x.go" {
		t.Error("Resolve didn't expand the alias")
	}
}

func TestOverlapSegments(t *testing.T) {
	aliases := map[string]string{"cli": "cmd/furrow", "brief": "internal/brief"}
	cases := []struct {
		a, b string
		want bool
	}{
		// The table in FUR-0013's ## What.
		{"internal/brief/brief*", "internal/brief/check*", false},
		{"internal/**/*.go", "internal/**/*.md", true},
		{"internal/*.go", "internal/*.md", false},
		{"internal/brief/**", "internal/brief/check*", true},
		{"skills/*", "skills/furrow-write/**", true},
		{"cmd/*/main.go", "cmd/furrow/*.go", true},
		{"internal/a*/x", "internal/b*/x", false},

		// A ** before the first disagreement still overlaps, in either pattern.
		{"**/a*.go", "internal/b*.go", true},
		{"internal/**/a*", "internal/x/b*", true},
		{"internal/x/b*", "internal/**/a*", true},
		{"a/**", "a/b*", true},
		{"a/b*", "a/**", true},
		{"a/*/**", "a/b*", true},     // both match a/bx
		{"a/**", "a/b/c/*", true},    // ** at any depth
		{"**", "x/y*", true},         // ** at the root
		{"x/y*/**", "x/z*/q", false}, // the disagreement comes before the **

		// Depths without ** can't meet.
		{"a/*", "a/*/*", false},
		{"a/*/b*", "a/*", false},
		{"a/b*", "a/b*/c*", false},
		{"a/*", "a/*/**", true}, // a ** left in the longer one
		{"a/*", "a/*", true},

		// Literal prefixes and suffixes in one folder.
		{"cmd/furrow/*.go", "cmd/furrow/*.md", false},
		{"cmd/furrow/*_test.go", "cmd/furrow/*.go", true},
		{"cmd/furrow/next*.go", "cmd/furrow/n*.go", true},
		{"cmd/furrow/next*", "cmd/furrow/claim*", false},
		{"a/x*y", "a/xy*", true},
		{"a/ab*", "a/a*b", true}, // "ab" + "b" could be "abb"

		// ? and [ count as glob characters; [...] is judged only by what's outside it.
		{"a/b?c", "a/b*", true},
		{"a/b?c", "a/x?c", false},
		{"a/b?c", "a/b?d", false},
		{"a/[ab]x", "a/[cd]x", true}, // classes aren't compared: stay conservative
		{"a/[ab]x", "a/*y", false},
		{"a/q[ab]", "a/r*", false},
		{"a/[]]x", "a/*]x", true},   // a ] right after [ belongs to the class
		{"a/[\\]]x", "a/*]x", true}, // so does an escaped one
		{"a/[!x]z", "a/*z", true},
		{"a/x[", "a/y*", false}, // an unclosed class has no suffix, but the prefixes x and y still differ
		{"a/x[", "a/*z", true},  // ...and with no suffix to judge, it stays conservative
		{"a/\\*x", "a/*x", true},

		// Aliases are resolved before comparing.
		{"cli/*.go", "cmd/furrow/*.md", false},
		{"cli/*.go", "cmd/furrow/n*.go", true},
		{"brief/brief*", "internal/brief/check*", false},
		{"brief", "internal/brief/check*", true}, // a bare alias is its whole folder
		{"cli", "internal/*", false},
	}
	for _, c := range cases {
		for _, pair := range [][2]string{{c.a, c.b}, {c.b, c.a}} {
			if _, got := Overlap([]string{pair[0]}, []string{pair[1]}, aliases); got != c.want {
				t.Errorf("Overlap(%q, %q) = %v, want %v", pair[0], pair[1], got, c.want)
			}
		}
	}
}
