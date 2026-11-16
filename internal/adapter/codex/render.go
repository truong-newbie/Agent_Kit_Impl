// Package codex renders DTKit content without modifying the filesystem.
package codex

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

const AdapterRevision = "1"

type Artifact struct {
	Root        string
	Path        string
	Strategy    string
	ComponentID string
	Content     []byte
}

type templateVars struct {
	SkillName     string
	Scope         string
	KitID         string
	KitVersion    string
	SkillNames    map[string]string
	ResourcePaths map[string]string
}

// Render emits logical paths only. Applying them requires a separate safe installer.
func Render(p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	if scope != "project" && scope != "global" {
		return nil, apperr.Usage("invalid_scope", fmt.Sprintf("unsupported scope %q", scope))
	}
	if err := catalog.Validate(p, source); err != nil {
		return nil, err
	}
	if !contains(p.RuntimeTargets, "codex") {
		return nil, apperr.Usage("unsupported_runtime", fmt.Sprintf("package %q does not target codex", p.ID))
	}
	if !contains(p.Scopes, scope) {
		return nil, apperr.Usage("unsupported_scope", fmt.Sprintf("package %q does not support scope %q", p.ID, scope))
	}

	root := "project"
	if scope == "global" {
		root = "user-home"
	}

	skillNames := map[string]string{}
	resourcePaths := map[string]string{}
	resourcePrefix := path.Join(".dtkit/content", p.ID)
	if scope == "global" {
		resourcePrefix = path.Join("$HOME/.dtkit/content", p.ID)
	}
	for _, c := range p.Components {
		if c.Kind == "skill" {
			slug := strings.TrimPrefix(c.ID, "skill/")
			skillNames[slug] = "dtkit-" + scope + "-" + slug
		} else {
			resourcePaths[c.ID] = path.Join(resourcePrefix, c.Source)
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
			if err := validateSkill(content, name); err != nil {
				return nil, apperr.Wrap(apperr.ExitUsage, "invalid_skill_frontmatter", fmt.Sprintf("skill %s: %v", id, err), err)
			}
			artifacts = append(artifacts, Artifact{
				Root:        root,
				Path:        path.Join(".agents/skills", name, "SKILL.md"),
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
			Root:        root,
			Path:        path.Join(".dtkit/content", p.ID, outSource),
			Strategy:    "whole-file",
			ComponentID: component.ID,
			Content:     content,
		})
	}

	preference := ""
	if scope == "project" {
		preference = " Prefer dtkit-project skills for this project when both scopes are installed."
	}
	block := fmt.Sprintf("<!-- dtkit:begin %s %s -->\n# DTKit engineering guidance\n\nInspect relevant project instructions and code before changes. Respect the user's scope, preserve unrelated work, and report actual verification results. Use the dtkit-%s skills when appropriate.%s For relevant tasks, consult workflows, rules and templates under %s.\n<!-- dtkit:end %s %s -->\n", p.ID, scope, scope, preference, resourcePrefix, p.ID, scope)
	instructionRoot := root
	if scope == "global" {
		instructionRoot = "codex-home"
	}
	artifacts = append(artifacts, Artifact{
		Root:        instructionRoot,
		Path:        "AGENTS.md",
		Strategy:    "managed-block",
		ComponentID: "instructions",
		Content:     []byte(block),
	})
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

// validateSkill checks the deliberately narrow frontmatter format of bundled
// templates. General third-party YAML requires a full parser before support.
func validateSkill(content []byte, name string) error {
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
