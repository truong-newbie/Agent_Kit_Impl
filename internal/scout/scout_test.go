package scout

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestScoutPolyglotAndRedaction(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "polyrepo")

	files := map[string]string{
		"frontend/package.json":  `{"name":"web"}`,
		"frontend/tsconfig.json": `{"compilerOptions":{"strict":true}}`,
		"frontend/src/app.ts":    "export const apiUrl = 'https://example.test';\nconst token = \"ghp_1234567890abcdefghijklmnop\";\n",
		"backend/pom.xml":        "<project></project>\n",
		"backend/Auth.java":      "public class Auth {}\n",
		"ai/pyproject.toml":      "[project]\nname = \"ai\"\n",
		"ai/worker.py":           "API_KEY=\"sk-abcdef1234567890abcd\"\ndef run(): pass\n",
		"api/openapi.yaml":       "openapi: 3.1.0\n",
		".env":                   "SECRET_KEY=must-never-appear\n",
	}
	for rel, body := range files {
		full := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}

	rep, err := Run(context.Background(), env, Options{Root: repo, IncludeContent: true})
	if err != nil {
		t.Fatalf("scout.Run failed: %v", err)
	}
	if rep.LanguageStats["typescript"] != 1 || rep.LanguageStats["java"] != 1 || rep.LanguageStats["python"] != 1 {
		t.Fatalf("unexpected language stats: %+v", rep.LanguageStats)
	}
	if len(rep.Contracts) != 1 || rep.Contracts[0] != "api/openapi.yaml" {
		t.Fatalf("unexpected contracts: %+v", rep.Contracts)
	}
	for _, f := range rep.MatchedFiles {
		if f.RelPath == ".env" {
			t.Fatal(".env must never be scanned or returned")
		}
		if strings.Contains(f.Snippet, "ghp_") || strings.Contains(f.Snippet, "sk-abcdef") {
			t.Fatalf("secret was not redacted in %s: %s", f.RelPath, f.Snippet)
		}
	}

	// Query filter & zone filter
	qRep, err := Run(context.Background(), env, Options{Root: repo, Query: "Auth", Paths: []string{"backend"}})
	if err != nil {
		t.Fatalf("query scout failed: %v", err)
	}
	if len(qRep.MatchedFiles) != 1 || qRep.MatchedFiles[0].RelPath != "backend/Auth.java" {
		t.Fatalf("expected backend/Auth.java, got %+v", qRep.MatchedFiles)
	}

	// Write summary
	md := FormatMarkdownSummary(rep)
	outPath, err := WriteCodebaseSummary(context.Background(), env, repo, md)
	if err != nil {
		t.Fatalf("WriteCodebaseSummary failed: %v", err)
	}
	raw, err := os.ReadFile(outPath)
	if err != nil || !strings.Contains(string(raw), "# Codebase Summary") || !strings.Contains(string(raw), "api/openapi.yaml") {
		t.Fatalf("unexpected codebase summary content: %v\n%s", err, string(raw))
	}
}
