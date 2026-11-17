package eval

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
)

// AgentExecutor executes one isolated coding-agent turn inside a prepared workspace.
type AgentExecutor interface {
	Execute(ctx context.Context, req ExecRequest) (ExecResult, error)
}

// FakeExecutor provides deterministic unit-test execution for the benchmark runner.
type FakeExecutor struct {
	Fn func(ctx context.Context, req ExecRequest) (ExecResult, error)
}

func (f FakeExecutor) Execute(ctx context.Context, req ExecRequest) (ExecResult, error) {
	if f.Fn == nil {
		return ExecResult{}, errors.New("FakeExecutor Fn is nil")
	}
	return f.Fn(ctx, req)
}

// LoadManifest reads and parses an evaluation manifest JSON file.
func LoadManifest(filePath string) (Manifest, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// ComputeTreeDigest returns a deterministic SHA-256 hex digest of all regular files beneath root.
func ComputeTreeDigest(root string) (string, error) {
	hashes, err := snapshotFileHashes(root)
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s\x00%s\n", k, hashes[k])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func snapshotFileHashes(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(rel)
		if d.IsDir() {
			if slash == ".git" || strings.HasPrefix(slash, ".git/") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		out[slash] = hex.EncodeToString(sum[:])
		return nil
	})
	return out, err
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}

// PlanPairs creates a deterministic randomized schedule of paired trials (baseline vs with_kit).
func PlanPairs(scenarios []Scenario, repeats int, seed int64) []PlannedTrial {
	if repeats <= 0 {
		repeats = 1
	}
	rng := rand.New(rand.NewSource(seed))
	out := make([]PlannedTrial, 0, len(scenarios)*repeats*2)
	for _, sc := range scenarios {
		for r := 1; r <= repeats; r++ {
			first, second := ArmBaseline, ArmWithKit
			if rng.Intn(2) == 1 {
				first, second = ArmWithKit, ArmBaseline
			}
			out = append(out,
				PlannedTrial{Scenario: sc, Repeat: r, Arm: first},
				PlannedTrial{Scenario: sc, Repeat: r, Arm: second},
			)
		}
	}
	return out
}

// RunConfig configures one isolated RunTrial execution.
type RunConfig struct {
	FixtureRootDir string
	Runtime        string
	ModelID        string
	DtkitSHA       string
	Repeat         int
	Arm            string
	Scenario       Scenario
	Timeout        time.Duration
	MaxBudgetUSD   *float64
}

