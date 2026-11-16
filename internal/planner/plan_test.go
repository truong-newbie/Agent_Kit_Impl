package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
	"dtkit.local/dtkit/internal/state"
)

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestPlanDesiredDecisionMatrix(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}

	// 1. Fresh desired -> create.
	p, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(p, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanDesired("project", "engineer", roots, nil, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Conflicts != 0 || len(plan.Actions) != 15 {
		t.Fatalf("unexpected fresh plan: %+v", plan)
	}

	// 2. Installed matching disk -> all noop.
	existing := &state.Installation{
		SchemaVersion:   1,
		ManagedBy:       "dtkit",
		Generation:      1,
		Scope:           "project",
		Runtime:         "codex",
		AdapterRevision: "1",
		InstalledAt:     "2026-11-16T00:00:00Z",
		UpdatedAt:       "2026-11-16T00:00:00Z",
		Kit:             state.Kit{ID: "engineer", Version: "0.1.0", Digest: shaHex([]byte("k"))},
	}
	for _, a := range artifacts {
		target := filepath.Join(root, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		data := a.Content
		var blk *BlockState
		dig := shaHex(data)
		if a.Strategy == "managed-block" {
			var bs BlockState
			var err error
			data, bs, err = AppendBlock([]byte("# User header\r\n"), true, a.Content, "project")
			if err != nil {
				t.Fatal(err)
			}
			blk = &bs
			dig = bs.Digest
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			t.Fatal(err)
		}
		existing.Artifacts = append(existing.Artifacts, state.Artifact{
			Root: a.Root, Path: a.Path, Strategy: a.Strategy, Digest: dig, Block: blk,
		})
	}

	noopPlan, err := PlanDesired("project", "engineer", roots, existing, artifacts)
	if err != nil || noopPlan.Conflicts != 0 {
		t.Fatalf("expected clean noop plan, got %+v err=%v", noopPlan, err)
	}
	for _, act := range noopPlan.Actions {
		if act.Kind != "noop" {
			t.Fatalf("expected noop, got %+v", act)
		}
	}

	// 3. Removing an artifact from desired while disk matches state -> remove.
	fewer := artifacts[:len(artifacts)-1]
	removePlan, err := PlanDesired("project", "engineer", roots, existing, fewer)
	if err != nil || removePlan.Conflicts != 0 {
		t.Fatalf("expected clean removal plan, got %+v err=%v", removePlan, err)
	}
	removedCount := 0
	for _, act := range removePlan.Actions {
		if act.Kind == "remove" {
			removedCount++
		}
	}
	if removedCount != 1 {
		t.Fatalf("expected 1 remove action, got %d", removedCount)
	}

	// 4. Modifying managed file on disk -> conflict.
	firstSkill := filepath.Join(root, filepath.FromSlash(artifacts[0].Path))
	if err := os.WriteFile(firstSkill, []byte("user tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	conflictPlan, err := PlanDesired("project", "engineer", roots, existing, artifacts)
	if err != nil || conflictPlan.Conflicts != 1 {
		t.Fatalf("expected 1 conflict, got %+v err=%v", conflictPlan, err)
	}
}
