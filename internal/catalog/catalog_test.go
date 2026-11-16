package catalog

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedEngineer(t *testing.T) {
	p, files, err := Engineer()
	if err != nil { t.Fatal(err) }
	if p.ID != "engineer" || len(p.Components) != 14 { t.Fatalf("unexpected package: %+v", p) }
	for _, c := range p.Components {
		if c.Kind == "skill" {
			b, err := fs.ReadFile(files, c.Source+"/SKILL.md.tmpl")
			if err != nil { t.Fatal(err) }
			if !strings.Contains(string(b), "name: {{.SkillName}}") || !strings.Contains(string(b), "description:") { t.Fatalf("invalid skill template %s", c.ID) }
		}
	}
}

func TestValidationRejectsMissingAndCyclicDependencies(t *testing.T) {
	f := fstest.MapFS{"a": &fstest.MapFile{Data: []byte("content")}}
	base := Package{SchemaVersion: 1, ID: "engineer", Name: "Engineer", Version: "0.1.0"}
	for _, tc := range []struct { name string; components []Component; want string }{
		{"missing", []Component{{ID: "rule/a", Kind: "rule", Source: "a", Requires: []string{"rule/b"}}}, "missing dependency"},
		{"cycle", []Component{{ID: "rule/a", Kind: "rule", Source: "a", Requires: []string{"rule/b"}}, {ID: "rule/b", Kind: "rule", Source: "a", Requires: []string{"rule/a"}}}, "dependency cycle"},
		{"escape", []Component{{ID: "rule/a", Kind: "rule", Source: "../a"}}, "invalid component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base; p.Components = tc.components
			if err := Validate(p, f); err == nil || !strings.Contains(err.Error(), tc.want) { t.Fatalf("got %v, want %s", err, tc.want) }
		})
	}
}
