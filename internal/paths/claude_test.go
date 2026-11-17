package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveClaudeGlobal(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "user")
	customConfig := filepath.Join(base, "custom-claude-config")
	for _, d := range []string{home, customConfig} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Default resolves directly to ~/.claude with no warnings.
	env := Environment{
		UserHomeDir: func() (string, error) { return home, nil },
		Getenv:      func(string) string { return "" },
	}
	res, err := ResolveClaudeGlobal(env)
	if err != nil {
		t.Fatal(err)
	}
	if res.ClaudeHome != filepath.Join(home, ".claude") || len(res.Warnings) != 0 {
		t.Fatalf("unexpected default ClaudeHome: %+v", res)
	}

	// Verified against claude 2.1.319: CLAUDE_CONFIG_DIR replaces ~/.claude directly (no nested .claude).
	env.Getenv = func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return customConfig
		}
		return ""
	}
	res, err = ResolveClaudeGlobal(env)
	if err != nil {
		t.Fatal(err)
	}
	if res.ClaudeHome != customConfig {
		t.Fatalf("expected ClaudeHome=%s, got %s", customConfig, res.ClaudeHome)
	}

	// Relative CLAUDE_CONFIG_DIR emits a clear warning.
	env.Getenv = func(k string) string {
		if k == "CLAUDE_CONFIG_DIR" {
			return "rel-claude"
		}
		return ""
	}
	res, err = ResolveClaudeGlobal(env)
	if err != nil || !filepath.IsAbs(res.ClaudeHome) || len(res.Warnings) == 0 || !strings.Contains(res.Warnings[0], "CLAUDE_CONFIG_DIR") {
		t.Fatalf("expected relative CLAUDE_CONFIG_DIR warning, got %+v err=%v", res, err)
	}

	if runtime.GOOS != "windows" {
		symDir := filepath.Join(base, "sym-claude")
		if err := os.Symlink(customConfig, symDir); err == nil {
			env.Getenv = func(k string) string {
				if k == "CLAUDE_CONFIG_DIR" {
					return symDir
				}
				return ""
			}
			if _, err := ResolveClaudeGlobal(env); err == nil {
				t.Fatal("expected symlink CLAUDE_CONFIG_DIR to be rejected")
			}
		}
	}
}
