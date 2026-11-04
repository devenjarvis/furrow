// Package config reads .furrow.yml, the one file that tells Furrow how a repo is laid out.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName is the config file Furrow looks for, walking up from the working directory.
const FileName = ".furrow.yml"

// ErrNoConfig means there is no .furrow.yml in the directory or any parent.
var ErrNoConfig = errors.New("No .furrow.yml here or above. Run `furrow init` to start.")

// Config is a repo's .furrow.yml with defaults filled in.
type Config struct {
	Prefix  string            `yaml:"prefix"`
	Specs   string            `yaml:"specs"`
	Product string            `yaml:"product"`
	Check   string            `yaml:"check"`
	Remote  string            `yaml:"remote"`
	Base    string            `yaml:"base"`
	Branch  string            `yaml:"branch"`
	Aliases map[string]string `yaml:"aliases"`
	// Visual lists globs or aliases, resolved like touches. A diff touching one needs visual
	// proof in its brief. Empty by default.
	Visual []string `yaml:"visual"`
	// Stack is how many unmerged, handed-back steps a new step may sit on. 0, the default, is off.
	Stack int `yaml:"-"`
}

// Default returns the configuration used for every key a .furrow.yml leaves out.
func Default() Config {
	return Config{
		Prefix:  "SPEC",
		Specs:   "specs",
		Product: "docs/product.md",
		Remote:  "origin",
		Base:    "main",
		Branch:  "step/{num}-{step}",
		Aliases: map[string]string{},
		Visual:  []string{},
	}
}

// Find walks up from dir to the first .furrow.yml and returns the repo root and its config.
func Find(dir string) (string, Config, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", Config{}, err
	}
	for {
		path := filepath.Join(abs, FileName)
		if _, err := os.Stat(path); err == nil {
			cfg, err := Load(path)
			return abs, cfg, err
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", Config{}, ErrNoConfig
		}
		abs = parent
	}
}

// Load reads one config file and fills in defaults.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(data)
}

// Parse reads config YAML and fills in defaults.
func Parse(data []byte) (Config, error) {
	cfg := Default()
	var raw Config
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("%s: %v", FileName, err)
	}
	set := func(dst *string, v string) {
		if strings.TrimSpace(v) != "" {
			*dst = strings.TrimSpace(v)
		}
	}
	set(&cfg.Prefix, raw.Prefix)
	set(&cfg.Specs, raw.Specs)
	set(&cfg.Product, raw.Product)
	set(&cfg.Check, raw.Check)
	set(&cfg.Remote, raw.Remote)
	set(&cfg.Base, raw.Base)
	set(&cfg.Branch, raw.Branch)
	for k, v := range raw.Aliases {
		cfg.Aliases[k] = v
	}
	for _, v := range raw.Visual {
		if v = strings.TrimSpace(v); v != "" {
			cfg.Visual = append(cfg.Visual, v)
		}
	}
	stack, err := parseStack(data)
	if err != nil {
		return Config{}, err
	}
	cfg.Stack = stack
	if !strings.Contains(cfg.Branch, "{num}") || !strings.Contains(cfg.Branch, "{step}") {
		return Config{}, fmt.Errorf("%s: branch %q must contain {num} and {step}", FileName, cfg.Branch)
	}
	return cfg, nil
}

// parseStack reads the stack key: a whole number from 0 to 9, and 0 when it's left out.
func parseStack(data []byte) (int, error) {
	var raw struct {
		Stack yaml.Node `yaml:"stack"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return 0, fmt.Errorf("%s: %v", FileName, err)
	}
	n := &raw.Stack
	if n.Kind == 0 || (n.Kind == yaml.ScalarNode && n.Tag == "!!null") {
		return 0, nil
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!int" {
		if v, err := strconv.Atoi(n.Value); err == nil && v >= 0 && v <= 9 {
			return v, nil
		}
	}
	got := n.Value
	if n.Kind != yaml.ScalarNode {
		if out, err := yaml.Marshal(n); err == nil {
			got = strings.TrimSpace(string(out))
		}
	}
	return 0, fmt.Errorf("%s: stack must be a whole number from 0 to 9, got %q", FileName, got)
}
