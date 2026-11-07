package scaffold

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SettingsPath is the project's Claude Code settings, where furrow init declares the plugin.
const SettingsPath = ".claude/settings.json"

// PluginID is the Furrow plugin in the Furrow marketplace.
const PluginID = "furrow@furrow"

// pluginSettings is the .claude/settings.json that declares the Furrow plugin. Structs keep the
// keys in the order the spec shows; the standard library writes it (INV-7).
type pluginSettings struct {
	ExtraKnownMarketplaces map[string]marketplace `json:"extraKnownMarketplaces"`
	EnabledPlugins         map[string]bool        `json:"enabledPlugins"`
}

type marketplace struct {
	Source     marketplaceSource `json:"source"`
	AutoUpdate bool              `json:"autoUpdate"`
}

type marketplaceSource struct {
	Source string `json:"source"`
	Repo   string `json:"repo"`
}

// PluginSettingsText is the settings file that adds the Furrow plugin, indented by two spaces,
// with a final newline.
func PluginSettingsText() string {
	s := pluginSettings{
		ExtraKnownMarketplaces: map[string]marketplace{
			"furrow": {Source: marketplaceSource{Source: "github", Repo: "devenjarvis/furrow"}, AutoUpdate: true},
		},
		EnabledPlugins: map[string]bool{PluginID: true},
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		panic("scaffold: plugin settings: " + err.Error())
	}
	return string(data) + "\n"
}

// pluginNote writes .claude/settings.json when there's none and returns what to print. It never
// edits a file that exists: one that enables the plugin gets no note, one that doesn't gets the
// keys to add, and one that isn't JSON is left alone with a note saying so.
func pluginNote(root string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(SettingsPath))
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		created, err := writeNew(path, PluginSettingsText())
		if err != nil || !created {
			return "", err
		}
		return "wrote " + SettingsPath + " (the Furrow plugin)", nil
	case err != nil:
		return "", err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return SettingsPath + " isn't valid JSON; the Furrow plugin wasn't added", nil
	}
	if obj, ok := v.(map[string]any); ok {
		if plugins, ok := obj["enabledPlugins"].(map[string]any); ok && plugins[PluginID] == true {
			return "", nil
		}
	}
	return SettingsPath + " exists: add the Furrow plugin to it:\n" + strings.TrimSuffix(PluginSettingsText(), "\n"), nil
}

// FindCopies lists Furrow's skills and agents copied into the repo, sorted: every directory
// .claude/skills/furrow-* and every file .claude/agents/furrow-*.md.
func FindCopies(root string) ([]string, error) {
	copies := []string{}
	for _, c := range []struct {
		dir  string
		dirs bool
	}{{".claude/skills", true}, {".claude/agents", false}} {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(c.dir)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "furrow-") || (!c.dirs && !strings.HasSuffix(name, ".md")) {
				continue
			}
			info, err := os.Stat(filepath.Join(root, filepath.FromSlash(c.dir), name))
			if err != nil || info.IsDir() != c.dirs {
				continue
			}
			copies = append(copies, c.dir+"/"+name)
		}
	}
	sort.Strings(copies)
	return copies, nil
}
