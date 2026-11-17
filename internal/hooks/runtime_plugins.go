package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

// RenderOpenCodePlugin generates a standalone OpenCode JS module plugin invoking DTKit's
// bounded quality runner on tool.execute.after using execFileSync (no shell interpolation).
func RenderOpenCodePlugin(exePath, scope, stateRoot string, timeoutSec int) (content []byte, digest string, err error) {
	if !filepath.IsAbs(exePath) || !filepath.IsAbs(stateRoot) {
		return nil, "", apperr.Usage("invalid_hook_paths", "plugin executable and root must be absolute paths")
	}
	if scope != "project" && scope != "global" {
		return nil, "", apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if timeoutSec <= 0 {
		timeoutSec = 15
	}
	exeJSON, _ := json.Marshal(exePath)
	scopeJSON, _ := json.Marshal(scope)
	rootJSON, _ := json.Marshal(stateRoot)

	script := fmt.Sprintf(`// Managed by DTKit. Do not edit manually; remove via 'dtkit hooks disable check-changed --runtime opencode'.
import { execFileSync } from "node:child_process";

const DTKIT_BIN = %s;
const DTKIT_SCOPE = %s;
const DTKIT_ROOT = %s;
const EDIT_TOOLS = new Set(["edit", "write", "patch", "multiedit"]);

export const DtkitQualityPlugin = async () => {
  return {
    tool: {
      execute: {
        after: async (input, output) => {
          const toolName = String(input?.tool || "").toLowerCase();
          if (!EDIT_TOOLS.has(toolName) || process.env.DTKIT_HOOK_ACTIVE === "1") {
            return;
          }
          try {
            const payload = JSON.stringify({
              hook_event_name: "tool.execute.after",
              tool_name: toolName,
              tool_input: { file_path: String(input?.args?.filePath || input?.args?.file || input?.args?.path || "") }
            });
            const stdout = execFileSync(
              DTKIT_BIN,
              ["hook", "run", "check-changed", "--runtime", "opencode", "--scope", DTKIT_SCOPE, "--root", DTKIT_ROOT],
              {
                input: payload,
                encoding: "utf8",
                timeout: %d,
                env: { ...process.env, DTKIT_HOOK_ACTIVE: "1" }
              }
            );
            if (stdout && stdout.trim()) {
              const parsed = JSON.parse(stdout);
              if (parsed?.additionalContext && output && typeof output.output === "string") {
                output.output = output.output + "\n\n" + parsed.additionalContext;
              }
            }
          } catch {
            // Never crash user session if binary is missing or checks fail; check output surfaces explicitly when run.
          }
        }
      }
    }
  };
};
`, string(exeJSON), string(scopeJSON), string(rootJSON), timeoutSec*1000)

	content = []byte(script)
	sum := sha256.Sum256(content)
	return content, hex.EncodeToString(sum[:]), nil
}

// DigestBytes returns lowercase hex SHA-256 of plugin bytes.
func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
