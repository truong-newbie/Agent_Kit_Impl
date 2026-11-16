package state

import (
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
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
			record.Block = &BlockState{Digest: digest}
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
	bad.SchemaVersion = 3
	if err := bad.Validate(artifacts); err == nil {
		t.Fatal("accepted unsupported schema")
	}
	bad = s
	bad.SchemaVersion = 1
	bad.Hooks = []HookRegistration{{
		ID: "session-context", Event: "SessionStart", HandlerID: "session-context",
		Root: "project", Path: ".codex/hooks.json",
		Command: "/usr/local/bin/dtkit hook run session-context", CommandWindows: "& 'C:\\dtkit\\dtkit.exe' 'hook' 'run' 'session-context'",
		Executable: t.TempDir(), EntryDigest: digest, TimeoutSec: 3,
	}}
	if err := bad.Validate(artifacts); err == nil {
		t.Fatal("accepted hooks in schemaVersion 1")
	}
	goodV2 := bad
	goodV2.SchemaVersion = 2
	if err := goodV2.Validate(artifacts); err != nil {
		t.Fatalf("rejected valid schemaVersion 2 with hooks: %v", err)
	}
}
