package dist

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestImportSuperpowersAndClaudeConfigRepo(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "superpowers-src")
	outParent := filepath.Join(base, "out")

	files := map[string]string{
		"skills/tdd/SKILL.md":           "---\nname: tdd\ndescription: Enforce strict Red-Green-Refactor TDD.\n---\n# Strict TDD\nWrite failing test first.\n",
		"skills/brainstorming/SKILL.md": "# Socratic Brainstorming\nAsk clarifying questions before coding.\n",
		".claude/commands/cook.md":      "Execute implementation plan phase by phase with {{custom_var}}.\n",
		".claude/workflows/dev.md":      "# Dev Workflow\n1. Plan\n2. Cook\n",
		"CLAUDE.md":                     "# Core Rules\n- Always run tests.\n",
		".env":                          "SECRET=must-be-ignored\n",
	}
	for rel, body := range files {
		full := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Dry-run writes nothing
	dryRes, err := ImportKit(ImportOptions{
		SourceDir:       src,
		TargetParentDir: outParent,
		KitID:           "superpowers",
		DryRun:          true,
	})
	if err != nil {
		t.Fatalf("dry-run ImportKit failed: %v", err)
	}
	if _, statErr := os.Stat(dryRes.OutputDir); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run should not write output dir %s", dryRes.OutputDir)
	}

	// 2. Real import writes valid package that loads and renders across all 5 runtimes
	res, err := ImportKit(ImportOptions{
		SourceDir:       src,
		TargetParentDir: outParent,
		KitID:           "superpowers",
		Name:            "Superpowers Community Kit",
		Version:         "0.2.0",
	})
	if err != nil {
		t.Fatalf("ImportKit failed: %v", err)
	}
	pkg, fsys, digest, err := catalog.LoadDir(res.OutputDir)
	if err != nil {
		t.Fatalf("catalog.LoadDir on imported kit failed: %v", err)
	}
	if pkg.ID != "superpowers" || pkg.Version != "0.2.0" || digest != res.PackageDigest {
		t.Fatalf("unexpected imported package: pkg=%+v digest=%s want=%s", pkg, digest, res.PackageDigest)
	}
	for _, rt := range adapter.SupportedRuntimes() {
		arts, rErr := adapter.RenderForRuntime(rt, pkg, fsys, "project")
		if rErr != nil || len(arts) == 0 {
			t.Fatalf("RenderForRuntime(%s) failed: %v", rt, rErr)
		}
	}
}

func TestImportCursorRulesOnlySynthesizesBridgeSkill(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "cursor-src")
	outParent := filepath.Join(base, "out")

	mdc := "---\ndescription: React & Next.js rules\nglobs: **/*.tsx\nalwaysApply: true\n---\n# Next.js Rules\n- Prefer Server Components.\n"
	full := filepath.Join(src, ".cursor", "rules", "nextjs.mdc")
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(mdc), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := ImportKit(ImportOptions{
		SourceDir:       src,
		TargetParentDir: outParent,
		KitID:           "cursor-pack",
	})
	if err != nil {
		t.Fatalf("ImportKit(.mdc) failed: %v", err)
	}
	if res.DetectedFormat != "cursor-rules" {
		t.Fatalf("expected cursor-rules format, got %q", res.DetectedFormat)
	}
	ruleBytes, err := os.ReadFile(filepath.Join(res.OutputDir, "rules", "nextjs.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ruleBytes), "alwaysApply:") || !strings.Contains(string(ruleBytes), "Prefer Server Components") {
		t.Fatalf("expected frontmatter stripped from .mdc rule, got:\n%s", string(ruleBytes))
	}
}

func TestImportGitHubShorthandHTTPSValidationAndUpdate(t *testing.T) {
	base := t.TempDir()
	outParent := filepath.Join(base, "out")

	// Reject insecure non-loopback HTTP git URL
	if _, err := ImportKit(ImportOptions{
		SourceDir:       "http://example.com/obra/superpowers.git",
		TargetParentDir: outParent,
		KitID:           "superpowers",
	}); apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected insecure HTTP git URL rejection, got %v", err)
	}

	// Reject embedded credentials in git URL
	if _, err := ImportKit(ImportOptions{
		SourceDir:       "https://user:token@github.com/obra/superpowers.git",
		TargetParentDir: outParent,
		KitID:           "superpowers",
	}); apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected credentialed git URL rejection, got %v", err)
	}

	// Mock remote git fetch for GitHub shorthand owner/repo + verify --update atomic replace
	versionNum := "0.1.0"
	skillBody := "# Remote Superpowers\nInitial skill body.\n"
	fakeFetch := func(_ context.Context, cloneURL, destDir string) error {
		if cloneURL != "https://github.com/obra/superpowers.git" {
			t.Fatalf("unexpected expanded cloneURL %q", cloneURL)
		}
		target := filepath.Join(destDir, "skills", "tdd", "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, []byte(skillBody), 0644)
	}

	res1, err := ImportKit(ImportOptions{
		SourceDir:       "obra/superpowers",
		TargetParentDir: outParent,
		KitID:           "superpowers",
		Version:         versionNum,
		FetchGitRepo:    fakeFetch,
	})
	if err != nil {
		t.Fatalf("GitHub shorthand import failed: %v", err)
	}
	if res1.SourceURL != "https://github.com/obra/superpowers.git" || res1.Updated {
		t.Fatalf("unexpected res1: %+v", res1)
	}

	// Without --update, second import fails target_exists
	if _, dupErr := ImportKit(ImportOptions{
		SourceDir:       "obra/superpowers",
		TargetParentDir: outParent,
		KitID:           "superpowers",
		FetchGitRepo:    fakeFetch,
	}); apperr.ExitCodeOf(dupErr) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict without --update, got %v", dupErr)
	}

	// With Update: true, replaces atomically with bumped version and new digest
	skillBody = "# Remote Superpowers\nUpdated skill body v2.\n"
	res2, err := ImportKit(ImportOptions{
		SourceDir:       "obra/superpowers",
		TargetParentDir: outParent,
		KitID:           "superpowers",
		Version:         "0.2.0",
		Update:          true,
		FetchGitRepo:    fakeFetch,
	})
	if err != nil {
		t.Fatalf("GitHub shorthand --update failed: %v", err)
	}
	if !res2.Updated || res2.PackageDigest == res1.PackageDigest || res2.Version != "0.2.0" {
		t.Fatalf("expected updated digest & version, got %+v", res2)
	}
	rawSkill, err := os.ReadFile(filepath.Join(res2.OutputDir, "skills", "tdd", "SKILL.md.tmpl"))
	if err != nil || !strings.Contains(string(rawSkill), "Updated skill body v2") {
		t.Fatalf("expected updated skill body on disk, got err=%v body=%s", err, string(rawSkill))
	}
}
