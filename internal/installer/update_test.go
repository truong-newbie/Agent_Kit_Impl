package installer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/apperr"
	"dtkit.local/dtkit/internal/catalog"
)

func fixtureKit(version, ruleBody string, includeExtraSkill, includeAskSkill bool) (catalog.Package, fstest.MapFS) {
	files := fstest.MapFS{
		"rules/engineering.md":      &fstest.MapFile{Data: []byte(ruleBody)},
		"skills/plan/SKILL.md.tmpl": &fstest.MapFile{Data: []byte("---\nname: {{.SkillName}}\ndescription: Plan task\n---\n# Plan\n")},
	}
	comps := []catalog.Component{
		{ID: "rule/engineering", Kind: "rule", Source: "rules/engineering.md", Requires: []string{}},
		{ID: "skill/plan", Kind: "skill", Source: "skills/plan", Requires: []string{"rule/engineering"}},
	}
	if includeAskSkill {
		files["skills/ask/SKILL.md.tmpl"] = &fstest.MapFile{Data: []byte("---\nname: {{.SkillName}}\ndescription: Ask clarifying question\n---\n# Ask\n")}
		comps = append(comps, catalog.Component{ID: "skill/ask", Kind: "skill", Source: "skills/ask", Requires: []string{"rule/engineering"}})
	}
	if includeExtraSkill {
		files["skills/audit/SKILL.md.tmpl"] = &fstest.MapFile{Data: []byte("---\nname: {{.SkillName}}\ndescription: Audit code\n---\n# Audit\n")}
		comps = append(comps, catalog.Component{ID: "skill/audit", Kind: "skill", Source: "skills/audit", Requires: []string{"rule/engineering"}})
	}
	pkg := catalog.Package{
		SchemaVersion:        1,
		ID:                   "engineer",
		Name:                 "DTKit Engineer Kit",
		Version:              version,
		Description:          "Fixture kit",
		RuntimeTargets:       []string{"codex"},
		Scopes:               []string{"project", "global"},
		RequiresCapabilities: []string{"instructions", "skills"},
		PackageDependencies:  []string{},
		Components:           comps,
	}
	return pkg, files
}

func TestUpdateLifecycleMigrationImmutableAndDowngrade(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("\xef\xbb\xbf# Custom Rules\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Install v0.1.0 (has skill/plan + skill/ask).
	pkgV1, fsV1 := fixtureKit("0.1.0", "# Rules v1\n", false, true)
	artV1, err := codex.Render(pkgV1, fsV1, "project")
	if err != nil {
		t.Fatal(err)
	}
	t1 := time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC)
	if _, err := FreshInstall("project", roots, pkgV1, fsV1, artV1, t1); err != nil {
		t.Fatal(err)
	}
	// Append personal suffix outside managed block.
	curAgents, _ := os.ReadFile(agents)
	if err := os.WriteFile(agents, append(curAgents, []byte("# User Footer\r\n")...), 0644); err != nil {
		t.Fatal(err)
	}

	// Same version with different digest must fail as immutable_version.
	pkgV1Bad, fsV1Bad := fixtureKit("0.1.0", "# Rules v1 modified\n", false, true)
	artV1Bad, _ := codex.Render(pkgV1Bad, fsV1Bad, "project")
	_, err = Update("project", roots, pkgV1Bad, fsV1Bad, artV1Bad, t1, UpdateOptions{})
	if appErr := apperr.AsError(err); err == nil || appErr.ExitCode != apperr.ExitConflict || appErr.Code != "immutable_version" {
		t.Fatalf("expected immutable_version conflict, got %v", err)
	}

	// Update v0.1.0 -> v0.2.0: modifies rule, adds skill/audit, removes skill/ask.
	pkgV2, fsV2 := fixtureKit("0.2.0", "# Rules v2\n", true, false)
	artV2, err := codex.Render(pkgV2, fsV2, "project")
	if err != nil {
		t.Fatal(err)
	}
	t2 := time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC)
	upRes, err := Update("project", roots, pkgV2, fsV2, artV2, t2, UpdateOptions{SourceKind: "local"})
	if err != nil || !upRes.Updated || upRes.Installation.Generation != 2 {
		t.Fatalf("update failed: %+v err=%v", upRes, err)
	}
	if upRes.Installation.InstalledAt != t1.UTC().Format(time.RFC3339) {
		t.Fatalf("installedAt changed on update: %s", upRes.Installation.InstalledAt)
	}
	if upRes.Installation.UpdatedAt != t2.UTC().Format(time.RFC3339) {
		t.Fatalf("updatedAt not updated: %s", upRes.Installation.UpdatedAt)
	}
	// Check added skill exists, removed skill disappeared, and AGENTS.md user prefix/suffix preserved.
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "dtkit-project-audit", "SKILL.md")); err != nil {
		t.Fatalf("new skill missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "dtkit-project-ask")); !os.IsNotExist(err) {
		t.Fatalf("removed skill directory survived: %v", err)
	}
	agentsAfter, _ := os.ReadFile(agents)
	if !bytes.HasPrefix(agentsAfter, []byte("\xef\xbb\xbf# Custom Rules\r\n")) || !bytes.HasSuffix(agentsAfter, []byte("# User Footer\r\n")) {
		t.Fatalf("user bytes in AGENTS.md lost during update: %q", agentsAfter)
	}

	// Repeat update to v0.2.0 is a clean noop (generation stays 2).
	t3 := time.Date(2026, 11, 16, 3, 0, 0, 0, time.UTC)
	noopRes, err := Update("project", roots, pkgV2, fsV2, artV2, t3, UpdateOptions{})
	if err != nil || !noopRes.Noop || noopRes.Installation.Generation != 2 {
		t.Fatalf("expected noop update, got %+v err=%v", noopRes, err)
	}

	// Downgrade v0.2.0 -> v0.1.0 is rejected.
	if _, err := Update("project", roots, pkgV1, fsV1, artV1, t3, UpdateOptions{}); apperr.ExitCodeOf(err) != apperr.ExitConflict || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("expected downgrade rejection, got %v", err)
	}
}

