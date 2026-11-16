package catalog

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestStrictJSON(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		ok          bool
	}{
		{"object", `{"outer":{"key":1},"list":[1,true,null]}`, true},
		{"duplicate", `{"outer":{"key":1,"key":2}}`, false},
		{"trailing", `{} {}`, false},
		{"broken", `{"key":`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckStrictJSON([]byte(tc.input)); (err == nil) != tc.ok {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestPackageTreeRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"CON.txt", "file:stream", "trailing.", "one\\two", "script.sh", "run.ps1", "bin.exe", ".git/config", "node_modules/a.md"} {
		t.Run(name, func(t *testing.T) {
			f := fstest.MapFS{name: &fstest.MapFile{Data: []byte("x")}}
			if err := checkTree(f); err == nil {
				t.Fatalf("expected rejection of %s", name)
			}
		})
	}
	for _, content := range [][]byte{[]byte("#!/bin/sh\necho hi\n"), []byte("\x7fELF"), []byte("MZ\x90\x00"), []byte("\xff")} {
		if err := checkContentSafety("tool", content); err == nil {
			t.Fatalf("expected rejection of unsafe content %q", content)
		}
	}
	f := fstest.MapFS{"A.txt": &fstest.MapFile{}, "a.txt": &fstest.MapFile{}}
	if err := checkTree(f); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("expected collision, got %v", err)
	}
}

func TestLocalManifestDoesNotAcceptDuplicateKeys(t *testing.T) {
	f := fstest.MapFS{"package.json": &fstest.MapFile{Data: []byte(`{"id":"first","id":"second"}`)}}
	if _, err := Load(f); err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestRejectsWrongCaseMissingAndNullFields(t *testing.T) {
	for _, input := range []string{
		`{"SchemaVersion":1}`,
		`{"schemaVersion":1}`,
		`{"schemaVersion":1,"id":"engineer","name":"x","version":"0.1.0","description":"x","runtimeTargets":["codex"],"scopes":["project","global"],"requiresCapabilities":["instructions","skills"],"packageDependencies":null,"components":[]}`,
	} {
		if err := checkManifestKeys([]byte(input)); err == nil {
			t.Fatalf("accepted invalid manifest: %s", input)
		}
	}
}
