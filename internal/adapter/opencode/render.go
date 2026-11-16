// Package opencode renders DTKit content for OpenCode without modifying the filesystem.
package opencode

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const AdapterRevision = "1"

type Artifact = adapter.Artifact

// Render emits logical OpenCode paths without modifying the filesystem.
func Render(p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	return adapter.RenderForRuntime("opencode", p, source, scope)
}

// InspectConfigWarnings validates strict JSON OpenCode configuration files (if present)
// because OpenCode refuses to start when opencode.json is malformed.
func InspectConfigWarnings(candidates []string) []string {
	var warnings []string
	for _, cfgPath := range candidates {
		info, err := os.Lstat(cfgPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", cfgPath, err))
			continue
		}
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			warnings = append(warnings, fmt.Sprintf("%s is not a regular file", cfgPath))
			continue
		}
		if info.Size() > 1<<20 {
			warnings = append(warnings, fmt.Sprintf("%s exceeds 1 MiB limit", cfgPath))
			continue
		}
		b, err := os.ReadFile(cfgPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", cfgPath, err))
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal(b, &parsed); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s contains invalid JSON (%v); OpenCode fails startup on invalid %s", cfgPath, err, filepath.Base(cfgPath)))
		}
	}
	return warnings
}