func TestUpdateDriftConflictAndFaultInjectionRollback(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	pkgV1, fsV1 := fixtureKit("0.1.0", "# Rules v1\n", false, true)
	artV1, _ := codex.Render(pkgV1, fsV1, "project")
	if _, err := FreshInstall("project", roots, pkgV1, fsV1, artV1, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Modify a managed file on disk -> update must fail with Conflict.
	rulePath := filepath.Join(root, ".dtkit", "content", "engineer", "rules", "engineering.md")
	origRule, _ := os.ReadFile(rulePath)
	_ = os.WriteFile(rulePath, []byte("# User tampered\n"), 0644)

	pkgV2, fsV2 := fixtureKit("0.2.0", "# Rules v2\n", true, false)
	artV2, _ := codex.Render(pkgV2, fsV2, "project")
	if _, err := Update("project", roots, pkgV2, fsV2, artV2, time.Now(), UpdateOptions{}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected conflict on drifted file during update, got %v", err)
	}

	// Restore original file, inject fault mid-update -> rolls back to v0.1.0 cleanly.
	_ = os.WriteFile(rulePath, origRule, 0644)
	_, err = Update("project", roots, pkgV2, fsV2, artV2, time.Now(), UpdateOptions{
		Hooks: &TxHooks{
			BeforeStateCommit: func() error { return errors.New("injected state commit failure") },
		},
	})
	if err == nil {
		t.Fatal("expected injected error")
	}
	ruleAfterRollback, _ := os.ReadFile(rulePath)
	if string(ruleAfterRollback) != "# Rules v1\n" {
		t.Fatalf("rule not rolled back: %q", ruleAfterRollback)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "dtkit-project-ask", "SKILL.md")); err != nil {
		t.Fatalf("removed skill not restored on rollback: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "dtkit-project-audit")); !os.IsNotExist(err) {
		t.Fatalf("new skill survived failed update rollback: %v", err)
	}
}

func TestPlanUpdateDetectsAdapterMigration(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	pkgV1, fsV1 := fixtureKit("0.1.0", "# Rules v1\n", false, true)
	artV1, _ := codex.Render(pkgV1, fsV1, "project")
	if _, err := FreshInstall("project", roots, pkgV1, fsV1, artV1, time.Now()); err != nil {
		t.Fatal(err)
	}
	dig, err := catalog.Digest(fsV1)
	if err != nil {
		t.Fatal(err)
	}
	_, _, migrated, err := PlanUpdate("project", roots, pkgV1, dig, artV1, "2")
	if err != nil || !migrated {
		t.Fatalf("expected adapter migration detection, got migrated=%v err=%v", migrated, err)
	}
}
