package cli

import (
	"context"
	"fmt"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
)

func runRecover(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("recover")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var scope, projectPath, runtimeArg, txID string
	var dryRun, yes bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "project"
		case "--global":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "global"
		case "--dry-run":
			dryRun = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--runtime", "--path", "--transaction":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--runtime":
				runtimeArg = args[i]
			case "--path":
				projectPath = args[i]
			default:
				txID = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown recover option %q", args[i])))
		}
	}

	profile, rtErr := resolveRuntimeProfile(runtimeArg)
	if rtErr != nil {
		return fail(rtErr)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit recover --project --dry-run"))
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}

	roots, rootWarnings, err := resolveScopeRoots(ctx, deps, profile, scope, projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, rootWarnings...)
	env.Roots = roots

	if dryRun {
		report, err := installer.RecoverForRuntime(profile.ID, scope, roots, txID, true, deps.Now())
		env.Recovery = &report
		if err != nil {
			return fail(err)
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Recovery preview (%s): %s (%s)\n", scope, report.Outcome, report.Detail)
		return nil
	}

	// Check read-only first so no-op doesn't prompt.
	preview, err := installer.RecoverForRuntime(profile.ID, scope, roots, txID, true, deps.Now())
	if err != nil {
		env.Recovery = &preview
		return fail(err)
	}
	if preview.Outcome == "noop" {
		env.Recovery = &preview
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Recovery (%s): %s\n", scope, preview.Detail)
		return nil
	}

	if jsonOutput && !yes {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "--json mutation requires --yes"), "pass --dry-run to preview or --yes to apply"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive recover requires --yes; run --dry-run first"), "run with --dry-run first, then pass --yes"))
	}
	if !yes {
		confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Recover transaction %s (%s)?", preview.TransactionID, preview.Outcome))
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("recovery cancelled by user"))
		}
	}

	if ctx.Err() != nil {
		return fail(apperr.Cancelled("recovery cancelled"))
	}

	report, err := installer.RecoverForRuntime(profile.ID, scope, roots, txID, false, deps.Now())
	env.Recovery = &report
	if err != nil {
		return fail(err)
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Recovered (%s): %s (%s)\n", scope, report.Outcome, report.Detail)
	return nil
}
