// Package check executes repository quality checks defined in .dtkit/project.json
// with trust invalidation, argv-only invocation, bounded output, overlap locks, and process-tree timeouts.
package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

const (
	StatusPass        = "pass"
	StatusFail        = "fail"
	StatusSkipped     = "skipped"
	StatusMissingTool = "missing_tool"
	StatusTimeout     = "timeout"
	StatusCancelled   = "cancelled"

	MaxOutputBytes = 64 * 1024
)

var kindOrder = map[string]int{
	"format":    1,
	"lint":      2,
	"typecheck": 3,
	"build":     4,
	"test":      5,
	"custom":    6,
}

// Options configures dtkit check execution.
type Options struct {
	Path         string
	DryRun       bool
	ChangedOnly  bool
	All          bool
	Yes          bool
	AllowAutofix bool
	ExtraEnv     []string
	LookPath     func(string) (string, error)
	Now          func() time.Time
}

// CheckResult records the outcome of one configured check.
type CheckResult struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Cwd        string   `json:"cwd"`
	Executable string   `json:"executable"`
	Argv       []string `json:"argv"`
	Status     string   `json:"status"`
	ExitCode   int      `json:"exitCode"`
	DurationMs int64    `json:"durationMs"`
	Reason     string   `json:"reason,omitempty"`
	Output     string   `json:"output,omitempty"`
	Truncated  bool     `json:"truncated,omitempty"`
}

// Summary aggregates exact status counts without hiding missing tools or timeouts.
type Summary struct {
	Total       int `json:"total"`
	Pass        int `json:"pass"`
	Fail        int `json:"fail"`
	MissingTool int `json:"missingTool"`
	Timeout     int `json:"timeout"`
	Cancelled   int `json:"cancelled"`
	Skipped     int `json:"skipped"`
}

// Report is the structured output of dtkit check.
type Report struct {
	Root        string            `json:"root"`
	Mode        string            `json:"mode"` // changed | all
	DryRun      bool              `json:"dryRun"`
	Trusted     bool              `json:"trusted"`
	TrustDigest string            `json:"trustDigest"`
	Ok          bool              `json:"ok"`
	Changed     *ChangedSelection `json:"changed,omitempty"`
	Results     []CheckResult     `json:"results"`
	Summary     Summary           `json:"summary"`
	Warnings    []string          `json:"warnings,omitempty"`
}

