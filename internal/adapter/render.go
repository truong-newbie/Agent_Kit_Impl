package adapter

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"text/template"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

type templateVars struct {
	SkillName     string
	Scope         string
	KitID         string
	KitVersion    string
	SkillNames    map[string]string
	ResourcePaths map[string]string
}

// RenderForRuntime renders a kit package for the named supported runtime and scope.
func RenderForRuntime(runtime string, p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	profile, ok := Profile(runtime)
	if !ok {
		return nil, apperr.Usage("unsupported_runtime", fmt.Sprintf("unsupported runtime %q", runtime))
	}
	return RenderKit(profile, p, source, scope)
}

// CapabilityPlan summarizes DTKit's actual managed capabilities, runtime integration evidence,
// and deterministic fallback warnings for a given runtime profile, scope, and optional package.
type CapabilityPlan struct {
	Capabilities Capabilities
	Integration  IntegrationEvidence
	Warnings     []string
}

// EvaluateCapabilities strictly validates required package capabilities against what DTKit manages
// in (profile, scope) and collects deterministic optional-fallback warnings without filesystem access.
func EvaluateCapabilities(profile RuntimeProfile, scope string, pkg *catalog.Package) (CapabilityPlan, error) {
	var out CapabilityPlan
	if !profile.SupportsScope(scope) {
		return out, apperr.Usage("invalid_scope", fmt.Sprintf("unsupported scope %q for runtime %q", scope, profile.ID))
	}
	capSet, hasCaps := profile.CapabilitiesMap[scope]
	if !hasCaps {
		return out, apperr.Usage("unsupported_scope", fmt.Sprintf("runtime %q does not declare capabilities for %s scope", profile.ID, scope))
	}
	out.Capabilities = capSet
	if integ, ok := profile.IntegrationMap[scope]; ok {
		out.Integration = integ
	} else {
		out.Integration = IntegrationEvidence{Status: IntegrationUnverified, Note: "runtime integration unverified"}
	}

	_, hasInstr := profile.Instructions[scope]
	_, hasSkills := profile.Skills[scope]

	checkCap := func(name string) (supported bool, note string, known bool) {
		switch name {
		case "instructions":
			return hasInstr && capSet.Instructions.DTKitSupported, capSet.Instructions.Note, true
		case "skills":
			return hasSkills && capSet.Skills.DTKitSupported, capSet.Skills.Note, true
		case "hooks":
			return false, capSet.Hooks.Note, true
		default:
			return false, "", false
		}
	}

	if pkg != nil {
		for _, reqCap := range pkg.RequiresCapabilities {
			ok, note, known := checkCap(reqCap)
			if !known {
				return out, apperr.Usage("unsupported_capability", fmt.Sprintf("package %q requires unknown capability %q", pkg.ID, reqCap))
			}
			if !ok {
				detail := fmt.Sprintf("runtime %s %s scope does not support DTKit-managed required capability %q", profile.ID, scope, reqCap)
				if note != "" {
					detail += " (" + note + ")"
				}
				hint := fmt.Sprintf("choose a supported runtime/scope, or declare %q in optionalCapabilities if fallback is intended", reqCap)
				if reqCap == "hooks" {
					hint = "kit packages cannot auto-install hooks; remove \"hooks\" from requiresCapabilities (for Codex, enable opt-in hooks via 'dtkit hooks enable session-context')"
				}
				return out, apperr.WithHint(apperr.Usage("unsupported_capability", detail), hint)
			}
		}
		for _, optCap := range pkg.OptionalCapabilities {
			ok, note, known := checkCap(optCap)
			if !known {
				return out, apperr.Usage("unsupported_capability", fmt.Sprintf("package %q declares unknown optional capability %q", pkg.ID, optCap))
			}
			if !ok {
				msg := note
				if msg == "" {
					msg = fmt.Sprintf("runtime %s %s scope does not support optional capability %q; skipping", profile.ID, scope, optCap)
				}
				out.Warnings = append(out.Warnings, msg)
			}
		}
	} else if !capSet.Instructions.DTKitSupported && capSet.Instructions.Note != "" {
		out.Warnings = append(out.Warnings, capSet.Instructions.Note)
	}
	return out, nil
}

