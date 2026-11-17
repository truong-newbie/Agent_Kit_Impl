package adapter_test

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestEngineerKitEvaluationInvariantsAcrossRuntimesAndScopes(t *testing.T) {
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatalf("catalog.Engineer() failed: %v", err)
	}

	resourceRefRe := regexp.MustCompile(`[^\s` + "`" + `()]+\.(?:md)`)
	expectedSkills := []string{"ask", "debug", "explain", "implement", "plan", "review", "test"}

	for _, rt := range adapter.SupportedRuntimes() {
		for _, scope := range []string{"project", "global"} {
			t.Run(rt+"_"+scope, func(t *testing.T) {
				profile, ok := adapter.Profile(rt)
				if !ok {
					t.Fatalf("missing profile for %s", rt)
				}
				artifacts, err := adapter.RenderKit(profile, pkg, src, scope)
				if err != nil {
					t.Fatalf("RenderKit(%s, %s) failed: %v", rt, scope, err)
				}

				emittedPaths := make(map[string]bool, len(artifacts))
				var instructionBytes int
				var skillBytes int
				renderedSkills := make(map[string]string)

				for _, a := range artifacts {
					body := string(a.Content)
					if strings.Contains(body, "{{") || strings.Contains(body, "}}") || strings.Contains(body, "<no value>") {
						t.Fatalf("unrendered template artifact %s (%s/%s)", a.Path, rt, scope)
					}
					if a.Strategy == "managed-block" || strings.HasSuffix(a.Path, ".mdc") {
						instructionBytes += len(a.Content)
					}
					if strings.HasSuffix(a.Path, "/SKILL.md") {
						skillBytes += len(a.Content)
						for _, sk := range expectedSkills {
							if strings.Contains(a.Path, "-"+sk+"/SKILL.md") {
								renderedSkills[sk] = body
							}
						}
					}
					emittedPaths[a.Path] = true
				}

				if len(renderedSkills) != len(expectedSkills) {
					t.Fatalf("expected %d rendered skills, got %d (%v)", len(expectedSkills), len(renderedSkills), renderedSkills)
				}

				// Progressive disclosure invariant: always-loaded instructions stay compact (< 8 KiB) and smaller than on-demand skill payloads.
				if instructionBytes > 8192 {
					t.Fatalf("always-loaded instruction footprint %d bytes exceeds progressive disclosure budget (8192)", instructionBytes)
				}
				if skillBytes <= instructionBytes {
					t.Fatalf("expected on-demand skill payload (%d B) > always-loaded instructions (%d B)", skillBytes, instructionBytes)
				}

				// Check every content/engineer/*.md reference resolves to a real emitted resource artifact.
				for skName, skBody := range renderedSkills {
					norm := strings.ReplaceAll(skBody, "\r\n", "\n")
					if !strings.HasPrefix(norm, "---\nname: ") {
						t.Fatalf("skill %s missing YAML frontmatter name", skName)
					}
					for _, m := range resourceRefRe.FindAllString(skBody, -1) {
						if !strings.Contains(m, "content/engineer/") {
							continue
						}
						idx := strings.Index(m, "content/engineer/")
						suffix := m[idx:]
						matched := false
						for ep := range emittedPaths {
							if strings.HasSuffix(ep, "/"+suffix) || strings.HasSuffix(ep, suffix) {
								matched = true
								break
							}
						}
						if !matched {
							t.Fatalf("skill %s references dangling resource %q (suffix %q) not emitted for %s/%s", skName, m, suffix, rt, scope)
						}
					}
				}

				// Read-only / non-fabrication contract checks on critical skills.
				for _, roSkill := range []string{"plan", "review", "explain"} {
					b := renderedSkills[roSkill]
					if !strings.Contains(b, "Do not") {
						t.Fatalf("read-only skill %s missing explicit edit guardrail", roSkill)
					}
				}
				if !strings.Contains(renderedSkills["implement"], "Never fabricate") ||
					!strings.Contains(renderedSkills["test"], "Never claim tests passed without running them") {
					t.Fatalf("implement/test skills missing non-fabrication guardrail")
				}
				if bytes.Contains([]byte(renderedSkills["debug"]), []byte("Never suppress errors")) == false {
					t.Fatalf("debug skill missing test integrity guardrail")
				}
				// Verify repository-neutrality (no DTKit-internal examples inside engineer skill bodies).
				for _, leak := range []string{"internal/apperr", "AppendBlock", "ReplaceBlock", "RemoveBlock", "AGENTS.md", "Docker"} {
					for skName, skBody := range renderedSkills {
						if strings.Contains(skBody, leak) {
							t.Fatalf("skill %s still leaks repo-specific example %q", skName, leak)
						}
					}
				}
				if !strings.Contains(renderedSkills["test"], ".dtkit/project.json") {
					t.Fatalf("test skill missing .dtkit/project.json awareness")
				}
			})
		}
	}
}