// Run validates .dtkit/project.json, checks trust confirmation, selects checks, and executes them safely.
func Run(ctx context.Context, env paths.Environment, opts Options) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, apperr.Cancelled("check cancelled")
	}
	if opts.ChangedOnly && opts.All {
		return Report{}, apperr.Usage("conflicting_flags", "choose at most one of --changed or --all")
	}
	mode := "changed"
	if opts.All {
		mode = "all"
	}
	lookPathFn := opts.LookPath
	if lookPathFn == nil {
		lookPathFn = exec.LookPath
	}
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	projRes, err := paths.ResolveProject(ctx, env, opts.Path)
	if err != nil {
		return Report{}, err
	}
	root := projRes.Root

	prof, _, exists, err := project.LoadProfile(root)
	if err != nil {
		return Report{}, err
	}
	if !exists {
		return Report{}, apperr.WithHint(
			apperr.WithPath(
				apperr.Conflict("missing_project_profile", fmt.Sprintf("%s not found; initialize a project profile first", project.ProfileRelPath)),
				project.ProfileRelPath,
			),
			"run 'dtkit project inspect --path .' then 'dtkit project init --path . --yes'",
		)
	}

	trustInfo, err := InspectTrust(root, prof)
	if err != nil {
		return Report{}, err
	}

	rep := Report{
		Root:        root,
		Mode:        mode,
		DryRun:      opts.DryRun,
		Trusted:     trustInfo.Trusted,
		TrustDigest: trustInfo.CurrentDigest,
		Ok:          true,
		Results:     []CheckResult{},
		Warnings:    append([]string{}, projRes.Warnings...),
	}

	var changedSel ChangedSelection
	selectedRootSet := map[string]bool{}
	if mode == "changed" {
		changedSel = DetectChangedSelection(ctx, root, prof)
		rep.Changed = &changedSel
		if changedSel.FallbackReason != "" {
			rep.Warnings = append(rep.Warnings, changedSel.FallbackReason)
		}
		for _, r := range changedSel.MatchedRoots {
			selectedRootSet[r] = true
		}
	}

	ordered := append([]project.CheckSpec(nil), prof.Checks...)
	sort.Slice(ordered, func(i, j int) bool {
		ki := kindOrder[ordered[i].Kind]
		kj := kindOrder[ordered[j].Kind]
		if ki != kj {
			return ki < kj
		}
		return ordered[i].ID < ordered[j].ID
	})

	type plannedCheck struct {
		spec   project.CheckSpec
		run    bool
		reason string
	}
	plans := make([]plannedCheck, 0, len(ordered))
	anyRunnable := false
	for _, c := range ordered {
		if c.MutatesFiles && !opts.AllowAutofix {
			plans = append(plans, plannedCheck{spec: c, run: false, reason: "mutatesFiles requires --allow-autofix"})
			continue
		}
		if mode == "changed" && !changedSel.FallbackAll {
			normCwd := path.Clean(strings.ReplaceAll(c.Cwd, `\`, "/"))
			if len(changedSel.ChangedPaths) == 0 {
				plans = append(plans, plannedCheck{spec: c, run: false, reason: "no working-tree changes detected"})
				continue
			}
			if !selectedRootSet[normCwd] {
				plans = append(plans, plannedCheck{spec: c, run: false, reason: "workspace untouched in changed files"})
				continue
			}
		}
		anyRunnable = true
		plans = append(plans, plannedCheck{spec: c, run: true})
	}

	if opts.DryRun {
		for _, p := range plans {
			res := CheckResult{
				ID:         p.spec.ID,
				Kind:       p.spec.Kind,
				Cwd:        p.spec.Cwd,
				Executable: p.spec.Executable,
				Argv:       append([]string{}, p.spec.Argv...),
				Status:     StatusSkipped,
				Reason:     p.reason,
			}
			if p.run {
				res.Reason = "dry_run"
			}
			rep.Results = append(rep.Results, res)
			rep.Summary.Total++
			rep.Summary.Skipped++
		}
		return rep, nil
	}

	if anyRunnable && !trustInfo.Trusted {
		if !opts.Yes {
			return Report{}, apperr.WithHint(
				apperr.Conflict("untrusted_project_checks", "project check commands in .dtkit/project.json have not been trusted or changed since last approval"),
				"review checks via 'dtkit check --dry-run' then pass '--yes' to trust and execute",
			)
		}
		if err := RecordTrust(root, prof, nowFn()); err != nil {
			return Report{}, err
		}
		rep.Trusted = true
	}

	var releaseLock func()
	if anyRunnable {
		releaseLock, err = acquireCheckLock(root, nowFn())
		if err != nil {
			return Report{}, err
		}
		defer releaseLock()
	}

	for _, p := range plans {
		rep.Summary.Total++
		if !p.run {
			rep.Results = append(rep.Results, CheckResult{
				ID:         p.spec.ID,
				Kind:       p.spec.Kind,
				Cwd:        p.spec.Cwd,
				Executable: p.spec.Executable,
				Argv:       append([]string{}, p.spec.Argv...),
				Status:     StatusSkipped,
				Reason:     p.reason,
			})
			rep.Summary.Skipped++
			continue
		}

		res := executeOneCheck(ctx, root, p.spec, opts.ExtraEnv, lookPathFn)
		rep.Results = append(rep.Results, res)
		switch res.Status {
		case StatusPass:
			rep.Summary.Pass++
		case StatusFail:
			rep.Summary.Fail++
			rep.Ok = false
		case StatusMissingTool:
			rep.Summary.MissingTool++
			rep.Ok = false
		case StatusTimeout:
			rep.Summary.Timeout++
			rep.Ok = false
		case StatusCancelled:
			rep.Summary.Cancelled++
			rep.Ok = false
		default:
			rep.Summary.Skipped++
		}
	}

	return rep, nil
}

func executeOneCheck(ctx context.Context, repoRoot string, spec project.CheckSpec, extraEnv []string, lookPathFn func(string) (string, error)) CheckResult {
	res := CheckResult{
		ID:         spec.ID,
		Kind:       spec.Kind,
		Cwd:        spec.Cwd,
		Executable: spec.Executable,
		Argv:       append([]string{}, spec.Argv...),
	}

	if ctx.Err() != nil {
		res.Status = StatusCancelled
		res.Reason = "cancelled before start"
		return res
	}

	workDir := repoRoot
	if spec.Cwd != "." {
		resolvedCwd, err := paths.ResolveTarget(repoRoot, spec.Cwd)
		if err != nil {
			res.Status = StatusFail
			res.ExitCode = 1
			res.Reason = err.Error()
			return res
		}
		info, statErr := os.Lstat(resolvedCwd)
		if statErr != nil || paths.IsReparseOrSymlink(info) || !info.IsDir() {
			res.Status = StatusFail
			res.ExitCode = 1
			res.Reason = fmt.Sprintf("invalid working directory %q", spec.Cwd)
			return res
		}
		workDir = resolvedCwd
	}

	exePath, lpErr := lookPathFn(spec.Executable)
	if lpErr != nil || exePath == "" {
		res.Status = StatusMissingTool
		res.ExitCode = 127
		res.Reason = fmt.Sprintf("executable %q not found on PATH", spec.Executable)
		return res
	}

	timeout := time.Duration(spec.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = time.Duration(project.DefaultTimeoutSec) * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.Command(exePath, spec.Argv...)
	cmd.Dir = workDir
	cmd.Env = append(filteredCheckEnv(spec.AllowEnv), extraEnv...)
	configureCommandProcess(cmd)

	buf := &boundedBuffer{limit: MaxOutputBytes}
	cmd.Stdout = buf
	cmd.Stderr = buf

	start := time.Now()
	if err := cmd.Start(); err != nil {
		res.DurationMs = time.Since(start).Milliseconds()
		res.Status = StatusFail
		res.ExitCode = 1
		res.Reason = err.Error()
		return res
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	var waitErr error
	timedOut := false
	cancelled := false
	select {
	case waitErr = <-waitDone:
	case <-runCtx.Done():
		if errors.Is(ctx.Err(), context.Canceled) {
			cancelled = true
		} else {
			timedOut = true
		}
		killProcessTree(cmd)
		select {
		case waitErr = <-waitDone:
		case <-time.After(2 * time.Second):
		}
	}

	res.DurationMs = time.Since(start).Milliseconds()
	res.Output, res.Truncated = buf.String()

	if cancelled {
		res.Status = StatusCancelled
		res.ExitCode = 130
		res.Reason = "check cancelled"
		return res
	}
	if timedOut {
		res.Status = StatusTimeout
		res.ExitCode = 124
		res.Reason = fmt.Sprintf("check timed out after %s", timeout)
		return res
	}
	if waitErr != nil {
		res.Status = StatusFail
		res.ExitCode = 1
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			res.ExitCode = ee.ExitCode()
		}
		res.Reason = waitErr.Error()
		return res
	}

	res.Status = StatusPass
	res.ExitCode = 0
	return res
}

func filteredCheckEnv(allowExtra []string) []string {
	safeBase := map[string]bool{
		"PATH": true, "HOME": true, "USERPROFILE": true, "SYSTEMROOT": true, "WINDIR": true,
		"TMP": true, "TEMP": true, "TMPDIR": true, "GOPATH": true, "GOMODCACHE": true, "GOCACHE": true,
		"GOTMPDIR": true, "LANG": true, "LC_ALL": true, "TERM": true, "PATHEXT": true,
	}
	allowed := map[string]bool{}
	for k := range safeBase {
		allowed[k] = true
	}
	for _, k := range allowExtra {
		allowed[strings.ToUpper(k)] = true
	}
	var env []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || !allowed[strings.ToUpper(name)] {
			continue
		}
		env = append(env, kv)
	}
	return env
}

type boundedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	origLen := len(p)
	if b.buf.Len() < b.limit {
		rem := b.limit - b.buf.Len()
		if len(p) > rem {
			b.buf.Write(p[:rem])
			b.truncated = true
		} else {
			b.buf.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return origLen, nil
}

func (b *boundedBuffer) String() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.buf.String()
	if b.truncated {
		s += "\n... [truncated]"
	}
	return s, b.truncated
}
