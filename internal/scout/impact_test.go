package scout

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestAnalyzeImpactAcrossGoTSPythonJavaCpp(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")

	files := map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.27\n",
		"pkg/core/core.go": `package core
func Calc() int { return 1 }
`,
		"pkg/core/core_test.go": `package core
import "testing"
func TestCalc(t *testing.T) {}
`,
		"pkg/api/handler.go": `package api
import "example.com/app/pkg/core"
func Handle() int { return core.Calc() }
`,
		"cmd/server/main.go": `package main
import "example.com/app/pkg/api"
func main() { _ = api.Handle() }
`,
		"frontend/src/api.ts":                           "export const fetchOrder = () => 1;\n",
		"frontend/src/checkout.ts":                      "import { fetchOrder } from './api';\nexport const run = () => fetchOrder();\n",
		"frontend/src/api.test.ts":                      "import { fetchOrder } from './api';\n",
		"ai/utils.py":                                   "def clean(): return True\n",
		"ai/pipeline.py":                                "from utils import clean\n",
		"ai/test_utils.py":                              "from utils import clean\n",
		"backend/src/main/java/com/acme/Token.java":     "package com.acme;\npublic class Token {}\n",
		"backend/src/main/java/com/acme/Auth.java":      "package com.acme;\nimport com.acme.Token;\npublic class Auth {}\n",
		"backend/src/test/java/com/acme/TokenTest.java": "package com.acme;\npublic class TokenTest {}\n",
		"core/include/math.h":                           "int add(int a, int b);\n",
		"core/src/calc.cpp":                             "#include \"math.h\"\nint add(int a, int b) { return a + b; }\n",
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

	// 1. Go transitive impact: pkg/core/core.go -> pkg/api/handler.go (direct) -> cmd/server/main.go (transitive)
	goRep, err := AnalyzeImpact(context.Background(), env, repo, []string{"pkg/core/core.go"})
	if err != nil {
		t.Fatalf("AnalyzeImpact(Go) failed: %v", err)
	}
	if !slices.Contains(goRep.DirectDependents, "pkg/api/handler.go") {
		t.Fatalf("expected direct dependent pkg/api/handler.go, got %+v", goRep)
	}
	if !slices.Contains(goRep.TransitiveDependents, "cmd/server/main.go") {
		t.Fatalf("expected transitive dependent cmd/server/main.go, got %+v", goRep)
	}
	if !slices.Contains(goRep.RelatedTests, "pkg/core/core_test.go") {
		t.Fatalf("expected related test pkg/core/core_test.go, got %+v", goRep)
	}

	// 2. TypeScript + Python + Java + C++ multi-file impact
	polyRep, err := AnalyzeImpact(context.Background(), env, repo, []string{
		"frontend/src/api.ts",
		"ai/utils.py",
		"backend/src/main/java/com/acme/Token.java",
		"core/include/math.h",
	})
	if err != nil {
		t.Fatalf("AnalyzeImpact(polyglot) failed: %v", err)
	}
	for _, wantDep := range []string{
		"frontend/src/checkout.ts",
		"ai/pipeline.py",
		"backend/src/main/java/com/acme/Auth.java",
		"core/src/calc.cpp",
	} {
		if !slices.Contains(polyRep.DirectDependents, wantDep) {
			t.Fatalf("missing polyglot direct dependent %q in %+v", wantDep, polyRep.DirectDependents)
		}
	}
	for _, wantTest := range []string{
		"frontend/src/api.test.ts",
		"ai/test_utils.py",
		"backend/src/test/java/com/acme/TokenTest.java",
	} {
		if !slices.Contains(polyRep.RelatedTests, wantTest) {
			t.Fatalf("missing polyglot related test %q in %+v", wantTest, polyRep.RelatedTests)
		}
	}
}