// RunTrial prepares an isolated workspace and isolated home directory, runs the executor,
// enforces scope/tampering/false-completion checks, and runs private evaluator commands if configured.
func RunTrial(ctx context.Context, executor AgentExecutor, cfg RunConfig) (RunRecord, error) {
	rec := RunRecord{
		TaskID:            cfg.Scenario.ID,
		Arm:               cfg.Arm,
		Repeat:            cfg.Repeat,
		Runtime:           cfg.Runtime,
		ModelID:           cfg.ModelID,
		DtkitSHA:          cfg.DtkitSHA,
		AllowedWritePaths: append([]string{}, cfg.Scenario.AllowedWritePaths...),
		ChangedPaths:      []string{},
		Outcome:           OutcomeNotRun,
	}

	if cfg.Arm != ArmBaseline && cfg.Arm != ArmWithKit {
		return rec, fmt.Errorf("invalid eval arm %q", cfg.Arm)
	}
	profile, ok := adapter.Profile(cfg.Runtime)
	if !ok {
		return rec, fmt.Errorf("unsupported eval runtime %q", cfg.Runtime)
	}

	tempRoot, err := os.MkdirTemp("", "dtkit-eval-*")
	if err != nil {
		return rec, err
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()

	wsDir := filepath.Join(tempRoot, "workspace")
	homeDir := filepath.Join(tempRoot, "home")
	for _, d := range []string{wsDir, homeDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return rec, err
		}
	}

	snapSrc := filepath.Join(cfg.FixtureRootDir, filepath.FromSlash(cfg.Scenario.FixtureDir), "snapshot")
	if err := copyTree(snapSrc, wsDir); err != nil {
		return rec, fmt.Errorf("copy snapshot %s: %w", snapSrc, err)
	}
	fixHash, err := ComputeTreeDigest(snapSrc)
	if err != nil {
		return rec, err
	}
	rec.FixtureHash = fixHash

	var kitHashes map[string]string
	if cfg.Arm == ArmWithKit {
		pkg, srcFS, err := catalog.Engineer()
		if err != nil {
			return rec, err
		}
		kitDigest, err := catalog.Digest(srcFS)
		if err != nil {
			return rec, err
		}
		arts, err := adapter.RenderForRuntime(profile.ID, pkg, srcFS, "project")
		if err != nil {
			return rec, err
		}
		if _, err := installer.FreshInstallWithOptions("project", map[string]string{"project": wsDir}, pkg, srcFS, arts, time.Now(), installer.InstallOptions{
			Runtime:    profile.ID,
			SourceKind: "bundled",
		}); err != nil {
			return rec, err
		}
		rec.KitID = pkg.ID
		rec.KitVersion = pkg.Version
		rec.KitDigest = kitDigest
		kitHashes, err = snapshotFileHashes(wsDir)
		if err != nil {
			return rec, err
		}
	} else {
		kitHashes, err = snapshotFileHashes(wsDir)
		if err != nil {
			return rec, err
		}
	}

	cfgBytes, _ := json.Marshal(struct {
		Runtime string `json:"runtime"`
		Model   string `json:"model"`
		Arm     string `json:"arm"`
		KitVer  string `json:"kitVer"`
		KitDig  string `json:"kitDig"`
		FixHash string `json:"fixHash"`
	}{cfg.Runtime, cfg.ModelID, cfg.Arm, rec.KitVersion, rec.KitDigest, rec.FixtureHash})
	cfgSum := sha256.Sum256(cfgBytes)
	rec.ConfigHash = hex.EncodeToString(cfgSum[:])

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	execRes, execErr := executor.Execute(runCtx, ExecRequest{
		WorkspaceDir: wsDir,
		HomeDir:      homeDir,
		Runtime:      profile.ID,
		ModelID:      cfg.ModelID,
		Arm:          cfg.Arm,
		Scenario:     cfg.Scenario,
		TimeoutSec:   int(timeout.Seconds()),
		MaxBudgetUSD: cfg.MaxBudgetUSD,
	})
	rec.DurationMs = time.Since(start).Milliseconds()
	rec.ExitCode = execRes.ExitCode
	rec.InputTokens = execRes.InputTokens
	rec.OutputTokens = execRes.OutputTokens
	rec.CostUSD = execRes.CostUSD

	afterHashes, hashErr := snapshotFileHashes(wsDir)
	if hashErr != nil {
		return rec, hashErr
	}
	rec.ChangedPaths = diffHashes(kitHashes, afterHashes)
	rec.ScopeViolation = hasScopeViolation(rec.ChangedPaths, cfg.Scenario.ReadOnly, cfg.Scenario.AllowedWritePaths)
	rec.TestTampering = hasTestTampering(rec.ChangedPaths, cfg.Scenario.ProtectedTests, cfg.Scenario.AllowedWritePaths)

	if execRes.Cancelled || errors.Is(runCtx.Err(), context.Canceled) {
		rec.Outcome = OutcomeCancelled
		return rec, nil
	}
	if execRes.TimedOut || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		rec.Outcome = OutcomeTimeout
		return rec, nil
	}
	if execRes.Blocked {
		rec.Outcome = OutcomeEnvironmentBlocked
		rec.Notes = execRes.BlockedReason
		return rec, nil
	}
	if execErr != nil {
		rec.Outcome = OutcomeTaskFailed
		rec.Notes = execErr.Error()
		return rec, nil
	}

	evalDir := filepath.Join(cfg.FixtureRootDir, filepath.FromSlash(cfg.Scenario.FixtureDir), "evaluator")
	evalPassed, evalDetail := runPrivateEvaluator(runCtx, wsDir, evalDir, cfg.Scenario.EvaluatorTarget, cfg.Scenario.EvaluatorCmd)
	rec.EvaluatorPassed = evalPassed
	rec.EvaluatorDetail = evalDetail

	// Detect false completion when agent output claims tests passed or task completed despite evaluator failure / tampering.
	lowerOut := strings.ToLower(execRes.Output)
	claimedSuccess := strings.Contains(lowerOut, "all tests pass") || strings.Contains(lowerOut, "passed successfully")
	if claimedSuccess && (!evalPassed || execRes.ExitCode != 0) {
		rec.FalseCompletion = true
	}

	if rec.ScopeViolation || rec.TestTampering || !evalPassed || execRes.ExitCode != 0 {
		rec.Outcome = OutcomeTaskFailed
	} else {
		rec.Outcome = OutcomePassed
	}
	return rec, nil
}

