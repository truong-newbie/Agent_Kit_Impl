package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

func runProject(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("project")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit project <inspect|init> [options]"))
	}
	sub := args[0]
	env.Command = "project " + sub

	var projectPath string
	var dryRun, yes bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a value"))
			}
			projectPath = args[i]
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--dry-run":
			if sub == "inspect" {
				return fail(apperr.Usage("invalid_flag", "project inspect is always read-only and does not take --dry-run"))
			}
			dryRun = true
		case "--yes", "-y":
			if sub == "inspect" {
				return fail(apperr.Usage("invalid_flag", "project inspect is read-only and does not take --yes"))
			}
			yes = true
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown project %s option %q", sub, args[i])))
		}
	}

	switch sub {
	case "inspect":
		res, err := project.Inspect(ctx, deps.Env, projectPath)
		if err != nil {
			return fail(err)
		}
		env.ProjectInspect = &res
		env.Warnings = append(env.Warnings, res.Warnings...)
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Project root: %s\n", res.Root)
		fmt.Fprintf(deps.Stdout, "Profile file: %s (exists=%v, checks=%d, trustDigest=%s)\n", res.ConfigPath, res.ProfileExists, len(res.Profile.Checks), res.TrustDigest[:12])
		for _, r := range res.Profile.Roots {
			fmt.Fprintf(deps.Stdout, "  - root %s stack=%s (%s)\n", r.Path, r.Stack, strings.Join(r.Evidence, ", "))
		}
		for _, c := range res.Profile.Checks {
			fmt.Fprintf(deps.Stdout, "  - check %s [%s] cwd=%s: %s %s\n", c.ID, c.Kind, c.Cwd, c.Executable, strings.Join(c.Argv, " "))
		}
		return nil

	case "init":
		if dryRun && yes {
			return fail(apperr.Usage("conflicting_flags", "choose --dry-run or --yes, not both"))
		}
		preview, err := project.Init(ctx, deps.Env, project.InitOptions{Path: projectPath, DryRun: true})
		if err != nil {
			return fail(err)
		}
		if dryRun {
			env.DryRun = true
			env.ProjectInit = &preview
			if jsonOutput {
				return writeEnvelope(deps.Stdout, env)
			}
			fmt.Fprintf(deps.Stdout, "Read-only preview for %s (not written):\n%s", preview.ConfigPath, preview.PreviewJSON)
			return nil
		}
		if jsonOutput && !yes {
			return fail(apperr.WithHint(apperr.Usage("confirmation_required", "--json mutation requires --yes"), "pass --dry-run to preview or --yes to apply"))
		}
		if !yes && !deps.IsInteractive() {
			return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive project init requires --yes; run --dry-run first"), "run with --dry-run first, then pass --yes"))
		}
		if !yes {
			confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Create %s at %s?", project.ProfileRelPath, preview.ConfigPath))
			if err != nil {
				return fail(err)
			}
			if !confirmed {
				return fail(apperr.Cancelled("project init cancelled by user"))
			}
		}
		applied, err := project.Init(ctx, deps.Env, project.InitOptions{Path: projectPath, DryRun: false})
		if err != nil {
			return fail(err)
		}
		env.ProjectInit = &applied
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Created %s (checks=%d, trustDigest=%s).\n", applied.ConfigPath, len(applied.Profile.Checks), applied.TrustDigest[:12])
		return nil

	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown project subcommand %q (supported: inspect, init)", sub)))
	}
}
