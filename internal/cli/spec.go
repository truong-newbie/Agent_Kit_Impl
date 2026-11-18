package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/scout"
)

func runSpec(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("spec")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return fail(apperr.Usage("missing_subcommand", "dtkit spec requires a subcommand: init | check"))
	}
	sub := args[0]
	rest := args[1:]

	var targetPath, title, fileArg, slugArg string
	var dryRun, yes bool
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--path":
			i++
			if i >= len(rest) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			targetPath = rest[i]
		case "--title":
			i++
			if i >= len(rest) {
				return fail(apperr.Usage("missing_flag_value", "--title requires text"))
			}
			title = rest[i]
		case "--spec":
			i++
			if i >= len(rest) {
				return fail(apperr.Usage("missing_flag_value", "--spec requires a markdown file path"))
			}
			fileArg = rest[i]
		case "--dry-run":
			dryRun = true
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonOutput = true
		default:
			if strings.HasPrefix(rest[i], "-") {
				return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q in spec %s", rest[i], sub)))
			}
			if sub == "init" && slugArg == "" {
				slugArg = rest[i]
			} else if sub == "check" && fileArg == "" {
				fileArg = rest[i]
			} else {
				return fail(apperr.Usage("invalid_argument", fmt.Sprintf("unexpected argument %q in spec %s", rest[i], sub)))
			}
		}
	}

	switch sub {
	case "init":
		if strings.TrimSpace(slugArg) == "" {
			return fail(apperr.Usage("missing_spec_slug", "usage: dtkit spec init <slug> [--path <dir>] [--title <text>] [--dry-run] [--yes] [--json]"))
		}
		if !dryRun && !yes && (jsonOutput || !deps.IsInteractive()) {
			return fail(apperr.WithHint(
				apperr.Usage("confirmation_required", "non-interactive spec init requires --yes; pass --dry-run to preview"),
				"pass --dry-run to preview or --yes to create the spec file",
			))
		}
		if !dryRun && !yes && deps.IsInteractive() {
			ok, cErr := confirmPrompt(ctx, deps, fmt.Sprintf("Create feature spec docs/specs/%s.md?", strings.TrimSpace(slugArg)))
			if cErr != nil {
				return fail(cErr)
			}
			if !ok {
				return fail(apperr.Cancelled("spec init cancelled by user"))
			}
		}
		relPath, _, err := scout.InitSpec(ctx, deps.Env, targetPath, slugArg, title, dryRun)
		if err != nil {
			return fail(err)
		}
		written := !dryRun
		env.DryRun = dryRun
		env.Installed = &written
		if dryRun {
			env.SpecReport = &scout.SpecReport{SpecPath: relPath, Status: "preview"}
		} else {
			rep, inspectErr := scout.InspectSpec(ctx, deps.Env, targetPath, relPath)
			if inspectErr != nil {
				return fail(inspectErr)
			}
			env.SpecReport = &rep
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		if dryRun {
			fmt.Fprintf(deps.Stdout, "Preview: would create %s (nothing written).\n", relPath)
		} else {
			fmt.Fprintf(deps.Stdout, "Created feature spec at %s.\n", relPath)
		}
		return nil

	case "check":
		rep, err := scout.InspectSpec(ctx, deps.Env, targetPath, fileArg)
		if err != nil {
			return fail(err)
		}
		env.SpecReport = &rep
		if rep.Status == "needs_clarification" || rep.Status == "incomplete" {
			err := apperr.WithHint(
				apperr.Conflict("spec_validation_failed", fmt.Sprintf("spec %s status is %s", rep.SpecPath, rep.Status)),
				"resolve all [NEEDS CLARIFICATION] markers and supply P1/P2 user stories, Given/When/Then scenarios, FR-xxx, and SC-xxx criteria",
			)
			if jsonOutput {
				return failJSON(deps.Stdout, env, err)
			}
			fmt.Fprint(deps.Stdout, scout.FormatSpecMarkdown(rep))
			return err
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprint(deps.Stdout, scout.FormatSpecMarkdown(rep))
		return nil

	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown spec subcommand %q (expected init|check)", sub)))
	}
}
