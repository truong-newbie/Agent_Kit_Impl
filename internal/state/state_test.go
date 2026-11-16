package state

import (
	"strings"
	"testing"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
	"dtkit.local/dtkit/internal/planner"
)

func TestStateRejectsUnknownAndDuplicateArtifacts(t *testing.T) {
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	s := Installation{SchemaVersion: 1, ManagedBy: "dtkit", Generation: 1, Scope: "project", Runtime: "codex", AdapterRevision: "1", InstalledAt: "2026-11-16T00:00:00Z", UpdatedAt: "2026-11-16T00:00:00Z", Kit: Kit{ID: "engineer", Version: "0.1.0", Digest: digest}}
	for _, a := range artifacts {
		record := Artifact{Root: a.Root, Path: a.Path, Strategy: a.Strategy, Digest: digest}
		if a.Strategy == "managed-block" {
			record.Block = &planner.BlockState{Digest: digest}
		}
		s.Artifacts = append(s.Artifacts, record)
	}
	if err := s.Validate(artifacts); err != nil {
		t.Fatal(err)
	}
	bad := s
	bad.Artifacts = append([]Artifact{}, s.Artifacts...)
	bad.Artifacts[0].Path = "private.txt"
	if err := bad.Validate(artifacts); err == nil {
		t.Fatal("accepted arbitrary path")
	}
	bad.Artifacts[0] = bad.Artifacts[1]
	if err := bad.Validate(artifacts); err == nil {
		t.Fatal("accepted duplicate path")
	}
	bad = s
	bad.Artifacts = append([]Artifact{}, s.Artifacts...)
	bad.Artifacts[0].Digest = "invalid"
	if err := bad.Validate(artifacts); err == nil {
		t.Fatal("accepted invalid digest")
	}
	bad = s
	bad.Scope = "global"
	bad.CodexHome = t.TempDir()
	if err := bad.Validate(artifacts); err == nil {
		t.Fatal("accepted project artifacts in global state")
	}
	bad = s
	bad.SchemaVersion = 2
	if err := bad.Validate(artifacts); err == nil {
		t.Fatal("accepted unsupported schema")
	}
}
