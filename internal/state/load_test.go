package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileRejectsCorruptDuplicateAndFutureState(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{"duplicate_key", `{"schemaVersion":1,"schemaVersion":1}`, "duplicate key"},
		{"unknown_field", `{"schemaVersion":1,"extra":true}`, "unknown field"},
		{"future_version", `{"schemaVersion":99,"managedBy":"dtkit","generation":1,"scope":"project","runtime":"codex","adapterRevision":"1","kit":{"id":"engineer","version":"0.1.0","digest":"` + strings.Repeat("a", 64) + `"},"artifacts":[{"root":"project","path":"AGENTS.md","strategy":"managed-block","digest":"` + strings.Repeat("a", 64) + `","block":{"digest":"` + strings.Repeat("a", 64) + `","separator":"","createdFile":true}}],"installedAt":"2026-11-16T00:00:00Z","updatedAt":"2026-11-16T00:00:00Z"}`, "unsupported"},
		{"bad_allowlist", `{"schemaVersion":1,"managedBy":"dtkit","generation":1,"scope":"project","runtime":"codex","adapterRevision":"1","kit":{"id":"engineer","version":"0.1.0","digest":"` + strings.Repeat("a", 64) + `"},"artifacts":[{"root":"project","path":"../../etc/passwd","strategy":"whole-file","digest":"` + strings.Repeat("a", 64) + `"}],"installedAt":"2026-11-16T00:00:00Z","updatedAt":"2026-11-16T00:00:00Z"}`, "allowlist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(p, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			_, exists, err := LoadFile(p, "project")
			if !exists || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got exists=%v err=%v, want %q", exists, err, tc.want)
			}
		})
	}
}
