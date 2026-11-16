// Package gemini renders DTKit content for Gemini CLI without modifying the filesystem.
package gemini

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const AdapterRevision = "1"

type Artifact = adapter.Artifact

// Render emits logical Gemini CLI paths without modifying the filesystem.
func Render(p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	return adapter.RenderForRuntime("gemini", p, source, scope)
}

// InspectSettingsContextOverride inspects .gemini/settings.json for custom context.fileName
// values that would exclude GEMINI.md from Gemini CLI's context loading.
func InspectSettingsContextOverride(settingsPath string) (overridden bool, warning string, err error) {
	info, statErr := os.Lstat(settingsPath)
	if os.IsNotExist(statErr) {
		return false, "", nil
	}
	if statErr != nil {
		return false, "", statErr
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return true, fmt.Sprintf("%s is not a regular file", settingsPath), nil
	}
	if info.Size() > 1<<20 {
		return false, fmt.Sprintf("%s exceeds 1 MiB limit", settingsPath), nil
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return false, "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, fmt.Sprintf("%s contains invalid JSON (%v)", filepath.Base(settingsPath), err), nil
	}
	ctxVal, ok := doc["context"]
	if !ok {
		return false, "", nil
	}
	ctxMap, ok := ctxVal.(map[string]any)
	if !ok {
		return false, "", nil
	}
	fileNameVal, ok := ctxMap["fileName"]
	if !ok {
		return false, "", nil
	}
	switch v := fileNameVal.(type) {
	case string:
		if strings.TrimSpace(v) != "" && v != "GEMINI.md" {
			return true, fmt.Sprintf("%s overrides context.fileName to %q (GEMINI.md ignored unless included)", settingsPath, v), nil
		}
	case []any:
		found := false
		for _, item := range v {
			if s, ok := item.(string); ok && s == "GEMINI.md" {
				found = true
				break
			}
		}
		if len(v) > 0 && !found {
			return true, fmt.Sprintf("%s sets context.fileName without GEMINI.md", settingsPath), nil
		}
	}
	return false, "", nil
}
