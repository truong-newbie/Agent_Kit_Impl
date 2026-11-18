package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/guard"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestMCPServeInitializeListAndToolsCall(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "backend", "pom.xml"), []byte("<project></project>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "backend", "Auth.java"), []byte("public class Auth {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs", "specs"), 0755); err != nil {
		t.Fatal(err)
	}
	specBody := "# Feature Specification: Auth\n\n### User Story 1 — Login (Priority: P1)\n- **Given** valid credentials, **When** logging in, **Then** return token.\n\n- **FR-001:** System MUST issue token.\n- **SC-001:** Under 100ms.\n"
	if err := os.WriteFile(filepath.Join(repo, "docs", "specs", "auth.md"), []byte(specBody), 0644); err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordCheckPass(repo, []string{"backend-maven-test"}, time.Now()); err != nil {
		t.Fatal(err)
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}

	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"dtkit_scout","arguments":{"query":"Auth"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"dtkit_project_inspect","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"dtkit_guard_verify","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"dtkit_spec_check","arguments":{"spec":"docs/specs/auth.md"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"unknown/method"}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := Serve(context.Background(), env, repo, strings.NewReader(in), &out); err != nil {
		t.Fatalf("Serve failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("expected 7 JSON-RPC responses (notification ignored), got %d:\n%s", len(lines), out.String())
	}

	var initResp map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &initResp); err != nil {
		t.Fatal(err)
	}
	resMap := initResp["result"].(map[string]any)
	if resMap["protocolVersion"] != ProtocolVersion {
		t.Fatalf("unexpected protocolVersion: %v", resMap["protocolVersion"])
	}

	var listResp map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &listResp); err != nil {
		t.Fatal(err)
	}
	tools := listResp["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}
	for _, rawTool := range tools {
		tm := rawTool.(map[string]any)
		ann, ok := tm["annotations"].(map[string]any)
		if !ok || ann["readOnlyHint"] != true || ann["destructiveHint"] != false || ann["idempotentHint"] != true {
			t.Fatalf("expected readOnly/idempotent annotations on tool %v, got %v", tm["name"], tm["annotations"])
		}
	}

	if !strings.Contains(lines[2], "backend/Auth.java") {
		t.Fatalf("expected dtkit_scout response to contain backend/Auth.java: %s", lines[2])
	}
	if !strings.Contains(lines[3], `\"stack\": \"java\"`) {
		t.Fatalf("expected dtkit_project_inspect response to contain java stack: %s", lines[3])
	}
	if !strings.Contains(lines[4], `"verified": true`) && !strings.Contains(lines[4], `\"verified\": true`) {
		t.Fatalf("expected dtkit_guard_verify response to be verified: %s", lines[4])
	}
	if !strings.Contains(lines[5], "FR-001") || !strings.Contains(lines[5], "valid") {
		t.Fatalf("expected dtkit_spec_check response to report valid + FR-001: %s", lines[5])
	}
	if !strings.Contains(lines[6], "-32601") {
		t.Fatalf("expected -32601 for unknown method: %s", lines[6])
	}
}
