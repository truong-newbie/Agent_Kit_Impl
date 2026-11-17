package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
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
	SchemaVersion        int                 `json:"schemaVersion"`
	ID                   string              `json:"id"`
	Name                 string              `json:"name"`
	Version              string              `json:"version"`
	MinCLIVersion        string              `json:"minCliVersion,omitempty"`
	Description          string              `json:"description"`
	RuntimeTargets       []string            `json:"runtimeTargets"`
	Scopes               []string            `json:"scopes"`
	RequiresCapabilities []string            `json:"requiresCapabilities"`
	OptionalCapabilities []string            `json:"optionalCapabilities,omitempty"`
	PackageDependencies  []string            `json:"packageDependencies"`
	Components           []Component         `json:"components"`
	Presets              map[string][]string `json:"presets,omitempty"`
}

// SelectionSpec records preset/component filtering resolved deterministically across dependencies.
type SelectionSpec struct {
	Preset              string   `json:"preset,omitempty"`
	RequestedComponents []string `json:"requestedComponents,omitempty"`
	ResolvedComponents  []string `json:"resolvedComponents"`
}

func Engineer() (Package, fs.FS, error) {
	f, err := fs.Sub(files, "engineer")
	if err != nil {
		return Package{}, nil, err
	}
	p, err := Load(f)
	return p, f, err
}

// LoadDir validates and snapshots a local package directory into an immutable
// in-memory filesystem so later apply steps cannot read mutated files.
func LoadDir(dir string) (Package, fs.FS, string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Package{}, nil, "", err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return Package{}, nil, "", apperr.WithPath(apperr.Usage("package_not_found", "package directory does not exist"), abs)
		}
		return Package{}, nil, "", err
	}
	if paths.IsReparseOrSymlink(info) {
		return Package{}, nil, "", apperr.WithPath(apperr.Usage("symlink_not_allowed", "package directory cannot be a symbolic link or reparse point"), abs)
	}
	if !info.IsDir() {
		return Package{}, nil, "", apperr.WithPath(apperr.Usage("invalid_package_dir", "package path is not a directory"), abs)
	}
	diskFS := os.DirFS(abs)
	pkg, err := Load(diskFS)
	if err != nil {
		return Package{}, nil, "", err
	}
	snap, digest, err := Snapshot(diskFS)
	if err != nil {
		return Package{}, nil, "", err
	}
	return pkg, snap, digest, nil
}

// Snapshot reads a validated package tree into an immutable MapFS and computes its digest.
func Snapshot(source fs.FS) (fs.FS, string, error) {
	if err := checkTree(source); err != nil {
		return nil, "", err
	}
	snap := fstest.MapFS{}
	var total int64
	count := 0
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return apperr.WithPath(apperr.Usage("symlink_not_allowed", fmt.Sprintf("symbolic link not allowed: %s", name)), name)
		}
		file, err := source.Open(name)
		if err != nil {
			return err
		}
		contents, readErr := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		count++
		total += int64(len(contents))
		if len(contents) > MaxFileBytes || count > MaxFiles || total > MaxPackageBytes {
			return apperr.WithPath(apperr.Usage("package_size_limit", fmt.Sprintf("package size limit exceeded: %s", name)), name)
		}
		if err := checkContentSafety(name, contents); err != nil {
			return err
		}
		snap[name] = &fstest.MapFile{Data: contents, Mode: 0644}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	digest, err := Digest(snap)
	if err != nil {
		return nil, "", err
	}
	return snap, digest, nil
}

