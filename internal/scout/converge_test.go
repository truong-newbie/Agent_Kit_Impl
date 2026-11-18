package scout

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/guard"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestAnalyzeConvergenceAndAppendTasks(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")
	for rel, body := range map[string]string{
		".dtkit/constitution.md": "# Constitution\n- MUST keep handlers thin.\n",
		"implementation-plan.md": "# Plan\n- [x] Create `internal/service/ok.go`\n- [ ] Create `internal/service/missing.go`\n",
		"internal/service/ok.go": "package service\nfunc Ok() bool { return true }\n",
	} {
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

	rep1, err := AnalyzeConvergence(context.Background(), env, repo, "")
	if err != nil {
		t.Fatalf("AnalyzeConvergence(incomplete) failed: %v", err)
	}
	if rep1.Status != "incomplete" || rep1.Constitution != ".dtkit/constitution.md" || rep1.CompletedItems != 1 || len(rep1.UnmetItems) != 1 || len(rep1.MissingFiles) != 1 || rep1.MissingFiles[0] != "internal/service/missing.go" {
		t.Fatalf("unexpected incomplete ConvergenceReport: %+v", rep1)
	}

	tasksPath := filepath.Join(repo, "tasks.md")
	initialTasks := "# Tasks\n- [x] Setup\n"
	if err := os.WriteFile(tasksPath, []byte(initialTasks), 0644); err != nil {
		t.Fatal(err)
	}
	appended, err := AppendConvergenceTasks(repo, "tasks.md", rep1)
	if err != nil || !appended {
		t.Fatalf("AppendConvergenceTasks failed: appended=%v err=%v", appended, err)
	}
	afterTasks, _ := os.ReadFile(tasksPath)
	if !strings.HasPrefix(string(afterTasks), initialTasks) || !strings.Contains(string(afterTasks), "## Phase Convergence") || !strings.Contains(string(afterTasks), "internal/service/missing.go") {
		t.Fatalf("unexpected appended tasks.md:\n%s", string(afterTasks))
	}

	// Satisfy missing file & checklist + record check pass -> converged, and AppendConvergenceTasks leaves tasks.md byte-for-byte untouched.
	if err := os.WriteFile(filepath.Join(repo, "internal/service/missing.go"), []byte("package service\nfunc Missing() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "implementation-plan.md"), []byte("# Plan\n- [x] Create `internal/service/ok.go`\n- [x] Create `internal/service/missing.go`\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordCheckPass(repo, []string{"go-test"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	rep2, err := AnalyzeConvergence(context.Background(), env, repo, "implementation-plan.md")
	if err != nil {
		t.Fatalf("AnalyzeConvergence(converged) failed: %v", err)
	}
	if rep2.Status != "converged" || !rep2.ReceiptVerified {
		t.Fatalf("expected converged + verified receipt, got %+v", rep2)
	}
	beforeNoop, _ := os.ReadFile(tasksPath)
	appended2, err := AppendConvergenceTasks(repo, "tasks.md", rep2)
	if err != nil || appended2 {
		t.Fatalf("expected no-op append when converged, got appended=%v err=%v", appended2, err)
	}
	afterNoop, _ := os.ReadFile(tasksPath)
	if string(beforeNoop) != string(afterNoop) {
		t.Fatal("expected tasks.md byte-for-byte unchanged on converged run")
	}
}

func TestSpecInitCheckAndConvergeSpecAlignment(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}

	// 1. Dry-run InitSpec writes nothing; real InitSpec writes docs/specs/auth-sso.md and rejects overwrite.
	previewRel, previewBody, err := InitSpec(context.Background(), env, repo, "auth-sso", "SSO Authentication", true)
	if err != nil || previewRel != "docs/specs/auth-sso.md" || !strings.Contains(previewBody, "SSO Authentication") {
		t.Fatalf("dry-run InitSpec unexpected: rel=%q err=%v", previewRel, err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, "docs", "specs", "auth-sso.md")); !os.IsNotExist(statErr) {
		t.Fatal("expected dry-run InitSpec to write nothing")
	}
	rel, _, err := InitSpec(context.Background(), env, repo, "auth-sso", "SSO Authentication", false)
	if err != nil || rel != "docs/specs/auth-sso.md" {
		t.Fatalf("InitSpec failed: %v", err)
	}
	if _, _, dupErr := InitSpec(context.Background(), env, repo, "auth-sso", "", false); dupErr == nil {
		t.Fatal("expected InitSpec to refuse overwriting existing spec")
	}

	// 2. Spec containing [NEEDS CLARIFICATION] fails InspectSpec with needs_clarification and surfaces in AnalyzeConvergenceWithSpec.
	specAbs := filepath.Join(repo, "docs", "specs", "auth-sso.md")
	specWithClar := `# Feature Specification: SSO Authentication

## 1. Problem & User Scenarios
### User Story 1 — Login (Priority: P1)
- **Given** unauthenticated user, **When** clicking SSO, **Then** redirect to IdP.

## 3. Functional Requirements
- **FR-001:** Support OIDC login.
- **FR-002:** Enforce session timeout [NEEDS CLARIFICATION: 15m or 30m idle timeout?].

## 5. Measurable Success Criteria
- **SC-001:** Login completes in under 500ms.
`
	if err := os.WriteFile(specAbs, []byte(specWithClar), 0644); err != nil {
		t.Fatal(err)
	}
	sRep, err := InspectSpec(context.Background(), env, repo, "docs/specs/auth-sso.md")
	if err != nil || sRep.Status != "needs_clarification" || len(sRep.UnresolvedClarifications) != 1 {
		t.Fatalf("expected needs_clarification, got %+v err=%v", sRep, err)
	}

	planAbs := filepath.Join(repo, "implementation-plan.md")
	if err := os.WriteFile(planAbs, []byte("# Plan\nCovers FR-001.\n- [x] Implement `internal/auth/oidc.go`\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "internal", "auth"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "internal", "auth", "oidc.go"), []byte("package auth\n// FR-001\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cRep1, err := AnalyzeConvergenceWithSpec(context.Background(), env, repo, "implementation-plan.md", "docs/specs/auth-sso.md")
	if err != nil || cRep1.Status != "incomplete" || len(cRep1.UnresolvedClarifications) != 1 || len(cRep1.UncoveredRequirements) != 1 || cRep1.UncoveredRequirements[0] != "FR-002" {
		t.Fatalf("expected incomplete convergence with FR-002 uncovered + 1 clarification, got %+v err=%v", cRep1, err)
	}

	// 3. Resolve clarification, cover FR-002 in plan, record check pass -> converged!
	specResolved := strings.Replace(specWithClar, "[NEEDS CLARIFICATION: 15m or 30m idle timeout?]", "after 30m idle", 1)
	if err := os.WriteFile(specAbs, []byte(specResolved), 0644); err != nil {
		t.Fatal(err)
	}
	sRep2, err := InspectSpec(context.Background(), env, repo, "docs/specs/auth-sso.md")
	if err != nil || sRep2.Status != "valid" || len(sRep2.FunctionalRequirements) != 2 || len(sRep2.SuccessCriteria) != 1 {
		t.Fatalf("expected valid spec, got %+v err=%v", sRep2, err)
	}
	if err := os.WriteFile(planAbs, []byte("# Plan\nCovers FR-001 and FR-002.\n- [x] Implement `internal/auth/oidc.go`\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := guard.RecordCheckPass(repo, []string{"go-test"}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	cRep2, err := AnalyzeConvergenceWithSpec(context.Background(), env, repo, "implementation-plan.md", "docs/specs/auth-sso.md")
	if err != nil || cRep2.Status != "converged" || len(cRep2.UncoveredRequirements) != 0 || len(cRep2.UnresolvedClarifications) != 0 {
		t.Fatalf("expected fully converged Spec->Plan->Code, got %+v err=%v", cRep2, err)
	}
}
