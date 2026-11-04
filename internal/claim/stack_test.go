package claim_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

// pushClaim makes a claim commit for ref on top of from, with a furrow-stack trailer when stackOn
// is set, pushes it as branch, and returns the commit.
func pushClaim(t *testing.T, dir, from, ref, branch, stackOn string) string {
	t.Helper()
	msg := "Claim " + ref + ": a step\n\nfurrow-claim: 0123456789abcdef"
	if stackOn != "" {
		msg += "\nfurrow-stack: " + stackOn
	}
	sha := gittest.Must(t, dir, "commit-tree", from+"^{tree}", "-p", from, "-m", msg)
	gittest.Must(t, dir, "push", "-q", "origin", sha+":refs/heads/"+branch)
	return sha
}

// pushOnTop commits a change to one file on top of from, pushes it as branch, and returns it.
func pushOnTop(t *testing.T, dir, from, branch, file, content string) string {
	t.Helper()
	gittest.Must(t, dir, "switch", "-q", "--detach", from)
	r := &spectest.Repo{T: t, Root: dir}
	if content == "" {
		gittest.Must(t, dir, "rm", "-q", file)
	} else {
		r.Write(file, content)
	}
	gittest.Commit(t, dir, "work on "+branch)
	sha := gittest.Must(t, dir, "rev-parse", "HEAD")
	gittest.Must(t, dir, "push", "-q", "-f", "origin", sha+":refs/heads/"+branch)
	gittest.Must(t, dir, "switch", "-q", "main")
	return sha
}

func TestHandedBack(t *testing.T) {
	one := S{Num: 1, Steps: []St{{}, {}}}
	dir, _ := setup(t, one, S{Num: 2}, S{Num: 3}, S{Num: 4})
	repo := load(t, dir)

	// FUR-0001.1: furrow done ran on its branch, so the tip says merged: true.
	c1 := pushClaim(t, dir, "main", "FUR-0001.1", "step/0001-1", "")
	done := one
	done.Steps = []St{{Merged: true}, {}}
	tip1 := pushOnTop(t, dir, c1, "step/0001-1", one.Path(), spectest.Text(done))
	// FUR-0001.2: claimed, still merged: false at the tip, stacked on step/0001-1.
	pushClaim(t, dir, tip1, "FUR-0001.2", "step/0001-2", "step/0001-1")
	// FUR-0002.1: its tip has no spec file at all.
	c2 := pushClaim(t, dir, "main", "FUR-0002.1", "step/0002-1", "")
	pushOnTop(t, dir, c2, "step/0002-1", S{Num: 2}.Path(), "")
	// FUR-0003.1: its tip's spec doesn't parse.
	c3 := pushClaim(t, dir, "main", "FUR-0003.1", "step/0003-1", "")
	pushOnTop(t, dir, c3, "step/0003-1", S{Num: 3}.Path(), "---\nid: [\n---\n")
	// FUR-0004.1: claimed, nothing done yet.
	pushClaim(t, dir, "main", "FUR-0004.1", "step/0004-1", "")

	g := gittest.Git(dir)
	claims, err := claim.List(g, repo.Config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := claim.ReadStack(g, repo, claims)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		back    bool
		stackOn string
	}{
		"FUR-0001.1": {true, ""},
		"FUR-0001.2": {false, "step/0001-1"},
		"FUR-0002.1": {false, ""},
		"FUR-0003.1": {false, ""},
		"FUR-0004.1": {false, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("claims = %+v", got)
	}
	for _, c := range got {
		w := want[c.Ref.String()]
		if c.HandedBack != w.back || c.StackOn != w.stackOn {
			t.Errorf("%s: handed back %v, stack on %q; want %v, %q", c.Ref, c.HandedBack, c.StackOn, w.back, w.stackOn)
		}
	}
	// The tips were fetched where furrow claims reads them.
	if sha := gittest.Must(t, dir, "rev-parse", "refs/furrow/claims/step/0001-1"); sha != tip1 {
		t.Errorf("refs/furrow/claims/step/0001-1 = %s, want %s", sha, tip1)
	}

	// The claim commit is found through later work, and only for its own step.
	sha, stackOn, err := claim.ClaimCommit(g, "refs/furrow/claims/step/0001-2", spec.Ref{Spec: "FUR-0001", Step: 1})
	if err != nil || sha != c1 || stackOn != "" {
		t.Errorf("ClaimCommit(0001-2 tip, FUR-0001.1) = %s %q %v, want %s", sha, stackOn, err, c1)
	}
	if sha, _, _ := claim.ClaimCommit(g, "main", spec.Ref{Spec: "FUR-0001", Step: 1}); sha != "" {
		t.Errorf("main has no claim commit, got %s", sha)
	}

	bad := *repo
	bad.Config.Remote = filepath.Join(t.TempDir(), "missing.git")
	if _, err := claim.ReadStack(g, &bad, claims); !errors.Is(err, claim.ErrRemote) {
		t.Errorf("unreachable remote: err = %v, want ErrRemote", err)
	}
}
