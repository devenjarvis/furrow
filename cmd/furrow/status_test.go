package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/devenjarvis/furrow/internal/gitx"
	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

// statusNow is the clock every status test runs on.
var statusNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func statusEnv(dir string) (*Env, *bytes.Buffer, *bytes.Buffer) {
	env, out, errb := testEnv(dir)
	env.Now = func() time.Time { return statusNow }
	return env, out, errb
}

// gitAt runs git with author and committer dates pinned, so a claim's age is fixed.
func gitAt(t *testing.T, dir string, at time.Time, args ...string) {
	t.Helper()
	date := at.Format(time.RFC3339)
	g := gitx.Git{Dir: dir, Env: append(append([]string{}, gittest.Env...), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)}
	if _, err := g.Run(args...); err != nil {
		t.Fatal(err)
	}
}

// pushClaim pushes a claim branch for step 1 of spec num, made at `at` on top of from. With
// handBack, the branch's tip has the spec rewritten with the step merged: true, as furrow done
// writes it. stackOn adds a furrow-stack trailer.
func pushClaim(t *testing.T, r *spectest.Repo, s spectest.Spec, from string, at time.Time, handBack bool, stackOn string) {
	t.Helper()
	branch := fmt.Sprintf("step/%04d-1", s.Num)
	gittest.Must(t, r.Root, "checkout", "-q", "-b", branch, from)
	msg := fmt.Sprintf("Claim FUR-%04d.1: Step 1\n\nfurrow-claim: n%d\n", s.Num, s.Num)
	if stackOn != "" {
		msg += "furrow-stack: " + stackOn + "\n"
	}
	gitAt(t, r.Root, at, "commit", "-q", "--allow-empty", "-m", msg)
	if handBack {
		done := s
		var after []string
		if len(s.Steps) > 0 {
			after = s.Steps[0].After
		}
		done.Steps = []spectest.Step{{After: after, Merged: true}}
		r.Spec(done)
		gittest.Must(t, r.Root, "add", "-A")
		gitAt(t, r.Root, at, "commit", "-q", "-m", "done")
	}
	gittest.Must(t, r.Root, "push", "-q", "origin", branch)
	gittest.Must(t, r.Root, "checkout", "-q", "main")
}

// statusRepo has one of everything furrow status reports.
func statusRepo(t *testing.T) *spectest.Repo {
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	in := spectest.Spec{Num: 1}
	back := spectest.Spec{Num: 2}
	stacked := spectest.Spec{Num: 3, Steps: []spectest.Step{{After: []string{"FUR-0002.1"}}}}
	stale := spectest.Spec{Num: 4}
	for _, s := range []spectest.Spec{in, back, stacked, stale} {
		r.Spec(s)
	}
	r.Spec(spectest.Spec{Num: 5, Steps: []spectest.Step{{Title: "The thing"}}})
	r.Spec(spectest.Spec{Num: 6, Steps: []spectest.Step{{Title: "The other thing"}}})
	r.Spec(spectest.Spec{Num: 7, Steps: []spectest.Step{{After: []string{"FUR-0001.1"}}}})
	r.Spec(spectest.Spec{Num: 8, Steps: []spectest.Step{{Merged: true}}, Criteria: []string{"- [ ] AC-1: Looks right (human: look at it)"}})
	r.Spec(spectest.Spec{Num: 9, Steps: []spectest.Step{{Merged: true}}, Criteria: []string{"- [x] AC-1: It works (test: TestWorks)"}})
	r.Spec(spectest.Spec{Num: 10, Steps: []spectest.Step{{Merged: true}}})
	gittest.Init(t, r.Root)
	two := statusNow.Add(-2 * time.Hour)
	pushClaim(t, r, in, "main", two, false, "")
	pushClaim(t, r, back, "main", two, true, "")
	pushClaim(t, r, stacked, "step/0002-1", two, true, "step/0002-1")
	pushClaim(t, r, stale, "main", statusNow.Add(-72*time.Hour), false, "")
	pushClaim(t, r, spectest.Spec{Num: 10}, "main", two, false, "") // its step is merged: leftover
	return r
}

