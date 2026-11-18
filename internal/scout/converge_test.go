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
