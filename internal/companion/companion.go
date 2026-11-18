// Package companion bridges external developer/agent CLI tools (repomix, graphify, ast-grep, semgrep)
// with SHA-256 workspace-digest caching, secret redaction, and native DTKit scout fallbacks.
package companion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/guard"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/scout"
)

const (
	MaxCompanionOutputBytes = 64 * 1024
	DefaultCompanionTimeout = 60 * time.Second
	cacheDirRel             = ".dtkit/local/companion-cache"
)

// CompanionSpec defines a safe argv-only external companion blueprint.
type CompanionSpec struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"` // "context" | "analyzer"
	Executable  string   `json:"executable"`
	DefaultArgv []string `json:"defaultArgv"`
	Description string   `json:"description"`
	Fallback    string   `json:"fallback"`
}

// CompanionStatus reports whether a companion executable is available on PATH.
type CompanionStatus struct {
	Spec         CompanionSpec `json:"spec"`
	Available    bool          `json:"available"`
	ResolvedPath string        `json:"resolvedPath,omitempty"`
}

// RunResult captures the output of a companion invocation or its workspace-digest cache / fallback.
type RunResult struct {
	ToolID          string `json:"toolId"`
	Zone            string `json:"zone,omitempty"`
	Status          string `json:"status"` // "ok" | "cached" | "missing_tool" | "failed"
	CacheHit        bool   `json:"cacheHit"`
	WorkspaceDigest string `json:"workspaceDigest"`
	Output          string `json:"output,omitempty"`
	FallbackUsed    string `json:"fallbackUsed,omitempty"`
}

// BuiltInSpecs returns the curated external companion blueprints.
func BuiltInSpecs() []CompanionSpec {
	return []CompanionSpec{
		{
			ID:          "repomix",
			Category:    "context",
			Executable:  "repomix",
			DefaultArgv: []string{"--stdout", "--style", "markdown", "--no-security-check"},
			Description: "Pack repository or zone files into a single AI-friendly markdown bundle.",
			Fallback:    "dtkit scout --pack",
		},
		{
			ID:          "graphify",
			Category:    "context",
			Executable:  "graphify",
			DefaultArgv: []string{"query"},
			Description: "Query local structural AST knowledge graph for codebase orientation.",
			Fallback:    "dtkit scout --write-summary",
		},
		{
			ID:          "ast-grep",
			Category:    "analyzer",
			Executable:  "sg",
			DefaultArgv: []string{"scan"},
			Description: "Run structural AST lint and security rules via ast-grep (sg).",
			Fallback:    "dtkit check",
		},
		{
			ID:          "semgrep",
			Category:    "analyzer",
			Executable:  "semgrep",
			DefaultArgv: []string{"scan", "--error", "--quiet"},
			Description: "Run polyglot static security and bug analysis via Semgrep.",
			Fallback:    "dtkit check",
		},
	}
}

// LookupSpec finds a companion spec by ID.
func LookupSpec(id string) (CompanionSpec, bool) {
	norm := strings.ToLower(strings.TrimSpace(id))
	for _, s := range BuiltInSpecs() {
		if s.ID == norm {
			return s, true
		}
	}
	return CompanionSpec{}, false
}

// InspectCompanions checks availability of all built-in companion tools on PATH.
func InspectCompanions(lookPathFn func(string) (string, error)) []CompanionStatus {
	if lookPathFn == nil {
		lookPathFn = exec.LookPath
	}
	specs := BuiltInSpecs()
	out := make([]CompanionStatus, 0, len(specs))
	for _, s := range specs {
		resolved, err := lookPathFn(s.Executable)
		st := CompanionStatus{Spec: s}
		if err == nil && resolved != "" {
			st.Available = true
			st.ResolvedPath = resolved
		}
		out = append(out, st)
	}
	return out
}

// RegisterProjectCheck adds a safe argv check entry for an analyzer/companion tool into .dtkit/project.json
// without auto-approving .dtkit/local/check-trust.json.
func RegisterProjectCheck(ctx context.Context, env paths.Environment, repoRoot, toolID string, dryRun bool) (project.Profile, bool, error) {
	spec, ok := LookupSpec(toolID)
	if !ok {
		return project.Profile{}, false, apperr.Usage("unknown_companion", fmt.Sprintf("unknown companion tool %q (supported: repomix, graphify, ast-grep, semgrep)", toolID))
	}
	projRes, err := paths.ResolveProject(ctx, env, repoRoot)
	if err != nil {
		return project.Profile{}, false, err
	}
	root := projRes.Root

	prof, _, exists, err := project.LoadProfile(root)
	if err != nil {
		return project.Profile{}, false, err
	}
	if !exists {
		insp, iErr := project.Inspect(ctx, env, root)
		if iErr != nil {
			return project.Profile{}, false, iErr
		}
		prof = insp.Profile
	}

	checkID := "companion-" + spec.ID
	for _, c := range prof.Checks {
		if c.ID == checkID {
			return prof, false, nil
		}
	}
	kind := "custom"
	if spec.Category == "analyzer" {
		kind = "lint"
	}
	prof.Checks = append(prof.Checks, project.CheckSpec{
		ID:         checkID,
		Kind:       kind,
		Executable: spec.Executable,
		Argv:       append([]string(nil), spec.DefaultArgv...),
		Cwd:        ".",
		TimeoutSec: 120,
	})
	if err := project.ValidateProfile(prof); err != nil {
		return project.Profile{}, false, err
	}
	if dryRun {
		return prof, true, nil
	}

	cfgPath, err := paths.ResolveTarget(root, project.ProfileRelPath)
	if err != nil {
		return project.Profile{}, false, err
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		return project.Profile{}, false, err
	}
	data, err := json.MarshalIndent(prof, "", "  ")
	if err != nil {
		return project.Profile{}, false, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return project.Profile{}, false, err
	}
	return prof, true, nil
}

