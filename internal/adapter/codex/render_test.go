package codex

import (
	"bytes"
	"testing"

	"dtkit.local/dtkit/internal/catalog"
)

func TestRenderScopeAndDeterminism(t *testing.T) {
	p, source, err := catalog.Engineer(); if err != nil { t.Fatal(err) }
	for _, scope := range []string{"project", "global"} {
		a, err := Render(p, source, scope); if err != nil { t.Fatal(err) }
		b, err := Render(p, source, scope); if err != nil { t.Fatal(err) }
		if len(a) != 15 || len(a) != len(b) { t.Fatalf("unexpected artifacts: %d", len(a)) }
		skillCount := 0; instructionCount := 0
		for i := range a {
			if a[i].Path != b[i].Path || !bytes.Equal(a[i].Content, b[i].Content) { t.Fatal("non-deterministic output") }
			if a[i].Strategy == "managed-block" { instructionCount++; if scope == "global" && a[i].Root != "codex-home" { t.Fatal("incorrect global instruction root") } }
			if bytes.Contains(a[i].Content, []byte("name: dtkit-"+scope+"-")) { skillCount++ }
		}
		if skillCount != 7 || instructionCount != 1 { t.Fatalf("skills=%d blocks=%d", skillCount, instructionCount) }
	}
}
