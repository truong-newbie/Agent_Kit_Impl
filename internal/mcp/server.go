// Package mcp implements a zero-dependency STDIO JSON-RPC 2.0 Model Context Protocol server
// exposing DTKit's scout, project inspection, and guard verification tools directly to AI agents.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/guard"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/scout"
)

const ProtocolVersion = "2024-11-05"

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolAnnotations advertises MCP safety hints so clients know DTKit tools are non-destructive and read-only.
type ToolAnnotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
	IdempotentHint  bool `json:"idempotentHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
}

type toolDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	InputSchema map[string]any   `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// Serve runs a blocking STDIO JSON-RPC 2.0 MCP server loop until EOF or context cancellation.
func Serve(ctx context.Context, env paths.Environment, defaultRoot string, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			if wErr := enc.Encode(response{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   &rpcError{Code: -32700, Message: fmt.Sprintf("parse error: %v", err)},
			}); wErr != nil {
				return wErr
			}
			continue
		}

		// JSON-RPC notifications have no id and require no response.
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue
		}

		resp := dispatch(ctx, env, defaultRoot, req)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func dispatch(ctx context.Context, env paths.Environment, defaultRoot string, req request) response {
	base := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		base.Result = map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "dtkit",
				"version": buildinfo.Current().Version,
			},
		}
		return base
	case "ping":
		base.Result = map[string]any{}
		return base
	case "tools/list":
		base.Result = map[string]any{
			"tools": ToolDefinitions(),
		}
		return base
	case "tools/call":
		var params toolCallParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &params); err != nil {
				base.Error = &rpcError{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
				return base
			}
		}
		base.Result = executeTool(ctx, env, defaultRoot, params)
		return base
	default:
		base.Error = &rpcError{Code: -32601, Message: fmt.Sprintf("method %q not found", req.Method)}
		return base
	}
}

// ToolDefinitions returns the MCP tool metadata advertised by dtkit mcp serve.
func ToolDefinitions() []toolDefinition {
	ro := &ToolAnnotations{
		ReadOnlyHint:    true,
		DestructiveHint: false,
		IdempotentHint:  true,
		OpenWorldHint:   false,
	}
	return []toolDefinition{
		{
			Name:        "dtkit_scout",
			Description: "Scan repository layout, workspace zones (FE/BE/AI/DB), languages, contracts, and matched files read-only.",
			Annotations: ro,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string", "description": "Optional repository path override."},
					"query":    map[string]any{"type": "string", "description": "Keyword(s) to rank matching files."},
					"zone":     map[string]any{"type": "string", "description": "Optional subdirectory zone filter (e.g. backend, frontend)."},
					"pack":     map[string]any{"type": "boolean", "description": "Include redacted file snippets up to maxBytes."},
					"maxBytes": map[string]any{"type": "integer", "description": "Content pack byte budget (default 32768)."},
				},
			},
		},
		{
			Name:        "dtkit_project_inspect",
			Description: "Inspect detected workspace stacks (Go, Node/TS, Python, Java, C++, Rust, .NET) and quality checks strictly read-only.",
			Annotations: ro,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Optional repository path override."},
				},
			},
		},
		{
			Name:        "dtkit_guard_status",
			Description: "Read active DTKit edit firewall mode, allowed/denied globs, failure streak, and verification receipt status.",
			Annotations: ro,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Optional repository path override."},
				},
			},
		},
		{
			Name:        "dtkit_guard_verify",
			Description: "Verify cryptographic test receipt matches the current workspace digest and check for test tampering.",
			Annotations: ro,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Optional repository path override."},
				},
			},
		},
		{
			Name:        "dtkit_context_impact",
			Description: "Compute intra-repo reverse dependencies (direct/transitive dependents) and related tests across Go, TS/JS, Python, Java, and C/C++.",
			Annotations: ro,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":  map[string]any{"type": "string", "description": "Optional repository path override."},
					"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Changed file path(s) relative to repository root."},
				},
			},
		},
		{
			Name:        "dtkit_converge",
			Description: "Assess plan/spec checklist completion, referenced files on disk, and test receipt readiness strictly read-only.",
			Annotations: ro,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Optional repository path override."},
					"plan": map[string]any{"type": "string", "description": "Optional workspace-relative plan/spec markdown path."},
				},
			},
		},
	}
}

