package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGuardModesAndGlobs(t *testing.T) {
	repo := t.TempDir()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	// Default unlocked
	if ok, reason := EvaluateEdit(repo, "internal/cli/cli.go"); !ok || reason != "" {
		t.Fatalf("expected unlocked allow, got ok=%v reason=%q", ok, reason)
	}

	// Read-only blocks everything
	if _, err := SavePolicy(repo, Policy{Mode: ModeReadOnly}, now); err != nil {
		t.Fatal(err)
	}
	if ok, reason := EvaluateEdit(repo, "README.md"); ok || !strings.Contains(reason, "read-only") {
		t.Fatalf("expected read-only deny, got ok=%v reason=%q", ok, reason)
	}

	// TDD-red allows tests only
	if _, err := SavePolicy(repo, Policy{Mode: ModeTDDRed}, now); err != nil {
		t.Fatal(err)
	}
	if ok, reason := EvaluateEdit(repo, "backend/src/test/java/AuthTest.java"); !ok {
		t.Fatalf("expected tdd-red allow test file, got %q", reason)
	}
	if ok, reason := EvaluateEdit(repo, "backend/src/main/java/Auth.java"); ok || !strings.Contains(reason, "tdd-red") {
		t.Fatalf("expected tdd-red deny source file, got ok=%v reason=%q", ok, reason)
	}

	// TDD-green allows implementation and protects test files
	if _, err := SavePolicy(repo, Policy{Mode: ModeTDDGreen, AllowGlobs: []string{"internal/cli/**"}}, now); err != nil {
		t.Fatal(err)
	}
	if ok, reason := EvaluateEdit(repo, "internal/cli/guard.go"); !ok {
		t.Fatalf("expected tdd-green allow inside glob, got %q", reason)
	}
	if ok, reason := EvaluateEdit(repo, "internal/cli/guard_test.go"); ok || !strings.Contains(reason, "protected") {
		t.Fatalf("expected tdd-green protect test file, got ok=%v reason=%q", ok, reason)
	}
	if ok, reason := EvaluateEdit(repo, "internal/state/state.go"); ok || !strings.Contains(reason, "outside allowed globs") {
		t.Fatalf("expected tdd-green deny outside glob, got ok=%v reason=%q", ok, reason)
	}

	if err := ClearPolicy(repo); err != nil {
		t.Fatal(err)
	}
	if ok, _ := EvaluateEdit(repo, "internal/state/state.go"); !ok {
		t.Fatal("expected unlock to restore edits")
	}
}

func TestAnalyzeUnifiedDiffTampering(t *testing.T) {
	diff := `diff --git a/pkg/foo_test.go b/pkg/foo_test.go
--- a/pkg/foo_test.go
+++ b/pkg/foo_test.go
@@ -10,4 +10,3 @@
+	t.Skip("skipping broken check")
-func TestCriticalBehavior(t *testing.T) {
-	if err != nil { t.Fatalf("boom: %v", err) }
+func helper() {
diff --git a/ai/test_worker.py b/ai/test_worker.py
--- a/ai/test_worker.py
+++ b/ai/test_worker.py
@@ -1,2 +1,2 @@
-@pytest.mark.parametrize
-    assert result == 42
+@pytest.mark.skip(reason="flaky")
`
	findings := AnalyzeUnifiedDiff(diff)
	rules := map[string]bool{}
	for _, f := range findings {
		rules[f.Path+":"+f.Rule] = true
	}
	for _, want := range []string{
		"pkg/foo_test.go:added_skip",
		"pkg/foo_test.go:removed_test_case",
		"pkg/foo_test.go:reduced_assertions",
		"ai/test_worker.py:added_skip",
		"ai/test_worker.py:reduced_assertions",
	} {
		if !rules[want] {
			t.Fatalf("missing tampering finding %q (got %+v)", want, findings)
		}
	}
}

func TestReceiptInvalidatesOnEditAndGitTampering(t *testing.T) {
	repo := t.TempDir()
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)

	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestOne(t *testing.T) { t.Fatalf(\"fail\") }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := exec.LookPath("git"); err == nil {
		for _, args := range [][]string{
			{"init"},
			{"config", "user.email", "dtkit@example.test"},
			{"config", "user.name", "DTKit"},
			{"add", "."},
			{"commit", "-m", "init"},
		} {
			if out, gErr := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); gErr != nil {
				t.Fatalf("git %v failed: %v\n%s", args, gErr, string(out))
			}
		}
	}

	// Missing receipt initially
	st, err := VerifyWorkspace(repo)
	if err != nil || st.Verified {
		t.Fatalf("expected unverified without receipt, got st=%+v err=%v", st, err)
	}

	// Record passing check -> verified
	if err := RecordCheckPass(repo, []string{"go-test"}, now); err != nil {
		t.Fatal(err)
	}
	st, err = VerifyWorkspace(repo)
	if err != nil || !st.Verified {
		t.Fatalf("expected verified after RecordCheckPass, got st=%+v err=%v", st, err)
	}

	// Edit main.go -> invalidates receipt
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc main() { println(1) }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	st, err = VerifyWorkspace(repo)
	if err != nil || st.Verified || !strings.Contains(st.Reason, "UNVERIFIED") {
		t.Fatalf("expected receipt invalidation after edit, got st=%+v err=%v", st, err)
	}

	// Streak tracking
	s, err := RecordCheckFailure(repo, []string{"go-test"}, now)
	if err != nil || s.ConsecutiveFailures != 1 {
		t.Fatalf("unexpected streak: %+v err=%v", s, err)
	}
	s, err = RecordCheckFailure(repo, []string{"go-test"}, now)
	if err != nil || s.ConsecutiveFailures != 2 {
		t.Fatalf("unexpected streak: %+v err=%v", s, err)
	}
	if err := RecordCheckPass(repo, []string{"go-test"}, now); err != nil {
		t.Fatal(err)
	}
	s, err = LoadStreak(repo)
	if err != nil || s.ConsecutiveFailures != 0 {
		t.Fatalf("expected streak reset to 0 after pass, got %+v err=%v", s, err)
	}
}
