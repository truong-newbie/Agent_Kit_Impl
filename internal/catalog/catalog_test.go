package catalog

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedEngineer(t *testing.T) {
	p, files, err := Engineer()
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "engineer" || len(p.Components) != 19 {
		t.Fatalf("unexpected package: %+v", p)
	}
	for _, c := range p.Components {
		if c.Kind == "skill" {
			b, err := fs.ReadFile(files, c.Source+"/SKILL.md.tmpl")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), "name: {{.SkillName}}") || !strings.Contains(string(b), "description:") {
				t.Fatalf("invalid skill template %s", c.ID)
			}
		}
	}
}

func TestValidationRejectsMissingAndCyclicDependencies(t *testing.T) {
	f := fstest.MapFS{"a": &fstest.MapFile{Data: []byte("content")}, "b": &fstest.MapFile{Data: []byte("content")}}
	base := Package{SchemaVersion: 1, ID: "engineer", Name: "Engineer", Version: "0.1.0", Description: "Test", RuntimeTargets: []string{"codex"}, Scopes: []string{"project", "global"}, RequiresCapabilities: []string{"instructions", "skills"}}
	for _, tc := range []struct {
		name       string
		components []Component
		want       string
	}{
		{"missing", []Component{{ID: "rule/a", Kind: "rule", Source: "a", Requires: []string{"rule/b"}}}, "missing dependency"},
		{"cycle", []Component{{ID: "rule/a", Kind: "rule", Source: "a", Requires: []string{"rule/b"}}, {ID: "rule/b", Kind: "rule", Source: "b", Requires: []string{"rule/a"}}}, "rule/a -> rule/b -> rule/a"},
		{"escape", []Component{{ID: "rule/a", Kind: "rule", Source: "../a"}}, "invalid component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			p.Components = tc.components
			if err := Validate(p, f); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
		})
	}
}

func TestSkillCannotHaveBothTemplateAndStaticFile(t *testing.T) {
	f := fstest.MapFS{
		"skills/plan/SKILL.md":      &fstest.MapFile{Data: []byte("---\nname: dtkit-project-plan\ndescription: plan\n---\nbody")},
		"skills/plan/SKILL.md.tmpl": &fstest.MapFile{Data: []byte("---\nname: {{.SkillName}}\ndescription: plan\n---\nbody")},
	}
	p := Package{
		SchemaVersion:        1,
		ID:                   "engineer",
		Name:                 "Engineer",
		Version:              "0.1.0",
		Description:          "Test",
		RuntimeTargets:       []string{"codex"},
		Scopes:               []string{"project", "global"},
		RequiresCapabilities: []string{"instructions", "skills"},
		Components:           []Component{{ID: "skill/plan", Kind: "skill", Source: "skills/plan"}},
	}
	if err := Validate(p, f); err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("expected ambiguous skill error, got %v", err)
	}
}
