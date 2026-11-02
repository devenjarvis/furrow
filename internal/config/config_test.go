package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("prefix: FUR\naliases:\n  cli: cmd/furrow\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, cfg, err := Find(deep)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("root = %q, want %q", got, root)
	}
	if cfg.Prefix != "FUR" || cfg.Specs != "specs" || cfg.Remote != "origin" || cfg.Base != "main" ||
		cfg.Branch != "step/{num}-{step}" || cfg.Product != "docs/product.md" || cfg.Aliases["cli"] != "cmd/furrow" {
		t.Errorf("defaults not applied: %+v", cfg)
	}

	_, _, err = Find(t.TempDir())
	if !errors.Is(err, ErrNoConfig) {
		t.Fatalf("err = %v, want ErrNoConfig", err)
	}
	if want := "No .furrow.yml here or above. Run `furrow init` to start."; err.Error() != want {
		t.Errorf("message = %q", err.Error())
	}

	if _, err := Parse([]byte("branch: work/{num}\n")); err == nil {
		t.Error("a branch pattern without {step} should be refused")
	}
}

func TestParseVisual(t *testing.T) {
	cfg, err := Parse([]byte("prefix: FUR\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Visual == nil || len(cfg.Visual) != 0 {
		t.Errorf("visual defaults to %#v, want an empty list", cfg.Visual)
	}
	cfg, err = Parse([]byte("visual: [web/src/**, ui, ' ', ' docs/*.png ']\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"web/src/**", "ui", "docs/*.png"}
	if len(cfg.Visual) != len(want) {
		t.Fatalf("visual = %#v, want %#v", cfg.Visual, want)
	}
	for i := range want {
		if cfg.Visual[i] != want[i] {
			t.Errorf("visual = %#v, want %#v", cfg.Visual, want)
		}
	}
}