// RunCompanion executes a companion tool (or returns its workspace-digest cached output / native scout fallback).
func RunCompanion(ctx context.Context, env paths.Environment, repoRoot, toolID, zone string, noCache bool, lookPathFn func(string) (string, error)) (RunResult, error) {
	spec, ok := LookupSpec(toolID)
	if !ok {
		return RunResult{}, apperr.Usage("unknown_companion", fmt.Sprintf("unknown companion tool %q (supported: repomix, graphify, ast-grep, semgrep)", toolID))
	}
	if lookPathFn == nil {
		lookPathFn = exec.LookPath
	}
	projRes, err := paths.ResolveProject(ctx, env, repoRoot)
	if err != nil {
		return RunResult{}, err
	}
	root := projRes.Root
	cleanZone := strings.Trim(path.Clean(strings.ReplaceAll(strings.TrimSpace(zone), `\`, "/")), "/")
	if cleanZone == "." || cleanZone == ".." || strings.HasPrefix(cleanZone, "../") {
		cleanZone = ""
	}

	digest, _, err := guard.ComputeWorkspaceStateDigest(root)
	if err != nil {
		return RunResult{}, err
	}
	cacheKey := computeCacheKey(spec.ID, cleanZone, digest)

	if !noCache {
		if cached, hit := readCache(root, cacheKey); hit {
			cached.Status = "cached"
			cached.CacheHit = true
			return cached, nil
		}
	}

	exePath, lpErr := lookPathFn(spec.Executable)
	if lpErr != nil || exePath == "" {
		res := RunResult{
			ToolID:          spec.ID,
			Zone:            cleanZone,
			Status:          "missing_tool",
			WorkspaceDigest: digest,
			FallbackUsed:    spec.Fallback,
		}
		if spec.Category == "context" {
			opts := scout.Options{Root: root, IncludeContent: spec.ID == "repomix"}
			if cleanZone != "" {
				opts.Paths = []string{cleanZone}
			}
			if sRep, sErr := scout.Run(ctx, env, opts); sErr == nil {
				res.Output = scout.FormatMarkdownSummary(sRep)
			}
		}
		return res, nil
	}

	argv := append([]string(nil), spec.DefaultArgv...)
	if spec.ID == "repomix" && cleanZone != "" {
		argv = append(argv, "--include", cleanZone+"/**")
	} else if spec.ID == "graphify" && cleanZone != "" {
		argv = append(argv, cleanZone)
	}

	runCtx, cancel := context.WithTimeout(ctx, DefaultCompanionTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, exePath, argv...)
	cmd.Dir = root
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	runErr := cmd.Run()
	rawOut := outBuf.String()
	if len(rawOut) > MaxCompanionOutputBytes {
		rawOut = rawOut[:MaxCompanionOutputBytes] + "\n... [truncated]"
	}
	redacted := scout.RedactText(rawOut)

	if runErr != nil {
		return RunResult{
			ToolID:          spec.ID,
			Zone:            cleanZone,
			Status:          "failed",
			WorkspaceDigest: digest,
			Output:          redacted,
		}, apperr.Wrap(apperr.ExitInternal, "companion_failed", fmt.Sprintf("companion %s failed: %v", spec.ID, runErr), runErr)
	}

	res := RunResult{
		ToolID:          spec.ID,
		Zone:            cleanZone,
		Status:          "ok",
		CacheHit:        false,
		WorkspaceDigest: digest,
		Output:          redacted,
	}
	_ = writeCache(root, cacheKey, res)
	return res, nil
}

func computeCacheKey(toolID, zone, digest string) string {
	sum := sha256.Sum256([]byte(toolID + "|" + zone + "|" + digest))
	return toolID + "-" + hex.EncodeToString(sum[:12])
}

func readCache(repoRoot, key string) (RunResult, bool) {
	rel := cacheDirRel + "/" + key + ".json"
	target, err := paths.ResolveTarget(repoRoot, rel)
	if err != nil {
		return RunResult{}, false
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return RunResult{}, false
	}
	var res RunResult
	if json.Unmarshal(raw, &res) != nil {
		return RunResult{}, false
	}
	return res, true
}

func writeCache(repoRoot, key string, res RunResult) error {
	rel := cacheDirRel + "/" + key + ".json"
	target, err := paths.ResolveTarget(repoRoot, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	if info, lErr := os.Lstat(target); lErr == nil && paths.IsReparseOrSymlink(info) {
		return errors.New("refusing symlinked companion cache")
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(target, append(data, '\n'), 0600)
}
