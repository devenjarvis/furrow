package importer

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/spec"
)

func editionRepo(t *testing.T, root string) *spec.Repo {
	t.Helper()
	plan, err := Edition(opts(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Problems) > 0 {
		t.Fatalf("problems: %v", plan.Problems)
	}
	if err := plan.Write(); err != nil {
		t.Fatal(err)
	}
	cfg := opts(t, root).Config
	repo, err := spec.Load(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range spec.Lint(repo) {
		if !p.Warning {
			t.Errorf("lint: %s", p)
		}
	}
	return repo
}

// AC-3: Importing a copy of Edition's specs gives a lint-clean tree, and its step lines become steps.
func TestImportEdition(t *testing.T) {
	root := fixtureOf(t, "edition")
	repo := editionRepo(t, root)

	// Numbered in file-name order; Shipped goes to archive/, merged.
	want := map[string]string{
		"EDN-0001": "specs/changes/0001-almanac.md",
		"EDN-0002": "specs/changes/0002-live-mail.md",
		"EDN-0003": "specs/changes/0003-plans.md",
		"EDN-0004": "specs/archive/0004-write-and-send.md",
	}
	for id, file := range want {
		s := repo.Spec(id)
		if s == nil || s.File != file {
			t.Fatalf("%s: got %+v, want it at %s", id, s, file)
		}
	}
	statuses := map[string]string{"EDN-0001": spec.Draft, "EDN-0002": spec.Paused, "EDN-0003": spec.Accepted, "EDN-0004": spec.Done}
	for id, st := range statuses {
		if got := repo.Spec(id).Status; got != st {
			t.Errorf("%s status = %q, want %q", id, got, st)
		}
	}
	if s := repo.Spec("EDN-0004"); !s.Complete() || !s.Archived {
		t.Errorf("shipped spec isn't archived and complete: %+v", s)
	}
	for _, f := range []string{"docs/specs/plans.md", "docs/specs/README.md", "docs/specs/TEMPLATE.md"} {
		if exists(root, f) {
			t.Errorf("%s is still there", f)
		}
	}

	// Step lines become steps: After refs, Touches globs, covers from the criteria named, blocked.
	plans := repo.Spec("EDN-0003")
	if plans.Area != "subscriptions" || plans.Priority != "p1" {
		t.Errorf("area %q priority %q", plans.Area, plans.Priority)
	}
	type step struct {
		after, touches, covers []string
		merged                 bool
		blocked                string
	}
	got := []step{}
	for _, st := range plans.Steps {
		got = append(got, step{st.After, st.Touches, st.Covers, spec.StepMerged(st), st.Blocked})
	}
	wantSteps := []step{
		{[]string{}, []string{"EditionKit/Sources/EditionKit/Access/**", "EditionKit/Tests/EditionKitTests/AccessTests.swift"}, []string{"AC-1", "AC-2"}, true, ""},
		{[]string{"1", "EDN-0004.1"}, []string{"Edition", "EditionKit/Sources/EditionKit/Gate/**"}, []string{"AC-2", "AC-3"}, false, ""},
		{[]string{"2"}, []string{"Edition"}, []string{}, false, "needs the paid membership"},
	}
	if len(got) != len(wantSteps) {
		t.Fatalf("steps = %+v", got)
	}
	for i := range got {
		g, w := got[i], wantSteps[i]
		if !reflect.DeepEqual(nonNil(g.after), w.after) || !reflect.DeepEqual(g.touches, w.touches) ||
			!reflect.DeepEqual(nonNil(g.covers), w.covers) || g.merged != w.merged || g.blocked != w.blocked {
			t.Errorf("step %d = %+v, want %+v", i+1, g, w)
		}
	}
	if plans.Steps[1].Title != "Gate sync on access in the shell" {
		t.Errorf("step 2 title = %q", plans.Steps[1].Title)
	}

	// Criteria: AC-n keeping the letter, a proof, ticked only when every step covering it merged.
	ticks := map[string]bool{}
	for _, c := range plans.Criteria {
		ticks[c.ID] = c.Ticked
		if c.Proof != "test" {
			t.Errorf("%s proof = %q", c.ID, c.Proof)
		}
	}
	if !reflect.DeepEqual(ticks, map[string]bool{"AC-1": true, "AC-2": false, "AC-3": false}) {
		t.Errorf("ticks = %v", ticks)
	}
	text := read(t, root, plans.File)
	for _, line := range []string{
		"- [x] AC-1: S1. Yearly and Monthly are in one group. (test: s1…)",
		"- [ ] AC-2: S2. Prices come from StoreKit, as [the outbox](../archive/0004-write-and-send.md) does for retries. (test: s2…)",
		"See specs/archive/0004-write-and-send.md for the outbox.",
		"- **Depends on:** `../archive/0004-write-and-send.md` step 1 for step 2",
		"- **Status when imported:** Building. Step 1 done; step 3 waits for the paid membership",
		"  - docs/decisions/0010-subscription-trial.md",
		"  - specs/archive/0004-write-and-send.md",
		"- 2026-11-03: Imported from docs/specs/plans.md. Criteria S1–S3 are AC-1–AC-3, in order, and each one's text starts with its letter.",
		"2. Gate sync on access in the shell (S2, S3). It also adds\n   a Settings line.",
	} {
		if !strings.Contains(text, line) {
			t.Errorf("%s is missing %q", plans.File, line)
		}
	}

	// Answered open questions move to Decisions; open ones stay.
	if q := spec.SectionText(plans.Body, "Open questions"); strings.Contains(q, "Product IDs") || !strings.Contains(q, "When the trial starts") {
		t.Errorf("Open questions = %q", q)
	}
	if d := spec.SectionText(plans.Body, "Decisions"); !strings.Contains(d, "- **Product IDs:** final once created.") {
		t.Errorf("Decisions = %q", d)
	}

	// A wrapped criterion is joined onto one line.
	if !strings.Contains(read(t, root, "specs/archive/0004-write-and-send.md"),
		"- [x] AC-2: W2. A message that can't be sent waits in the outbox and is sent once the desk is back online. (test: w2…)") {
		t.Error("W2 wasn't joined onto one line")
	}

	// A paused spec's open step with no step line is blocked by the pause; drafts after 1.0 are p2.
	if b := repo.Spec("EDN-0002").Steps[1].Blocked; !strings.Contains(b, "needs a paid Apple Developer Program membership") {
		t.Errorf("paused step blocked = %q", b)
	}
	if p := repo.Spec("EDN-0001").Priority; p != "p2" {
		t.Errorf("almanac priority = %q", p)
	}

	// Areas: History for the archived spec, Furrow's standard in specs/.
	if !strings.Contains(read(t, root, "specs/areas/mail.md"), "- EDN-0004: Write and send (archived 2026-11-03, imported)") {
		t.Error("mail.md has no History line for EDN-0004")
	}
	if !strings.Contains(read(t, root, "specs/README.md"), "# The spec standard") {
		t.Error("specs/README.md isn't Furrow's")
	}
}

// AC-3: the dry run writes nothing, and a missing --areas or a broken step line is refused or listed.
func TestImportEditionRefusals(t *testing.T) {
	root := fixtureOf(t, "edition")
	before := snapshot(t, root)
	o := opts(t, root)
	plan, err := Edition(o)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	plan.Text(&out, false)
	if !strings.Contains(out.String(), "4 Edition specs") || !strings.Contains(out.String(), "Removes docs/specs/README.md and docs/specs/TEMPLATE.md.") ||
		!strings.Contains(out.String(), "Dry run: nothing written.") {
		t.Errorf("dry run said:\n%s", out.String())
	}
	if !reflect.DeepEqual(snapshot(t, root), before) {
		t.Error("the dry run changed files")
	}

	o.AreasFile = ""
	if _, err := Edition(o); err == nil || err.Error() != NoAreas {
		t.Errorf("no --areas: %v", err)
	}

	path := filepath.Join(root, "docs", "specs", "plans.md")
	data, _ := os.ReadFile(path)
	broken := strings.Replace(string(data), "**After:** 1, write-and-send 1", "**After:** 1, sending 1", 1)
	broken = strings.Replace(broken, "**Area:** shared", "**Area:** nowhere", 1)
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "areas.yml"), []byte("areas:\n  - area: mail\n    title: Mail\n    paths: []\n  - area: relay\n    title: Relay\n    paths: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err = Edition(opts(t, root))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Problems, "\n")
	for _, want := range []string{
		"EDN-0003 (docs/specs/plans.md): step 2 is after sending 1, and there's no docs/specs/sending.md",
		"EDN-0003 (docs/specs/plans.md) matches no area.",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems lack %q:\n%s", want, joined)
		}
	}
	if err := plan.Write(); err == nil || !strings.Contains(err.Error(), "in Edition's format") {
		t.Errorf("write with problems: %v", err)
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
