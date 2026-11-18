package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestCLIInitIntegrateImportCompanionAndMCPConfig(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")
	extSrc := filepath.Join(base, "superpowers-repo")
	stableBinDir := filepath.Join(home, "bin")
	stableBin := filepath.Join(stableBinDir, "dtkit")

	if err := os.MkdirAll(stableBinDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stableBin, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	repoFiles := map[string]string{
		"backend/pom.xml": "<project></project>\n",
		"backend/src/main/java/com/acme/Token.java":     "package com.acme;\npublic class Token {}\n",
		"backend/src/main/java/com/acme/User.java":      "package com.acme;\nimport com.acme.Token;\npublic class User {}\n",
		"backend/src/test/java/com/acme/TokenTest.java": "package com.acme;\npublic class TokenTest {}\n",
		"frontend/package.json":                         `{"name":"fe","scripts":{"test":"echo ok"}}`,
	}
	for rel, body := range repoFiles {
		full := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	extFiles := map[string]string{
		"skills/tdd/SKILL.md":     "---\nname: tdd\ndescription: Red-Green-Refactor TDD skill.\n---\n# TDD\nWrite failing test first.\n",
		".cursor/rules/style.mdc": "---\ndescription: Style rule\nalwaysApply: true\n---\n# Style\n- Keep functions short.\n",
	}
	for rel, body := range extFiles {
		full := filepath.Join(extSrc, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}
	run := func(args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(strings.NewReader(""), &out, &errBuf)
		deps.Env = env
		deps.Executable = func() (string, error) { return stableBin, nil }
		err := RunWithDeps(context.Background(), args, deps)
		var parsed Envelope
		_ = json.Unmarshal(out.Bytes(), &parsed)
		return parsed, out.String() + errBuf.String(), err
	}

	// 1. dtkit init --dry-run writes nothing
	dryEnv, dryRaw, err := run("init", "--path", repo, "--runtime", "claude", "--with-mcp", "--with-hooks", "--lock", "--dry-run", "--json")
	if err != nil || dryEnv.InitSummary == nil || !dryEnv.DryRun {
		t.Fatalf("dtkit init --dry-run failed: %v (%s)", err, dryRaw)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "docs", "codebase-summary.md")); !os.IsNotExist(statErr) {
		t.Fatal("dtkit init --dry-run should not write docs/codebase-summary.md")
	}

	// 2. dtkit init --runtime claude --with-mcp --with-hooks --lock --yes
	initEnv, initRaw, err := run("init", "--path", repo, "--runtime", "claude", "--with-mcp", "--with-hooks", "--lock", "--yes", "--json")
	if err != nil || initEnv.InitSummary == nil {
		t.Fatalf("dtkit init failed: %v (%s)", err, initRaw)
	}
	for _, rel := range []string{
		"docs/codebase-summary.md",
		".dtkit/project.json",
		".mcp.json",
		".claude/settings.json",
		"dtkit.lock",
	} {
		if _, statErr := os.Stat(filepath.Join(repo, filepath.FromSlash(rel))); statErr != nil {
			t.Fatalf("expected %s created by dtkit init: %v", rel, statErr)
		}
	}

	// Re-running dtkit init is idempotent
	if _, againRaw, againErr := run("init", "--path", repo, "--runtime", "claude", "--with-mcp", "--with-hooks", "--lock", "--yes", "--json"); againErr != nil {
		t.Fatalf("second dtkit init should be idempotent: %v (%s)", againErr, againRaw)
	}

	// 3. dtkit integrate import --install coexists with engineer@0.5.0 via Multi-Kit v3
	impEnv, impRaw, err := run("integrate", "import", extSrc, "--as", "superpowers", "--install", "--runtime", "claude", "--path", repo, "--yes", "--json")
	if err != nil || impEnv.Import == nil || impEnv.Import.KitID != "superpowers" {
		t.Fatalf("integrate import --install failed: %v (%s)", err, impRaw)
	}
	stEnv, stRaw, err := run("status", "--project", "--runtime", "claude", "--path", repo, "--json")
	if err != nil || len(stEnv.Installations) != 2 {
		t.Fatalf("expected 2 coexisting kits (engineer + superpowers), got %d: %v (%s)", len(stEnv.Installations), err, stRaw)
	}

	// 4. dtkit integrate list, tool, and run (fallback)
	listEnv, listRaw, err := run("integrate", "list", "--path", repo, "--json")
	if err != nil || len(listEnv.Companions) != 4 {
		t.Fatalf("integrate list failed: %v (%s)", err, listRaw)
	}
	if _, toolRaw, tErr := run("integrate", "tool", "ast-grep", "--path", repo, "--yes", "--json"); tErr != nil {
		t.Fatalf("integrate tool ast-grep failed: %v (%s)", tErr, toolRaw)
	}
	runEnv, runRaw, err := run("integrate", "run", "repomix", "--path", repo, "--zone", "backend", "--json")
	if err != nil || runEnv.CompanionRun == nil || !strings.Contains(runEnv.CompanionRun.Output, "backend/src/main/java/com/acme/User.java") {
		t.Fatalf("integrate run repomix failed: %v (%s)", err, runRaw)
	}

	// 5. dtkit mcp status --all-runtimes, codex .codex/config.toml enable/disable, and scout --impact
	mcpStEnv, mcpStRaw, err := run("mcp", "status", "--all-runtimes", "--path", repo, "--json")
	if err != nil || len(mcpStEnv.MCPConfigs) != 5 {
		t.Fatalf("mcp status --all-runtimes failed: %v (%s)", err, mcpStRaw)
	}
	codexEnEnv, codexEnRaw, err := run("mcp", "enable", "--runtime", "codex", "--path", repo, "--yes", "--json")
	if err != nil || len(codexEnEnv.MCPConfigs) != 1 || codexEnEnv.MCPConfigs[0].ConfigPath != ".codex/config.toml" || !codexEnEnv.MCPConfigs[0].Enabled {
		t.Fatalf("mcp enable codex config.toml failed: %v (%s)", err, codexEnRaw)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".codex", "config.toml")); statErr != nil {
		t.Fatalf("expected .codex/config.toml created: %v", statErr)
	}
	if _, codexDisRaw, err := run("mcp", "disable", "--runtime", "codex", "--path", repo, "--yes", "--json"); err != nil {
		t.Fatalf("mcp disable codex failed: %v (%s)", err, codexDisRaw)
	}
	disEnv, disRaw, err := run("mcp", "disable", "--runtime", "claude", "--path", repo, "--yes", "--json")
	if err != nil || len(disEnv.MCPConfigs) != 1 || disEnv.MCPConfigs[0].Enabled {
		t.Fatalf("mcp disable failed: %v (%s)", err, disRaw)
	}

	impGraphEnv, impGraphRaw, err := run("scout", "--path", repo, "--impact", "backend/src/main/java/com/acme/Token.java", "--json")
	if err != nil || impGraphEnv.ScoutImpact == nil || len(impGraphEnv.ScoutImpact.DirectDependents) == 0 || len(impGraphEnv.ScoutImpact.RelatedTests) == 0 {
		t.Fatalf("dtkit scout --impact failed: %v (%s)", err, impGraphRaw)
	}

	// 6. Test integrate import --update --install auto patch bump + unified dtkit update / dtkit refresh
	skillFile := filepath.Join(extSrc, "skills", "tdd", "SKILL.md")
	if wErr := os.WriteFile(skillFile, []byte("# Strict TDD v2\nUpdated guidance.\n"), 0644); wErr != nil {
		t.Fatal(wErr)
	}
	impUpEnv, impUpRaw, err := run("integrate", "import", extSrc, "--as", "superpowers", "--update", "--install", "--runtime", "claude", "--path", repo, "--yes", "--json")
	if err != nil || impUpEnv.Import == nil || impUpEnv.Import.Version != "0.1.1" {
		t.Fatalf("integrate import --update --install without --version failed: %v (%s)", err, impUpRaw)
	}

	// Grow repo with a new Python AI zone so dtkit update refreshes summary + project.json + kits + dtkit.lock
	aiPkg := filepath.Join(repo, "ai", "pyproject.toml")
	aiSrc := filepath.Join(repo, "ai", "src", "embed.py")
	if err := os.MkdirAll(filepath.Dir(aiSrc), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aiPkg, []byte("[project]\nname = \"ai\"\nversion = \"0.1.0\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(aiSrc, []byte("def embed(x): return [len(x)]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	updDryEnv, updDryRaw, err := run("update", "--path", repo, "--dry-run", "--json")
	if err != nil || updDryEnv.UpdateSummary == nil || !updDryEnv.UpdateSummary.CodebaseSummary || !updDryEnv.UpdateSummary.ProjectProfile {
		t.Fatalf("dtkit update --dry-run failed: %v (%s)", err, updDryRaw)
	}
	updApplyEnv, updApplyRaw, err := run("update", "--path", repo, "--yes", "--json")
	if err != nil || updApplyEnv.UpdateSummary == nil || !updApplyEnv.UpdateSummary.CodebaseSummary || !updApplyEnv.UpdateSummary.ProjectProfile {
		t.Fatalf("dtkit update --yes failed: %v (%s)", err, updApplyRaw)
	}
	if sumBytes, rErr := os.ReadFile(filepath.Join(repo, "docs", "codebase-summary.md")); rErr != nil || !strings.Contains(string(sumBytes), "python") {
		t.Fatalf("expected updated codebase-summary.md to mention python/ai, err=%v", rErr)
	}

	refEnv, refRaw, err := run("refresh", "--path", repo, "--yes", "--json")
	if err != nil || refEnv.UpdateSummary == nil || refEnv.UpdateSummary.CodebaseSummary || refEnv.UpdateSummary.ProjectProfile {
		t.Fatalf("expected idempotent dtkit refresh no-op, got err=%v (%s)", err, refRaw)
	}
}
