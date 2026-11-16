package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoldenStatesCompatibilityAcrossVersionsAndRuntimes(t *testing.T) {
	digA := strings.Repeat("a", 64)
	digB := strings.Repeat("b", 64)
	dir := t.TempDir()

	// Exact Codex v1 state format written by main (f81ce6e) with engineer@0.1.0.
	mainCodexV1Project := `{
  "schemaVersion": 1,
  "managedBy": "dtkit",
  "generation": 1,
  "scope": "project",
  "runtime": "codex",
  "adapterRevision": "1",
  "kit": {
    "id": "engineer",
    "version": "0.1.0",
    "digest": "88bc2c800d3b610b932b65a8b551e9d3686506d17e7a9c6056a75042da42245b",
    "sourceKind": "bundled"
  },
  "artifacts": [
    {
      "root": "project",
      "path": "AGENTS.md",
      "strategy": "managed-block",
      "digest": "43a0311e2edf63ad182c877827a2b4f14b3fe8404596961e605867ca54651902",
      "block": {
        "digest": "43a0311e2edf63ad182c877827a2b4f14b3fe8404596961e605867ca54651902",
        "separator": "",
        "createdFile": false
      }
    },
    {
      "root": "project",
      "path": ".agents/skills/dtkit-project-plan/SKILL.md",
      "strategy": "whole-file",
      "digest": "` + digA + `"
    }
  ],
  "lastTransactionID": "08aa63bb6568252bca571814d3567c16",
  "installedAt": "2026-11-16T10:43:38Z",
  "updatedAt": "2026-11-16T10:43:38Z"
}`

	absCodexHome := filepath.Join(dir, "codex-home")
	absGeminiHome := filepath.Join(dir, "gemini-home")
	absOCHome := filepath.Join(dir, "opencode-home")
	absExe := filepath.Join(dir, "bin", "dtkit")

	codexV2GlobalWithHook := `{
  "schemaVersion": 2,
  "managedBy": "dtkit",
  "generation": 2,
  "scope": "global",
  "runtime": "codex",
  "adapterRevision": "1",
  "codexHome": ` + jsonString(absCodexHome) + `,
  "kit": {
    "id": "engineer",
    "version": "0.1.0",
    "digest": "` + digA + `",
    "sourceKind": "bundled"
  },
  "artifacts": [
    {
      "root": "codex-home",
      "path": "AGENTS.md",
      "strategy": "managed-block",
      "digest": "` + digB + `",
      "block": {"digest": "` + digB + `", "separator": "", "createdFile": true}
    }
  ],
  "hooks": [
    {
      "id": "session-context",
      "event": "SessionStart",
      "handlerId": "session-context",
      "root": "codex-home",
      "path": "hooks.json",
      "command": "'/usr/local/bin/dtkit' hook run session-context --scope global --root '/tmp/home'",
      "commandWindows": "& 'C:\\dtkit\\dtkit.exe' hook run session-context --scope global --root 'C:\\home'",
      "executable": ` + jsonString(absExe) + `,
      "entryDigest": "` + digB + `",
      "timeoutSec": 3,
      "createdFile": true
    }
  ],
  "installedAt": "2026-11-16T10:00:00Z",
  "updatedAt": "2026-11-16T10:05:00Z"
}`

	geminiGlobal := `{
  "schemaVersion": 1,
  "managedBy": "dtkit",
  "generation": 1,
  "scope": "global",
  "runtime": "gemini",
  "adapterRevision": "1",
  "runtimeHome": ` + jsonString(absGeminiHome) + `,
  "kit": {"id": "engineer", "version": "0.2.0", "digest": "` + digA + `", "sourceKind": "bundled"},
  "artifacts": [
    {"root": "gemini-home", "path": "GEMINI.md", "strategy": "managed-block", "digest": "` + digB + `", "block": {"digest": "` + digB + `", "separator": "", "createdFile": true}},
    {"root": "gemini-home", "path": "skills/dtkit-gemini-global-plan/SKILL.md", "strategy": "whole-file", "digest": "` + digA + `"}
  ],
  "installedAt": "2026-11-16T11:00:00Z",
  "updatedAt": "2026-11-16T11:00:00Z"
}`

	opencodeGlobal := `{
  "schemaVersion": 1,
  "managedBy": "dtkit",
  "generation": 1,
  "scope": "global",
  "runtime": "opencode",
  "adapterRevision": "1",
  "runtimeHome": ` + jsonString(absOCHome) + `,
  "kit": {"id": "engineer", "version": "0.2.0", "digest": "` + digA + `", "sourceKind": "bundled"},
  "artifacts": [
    {"root": "opencode-home", "path": "AGENTS.md", "strategy": "managed-block", "digest": "` + digB + `", "block": {"digest": "` + digB + `", "separator": "", "createdFile": true}},
    {"root": "opencode-home", "path": "skills/dtkit-opencode-global-plan/SKILL.md", "strategy": "whole-file", "digest": "` + digA + `"}
  ],
  "installedAt": "2026-11-16T11:10:00Z",
  "updatedAt": "2026-11-16T11:10:00Z"
}`

	cursorProject := `{
  "schemaVersion": 1,
  "managedBy": "dtkit",
  "generation": 1,
  "scope": "project",
  "runtime": "cursor",
  "adapterRevision": "1",
  "kit": {"id": "engineer", "version": "0.2.0", "digest": "` + digA + `", "sourceKind": "bundled"},
  "artifacts": [
    {"root": "project", "path": ".cursor/rules/dtkit-engineer.mdc", "strategy": "whole-file", "digest": "` + digB + `"},
    {"root": "project", "path": ".cursor/skills/dtkit-cursor-project-plan/SKILL.md", "strategy": "whole-file", "digest": "` + digA + `"}
  ],
  "installedAt": "2026-11-16T11:20:00Z",
  "updatedAt": "2026-11-16T11:20:00Z"
}`

	cursorGlobal := `{
  "schemaVersion": 1,
  "managedBy": "dtkit",
  "generation": 1,
  "scope": "global",
  "runtime": "cursor",
  "adapterRevision": "1",
  "kit": {"id": "engineer", "version": "0.2.0", "digest": "` + digA + `", "sourceKind": "bundled"},
  "artifacts": [
    {"root": "user-home", "path": ".cursor/skills/dtkit-cursor-global-plan/SKILL.md", "strategy": "whole-file", "digest": "` + digA + `"}
  ],
  "installedAt": "2026-11-16T11:30:00Z",
  "updatedAt": "2026-11-16T11:30:00Z"
}`

	for _, tc := range []struct {
		name    string
		scope   string
		runtime string
		raw     string
	}{
		{"main_codex_v1_project", "project", "codex", mainCodexV1Project},
		{"codex_v2_global_hooks", "global", "codex", codexV2GlobalWithHook},
		{"gemini_global", "global", "gemini", geminiGlobal},
		{"opencode_global", "global", "opencode", opencodeGlobal},
		{"cursor_project", "project", "cursor", cursorProject},
		{"cursor_global", "global", "cursor", cursorGlobal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(p, []byte(tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			inst, exists, err := LoadFileForRuntime(p, tc.scope, tc.runtime)
			if !exists || err != nil {
				t.Fatalf("expected compatible state, got exists=%v err=%v", exists, err)
			}
			if inst.Runtime != tc.runtime {
				t.Fatalf("runtime mismatch: got %q", inst.Runtime)
			}
		})
	}

	// Unknown / future schema versions produce explicit actionable error messages.
	for _, bad := range []struct {
		name    string
		raw     string
		wantSub string
	}{
		{"schema_99", strings.Replace(mainCodexV1Project, `"schemaVersion": 1`, `"schemaVersion": 99`, 1), "schemaVersion 99"},
		{"schema_0", strings.Replace(mainCodexV1Project, `"schemaVersion": 1`, `"schemaVersion": 0`, 1), "schemaVersion 0"},
		{"future_adapter", strings.Replace(mainCodexV1Project, `"adapterRevision": "1"`, `"adapterRevision": "9"`, 1), "unsupported or invalid installation metadata"},
	} {
		t.Run(bad.name, func(t *testing.T) {
			if _, err := ParseAndValidate([]byte(bad.raw), "project"); err == nil || !strings.Contains(err.Error(), bad.wantSub) {
				t.Fatalf("expected %q in error, got %v", bad.wantSub, err)
			}
		})
	}
}

func jsonString(s string) string {
	b := strings.ReplaceAll(s, `\`, `\\`)
	b = strings.ReplaceAll(b, `"`, `\"`)
	return `"` + b + `"`
}
