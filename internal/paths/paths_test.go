package paths

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func TestResolveProjectModes(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")
	sub := filepath.Join(repo, "sub dir", "dự-án")
	for _, d := range []string{home, repo, sub} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	env := Environment{
		Getwd:       func() (string, error) { return sub, nil },
		UserHomeDir: func() (string, error) { return home, nil },
		Getenv:      func(string) string { return "" },
		GitTopLevel: func(context.Context, string) (string, error) { return repo, nil },
	}
	res, err := ResolveProject(context.Background(), env, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Root != repo || len(res.Warnings) != 0 {
		t.Fatalf("unexpected git root resolution: %+v", res)
	}

	// Explicit subfolder path emits warning when different from git root.
	res, err = ResolveProject(context.Background(), env, sub)
	if err != nil {
		t.Fatal(err)
	}
	if res.Root != sub || len(res.Warnings) == 0 {
		t.Fatalf("expected subfolder warning: %+v", res)
	}

	// Home directory rejected for --project.
	_, err = ResolveProject(context.Background(), env, home)
	if apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected usage error for home root, got %v", err)
	}

	// Non-git fallback to CWD emits warning.
	env.GitTopLevel = func(context.Context, string) (string, error) { return "", errors.New("not git") }
	res, err = ResolveProject(context.Background(), env, "")
	if err != nil || res.Root != sub || len(res.Warnings) == 0 {
		t.Fatalf("unexpected cwd fallback: %+v, err=%v", res, err)
	}
}

func TestResolveGlobalCodexHome(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "user")
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	env := Environment{
		UserHomeDir: func() (string, error) { return home, nil },
		Getenv:      func(k string) string { return "relative-codex" },
	}
	res, err := ResolveGlobal(env)
	if err != nil {
		t.Fatal(err)
	}
	if res.UserHome != home || !filepath.IsAbs(res.CodexHome) || len(res.Warnings) == 0 {
		t.Fatalf("expected relative CODEX_HOME warning and abs path: %+v", res)
	}
}

func TestResolveTargetRejectsUnsafeAndAllowsUnresolvedParentTemp(t *testing.T) {
	root := t.TempDir()
	target, err := ResolveTarget(root, ".agents/skills/dtkit-project-plan/SKILL.md")
	if err != nil {
		t.Fatalf("unexpected error on t.TempDir: %v", err)
	}
	if !strings.HasPrefix(target, filepath.Clean(root)) {
		t.Fatalf("unexpected target: %s", target)
	}
	for _, bad := range []string{"../escape", "CON.txt", "a:b", "dir\\file", "trailing."} {
		if _, err := ResolveTarget(root, bad); err == nil {
			t.Fatalf("accepted unsafe relative path %q", bad)
		}
	}
}

func TestResolveTargetRejectsSymlinkRootAndChild(t *testing.T) {
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	linkRoot := filepath.Join(base, "link-root")
	if err := os.Symlink(realDir, linkRoot); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := ResolveTarget(linkRoot, "AGENTS.md"); err == nil {
		t.Fatal("accepted symlink root")
	}
	childLink := filepath.Join(realDir, ".agents")
	otherDir := filepath.Join(base, "other")
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(otherDir, childLink); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := ResolveTarget(realDir, ".agents/skills/x/SKILL.md"); err == nil {
		t.Fatal("accepted symlink child ancestor")
	}
}
