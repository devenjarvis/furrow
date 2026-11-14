package claim_test

import (
	"bytes"
	"encoding/json"
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

func TestReleaseDeletes(t *testing.T) {
	dir, remote := setup(t, S{Num: 1})
	g := gittest.Git(dir)
	if _, err := claim.Take(g, load(t, dir), "FUR-0001.1", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	res, err := claim.Release(g, load(t, dir), "FUR-0001.1")
	if err != nil || !res.Released || res.How != "deleted" || res.Branch != "step/0001-1" {
		t.Fatalf("release = %+v %v", res, err)
	}
	if out := gittest.Must(t, remote, "branch", "--list", "step/0001-1"); out != "" {
		t.Error("branch still on the remote")
	}
	var b bytes.Buffer
	res.Text(&b, "origin")
	if b.String() != "Released FUR-0001.1: deleted step/0001-1 on origin.\n" {
		t.Errorf("text = %q", b.String())
	}
	res, err = claim.Release(g, load(t, dir), "FUR-0001.1")
	if err != nil || res.Released || res.How != "" {
		t.Errorf("releasing an unclaimed step: %+v %v", res, err)
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

// Stacking fixtures: FUR-0090.1 touches internal/a/**, and FUR-0091.1 comes after it and touches
// the same files, so it can only start stacked on it.
var (
	parent090 = S{Num: 90, Steps: []St{{Title: "Parent", Touches: []string{"internal/a/**"}}}}
	child091  = S{Num: 91, Steps: []St{{Title: "Child", After: []string{"FUR-0090.1"}, Touches: []string{"internal/a/**"}}}}
)

// setupStack is setup with stack: 3 in .furrow.yml.
func setupStack(t *testing.T, specs ...S) (string, string) {
	t.Helper()
	r := spectest.New(t)
	r.Write(".furrow.yml", spectest.Config+"stack: 3\n")
	r.Write("internal/.keep", "")
	r.Write("cmd/furrow/.keep", "")
	for _, s := range specs {
		r.Spec(s)
	}
	remote := gittest.Init(t, r.Root)
	return r.Root, remote
}

// handBack claims FUR-0090.1 from dir and pushes furrow done's edit on top: its step marked
// merged: true. It returns the branch's new tip.
func handBack(t *testing.T, dir string) string {
	t.Helper()
	res, err := claim.Take(gittest.Git(dir), load(t, dir), "FUR-0090.1", claim.Options{NoCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	done := parent090
	done.Steps = []St{{Title: "Parent", Touches: []string{"internal/a/**"}, Merged: true}}
	return pushOnTop(t, dir, res.SHA, "step/0090-1", parent090.Path(), spectest.Text(done))
}

func TestClaimStacked(t *testing.T) {
	dir, remote := setupStack(t, parent090, child091)
	g := gittest.Git(dir)
	tip := handBack(t, dir)
	// The parent's branch moves after it's handed back: the claim builds on where it is now.
	tip = pushOnTop(t, dir, tip, "step/0090-1", "internal/a/x.go", "x\n")

	res, err := claim.Take(g, load(t, dir), "FUR-0091.1", claim.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.StackOn != "step/0090-1" || res.BaseBranch != "step/0090-1" || res.Branch != "step/0091-1" {
		t.Errorf("result = %+v", res)
	}
	if onRemote := gittest.Must(t, remote, "rev-parse", "refs/heads/step/0091-1"); onRemote != res.SHA {
		t.Errorf("remote has %s, claim made %s", onRemote, res.SHA)
	}
	if p := gittest.Must(t, remote, "rev-parse", res.SHA+"^"); p != tip {
		t.Errorf("claim commit's parent = %s, want the parent branch's tip %s", p, tip)
	}
	if diff := gittest.Must(t, dir, "diff", "--name-only", tip, res.SHA); diff != "" {
		t.Errorf("claim commit changed files: %s", diff)
	}
	msg := gittest.Must(t, dir, "log", "-1", "--format=%B", res.SHA)
	lines := strings.Split(msg, "\n")
	if lines[0] != "Claim FUR-0091.1: Child" || len(lines) != 4 ||
		!strings.HasPrefix(lines[2], "furrow-claim: ") || lines[3] != "furrow-stack: step/0090-1" {
		t.Errorf("message = %q", msg)
	}
	if cur := gittest.Must(t, dir, "branch", "--show-current"); cur != "step/0091-1" {
		t.Errorf("checked out %q", cur)
	}

	var b bytes.Buffer
	res.Text(&b, "origin", false)
	want := "Claimed FUR-0091.1 on step/0091-1.\nStacked on step/0090-1. Open the PR against that branch.\nOpen a draft PR titled: [FUR-0091.1] Child\n"
	if b.String() != want {
		t.Errorf("text = %q", b.String())
	}
	data, _ := json.Marshal(res)
	if !strings.Contains(string(data), `"base_branch":"step/0090-1"`) {
		t.Errorf("json = %s", data)
	}

	// An unstacked claim's base_branch is the config's base, and its text is as before.
	plain, _ := setup(t, S{Num: 1, Steps: []St{{Title: "The step"}}})
	res, err = claim.Take(gittest.Git(plain), load(t, plain), "FUR-0001.1", claim.Options{NoCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(res)
	if !strings.Contains(string(data), `"base_branch":"main"`) {
		t.Errorf("unstacked json = %s", data)
	}
	b.Reset()
	res.Text(&b, "origin", true)
	want = "Claimed FUR-0001.1 on step/0001-1.\nSwitch to it with: git switch -c step/0001-1 origin/step/0001-1\nOpen a draft PR titled: [FUR-0001.1] The step\n"
	if b.String() != want {
		t.Errorf("unstacked text = %q", b.String())
	}
	if m := gittest.Must(t, plain, "log", "-1", "--format=%B", res.SHA); strings.Contains(m, "furrow-stack") {
		t.Errorf("unstacked message = %q", m)
	}
}

func TestClaimStackedRace(t *testing.T) {
	dir, remote := setupStack(t, parent090, child091)
	handBack(t, dir)
	a, b := gittest.Clone(t, remote), gittest.Clone(t, remote)
	errs := make([]error, 2)
	results := make([]*claim.Result, 2)
	var wg sync.WaitGroup
	for i, d := range []string{a, b} {
		wg.Add(1)
		go func(i int, d string) {
			defer wg.Done()
			repo, err := spec.LoadDir(d)
			if err != nil {
				errs[i] = err
				return
			}
			results[i], errs[i] = claim.Take(gittest.Git(d), repo, "FUR-0091.1", claim.Options{})
		}(i, d)
	}
	wg.Wait()
	wins, lost := 0, 0
	for i, err := range errs {
		var ce *claim.ClaimedError
		switch {
		case err == nil:
			wins++
			if results[i].StackOn != "step/0090-1" {
				t.Errorf("winner wasn't stacked: %+v", results[i])
			}
		case errors.As(err, &ce): // furrow claim exits 3 on a ClaimedError
			lost++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || lost != 1 {
		t.Errorf("wins = %d, lost = %d; want exactly one of each", wins, lost)
	}
}

func TestClaimStackParentGone(t *testing.T) {
	want := "FUR-0091.1 waits on FUR-0090.1, which is no longer handed back. Run furrow next again."
	dir, remote := setupStack(t, parent090, child091)
	g := gittest.Git(dir)
	tip := handBack(t, dir)

	// The parent's tip no longer says merged: true.
	pushOnTop(t, dir, tip, "step/0090-1", parent090.Path(), spectest.Text(parent090))
	if _, err := claim.Take(g, load(t, dir), "FUR-0091.1", claim.Options{NoCheckout: true}); err == nil || err.Error() != want {
		t.Errorf("tip not handed back: err = %v, want %q", err, want)
	}
	// The parent's branch is gone.
	gittest.Must(t, dir, "push", "-q", "origin", "--delete", "refs/heads/step/0090-1")
	if _, err := claim.Take(g, load(t, dir), "FUR-0091.1", claim.Options{NoCheckout: true}); err == nil || err.Error() != want {
		t.Errorf("branch gone: err = %v, want %q", err, want)
	}
	if out := gittest.Must(t, remote, "branch", "--list", "step/0091-1"); out != "" {
		t.Error("a refused claim still pushed a branch")
	}

	// The parent merged on base meanwhile: the step is plain ready and is claimed on base.
	r := &spectest.Repo{T: t, Root: dir}
	done := parent090
	done.Steps = []St{{Title: "Parent", Touches: []string{"internal/a/**"}, Merged: true}}
	r.Write(parent090.Path(), spectest.Text(done))
	gittest.Commit(t, dir, "merge FUR-0090.1")
	gittest.Must(t, dir, "push", "-q", "origin", "main")
	res, err := claim.Take(g, load(t, dir), "FUR-0091.1", claim.Options{NoCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.StackOn != "" || res.BaseBranch != "main" {
		t.Errorf("result = %+v", res)
	}
	if p := gittest.Must(t, remote, "rev-parse", res.SHA+"^"); p != gittest.Must(t, remote, "rev-parse", "main") {
		t.Error("claim commit isn't on base")
	}
	if m := gittest.Must(t, dir, "log", "-1", "--format=%B", res.SHA); strings.Contains(m, "furrow-stack") {
		t.Errorf("message = %q", m)
	}
}

func TestClaimsStackOn(t *testing.T) {
	dir, _ := setupStack(t, parent090, child091)
	g := gittest.Git(dir)
	handBack(t, dir)
	if _, err := claim.Take(g, load(t, dir), "FUR-0091.1", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	statuses, err := claim.Statuses(g, load(t, dir), time.Now(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	claim.StatusText(&b, statuses)
	wantText := "FUR-0090.1  step/0090-1  active    0m\nFUR-0091.1  step/0091-1  active    0m  on step/0090-1\n"
	if b.String() != wantText {
		t.Errorf("text = %q, want %q", b.String(), wantText)
	}
	data, _ := json.Marshal(statuses)
	if !strings.Contains(string(data), `"state":"active","stack_on":"step/0090-1"}`) || strings.Count(string(data), "stack_on") != 1 {
		t.Errorf("json = %s", data)
	}

	// The parent's PR closed without merging: its branch is gone.
	gittest.Must(t, dir, "push", "-q", "origin", "--delete", "refs/heads/step/0090-1")
	statuses, err = claim.Statuses(g, load(t, dir), time.Now(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	b.Reset()
	claim.StatusText(&b, statuses)
	if b.String() != "FUR-0091.1  step/0091-1  active    0m  orphaned (step/0090-1 is gone)\n" {
		t.Errorf("orphaned text = %q", b.String())
	}
	data, _ = json.Marshal(statuses)
	if !strings.Contains(string(data), `"stack_on":"step/0090-1","orphaned":true`) {
		t.Errorf("orphaned json = %s", data)
	}

	// The parent merged and its branch was deleted: not orphaned.
	r := &spectest.Repo{T: t, Root: dir}
	done := parent090
	done.Steps = []St{{Title: "Parent", Touches: []string{"internal/a/**"}, Merged: true}}
	r.Write(parent090.Path(), spectest.Text(done))
	gittest.Commit(t, dir, "merge FUR-0090.1")
	statuses, err = claim.Statuses(g, load(t, dir), time.Now(), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Orphaned || statuses[0].StackOn != "step/0090-1" {
		t.Errorf("merged parent: %+v", statuses)
	}
}
