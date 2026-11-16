package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
)

func runCreate(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("create")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit create <kit|skill> ..."))
	}
	sub := args[0]
	env.Command = "create " + sub

	switch sub {
	case "kit":
		if len(args) < 2 || args[1] == "" || args[1][0] == '-' {
			return fail(apperr.Usage("missing_kit_id", "usage: dtkit create kit <id> [--dir <parent>] [--name <title>] [--json]"))
		}
		id := args[1]
		var parentDir, displayName string
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--json":
				jsonOutput = true
			case "--verbose":
				deps.Verbose = true
			case "--dir", "--name":
				flag := args[i]
				i++
				if i >= len(args) {
					return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
				}
				if flag == "--dir" {
					parentDir = args[i]
				} else {
					displayName = args[i]
				}
			default:
				return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown create kit option %q", args[i])))
			}
		}
		if parentDir == "" {
			cwd, err := deps.Env.Getwd()
			if err != nil {
				cwd, err = os.Getwd()
			}
			if err != nil {
				return fail(err)
			}
			parentDir = cwd
		}
		res, err := dist.CreateKit(parentDir, id, displayName)
		if err != nil {
			return fail(err)
		}
		env.CreateKit = &res
		env.Kit = &KitSummary{ID: res.ID, Version: res.Version, Digest: res.Digest}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Created kit %s %s at %s (digest %s).\n", res.ID, res.Version, res.Dir, res.Digest[:12])
		return nil

	case "skill":
		if len(args) < 2 || args[1] == "" || args[1][0] == '-' {
			return fail(apperr.Usage("missing_skill_slug", "usage: dtkit create skill <slug> --kit-dir <dir> [--description <text>] [--json]"))
		}
		slug := args[1]
		var kitDir, desc string
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--json":
				jsonOutput = true
			case "--verbose":
				deps.Verbose = true
			case "--kit-dir", "--description":
				flag := args[i]
				i++
				if i >= len(args) {
					return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
				}
				if flag == "--kit-dir" {
					kitDir = args[i]
				} else {
					desc = args[i]
				}
			default:
				return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown create skill option %q", args[i])))
			}
		}
		if kitDir == "" {
			cwd, err := deps.Env.Getwd()
			if err != nil {
				cwd, err = os.Getwd()
			}
			if err != nil {
				return fail(err)
			}
			kitDir = cwd
		}
		res, err := dist.CreateSkill(kitDir, slug, desc)
		if err != nil {
			return fail(err)
		}
		env.CreateSkill = &res
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Created skill %s in %s (%s).\n", res.SkillID, res.File, res.Digest[:12])
		return nil

	default:
		_ = ctx
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown create subcommand %q (expected kit or skill)", sub)))
	}
}
