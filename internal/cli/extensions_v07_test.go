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
		"backend/pom.xml":       "<project></project>\n",
		"backend/User.java":     "public class User {}\n",
		"frontend/package.json": `{"name":"fe","scripts":{"test":"echo ok"}}`,
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
	if err != nil || runEnv.CompanionRun == nil || !strings.Contains(runEnv.CompanionRun.Output, "backend/User.java") {
		t.Fatalf("integrate run repomix failed: %v (%s)", err, runRaw)
	}

	// 5. dtkit mcp status --all-runtimes and disable
	mcpStEnv, mcpStRaw, err := run("mcp", "status", "--all-runtimes", "--path", repo, "--json")
	if err != nil || len(mcpStEnv.MCPConfigs) != 5 {
		t.Fatalf("mcp status --all-runtimes failed: %v (%s)", err, mcpStRaw)
	}
	disEnv, disRaw, err := run("mcp", "disable", "--runtime", "claude", "--path", repo, "--yes", "--json")
	if err != nil || len(disEnv.MCPConfigs) != 1 || disEnv.MCPConfigs[0].Enabled {
		t.Fatalf("mcp disable failed: %v (%s)", err, disRaw)
	}
}
