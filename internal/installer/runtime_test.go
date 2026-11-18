package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/gemini"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestGeminiInstallCoexistenceRollbackUpdateAndRemove(t *testing.T) {
	base := t.TempDir()
	proj := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(home, ".codex")
	geminiHome := filepath.Join(home, ".gemini")
	for _, d := range []string{proj, home} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	userGeminiMD := "# My Repo Gemini Rules\nKeep edits small.\n"
	if err := os.WriteFile(filepath.Join(proj, "GEMINI.md"), []byte(userGeminiMD), 0644); err != nil {
		t.Fatal(err)
	}

	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	codexArts, err := codex.Render(pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}
	geminiArts, err := gemini.Render(pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	projRoots := map[string]string{"project": proj}

	// 1. Install Codex first in project.
	if _, err := FreshInstall("project", projRoots, pkg, src, codexArts, now); err != nil {
		t.Fatalf("codex install failed: %v", err)
	}

	// 2. Fault injection during Gemini install rolls back cleanly without affecting Codex.
	boom := errors.New("simulated crash during gemini install")
	_, err = FreshInstallWithOptions("project", projRoots, pkg, src, geminiArts, now, InstallOptions{
		Runtime: "gemini",
		Hooks: &TxHooks{
			BeforeStateCommit: func() error { return boom },
		},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected injected error, got %v", err)
	}
	gemMDBytes, err := os.ReadFile(filepath.Join(proj, "GEMINI.md"))
	if err != nil || string(gemMDBytes) != userGeminiMD {
		t.Fatalf("GEMINI.md not rolled back cleanly: %q err=%v", string(gemMDBytes), err)
	}

	// Clear committed/rolled-back check & install Gemini for real.
	gemState, err := FreshInstallWithOptions("project", projRoots, pkg, src, geminiArts, now, InstallOptions{Runtime: "gemini"})
	if err != nil {
		t.Fatalf("gemini install failed: %v", err)
	}
	if gemState.Runtime != "gemini" || len(gemState.Artifacts) != 22 {
		t.Fatalf("unexpected gemini state: %+v", gemState)
	}

	// Check unknown personal file inside Gemini skill dir is reported and preserved on remove.
	personalNote := filepath.Join(proj, ".gemini", "skills", "dtkit-gemini-project-plan", "notes.md")
	if err := os.WriteFile(personalNote, []byte("personal"), 0644); err != nil {
		t.Fatal(err)
	}
	rep, err := InspectRuntime("gemini", "project", projRoots, geminiArts)
	if err != nil || !rep.Healthy || len(rep.UnknownFiles) != 1 {
		t.Fatalf("unexpected gemini status: %+v err=%v", rep, err)
	}

	// Global Codex + Gemini coexistence too.
	gCodexArts, err := codex.Render(pkg, src, "global")
	if err != nil {
		t.Fatal(err)
	}
	gGeminiArts, err := gemini.Render(pkg, src, "global")
	if err != nil {
		t.Fatal(err)
	}
	codexGlobalRoots := map[string]string{"user-home": home, "codex-home": codexHome}
	geminiGlobalRoots := map[string]string{"user-home": home, "gemini-home": geminiHome}
	if _, err := FreshInstall("global", codexGlobalRoots, pkg, src, gCodexArts, now); err != nil {
		t.Fatalf("global codex install: %v", err)
	}
	if _, err := FreshInstallWithOptions("global", geminiGlobalRoots, pkg, src, gGeminiArts, now, InstallOptions{Runtime: "gemini"}); err != nil {
		t.Fatalf("global gemini install: %v", err)
	}

	// Profile mismatch detection when GEMINI_CLI_HOME moves.
	mismatchRep, err := InspectRuntime("gemini", "global", map[string]string{"user-home": home, "gemini-home": filepath.Join(base, "other-gemini")}, nil)
	if err != nil || !mismatchRep.ProfileMismatch || mismatchRep.Classification != "profile_mismatch" {
		t.Fatalf("expected gemini profile_mismatch, got %+v err=%v", mismatchRep, err)
	}

	// Update Gemini to v0.3.0 and verify Codex stays untouched.
	v2FS := fstest.MapFS{
		"rules/engineering.md": &fstest.MapFile{Data: []byte("# Updated Gemini Rule\n")},
		"skills/plan/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: dtkit-gemini-project-plan\ndescription: Updated gemini planner.\n---\nBody\n")},
	}
	v2Pkg := catalog.Package{
		SchemaVersion:        1,
		ID:                   "engineer",
		Name:                 "Engineer",
		Version:              "0.9.0",
		Description:          "v9",
		RuntimeTargets:       []string{"gemini"},
		Scopes:               []string{"project"},
		RequiresCapabilities: []string{"instructions", "skills"},
		PackageDependencies:  []string{},
		Components: []catalog.Component{
			{ID: "rule/engineering", Kind: "rule", Source: "rules/engineering.md"},
			{ID: "skill/plan", Kind: "skill", Source: "skills/plan", Requires: []string{"rule/engineering"}},
		},
	}
	v2Arts, err := gemini.Render(v2Pkg, v2FS, "project")
	if err != nil {
		t.Fatal(err)
	}
	upRes, err := Update("project", projRoots, v2Pkg, v2FS, v2Arts, now.Add(time.Minute), UpdateOptions{Runtime: "gemini"})
	if err != nil || !upRes.Updated {
		t.Fatalf("gemini update failed: %+v err=%v", upRes, err)
	}

	// Remove Gemini project install -> Codex still healthy, personal GEMINI.md + notes.md preserved!
	rmRes, err := Remove("project", "engineer", projRoots, now.Add(2*time.Minute), RemoveOptions{Runtime: "gemini"})
	if err != nil || !rmRes.Removed {
		t.Fatalf("gemini remove failed: %+v err=%v", rmRes, err)
	}
	afterGemMD, err := os.ReadFile(filepath.Join(proj, "GEMINI.md"))
	if err != nil || !strings.Contains(string(afterGemMD), "Keep edits small.") || strings.Contains(string(afterGemMD), "dtkit:begin") {
		t.Fatalf("GEMINI.md not cleaned properly after remove: %q err=%v", string(afterGemMD), err)
	}
	if _, err := os.Stat(personalNote); err != nil {
		t.Fatalf("personal note inside gemini skill dir deleted unexpectedly: %v", err)
	}
	codexRep, err := InspectRuntime("codex", "project", projRoots, codexArts)
	if err != nil || !codexRep.Installed || !codexRep.Healthy {
		t.Fatalf("codex state damaged by gemini remove: %+v err=%v", codexRep, err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".dtkit", "runtimes", "gemini", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected gemini state.json removed")
	}
	if code := apperr.ExitCodeOf(nil); code != 0 {
		t.Fatal(code)
	}
}
