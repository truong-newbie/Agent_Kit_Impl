// Package eval provides reproducible, isolated baseline vs. with-kit evaluation harnesses
// for coding-agent benchmarking without mutating real user state or mixing evaluator tests
// into the agent workspace prematurely.
package eval

const (
	OutcomePassed             = "passed"
	OutcomeTaskFailed         = "task_failed"
	OutcomeEnvironmentBlocked = "environment_blocked"
	OutcomeTimeout            = "timeout"
	OutcomeCancelled          = "cancelled"
	OutcomeNotRun             = "not_run"

	ArmBaseline = "baseline"
	ArmWithKit  = "with_kit"
)

// Scenario defines one isolated benchmark task pinned to a repository fixture snapshot.
type Scenario struct {
	ID                string   `json:"id"`
	Stack             string   `json:"stack"`    // go | ts | python | generic
	Category          string   `json:"category"` // ask | plan | implement | debug | review | test | explain | scope-guard | triage
	Suite             string   `json:"suite"`    // dev | held_out | dtkit
	FixtureDir        string   `json:"fixtureDir"`
	Prompt            string   `json:"prompt"`
	AllowedWritePaths []string `json:"allowedWritePaths"`
	ProtectedTests    []string `json:"protectedTests,omitempty"`
	EvaluatorTarget   string   `json:"evaluatorTarget,omitempty"` // relative path inside workspace where evaluator files are overlaid post-run
	EvaluatorCmd      []string `json:"evaluatorCmd,omitempty"`    // optional independent command run post-execution
	ReadOnly          bool     `json:"readOnly"`
	ExpectedBehavior  string   `json:"expectedBehavior"`
}

// Manifest freezes baseline kit metadata and evaluation scenarios.
type Manifest struct {
	SchemaVersion            int        `json:"schemaVersion"`
	FrozenBaselineKitID      string     `json:"frozenBaselineKitId"`
	FrozenBaselineKitVersion string     `json:"frozenBaselineKitVersion"`
	FrozenBaselineKitDigest  string     `json:"frozenBaselineKitDigest"`
	DevScenarios             []Scenario `json:"devScenarios"`
	HeldOutScenarios         []Scenario `json:"heldOutScenarios"`
	DTKitRegressionScenarios []Scenario `json:"dtkitRegressionScenarios,omitempty"`
}

// ExecRequest describes one isolated trial handed to an AgentExecutor.
type ExecRequest struct {
	WorkspaceDir string
	HomeDir      string
	Runtime      string
	ModelID      string
	Arm          string
	Scenario     Scenario
	TimeoutSec   int
	MaxBudgetUSD *float64
}

// ExecResult reports raw agent execution observations from an AgentExecutor.
type ExecResult struct {
	ExitCode      int
	Output        string
	Blocked       bool
	BlockedReason string
	Cancelled     bool
	TimedOut      bool
	InputTokens   *int64
	OutputTokens  *int64
	CostUSD       *float64
}

// RunRecord stores all required metrics and tampering/scope checks for one run.
type RunRecord struct {
	TaskID            string   `json:"taskId"`
	FixtureHash       string   `json:"fixtureHash"`
	Arm               string   `json:"arm"`
	Repeat            int      `json:"repeat"`
	Runtime           string   `json:"runtime"`
	ModelID           string   `json:"modelId"`
	KitID             string   `json:"kitId,omitempty"`
	KitVersion        string   `json:"kitVersion,omitempty"`
	KitDigest         string   `json:"kitDigest,omitempty"`
	DtkitSHA          string   `json:"dtkitSha"`
	ConfigHash        string   `json:"configHash"`
	Outcome           string   `json:"outcome"`
	DurationMs        int64    `json:"durationMs"`
	InputTokens       *int64   `json:"inputTokens"`
	OutputTokens      *int64   `json:"outputTokens"`
	CostUSD           *float64 `json:"costUsd"`
	ExitCode          int      `json:"exitCode"`
	ChangedPaths      []string `json:"changedPaths"`
	AllowedWritePaths []string `json:"allowedWritePaths"`
	ScopeViolation    bool     `json:"scopeViolation"`
	TestTampering     bool     `json:"testTampering"`
	FalseCompletion   bool     `json:"falseCompletion"`
	FabricatedClaim   bool     `json:"fabricatedClaim"`
	EvaluatorPassed   bool     `json:"evaluatorPassed"`
	EvaluatorDetail   string   `json:"evaluatorDetail,omitempty"`
	ArtifactPaths     []string `json:"artifactPaths,omitempty"`
	Notes             string   `json:"notes,omitempty"`
}

// PlannedTrial identifies a single scheduled (scenario, repeat, arm) execution.
type PlannedTrial struct {
	Scenario Scenario
	Repeat   int
	Arm      string
}

// PairResult compares baseline vs. with_kit for a single (task, repeat) pair.
type PairResult struct {
	TaskID  string `json:"taskId"`
	Repeat  int    `json:"repeat"`
	Winner  string `json:"winner"` // baseline | with_kit | tie | incomplete
	Details string `json:"details,omitempty"`
}

// ArmCounts summarizes raw outcomes for one arm without hiding blocked or failed runs.
type ArmCounts struct {
	Total              int      `json:"total"`
	Passed             int      `json:"passed"`
	TaskFailed         int      `json:"taskFailed"`
	EnvironmentBlocked int      `json:"environmentBlocked"`
	Timeout            int      `json:"timeout"`
	Cancelled          int      `json:"cancelled"`
	NotRun             int      `json:"notRun"`
	ScopeViolations    int      `json:"scopeViolations"`
	TestTampering      int      `json:"testTampering"`
	FalseCompletions   int      `json:"falseCompletions"`
	ValidPassRate      *float64 `json:"validPassRate"` // nil when 0 valid non-blocked runs exist
}

// SuiteSummary aggregates paired results and raw counts cleanly.
type SuiteSummary struct {
	Baseline     ArmCounts    `json:"baseline"`
	WithKit      ArmCounts    `json:"withKit"`
	Pairs        []PairResult `json:"pairs"`
	WithKitWins  int          `json:"withKitWins"`
	BaselineWins int          `json:"baselineWins"`
	Ties         int          `json:"ties"`
	Incomplete   int          `json:"incomplete"`
}