// RenderKit deterministically renders a validated kit package for a specific RuntimeProfile and scope
// without touching the filesystem.
func RenderKit(profile RuntimeProfile, p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	if !profile.SupportsScope(scope) {
		return nil, apperr.Usage("invalid_scope", fmt.Sprintf("unsupported scope %q for runtime %q", scope, profile.ID))
	}
	if err := catalog.Validate(p, source); err != nil {
		return nil, err
	}
	if !contains(p.RuntimeTargets, profile.ID) {
		return nil, apperr.Usage("unsupported_runtime", fmt.Sprintf("package %q does not target %s", p.ID, profile.ID))
	}
	if !contains(p.Scopes, scope) {
		return nil, apperr.Usage("unsupported_scope", fmt.Sprintf("package %q does not support scope %q", p.ID, scope))
	}
	if _, err := EvaluateCapabilities(profile, scope, &p); err != nil {
		return nil, err
	}

	skillLoc, hasSkills := profile.Skills[scope]
	instrLoc, hasInstr := profile.Instructions[scope]
	contentRoot, hasContent := profile.ContentRoot[scope]
	if !hasSkills || !hasContent {
		return nil, apperr.Usage("unsupported_scope", fmt.Sprintf("runtime %q does not define complete %s layout", profile.ID, scope))
	}
	if !hasInstr {
		if capSet, ok := profile.CapabilitiesMap[scope]; !ok || capSet.Instructions.DTKitSupported {
			return nil, apperr.Usage("unsupported_scope", fmt.Sprintf("runtime %q missing instructions layout for %s scope", profile.ID, scope))
		}
	}

	resourceDisplayPrefix := path.Join(profile.ContentRelBase, p.ID)
	if scope == "global" {
		resourceDisplayPrefix = path.Join("$HOME", resourceDisplayPrefix)
	}

	skillNames := map[string]string{}
	resourcePaths := map[string]string{}
	for _, c := range p.Components {
		if c.Kind == "skill" {
			slug := strings.TrimPrefix(c.ID, "skill/")
			skillNames[slug] = skillLoc.Prefix + slug
		} else {
			resourcePaths[c.ID] = path.Join(resourceDisplayPrefix, c.Source)
		}
	}

	artifacts := make([]Artifact, 0, len(p.Components)+1)
	for _, component := range p.Components {
		if component.Kind == "skill" {
			id := strings.TrimPrefix(component.ID, "skill/")
			if id == component.ID || strings.ContainsAny(id, "/\\") || id == "" {
				return nil, apperr.Usage("invalid_skill_id", fmt.Sprintf("invalid skill id %q", component.ID))
			}
			name := skillNames[id]
			if strings.Contains(name, "--") {
				return nil, apperr.Usage("invalid_skill_name", fmt.Sprintf("rendered skill name %q cannot contain '--'", name))
			}
			var content []byte
			if b, err := fs.ReadFile(source, component.Source+"/SKILL.md.tmpl"); err == nil {
				tmpl, err := template.New(id).Option("missingkey=error").Parse(string(b))
				if err != nil {
					return nil, apperr.Wrap(apperr.ExitUsage, "invalid_skill_template", fmt.Sprintf("skill %s: %v", id, err), err)
				}
				var rendered bytes.Buffer
				vars := templateVars{
					SkillName:     name,
					Scope:         scope,
					KitID:         p.ID,
					KitVersion:    p.Version,
					SkillNames:    skillNames,
					ResourcePaths: resourcePaths,
				}
				if err := tmpl.Execute(&rendered, vars); err != nil {
					return nil, apperr.Wrap(apperr.ExitUsage, "template_render_failed", fmt.Sprintf("skill %s: %v", id, err), err)
				}
				content = rendered.Bytes()
			} else if staticBytes, staticErr := fs.ReadFile(source, component.Source+"/SKILL.md"); staticErr == nil {
				content = staticBytes
			} else {
				return nil, apperr.Wrap(apperr.ExitUsage, "missing_skill_file", fmt.Sprintf("skill %s: %v", id, err), err)
			}

			if bytes.Contains(content, []byte("{{")) || bytes.Contains(content, []byte("}}")) {
				return nil, apperr.Usage("unrendered_template", fmt.Sprintf("skill %s contains unrendered template markers", id))
			}
			if err := ValidateSkill(content, name); err != nil {
				return nil, apperr.Wrap(apperr.ExitUsage, "invalid_skill_frontmatter", fmt.Sprintf("skill %s: %v", id, err), err)
			}
			relSkillPath := path.Join(skillLoc.Dir, name, "SKILL.md")
			if skillLoc.Dir == "" {
				relSkillPath = path.Join(name, "SKILL.md")
			}
			artifacts = append(artifacts, Artifact{
				Root:        skillLoc.Root,
				Path:        relSkillPath,
				Strategy:    "whole-file",
				ComponentID: component.ID,
				Content:     content,
			})
			continue
		}

		content, err := fs.ReadFile(source, component.Source)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(component.Source, ".tmpl") {
			tmpl, err := template.New(component.ID).Option("missingkey=error").Parse(string(content))
			if err != nil {
				return nil, apperr.Wrap(apperr.ExitUsage, "invalid_template", fmt.Sprintf("component %s: %v", component.ID, err), err)
			}
			var rendered bytes.Buffer
			vars := templateVars{
				Scope:         scope,
				KitID:         p.ID,
				KitVersion:    p.Version,
				SkillNames:    skillNames,
				ResourcePaths: resourcePaths,
			}
			if err := tmpl.Execute(&rendered, vars); err != nil {
				return nil, apperr.Wrap(apperr.ExitUsage, "template_render_failed", fmt.Sprintf("component %s: %v", component.ID, err), err)
			}
			content = rendered.Bytes()
		}
		outSource := strings.TrimSuffix(component.Source, ".tmpl")
		artifacts = append(artifacts, Artifact{
			Root:        contentRoot,
			Path:        path.Join(profile.ContentRelBase, p.ID, outSource),
			Strategy:    "whole-file",
			ComponentID: component.ID,
			Content:     content,
		})
	}

	if hasInstr {
		preference := ""
		if scope == "project" {
			if projSkills, ok := profile.Skills["project"]; ok {
				projGroup := strings.TrimSuffix(projSkills.Prefix, "-")
				preference = fmt.Sprintf(" Prefer %s skills for this project when both scopes are installed.", projGroup)
			}
		}
		skillGroup := strings.TrimSuffix(skillLoc.Prefix, "-")
		body := fmt.Sprintf("# DTKit engineering guidance\n\nInspect relevant project instructions and code before changes. Respect the user's scope, preserve unrelated work, and report actual verification results. Use the %s skills when appropriate.%s For relevant tasks, consult workflows, rules and templates under %s.\n",
			skillGroup, preference, resourceDisplayPrefix)
		strategy := instrLoc.EffectiveStrategy()
		var contentStr string
		if strategy == "whole-file" {
			contentStr = fmt.Sprintf("---\ndescription: DTKit %s %s engineering guidance\nalwaysApply: true\n---\n\n%s", p.ID, scope, body)
		} else {
			tag := profile.BlockTag(p.ID, scope)
			contentStr = fmt.Sprintf("<!-- dtkit:begin %s -->\n%s<!-- dtkit:end %s -->\n", tag, body, tag)
		}
		artifacts = append(artifacts, Artifact{
			Root:        instrLoc.Root,
			Path:        instrLoc.RelPath(p.ID),
			Strategy:    strategy,
			ComponentID: "instructions",
			Content:     []byte(contentStr),
		})
	}

	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Root != artifacts[j].Root {
			return artifacts[i].Root < artifacts[j].Root
		}
		return artifacts[i].Path < artifacts[j].Path
	})
	return artifacts, nil
}

func contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// ValidateSkill checks the narrow YAML frontmatter contract (name + description + non-empty body).
func ValidateSkill(content []byte, name string) error {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	if len(lines) < 5 || lines[0] != "---" {
		return fmt.Errorf("missing YAML frontmatter")
	}
	seenName, seenDescription, end := false, false, -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
		if strings.HasPrefix(lines[i], "name: ") {
			if seenName || strings.TrimPrefix(lines[i], "name: ") != name {
				return fmt.Errorf("invalid skill name")
			}
			seenName = true
		} else if strings.HasPrefix(lines[i], "description: ") {
			desc := strings.TrimSpace(strings.TrimPrefix(lines[i], "description: "))
			if seenDescription || len(desc) == 0 || len(desc) > 1024 {
				return fmt.Errorf("invalid skill description")
			}
			seenDescription = true
		} else {
			return fmt.Errorf("unsupported frontmatter field")
		}
	}
	if end < 0 || !seenName || !seenDescription || strings.TrimSpace(strings.Join(lines[end+1:], "\n")) == "" {
		return fmt.Errorf("incomplete skill metadata or body")
	}
	return nil
}
