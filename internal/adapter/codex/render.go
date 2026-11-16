// Package codex renders DTKit content for Codex CLI without modifying the filesystem.
package codex

import (
	"io/fs"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const AdapterRevision = "1"

type Artifact = adapter.Artifact

var validateSkill = adapter.ValidateSkill

// Render emits logical paths only. Applying them requires a separate safe installer.
func Render(p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	return adapter.RenderForRuntime("codex", p, source, scope)
}
