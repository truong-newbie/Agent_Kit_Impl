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

	"dtkit.local/dtkit/internal/catalog"
)

type Artifact struct {
	Root     string
	Path     string
	Strategy string
	Content  []byte
}

// Render emits logical paths only. Applying them requires a separate safe installer.
func Render(p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	if scope != "project" && scope != "global" {
		return nil, fmt.Errorf("unsupported scope %q", scope)
	}
	if p.ID != "engineer" {
		return nil, fmt.Errorf("unsupported package %q", p.ID)
	}
	if err := catalog.Validate(p, source); err != nil {
		return nil, err
	}
	artifacts := make([]Artifact, 0, len(p.Components))
	root := "project"
	if scope == "global" {
		root = "user-home"
	}
	for _, component := range p.Components {
		if component.Kind == "skill" {
			id := strings.TrimPrefix(component.ID, "skill/")
			if id == component.ID || strings.ContainsAny(id, "/\\") || id == "" {
				return nil, fmt.Errorf("invalid skill id %q", component.ID)
			}
			name := "dtkit-" + scope + "-" + id
			b, err := fs.ReadFile(source, component.Source+"/SKILL.md.tmpl")
			if err != nil {
				return nil, fmt.Errorf("skill %s: %w", id, err)
			}
			tmpl, err := template.New(id).Option("missingkey=error").Parse(string(b))
			if err != nil {
				return nil, err
			}
			var rendered bytes.Buffer
			if err := tmpl.Execute(&rendered, struct{ SkillName string }{name}); err != nil {
				return nil, err
			}
			if err := validateSkill(rendered.Bytes(), name); err != nil {
				return nil, fmt.Errorf("skill %s: %w", id, err)
			}
			artifacts = append(artifacts, Artifact{Root: root, Path: path.Join(".agents/skills", name, "SKILL.md"), Strategy: "whole-file", Content: rendered.Bytes()})
			continue
		}
		content, err := fs.ReadFile(source, component.Source)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, Artifact{Root: root, Path: path.Join(".dtkit/content", p.ID, component.Source), Strategy: "whole-file", Content: content})
	}
	resourcePath := ".dtkit/content/engineer"
	if scope == "global" {
		resourcePath = "$HOME/.dtkit/content/engineer"
	}
	preference := ""
	if scope == "project" {
		preference = " Prefer dtkit-project skills for this project when both scopes are installed."
	}
	block := fmt.Sprintf("<!-- dtkit:begin engineer %s -->\n# DTKit engineering guidance\n\nInspect relevant project instructions and code before changes. Respect the user's scope, preserve unrelated work, and report actual verification results. Use the dtkit-%s skills when appropriate.%s For relevant tasks, consult workflows, rules and templates under %s.\n<!-- dtkit:end engineer %s -->\n", scope, scope, preference, resourcePath, scope)
	instructionRoot := root
	if scope == "global" {
		instructionRoot = "codex-home"
	}
	artifacts = append(artifacts, Artifact{Root: instructionRoot, Path: "AGENTS.md", Strategy: "managed-block", Content: []byte(block)})
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Root != artifacts[j].Root {
			return artifacts[i].Root < artifacts[j].Root
		}
		return artifacts[i].Path < artifacts[j].Path
	})
	return artifacts, nil
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
