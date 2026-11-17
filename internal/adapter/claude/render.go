// Package claude renders DTKit content for Claude Code without modifying the filesystem.
package claude

import (
	"io/fs"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const AdapterRevision = "1"

type Artifact = adapter.Artifact

// Render emits logical Claude Code paths without modifying the filesystem.
func Render(p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	return adapter.RenderForRuntime("claude", p, source, scope)
}