func executeTool(ctx context.Context, env paths.Environment, defaultRoot string, params toolCallParams) toolCallResult {
	targetPath := defaultRoot
	if raw, ok := params.Arguments["path"].(string); ok && strings.TrimSpace(raw) != "" {
		targetPath = strings.TrimSpace(raw)
	}

	switch params.Name {
	case "dtkit_scout":
		opts := scout.Options{Root: targetPath}
		if q, ok := params.Arguments["query"].(string); ok {
			opts.Query = q
		}
		if z, ok := params.Arguments["zone"].(string); ok && strings.TrimSpace(z) != "" {
			opts.Paths = []string{strings.TrimSpace(z)}
		}
		if p, ok := params.Arguments["pack"].(bool); ok {
			opts.IncludeContent = p
		}
		if mb, ok := params.Arguments["maxBytes"].(float64); ok && mb > 0 {
			opts.MaxBytes = int(mb)
		}
		rep, err := scout.Run(ctx, env, opts)
		return encodeToolJSON(rep, err)

	case "dtkit_project_inspect":
		res, err := project.Inspect(ctx, env, targetPath)
		return encodeToolJSON(res, err)

	case "dtkit_guard_status":
		proj, err := paths.ResolveProject(ctx, env, targetPath)
		if err != nil {
			return encodeToolJSON(nil, err)
		}
		pol, pErr := guard.LoadPolicy(proj.Root)
		if pErr != nil {
			return encodeToolJSON(nil, pErr)
		}
		ver, vErr := guard.VerifyWorkspace(proj.Root)
		if vErr != nil {
			return encodeToolJSON(nil, vErr)
		}
		st, _ := guard.LoadStreak(proj.Root)
		return encodeToolJSON(guard.StatusReport{
			Root:         proj.Root,
			Policy:       pol,
			Verification: ver,
			Streak:       st,
		}, nil)

	case "dtkit_guard_verify":
		proj, err := paths.ResolveProject(ctx, env, targetPath)
		if err != nil {
			return encodeToolJSON(nil, err)
		}
		ver, vErr := guard.VerifyWorkspace(proj.Root)
		if vErr != nil {
			return encodeToolJSON(nil, vErr)
		}
		res := encodeToolJSON(ver, nil)
		if !ver.Verified {
			res.IsError = true
		}
		return res

	case "dtkit_context_impact":
		var changed []string
		if rawList, ok := params.Arguments["paths"].([]any); ok {
			for _, item := range rawList {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					changed = append(changed, strings.TrimSpace(s))
				}
			}
		} else if single, ok := params.Arguments["file"].(string); ok && strings.TrimSpace(single) != "" {
			changed = append(changed, strings.TrimSpace(single))
		}
		rep, err := scout.AnalyzeImpact(ctx, env, targetPath, changed)
		return encodeToolJSON(rep, err)

	case "dtkit_converge":
		planPath := ""
		if p, ok := params.Arguments["plan"].(string); ok {
			planPath = strings.TrimSpace(p)
		}
		rep, err := scout.AnalyzeConvergence(ctx, env, targetPath, planPath)
		return encodeToolJSON(rep, err)

	default:
		return toolCallResult{
			IsError: true,
			Content: []toolContent{{Type: "text", Text: fmt.Sprintf("unknown tool %q", params.Name)}},
		}
	}
}

func encodeToolJSON(v any, err error) toolCallResult {
	if err != nil {
		return toolCallResult{
			IsError: true,
			Content: []toolContent{{Type: "text", Text: err.Error()}},
		}
	}
	b, mErr := json.MarshalIndent(v, "", "  ")
	if mErr != nil {
		return toolCallResult{
			IsError: true,
			Content: []toolContent{{Type: "text", Text: mErr.Error()}},
		}
	}
	return toolCallResult{
		Content: []toolContent{{Type: "text", Text: string(b)}},
	}
}
