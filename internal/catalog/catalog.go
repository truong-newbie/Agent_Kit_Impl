package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"io"
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
	if err := checkTree(fsys); err != nil { return p, err }
	b, err := fs.ReadFile(fsys, "package.json")
	if err != nil { return p, err }
	if len(b) > 256*1024 { return p, fmt.Errorf("manifest exceeds 256 KiB") }
	if err := checkJSON(b); err != nil { return p, fmt.Errorf("manifest: %w", err) }
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil { return p, fmt.Errorf("manifest: %w", err) }
	if err := dec.Decode(new(any)); err != io.EOF { return p, fmt.Errorf("manifest must contain exactly one JSON value") }
	if err := Validate(p, fsys); err != nil { return p, err }
	return p, nil
}

func Validate(p Package, fsys fs.FS) error {
	if p.SchemaVersion != 1 || !validID(p.ID, 32) || !versionPattern.MatchString(p.Version) || strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Description) == "" { return fmt.Errorf("invalid package metadata") }
	if !equalSet(p.RuntimeTargets, []string{"codex"}) || !equalSet(p.Scopes, []string{"project", "global"}) || !equalSet(p.RequiresCapabilities, []string{"instructions", "skills"}) { return fmt.Errorf("unsupported runtime, scope or capability") }
	if len(p.PackageDependencies) != 0 { return fmt.Errorf("package dependencies are not supported") }
	if len(p.Components) == 0 || len(p.Components) > 128 { return fmt.Errorf("invalid component count") }
	ids := map[string]Component{}
	sources := map[string]string{}
	for _, c := range p.Components {
		if _, ok := ids[c.ID]; ok { return fmt.Errorf("duplicate component %q", c.ID) }
		prefix := c.Kind + "/"
		if !strings.HasPrefix(c.ID, prefix) || !validID(strings.TrimPrefix(c.ID, prefix), 32) || !validPath(c.Source) { return fmt.Errorf("invalid component path or id: %q", c.ID) }
		switch c.Kind { case "skill", "rule", "workflow", "template": default: return fmt.Errorf("unsupported component kind %q", c.Kind) }
		key := strings.ToLower(c.Source); if prev, ok := sources[key]; ok { return fmt.Errorf("duplicate component source %q and %q", prev, c.Source) }; sources[key] = c.Source
		info, err := fs.Stat(fsys, c.Source); if err != nil { return fmt.Errorf("component %s: %w", c.ID, err) }
		if c.Kind == "skill" {
			if !info.IsDir() { return fmt.Errorf("skill %s must be a directory", c.ID) }
			if _, err := fs.Stat(fsys, c.Source+"/SKILL.md.tmpl"); err != nil { return fmt.Errorf("skill %s missing template: %w", c.ID, err) }
		} else if !info.Mode().IsRegular() { return fmt.Errorf("component %s must be a regular file", c.ID) }
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
