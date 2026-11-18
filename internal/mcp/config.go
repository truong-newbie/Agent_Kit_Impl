package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// MCPConfigStatus describes DTKit's MCP server wiring state for one runtime.
type MCPConfigStatus struct {
	Runtime     string   `json:"runtime"`
	ConfigPath  string   `json:"configPath"`
	Enabled     bool     `json:"enabled"`
	Command     string   `json:"command,omitempty"`
	Args        []string `json:"args,omitempty"`
	PeerServers []string `json:"peerServers,omitempty"`
}

// ConfigRelPath returns the project-relative MCP configuration file path for runtimeID.
func ConfigRelPath(runtimeID string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(runtimeID)) {
	case "", "codex":
		return ".codex/mcp.json", nil
	case "claude":
		return ".mcp.json", nil
	case "cursor":
		return ".cursor/mcp.json", nil
	case "gemini":
		return ".gemini/settings.json", nil
	case "opencode":
		return "opencode.json", nil
	default:
		return "", apperr.Usage("unsupported_runtime", fmt.Sprintf("unsupported runtime %q", runtimeID))
	}
}

// InspectMCPConfig reads the runtime's project MCP config without mutating disk.
func InspectMCPConfig(repoRoot, runtimeID string) (MCPConfigStatus, error) {
	normRT := normalizeRuntimeID(runtimeID)
	rel, err := ConfigRelPath(normRT)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	target, err := paths.ResolveTarget(repoRoot, rel)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	st := MCPConfigStatus{
		Runtime:    normRT,
		ConfigPath: rel,
	}
	info, lErr := os.Lstat(target)
	if errors.Is(lErr, os.ErrNotExist) {
		return st, nil
	}
	if lErr != nil {
		return st, lErr
	}
	if paths.IsReparseOrSymlink(info) {
		return st, apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked MCP config %s", rel)), rel)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return st, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return st, nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return st, apperr.WithPath(apperr.Conflict("invalid_mcp_config", fmt.Sprintf("invalid JSON in %s: %v", rel, err)), rel)
	}

	sectionKey := "mcpServers"
	if normRT == "opencode" {
		sectionKey = "mcp"
	}
	servers, ok := root[sectionKey].(map[string]any)
	if !ok {
		return st, nil
	}

	for name, val := range servers {
		if name != "dtkit" {
			st.PeerServers = append(st.PeerServers, name)
			continue
		}
		entry, ok := val.(map[string]any)
		if !ok {
			continue
		}
		st.Enabled = true
		if normRT == "opencode" {
			if cmdSlice, ok := entry["command"].([]any); ok && len(cmdSlice) > 0 {
				if s, ok := cmdSlice[0].(string); ok {
					st.Command = s
				}
				for i := 1; i < len(cmdSlice); i++ {
					if s, ok := cmdSlice[i].(string); ok {
						st.Args = append(st.Args, s)
					}
				}
			}
		} else {
			if c, ok := entry["command"].(string); ok {
				st.Command = c
			}
			if argSlice, ok := entry["args"].([]any); ok {
				for _, a := range argSlice {
					if s, ok := a.(string); ok {
						st.Args = append(st.Args, s)
					}
				}
			}
		}
	}
	sort.Strings(st.PeerServers)
	return st, nil
}

// EnableMCPConfig merges the dtkit MCP server entry into the runtime's project config file while preserving user entries.
func EnableMCPConfig(repoRoot, runtimeID, exePath string, dryRun bool) (MCPConfigStatus, error) {
	normRT := normalizeRuntimeID(runtimeID)
	rel, err := ConfigRelPath(normRT)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	target, err := paths.ResolveTarget(repoRoot, rel)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	cmdBin := strings.TrimSpace(exePath)
	if cmdBin == "" {
		cmdBin = "dtkit"
	}

	root, err := loadOrInitJSONObject(target, rel)
	if err != nil {
		return MCPConfigStatus{}, err
	}

	sectionKey := "mcpServers"
	if normRT == "opencode" {
		sectionKey = "mcp"
	}
	servers, ok := root[sectionKey].(map[string]any)
	if !ok || servers == nil {
		servers = map[string]any{}
	}

	var peers []string
	for k := range servers {
		if k != "dtkit" {
			peers = append(peers, k)
		}
	}
	sort.Strings(peers)

	args := []string{"mcp", "serve"}
	if normRT == "opencode" {
		servers["dtkit"] = map[string]any{
			"type":    "local",
			"command": []string{cmdBin, "mcp", "serve"},
			"enabled": true,
		}
	} else {
		servers["dtkit"] = map[string]any{
			"command": cmdBin,
			"args":    args,
		}
	}
	root[sectionKey] = servers

	st := MCPConfigStatus{
		Runtime:     normRT,
		ConfigPath:  rel,
		Enabled:     true,
		Command:     cmdBin,
		Args:        args,
		PeerServers: peers,
	}
	if dryRun {
		return st, nil
	}
	if err := writeJSONObjectAtomically(target, rel, root); err != nil {
		return MCPConfigStatus{}, err
	}
	return st, nil
}

// DisableMCPConfig removes only the dtkit MCP server entry from the runtime's project config file, preserving all user keys.
func DisableMCPConfig(repoRoot, runtimeID string, dryRun bool) (MCPConfigStatus, error) {
	normRT := normalizeRuntimeID(runtimeID)
	rel, err := ConfigRelPath(normRT)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	target, err := paths.ResolveTarget(repoRoot, rel)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	info, lErr := os.Lstat(target)
	if errors.Is(lErr, os.ErrNotExist) {
		return MCPConfigStatus{Runtime: normRT, ConfigPath: rel, Enabled: false}, nil
	}
	if lErr != nil {
		return MCPConfigStatus{}, lErr
	}
	if paths.IsReparseOrSymlink(info) {
		return MCPConfigStatus{}, apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked MCP config %s", rel)), rel)
	}

	root, err := loadOrInitJSONObject(target, rel)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	sectionKey := "mcpServers"
	if normRT == "opencode" {
		sectionKey = "mcp"
	}
	var peers []string
	if servers, ok := root[sectionKey].(map[string]any); ok && servers != nil {
		delete(servers, "dtkit")
		for k := range servers {
			peers = append(peers, k)
		}
		sort.Strings(peers)
		root[sectionKey] = servers
	}
	st := MCPConfigStatus{
		Runtime:     normRT,
		ConfigPath:  rel,
		Enabled:     false,
		PeerServers: peers,
	}
	if dryRun {
		return st, nil
	}
	if err := writeJSONObjectAtomically(target, rel, root); err != nil {
		return MCPConfigStatus{}, err
	}
	return st, nil
}

func normalizeRuntimeID(rt string) string {
	clean := strings.ToLower(strings.TrimSpace(rt))
	if clean == "" {
		return adapter.DefaultRuntime
	}
	return clean
}

func loadOrInitJSONObject(target, rel string) (map[string]any, error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	if paths.IsReparseOrSymlink(info) {
		return nil, apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked MCP config %s", rel)), rel)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil || root == nil {
		return nil, apperr.WithPath(apperr.Conflict("invalid_mcp_config", fmt.Sprintf("invalid JSON object in %s", rel)), rel)
	}
	return root, nil
}

func writeJSONObjectAtomically(target, rel string, root map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	if info, lErr := os.Lstat(target); lErr == nil && paths.IsReparseOrSymlink(info) {
		return apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked MCP config %s", rel)), rel)
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
