package opencode

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

func TestRenderOpenCodeScopesAndDeterminism(t *testing.T) {
	p, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	p.RuntimeTargets = []string{"codex", "gemini", "opencode"}

	refPattern := regexp.MustCompile(`(?:\$HOME/)?(\.dtkit/runtimes/opencode/content/[a-zA-Z0-9._/-]+)`)
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
			if strings.Contains(a[i].Path, ".agents/skills") || strings.Contains(a[i].Path, ".gemini") || strings.HasPrefix(a[i].Path, ".dtkit/content/") {
				t.Fatalf("%s: sibling runtime path leaked into opencode render: %s:%s", scope, a[i].Root, a[i].Path)
			}
			if a[i].Strategy == "managed-block" {
				instrCount++
				wantRoot := "project"
				if scope == "global" {
					wantRoot = "opencode-home"
				}
				if a[i].Root != wantRoot || a[i].Path != "AGENTS.md" {
					t.Fatalf("%s: unexpected instruction target %s:%s", scope, a[i].Root, a[i].Path)
				}
				wantMarker := "<!-- dtkit:begin engineer opencode " + scope + " -->"
				if !bytes.Contains(a[i].Content, []byte(wantMarker)) {
					t.Fatalf("%s: missing runtime-namespaced marker %q in %q", scope, wantMarker, string(a[i].Content))
				}
			}
			if bytes.Contains(a[i].Content, []byte("name: dtkit-opencode-"+scope+"-")) {
				skillCount++
				if scope == "project" && (a[i].Root != "project" || !strings.HasPrefix(a[i].Path, ".opencode/skills/dtkit-opencode-project-")) {
					t.Fatalf("unexpected project skill target %s:%s", a[i].Root, a[i].Path)
				}
				if scope == "global" && (a[i].Root != "opencode-home" || !strings.HasPrefix(a[i].Path, "skills/dtkit-opencode-global-")) {
					t.Fatalf("unexpected global skill target %s:%s", a[i].Root, a[i].Path)
				}
			}
		}
		if skillCount != 11 || instrCount != 1 {
			t.Fatalf("%s: skills=%d instr=%d", scope, skillCount, instrCount)
		}
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

func TestInspectConfigWarnings(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	invalid := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(valid, []byte(`{"instructions":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invalid, []byte(`{bad`), 0644); err != nil {
		t.Fatal(err)
	}
	warns := InspectConfigWarnings([]string{filepath.Join(dir, "missing.json"), valid, invalid})
	if len(warns) != 1 || !strings.Contains(warns[0], "invalid JSON") {
		t.Fatalf("expected 1 invalid JSON warning, got %v", warns)
	}
}
