package adapter

import (
	"testing"
)

func TestBuildAndDiffCompatibilityReports(t *testing.T) {
	rep, err := BuildCompatibilityReport()
	if err != nil {
		t.Fatalf("BuildCompatibilityReport failed: %v", err)
	}
	if rep.SchemaVersion != 1 || len(rep.Entries) != 10 {
		t.Fatalf("expected 10 (5 runtimes x 2 scopes) entries, got %d", len(rep.Entries))
	}

	// Self-diff produces zero changes.
	self := DiffCompatibilityReports(rep, rep)
	if self.ChangedCount != 0 || self.HasBreakingChanges {
		t.Fatalf("expected clean zero self-diff, got %+v", self)
	}

	// Mutating a skill prefix triggers breaking change classification cleanly.
	mod := rep
	mod.Entries = append([]CompatibilityEntry(nil), rep.Entries...)
	mod.Entries[0].SkillPrefix = ".agents/skills/renamed-"
	diff := DiffCompatibilityReports(rep, mod)
	if diff.ChangedCount != 1 || !diff.HasBreakingChanges || !diff.Changes[0].Breaking {
		t.Fatalf("expected 1 breaking change, got %+v", diff)
	}
}