func TestStatusText(t *testing.T) {
	r := statusRepo(t)
	env, out, errb := statusEnv(r.Root)
	if code := run(env, []string{"status"}); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	want := "In progress (2):\n" +
		"  FUR-0001.1  step/0001-1  active  2h\n" +
		"  FUR-0004.1  step/0004-1  stale  3d\n" +
		"Handed back, waiting for review (2):\n" +
		"  FUR-0002.1  step/0002-1  on main\n" +
		"  FUR-0003.1  step/0003-1  on step/0002-1\n" +
		"Ready now (2):\n" +
		"  FUR-0005.1  The thing\n" +
		"  FUR-0006.1  The other thing\n" +
		"Waiting on the owner:\n" +
		"  human checks: 1 (furrow review)\n" +
		"  ready to archive: FUR-0009\n" +
		"Stale or leftover claims: 2 (furrow claims)\n" +
		"Next: Run furrow claims, then furrow release what's left over.\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	// Every section empty prints (none).
	e := spectest.New(t)
	gittest.Init(t, e.Root)
	env, out, _ = statusEnv(e.Root)
	if code := run(env, []string{"status"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want = "In progress:\n  (none)\n" +
		"Handed back, waiting for review:\n  (none)\n" +
		"Ready now:\n  (none)\n" +
		"Waiting on the owner:\n  (none)\n" +
		"Stale or leftover claims: (none)\n" +
		"Next: Nothing to do. Write a spec with furrow-write.\n"
	if out.String() != want {
		t.Errorf("empty repo got:\n%s\nwant:\n%s", out.String(), want)
	}
}

// readyRefs returns the refs in a JSON document's "ready" list.
func readyRefs(t *testing.T, data []byte) []string {
	t.Helper()
	var doc struct {
		Ready []struct {
			Ref string `json:"ref"`
		} `json:"ready"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%v: %s", err, data)
	}
	var out []string
	for _, r := range doc.Ready {
		out = append(out, r.Ref)
	}
	return out
}

func TestStatusReadyCap(t *testing.T) {
	r := spectest.New(t)
	for n := 1; n <= 7; n++ {
		r.Spec(spectest.Spec{Num: n})
	}
	// FUR-0007.1 unblocks a step, so furrow next ranks it first.
	r.Spec(spectest.Spec{Num: 8, Steps: []spectest.Step{{After: []string{"FUR-0007.1"}}}})
	gittest.Init(t, r.Root)

	env, out, _ := statusEnv(r.Root)
	if code := run(env, []string{"status"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "Ready now (7):\n" +
		"  FUR-0007.1  Step 1\n" +
		"  FUR-0001.1  Step 1\n" +
		"  FUR-0002.1  Step 1\n" +
		"  FUR-0003.1  Step 1\n" +
		"  FUR-0004.1  Step 1\n" +
		"  +2 more: furrow next\n" +
		"Waiting on the owner:\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("got:\n%s\nwant it to contain:\n%s", out.String(), want)
	}

	env, out, _ = statusEnv(r.Root)
	run(env, []string{"status", "--json"})
	var doc struct {
		ReadyTotal int `json:"ready_total"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	got := readyRefs(t, out.Bytes())
	env, nout, _ := statusEnv(r.Root)
	run(env, []string{"next", "--json"})
	if next := readyRefs(t, nout.Bytes()); !reflect.DeepEqual(got, next) {
		t.Errorf("status ready %v, next ready %v", got, next)
	}
	if len(got) != 7 || doc.ReadyTotal != 7 {
		t.Errorf("ready %v, ready_total %d; want 7 of each", got, doc.ReadyTotal)
	}
}

func TestStatusJSON(t *testing.T) {
	r := statusRepo(t)
	env, out, _ := statusEnv(r.Root)
	if code := run(env, []string{"status", "--json"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	first := out.String()
	env, out, _ = statusEnv(r.Root)
	run(env, []string{"status", "--json"})
	if out.String() != first {
		t.Errorf("two runs differ:\n%s\n---\n%s", first, out.String())
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(first), &keys); err != nil {
		t.Fatal(err)
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	wantKeys := []string{"claims_checked", "handed_back", "human_checks", "in_progress", "leftover_claims", "next", "ready", "ready_to_archive", "ready_total"}
	if !reflect.DeepEqual(names, wantKeys) {
		t.Errorf("keys %v, want %v", names, wantKeys)
	}

	want := `{
  "claims_checked": true,
  "in_progress": [
    {
      "ref": "FUR-0001.1",
      "branch": "step/0001-1",
      "state": "active",
      "age": "2h"
    },
    {
      "ref": "FUR-0004.1",
      "branch": "step/0004-1",
      "state": "stale",
      "age": "3d"
    }
  ],
  "handed_back": [
    {
      "ref": "FUR-0002.1",
      "branch": "step/0002-1",
      "stack_on": null
    },
    {
      "ref": "FUR-0003.1",
      "branch": "step/0003-1",
      "stack_on": "step/0002-1"
    }
  ],
  "ready": [
    {
      "ref": "FUR-0005.1",
      "title": "The thing",
      "stack_on": null
    },
    {
      "ref": "FUR-0006.1",
      "title": "The other thing",
      "stack_on": null
    }
  ],
  "ready_total": 2,
  "human_checks": 1,
  "ready_to_archive": [
    "FUR-0009"
  ],
  "leftover_claims": 2,
  "next": "Run furrow claims, then furrow release what's left over."
}
`
	if first != want {
		t.Errorf("got:\n%s\nwant:\n%s", first, want)
	}

	// The text says the same: its Next line is the JSON's next.
	env, text, _ := statusEnv(r.Root)
	run(env, []string{"status"})
	if !strings.HasSuffix(text.String(), "Next: Run furrow claims, then furrow release what's left over.\n") {
		t.Errorf("text:\n%s", text.String())
	}
}

func TestStatusOffline(t *testing.T) {
	r := spectest.New(t)
	r.Spec(spectest.Spec{Num: 1})
	r.Spec(spectest.Spec{Num: 2})
	gittest.Init(t, r.Root)
	pushClaim(t, r, spectest.Spec{Num: 1}, "main", statusNow.Add(-time.Hour), false, "")

	check := func(name string, statusArgs, nextArgs []string) {
		t.Helper()
		env, out, errb := statusEnv(r.Root)
		if code := run(env, statusArgs); code != 0 {
			t.Fatalf("%s: exit %d: %s", name, code, errb.String())
		}
		want := "In progress:\n  (remote not checked)\n" +
			"Handed back, waiting for review:\n  (remote not checked)\n" +
			"Ready now (2):\n  FUR-0001.1  Step 1\n  FUR-0002.1  Step 1\n" +
			"Waiting on the owner:\n  (none)\n" +
			"Stale or leftover claims: (remote not checked)\n" +
			"Next: Run furrow next --agents N for the 2 ready steps.\n"
		if out.String() != want {
			t.Errorf("%s got:\n%s\nwant:\n%s", name, out.String(), want)
		}
		env, out, _ = statusEnv(r.Root)
		if code := run(env, append(statusArgs, "--json")); code != 0 {
			t.Fatalf("%s --json: exit %d", name, code)
		}
		if !strings.Contains(out.String(), `"claims_checked": false`) {
			t.Errorf("%s json:\n%s", name, out.String())
		}
		env, nout, _ := statusEnv(r.Root)
		run(env, append(nextArgs, "--json"))
		if got, next := readyRefs(t, out.Bytes()), readyRefs(t, nout.Bytes()); !reflect.DeepEqual(got, next) {
			t.Errorf("%s: status ready %v, next ready %v", name, got, next)
		}
	}
	check("--offline", []string{"status", "--offline"}, []string{"next", "--offline"})

	gittest.Must(t, r.Root, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	check("unreachable", []string{"status"}, []string{"next"})

	// No .furrow.yml: the same error and exit code as furrow next.
	bare := t.TempDir()
	env, _, serr := statusEnv(bare)
	scode := run(env, []string{"status"})
	env, _, nerr := statusEnv(bare)
	ncode := run(env, []string{"next"})
	if scode != ncode || serr.String() != nerr.String() || scode == 0 {
		t.Errorf("status exit %d %q, next exit %d %q", scode, serr.String(), ncode, nerr.String())
	}
}
