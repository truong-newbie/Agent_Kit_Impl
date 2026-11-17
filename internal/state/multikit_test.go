package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func testDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func sampleInstallation(kitID, runtime, skillPath string) Installation {
	dig := testDigest(kitID + ":" + skillPath)
	blockDig := testDigest("block:" + kitID)
	instrPath := "AGENTS.md"
	if runtime == "claude" {
		instrPath = "CLAUDE.md"
	}
	return Installation{
		SchemaVersion:   1,
		ManagedBy:       "dtkit",
		Generation:      1,
		Scope:           "project",
		Runtime:         runtime,
		AdapterRevision: "1",
		Kit: Kit{
			ID:         kitID,
			Version:    "0.1.0",
			Digest:     testDigest("pkg:" + kitID),
			SourceKind: "local",
		},
		Artifacts: []Artifact{
			{
				Root:     "project",
				Path:     instrPath,
				Strategy: "managed-block",
				Digest:   blockDig,
				Block:    &BlockState{Digest: blockDig, Separator: "\n", CreatedFile: false},
			},
			{
				Root:     "project",
				Path:     skillPath,
				Strategy: "whole-file",
				Digest:   dig,
			},
		},
		InstalledAt: "2026-11-17T12:00:00Z",
		UpdatedAt:   "2026-11-17T12:00:00Z",
	}
}

func TestRegistryV3ValidationDisjointOwnershipAndRoundTrip(t *testing.T) {
	instA := sampleInstallation("engineer", "claude", ".claude/skills/dtkit-claude-project-review/SKILL.md")
	instB := sampleInstallation("security-review", "claude", ".claude/skills/dtkit-claude-project-security-review-review/SKILL.md")

	reg := Registry{
		SchemaVersion:   3,
		ManagedBy:       "dtkit",
		Scope:           "project",
		Runtime:         "claude",
		AdapterRevision: "1",
		Kits: map[string]Installation{
			"engineer":        instA,
			"security-review": instB,
		},
		UpdatedAt: "2026-11-17T12:05:00Z",
	}

	raw, err := reg.MarshalSnapshot("tx-123")
	if err != nil {
		t.Fatalf("MarshalSnapshot(v3) failed: %v", err)
	}
	if !strings.Contains(string(raw), `"schemaVersion": 3`) {
		t.Fatalf("expected schemaVersion 3 JSON:\n%s", string(raw))
	}

	parsed, err := ParseSnapshot(raw, "project")
	if err != nil {
		t.Fatalf("ParseSnapshot(v3) failed: %v", err)
	}
	if len(parsed.Kits) != 2 {
		t.Fatalf("expected 2 kits in parsed registry, got %d", len(parsed.Kits))
	}
	peers := parsed.PeerOwnedWholeFiles("security-review")
	if owner := peers["project\x00.claude/skills/dtkit-claude-project-review/skill.md"]; owner != "engineer" {
		t.Fatalf("expected engineer to own whole-file skill, got %q (%+v)", owner, peers)
	}
	// Shared managed-block file CLAUDE.md must NOT block peer kits.
	if _, sharedBlocked := peers["project\x00claude.md"]; sharedBlocked {
		t.Fatalf("managed-block CLAUDE.md should not be treated as exclusive whole-file")
	}

	// Reject colliding whole-file path owned by two kits in v3 registry.
	colliding := instB
	colliding.Artifacts[1].Path = instA.Artifacts[1].Path
	regDup := reg
	regDup.Kits = map[string]Installation{"engineer": instA, "security-review": colliding}
	rawDup, _ := json.Marshal(regDup)
	if _, err := ParseSnapshot(rawDup, "project"); err == nil || !strings.Contains(err.Error(), "claimed by multiple kits") {
		t.Fatalf("expected cross-kit ownership collision rejection, got %v", err)
	}

	// Demoting back to 1 kit serializes cleanly as v1/v2 Installation.
	delete(parsed.Kits, "engineer")
	demoted, err := parsed.MarshalSnapshot("tx-124")
	if err != nil {
		t.Fatalf("demote MarshalSnapshot failed: %v", err)
	}
	if !strings.Contains(string(demoted), `"schemaVersion": 1`) {
		t.Fatalf("expected schemaVersion 1 on single remaining kit:\n%s", string(demoted))
	}
}
