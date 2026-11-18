package cursor

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestRenderCursorScopesAndCapabilityFallback(t *testing.T) {
	p, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	p.RuntimeTargets = []string{"codex", "gemini", "opencode", "cursor"}

	refPattern := regexp.MustCompile(`(?:\$HOME/)?(\.dtkit/runtimes/cursor/content/[a-zA-Z0-9._/-]+)`)

	// Project scope emits 22 artifacts including .cursor/rules/dtkit-engineer.mdc (alwaysApply: true).
	projArts, err := Render(p, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	if len(projArts) != 22 {
		t.Fatalf("project: expected 22 artifacts, got %d", len(projArts))
	}
	sawRule, skillCount := false, 0
	pathsMap := map[string]adapter.Artifact{}
	for _, a := range projArts {
		pathsMap[a.Path] = a
		if strings.Contains(a.Path, "AGENTS.md") || strings.Contains(a.Path, "GEMINI.md") || strings.Contains(a.Path, ".agents/skills") {
			t.Fatalf("leaked non-cursor path: %s:%s", a.Root, a.Path)
		}
		if a.Path == ".cursor/rules/dtkit-engineer.mdc" {
			sawRule = true
			if a.Root != "project" || a.Strategy != "whole-file" || !bytes.Contains(a.Content, []byte("alwaysApply: true")) {
				t.Fatalf("unexpected project rule artifact: %+v", a)
			}
		}
		if strings.HasPrefix(a.Path, ".cursor/skills/dtkit-cursor-project-") {
			skillCount++
		}
	}
	if !sawRule || skillCount != 11 {
		t.Fatalf("project rule=%v skills=%d", sawRule, skillCount)
	}
	for _, a := range projArts {
		for _, m := range refPattern.FindAllStringSubmatch(string(a.Content), -1) {
			if strings.HasSuffix(m[1], ".") {
				continue
			}
			if _, ok := pathsMap[m[1]]; !ok {
				t.Fatalf("project: dangling ref %q in %s", m[1], a.Path)
			}
		}
	}

	// Global scope has no file-based User Rules instruction artifact (21 artifacts: 11 skills + 10 resources).
	globArts, err := Render(p, source, "global")
	if err != nil {
		t.Fatal(err)
	}
	if len(globArts) != 21 {
		t.Fatalf("global: expected 21 artifacts (skills+resources, no on-disk global instruction file), got %d", len(globArts))
	}
	globSkills := 0
	for _, a := range globArts {
		if a.ComponentID == "instructions" || strings.HasSuffix(a.Path, ".mdc") {
			t.Fatalf("global cursor should not invent an on-disk global instruction file: %s", a.Path)
		}
		if a.Root != "user-home" {
			t.Fatalf("expected user-home root in cursor global, got %s:%s", a.Root, a.Path)
		}
		if strings.HasPrefix(a.Path, ".cursor/skills/dtkit-cursor-global-") {
			globSkills++
		}
	}
	if globSkills != 11 {
		t.Fatalf("global: expected 11 skills, got %d", globSkills)
	}
}
