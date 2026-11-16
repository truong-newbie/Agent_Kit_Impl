// Package schemas embeds machine-readable JSON schemas for DTKit.
package schemas

import "embed"

//go:embed *.schema.json
var FS embed.FS
