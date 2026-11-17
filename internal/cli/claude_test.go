package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestClaudeRuntimeLifecycleCoexistenceWorktreeAndMismatch(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "repo")
	subDir := filepath.Join(proj, "packages", "api")
	home := filepath.Join(base, "home")
	claudeCfg := filepath.Join(base, "claude-cfg")
	codexHome := filepath.Join(home, ".codex")
	for _, d := range []string{subDir, home, claudeCfg, codexHome} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Pre-seed user CLAUDE.md, user Claude skill, and user .claude/settings.json.
	userClaudeMD := "# Personal Project Claude Notes\nKeep user section intact.\n"
	if err := os.WriteFile(filepath.Join(proj, "CLAUDE.md"), []byte(userClaudeMD), 0644); err != nil {
		t.Fatal(err)
	}
	userSkillPath := filepath.Join(proj, ".claude", "skills", "team-lint", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userSkillPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userSkillPath, []byte("---\nname: team-lint\ndescription: user skill\n---\nUser lint rules.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(proj, ".claude", "settings.json")
	settingsBody := `{"permissions":{"allow":["Read"]}}` + "\n"
	if err := os.WriteFile(settingsPath, []byte(settingsBody), 0644); err != nil {
		t.Fatal(err)
	}

	curCfg := ""
	cwd := proj
	makeDeps := func() (Deps, *bytes.Buffer, *bytes.Buffer) {
		var out, errBuf bytes.Buffer
		return Deps{
			Stdout: &out,
			Stderr: &errBuf,
			Env: paths.Environment{
				Getwd:       func() (string, error) { return cwd, nil },
				UserHomeDir: func() (string, error) { return home, nil },
				Getenv: func(k string) string {
					switch k {
					case "HOME", "USERPROFILE":
						return home
					case "CODEX_HOME":
						return codexHome
					case "CLAUDE_CONFIG_DIR":
						return curCfg
					}
					return ""
				},
			},
			Now:      func() time.Time { return time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC) },
			LookPath: func(file string) (string, error) { return filepath.Join(base, "bin", file), nil },
		}, &out, &errBuf
	}

	// 1. Install all 5 runtimes (codex, gemini, opencode, cursor, claude) into the SAME project.
	for _, rt := range adapter.SupportedRuntimes() {
		deps, out, _ := makeDeps()
		if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--path", proj, "--runtime", rt, "--yes", "--json"}, deps); err != nil {
			t.Fatalf("install runtime %s failed: %v\n%s", rt, err, out.String())
		}
	}

	// Unified status must report 5 healthy installations.
	deps, out, _ := makeDeps()
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--path", proj, "--json"}, deps); err != nil {
		t.Fatalf("status failed: %v\n%s", err, out.String())
	}
	var stEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &stEnv); err != nil {
		t.Fatal(err)
	}
	if len(stEnv.Installations) != 5 {
		t.Fatalf("expected 5 coexistence installations, got %d (%s)", len(stEnv.Installations), out.String())
	}

	// 2. Global Claude install respects default ~/.claude, then custom CLAUDE_CONFIG_DIR triggers profile_mismatch.
	deps, out, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--global", "--runtime", "claude", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("claude global default install failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "CLAUDE.md")); err != nil {
		t.Fatalf("expected global ~/.claude/CLAUDE.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "dtkit-claude-global-implement", "SKILL.md")); err != nil {
		t.Fatalf("expected global skill in ~/.claude/skills: %v", err)
	}

	// Switch CLAUDE_CONFIG_DIR -> status reports profile mismatch cleanly.
	curCfg = claudeCfg
	deps, out, _ = makeDeps()
	err = RunWithDeps(context.Background(), []string{"status", "--global", "--runtime", "claude", "--json"}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected conflict on CLAUDE_CONFIG_DIR mismatch, got %v (%s)", err, out.String())
	}
	// Fresh install in custom CLAUDE_CONFIG_DIR succeeds after removing old ~/.claude installation.
	curCfg = ""
	deps, _, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"remove", "engineer", "--global", "--runtime", "claude", "--yes"}, deps); err != nil {
		t.Fatal(err)
	}
	curCfg = claudeCfg
	deps, _, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--global", "--runtime", "claude", "--yes"}, deps); err != nil {
		t.Fatalf("claude global CLAUDE_CONFIG_DIR install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(claudeCfg, "skills", "dtkit-claude-global-plan", "SKILL.md")); err != nil {
		t.Fatalf("expected skill under $CLAUDE_CONFIG_DIR/skills: %v", err)
	}

	// 3. Monorepo subdirectory + Git worktree resolution check.
	if _, gitErr := exec.LookPath("git"); gitErr == nil {
		gitRun := func(dir string, args ...string) {
			c := exec.Command("git", append([]string{"-C", dir}, args...)...)
			if goOut, gErr := c.CombinedOutput(); gErr != nil {
				t.Fatalf("git %v failed: %v\n%s", args, gErr, goOut)
			}
		}
		gitRun(proj, "init", "-q")
		gitRun(proj, "config", "user.name", "DTKit Test")
		gitRun(proj, "config", "user.email", "dtkit@example.com")
		gitRun(proj, "add", "CLAUDE.md")
		gitRun(proj, "commit", "-q", "-m", "init")

		// Subdirectory inside monorepo resolves to main repo root via default Environment GitTopLevel!
		subDeps := DefaultDeps(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
		subDeps.Env.Getwd = func() (string, error) { return subDir, nil }
		subDeps.Env.UserHomeDir = func() (string, error) { return home, nil }
		resProj, pErr := paths.ResolveProject(context.Background(), subDeps.Env, "")
		if pErr != nil || resProj.Root != proj {
			t.Fatalf("expected monorepo subdir to resolve to %s, got %+v err=%v", proj, resProj, pErr)
		}

		wtDir := filepath.Join(base, "wt-branch")
		gitRun(proj, "worktree", "add", "-q", "-b", "wt-feat", wtDir)
		wtDeps := DefaultDeps(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
		wtDeps.Env.Getwd = func() (string, error) { return wtDir, nil }
		wtDeps.Env.UserHomeDir = func() (string, error) { return home, nil }
		wtProj, wErr := paths.ResolveProject(context.Background(), wtDeps.Env, "")
		if wErr != nil || wtProj.Root != wtDir {
			t.Fatalf("expected git worktree to resolve to its own top-level %s, got %+v err=%v", wtDir, wtProj, wErr)
		}
	}

	// 4. Clean remove restores user CLAUDE.md, user skill, and .claude/settings.json cleanly.
	deps, _, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"remove", "engineer", "--project", "--path", proj, "--runtime", "claude", "--yes"}, deps); err != nil {
		t.Fatalf("claude project remove failed: %v", err)
	}
	gotClaudeMD, _ := os.ReadFile(filepath.Join(proj, "CLAUDE.md"))
	if string(gotClaudeMD) != userClaudeMD {
		t.Fatalf("user CLAUDE.md corrupted after remove:\n%s", string(gotClaudeMD))
	}
	gotSkill, _ := os.ReadFile(userSkillPath)
	if !strings.Contains(string(gotSkill), "User lint rules.") {
		t.Fatal("user .claude/skills/team-lint lost after remove")
	}
	gotSettings, _ := os.ReadFile(settingsPath)
	if string(gotSettings) != settingsBody {
		t.Fatal("user .claude/settings.json altered after remove")
	}
}
