package claim_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devenjarvis/furrow/internal/claim"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/schedule"
	"github.com/devenjarvis/furrow/internal/spec"
)

// cloudRemote makes the bare remote behave like a cloud agent's: it refuses deletes and
// non-fast-forward pushes, so every push furrow makes must be a fast-forward.
func cloudRemote(t *testing.T, remote string) {
	t.Helper()
	gittest.Must(t, remote, "config", "receive.denyDeletes", "true")
	gittest.Must(t, remote, "config", "receive.denyNonFastForwards", "true")
}

// claimAndMark claims FUR-0001.1 from dir without checking it out, and releases it against a
// remote that refuses deletes. It returns the claim's tip and the release commit.
func claimAndMark(t *testing.T, dir string) (string, string) {
	t.Helper()
	g := gittest.Git(dir)
	res, err := claim.Take(g, load(t, dir), "FUR-0001.1", claim.Options{NoCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := claim.Release(g, load(t, dir), "FUR-0001.1")
	if err != nil || rel.How != "marked" {
		t.Fatalf("release = %+v %v", rel, err)
	}
	return res.SHA, gittest.Must(t, dir, "ls-remote", "origin", "refs/heads/step/0001-1")[:40]
}

func TestReleaseMarksWhenDeleteRefused(t *testing.T) {
	dir, remote := setup(t, S{Num: 1})
	cloudRemote(t, remote)
	g := gittest.Git(dir)
	res, err := claim.Take(g, load(t, dir), "FUR-0001.1", claim.Options{NoCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := claim.Release(g, load(t, dir), "FUR-0001.1")
	if err != nil {
		t.Fatal(err)
	}
	if !rel.Released || rel.How != "marked" {
		t.Fatalf("release = %+v", rel)
	}
	tip := gittest.Must(t, remote, "rev-parse", "refs/heads/step/0001-1")
	if tip == res.SHA {
		t.Fatal("no release commit was pushed")
	}
	if parents := gittest.Must(t, remote, "rev-list", "--parents", "-n1", tip); parents != tip+" "+res.SHA {
		t.Errorf("release commit's parents = %q, want the old tip %s", parents, res.SHA)
	}
	if gittest.Must(t, remote, "rev-parse", tip+"^{tree}") != gittest.Must(t, remote, "rev-parse", res.SHA+"^{tree}") {
		t.Error("release commit changed the tree")
	}
	msg := gittest.Must(t, remote, "log", "-1", "--format=%B", tip)
	lines := strings.Split(msg, "\n")
	if len(lines) != 3 || lines[0] != "Release FUR-0001.1" || lines[1] != "" ||
		!strings.HasPrefix(lines[2], claim.ReleaseTrailer+": ") || len(strings.TrimPrefix(lines[2], "furrow-release: ")) != 16 {
		t.Errorf("message = %q", msg)
	}
	var b bytes.Buffer
	rel.Text(&b, "origin")
	want := "Released FUR-0001.1: origin wouldn't delete step/0001-1, so furrow marked it released.\n" +
		"Delete the branch when you can; until then furrow treats it as free.\n"
	if b.String() != want {
		t.Errorf("text:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestReleaseAlreadyReleased(t *testing.T) {
	dir, remote := setup(t, S{Num: 1, Steps: []St{{}, {}}})
	cloudRemote(t, remote)
	g := gittest.Git(dir)
	_, marked := claimAndMark(t, dir)

	rel, err := claim.Release(g, load(t, dir), "FUR-0001.1")
	if err != nil || rel.Released || rel.How != "" || !rel.WasReleased {
		t.Fatalf("releasing a released step = %+v %v", rel, err)
	}
	if tip := gittest.Must(t, remote, "rev-parse", "refs/heads/step/0001-1"); tip != marked {
		t.Errorf("releasing again pushed %s", tip)
	}
	var b bytes.Buffer
	rel.Text(&b, "origin")
	if b.String() != "FUR-0001.1 isn't claimed (step/0001-1 on origin is released).\n" {
		t.Errorf("text = %q", b.String())
	}

	rel, err = claim.Release(g, load(t, dir), "FUR-0001.2")
	if err != nil || rel.Released || rel.How != "" || rel.WasReleased {
		t.Fatalf("releasing an unclaimed step = %+v %v", rel, err)
	}
	b.Reset()
	rel.Text(&b, "origin")
	if b.String() != "FUR-0001.2 isn't claimed (no step/0001-2 on origin).\n" {
		t.Errorf("text = %q", b.String())
	}
}

func TestReleaseLeaseLost(t *testing.T) {
	dir, remote := setup(t, S{Num: 1})
	cloudRemote(t, remote)
	g := gittest.Git(dir)
	res, err := claim.Take(g, load(t, dir), "FUR-0001.1", claim.Options{NoCheckout: true})
	if err != nil {
		t.Fatal(err)
	}
	// Someone else's work on the branch, already on the remote but not yet its tip.
	work := gittest.Must(t, dir, "commit-tree", res.SHA+"^{tree}", "-p", res.SHA, "-m", "more work")
	gittest.Must(t, dir, "push", "-q", "origin", work+":refs/heads/elsewhere")
	// The branch moves after furrow's ls-remote: when furrow tries the delete, the remote's
	// branch is moved on, and then the delete is refused.
	hook := "#!/bin/sh\n" +
		"if grep -q '^(delete)' ; then git --git-dir='" + remote + "' update-ref refs/heads/step/0001-1 " + work + "; fi\n"
	if err := os.WriteFile(filepath.Join(dir, ".git", "hooks", "pre-push"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = claim.Release(g, load(t, dir), "FUR-0001.1")
	if err == nil || err.Error() != "step/0001-1 moved while furrow was releasing it. Run furrow release FUR-0001.1 again." {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, claim.ErrRemote) {
		t.Error("a lost lease isn't ErrRemote")
	}
	if tip := gittest.Must(t, remote, "rev-parse", "refs/heads/step/0001-1"); tip != work {
		t.Errorf("remote tip = %s, want the moved tip %s untouched", tip, work)
	}
	if n := gittest.Must(t, remote, "log", "--all", "--format=%H", "--grep=furrow-release:"); n != "" {
		t.Errorf("a release commit reached the remote: %s", n)
	}
}

func TestListDropsReleased(t *testing.T) {
	dir, remote := setup(t, S{Num: 1, Steps: []St{{}, {}}})
	cloudRemote(t, remote)
	g := gittest.Git(dir)
	// FUR-0001.1 is released: its tip is a release commit.
	claimAndMark(t, dir)
	// FUR-0001.2 was released and claimed again: a release commit deeper in its history.
	c2 := pushClaim(t, dir, "main", "FUR-0001.2", "step/0001-2", "")
	rel := gittest.Must(t, dir, "commit-tree", c2+"^{tree}", "-p", c2, "-m", "Release FUR-0001.2\n\nfurrow-release: 0123456789abcdef")
	gittest.Must(t, dir, "push", "-q", "origin", rel+":refs/heads/step/0001-2")
	pushOnTop(t, dir, rel, "step/0001-2", "internal/x.go", "x\n")

	repo := load(t, dir)
	claims, err := claim.List(g, repo.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].Ref.String() != "FUR-0001.2" {
		t.Errorf("List = %+v, want only FUR-0001.2", claims)
	}
	all, err := claim.ListAll(g, repo.Config)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range all {
		got = append(got, l.Ref.String()+"="+map[bool]string{true: "released", false: "held"}[l.Released])
	}
	if strings.Join(got, " ") != "FUR-0001.1=released FUR-0001.2=held" {
		t.Errorf("ListAll = %v", got)
	}

	bad := repo.Config
	bad.Remote = filepath.Join(t.TempDir(), "missing.git")
	if _, err := claim.ListAll(g, bad); !errors.Is(err, claim.ErrRemote) {
		t.Errorf("unreachable remote: err = %v, want ErrRemote", err)
	}
}

// ready lists the refs furrow next offers as ready to start, read the way it reads them.
func ready(t *testing.T, dir string) []string {
	t.Helper()
	repo := load(t, dir)
	claims, err := claim.List(gittest.Git(dir), repo.Config)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, it := range schedule.Build(repo, claims, true, 0).Ready {
		out = append(out, it.Ref())
	}
	sort.Strings(out)
	return out
}

func TestNextOffersReleasedStep(t *testing.T) {
	dir, remote := setup(t, S{Num: 1})
	cloudRemote(t, remote)
	g := gittest.Git(dir)
	if _, err := claim.Take(g, load(t, dir), "FUR-0001.1", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	if got := ready(t, dir); len(got) != 0 {
		t.Fatalf("ready while claimed = %v", got)
	}
	if rel, err := claim.Release(g, load(t, dir), "FUR-0001.1"); err != nil || rel.How != "marked" {
		t.Fatalf("release = %+v %v", rel, err)
	}
	if got := ready(t, dir); strings.Join(got, " ") != "FUR-0001.1" {
		t.Errorf("ready after release = %v, want FUR-0001.1", got)
	}
}

func TestClaimReleasedBranch(t *testing.T) {
	dir, remote := setup(t, S{Num: 1, Steps: []St{{Title: "The step"}}})
	cloudRemote(t, remote)
	_, marked := claimAndMark(t, dir)

	other := gittest.Clone(t, remote)
	res, err := claim.Take(gittest.Git(other), load(t, other), "FUR-0001.1", claim.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Branch != "step/0001-1" || res.PRTitle != "[FUR-0001.1] The step" || res.BaseBranch != "main" {
		t.Errorf("result = %+v", res)
	}
	if tip := gittest.Must(t, remote, "rev-parse", "refs/heads/step/0001-1"); tip != res.SHA {
		t.Errorf("remote has %s, claim made %s", tip, res.SHA)
	}
	base := gittest.Must(t, other, "rev-parse", "origin/main")
	if parents := gittest.Must(t, other, "rev-list", "--parents", "-n1", res.SHA); parents != res.SHA+" "+base+" "+marked {
		t.Errorf("parents = %q, want base %s then the released tip %s", parents, base, marked)
	}
	if gittest.Must(t, other, "rev-parse", res.SHA+"^{tree}") != gittest.Must(t, other, "rev-parse", base+"^{tree}") {
		t.Error("claim commit's tree isn't base's")
	}
	if cur := gittest.Must(t, other, "branch", "--show-current"); cur != "step/0001-1" {
		t.Errorf("checked out %q", cur)
	}
	if diff := gittest.Must(t, other, "diff", "--name-only", "origin/main...HEAD"); diff != "" {
		t.Errorf("git diff origin/main...HEAD = %s", diff)
	}
	if sha, _, err := claim.ClaimCommit(gittest.Git(other), "HEAD", spec.Ref{Spec: "FUR-0001", Step: 1}); err != nil || sha != res.SHA {
		t.Errorf("ClaimCommit = %s %v, want the new claim %s", sha, err, res.SHA)
	}
	claims, err := claim.List(gittest.Git(other), load(t, other).Config)
	if err != nil || len(claims) != 1 {
		t.Errorf("claimed again but List = %+v %v", claims, err)
	}
}

func TestClaimReleasedBranchRace(t *testing.T) {
	dir, remote := setup(t, S{Num: 1})
	cloudRemote(t, remote)
	claimAndMark(t, dir)

	repos := []string{gittest.Clone(t, remote), gittest.Clone(t, remote)}
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, d := range repos {
		wg.Add(1)
		go func(i int, d string) {
			defer wg.Done()
			repo, err := spec.LoadDir(d)
			if err != nil {
				errs[i] = err
				return
			}
			_, errs[i] = claim.Take(gittest.Git(d), repo, "FUR-0001.1", claim.Options{})
		}(i, d)
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
		t.Errorf("wins=%d lost=%d, want exactly one of each", wins, lost)
	}
}

func TestClaimsShowsReleased(t *testing.T) {
	dir, remote := setup(t, S{Num: 1, Steps: []St{{}, {}}})
	cloudRemote(t, remote)
	g := gittest.Git(dir)
	claimAndMark(t, dir)
	if _, err := claim.Take(g, load(t, dir), "FUR-0001.2", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	// Long after both commits: the live claim is stale, the released one is still released.
	later := time.Now().Add(3 * time.Hour)
	statuses, err := claim.StatusesAll(g, load(t, dir), later, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range statuses {
		got = append(got, s.Ref+"="+s.State)
	}
	if strings.Join(got, " ") != "FUR-0001.1=released FUR-0001.2=stale" {
		t.Errorf("states = %v", got)
	}
	var b bytes.Buffer
	claim.StatusText(&b, statuses)
	if line := strings.Split(b.String(), "\n")[0]; line != "FUR-0001.1  step/0001-1  released  3h  (released: delete the branch when you can)" {
		t.Errorf("line = %q", line)
	}
	js, _ := json.Marshal(statuses[0])
	if !strings.Contains(string(js), `"state":"released"`) {
		t.Errorf("json = %s", js)
	}
	// Released branches aren't claims, so furrow status's Statuses leaves them out.
	held, err := claim.Statuses(g, load(t, dir), later, time.Hour)
	if err != nil || len(held) != 1 || held[0].Ref != "FUR-0001.2" {
		t.Errorf("Statuses = %+v %v", held, err)
	}
}

func TestReleaseJSON(t *testing.T) {
	dir, remote := setup(t, S{Num: 1, Steps: []St{{}, {}}})
	cloudRemote(t, remote)
	g := gittest.Git(dir)
	if _, err := claim.Take(g, load(t, dir), "FUR-0001.1", claim.Options{NoCheckout: true}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		ref  string
		want string
	}{
		{"FUR-0001.1", `{"ref":"FUR-0001.1","branch":"step/0001-1","released":true,"how":"marked"}`},
		{"FUR-0001.1", `{"ref":"FUR-0001.1","branch":"step/0001-1","released":false,"how":""}`},
		{"FUR-0001.2", `{"ref":"FUR-0001.2","branch":"step/0001-2","released":false,"how":""}`},
	} {
		res, err := claim.Release(g, load(t, dir), c.ref)
		if err != nil {
			t.Fatal(err)
		}
		js, _ := json.Marshal(res)
		if string(js) != c.want {
			t.Errorf("json = %s, want %s", js, c.want)
		}
	}
}
