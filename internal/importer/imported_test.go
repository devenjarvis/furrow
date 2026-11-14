package importer

import (
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/spec"
)

// AC-10: Both importers mark each spec they write, and the output lints with a warning, not an
// error, for each spec that still needs a ## Before and after.
func TestImportMarksSpecs(t *testing.T) {
	for _, kind := range []string{"fetch", "edition"} {
		t.Run(kind, func(t *testing.T) {
			root := fixtureOf(t, kind)
			run := Fetch
			if kind == "edition" {
				run = Edition
			}
			plan, err := run(opts(t, root))
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Problems) > 0 {
				t.Fatalf("problems: %v", plan.Problems)
			}
			if err := plan.Write(); err != nil {
				t.Fatal(err)
			}
			repo := load(t, root)
			if len(repo.Specs) == 0 {
				t.Fatal("no specs imported")
			}
			needs := 0
			for _, s := range repo.Specs {
				if s.Imported != kind {
					t.Errorf("%s: imported = %q, want %q", s.File, s.Imported, kind)
				}
				if text := read(t, root, s.File); !strings.Contains(text, "\nhuman_review: "+s.HumanReview+"\nimported: "+kind+"\n") &&
					!strings.Contains(text, "\nhuman_review: \""+s.HumanReview+"\"\nimported: "+kind+"\n") {
					t.Errorf("%s: imported: isn't right after human_review:\n%s", s.File, text)
				}
				if !s.Archived && s.Status != spec.Draft && s.MergedSteps() < len(s.Steps) {
					needs++
				}
			}
			if errs := lintErrors(repo); len(errs) != 0 {
				t.Errorf("lint errors: %v", errs)
			}
			warned := 0
			for _, p := range spec.Lint(repo) {
				if p.Warning && p.Message == `missing section "## Before and after"` {
					warned++
				}
			}
			if needs == 0 || warned != needs {
				t.Errorf("%d warnings for a missing Before and after, want %d (and more than 0)", warned, needs)
			}
		})
	}
}
