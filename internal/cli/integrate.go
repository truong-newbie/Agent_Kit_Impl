package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/companion"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runIntegrate(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("integrate")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit integrate <list|import|tool|run> [options]"))
	}
	sub := args[0]
	env.Command = "integrate " + sub

	switch sub {
	case "list":
		return runIntegrateList(args[1:], deps, env, jsonOutput, fail)
	case "import":
		return runIntegrateImport(ctx, args[1:], deps, env, jsonOutput, fail)
	case "tool":
		return runIntegrateTool(ctx, args[1:], deps, env, jsonOutput, fail)
	case "run":
		return runIntegrateRun(ctx, args[1:], deps, env, jsonOutput, fail)
	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown integrate subcommand %q (expected list|import|tool|run)", sub)))
	}
}

func runIntegrateList(args []string, deps Deps, env Envelope, jsonOutput bool, fail func(error) error) error {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q for dtkit integrate list", args[i])))
		}
	}
	statuses := companion.InspectCompanions(nil)
	if jsonOutput {
		env.Companions = statuses
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintln(deps.Stdout, "DTKit External Companions:")
	for _, st := range statuses {
		avail := "missing (fallback: " + st.Spec.Fallback + ")"
		if st.Available {
			avail = "available (" + st.ResolvedPath + ")"
		}
		fmt.Fprintf(deps.Stdout, "  - %-10s [%s] %s — %s\n", st.Spec.ID, st.Spec.Category, avail, st.Spec.Description)
	}
	return nil
}

func runIntegrateImport(ctx context.Context, args []string, deps Deps, env Envelope, jsonOutput bool, fail func(error) error) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fail(apperr.Usage("missing_source", "usage: dtkit integrate import <source-dir> --as <kit-id> [options]"))
	}
	srcDir := args[0]

	var kitID, outParent, version, title, runtimeArg, scope, projectPath string
	var doInstall, dryRun, redact, yes bool

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--as":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--as requires a kit ID"))
			}
			i++
			kitID = args[i]
		case "--dir":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--dir requires a parent directory"))
			}
			i++
			outParent = args[i]
		case "--version":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--version requires X.Y.Z"))
			}
			i++
			version = args[i]
		case "--name":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--name requires a display name"))
			}
			i++
			title = args[i]
		case "--runtime":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a runtime ID"))
			}
			i++
			runtimeArg = args[i]
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
			projectPath = args[i]
		case "--project":
			scope = "project"
		case "--global":
			scope = "global"
		case "--install":
			doInstall = true
		case "--dry-run":
			dryRun = true
		case "--redact":
			redact = true
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q for dtkit integrate import", args[i])))
		}
	}

	if strings.TrimSpace(kitID) == "" {
		return fail(apperr.Usage("missing_kit_id", "dtkit integrate import requires --as <kit-id>"))
	}
	if outParent == "" {
		projRes, err := paths.ResolveProject(ctx, deps.Env, projectPath)
		if err != nil {
			return fail(err)
		}
		outParent = filepath.Join(projRes.Root, ".dtkit", "imported")
	}

	impRes, err := dist.ImportKit(dist.ImportOptions{
		SourceDir:       srcDir,
		TargetParentDir: outParent,
		KitID:           kitID,
		Name:            title,
		Version:         version,
		DryRun:          dryRun,
	})
	if err != nil {
		return fail(err)
	}
	if redact {
		impRes.OutputDir = redactDiagnosticText(impRes.OutputDir, collectHomeCandidates(deps))
	}

	if doInstall && !dryRun {
		if scope == "" {
			scope = "project"
		}
		instArgs := []string{kitID, "--from", impRes.OutputDir, "--" + scope}
		if runtimeArg != "" {
			instArgs = append(instArgs, "--runtime", runtimeArg)
		}
		if projectPath != "" {
			instArgs = append(instArgs, "--path", projectPath)
		}
		if yes {
			instArgs = append(instArgs, "--yes")
		}
		subDeps := deps
		subDeps.Stdout = ioDiscard{}
		if iErr := runInstall(ctx, instArgs, subDeps); iErr != nil {
			return fail(iErr)
		}
	}

	env.DryRun = dryRun
	env.Import = &impRes
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Imported kit %s@%s (%s, %d items) -> %s\n", impRes.KitID, impRes.Version, impRes.DetectedFormat, len(impRes.Items), impRes.OutputDir)
	return nil
}

func runIntegrateTool(ctx context.Context, args []string, deps Deps, env Envelope, jsonOutput bool, fail func(error) error) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fail(apperr.Usage("missing_tool_id", "usage: dtkit integrate tool <repomix|graphify|ast-grep|semgrep> [--path <dir>] [--dry-run] [--yes] [--json]"))
	}
	toolID := args[0]
	var targetPath string
	var dryRun, yes bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
			targetPath = args[i]
		case "--dry-run":
			dryRun = true
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q for dtkit integrate tool", args[i])))
		}
	}
	if !dryRun && !yes && (jsonOutput || !deps.IsInteractive()) {
		return fail(apperr.Usage("confirmation_required", "pass --yes to register companion check in .dtkit/project.json"))
	}
	prof, added, err := companion.RegisterProjectCheck(ctx, deps.Env, targetPath, toolID, dryRun)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	if jsonOutput {
		_ = prof
		_ = added
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Registered companion tool %q (added=%v, totalChecks=%d)\n", toolID, added, len(prof.Checks))
	return nil
}

func runIntegrateRun(ctx context.Context, args []string, deps Deps, env Envelope, jsonOutput bool, fail func(error) error) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fail(apperr.Usage("missing_tool_id", "usage: dtkit integrate run <repomix|graphify|ast-grep|semgrep> [--path <dir>] [--zone <subpath>] [--no-cache] [--json]"))
	}
	toolID := args[0]
	var targetPath, zone string
	noCache := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
			targetPath = args[i]
		case "--zone":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--zone requires a subdirectory"))
			}
			i++
			zone = args[i]
		case "--no-cache":
			noCache = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q for dtkit integrate run", args[i])))
		}
	}
	res, err := companion.RunCompanion(ctx, deps.Env, targetPath, toolID, zone, noCache, nil)
	if err != nil {
		return fail(err)
	}
	env.CompanionRun = &res
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Companion %s status=%s cacheHit=%v digest=%s\n", res.ToolID, res.Status, res.CacheHit, res.WorkspaceDigest)
	if res.FallbackUsed != "" {
		fmt.Fprintf(deps.Stdout, "Fallback used: %s\n", res.FallbackUsed)
	}
	if res.Output != "" {
		fmt.Fprintln(deps.Stdout, res.Output)
	}
	return nil
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
