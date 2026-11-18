package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
)

func TestMCPConfigEnableInspectAndDisablePreservesPeers(t *testing.T) {
	repo := t.TempDir()

	// Seed Claude .mcp.json and Gemini .gemini/settings.json with existing user config & peer servers
	if err := os.WriteFile(filepath.Join(repo, ".mcp.json"), []byte(`{"mcpServers":{"github":{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".gemini"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gemini", "settings.json"), []byte(`{"theme":"dark","mcpServers":{"custom":{"command":"custom-mcp"}}}`), 0644); err != nil {
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

	// Enable across all 5 runtimes
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

	// Verify peer servers and unrelated settings survived
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
}
