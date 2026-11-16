package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed engineer
var files embed.FS

type Component struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Source   string   `json:"source"`
	Requires []string `json:"requires"`
}

type Package struct {
	SchemaVersion       int         `json:"schemaVersion"`
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Version             string      `json:"version"`
	Description         string      `json:"description"`
	RuntimeTargets      []string    `json:"runtimeTargets"`
	Scopes              []string    `json:"scopes"`
	RequiresCapabilities []string   `json:"requiresCapabilities"`
	PackageDependencies []string    `json:"packageDependencies"`
	Components          []Component `json:"components"`
}

func Engineer() (Package, fs.FS, error) {
	f, err := fs.Sub(files, "engineer")
	if err != nil { return Package{}, nil, err }
	p, err := Load(f)
	return p, f, err
}

func Load(fsys fs.FS) (Package, error) {
	var p Package
	b, err := fs.ReadFile(fsys, "package.json")
	if err != nil { return p, err }
	if len(b) > 256*1024 { return p, fmt.Errorf("manifest exceeds 256 KiB") }
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil { return p, fmt.Errorf("manifest: %w", err) }
	if err := Validate(p, fsys); err != nil { return p, err }
	return p, nil
}

func Validate(p Package, fsys fs.FS) error {
	if p.SchemaVersion != 1 || p.ID == "" || p.Version == "" || p.Name == "" { return fmt.Errorf("invalid package metadata") }
	if len(p.PackageDependencies) != 0 { return fmt.Errorf("package dependencies are not supported") }
	if len(p.Components) == 0 || len(p.Components) > 128 { return fmt.Errorf("invalid component count") }
	ids := map[string]Component{}
	for _, c := range p.Components {
		if _, ok := ids[c.ID]; ok { return fmt.Errorf("duplicate component %q", c.ID) }
		if c.ID == "" || !fs.ValidPath(c.Source) || c.Source == "." { return fmt.Errorf("invalid component path or id: %q", c.ID) }
		switch c.Kind { case "skill", "rule", "workflow", "template": default: return fmt.Errorf("unsupported component kind %q", c.Kind) }
		if _, err := fs.Stat(fsys, c.Source); err != nil { return fmt.Errorf("component %s: %w", c.ID, err) }
		ids[c.ID] = c
	}
	marks := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if marks[id] == 1 { return fmt.Errorf("dependency cycle at %s", id) }
		if marks[id] == 2 { return nil }
		c, ok := ids[id]; if !ok { return fmt.Errorf("missing dependency %s", id) }
		marks[id] = 1
		for _, dep := range c.Requires { if err := visit(dep); err != nil { return err } }
		marks[id] = 2
		return nil
	}
	keys := make([]string, 0, len(ids)); for id := range ids { keys = append(keys, id) }; sort.Strings(keys)
	for _, id := range keys { if err := visit(id); err != nil { return err } }
	return nil
}
