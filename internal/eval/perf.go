package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	logURLRe    = regexp.MustCompile(`https?://[^\s"'<>]+`)
	logBearerRe = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/-]+=*`)
	logKVRe     = regexp.MustCompile(`(?i)\b(token|secret|password|passwd|apikey|api_key|anthropic_api_key|openai_api_key|github_token|gh_token|authorization)=([^\s&"';]+)`)
)

// CohortKey isolates benchmark performance summaries strictly by model, runtime, and kit version.
type CohortKey struct {
	ModelID    string `json:"modelId"`
	Runtime    string `json:"runtime"`
	KitID      string `json:"kitId,omitempty"`
	KitVersion string `json:"kitVersion,omitempty"`
}

// CohortPerfSummary summarizes task pass rate, latency percentiles, tokens, and cost for one cohort.
type CohortPerfSummary struct {
	Cohort           CohortKey `json:"cohort"`
	TotalRuns        int       `json:"totalRuns"`
	ValidRuns        int       `json:"validRuns"`
	BlockedRuns      int       `json:"blockedRuns"`
	PassedRuns       int       `json:"passedRuns"`
	ValidPassRate    *float64  `json:"validPassRate"`
	MedianDurationMs *int64    `json:"medianDurationMs"`
	P95DurationMs    *int64    `json:"p95DurationMs"`
	MeanInputTokens  *float64  `json:"meanInputTokens"`
	MeanOutputTokens *float64  `json:"meanOutputTokens"`
	MeanTotalTokens  *float64  `json:"meanTotalTokens"`
	TotalCostUSD     *float64  `json:"totalCostUsd"`
	MeanCostUSD      *float64  `json:"meanCostUsd"`
}

// RegressionBudget sets explicit baseline-anchored tolerance bounds against a prior CohortPerfSummary.
type RegressionBudget struct {
	MinValidRuns         int     `json:"minValidRuns"`
	MaxPassRateDrop      float64 `json:"maxPassRateDrop"`      // e.g. 0.05 = max 5 percentage points drop
	MaxLatencyMultiplier float64 `json:"maxLatencyMultiplier"` // e.g. 1.50 = max +50% median latency
	MaxTokenMultiplier   float64 `json:"maxTokenMultiplier"`   // e.g. 1.40 = max +40% mean total tokens
	MaxCostMultiplier    float64 `json:"maxCostMultiplier"`    // e.g. 1.40 = max +40% mean USD cost
}

// RegressionCheckResult reports pass/regressed/insufficient_data vs baseline with exact violation details.
type RegressionCheckResult struct {
	Status        string    `json:"status"` // "pass" | "regressed" | "insufficient_data"
	Cohort        CohortKey `json:"cohort"`
	BaselineKit   string    `json:"baselineKit"`
	CandidateKit  string    `json:"candidateKit"`
	PassRateDelta *float64  `json:"passRateDelta,omitempty"`
	LatencyRatio  *float64  `json:"latencyRatio,omitempty"`
	TokenRatio    *float64  `json:"tokenRatio,omitempty"`
	CostRatio     *float64  `json:"costRatio,omitempty"`
	Violations    []string  `json:"violations,omitempty"`
}

// SummarizeCohorts groups RunRecords cleanly by (ModelID, Runtime, KitID, KitVersion) without mixing models/runtimes.
func SummarizeCohorts(records []RunRecord) []CohortPerfSummary {
	grouped := map[CohortKey][]RunRecord{}
	for _, r := range records {
		k := CohortKey{
			ModelID:    r.ModelID,
			Runtime:    r.Runtime,
			KitID:      r.KitID,
			KitVersion: r.KitVersion,
		}
		grouped[k] = append(grouped[k], r)
	}

	keys := make([]CohortKey, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ModelID != keys[j].ModelID {
			return keys[i].ModelID < keys[j].ModelID
		}
		if keys[i].Runtime != keys[j].Runtime {
			return keys[i].Runtime < keys[j].Runtime
		}
		if keys[i].KitID != keys[j].KitID {
			return keys[i].KitID < keys[j].KitID
		}
		return keys[i].KitVersion < keys[j].KitVersion
	})

	out := make([]CohortPerfSummary, 0, len(keys))
	for _, k := range keys {
		out = append(out, summarizeOneCohort(k, grouped[k]))
	}
	return out
}

