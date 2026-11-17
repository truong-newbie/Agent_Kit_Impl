package planner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
)

func TestComputePlanDiffManagedBlockPrivacyAndBounds(t *testing.T) {
	proj := t.TempDir()
	roots := map[string]string{"project": proj}

	// Existing AGENTS.md holds private user instructions and another kit's block.
	agentsPath := filepath.Join(proj, "AGENTS.md")
	seedBlock := "<!-- dtkit:begin engineer project -->\nOld DTKit rule line\n<!-- dtkit:end engineer project -->\n"
	peerBlock := "<!-- dtkit:begin security-review project -->\nPeer Kit Secret Rule\n<!-- dtkit:end security-review project -->\n"
	userText := "# TOP SECRET USER NOTES\nAPI_TOKEN_PRIVATE=xyz\n\n"
	if err := os.WriteFile(agentsPath, []byte(userText+seedBlock+peerBlock), 0644); err != nil {
		t.Fatal(err)
	}

	// Unowned skill file on disk already exists -> must NOT leak its contents.
	unownedRel := ".agents/skills/dtkit-project-ask/SKILL.md"
	unownedAbs := filepath.Join(proj, filepath.FromSlash(unownedRel))
	if err := os.MkdirAll(filepath.Dir(unownedAbs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unownedAbs, []byte("PRIVATE USER SKILL DO NOT LEAK"), 0644); err != nil {
		t.Fatal(err)
	}

	desired := []adapter.Artifact{
		{
			Root:        "project",
			Path:        "AGENTS.md",
			Strategy:    "managed-block",
			ComponentID: "instructions",
			Content:     []byte("<!-- dtkit:begin engineer project -->\nNew DTKit rule line\n<!-- dtkit:end engineer project -->\n"),
		},
		{
			Root:        "project",
			Path:        unownedRel,
			Strategy:    "whole-file",
			ComponentID: "skill/ask",
			Content:     []byte("---\nname: dtkit-project-ask\ndescription: Ask\n---\n"),
		},
		{
			Root:        "project",
			Path:        ".agents/skills/dtkit-project-plan/SKILL.md",
			Strategy:    "whole-file",
			ComponentID: "skill/plan",
			Content:     []byte(strings.Repeat("diff_line_payload_0123456789\n", 1000)), // ~29 KiB -> triggers MaxFileDiffBytes (16 KiB)
		},
		{
			Root:        "project",
			Path:        ".dtkit/content/engineer/bin.dat",
			Strategy:    "whole-file",
			ComponentID: "rule/engineering",
			Content:     []byte{0x00, 0x01, 0xff},
		},
	}

	plan := InstallPlan{
		Conflicts: 1,
		Actions: []Action{
			{Root: "project", Path: ".agents/skills/dtkit-project-plan/SKILL.md", Kind: "create", Reason: "target_missing"},
			{Root: "project", Path: unownedRel, Kind: "conflict", Reason: "unowned_file"},
			{Root: "project", Path: ".dtkit/content/engineer/bin.dat", Kind: "create", Reason: "target_missing"},
			{Root: "project", Path: "AGENTS.md", Kind: "update", Reason: "content_changed"},
		},
	}

	diff, err := ComputePlanDiff("codex", "project", "engineer", roots, plan, desired, false)
	if err != nil {
		t.Fatalf("ComputePlanDiff failed: %v", err)
	}
	if !diff.Truncated || diff.TotalBytes > MaxTotalDiffBytes {
		t.Fatalf("expected bounded truncated diff, got total=%d truncated=%v", diff.TotalBytes, diff.Truncated)
	}
	for _, ch := range diff.Changes {
		if strings.Contains(ch.Patch, "TOP SECRET USER NOTES") || strings.Contains(ch.Patch, "Peer Kit Secret Rule") || strings.Contains(ch.Patch, "PRIVATE USER SKILL DO NOT LEAK") {
			t.Fatalf("privacy leak in diff for %s:\n%s", ch.Path, ch.Patch)
		}
		if ch.Path == "AGENTS.md" {
			if !strings.Contains(ch.Patch, "-Old DTKit rule line") || !strings.Contains(ch.Patch, "+New DTKit rule line") {
				t.Fatalf("expected managed-block diff lines for AGENTS.md:\n%s", ch.Patch)
			}
		}
		if ch.Path == unownedRel && ch.Patch != "" {
			t.Fatalf("expected empty patch for unowned conflict file, got %q", ch.Patch)
		}
		if ch.Path == ".dtkit/content/engineer/bin.dat" && (!ch.Binary || ch.Patch != "") {
			t.Fatalf("expected binary flag without raw patch for binary file, got %+v", ch)
		}
	}
}
