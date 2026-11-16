// Package cursor renders DTKit content for Cursor without modifying the filesystem.
package cursor

import (
	"io/fs"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const AdapterRevision = "1"

type Artifact = adapter.Artifact

// Render emits logical Cursor paths without modifying the filesystem.
func Render(p catalog.Package, source fs.FS, scope string) ([]Artifact, error) {
	return adapter.RenderForRuntime("cursor", p, source, scope)
}
