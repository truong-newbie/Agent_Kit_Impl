package schemas

import (
	"encoding/json"
	"io/fs"
	"testing"
)

func TestEmbeddedSchemasAreValidJSON(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 schema files, got %d", len(entries))
	}
	for _, entry := range entries {
		b, err := fs.ReadFile(FS, entry.Name())
		if err != nil {
			t.Fatalf("%s: %v", entry.Name(), err)
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatalf("%s invalid json: %v", entry.Name(), err)
		}
	}
}
