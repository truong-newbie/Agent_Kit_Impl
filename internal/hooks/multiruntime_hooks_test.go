package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGeminiCursorOpenCodeHooksAndSharedRunner(t *testing.T) {
	// 1. Gemini CLI: settings.json merge preserves context.fileName and uses ms timeout.
	seedGemini := []byte("{\n  \"context\": {\"fileName\": \"GEMINI.md\"},\n  \"mcpServers\": {\"local\": {}}\n}\n")
	gemEntry := BuildGeminiHookEntry("dtkit-check-changed", "'/usr/local/bin/dtkit' hook run check-changed --runtime gemini", "write_file|replace", 15)
	mergedGem, digGem, err := MergeEventHookJSON(seedGemini, true, EventAfterTool, gemEntry)
	if err != nil {
		t.Fatalf("Gemini MergeEventHookJSON failed: %v", err)
	}
	if !strings.Contains(string(mergedGem), `"GEMINI.md"`) || !strings.Contains(string(mergedGem), `"timeout": 15000`) {
		t.Fatalf("unexpected Gemini settings.json:\n%s", string(mergedGem))
	}
	unmergedGem, delGem, err := UnmergeEventHookJSON(mergedGem, EventAfterTool, digGem, false)
	if err != nil || delGem || !strings.Contains(string(unmergedGem), `"GEMINI.md"`) || strings.Contains(string(unmergedGem), "AfterTool") {
		t.Fatalf("unexpected Gemini unmerge: del=%v err=%v\n%s", delGem, err, string(unmergedGem))
	}

	// Gemini BeforeTool guard-edit returns official {"decision":"deny"|"allow"}.
	proj := t.TempDir()
	var gOut, gErr bytes.Buffer
	if err := RunGuardEditForRuntime(context.Background(), strings.NewReader(`{"hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"file_path":".env"}}`), &gOut, &gErr, "gemini", "project", proj); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gOut.String(), `"decision":"deny"`) {
		t.Fatalf("expected Gemini deny output for .env, got %s", gOut.String())
	}
	gOut.Reset()
	if err := RunGuardEditForRuntime(context.Background(), strings.NewReader(`{"hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"file_path":"main.go"}}`), &gOut, &gErr, "gemini", "project", proj); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gOut.String(), `"decision":"allow"`) {
		t.Fatalf("expected Gemini allow output for main.go, got %s", gOut.String())
	}

	// 2. Cursor: .cursor/hooks.json enforces version: 1 and preserves existing user hooks.
	seedCursor := []byte("{\n  \"version\": 1,\n  \"hooks\": {\n    \"afterFileEdit\": [{\"command\": \"./user-fmt.sh\"}]\n  }\n}\n")
	curEntry := BuildCursorHookEntry("'/usr/local/bin/dtkit' hook run check-changed --runtime cursor")
	mergedCur, digCur, err := MergeCursorHooksJSON(seedCursor, true, EventAfterFileEdit, curEntry)
	if err != nil {
		t.Fatalf("Cursor merge failed: %v", err)
	}
	if !strings.Contains(string(mergedCur), `"version": 1`) || !strings.Contains(string(mergedCur), `./user-fmt.sh`) {
		t.Fatalf("unexpected Cursor hooks.json:\n%s", string(mergedCur))
	}
	unmergedCur, delCur, err := UnmergeCursorHooksJSON(mergedCur, EventAfterFileEdit, digCur, false)
	if err != nil || delCur || !strings.Contains(string(unmergedCur), `./user-fmt.sh`) || !strings.Contains(string(unmergedCur), `"version": 1`) {
		t.Fatalf("unexpected Cursor unmerge: del=%v err=%v\n%s", delCur, err, string(unmergedCur))
	}

	// Unsupported Cursor version: 2 rejects cleanly.
	if _, _, err := MergeCursorHooksJSON([]byte(`{"version":2,"hooks":{}}`), true, EventAfterFileEdit, curEntry); err == nil {
		t.Fatal("expected unsupported Cursor hooks.json version error")
	}

	// 3. OpenCode: standalone JS plugin uses safe execFileSync JSON arrays without shell interpolation.
	exeSpaced := filepath.Join(t.TempDir(), "Program Files", "dtkit's bin", "dtkit")
	pluginBytes, pluginDig, err := RenderOpenCodePlugin(exeSpaced, "project", proj, 15)
	if err != nil || len(pluginDig) != 64 {
		t.Fatalf("RenderOpenCodePlugin failed: err=%v dig=%q", err, pluginDig)
	}
	if !strings.Contains(string(pluginBytes), "execFileSync") || !strings.Contains(string(pluginBytes), `"tool.execute.after"`) || !strings.Contains(string(pluginBytes), "DTKIT_HOOK_ACTIVE") {
		t.Fatalf("unexpected OpenCode plugin content:\n%s", string(pluginBytes))
	}

	// 4. Shared RunCheckChangedForRuntime formats outputs per runtime and honors recursion guard & untrusted trust boundary.
	if err := os.MkdirAll(filepath.Join(proj, ".dtkit"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".dtkit", "project.json"), []byte(`{"schemaVersion":1,"roots":[{"path":".","stack":"go"}],"checks":[{"id":"t","kind":"test","executable":"go","argv":["version"],"cwd":".","timeoutSec":10}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)
	for _, rt := range []string{"gemini", "cursor", "opencode"} {
		var out, errBuf bytes.Buffer
		if err := runCheckChangedForRuntimeWithNow(context.Background(), nil, &out, &errBuf, rt, "project", proj, func() time.Time { return now }); err != nil {
			t.Fatalf("%s check-changed failed: %v", rt, err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("%s output invalid JSON: %v (%s)", rt, err, out.String())
		}
		if rt == "gemini" && parsed["decision"] != "allow" {
			t.Fatalf("expected gemini decision=allow, got %+v", parsed)
		}
		if (rt == "cursor" || rt == "opencode") && !strings.Contains(out.String(), "dtkit check --yes") {
			t.Fatalf("expected untrusted reminder in %s output, got %s", rt, out.String())
		}
	}

	// Recursion guard skips checks cleanly when DTKIT_HOOK_ACTIVE=1.
	t.Setenv("DTKIT_HOOK_ACTIVE", "1")
	var recOut bytes.Buffer
	if err := runCheckChangedForRuntimeWithNow(context.Background(), nil, &recOut, &bytes.Buffer{}, "cursor", "project", proj, func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(recOut.String(), "dtkit check") {
		t.Fatalf("expected DTKIT_HOOK_ACTIVE=1 loop guard skip, got %s", recOut.String())
	}
}
