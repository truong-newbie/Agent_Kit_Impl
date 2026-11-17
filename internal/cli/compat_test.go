package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLICompatSnapshotAndDiff(t *testing.T) {
	snapPath := filepath.Join(t.TempDir(), "compat-baseline.json")
	var out bytes.Buffer
	if err := RunWithDeps(context.Background(), []string{"compat", "snapshot", "--out", snapPath, "--json"}, DefaultDeps(nil, &out, &bytes.Buffer{})); err != nil {
		t.Fatalf("compat snapshot failed: %v (%s)", err, out.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.CompatReport == nil || len(env.CompatReport.Entries) != 10 {
		t.Fatalf("expected 10 compat entries, got %+v", env.CompatReport)
	}

	// Self-diff produces zero changes.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"compat", "diff", "--baseline", snapPath, "--json"}, DefaultDeps(nil, &out, &bytes.Buffer{})); err != nil {
		t.Fatalf("compat diff failed: %v (%s)", err, out.String())
	}
	var diffEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &diffEnv); err != nil {
		t.Fatal(err)
	}
	if diffEnv.CompatDiff == nil || diffEnv.CompatDiff.ChangedCount != 0 {
		t.Fatalf("expected zero diff against fresh snapshot, got %+v", diffEnv.CompatDiff)
	}

	// Modify baseline skill prefix -> compat diff flags breaking change clearly.
	rawSnap, err := os.ReadFile(snapPath)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(string(rawSnap), "dtkit-project-", "renamed-prefix-", 1)
	if err := os.WriteFile(snapPath, []byte(mutated), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"compat", "diff", "--baseline", snapPath, "--json"}, DefaultDeps(nil, &out, &bytes.Buffer{})); err != nil {
		t.Fatalf("compat diff mutated failed: %v", err)
	}
	var mutEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &mutEnv); err != nil {
		t.Fatal(err)
	}
	if mutEnv.CompatDiff == nil || !mutEnv.CompatDiff.HasBreakingChanges || mutEnv.CompatDiff.ChangedCount != 1 {
		t.Fatalf("expected breaking change detection, got %+v", mutEnv.CompatDiff)
	}
}
