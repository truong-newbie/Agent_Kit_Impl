package codex

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestRenderScopeAndDeterminism(t *testing.T) {
	p, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"project", "global"} {
		a, err := Render(p, source, scope)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Render(p, source, scope)
		if err != nil {
			t.Fatal(err)
		}
		if len(a) != 23 || len(a) != len(b) {
			t.Fatalf("unexpected artifacts: %d", len(a))
		}
		skillCount := 0
		instructionCount := 0
		for i := range a {
			if a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) {
				t.Fatal("non-deterministic output")
			}
			if a[i].Strategy == "managed-block" {
				instructionCount++
				if scope == "global" && a[i].Root != "codex-home" {
					t.Fatal("incorrect global instruction root")
				}
			}
			if bytes.Contains(a[i].Content, []byte("name: dtkit-"+scope+"-")) {
				skillCount++
			}
		}
		if skillCount != 11 || instructionCount != 1 {
			t.Fatalf("skills=%d blocks=%d", skillCount, instructionCount)
		}
	}
}

func TestSkillRejectsMalformedFrontmatter(t *testing.T) {
	for _, content := range []string{
		"---\nname: wrong\ndescription: hi\n---\nbody",
		"---\nname: dtkit-project-plan\nname: dtkit-project-plan\ndescription: hi\n---\nbody",
		"---\nname: dtkit-project-plan\ndescription: \n---\nbody",
		"---\nname: dtkit-project-plan\ndescription: hi\n---\n",
	} {
		if err := validateSkill([]byte(content), "dtkit-project-plan"); err == nil {
			t.Fatalf("accepted malformed skill: %q", content)
		}
	}
}

func TestRenderedSkillsContentLinksAndSizeLimits(t *testing.T) {
	p, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	refPattern := regexp.MustCompile(`(?:\$HOME/)?(\.dtkit/content/[a-zA-Z0-9._/-]+)`)
	for _, scope := range []string{"project", "global"} {
		artifacts, err := Render(p, source, scope)
		if err != nil {
			t.Fatal(err)
		}
		artifactPaths := map[string]bool{}
		for _, a := range artifacts {
			artifactPaths[a.Path] = true
		}
		otherScope := "global"
		if scope == "global" {
			otherScope = "project"
		}
		for _, a := range artifacts {
			if a.Strategy == "managed-block" {
				if len(a.Content) >= 4096 {
					t.Fatalf("%s managed block exceeds 4 KiB (%d bytes)", scope, len(a.Content))
				}
				continue
			}
			if strings.HasPrefix(a.Path, ".agents/skills/") {
				text := string(a.Content)
				if strings.Contains(text, "{{") || strings.Contains(text, "}}") {
					t.Fatalf("%s contains unrendered template markers", a.Path)
				}
				if strings.Contains(text, "dtkit-"+otherScope+"-") {
					t.Fatalf("%s references wrong scope %s", a.Path, otherScope)
				}
				lines := strings.Count(text, "\n") + 1
				if lines >= 250 {
					t.Fatalf("%s has %d lines (expected < 250)", a.Path, lines)
				}
				for _, sec := range []string{"## When to use", "## When not to use", "## Steps", "## Constraints", "## Examples"} {
					if !strings.Contains(text, sec) {
						t.Fatalf("%s missing required section %q", a.Path, sec)
					}
				}
				for _, match := range refPattern.FindAllStringSubmatch(text, -1) {
					rel := match[1]
					if !artifactPaths[rel] {
						t.Fatalf("%s references missing resource %q", a.Path, rel)
					}
				}
			}
		}
	}
}