func summarizeOneCohort(key CohortKey, recs []RunRecord) CohortPerfSummary {
	sum := CohortPerfSummary{
		Cohort:    key,
		TotalRuns: len(recs),
	}
	var durations []int64
	var inTokensSum, outTokensSum, bothTokensSum int64
	var inCount, outCount, bothCount int
	var costSum float64
	var costCount int

	for _, r := range recs {
		if r.Outcome == OutcomeEnvironmentBlocked || r.Outcome == OutcomeNotRun {
			sum.BlockedRuns++
			continue
		}
		sum.ValidRuns++
		if r.Outcome == OutcomePassed {
			sum.PassedRuns++
		}
		if r.DurationMs > 0 {
			durations = append(durations, r.DurationMs)
		}
		if r.InputTokens != nil {
			inTokensSum += *r.InputTokens
			inCount++
		}
		if r.OutputTokens != nil {
			outTokensSum += *r.OutputTokens
			outCount++
		}
		if r.InputTokens != nil && r.OutputTokens != nil {
			bothTokensSum += *r.InputTokens + *r.OutputTokens
			bothCount++
		}
		if r.CostUSD != nil {
			costSum += *r.CostUSD
			costCount++
		}
	}

	if sum.ValidRuns > 0 {
		rate := float64(sum.PassedRuns) / float64(sum.ValidRuns)
		sum.ValidPassRate = &rate
	}
	if len(durations) > 0 {
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		med := durations[len(durations)/2]
		p95Idx := int(math.Ceil(0.95*float64(len(durations)))) - 1
		if p95Idx < 0 {
			p95Idx = 0
		}
		if p95Idx >= len(durations) {
			p95Idx = len(durations) - 1
		}
		p95 := durations[p95Idx]
		sum.MedianDurationMs = &med
		sum.P95DurationMs = &p95
	}
	if inCount > 0 {
		v := float64(inTokensSum) / float64(inCount)
		sum.MeanInputTokens = &v
	}
	if outCount > 0 {
		v := float64(outTokensSum) / float64(outCount)
		sum.MeanOutputTokens = &v
	}
	if bothCount > 0 {
		v := float64(bothTokensSum) / float64(bothCount)
		sum.MeanTotalTokens = &v
	}
	if costCount > 0 {
		total := costSum
		mean := costSum / float64(costCount)
		sum.TotalCostUSD = &total
		sum.MeanCostUSD = &mean
	}
	return sum
}

