package lifecycle

import "testing"

func TestAppendToSection(t *testing.T) {
	cases := []struct{ in, want string }{
		{"# T\n\n## History\n", "# T\n\n## History\n\n- x\n"},
		{"# T\n\n## History\n\n", "# T\n\n## History\n\n- x\n"},
		{"# T\n\n## History\n\n- a\n", "# T\n\n## History\n\n- a\n- x\n"},
		{"# T\n\n## History\n\n- a\n\n## Next\n\nText.\n", "# T\n\n## History\n\n- a\n- x\n\n## Next\n\nText.\n"},
		{"# T\n\n## History\n## Next\n", "# T\n\n## History\n\n- x\n\n## Next\n"},
		{"# T\n\n## History\n\n## Next\n", "# T\n\n## History\n\n- x\n\n## Next\n"},
		{"# T\n\n## History\n\nA paragraph.\n", "# T\n\n## History\n\nA paragraph.\n\n- x\n"},
		{"# T\n\nText.", "# T\n\nText.\n\n## History\n\n- x\n"},
		{"---\na: 1\n---\n\n## History\n\n- a", "---\na: 1\n---\n\n## History\n\n- a\n- x"},
	}
	for _, c := range cases {
		if got := appendToSection(c.in, "History", "- x"); got != c.want {
			t.Errorf("appendToSection(%q):\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestSetMergedAddsMissingLine(t *testing.T) {
	in := "---\nid: X\nsteps:\n  - n: 1\n    title: A\n---\n"
	got, err := setMerged(in, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := "---\nid: X\nsteps:\n  - n: 1\n    merged: true\n    title: A\n---\n"; got != want {
		t.Errorf("got %q", got)
	}
	flow := "---\nid: X\nsteps: [{n: 1, merged: false}]\n---\n"
	if got, err := setMerged(flow, 1); err != nil || got != "---\nid: X\nsteps: [{n: 1, merged: true}]\n---\n" {
		t.Errorf("flow: %q %v", got, err)
	}
	if _, err := setMerged(in, 2); err == nil {
		t.Error("missing step was accepted")
	}
}
