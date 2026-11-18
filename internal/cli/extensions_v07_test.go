package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
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

func TestSpecKitLearningsConvergeOverridesAndAgyAlias(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/app\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "implementation-plan.md"), []byte("# Plan\n- [ ] Create `pkg/todo.go`\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tasks.md"), []byte("# Tasks\n- [x] Init\n"), 0644); err != nil {
		t.Fatal(err)
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}
	run := func(args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(strings.NewReader(""), &out, &errBuf)
		deps.Env = env
		err := RunWithDeps(context.Background(), args, deps)
		var parsed Envelope
		_ = json.Unmarshal(out.Bytes(), &parsed)
		return parsed, out.String() + errBuf.String(), err
	}

	// 1. S01: Install using agy runtime alias -> AGENTS.md + .agents/skills/dtkit-project-converge
	instEnv, instRaw, err := run("install", "engineer", "--project", "--runtime", "agy", "--path", repo, "--yes", "--json")
	if err != nil || instEnv.Runtime != "codex" || instEnv.Kit == nil || instEnv.Kit.Version != "0.7.0" {
		t.Fatalf("install via agy alias failed: %v (%s)", err, instRaw)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".agents", "skills", "dtkit-project-converge", "SKILL.md")); statErr != nil {
		t.Fatalf("expected engineer@0.7.0 converge skill rendered: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".dtkit", "content", "engineer", "templates", "feature-spec.md")); statErr != nil {
		t.Fatalf("expected feature-spec.md template rendered: %v", statErr)
	}

	// 2. S05: Add project-local template override without forking -> dtkit update applies override & status stays healthy
	overridePath := filepath.Join(repo, ".dtkit", "overrides", "engineer", "templates", "implementation-plan.md")
	if err := os.MkdirAll(filepath.Dir(overridePath), 0755); err != nil {
		t.Fatal(err)
	}
	customPlanTmpl := "# Custom Company Implementation Plan\nMust include rollback checklist.\n"
	if err := os.WriteFile(overridePath, []byte(customPlanTmpl), 0644); err != nil {
		t.Fatal(err)
	}
	updEnv, updRaw, err := run("update", "--path", repo, "--yes", "--json")
	if err != nil || updEnv.UpdateSummary == nil || len(updEnv.UpdateSummary.Kits) != 1 || updEnv.UpdateSummary.Kits[0].Status != "updated" {
		t.Fatalf("expected project update to apply local override, got %v (%s)", err, updRaw)
	}
	renderedTmpl, rErr := os.ReadFile(filepath.Join(repo, ".dtkit", "content", "engineer", "templates", "implementation-plan.md"))
	if rErr != nil || !strings.Contains(string(renderedTmpl), "Custom Company Implementation Plan") {
		t.Fatalf("expected overridden template content on disk, err=%v body=%s", rErr, string(renderedTmpl))
	}
	stEnv, stRaw, err := run("status", "--project", "--runtime", "codex", "--path", repo, "--json")
	if err != nil || len(stEnv.Installations) != 1 || !stEnv.Installations[0].Healthy {
		t.Fatalf("expected overridden project kit status healthy, got %v (%s)", err, stRaw)
	}

	// 3. S03: dtkit converge --append-tasks --yes appends unmet item
	convEnv, convRaw, err := run("converge", "--path", repo, "--plan", "implementation-plan.md", "--append-tasks", "tasks.md", "--yes", "--json")
	if err != nil || convEnv.Convergence == nil || convEnv.Convergence.Status != "incomplete" || len(convEnv.Convergence.MissingFiles) != 1 {
		t.Fatalf("dtkit converge failed: %v (%s)", err, convRaw)
	}
	tasksBytes, _ := os.ReadFile(filepath.Join(repo, "tasks.md"))
	if !strings.Contains(string(tasksBytes), "## Phase Convergence") || !strings.Contains(string(tasksBytes), "pkg/todo.go") {
		t.Fatalf("expected appended convergence phase in tasks.md:\n%s", string(tasksBytes))
	}

	// 4. Feature Spec CLI: dtkit spec init + dtkit spec check + dtkit converge --spec
	specInitEnv, specInitRaw, err := run("spec", "init", "todo-export", "--title", "Todo Export", "--path", repo, "--yes", "--json")
	if err != nil || specInitEnv.SpecReport == nil || specInitEnv.SpecReport.SpecPath != "docs/specs/todo-export.md" {
		t.Fatalf("dtkit spec init failed: %v (%s)", err, specInitRaw)
	}
	specCheckEnv, specCheckRaw, err := run("spec", "check", "docs/specs/todo-export.md", "--path", repo, "--json")
	if apperr.ExitCodeOf(err) != apperr.ExitConflict || specCheckEnv.SpecReport == nil || specCheckEnv.SpecReport.Status != "needs_clarification" {
		t.Fatalf("expected initial template spec to flag needs_clarification (exit 3), got err=%v raw=%s", err, specCheckRaw)
	}
	resolvedSpec := "# Feature Specification: Todo Export\n\n### User Story 1 — CSV Export (Priority: P1)\n- **Given** todos exist, **When** exporting CSV, **Then** download valid CSV.\n\n- **FR-001:** Export CSV.\n- **SC-001:** Under 1s.\n"
	if wErr := os.WriteFile(filepath.Join(repo, "docs", "specs", "todo-export.md"), []byte(resolvedSpec), 0644); wErr != nil {
		t.Fatal(wErr)
	}
	specValidEnv, specValidRaw, err := run("spec", "check", "docs/specs/todo-export.md", "--path", repo, "--json")
	if err != nil || specValidEnv.SpecReport == nil || specValidEnv.SpecReport.Status != "valid" {
		t.Fatalf("expected valid spec check, got err=%v raw=%s", err, specValidRaw)
	}
	convSpecEnv, convSpecRaw, err := run("converge", "--path", repo, "--plan", "implementation-plan.md", "--spec", "docs/specs/todo-export.md", "--dry-run", "--json")
	if err != nil || convSpecEnv.Convergence == nil || convSpecEnv.Convergence.SpecPath != "docs/specs/todo-export.md" || len(convSpecEnv.Convergence.UncoveredRequirements) != 1 {
		t.Fatalf("expected converge --spec to surface uncovered FR-001, got err=%v raw=%s", err, convSpecRaw)
	}
}
