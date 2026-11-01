package claim_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/spec"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

type S = spectest.Spec
type St = spectest.Step

// setup writes specs, commits them, and returns the repo dir and its bare remote.
func setup(t *testing.T, specs ...S) (string, string) {
	t.Helper()
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	r.Write("cmd/furrow/.keep", "")
	for _, s := range specs {
		r.Spec(s)
	}
	remote := gittest.Init(t, r.Root)
	return r.Root, remote
}

func load(t *testing.T, dir string) *spec.Repo {
	t.Helper()
	repo, err := spec.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestBranchAndList(t *testing.T) {
	cfg := config.Default()
	cfg.Prefix = "FUR"
	if got := claim.Branch(cfg, spec.Ref{Spec: "FUR-0003", Step: 1}); got != "step/0003-1" {
		t.Errorf("Branch = %q", got)
	}
	if ref, ok := claim.Parse(cfg, "step/0012-3"); !ok || ref != (spec.Ref{Spec: "FUR-0012", Step: 3}) {
		t.Errorf("Parse = %v %v", ref, ok)
	}
	if _, ok := claim.Parse(cfg, "step/12-3"); ok {
		t.Error("Parse accepted a branch that doesn't match")
	}

	dir, _ := setup(t, S{Num: 1})
	for _, b := range []string{"step/0001-1", "step/0002-2", "feature/x", "step/abc-1"} {
		gittest.Must(t, dir, "push", "-q", "origin", "main:refs/heads/"+b)
	}
	repo := load(t, dir)
	claims, err := claim.List(gittest.Git(dir), repo.Config)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range claims {
		got = append(got, c.Ref.String()+"="+c.Branch)
	}
	if strings.Join(got, " ") != "FUR-0001.1=step/0001-1 FUR-0002.2=step/0002-2" {
		t.Errorf("List = %v", got)
	}

	bad := repo.Config
	bad.Remote = filepath.Join(t.TempDir(), "missing.git")
	if _, err := claim.List(gittest.Git(dir), bad); !errors.Is(err, claim.ErrRemote) {
		t.Errorf("unreachable remote: err = %v, want ErrRemote", err)
	}
}

func TestClaimCreatesBranch(t *testing.T) {
	dir, remote := setup(t, S{Num: 1, Steps: []St{{Title: "The step"}}})
	res, err := claim.Take(gittest.Git(dir), load(t, dir), "FUR-0001.1", claim.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Branch != "step/0001-1" || res.PRTitle != "[FUR-0001.1] The step" {
		t.Errorf("result = %+v", res)
	}
	if cur := gittest.Must(t, dir, "branch", "--show-current"); cur != "step/0001-1" {
		t.Errorf("checked out %q", cur)
	}
	onRemote := gittest.Must(t, remote, "rev-parse", "refs/heads/step/0001-1")
	if onRemote != res.SHA {
		t.Errorf("remote has %s, claim made %s", onRemote, res.SHA)
	}
	// The claim commit sits on base and changes nothing.
	parent := gittest.Must(t, remote, "rev-parse", res.SHA+"^")
	if parent != gittest.Must(t, remote, "rev-parse", "main") {
		t.Error("claim commit isn't on base")
	}
	if diff := gittest.Must(t, dir, "diff", "--name-only", "main", res.SHA); diff != "" {
		t.Errorf("claim commit changed files: %s", diff)
	}
	if up := gittest.Must(t, dir, "rev-parse", "--abbrev-ref", "@{upstream}"); up != "origin/step/0001-1" {
		t.Errorf("upstream = %q", up)
	}
}

func TestClaimRace(t *testing.T) {
	_, remote := setup(t, S{Num: 1})
	a, b := gittest.Clone(t, remote), gittest.Clone(t, remote)
	repos := []string{a, b}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, dir := range repos {
		wg.Add(1)
		go func(i int, dir string) {
			defer wg.Done()
			repo, err := spec.LoadDir(dir)
			if err != nil {
				errs[i] = err
				return
			}
			_, errs[i] = claim.Take(gittest.Git(dir), repo, "FUR-0001.1", claim.Options{})
		}(i, dir)
	}
	wg.Wait()
	wins, lost := 0, 0
	for _, err := range errs {
		var ce *claim.ClaimedError
		switch {
		case err == nil:
			wins++
		case errors.As(err, &ce):
			lost++
			if err.Error() != "FUR-0001.1 is already claimed (step/0001-1). Run `furrow next` for another step." {
				t.Errorf("message = %q", err)
			}
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || lost != 1 {
		t.Errorf("wins = %d, lost = %d; want exactly one of each", wins, lost)
	}
	// Claiming again from a clone that already lost still loses.
	_, err := claim.Take(gittest.Git(a), load(t, a), "FUR-0001.1", claim.Options{NoCheckout: true})
	var ce *claim.ClaimedError
	if !errors.As(err, &ce) {
		t.Errorf("a second claim should lose, got %v", err)
	}
}

func TestClaimRefusals(t *testing.T) {
	dir, remote := setup(t,
		S{Num: 1, Steps: []St{{Touches: []string{"internal/a/**"}}}},
		S{Num: 2, Steps: []St{{After: []string{"FUR-0001.1"}}}},
		S{Num: 3, Steps: []St{{Blocked: "needs a key"}}},
		S{Num: 4, Steps: []St{{Touches: []string{"internal/a/x.go"}}}},
		S{Num: 5, Status: "draft"},
	)
	g := gittest.Git(dir)
	want := map[string]string{
		"FUR-0002.1": "FUR-0002.1 waits on FUR-0001.1.",
		"FUR-0003.1": "FUR-0003.1 is blocked: needs a key",
		"FUR-0005.1": "FUR-0005.1 isn't open: its spec is draft.",
	}
	for ref, msg := range want {
		if _, err := claim.Take(g, load(t, dir), ref, claim.Options{NoCheckout: true}); err == nil || err.Error() != msg {
			t.Errorf("%s: err = %v, want %q", ref, err, msg)
		}
	}
	// Someone holds FUR-0001.1; FUR-0004.1 overlaps it.
	other := gittest.Clone(t, remote)
	if _, err := claim.Take(gittest.Git(other), load(t, other), "FUR-0001.1", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	_, err := claim.Take(g, load(t, dir), "FUR-0004.1", claim.Options{NoCheckout: true})
	if err == nil || !strings.Contains(err.Error(), "shares internal/a/x.go and internal/a/** with FUR-0001.1") {
		t.Errorf("overlap: err = %v", err)
	}
	// A dirty tree is refused before anything is pushed.
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = claim.Take(g, load(t, dir), "FUR-0004.1", claim.Options{Force: true})
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Errorf("dirty tree: err = %v", err)
	}
	if out := gittest.Must(t, remote, "branch", "--list", "step/0004-1"); out != "" {
		t.Error("a refused claim still pushed a branch")
	}
	// --force skips only the overlap check.
	if _, err := claim.Take(g, load(t, dir), "FUR-0004.1", claim.Options{Force: true, NoCheckout: true}); err != nil {
		t.Errorf("--force: %v", err)
	}
}

func TestRelease(t *testing.T) {
	dir, remote := setup(t, S{Num: 1})
	g := gittest.Git(dir)
	if _, err := claim.Take(g, load(t, dir), "FUR-0001.1", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	branch, existed, err := claim.Release(g, load(t, dir), "FUR-0001.1")
	if err != nil || !existed || branch != "step/0001-1" {
		t.Fatalf("release = %q %v %v", branch, existed, err)
	}
	if out := gittest.Must(t, remote, "branch", "--list", "step/0001-1"); out != "" {
		t.Error("branch still on the remote")
	}
	if _, existed, err := claim.Release(g, load(t, dir), "FUR-0001.1"); err != nil || existed {
		t.Errorf("releasing an unclaimed step: %v %v", existed, err)
	}
}

func TestClaimsAges(t *testing.T) {
	dir, _ := setup(t,
		S{Num: 1, Steps: []St{{Merged: true}, {}}},
		S{Num: 2},
	)
	g := gittest.Git(dir)
	// A leftover claim on a merged step, and two live claims.
	gittest.Must(t, dir, "push", "-q", "origin", "main:refs/heads/step/0001-1")
	if _, err := claim.Take(g, load(t, dir), "FUR-0001.2", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := claim.Take(g, load(t, dir), "FUR-0002.1", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	statuses, err := claim.Statuses(g, load(t, dir), time.Now().Add(2*time.Hour), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range statuses {
		got = append(got, s.Ref+"="+s.State)
	}
	if strings.Join(got, " ") != "FUR-0001.1=leftover FUR-0001.2=stale FUR-0002.1=stale" {
		t.Errorf("states = %v", got)
	}
	statuses, _ = claim.Statuses(g, load(t, dir), time.Now(), 24*time.Hour)
	if statuses[1].State != "active" {
		t.Errorf("fresh claim = %s", statuses[1].State)
	}
	var b bytes.Buffer
	claim.StatusText(&b, statuses)
	if !strings.Contains(b.String(), "FUR-0001.1  step/0001-1  leftover  0m  (merged: release it)") {
		t.Errorf("text:\n%s", b.String())
	}
}
