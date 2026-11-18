package gemini

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestRenderGeminiScopesAndDeterminism(t *testing.T) {
	p, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	p.RuntimeTargets = []string{"codex", "gemini"}

	refPattern := regexp.MustCompile(`(?:\$HOME/)?(\.dtkit/runtimes/gemini/content/[a-zA-Z0-9._/-]+)`)
	for _, scope := range []string{"project", "global"} {
		a, err := Render(p, source, scope)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Render(p, source, scope)
		if err != nil {
			t.Fatal(err)
		}
		if len(a) != 22 || len(a) != len(b) {
			t.Fatalf("%s: expected 22 artifacts, got %d", scope, len(a))
		}
		pathsMap := map[string]adapter.Artifact{}
		skillCount := 0
		instrCount := 0
		for i := range a {
			if a[i].Root != b[i].Root || a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) {
				t.Fatalf("%s: non-deterministic output at %d", scope, i)
			}
			pathsMap[a[i].Path] = a[i]
			if strings.Contains(a[i].Path, ".agents/skills") || a[i].Path == "AGENTS.md" || strings.HasPrefix(a[i].Path, ".dtkit/content/") {
				t.Fatalf("%s: Codex artifact path leaked into Gemini render: %s:%s", scope, a[i].Root, a[i].Path)
			}
			if a[i].Strategy == "managed-block" {
				instrCount++
				wantRoot := "project"
				if scope == "global" {
					wantRoot = "gemini-home"
				}
				if a[i].Root != wantRoot || a[i].Path != "GEMINI.md" {
					t.Fatalf("%s: unexpected instruction target %s:%s", scope, a[i].Root, a[i].Path)
				}
				if len(a[i].Content) >= 4096 {
					t.Fatalf("%s: managed block exceeds 4 KiB", scope)
				}
				if !bytes.Contains(a[i].Content, []byte("dtkit-gemini-"+scope)) {
					t.Fatalf("%s: GEMINI.md block missing dtkit-gemini-%s hint", scope, scope)
				}
			}
			if bytes.Contains(a[i].Content, []byte("name: dtkit-gemini-"+scope+"-")) {
				skillCount++
				if scope == "project" && (a[i].Root != "project" || !strings.HasPrefix(a[i].Path, ".gemini/skills/dtkit-gemini-project-")) {
					t.Fatalf("unexpected project skill target %s:%s", a[i].Root, a[i].Path)
				}
				if scope == "global" && (a[i].Root != "gemini-home" || !strings.HasPrefix(a[i].Path, "skills/dtkit-gemini-global-")) {
					t.Fatalf("unexpected global skill target %s:%s", a[i].Root, a[i].Path)
				}
			}
		}
		if skillCount != 11 || instrCount != 1 {
			t.Fatalf("%s: skills=%d instr=%d", scope, skillCount, instrCount)
		}

		// Verify all resource references inside skills resolve to emitted Gemini resources.
		for _, art := range a {
			if strings.Contains(string(art.Content), "{{") || strings.Contains(string(art.Content), "}}") {
				t.Fatalf("%s: unrendered template marker in %s", scope, art.Path)
			}
			if art.Strategy != "whole-file" {
				continue
			}
			for _, m := range refPattern.FindAllStringSubmatch(string(art.Content), -1) {
				if _, ok := pathsMap[m[1]]; !ok {
					t.Fatalf("%s: dangling resource reference %q in %s", scope, m[1], art.Path)
				}
			}
		}
	}
}

func TestInspectSettingsContextOverride(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Missing settings file is benign.
	over, warn, err := InspectSettingsContextOverride(settingsPath)
	if err != nil || over || warn != "" {
		t.Fatalf("unexpected missing result: over=%v warn=%q err=%v", over, warn, err)
	}

	// Array including GEMINI.md does not override it.
	if err := os.WriteFile(settingsPath, []byte(`{"context":{"fileName":["AGENTS.md","GEMINI.md"]}}`), 0644); err != nil {
		t.Fatal(err)
	}
	over, _, err = InspectSettingsContextOverride(settingsPath)
	if err != nil || over {
		t.Fatalf("array containing GEMINI.md should not flag override")
	}

	// Custom fileName excluding GEMINI.md warns cleanly.
	if err := os.WriteFile(settingsPath, []byte(`{"context":{"fileName":"CUSTOM.md"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	over, warn, err = InspectSettingsContextOverride(settingsPath)
	if err != nil || !over || !strings.Contains(warn, "CUSTOM.md") {
		t.Fatalf("expected override warning, got over=%v warn=%q err=%v", over, warn, err)
	}
}
