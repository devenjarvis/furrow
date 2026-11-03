package lifecycle_test

import (
	"strings"
	"testing"

	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/lifecycle"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

// isolateGit keeps the user's own git config out of signoff's lookup, which runs with the
// process environment.
func isolateGit(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("CLAUDECODE", "")
	t.Setenv("FURROW_AGENT", "")
}

// waitingSpec is a git repo holding FUR-0001 with AC-1 and AC-2 waiting for sign-off, so signing
// off one leaves the spec where it is.
func waitingSpec(t *testing.T) (*spectest.Repo, string) {
	r := spectest.New(t)
	path := r.Spec(S{Num: 1, Criteria: []string{"- [ ] AC-1: Clear (human: look)", "- [ ] AC-2: Calm (human: use it)"}, Steps: []St{{Merged: true}}})
	gittest.Init(t, r.Root)
	return r, path
}

func TestSignoffNameFromGit(t *testing.T) {
	isolateGit(t)

	// No --by: the name comes from git config user.name in the repo.
	r, path := waitingSpec(t)
	gittest.Must(t, r.Root, "config", "user.name", "Ann Lee")
	msg, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-1"}, Getenv: person, Now: day})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "by Ann Lee.") {
		t.Errorf("message: %q", msg)
	}
	if got := r.Read(path); !strings.Contains(got, "- 2026-11-01: AC-1 signed off by Ann Lee.\n") {
		t.Errorf("no sign-off line naming git's user.name:\n%s", got)
	}

	// --by wins over git.
	r, path = waitingSpec(t)
	gittest.Must(t, r.Root, "config", "user.name", "Ann Lee")
	if _, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-1"}, By: "Bo", Getenv: person, Now: day}); err != nil {
		t.Fatal(err)
	}
	if got := r.Read(path); !strings.Contains(got, "AC-1 signed off by Bo.\n") {
		t.Errorf("--by didn't win:\n%s", got)
	}

	// Neither: refuses and changes nothing.
	r, path = waitingSpec(t)
	before := r.Read(path)
	_, err = lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-1"}, Getenv: person, Now: day})
	if err == nil || err.Error() != "Say who checked it: --by NAME." {
		t.Errorf("with no name anywhere: got %v", err)
	}
	if r.Read(path) != before {
		t.Error("signoff changed the file after refusing")
	}

	// Not a git repo at all: same refusal.
	r2 := spectest.New(t)
	p2 := r2.Spec(S{Num: 1, Criteria: []string{"- [ ] AC-1: Clear (human: look)"}, Steps: []St{{Merged: true}}})
	before = r2.Read(p2)
	t.Setenv("GIT_CEILING_DIRECTORIES", r2.Root)
	if _, err := lifecycle.Signoff(r2.Load(), lifecycle.SignoffRequest{ID: "FUR-0001", Criteria: []string{"AC-1"}, Getenv: person, Now: day}); err == nil || err.Error() != "Say who checked it: --by NAME." {
		t.Errorf("outside git: got %v", err)
	}
	if r2.Read(p2) != before {
		t.Error("signoff changed the file outside git")
	}
}

func TestSignoffRefusesAgentsBeforeGit(t *testing.T) {
	isolateGit(t)
	r, path := waitingSpec(t)
	gittest.Must(t, r.Root, "config", "user.name", "Ann Lee")
	before := r.Read(path)
	for _, k := range []string{"CLAUDECODE", "FURROW_AGENT"} {
		asked := false
		_, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{
			ID: "FUR-0001", Criteria: []string{"AC-1"}, Getenv: envOf(map[string]string{k: "1"}), Now: day,
			Name: func() string { asked = true; return "Ann Lee" },
		})
		if err == nil || err.Error() != lifecycle.AgentRefusal {
			t.Errorf("%s set: got %v", k, err)
		}
		if asked {
			t.Errorf("%s set: signoff asked git for a name before refusing", k)
		}
		if r.Read(path) != before {
			t.Errorf("%s set: signoff changed the file", k)
		}
	}
	// A person with no --by: the lookup runs, and its name is used.
	asked := false
	msg, err := lifecycle.Signoff(r.Load(), lifecycle.SignoffRequest{
		ID: "FUR-0001", Criteria: []string{"AC-1"}, Getenv: person, Now: day,
		Name: func() string { asked = true; return "Cy" },
	})
	if err != nil || !asked || !strings.Contains(msg, "by Cy.") {
		t.Errorf("person: asked=%v msg=%q err=%v", asked, msg, err)
	}
}