// EvaluateCohortRegression enforces explicit baseline-anchored pass-rate, latency, token, and cost budgets
// within the same (ModelID, Runtime) cohort.
func EvaluateCohortRegression(baseline, candidate CohortPerfSummary, budget RegressionBudget) RegressionCheckResult {
	res := RegressionCheckResult{
		Status:       "pass",
		Cohort:       candidate.Cohort,
		BaselineKit:  baseline.Cohort.KitID + "@" + baseline.Cohort.KitVersion,
		CandidateKit: candidate.Cohort.KitID + "@" + candidate.Cohort.KitVersion,
	}
	if baseline.Cohort.ModelID != candidate.Cohort.ModelID || baseline.Cohort.Runtime != candidate.Cohort.Runtime {
		res.Status = "insufficient_data"
		res.Violations = append(res.Violations, fmt.Sprintf("cross-cohort comparison refused (%s/%s vs %s/%s)",
			baseline.Cohort.ModelID, baseline.Cohort.Runtime, candidate.Cohort.ModelID, candidate.Cohort.Runtime))
		return res
	}
	minRuns := budget.MinValidRuns
	if minRuns <= 0 {
		minRuns = 1
	}
	if baseline.ValidRuns < minRuns || candidate.ValidRuns < minRuns || baseline.ValidPassRate == nil || candidate.ValidPassRate == nil {
		res.Status = "insufficient_data"
		res.Violations = append(res.Violations, fmt.Sprintf("insufficient valid runs (baseline=%d, candidate=%d, required=%d)",
			baseline.ValidRuns, candidate.ValidRuns, minRuns))
		return res
	}

	delta := *candidate.ValidPassRate - *baseline.ValidPassRate
	res.PassRateDelta = &delta
	if delta < -budget.MaxPassRateDrop {
		res.Status = "regressed"
		res.Violations = append(res.Violations, fmt.Sprintf("pass rate dropped by %.3f (allowed max drop %.3f)", -delta, budget.MaxPassRateDrop))
	}

	if budget.MaxLatencyMultiplier > 0 && baseline.MedianDurationMs != nil && *baseline.MedianDurationMs > 0 && candidate.MedianDurationMs != nil {
		ratio := float64(*candidate.MedianDurationMs) / float64(*baseline.MedianDurationMs)
		res.LatencyRatio = &ratio
		if ratio > budget.MaxLatencyMultiplier {
			res.Status = "regressed"
			res.Violations = append(res.Violations, fmt.Sprintf("median latency ratio %.2fx exceeds budget %.2fx", ratio, budget.MaxLatencyMultiplier))
		}
	}

	if budget.MaxTokenMultiplier > 0 && baseline.MeanTotalTokens != nil && *baseline.MeanTotalTokens > 0 && candidate.MeanTotalTokens != nil {
		ratio := *candidate.MeanTotalTokens / *baseline.MeanTotalTokens
		res.TokenRatio = &ratio
		if ratio > budget.MaxTokenMultiplier {
			res.Status = "regressed"
			res.Violations = append(res.Violations, fmt.Sprintf("mean token ratio %.2fx exceeds budget %.2fx", ratio, budget.MaxTokenMultiplier))
		}
	}

	if budget.MaxCostMultiplier > 0 && baseline.MeanCostUSD != nil && *baseline.MeanCostUSD > 0 && candidate.MeanCostUSD != nil {
		ratio := *candidate.MeanCostUSD / *baseline.MeanCostUSD
		res.CostRatio = &ratio
		if ratio > budget.MaxCostMultiplier {
			res.Status = "regressed"
			res.Violations = append(res.Violations, fmt.Sprintf("mean cost ratio %.2fx exceeds budget %.2fx", ratio, budget.MaxCostMultiplier))
		}
	}
	return res
}

// RedactLogText strips local home paths, URL credentials/query secrets, Bearer headers, and secret KV pairs from eval logs.
func RedactLogText(raw string, extraSecrets ...string) string {
	if raw == "" {
		return ""
	}
	out := raw
	for _, s := range extraSecrets {
		if s != "" {
			out = strings.ReplaceAll(out, s, "<REDACTED>")
		}
	}
	for _, envKey := range []string{"HOME", "USERPROFILE"} {
		if h := os.Getenv(envKey); h != "" {
			out = replaceHomeVariants(out, h)
		}
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		out = replaceHomeVariants(out, h)
	}
	out = logURLRe.ReplaceAllStringFunc(out, sanitizeEvalURL)
	out = logBearerRe.ReplaceAllString(out, "${1}<REDACTED>")
	out = logKVRe.ReplaceAllString(out, "${1}=<REDACTED>")
	return out
}

func replaceHomeVariants(s, home string) string {
	for _, v := range []string{filepath.Clean(home), filepath.ToSlash(filepath.Clean(home))} {
		v = strings.TrimRight(v, `/\`)
		if len(v) > 1 {
			s = strings.ReplaceAll(s, v, "<HOME>")
		}
	}
	return s
}

func sanitizeEvalURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User != nil {
		u.User = nil
	}
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			kl := strings.ToLower(k)
			if strings.Contains(kl, "token") || strings.Contains(kl, "key") || strings.Contains(kl, "secret") || strings.Contains(kl, "sig") {
				q.Set(k, "REDACTED")
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// DigestArtifacts computes a deterministic digest over a map of relative path -> content bytes.
func DigestArtifacts(files map[string][]byte) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", k, len(files[k]))
		h.Write(files[k])
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
