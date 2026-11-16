package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseWorkflowSecurityAndSmokeContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release.yml: %v", err)
	}
	yml := string(raw)

	// Disallow direct shell interpolation of workflow_dispatch inputs inside run blocks.
	if strings.Contains(yml, "${{ inputs.version }}") {
		// Verify inputs.version is only ever mapped into env vars, never inside run shell strings.
		for _, line := range strings.Split(yml, "\n") {
			if strings.Contains(line, "inputs.version") && !strings.Contains(line, "INPUT_VERSION:") {
				t.Fatalf("unsafe direct interpolation or unexpected usage of inputs.version: %s", strings.TrimSpace(line))
			}
		}
	}
	for _, required := range []string{"INPUT_VERSION:", "SEMVER_RE=", "sha256sum -c SHA256SUMS", "native-smoke:", "draft-release:", "--draft"} {
		if !strings.Contains(yml, required) {
			t.Fatalf("release.yml missing hardened requirement %q", required)
		}
	}
}
