// Package scaffold starts Furrow in a repo and starts new specs and areas: furrow init, new, area
// and invariants. It only writes files, and never overwrites one.
package scaffold

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/devenjarvis/furrow/internal/config"
	"github.com/devenjarvis/furrow/internal/spec"
	"gopkg.in/yaml.v3"
)

//go:embed assets/*
var assets embed.FS

func asset(name string) string {
	data, err := assets.ReadFile("assets/" + name)
	if err != nil {
		panic("scaffold: missing asset " + name)
	}
	return string(data)
}

var (
	slugRe   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	prefixRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*$`)
	numRe    = regexp.MustCompile(`^(\d{4})-.*\.md$`)
)

// DefaultPrefix is the prefix init picks for a repo folder: its name upper-cased, letters only,
// at most 6, else SPEC.
func DefaultPrefix(dirName string) string {
	var b strings.Builder
	for _, c := range strings.ToUpper(dirName) {
		if c >= 'A' && c <= 'Z' && b.Len() < 6 {
			b.WriteRune(c)
		}
	}
	if b.Len() == 0 {
		return "SPEC"
	}
	return b.String()
}

// Entry is one path init looked at.
type Entry struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// InitResult is what init did, in a fixed order, and anything the owner should do next.
type InitResult struct {
	Entries []Entry  `json:"entries"`
	Notes   []string `json:"notes"`
}

// Text prints "created <path>" or "kept <path>" for each entry, then the notes.
func (r InitResult) Text(w io.Writer) {
	for _, e := range r.Entries {
		verb := "kept"
		if e.Created {
			verb = "created"
		}
		fmt.Fprintf(w, "%s %s\n", verb, e.Path)
	}
	for _, n := range r.Notes {
		fmt.Fprintln(w, n)
	}
}

// Init writes Furrow's files into root without overwriting any that exist. An empty prefix means
// DefaultPrefix of root's folder name. When .furrow.yml already exists, its settings win.
func Init(root, prefix string) (InitResult, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return InitResult{}, err
	}
	name := filepath.Base(abs)
	if prefix == "" {
		prefix = DefaultPrefix(name)
	}
	if !prefixRe.MatchString(prefix) {
		return InitResult{}, fmt.Errorf("Prefix %q must be uppercase letters and digits, starting with a letter, such as APP.", prefix)
	}
	cfg := config.Default()
	cfg.Prefix = prefix
	cfgPath := filepath.Join(abs, config.FileName)
	if _, err := os.Stat(cfgPath); err == nil {
		if cfg, err = config.Load(cfgPath); err != nil {
			return InitResult{}, err
		}
	}

	var res InitResult
	write := func(rel, content string) error {
		created, err := writeNew(filepath.Join(abs, filepath.FromSlash(rel)), content)
		if err != nil {
			return err
		}
		res.Entries = append(res.Entries, Entry{Path: rel, Created: created})
		return nil
	}
	fill := func(text string) string {
		return strings.NewReplacer("{{PREFIX}}", cfg.Prefix, "{{NAME}}", name).Replace(text)
	}
	specs := filepath.ToSlash(cfg.Specs)
	files := []struct{ rel, content string }{
		{config.FileName, configText(cfg)},
		{specs + "/README.md", fill(asset("README.md"))},
		{specs + "/_template.md", fill(asset("_template.md"))},
		{specs + "/changes/.gitkeep", ""},
		{specs + "/areas/.gitkeep", ""},
		{specs + "/archive/.gitkeep", ""},
		{filepath.ToSlash(cfg.Product), fill(asset("product.md"))},
	}
	for _, f := range files {
		if err := write(f.rel, f.content); err != nil {
			return res, err
		}
	}
	product := filepath.ToSlash(cfg.Product)
	if data, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(product))); err == nil && !hasSection(string(data), "Invariants") {
		res.Notes = append(res.Notes, fmt.Sprintf("%s has no \"## Invariants\" section. Add one, with lines like **INV-1: <rule>.**, so specs can name the rules they touch.", product))
	}
	return res, nil
}

func hasSection(text, name string) bool {
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(l, "## ") && strings.TrimSpace(l[3:]) == name {
			return true
		}
	}
	return false
}

func configText(cfg config.Config) string {
	return fmt.Sprintf(`# Furrow's settings for this repo. See specs/README.md.
prefix: %s
specs: %s
product: %s
check: %s   # the command that proves a step is done, such as "make check"
remote: %s
base: %s
branch: %s
aliases: {}
`, cfg.Prefix, yamlString(cfg.Specs), yamlString(cfg.Product), yamlString(cfg.Check),
		yamlString(cfg.Remote), yamlString(cfg.Base), yamlString(cfg.Branch))
}

// yamlString renders s as a YAML scalar, quoting it only when it must.
func yamlString(s string) string {
	data, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSuffix(string(data), "\n")
}

// writeNew writes a file only if nothing is at path. It reports whether it wrote one.
func writeNew(path, content string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}

// Created is the spec furrow new wrote.
type Created struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Area  string `json:"area"`
	File  string `json:"file"`
}

// New writes specs/changes/NNNN-<slug>.md as a draft, numbered one past the highest number in
// changes and archive, from the repo's specs/_template.md or the built-in template.
func New(repo *spec.Repo, slug, title, area string) (Created, error) {
	if !slugRe.MatchString(slug) {
		return Created{}, fmt.Errorf("Slug %q must be lowercase letters, digits and dashes, such as add-csv-export.", slug)
	}
	if strings.TrimSpace(title) == "" {
		return Created{}, fmt.Errorf("Give the spec a title with --title.")
	}
	if _, ok := repo.Areas[area]; !ok {
		return Created{}, fmt.Errorf("No area '%s'. Create it with `furrow area %s --title …`.", area, area)
	}
	specsDir := filepath.Join(repo.Root, repo.Config.Specs)
	highest := 0
	for _, dir := range []string{"changes", "archive"} {
		entries, err := os.ReadDir(filepath.Join(specsDir, dir))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Created{}, err
		}
		for _, e := range entries {
			if m := numRe.FindStringSubmatch(e.Name()); m != nil {
				n, _ := strconv.Atoi(m[1])
				highest = max(highest, n)
			}
		}
	}
	num := highest + 1
	if num > 9999 {
		return Created{}, fmt.Errorf("Spec numbers run out at 9999.")
	}
	tmpl, err := os.ReadFile(filepath.Join(specsDir, "_template.md"))
	if errors.Is(err, fs.ErrNotExist) {
		tmpl = []byte(strings.ReplaceAll(asset("_template.md"), "{{PREFIX}}", repo.Config.Prefix))
	} else if err != nil {
		return Created{}, err
	}
	c := Created{ID: fmt.Sprintf("%s-%04d", repo.Config.Prefix, num), Title: title, Area: area}
	text := fillTemplate(string(tmpl), map[string]string{
		"id": c.ID, "title": yamlString(title), "status": spec.Draft, "area": yamlString(area),
	}, fmt.Sprintf("# %s: %s", c.ID, title))
	c.File = filepath.ToSlash(filepath.Join(repo.Config.Specs, "changes", fmt.Sprintf("%04d-%s.md", num, slug)))
	created, err := writeNew(filepath.Join(repo.Root, filepath.FromSlash(c.File)), text)
	if err != nil {
		return Created{}, err
	}
	if !created {
		return Created{}, fmt.Errorf("%s already exists.", c.File)
	}
	return c, nil
}

// fillTemplate sets the top-level front matter keys in values (adding any the template lacks) and
// replaces the body's first "# " heading.
func fillTemplate(tmpl string, values map[string]string, heading string) string {
	lines := strings.Split(strings.ReplaceAll(tmpl, "\r\n", "\n"), "\n")
	set := map[string]bool{}
	end := -1
	if len(lines) > 0 && lines[0] == "---" {
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				end = i
				break
			}
			for key, v := range values {
				if strings.HasPrefix(lines[i], key+":") {
					lines[i] = key + ": " + v
					set[key] = true
				}
			}
		}
	}
	headed := false
	for i := end + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "# ") {
			lines[i] = heading
			headed = true
			break
		}
	}
	var missing []string
	for _, key := range []string{"id", "title", "status", "area"} {
		if !set[key] {
			missing = append(missing, key+": "+values[key])
		}
	}
	body := lines
	var front []string
	if end >= 0 {
		front, body = lines[1:end], lines[end+1:]
	}
	if !headed {
		body = append([]string{"", heading}, body...)
	}
	out := append([]string{"---"}, missing...)
	out = append(out, front...)
	out = append(out, "---")
	out = append(out, body...)
	return strings.Join(out, "\n")
}

// Area writes specs/areas/<name>.md. It refuses a name that isn't a slug and an area that exists.
func Area(repo *spec.Repo, name, title string) (string, error) {
	if !slugRe.MatchString(name) {
		return "", fmt.Errorf("Area name %q must be lowercase letters, digits and dashes, such as billing.", name)
	}
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("Give the area a title with --title.")
	}
	rel := filepath.ToSlash(filepath.Join(repo.Config.Specs, "areas", name+".md"))
	text := strings.NewReplacer("{{AREA}}", name, "{{TITLE}}", yamlString(title), "{{HEADING}}", title).Replace(asset("area.md"))
	created, err := writeNew(filepath.Join(repo.Root, filepath.FromSlash(rel)), text)
	if err != nil {
		return "", err
	}
	if !created {
		return "", fmt.Errorf("Area '%s' already exists (%s). Edit it, or pick another name.", name, rel)
	}
	return rel, nil
}

// Invariants reads the invariants in a product file.
func Invariants(productPath string) ([]spec.Invariant, error) {
	data, err := os.ReadFile(productPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("No product file at %s. Run `furrow init` to start one.", productPath)
		}
		return nil, err
	}
	invs := spec.ParseInvariants(string(data))
	if invs == nil {
		invs = []spec.Invariant{}
	}
	return invs, nil
}

// WriteInvariants prints one "INV-n  <name>" line per invariant.
func WriteInvariants(w io.Writer, invs []spec.Invariant) {
	for _, inv := range invs {
		fmt.Fprintf(w, "%s  %s\n", inv.ID, inv.Text)
	}
}
