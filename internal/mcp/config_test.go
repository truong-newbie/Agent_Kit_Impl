package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func TestMCPConfigEnableInspectAndDisablePreservesPeers(t *testing.T) {
	repo := t.TempDir()

	// Seed Claude .mcp.json, Gemini .gemini/settings.json, and Codex .codex/config.toml
	if err := os.WriteFile(filepath.Join(repo, ".mcp.json"), []byte(`{"mcpServers":{"github":{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".gemini"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gemini", "settings.json"), []byte(`{"theme":"dark","mcpServers":{"custom":{"command":"custom-mcp"}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}
	userToml := "model = \"gpt-5-codex\"\n\n[mcp_servers.github]\ncommand = \"npx\"\n"
	if err := os.WriteFile(filepath.Join(repo, ".codex", "config.toml"), []byte(userToml), 0644); err != nil {
		t.Fatal(err)
	}

	// Dry-run does not modify disk
	drySt, err := EnableMCPConfig(repo, "cursor", "/usr/local/bin/dtkit", true)
	if err != nil || !drySt.Enabled {
		t.Fatalf("dry-run EnableMCPConfig failed: %+v err=%v", drySt, err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".cursor", "mcp.json")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run should not write .cursor/mcp.json")
	}

	// Enable across all 5 runtimes (including Codex .codex/config.toml)
	for _, rt := range adapter.SupportedRuntimes() {
		st, eErr := EnableMCPConfig(repo, rt, "/usr/local/bin/dtkit", false)
		if eErr != nil || !st.Enabled {
			t.Fatalf("EnableMCPConfig(%s) failed: %+v err=%v", rt, st, eErr)
		}
		insp, iErr := InspectMCPConfig(repo, rt)
		if iErr != nil || !insp.Enabled || insp.Command != "/usr/local/bin/dtkit" {
			t.Fatalf("InspectMCPConfig(%s) mismatch: %+v err=%v", rt, insp, iErr)
		}
	}

	// Verify Codex config.toml preserved user lines & peer [mcp_servers.github], then cleanly disabled
	codexSt, err := InspectMCPConfig(repo, "codex")
	if err != nil || len(codexSt.PeerServers) != 1 || codexSt.PeerServers[0] != "github" {
		t.Fatalf("expected peer server github in .codex/config.toml: %+v err=%v", codexSt, err)
	}
	if _, err := DisableMCPConfig(repo, "codex", false); err != nil {
		t.Fatalf("DisableMCPConfig(codex) failed: %v", err)
	}
	afterToml, err := os.ReadFile(filepath.Join(repo, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterToml) != userToml {
		t.Fatalf("expected byte-exact preservation of user config.toml after disable, got:\n%q\nwant:\n%q", string(afterToml), userToml)
	}

	// Verify peer servers survived in Claude
	claudeSt, err := InspectMCPConfig(repo, "claude")
	if err != nil || len(claudeSt.PeerServers) != 1 || claudeSt.PeerServers[0] != "github" {
		t.Fatalf("expected peer server github preserved in .mcp.json: %+v err=%v", claudeSt, err)
	}

	// Disable on Gemini preserves "theme":"dark" and peer server "custom"
	disSt, err := DisableMCPConfig(repo, "gemini", false)
	if err != nil || disSt.Enabled || len(disSt.PeerServers) != 1 || disSt.PeerServers[0] != "custom" {
		t.Fatalf("DisableMCPConfig(gemini) failed: %+v err=%v", disSt, err)
	}
	gemRaw, err := os.ReadFile(filepath.Join(repo, ".gemini", "settings.json"))
	if err != nil || !strings.Contains(string(gemRaw), `"theme": "dark"`) || !strings.Contains(string(gemRaw), `"custom"`) {
		t.Fatalf("expected Gemini user settings preserved after disable:\n%s", string(gemRaw))
	}

	// Unmanaged [mcp_servers.dtkit] in .codex/config.toml refuses overwrite with ExitConflict
	conflictRepo := t.TempDir()
	_ = os.MkdirAll(filepath.Join(conflictRepo, ".codex"), 0755)
	_ = os.WriteFile(filepath.Join(conflictRepo, ".codex", "config.toml"), []byte("[mcp_servers.dtkit]\ncommand = \"custom\"\n"), 0644)
	if _, err := EnableMCPConfig(conflictRepo, "codex", "/usr/local/bin/dtkit", false); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict for unmanaged [mcp_servers.dtkit], got %v", err)
	}
}
