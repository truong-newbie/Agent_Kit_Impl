// Package dist implements v0.3 kit authoring, deterministic packaging,
// hostile-archive validation, digest-keyed caching, and explicit remote resolution.
package dist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

var slugPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

type CreateKitResult struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Dir     string `json:"dir"`
	Digest  string `json:"digest"`
}

type CreateSkillResult struct {
	KitID   string `json:"kitId"`
	SkillID string `json:"skillId"`
	Dir     string `json:"dir"`
	File    string `json:"file"`
	Digest  string `json:"digest"`
}

// CreateKit scaffolds a valid standalone kit package directory that passes dtkit validate.
func CreateKit(parentDir, id, displayName string) (CreateKitResult, error) {
	var res CreateKitResult
	if len(id) > 32 || !slugPattern.MatchString(id) {
		return res, apperr.Usage("invalid_kit_id", fmt.Sprintf("kit ID %q must be <=32 lowercase ASCII letters/digits/hyphens", id))
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = "DTKit " + id + " Kit"
	}
	absParent, err := filepath.Abs(parentDir)
	if err != nil {
		return res, err
	}
	kitDir := filepath.Join(absParent, id)
	if _, err := os.Lstat(kitDir); err == nil {
		return res, apperr.WithPath(apperr.Conflict("target_exists", "kit directory already exists"), kitDir)
	} else if !os.IsNotExist(err) {
		return res, err
	}

	rulesDir := filepath.Join(kitDir, "rules")
	skillDir := filepath.Join(kitDir, "skills", "example")
	for _, d := range []string{rulesDir, skillDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return res, err
		}
	}

	ruleText := "# Engineering guidance\n\n- Inspect existing code and instructions before editing.\n- Keep changes focused and report actual verification results.\n"
	if err := writeRegularFile(filepath.Join(rulesDir, "engineering.md"), []byte(ruleText), 0644); err != nil {
		return res, err
	}

	skillTmpl := `---
name: {{.SkillName}}
description: Starter skill for the ` + id + ` kit. Customize instructions and scope for your workflow.
---

# Example Skill

Follow the kit guidance in ` + "`{{index .ResourcePaths \"rule/engineering\"}}`" + `.

## When to use
- Use when applying the starter workflow for ` + id + `.

## When not to use
- Do not use for unrelated changes outside the requested scope.

## Steps
1. Read relevant repository files and instructions.
2. Confirm scope and constraints before modifying files.
3. Run verification checks and report actual results.

## Constraints
- Respect existing conventions and avoid unrequested API changes.

## Examples
1. **Inspection:** Review a target module and summarize findings without editing.
2. **Scoped change:** Apply a focused edit and verify tests pass.
3. **Missing information:** Ask concise clarifying questions when requirements conflict.
`
	if err := writeRegularFile(filepath.Join(skillDir, "SKILL.md.tmpl"), []byte(skillTmpl), 0644); err != nil {
		return res, err
	}

	pkg := catalog.Package{
		SchemaVersion:        1,
		ID:                   id,
		Name:                 displayName,
		Version:              "0.1.0",
		Description:          "Custom DTKit engineering package.",
		RuntimeTargets:       adapter.SupportedRuntimes(),
		Scopes:               []string{"project", "global"},
		RequiresCapabilities: []string{"skills"},
		OptionalCapabilities: []string{"instructions"},
		PackageDependencies:  []string{},
		Components: []catalog.Component{
			{ID: "rule/engineering", Kind: "rule", Source: "rules/engineering.md", Requires: []string{}},
			{ID: "skill/example", Kind: "skill", Source: "skills/example", Requires: []string{"rule/engineering"}},
		},
	}
	manifestBytes, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return res, err
	}
	if err := writeRegularFile(filepath.Join(kitDir, "package.json"), append(manifestBytes, '\n'), 0644); err != nil {
		return res, err
	}

	loaded, snap, digest, err := catalog.LoadDir(kitDir)
	if err != nil {
		return res, err
	}
	if err := validatePackageRenderTargets(loaded, snap); err != nil {
		return res, err
	}
	return CreateKitResult{ID: loaded.ID, Version: loaded.Version, Dir: kitDir, Digest: digest}, nil
}

// CreateSkill adds a new templated skill to an existing local kit directory and re-validates it.
func CreateSkill(kitDir, slug, description string) (CreateSkillResult, error) {
	var res CreateSkillResult
	if len(slug) > 32 || !slugPattern.MatchString(slug) || strings.Contains(slug, "--") {
		return res, apperr.Usage("invalid_skill_slug", fmt.Sprintf("skill slug %q must be <=32 lowercase ASCII chars without '--'", slug))
	}
	if strings.TrimSpace(description) == "" {
		description = "Perform the " + slug + " engineering task with repository evidence."
	}

	pkg, _, _, err := catalog.LoadDir(kitDir)
	if err != nil {
		return res, err
	}
	compID := "skill/" + slug
	for _, c := range pkg.Components {
		if c.ID == compID {
			return res, apperr.Conflict("duplicate_component", fmt.Sprintf("component %q already exists in kit", compID))
		}
	}

	absKit, err := filepath.Abs(kitDir)
	if err != nil {
		return res, err
	}
	skillDir, err := paths.ResolveTarget(absKit, "skills/"+slug)
	if err != nil {
		return res, err
	}
	if _, err := os.Lstat(skillDir); err == nil {
		return res, apperr.WithPath(apperr.Conflict("target_exists", "skill directory already exists"), skillDir)
	} else if !os.IsNotExist(err) {
		return res, err
	}
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return res, err
	}

	tmpl := `---
name: {{.SkillName}}
description: ` + strings.ReplaceAll(strings.TrimSpace(description), "\n", " ") + `
---

# ` + slug + `

## When to use
- Use when the task matches ` + slug + `.

## When not to use
- Do not modify files when the request is read-only.

## Steps
1. Read relevant repository context before acting.
2. Execute the scoped steps for ` + slug + `.
3. Report observed evidence and actual check results.

## Constraints
- Respect existing architecture and user scope boundaries.

## Examples
1. **Standard use:** Apply ` + slug + ` to an existing module with tests.
2. **Scoped check:** Perform analysis without unrequested edits.
3. **Missing information:** Ask concise questions if essential requirements are absent.
`
	skillFile := filepath.Join(skillDir, "SKILL.md.tmpl")
	if err := writeRegularFile(skillFile, []byte(tmpl), 0644); err != nil {
		return res, err
	}

	requires := []string{}
	for _, c := range pkg.Components {
		if c.ID == "rule/engineering" {
			requires = []string{"rule/engineering"}
			break
		}
	}
	pkg.Components = append(pkg.Components, catalog.Component{
		ID:       compID,
		Kind:     "skill",
		Source:   "skills/" + slug,
		Requires: requires,
	})
	manifestBytes, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return res, err
	}
	if err := writeRegularFile(filepath.Join(absKit, "package.json"), append(manifestBytes, '\n'), 0644); err != nil {
		return res, err
	}

	loaded, snap, digest, err := catalog.LoadDir(absKit)
	if err != nil {
		return res, err
	}
	if err := validatePackageRenderTargets(loaded, snap); err != nil {
		return res, err
	}
	return CreateSkillResult{KitID: loaded.ID, SkillID: compID, Dir: skillDir, File: skillFile, Digest: digest}, nil
}

func writeRegularFile(target string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(target)
	temp, err := os.CreateTemp(dir, ".dtkit-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err = temp.Chmod(mode); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempName, target)
}
