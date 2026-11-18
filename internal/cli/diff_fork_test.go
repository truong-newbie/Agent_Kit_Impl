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

func TestCLIDiffForkUpdateWorkflowUserEditsAndRedaction(t *testing.T) {
	home := filepath.Join(t.TempDir(), "alice-home")
	claudeHome := filepath.Join(home, ".claude")
	proj := filepath.Join(home, "workspace", "repo")
	forkParent := filepath.Join(home, "custom-kits")
	for _, d := range []string{home, claudeHome, proj, forkParent} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Personal instructions outside DTKit managed block must stay private & preserved.
	claudeMDPath := filepath.Join(proj, "CLAUDE.md")
	if err := os.WriteFile(claudeMDPath, []byte("# Personal Secret Header\nMY_PRIVATE_WORKFLOW=keep-me\n\n"), 0644); err != nil {
		t.Fatal(err)
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return proj, nil },
		UserHomeDir: func() (string, error) { return home, nil },
		Getenv: func(k string) string {
			switch k {
			case "HOME", "USERPROFILE":
				return home
			case "CLAUDE_CONFIG_DIR":
				return claudeHome
			default:
				return ""
			}
		},
	}

	run := func(args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(nil, &out, &errBuf)
		deps.Env = env
		deps.LookPath = func(file string) (string, error) {
			if file == "claude" {
				return "/usr/local/bin/claude", nil
			}
			return "", os.ErrNotExist
		}
		err := RunWithDeps(context.Background(), args, deps)
		var resp Envelope
		if bytes.Contains(out.Bytes(), []byte(`"schemaVersion"`)) {
			_ = json.Unmarshal(out.Bytes(), &resp)
		}
		return resp, out.String(), err
	}

	// 1. Read-only install --dry-run --diff --redact creates nothing and redacts home paths.
	prevEnv, _, err := run("install", "engineer", "--runtime", "claude", "--preset", "minimal", "--project", "--dry-run", "--diff", "--redact", "--json")
	if err != nil {
		t.Fatalf("install --dry-run --diff failed: %v", err)
	}
	if prevEnv.Diff == nil || len(prevEnv.Diff.Changes) == 0 || prevEnv.Explain == nil {
		t.Fatalf("expected populated Explain and Diff on preview: %+v", prevEnv)
	}
	if strings.Contains(prevEnv.Roots["project"], "alice-home") {
		t.Fatalf("expected redacted root path, got %q", prevEnv.Roots["project"])
	}
	if _, err := os.Stat(filepath.Join(proj, ".dtkit")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write .dtkit directory")
	}

	// 2. Install engineer@0.4.0 cleanly for claude/project.
	if _, raw, err := run("install", "engineer", "--runtime", "claude", "--preset", "minimal", "--project", "--yes", "--json"); err != nil {
		t.Fatalf("install failed: %v (%s)", err, raw)
	}

	// 3. Simulate direct user edit to a managed skill -> dtkit diff shows drift patch WITHOUT auto-adopting or leaking CLAUDE.md user header.
	managedSkill := filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-ask", "SKILL.md")
	origSkillBytes, err := os.ReadFile(managedSkill)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedSkill, append(origSkillBytes, []byte("\nLOCAL_DRIFT_LINE\n")...), 0644); err != nil {
		t.Fatal(err)
	}

	driftEnv, driftRaw, err := run("diff", "--project", "--runtime", "claude", "--redact", "--json")
	if err != nil {
		t.Fatalf("diff drift failed: %v (%s)", err, driftRaw)
	}
	if driftEnv.Explain == nil || driftEnv.Explain.DriftFiles != 1 || driftEnv.Diff == nil || len(driftEnv.Diff.Changes) != 1 {
		t.Fatalf("expected 1 drifted file in diff, got %+v", driftEnv)
	}
	if !strings.Contains(driftEnv.Diff.Changes[0].Patch, "-LOCAL_DRIFT_LINE") {
		t.Fatalf("expected drift patch showing LOCAL_DRIFT_LINE, got:\n%s", driftEnv.Diff.Changes[0].Patch)
	}
	if strings.Contains(driftRaw, "Personal Secret Header") || strings.Contains(driftRaw, "MY_PRIVATE_WORKFLOW") {
		t.Fatalf("user content outside managed block leaked in diff output:\n%s", driftRaw)
	}

	// Restore clean managed skill (no auto-adoption).
	if err := os.WriteFile(managedSkill, origSkillBytes, 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Fork engineer -> duplicate ID without --same-id fails, then --same-id succeeds with auto patch bump + redacted output.
	_, _, dupErr := run("fork", "engineer", "--as", "engineer", "--dir", forkParent)
	if apperr.ExitCodeOf(dupErr) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict on duplicate fork ID, got %v", dupErr)
	}

	forkEnv, forkRaw, err := run("fork", "engineer", "--as", "engineer", "--same-id", "--dir", forkParent, "--redact", "--json")
	if err != nil {
		t.Fatalf("fork --same-id failed: %v (%s)", err, forkRaw)
	}
	if forkEnv.Fork == nil || forkEnv.Fork.Version != "0.6.1" {
		t.Fatalf("expected forked version 0.6.1, got %+v", forkEnv.Fork)
	}
	if strings.Contains(forkEnv.Fork.Dir, "alice-home") || !strings.HasPrefix(forkEnv.Fork.Dir, "<HOME>") {
		t.Fatalf("expected redacted Fork.Dir, got %q", forkEnv.Fork.Dir)
	}

	// 5. Customize forked kit's rules, preview diff with --redact, then update cleanly twice.
	forkDir := filepath.Join(forkParent, "engineer")
	rulePath := filepath.Join(forkDir, "rules", "engineering.md")
	ruleBytes, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulePath, append(ruleBytes, []byte("\n- Custom Fork Rule One.\n")...), 0644); err != nil {
		t.Fatal(err)
	}

	updDiffEnv, updDiffRaw, err := run("diff", "engineer", "--from", forkDir, "--project", "--runtime", "claude", "--redact", "--json")
	if err != nil {
		t.Fatalf("diff against fork failed: %v (%s)", err, updDiffRaw)
	}
	if updDiffEnv.Explain == nil || updDiffEnv.Explain.InstalledVersion != "0.6.0" || updDiffEnv.Explain.TargetVersion != "0.6.1" {
		t.Fatalf("unexpected Explain metadata: %+v", updDiffEnv.Explain)
	}
	if strings.Contains(updDiffEnv.Explain.SourceLocation, "alice-home") {
		t.Fatalf("expected redacted Explain.SourceLocation, got %q", updDiffEnv.Explain.SourceLocation)
	}
	if updDiffEnv.Diff == nil || len(updDiffEnv.Diff.Changes) == 0 {
		t.Fatalf("expected changes in diff against fork")
	}

	updEnv, updRaw, err := run("update", "engineer", "--from", forkDir, "--project", "--runtime", "claude", "--diff", "--redact", "--yes", "--json")
	if err != nil {
		t.Fatalf("update from fork failed: %v (%s)", err, updRaw)
	}
	if updEnv.Kit == nil || updEnv.Kit.Version != "0.6.1" {
		t.Fatalf("expected updated version 0.6.1, got %+v", updEnv.Kit)
	}

	// Confirm personal CLAUDE.md text stayed untouched while fork rule landed in DTKit's managed content root.
	afterClaudeMD, err := os.ReadFile(claudeMDPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(afterClaudeMD), "MY_PRIVATE_WORKFLOW=keep-me") || !strings.Contains(string(afterClaudeMD), "<!-- dtkit:begin engineer claude project -->") {
		t.Fatalf("unexpected CLAUDE.md after fork update:\n%s", string(afterClaudeMD))
	}
	updatedRule, err := os.ReadFile(filepath.Join(proj, ".dtkit", "runtimes", "claude", "content", "engineer", "rules", "engineering.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updatedRule), "Custom Fork Rule One.") {
		t.Fatalf("expected updated forked rule in .dtkit/runtimes/claude/content:\n%s", string(updatedRule))
	}

	// Bump forked manifest 0.4.1 -> 0.4.2 and update again cleanly.
	pkgJSONPath := filepath.Join(forkDir, "package.json")
	pkgRaw, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkgJSONPath, bytes.Replace(pkgRaw, []byte(`"0.4.1"`), []byte(`"0.4.2"`), 1), 0644); err != nil {
		t.Fatal(err)
	}
	if _, upd2Raw, err := run("update", "engineer", "--from", forkDir, "--project", "--runtime", "claude", "--yes", "--json"); err != nil {
		t.Fatalf("second update after fork failed: %v (%s)", err, upd2Raw)
	}

	// 6. Fork as an independent ID (team-kit) and install alongside engineer without collisions.
	if _, fork2Raw, err := run("fork", "engineer", "--as", "team-kit", "--dir", forkParent, "--json"); err != nil {
		t.Fatalf("fork team-kit failed: %v (%s)", err, fork2Raw)
	}
	teamDir := filepath.Join(forkParent, "team-kit")
	if _, inst2Raw, err := run("install", "team-kit", "--from", teamDir, "--runtime", "claude", "--preset", "minimal", "--project", "--yes", "--json"); err != nil {
		t.Fatalf("coexistence install team-kit failed: %v (%s)", err, inst2Raw)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-team-kit-ask", "SKILL.md")); err != nil {
		t.Fatalf("missing namespaced team-kit skill: %v", err)
	}
}
