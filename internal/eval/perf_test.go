package eval

import (
	"strings"
	"testing"
)

func i64p(v int64) *int64     { return &v }
func f64p(v float64) *float64 { return &v }

func TestSummarizeCohortsAndRegressionBudget(t *testing.T) {
	records := []RunRecord{
		// Baseline cohort: engineer@0.3.0 on claude / claude-sonnet-4-5
		{TaskID: "H01", Arm: ArmWithKit, Runtime: "claude", ModelID: "claude-sonnet-4-5", KitID: "engineer", KitVersion: "0.3.0", Outcome: OutcomePassed, DurationMs: 1000, InputTokens: i64p(1000), OutputTokens: i64p(200), CostUSD: f64p(0.010)},
		{TaskID: "H02", Arm: ArmWithKit, Runtime: "claude", ModelID: "claude-sonnet-4-5", KitID: "engineer", KitVersion: "0.3.0", Outcome: OutcomePassed, DurationMs: 1200, InputTokens: i64p(1100), OutputTokens: i64p(250), CostUSD: f64p(0.012)},
		{TaskID: "H03", Arm: ArmWithKit, Runtime: "claude", ModelID: "claude-sonnet-4-5", KitID: "engineer", KitVersion: "0.3.0", Outcome: OutcomeTaskFailed, DurationMs: 1400, InputTokens: i64p(1200), OutputTokens: i64p(300), CostUSD: f64p(0.014)},

		// Candidate cohort: engineer@0.4.0 on same model/runtime -> better pass rate, bounded cost/latency -> pass
		{TaskID: "H01", Arm: ArmWithKit, Runtime: "claude", ModelID: "claude-sonnet-4-5", KitID: "engineer", KitVersion: "0.4.0", Outcome: OutcomePassed, DurationMs: 1100, InputTokens: i64p(1050), OutputTokens: i64p(210), CostUSD: f64p(0.011)},
		{TaskID: "H02", Arm: ArmWithKit, Runtime: "claude", ModelID: "claude-sonnet-4-5", KitID: "engineer", KitVersion: "0.4.0", Outcome: OutcomePassed, DurationMs: 1250, InputTokens: i64p(1150), OutputTokens: i64p(220), CostUSD: f64p(0.012)},
		{TaskID: "H03", Arm: ArmWithKit, Runtime: "claude", ModelID: "claude-sonnet-4-5", KitID: "engineer", KitVersion: "0.4.0", Outcome: OutcomePassed, DurationMs: 1300, InputTokens: i64p(1180), OutputTokens: i64p(240), CostUSD: f64p(0.013)},

		// Candidate cohort on different runtime (codex) -> must NOT mix or compare against claude baseline
		{TaskID: "H01", Arm: ArmWithKit, Runtime: "codex", ModelID: "claude-sonnet-4-5", KitID: "engineer", KitVersion: "0.4.0", Outcome: OutcomeEnvironmentBlocked, DurationMs: 0},
	}

	cohorts := SummarizeCohorts(records)
	if len(cohorts) != 3 {
		t.Fatalf("expected 3 isolated cohorts, got %d", len(cohorts))
	}
	base := cohorts[0]
	cand := cohorts[1]
	codexCand := cohorts[2]

	if codexCand.ValidPassRate != nil || codexCand.BlockedRuns != 1 {
		t.Fatalf("blocked cohort should keep ValidPassRate nil, got %+v", codexCand)
	}

	budget := RegressionBudget{
		MinValidRuns:         3,
		MaxPassRateDrop:      0.05,
		MaxLatencyMultiplier: 1.50,
		MaxTokenMultiplier:   1.40,
		MaxCostMultiplier:    1.40,
	}
	okRes := EvaluateCohortRegression(base, cand, budget)
	if okRes.Status != "pass" || len(okRes.Violations) != 0 {
		t.Fatalf("expected regression check pass, got %+v", okRes)
	}

	// Cross-runtime comparison refuses cleanly as insufficient_data.
	crossRes := EvaluateCohortRegression(base, codexCand, budget)
	if crossRes.Status != "insufficient_data" {
		t.Fatalf("expected insufficient_data on cross-runtime comparison, got %+v", crossRes)
	}

	// Simulate latency & token explosion regression.
	badCand := cand
	slow := int64(5000)
	highTok := float64(5000)
	lowPass := 0.25
	badCand.MedianDurationMs = &slow
	badCand.MeanTotalTokens = &highTok
	badCand.ValidPassRate = &lowPass
	regRes := EvaluateCohortRegression(base, badCand, budget)
	if regRes.Status != "regressed" || len(regRes.Violations) < 3 {
		t.Fatalf("expected regressed with >=3 violations, got %+v", regRes)
	}
}

func TestRedactLogText(t *testing.T) {
	t.Setenv("HOME", "/home/alice")
	raw := "Running in /home/alice/repo ANTHROPIC_API_KEY=sk-ant-12345 Authorization=Bearer tok999 https://user:pw@host.test/idx?token=abc"
	got := RedactLogText(raw, "sk-ant-12345")
	for _, secret := range []string{"/home/alice", "sk-ant-12345", "tok999", "user:pw", "token=abc"} {
		if strings.Contains(got, secret) {
			t.Fatalf("log redaction leaked %q in %q", secret, got)
		}
	}
}
