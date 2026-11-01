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