func Load(fsys fs.FS) (Package, error) {
	var p Package
	if err := checkTree(fsys); err != nil {
		return p, err
	}
	file, err := fsys.Open("package.json")
	if err != nil {
		return p, apperr.Wrap(apperr.ExitUsage, "missing_manifest", "cannot open package.json", err)
	}
	b, readErr := io.ReadAll(io.LimitReader(file, MaxManifestBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return p, readErr
	}
	if closeErr != nil {
		return p, closeErr
	}
	if len(b) > MaxManifestBytes {
		return p, apperr.Usage("manifest_too_large", "manifest exceeds 256 KiB")
	}
	if err := CheckStrictJSON(b); err != nil {
		return p, apperr.Wrap(apperr.ExitUsage, "invalid_manifest_json", fmt.Sprintf("manifest: %v", err), err)
	}
	if err := checkManifestKeys(b); err != nil {
		return p, apperr.Wrap(apperr.ExitUsage, "invalid_manifest_schema", fmt.Sprintf("manifest: %v", err), err)
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return p, apperr.Wrap(apperr.ExitUsage, "invalid_manifest_schema", fmt.Sprintf("manifest: %v", err), err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return p, apperr.Usage("invalid_manifest_json", "manifest must contain exactly one JSON value")
	}
	if err := Validate(p, fsys); err != nil {
		return p, err
	}
	return p, nil
}

func Validate(p Package, fsys fs.FS) error {
	if _, err := ResolveOrder(p, fsys); err != nil {
		return err
	}
	return nil
}

// ResolveOrder validates package metadata, components, and dependency graph,
// returning components in a deterministic topological order.
func ResolveOrder(p Package, fsys fs.FS) ([]Component, error) {
	if p.SchemaVersion != 1 {
		return nil, apperr.Usage("unsupported_package_schema", fmt.Sprintf("unsupported package schemaVersion %d (supported: 1)", p.SchemaVersion))
	}
	if !validID(p.ID, 32) || !versionPattern.MatchString(p.Version) || strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.Description) == "" {
		return nil, apperr.Usage("invalid_package_metadata", "invalid package metadata")
	}
	if p.MinCLIVersion != "" && !versionPattern.MatchString(p.MinCLIVersion) {
		return nil, apperr.Usage("invalid_package_metadata", fmt.Sprintf("invalid minCliVersion %q", p.MinCLIVersion))
	}
	allowedCaps := []string{"instructions", "skills", "hooks"}
	if !subsetUnique(p.RuntimeTargets, []string{"codex", "gemini", "opencode", "cursor", "claude"}) || !subsetUnique(p.Scopes, []string{"project", "global"}) || !subsetUniqueOptional(p.RequiresCapabilities, allowedCaps) || !subsetUniqueOptional(p.OptionalCapabilities, allowedCaps) {
		return nil, apperr.Usage("unsupported_package_target", "unsupported runtime, scope or capability")
	}
	if len(p.RequiresCapabilities)+len(p.OptionalCapabilities) == 0 {
		return nil, apperr.Usage("unsupported_package_target", "package must declare at least one capability in requiresCapabilities or optionalCapabilities")
	}
	reqSet := make(map[string]bool, len(p.RequiresCapabilities))
	for _, rc := range p.RequiresCapabilities {
		reqSet[rc] = true
	}
	for _, oc := range p.OptionalCapabilities {
		if reqSet[oc] {
			return nil, apperr.Usage("conflicting_capability_contract", fmt.Sprintf("capability %q cannot appear in both requiresCapabilities and optionalCapabilities", oc))
		}
	}
	if len(p.PackageDependencies) != 0 {
		return nil, apperr.Usage("unsupported_package_deps", "package dependencies are not supported in v0.1")
	}
	if len(p.Components) == 0 || len(p.Components) > MaxComponents {
		return nil, apperr.Usage("invalid_component_count", "invalid component count")
	}
	ids := map[string]Component{}
	sources := map[string]string{}
	for _, c := range p.Components {
		if _, ok := ids[c.ID]; ok {
			return nil, apperr.Usage("duplicate_component", fmt.Sprintf("duplicate component %q", c.ID))
		}
		prefix := c.Kind + "/"
		if !strings.HasPrefix(c.ID, prefix) || !validID(strings.TrimPrefix(c.ID, prefix), 32) || !paths.ValidateRelativePath(c.Source) {
			return nil, apperr.Usage("invalid_component", fmt.Sprintf("invalid component path or id: %q", c.ID))
		}
		switch c.Kind {
		case "skill", "rule", "workflow", "template":
		default:
			return nil, apperr.Usage("unsupported_component_kind", fmt.Sprintf("unsupported component kind %q", c.Kind))
		}
		key := strings.ToLower(c.Source)
		if prev, ok := sources[key]; ok {
			return nil, apperr.Usage("duplicate_component_source", fmt.Sprintf("duplicate component source %q and %q", prev, c.Source))
		}
		sources[key] = c.Source
		info, err := fs.Stat(fsys, c.Source)
		if err != nil {
			return nil, apperr.Wrap(apperr.ExitUsage, "missing_component_source", fmt.Sprintf("component %s: %v", c.ID, err), err)
		}
		if c.Kind == "skill" {
			if !info.IsDir() {
				return nil, apperr.Usage("invalid_skill_source", fmt.Sprintf("skill %s must be a directory", c.ID))
			}
			_, tmplErr := fs.Stat(fsys, c.Source+"/SKILL.md.tmpl")
			_, staticErr := fs.Stat(fsys, c.Source+"/SKILL.md")
			if tmplErr == nil && staticErr == nil {
				return nil, apperr.Usage("ambiguous_skill_source", fmt.Sprintf("skill %s cannot contain both SKILL.md.tmpl and SKILL.md", c.ID))
			}
			if tmplErr != nil && staticErr != nil {
				return nil, apperr.Wrap(apperr.ExitUsage, "missing_skill_file", fmt.Sprintf("skill %s missing SKILL.md.tmpl or SKILL.md", c.ID), tmplErr)
			}
		} else if !info.Mode().IsRegular() {
			return nil, apperr.Usage("invalid_component_source", fmt.Sprintf("component %s must be a regular file", c.ID))
		}
		ids[c.ID] = c
	}

	marks := map[string]int{}
	order := make([]Component, 0, len(p.Components))
	var stack []string
	var visit func(string) error
	visit = func(id string) error {
		if marks[id] == 1 {
			cycle := append(append([]string{}, stack...), id)
			return apperr.Usage("dependency_cycle", fmt.Sprintf("dependency cycle: %s", strings.Join(cycle, " -> ")))
		}
		if marks[id] == 2 {
			return nil
		}
		c, ok := ids[id]
		if !ok {
			return apperr.Usage("missing_dependency", fmt.Sprintf("missing dependency %s", id))
		}
		marks[id] = 1
		stack = append(stack, id)
		deps := append([]string(nil), c.Requires...)
		sort.Strings(deps)
		for _, dep := range deps {
			if err := visit(dep); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		marks[id] = 2
		order = append(order, c)
		return nil
	}
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	for presetName, presetComps := range p.Presets {
		if !validID(presetName, 32) {
			return nil, apperr.Usage("invalid_preset", fmt.Sprintf("invalid preset name %q", presetName))
		}
		if len(presetComps) == 0 {
			return nil, apperr.Usage("invalid_preset", fmt.Sprintf("preset %q must list at least one component", presetName))
		}
		seenPreset := map[string]bool{}
		for _, cid := range presetComps {
			if _, ok := ids[cid]; !ok {
				return nil, apperr.Usage("invalid_preset", fmt.Sprintf("preset %q references unknown component %q", presetName, cid))
			}
			if seenPreset[cid] {
				return nil, apperr.Usage("invalid_preset", fmt.Sprintf("preset %q lists duplicate component %q", presetName, cid))
			}
			seenPreset[cid] = true
		}
	}
	return order, nil
}

// ResolveComponentSelection expands a preset or explicit component list deterministically
// over each component's Requires dependency graph. Nil is returned when no filter is requested (full kit).
func ResolveComponentSelection(p Package, fsys fs.FS, preset string, requested []string) (*SelectionSpec, error) {
	if preset != "" && len(requested) > 0 {
		return nil, apperr.Usage("conflicting_selection", "choose at most one of --preset or --components")
	}
	if _, err := ResolveOrder(p, fsys); err != nil {
		return nil, err
	}
	if preset == "" && len(requested) == 0 {
		return nil, nil
	}

	byID := make(map[string]Component, len(p.Components))
	for _, c := range p.Components {
		byID[c.ID] = c
	}

	var seeds []string
	var spec SelectionSpec
	if preset != "" {
		presetSeeds, ok := p.Presets[preset]
		if !ok {
			var avail []string
			for k := range p.Presets {
				avail = append(avail, k)
			}
			sort.Strings(avail)
			hint := "omit --preset to install the entire kit"
			if len(avail) > 0 {
				hint = fmt.Sprintf("available presets for %s: %s", p.ID, strings.Join(avail, ", "))
			}
			return nil, apperr.WithHint(apperr.Usage("unknown_preset", fmt.Sprintf("unknown preset %q for kit %q", preset, p.ID)), hint)
		}
		spec.Preset = preset
		seeds = append(seeds, presetSeeds...)
	} else {
		seenReq := map[string]bool{}
		for _, raw := range requested {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			if _, ok := byID[id]; !ok {
				return nil, apperr.Usage("unknown_component", fmt.Sprintf("unknown component %q in kit %q", id, p.ID))
			}
			if !seenReq[id] {
				seenReq[id] = true
				spec.RequestedComponents = append(spec.RequestedComponents, id)
			}
		}
		if len(spec.RequestedComponents) == 0 {
			return nil, apperr.Usage("empty_components", "--components must specify at least one valid component ID")
		}
		sort.Strings(spec.RequestedComponents)
		seeds = append(seeds, spec.RequestedComponents...)
	}

	resolvedSet := map[string]bool{}
	var addWithDeps func(string)
	addWithDeps = func(id string) {
		if resolvedSet[id] {
			return
		}
		resolvedSet[id] = true
		for _, dep := range byID[id].Requires {
			addWithDeps(dep)
		}
	}
	for _, s := range seeds {
		addWithDeps(s)
	}
	for id := range resolvedSet {
		spec.ResolvedComponents = append(spec.ResolvedComponents, id)
	}
	sort.Strings(spec.ResolvedComponents)
	return &spec, nil
}

// SelectionsEqual reports whether two optional component selections describe the same selection.
func SelectionsEqual(a, b *SelectionSpec) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Preset != b.Preset || len(a.RequestedComponents) != len(b.RequestedComponents) || len(a.ResolvedComponents) != len(b.ResolvedComponents) {
		return false
	}
	for i := range a.RequestedComponents {
		if a.RequestedComponents[i] != b.RequestedComponents[i] {
			return false
		}
	}
	for i := range a.ResolvedComponents {
		if a.ResolvedComponents[i] != b.ResolvedComponents[i] {
			return false
		}
	}
	return true
}
