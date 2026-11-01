// Package config reads .furrow.yml, the one file that tells Furrow how a repo is laid out.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if !strings.Contains(cfg.Branch, "{num}") || !strings.Contains(cfg.Branch, "{step}") {
		return Config{}, fmt.Errorf("%s: branch %q must contain {num} and {step}", FileName, cfg.Branch)
	}
	return cfg, nil
}
