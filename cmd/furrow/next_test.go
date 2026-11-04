package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devenjarvis/furrow/internal/gitx/gittest"
	"github.com/devenjarvis/furrow/internal/spec/spectest"
)

func testEnv(dir string) (*Env, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	getenv := func(k string) string {
		switch k {
		case "GIT_CONFIG_GLOBAL":
			return "/dev/null"
		case "GIT_CONFIG_NOSYSTEM":
			return "1"
		}
		return ""
	}
	return &Env{Dir: dir, Stdout: &out, Stderr: &errb, Getenv: getenv, Now: func() time.Time { return time.Unix(0, 0) }}, &out, &errb
}

func TestNextLeavesOutClaims(t *testing.T) {
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Touches: []string{"internal/a/**"}}}})
	r.Spec(spectest.Spec{Num: 2, Steps: []spectest.Step{{Touches: []string{"internal/a/x.go"}}}})
	r.Spec(spectest.Spec{Num: 3, Steps: []spectest.Step{{Touches: []string{"internal/b/**"}}}})
	gittest.Init(t, r.Root)
	gittest.Must(t, r.Root, "push", "-q", "origin", "main:refs/heads/step/0001-1")

	env, out, _ := testEnv(r.Root)
	if code := run(env, []string{"next"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "Ready to start:\n" +
		"  FUR-0002.1  p1  s  Step 1\n" +
		"              specs/changes/0002-spec-2.md · touches internal/a/x.go\n" +
		"  FUR-0003.1  p1  s  Step 1\n" +
		"              specs/changes/0003-spec-3.md · touches internal/b/**\n" +
		"Can run together: FUR-0003.1\n" +
		"  not FUR-0002.1: shares internal/a/x.go and internal/a/** with FUR-0001.1\n" +
		"Claimed:\n" +
		"  FUR-0001.1  step/0001-1\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	env, out, _ = testEnv(r.Root)
	run(env, []string{"next", "--json"})
	var plan struct {
		ClaimsChecked bool `json:"claims_checked"`
		Claimed       []struct {
			Ref, Branch string
		} `json:"claimed"`
	}
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.ClaimsChecked || len(plan.Claimed) != 1 || plan.Claimed[0].Branch != "step/0001-1" {
		t.Errorf("json = %s", out.String())
	}

	env, out, _ = testEnv(r.Root)
	run(env, []string{"graph"})
	if !strings.Contains(out.String(), "FUR-0001.1  [claimed]") {
		t.Errorf("graph:\n%s", out.String())
	}
}

func TestNextRemoteDown(t *testing.T) {
	r := spectest.New(t)
	r.Spec(spectest.Spec{Num: 1})
	gittest.Init(t, r.Root)
	gittest.Must(t, r.Root, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	env, out, _ := testEnv(r.Root)
	if code := run(env, []string{"next"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out.String(), "(Couldn't reach origin, so claimed steps aren't left out.)\nReady to start:\n  FUR-0001.1") {
		t.Errorf("got:\n%s", out.String())
	}
	env, out, _ = testEnv(r.Root)
	run(env, []string{"next", "--json"})
	if !strings.Contains(out.String(), `"claims_checked": false`) {
		t.Errorf("json:\n%s", out.String())
	}
	env, out, _ = testEnv(r.Root)
	run(env, []string{"next", "--offline"})
	if !strings.HasPrefix(out.String(), "(Claims not checked: offline.)\n") {
		t.Errorf("offline:\n%s", out.String())
	}
}

func TestNextAgentsShowsHeld(t *testing.T) {
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Touches: []string{"internal/a/**"}}}})
	r.Spec(spectest.Spec{Num: 2, Steps: []spectest.Step{{Touches: []string{"internal/a/x.go"}}}})
	r.Spec(spectest.Spec{Num: 3, Steps: []spectest.Step{{Touches: []string{"internal/b/**"}}}})
	r.Spec(spectest.Spec{Num: 4, Steps: []spectest.Step{{Touches: []string{"internal/b/y.go"}}}})
	gittest.Init(t, r.Root)
	gittest.Must(t, r.Root, "push", "-q", "origin", "main:refs/heads/step/0001-1")

	env, out, _ := testEnv(r.Root)
	if code := run(env, []string{"next", "--agents", "2"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "Assignments:\n" +
		"  FUR-0003.1  p1  s  Step 1\n" +
		"              specs/changes/0003-spec-3.md · touches internal/b/**\n" +
		"  not FUR-0002.1: shares internal/a/x.go and internal/a/** with FUR-0001.1\n" +
		"  not FUR-0004.1: shares internal/b/y.go and internal/b/** with FUR-0003.1\n" +
		"Claimed:\n" +
		"  FUR-0001.1  step/0001-1\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestNextAgentsNoneAssignable(t *testing.T) {
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	r.Spec(spectest.Spec{Num: 1, Steps: []spectest.Step{{Touches: []string{"internal/a/**"}}}})
	r.Spec(spectest.Spec{Num: 2, Steps: []spectest.Step{{Touches: []string{"internal/a/x.go"}}}})
	r.Spec(spectest.Spec{Num: 3, Steps: []spectest.Step{{Touches: []string{"internal/a/y.go"}}}})
	gittest.Init(t, r.Root)
	gittest.Must(t, r.Root, "push", "-q", "origin", "main:refs/heads/step/0001-1")

	env, out, _ := testEnv(r.Root)
	if code := run(env, []string{"next", "--agents", "4"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "Assignments: none. Every ready step shares files with a claimed step.\n" +
		"  not FUR-0002.1: shares internal/a/x.go and internal/a/** with FUR-0001.1\n" +
		"  not FUR-0003.1: shares internal/a/y.go and internal/a/** with FUR-0001.1\n" +
		"Claimed:\n" +
		"  FUR-0001.1  step/0001-1\n"
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}

	// --json doesn't change: the holds are already there.
	env, out, _ = testEnv(r.Root)
	run(env, []string{"next", "--agents", "4", "--json"})
	var plan struct {
		Together []json.RawMessage `json:"together"`
		Held     []struct {
			Step      string `json:"step"`
			ClashWith string `json:"clash_with"`
		} `json:"held"`
	}
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Together) != 0 || len(plan.Held) != 2 || plan.Held[0].ClashWith != "FUR-0001.1" {
		t.Errorf("json = %s", out.String())
	}
}

func TestNextStack(t *testing.T) {
	r := spectest.New(t)
	r.Write("internal/.keep", "")
	one := spectest.Spec{Num: 1, Steps: []spectest.Step{{Touches: []string{"internal/a/**"}}}}
	r.Spec(one)
	r.Spec(spectest.Spec{Num: 2, Steps: []spectest.Step{{After: []string{"FUR-0001.1"}, Touches: []string{"internal/a/x.go"}}}})
	r.Spec(spectest.Spec{Num: 3, Steps: []spectest.Step{{Touches: []string{"internal/c/**"}}}})
	r.Spec(spectest.Spec{Num: 4, Steps: []spectest.Step{{After: []string{"FUR-0002.1"}}}})
	gittest.Init(t, r.Root)
	// FUR-0001.1 is claimed, and furrow done has run on its branch: it's handed back.
	gittest.Must(t, r.Root, "switch", "-q", "-c", "step/0001-1")
	gittest.Must(t, r.Root, "commit", "-q", "--allow-empty", "-m", "Claim FUR-0001.1: Step 1\n\nfurrow-claim: 0123456789abcdef")
	done := one
	done.Steps = []spectest.Step{{Touches: []string{"internal/a/**"}, Merged: true}}
	r.Spec(done)
	gittest.Commit(t, r.Root, "FUR-0001.1: done")
	gittest.Must(t, r.Root, "push", "-q", "origin", "step/0001-1")
	gittest.Must(t, r.Root, "switch", "-q", "main")

	// stack: 0, the default: exactly what furrow next printed before stacking.
	before := "Ready to start:\n" +
		"  FUR-0003.1  p1  s  Step 1\n" +
		"              specs/changes/0003-spec-3.md · touches internal/c/**\n" +
		"Can run together: FUR-0003.1\n" +
		"Claimed:\n" +
		"  FUR-0001.1  step/0001-1\n" +
		"Waiting:\n" +
		"  FUR-0002.1  after FUR-0001.1\n" +
		"  FUR-0004.1  after FUR-0002.1\n"
	env, out, _ := testEnv(r.Root)
	if code := run(env, []string{"next"}); code != 0 || out.String() != before {
		t.Errorf("stack: 0 (exit %d):\n%s\nwant:\n%s", code, out.String(), before)
	}
	type stepJSON struct {
		Ref     string `json:"ref"`
		StackOn *struct {
			Ref    string `json:"ref"`
			Branch string `json:"branch"`
		} `json:"stack_on"`
		Depth *int `json:"depth"`
	}
	type planJSON struct {
		Ready   []stepJSON `json:"ready"`
		Waiting []stepJSON `json:"waiting"`
	}
	nextJSON := func(args ...string) planJSON {
		t.Helper()
		env, out, _ := testEnv(r.Root)
		if code := run(env, append([]string{"next", "--json"}, args...)); code != 0 {
			t.Fatalf("exit %d", code)
		}
		var p planJSON
		if err := json.Unmarshal(out.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := nextJSON()
	if len(p.Ready) != 1 || p.Ready[0].StackOn != nil || p.Ready[0].Depth == nil || *p.Ready[0].Depth != 0 {
		t.Errorf("stack: 0 json = %+v", p)
	}

	r.Write(".furrow.yml", spectest.Config+"stack: 2\n")
	env, out, _ = testEnv(r.Root)
	if code := run(env, []string{"next"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "Ready to start:\n" +
		"  FUR-0003.1  p1  s  Step 1\n" +
		"              specs/changes/0003-spec-3.md · touches internal/c/**\n" +
		"  FUR-0002.1  p1  s  Step 1\n" +
		"              specs/changes/0002-spec-2.md · touches internal/a/x.go\n" +
		"  FUR-0002.1  stacked on step/0001-1 (FUR-0001.1)\n" +
		"Can run together: FUR-0003.1, FUR-0002.1\n" +
		"Claimed:\n" +
		"  FUR-0001.1  step/0001-1\n" +
		"Waiting:\n" +
		"  FUR-0004.1  after FUR-0002.1 (FUR-0002.1 isn't handed back)\n"
	if out.String() != want {
		t.Errorf("stack: 2:\n%s\nwant:\n%s", out.String(), want)
	}
	p = nextJSON()
	if len(p.Ready) != 2 || p.Ready[0].StackOn != nil || *p.Ready[0].Depth != 0 {
		t.Fatalf("stack: 2 json = %+v", p)
	}
	if s := p.Ready[1]; s.Ref != "FUR-0002.1" || s.StackOn == nil || s.StackOn.Ref != "FUR-0001.1" || s.StackOn.Branch != "step/0001-1" || *s.Depth != 1 {
		t.Errorf("stacked step json = %+v", s)
	}

	// Offline, the tips can't be read: nothing stacks.
	env, out, _ = testEnv(r.Root)
	run(env, []string{"next", "--offline"})
	if !strings.HasPrefix(out.String(), "(Claims not checked: offline.)\n") || strings.Contains(out.String(), "stacked") {
		t.Errorf("offline:\n%s", out.String())
	}
	if p := nextJSON("--offline"); len(p.Ready) != 2 || p.Ready[0].StackOn != nil || p.Ready[1].StackOn != nil {
		t.Errorf("offline json = %+v", p)
	}

	env, out, _ = testEnv(r.Root)
	run(env, []string{"graph"})
	if !strings.Contains(out.String(), "FUR-0002.1  [ready (stacked)]") {
		t.Errorf("graph:\n%s", out.String())
	}
}