func runPrivateEvaluator(ctx context.Context, wsDir, evalDir, evalTarget string, cmdArgs []string) (bool, string) {
	if len(cmdArgs) == 0 {
		return true, "no external evaluator command configured"
	}
	info, statErr := os.Stat(evalDir)
	targetDir := wsDir
	if evalTarget != "" {
		targetDir = filepath.Join(wsDir, filepath.FromSlash(evalTarget))
	}
	if statErr == nil && info.IsDir() {
		if err := copyTree(evalDir, targetDir); err != nil {
			return false, fmt.Sprintf("copy private evaluator failed: %v", err)
		}
	}
	exeName := cmdArgs[0]
	if exeName == "python" {
		if _, lpErr := exec.LookPath("python"); lpErr != nil {
			if _, lp3Err := exec.LookPath("python3"); lp3Err == nil {
				exeName = "python3"
			}
		}
	}
	cmd := exec.CommandContext(ctx, exeName, cmdArgs[1:]...)
	cmd.Dir = wsDir
	cmd.Env = append(os.Environ(), "PYTHONPATH="+wsDir)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	err := cmd.Run()
	if err != nil {
		return false, fmt.Sprintf("evaluator %v failed (%v): %s", cmdArgs, err, strings.TrimSpace(outBuf.String()))
	}
	return true, "evaluator passed"
}

func diffHashes(before, after map[string]string) []string {
	seen := map[string]bool{}
	var changed []string
	for k, v := range after {
		seen[k] = true
		if bv, ok := before[k]; !ok || bv != v {
			changed = append(changed, k)
		}
	}
	for k := range before {
		if !seen[k] {
			changed = append(changed, k)
		}
	}
	sort.Strings(changed)
	return changed
}

func matchesAllowed(relPath string, allowed []string) bool {
	norm := path.Clean(strings.TrimPrefix(filepath.ToSlash(relPath), "./"))
	for _, pat := range allowed {
		p := path.Clean(strings.TrimPrefix(filepath.ToSlash(pat), "./"))
		if ok, _ := path.Match(p, norm); ok || norm == p {
			return true
		}
	}
	return false
}

func hasScopeViolation(changed []string, readOnly bool, allowed []string) bool {
	if len(changed) == 0 {
		return false
	}
	if readOnly {
		return true
	}
	for _, c := range changed {
		if !matchesAllowed(c, allowed) {
			return true
		}
	}
	return false
}

func hasTestTampering(changed, protectedTests, allowed []string) bool {
	for _, c := range changed {
		for _, pt := range protectedTests {
			if c == pt && !matchesAllowed(c, allowed) {
				return true
			}
		}
	}
	return false
}

// SummarizeSuite computes raw counts and paired outcomes without hiding failures or blocked runs.
func SummarizeSuite(records []RunRecord) SuiteSummary {
	var sum SuiteSummary
	type pairKey struct {
		taskID string
		repeat int
	}
	paired := map[pairKey]map[string]RunRecord{}
	for _, r := range records {
		target := &sum.Baseline
		if r.Arm == ArmWithKit {
			target = &sum.WithKit
		}
		target.Total++
		switch r.Outcome {
		case OutcomePassed:
			target.Passed++
		case OutcomeTaskFailed:
			target.TaskFailed++
		case OutcomeEnvironmentBlocked:
			target.EnvironmentBlocked++
		case OutcomeTimeout:
			target.Timeout++
		case OutcomeCancelled:
			target.Cancelled++
		default:
			target.NotRun++
		}
		if r.ScopeViolation {
			target.ScopeViolations++
		}
		if r.TestTampering {
			target.TestTampering++
		}
		if r.FalseCompletion {
			target.FalseCompletions++
		}
		pk := pairKey{taskID: r.TaskID, repeat: r.Repeat}
		if paired[pk] == nil {
			paired[pk] = map[string]RunRecord{}
		}
		paired[pk][r.Arm] = r
	}

	calcRate := func(c *ArmCounts) {
		valid := c.Total - c.EnvironmentBlocked - c.NotRun
		if valid > 0 {
			rate := float64(c.Passed) / float64(valid)
			c.ValidPassRate = &rate
		}
	}
	calcRate(&sum.Baseline)
	calcRate(&sum.WithKit)

	keys := make([]pairKey, 0, len(paired))
	for k := range paired {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].taskID != keys[j].taskID {
			return keys[i].taskID < keys[j].taskID
		}
		return keys[i].repeat < keys[j].repeat
	})
	for _, k := range keys {
		m := paired[k]
		b, hasB := m[ArmBaseline]
		w, hasW := m[ArmWithKit]
		res := PairResult{TaskID: k.taskID, Repeat: k.repeat}
		if !hasB || !hasW || b.Outcome == OutcomeEnvironmentBlocked || w.Outcome == OutcomeEnvironmentBlocked || b.Outcome == OutcomeNotRun || w.Outcome == OutcomeNotRun {
			res.Winner = "incomplete"
			sum.Incomplete++
		} else if w.Outcome == OutcomePassed && b.Outcome != OutcomePassed {
			res.Winner = ArmWithKit
			sum.WithKitWins++
		} else if b.Outcome == OutcomePassed && w.Outcome != OutcomePassed {
			res.Winner = ArmBaseline
			sum.BaselineWins++
		} else {
			res.Winner = "tie"
			sum.Ties++
		}
		sum.Pairs = append(sum.Pairs, res)
	}
	return sum
}
