package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/scout"
)

func runConverge(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("converge")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var targetPath, planPath, specPath, appendTasksPath string
	var dryRun, yes bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			targetPath = args[i]
		case "--plan":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--plan requires a markdown file path"))
			}
			planPath = args[i]
		case "--spec":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--spec requires a markdown file path"))
			}
			specPath = args[i]
		case "--append-tasks":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--append-tasks requires a markdown file path"))
			}
			appendTasksPath = args[i]
		case "--dry-run":
			dryRun = true
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected argument %q in converge", args[i])))
		}
	}

	if appendTasksPath != "" && !dryRun && !yes && (jsonOutput || !deps.IsInteractive()) {
		return fail(apperr.WithHint(
			apperr.Usage("confirmation_required", "non-interactive --append-tasks requires --yes; run --dry-run first"),
			"pass --dry-run to preview or --yes to append unmet items",
		))
	}

	rep, err := scout.AnalyzeConvergenceWithSpec(ctx, deps.Env, targetPath, planPath, specPath)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	env.Convergence = &rep

	tasksUpdated := false
	if appendTasksPath != "" && !dryRun {
		if !yes && deps.IsInteractive() {
			ok, cErr := confirmPrompt(ctx, deps, fmt.Sprintf("Append missing convergence items to %s?", appendTasksPath))
			if cErr != nil {
				return fail(cErr)
			}
			if !ok {
				return fail(apperr.Cancelled("converge append cancelled by user"))
			}
		}
		updated, aErr := scout.AppendConvergenceTasks(rep.Root, appendTasksPath, rep)
		if aErr != nil {
			return fail(aErr)
		}
		tasksUpdated = updated
	} else if appendTasksPath != "" && dryRun {
		tasksUpdated = len(rep.UnmetItems) > 0 || len(rep.MissingFiles) > 0 || len(rep.UnresolvedClarifications) > 0 || len(rep.UncoveredRequirements) > 0
	}
	if appendTasksPath != "" {
		env.Installed = &tasksUpdated
	}

	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprint(deps.Stdout, scout.FormatConvergenceMarkdown(rep))
	if appendTasksPath != "" {
		switch {
		case dryRun && tasksUpdated:
			fmt.Fprintf(deps.Stdout, "Preview: would append unmet items to %s (nothing written).\n", strings.TrimSpace(appendTasksPath))
		case tasksUpdated:
			fmt.Fprintf(deps.Stdout, "Appended convergence tasks to %s.\n", strings.TrimSpace(appendTasksPath))
		default:
			fmt.Fprintf(deps.Stdout, "No tasks appended to %s.\n", strings.TrimSpace(appendTasksPath))
		}
	}
	return nil
}
