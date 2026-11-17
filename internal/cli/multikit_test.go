package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func TestCLIMultiKitInstallStatusDoctorUpdateAndRemove(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	claudeHome := filepath.Join(base, "claude-home")
	secDirV1 := filepath.Join(base, "sec-v1")
	secDirV2 := filepath.Join(base, "sec-v2")
	for _, d := range []string{proj, home, claudeHome, filepath.Join(secDirV1, "rules"), filepath.Join(secDirV1, "skills", "audit"), filepath.Join(secDirV2, "rules"), filepath.Join(secDirV2, "skills", "audit")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	env := selectionTestEnv(base, proj, home, claudeHome)

	writeSecKit := func(dir, ver string) {
		pkgJSON := `{"schemaVersion":1,"id":"sec-kit","name":"Sec Kit","version":"` + ver + `","description":"Security checks","runtimeTargets":["claude","codex"],"scopes":["project","global"],"requiresCapabilities":["skills"],"optionalCapabilities":["instructions"],"packageDependencies":[],"components":[{"id":"rule/sec","kind":"rule","source":"rules/sec.md","requires":[]},{"id":"skill/audit","kind":"skill","source":"skills/audit","requires":["rule/sec"]}]}`
		_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkgJSON), 0644)
		_ = os.WriteFile(filepath.Join(dir, "rules", "sec.md"), []byte("Security v"+ver+"\n"), 0644)
		_ = os.WriteFile(filepath.Join(dir, "skills", "audit", "SKILL.md.tmpl"), []byte("---\nname: {{.SkillName}}\ndescription: Sec audit\n---\n# Sec Audit "+ver+"\n"), 0644)
	}
	writeSecKit(secDirV1, "0.1.0")
	writeSecKit(secDirV2, "0.2.0")

	if _, _, _, err := runSelectionCLI(t, env, "install", "engineer", "--project", "--runtime", "claude", "--preset", "minimal", "--yes", "--json"); err != nil {
		t.Fatalf("install engineer failed: %v", err)
	}
	if _, _, _, err := runSelectionCLI(t, env, "install", "sec-kit", "--project", "--runtime", "claude", "--from", secDirV1, "--yes", "--json"); err != nil {
		t.Fatalf("install sec-kit failed: %v", err)
	}

	statusEnv, _, _, err := runSelectionCLI(t, env, "status", "--project", "--runtime", "claude", "--json")
	if err != nil {
		t.Fatalf("status multi-kit failed: %v", err)
	}
	if len(statusEnv.Installations) != 2 || !statusEnv.Installations[0].Healthy || !statusEnv.Installations[1].Healthy {
		t.Fatalf("expected 2 healthy kits in status, got %+v", statusEnv.Installations)
	}

	docEnv, _, _, err := runSelectionCLI(t, env, "doctor", "--project", "--runtime", "claude", "--json")
	if err != nil {
		t.Fatalf("doctor multi-kit failed: %v", err)
	}
	if !docEnv.Ok {
		t.Fatalf("expected doctor ok=true, got %+v", docEnv)
	}

	if _, _, _, err := runSelectionCLI(t, env, "update", "sec-kit", "--project", "--runtime", "claude", "--from", secDirV2, "--yes", "--json"); err != nil {
		t.Fatalf("update sec-kit failed: %v", err)
	}
	statusAfter, _, _, err := runSelectionCLI(t, env, "status", "--project", "--runtime", "claude", "--json")
	if err != nil || len(statusAfter.Installations) != 2 {
		t.Fatalf("status after sec-kit update failed: %v %+v", err, statusAfter)
	}
	if statusAfter.Installations[0].Selection == nil || statusAfter.Installations[0].Selection.Preset != "minimal" {
		t.Fatalf("expected engineer minimal preset preserved, got %+v", statusAfter.Installations[0])
	}
	if statusAfter.Installations[1].KitID != "sec-kit" || statusAfter.Installations[1].KitVersion != "0.2.0" {
		t.Fatalf("expected sec-kit updated to 0.2.0, got %+v", statusAfter.Installations[1])
	}

	if _, _, _, err := runSelectionCLI(t, env, "remove", "engineer", "--project", "--runtime", "claude", "--yes", "--json"); err != nil {
		t.Fatalf("remove engineer failed: %v", err)
	}
	statusOne, _, _, err := runSelectionCLI(t, env, "status", "--project", "--runtime", "claude", "--json")
	if err != nil || len(statusOne.Installations) != 1 || statusOne.Installations[0].KitID != "sec-kit" {
		t.Fatalf("expected only sec-kit remaining after engineer remove, got err=%v (%+v)", err, statusOne.Installations)
	}

	// Removing unknown kit fails cleanly without touching sec-kit.
	_, _, _, mismatchErr := runSelectionCLI(t, env, "remove", "other-kit", "--project", "--runtime", "claude", "--yes")
	if mismatchErr == nil || apperr.ExitCodeOf(mismatchErr) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict removing non-installed kit, got %v", mismatchErr)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-sec-kit-audit", "SKILL.md")); err != nil {
		t.Fatalf("expected sec-kit skill intact: %v", err)
	}
	_ = strings.Contains
}
