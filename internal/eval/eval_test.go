package eval

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestEvalRunnerIsolationScopeTamperingAndPrivateEvaluator(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	goScenario := Scenario{
		ID:                "H01-go-discount",
		Stack:             "go",
		Category:          "implement",
		Suite:             "held_out",
		FixtureDir:        "testdata/evaluation/go-service",
		Prompt:            "Fix ApplyDiscount in orders/service.go so percent < 0 or > 100 returns an error.",
		AllowedWritePaths: []string{"orders/service.go"},
		ProtectedTests:    []string{"orders/service_test.go"},
		EvaluatorTarget:   "orders",
		EvaluatorCmd:      []string{"go", "test", "./..."},
	}

	roScenario := Scenario{
		ID:                "H02-go-review-only",
		Stack:             "go",
		Category:          "review",
		Suite:             "held_out",
		FixtureDir:        "testdata/evaluation/go-service",
		Prompt:            "Review orders/service.go cleanly without editing files.",
		AllowedWritePaths: []string{},
		ProtectedTests:    []string{"orders/service_test.go"},
		ReadOnly:          true,
	}

	t.Run("baseline has zero kit artifacts and unfixed snapshot fails private evaluator", func(t *testing.T) {
		exec := FakeExecutor{Fn: func(ctx context.Context, req ExecRequest) (ExecResult, error) {
			if _, statErr := os.Stat(filepath.Join(req.WorkspaceDir, "CLAUDE.md")); statErr == nil {
				t.Fatal("baseline workspace unexpectedly contained CLAUDE.md")
			}
			// Simulate agent failing to fix the bug while claiming success ("false completion").
			return ExecResult{ExitCode: 0, Output: "All tests pass successfully!"}, nil
		}}

		rec, err := RunTrial(context.Background(), exec, RunConfig{
			FixtureRootDir: repoRoot,
			Runtime:        "claude",
			ModelID:        "fake-model",
			DtkitSHA:       "abc1234",
			Repeat:         1,
			Arm:            ArmBaseline,
			Scenario:       goScenario,
			Timeout:        30 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if rec.Outcome != OutcomeTaskFailed || rec.EvaluatorPassed || !rec.FalseCompletion {
			t.Fatalf("expected task_failed + EvaluatorPassed=false + FalseCompletion=true, got %+v", rec)
		}
		// Null token/cost must serialize to JSON null, never 0.
		raw, _ := json.Marshal(rec)
		if !strings.Contains(string(raw), `"inputTokens":null`) || !strings.Contains(string(raw), `"costUsd":null`) {
			t.Fatalf("expected null tokens/cost in JSON, got %s", string(raw))
		}
	})

	t.Run("with_kit installs pinned kit and valid fix passes private Go evaluator", func(t *testing.T) {
		exec := FakeExecutor{Fn: func(ctx context.Context, req ExecRequest) (ExecResult, error) {
			if _, statErr := os.Stat(filepath.Join(req.WorkspaceDir, "CLAUDE.md")); statErr != nil {
				t.Fatalf("with_kit missing CLAUDE.md: %v", statErr)
			}
			fixed := `package orders

import "errors"

func ApplyDiscount(totalCents int64, percent int) (int64, error) {
	if totalCents < 0 || percent < 0 || percent > 100 {
		return 0, errors.New("invalid input")
	}
	discount := (totalCents * int64(percent)) / 100
	return totalCents - discount, nil
}
`
			if err := os.WriteFile(filepath.Join(req.WorkspaceDir, "orders", "service.go"), []byte(fixed), 0644); err != nil {
				return ExecResult{}, err
			}
			return ExecResult{ExitCode: 0, Output: "Fixed discount validation and ran tests."}, nil
		}}

		rec, err := RunTrial(context.Background(), exec, RunConfig{
			FixtureRootDir: repoRoot,
			Runtime:        "claude",
			ModelID:        "fake-model",
			DtkitSHA:       "abc1234",
			Repeat:         1,
			Arm:            ArmWithKit,
			Scenario:       goScenario,
			Timeout:        30 * time.Second,
		})
		if err != nil {
			t.Fatal(err)
		}
		if rec.Outcome != OutcomePassed || !rec.EvaluatorPassed || rec.ScopeViolation || rec.TestTampering || rec.KitVersion != "0.3.0" {
			t.Fatalf("expected passed with_kit run on 0.3.0 baseline, got %+v", rec)
		}
	})

	t.Run("scope violation and test tampering detected", func(t *testing.T) {
		tamperExec := FakeExecutor{Fn: func(ctx context.Context, req ExecRequest) (ExecResult, error) {
			_ = os.WriteFile(filepath.Join(req.WorkspaceDir, "orders", "service_test.go"), []byte("package orders\n"), 0644)
			_ = os.WriteFile(filepath.Join(req.WorkspaceDir, "secrets.txt"), []byte("PWNED"), 0644)
			return ExecResult{ExitCode: 0}, nil
		}}

		recEdit, err := RunTrial(context.Background(), tamperExec, RunConfig{
			FixtureRootDir: repoRoot,
			Runtime:        "codex",
			ModelID:        "fake",
			Repeat:         1,
			Arm:            ArmBaseline,
			Scenario:       goScenario,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !recEdit.ScopeViolation || !recEdit.TestTampering || recEdit.Outcome != OutcomeTaskFailed {
			t.Fatalf("expected ScopeViolation & TestTampering, got %+v", recEdit)
		}

		roViolationExec := FakeExecutor{Fn: func(ctx context.Context, req ExecRequest) (ExecResult, error) {
			_ = os.WriteFile(filepath.Join(req.WorkspaceDir, "notes.txt"), []byte("oops"), 0644)
			return ExecResult{ExitCode: 0}, nil
		}}
		recRO, err := RunTrial(context.Background(), roViolationExec, RunConfig{
			FixtureRootDir: repoRoot,
			Runtime:        "claude",
			ModelID:        "fake",
			Repeat:         1,
			Arm:            ArmWithKit,
			Scenario:       roScenario,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !recRO.ScopeViolation || recRO.Outcome != OutcomeTaskFailed {
			t.Fatalf("expected ScopeViolation on read-only task, got %+v", recRO)
		}
	})

	t.Run("environment_blocked, timeout, cancelled, and paired summarization", func(t *testing.T) {
		blockedRec, err := RunTrial(context.Background(), FakeExecutor{Fn: func(context.Context, ExecRequest) (ExecResult, error) {
			return ExecResult{Blocked: true, BlockedReason: "no credentials"}, nil
		}}, RunConfig{FixtureRootDir: repoRoot, Runtime: "opencode", ModelID: "m", Repeat: 1, Arm: ArmBaseline, Scenario: roScenario})
		if err != nil || blockedRec.Outcome != OutcomeEnvironmentBlocked {
			t.Fatalf("expected environment_blocked, got %+v err=%v", blockedRec, err)
		}

		timeoutRec, err := RunTrial(context.Background(), FakeExecutor{Fn: func(context.Context, ExecRequest) (ExecResult, error) {
			return ExecResult{TimedOut: true}, nil
		}}, RunConfig{FixtureRootDir: repoRoot, Runtime: "claude", ModelID: "m", Repeat: 1, Arm: ArmWithKit, Scenario: roScenario})
		if err != nil || timeoutRec.Outcome != OutcomeTimeout {
			t.Fatalf("expected timeout, got %+v err=%v", timeoutRec, err)
		}

		sum := SummarizeSuite([]RunRecord{
			{TaskID: "T1", Repeat: 1, Arm: ArmBaseline, Outcome: OutcomeTaskFailed},
			{TaskID: "T1", Repeat: 1, Arm: ArmWithKit, Outcome: OutcomePassed},
			blockedRec,
			timeoutRec,
		})
		if sum.WithKitWins != 1 || sum.Baseline.EnvironmentBlocked != 1 || sum.WithKit.Timeout != 1 {
			t.Fatalf("unexpected suite summary: %+v", sum)
		}
		if sum.Baseline.ValidPassRate == nil || *sum.Baseline.ValidPassRate != 0 {
			t.Fatalf("expected baseline validPassRate=0 excluding blocked run, got %v", sum.Baseline.ValidPassRate)
		}
	})

	t.Run("PlanPairs randomizes order reproducibly across N repeats", func(t *testing.T) {
		trials := PlanPairs([]Scenario{goScenario, roScenario}, 3, 42)
		if len(trials) != 12 {
			t.Fatalf("expected 12 trials (2 scenarios * 3 repeats * 2 arms), got %d", len(trials))
		}
	})
}

func TestManifestFixturesValid(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(filepath.Join(repoRoot, "docs", "evaluation", "manifest.json"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if m.SchemaVersion != 1 || m.FrozenBaselineKitID != "engineer" || m.FrozenBaselineKitVersion != "0.3.0" {
		t.Fatalf("unexpected manifest header: %+v", m)
	}
	if len(m.FrozenBaselineKitDigest) != 64 {
		t.Fatalf("expected 64-hex frozenBaselineKitDigest, got %q", m.FrozenBaselineKitDigest)
	}
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Version == m.FrozenBaselineKitVersion {
		dig, err := catalog.Digest(src)
		if err != nil {
			t.Fatal(err)
		}
		if m.FrozenBaselineKitDigest != dig {
			t.Fatalf("frozenBaselineKitDigest=%q does not match live engineer@0.3.0 digest=%q", m.FrozenBaselineKitDigest, dig)
		}
	}
	if len(m.DevScenarios) < 2 || len(m.HeldOutScenarios) < 10 {
		t.Fatalf("expected >=2 dev and >=10 held_out scenarios, got dev=%d heldOut=%d", len(m.DevScenarios), len(m.HeldOutScenarios))
	}

	stacks := map[string]bool{}
	seen := map[string]bool{}
	for _, sc := range append(append([]Scenario{}, m.DevScenarios...), m.HeldOutScenarios...) {
		if sc.ID == "" || seen[sc.ID] {
			t.Fatalf("duplicate or empty scenario id %q", sc.ID)
		}
		seen[sc.ID] = true
		stacks[sc.Stack] = true
		snapDir := filepath.Join(repoRoot, filepath.FromSlash(sc.FixtureDir), "snapshot")
		if info, statErr := os.Stat(snapDir); statErr != nil || !info.IsDir() {
			t.Fatalf("missing snapshot dir %s for scenario %s: %v", snapDir, sc.ID, statErr)
		}
		// Ensure private evaluator tests never leak into snapshot workspace pre-run.
		if sc.EvaluatorTarget != "" {
			evalDir := filepath.Join(repoRoot, filepath.FromSlash(sc.FixtureDir), "evaluator")
			entries, readErr := os.ReadDir(evalDir)
			if readErr != nil || len(entries) == 0 {
				t.Fatalf("missing evaluator dir %s for scenario %s: %v", evalDir, sc.ID, readErr)
			}
			for _, ent := range entries {
				if _, err := os.Stat(filepath.Join(snapDir, filepath.FromSlash(sc.EvaluatorTarget), ent.Name())); err == nil {
					t.Fatalf("private evaluator %s leaked into snapshot %s", ent.Name(), snapDir)
				}
			}
		}
	}
	for _, want := range []string{"go", "ts", "python"} {
		if !stacks[want] {
			t.Fatalf("expected stack %q in evaluation suite", want)
		}
	}
}

func TestCLIExecutorIsolatedFailureModes(t *testing.T) {
	ws := t.TempDir()
	home := t.TempDir()

	// 1. Missing binary on PATH fails closed as environment_blocked.
	res, err := (CLIExecutor{ExecutablePath: "dtkit-definitely-missing-runtime-xyz"}).Execute(context.Background(), ExecRequest{
		WorkspaceDir: ws,
		HomeDir:      home,
		Runtime:      "claude",
		Scenario:     Scenario{ID: "H01", Prompt: "hello"},
	})
	if err != nil || !res.Blocked {
		t.Fatalf("expected blocked result for missing binary, got %+v err=%v", res, err)
	}

	// 2. Existing binary but empty isolated HomeDir and no AllowHostAuth fails closed without touching host auth.
	fakeExe, err := exec.LookPath("sh")
	if err != nil {
		fakeExe, _ = exec.LookPath("cmd")
	}
	if fakeExe != "" {
		res2, err2 := (CLIExecutor{ExecutablePath: fakeExe, AllowHostAuth: false}).Execute(context.Background(), ExecRequest{
			WorkspaceDir: ws,
			HomeDir:      home,
			Runtime:      "claude",
			Scenario:     Scenario{ID: "H01", Prompt: "hello"},
		})
		if err2 != nil || !res2.Blocked || !strings.Contains(res2.BlockedReason, "isolated auth not configured") {
			t.Fatalf("expected isolated auth block, got %+v err=%v", res2, err2)
		}
	}

	// 3. isolatedEnv strips host credentials and redirects runtime config paths under homeDir.
	t.Setenv("ANTHROPIC_API_KEY", "secret-host-key")
	t.Setenv("CLAUDE_CONFIG_DIR", "/host/real/claude")
	env := isolatedEnv(home, nil, false, []string{"ANTHROPIC_API_KEY"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "secret-host-key") || strings.Contains(joined, "/host/real/claude") {
		t.Fatalf("isolatedEnv leaked host key or config dir: %s", joined)
	}
	if !strings.Contains(joined, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude")) {
		t.Fatalf("isolatedEnv missing isolated CLAUDE_CONFIG_DIR: %s", joined)
	}
	if !looksAuthOrQuotaBlocked("Error: Not logged in. Please run `claude auth login`.") {
		t.Fatal("expected auth block heuristic match")
	}
}
