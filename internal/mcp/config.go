package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	codexMCPBeginMarker = "# dtkit:begin mcp"
	codexMCPEndMarker   = "# dtkit:end mcp"
)

var (
	tomlSectionRe   = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(?:#.*)?$`)
	tomlStringValRe = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
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
		return ".codex/config.toml", nil
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
	if normRT == "codex" {
		return inspectCodexTOML(st, string(raw)), nil
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
	if normRT == "codex" {
		return enableCodexTOML(target, rel, normRT, cmdBin, dryRun)
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
	if normRT == "codex" {
		return disableCodexTOML(target, rel, normRT, dryRun)
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

func inspectCodexTOML(st MCPConfigStatus, content string) MCPConfigStatus {
	norm := strings.ReplaceAll(content, "\r\n", "\n")
	curSection := ""
	peersMap := map[string]bool{}
	for _, rawLine := range strings.Split(norm, "\n") {
		if m := tomlSectionRe.FindStringSubmatch(rawLine); len(m) == 2 {
			curSection = strings.TrimSpace(m[1])
			if strings.HasPrefix(curSection, "mcp_servers.") {
				name := strings.TrimPrefix(curSection, "mcp_servers.")
				if idx := strings.IndexByte(name, '.'); idx >= 0 {
					name = name[:idx]
				}
				if name != "" && name != "dtkit" {
					peersMap[name] = true
				}
			}
			continue
		}
		if curSection == "mcp_servers.dtkit" {
			st.Enabled = true
			trimmed := strings.TrimSpace(rawLine)
			if k, v, ok := strings.Cut(trimmed, "="); ok {
				key := strings.TrimSpace(k)
				val := strings.TrimSpace(v)
				switch key {
				case "command":
					if sm := tomlStringValRe.FindStringSubmatch(val); len(sm) == 2 {
						st.Command = unescapeTOMLString(sm[1])
					}
				case "args":
					for _, am := range tomlStringValRe.FindAllStringSubmatch(val, -1) {
						st.Args = append(st.Args, unescapeTOMLString(am[1]))
					}
				}
			}
		}
	}
	for p := range peersMap {
		st.PeerServers = append(st.PeerServers, p)
	}
	sort.Strings(st.PeerServers)
	return st
}

func enableCodexTOML(target, rel, normRT, cmdBin string, dryRun bool) (MCPConfigStatus, error) {
	var existing string
	exists := false
	info, lErr := os.Lstat(target)
	if lErr == nil {
		if paths.IsReparseOrSymlink(info) {
			return MCPConfigStatus{}, apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked MCP config %s", rel)), rel)
		}
		raw, err := os.ReadFile(target)
		if err != nil {
			return MCPConfigStatus{}, err
		}
		existing = string(raw)
		exists = true
	} else if !errors.Is(lErr, os.ErrNotExist) {
		return MCPConfigStatus{}, lErr
	}

	lineEnding := "\n"
	if strings.Contains(existing, "\r\n") {
		lineEnding = "\r\n"
	}
	norm := strings.ReplaceAll(existing, "\r\n", "\n")
	block := fmt.Sprintf("%s\n[mcp_servers.dtkit]\ncommand = %q\nargs = [\"mcp\", \"serve\"]\n%s\n",
		codexMCPBeginMarker, cmdBin, codexMCPEndMarker)

	beginIdx := strings.Index(norm, codexMCPBeginMarker)
	endIdx := strings.Index(norm, codexMCPEndMarker)
	var updated string
	switch {
	case beginIdx >= 0 && endIdx > beginIdx:
		afterEnd := endIdx + len(codexMCPEndMarker)
		if afterEnd < len(norm) && norm[afterEnd] == '\n' {
			afterEnd++
		}
		updated = norm[:beginIdx] + block + norm[afterEnd:]
	case (beginIdx >= 0) != (endIdx >= 0) || (beginIdx >= 0 && endIdx < beginIdx):
		return MCPConfigStatus{}, apperr.WithPath(apperr.Conflict("mcp_block_malformed", fmt.Sprintf("malformed dtkit MCP block markers in %s", rel)), rel)
	default:
		if hasUnmanagedCodexDTKitSection(norm) {
			return MCPConfigStatus{}, apperr.WithPath(apperr.Conflict("mcp_entry_conflict", fmt.Sprintf("unmanaged [mcp_servers.dtkit] already exists in %s", rel)), rel)
		}
		if !exists || len(strings.TrimSpace(norm)) == 0 {
			updated = block
		} else {
			sep := ""
			if !strings.HasSuffix(norm, "\n") {
				sep = "\n"
			}
			if !strings.HasSuffix(norm, "\n\n") {
				sep += "\n"
			}
			updated = norm + sep + block
		}
	}

	st := inspectCodexTOML(MCPConfigStatus{Runtime: normRT, ConfigPath: rel}, updated)
	if dryRun {
		return st, nil
	}
	finalBytes := []byte(strings.ReplaceAll(updated, "\n", lineEnding))
	if err := writeBytesAtomically(target, rel, finalBytes); err != nil {
		return MCPConfigStatus{}, err
	}
	return st, nil
}

func disableCodexTOML(target, rel, normRT string, dryRun bool) (MCPConfigStatus, error) {
	raw, err := os.ReadFile(target)
	if err != nil {
		return MCPConfigStatus{}, err
	}
	existing := string(raw)
	lineEnding := "\n"
	if strings.Contains(existing, "\r\n") {
		lineEnding = "\r\n"
	}
	norm := strings.ReplaceAll(existing, "\r\n", "\n")

	beginIdx := strings.Index(norm, codexMCPBeginMarker)
	endIdx := strings.Index(norm, codexMCPEndMarker)
	if beginIdx < 0 && endIdx < 0 {
		if hasUnmanagedCodexDTKitSection(norm) {
			return MCPConfigStatus{}, apperr.WithPath(apperr.Conflict("mcp_entry_conflict", fmt.Sprintf("unmanaged [mcp_servers.dtkit] in %s will not be removed", rel)), rel)
		}
		return inspectCodexTOML(MCPConfigStatus{Runtime: normRT, ConfigPath: rel}, norm), nil
	}
	if beginIdx < 0 || endIdx < beginIdx {
		return MCPConfigStatus{}, apperr.WithPath(apperr.Conflict("mcp_block_malformed", fmt.Sprintf("malformed dtkit MCP block markers in %s", rel)), rel)
	}
	afterEnd := endIdx + len(codexMCPEndMarker)
	if afterEnd < len(norm) && norm[afterEnd] == '\n' {
		afterEnd++
	}
	updated := strings.TrimRight(norm[:beginIdx], "\n")
	rest := strings.TrimLeft(norm[afterEnd:], "\n")
	switch {
	case updated != "" && rest != "":
		updated += "\n\n" + rest
	case updated != "":
		updated += "\n"
	case rest != "":
		updated = rest
	}
	st := inspectCodexTOML(MCPConfigStatus{Runtime: normRT, ConfigPath: rel}, updated)
	if dryRun {
		return st, nil
	}
	finalBytes := []byte(strings.ReplaceAll(updated, "\n", lineEnding))
	if err := writeBytesAtomically(target, rel, finalBytes); err != nil {
		return MCPConfigStatus{}, err
	}
	return st, nil
}

func hasUnmanagedCodexDTKitSection(norm string) bool {
	for _, rawLine := range strings.Split(norm, "\n") {
		if m := tomlSectionRe.FindStringSubmatch(rawLine); len(m) == 2 {
			sec := strings.TrimSpace(m[1])
			if sec == "mcp_servers.dtkit" || strings.HasPrefix(sec, "mcp_servers.dtkit.") {
				return true
			}
		}
	}
	return false
}

func unescapeTOMLString(s string) string {
	r := strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\n`, "\n", `\t`, "\t")
	return r.Replace(s)
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
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeBytesAtomically(target, rel, data)
}

func writeBytesAtomically(target, rel string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	if info, lErr := os.Lstat(target); lErr == nil && paths.IsReparseOrSymlink(info) {
		return apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked MCP config %s", rel)), rel)
	}
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
