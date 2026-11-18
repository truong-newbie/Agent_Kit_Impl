package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/mcp"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runMCP(ctx context.Context, args []string, deps Deps) error {
	if len(args) == 0 {
		return apperr.Usage("missing_subcommand", "usage: dtkit mcp <serve|status|enable|disable> [options]")
	}
	sub := args[0]
	if sub == "serve" {
		var targetPath string
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--path":
				if i+1 >= len(args) {
					return apperr.Usage("missing_flag_value", "--path requires a directory")
				}
				i++
				targetPath = args[i]
			default:
				return apperr.Usage("invalid_flag", fmt.Sprintf("unexpected argument %q in dtkit mcp serve", args[i]))
			}
		}
		projRes, err := paths.ResolveProject(ctx, deps.Env, targetPath)
		if err != nil {
			return err
		}
		return mcp.Serve(ctx, deps.Env, projRes.Root, deps.Stdin, deps.Stdout)
	}

	env := newEnvelope("mcp " + sub)
	jsonOutput := hasJSONFlag(args[1:])
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var targetPath, runtimeArg, exePath string
	allRuntimes := false
	dryRun := false
	yes := false

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
			targetPath = args[i]
		case "--runtime":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a runtime name"))
			}
			i++
			runtimeArg = args[i]
		case "--executable":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--executable requires an executable path"))
			}
			i++
			exePath = args[i]
		case "--all-runtimes":
			allRuntimes = true
		case "--dry-run":
			dryRun = true
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q for dtkit mcp %s", args[i], sub)))
		}
	}

	if allRuntimes && runtimeArg != "" {
		return fail(apperr.Usage("conflicting_flags", "choose at most one of --runtime or --all-runtimes"))
	}

	projRes, err := paths.ResolveProject(ctx, deps.Env, targetPath)
	if err != nil {
		return fail(err)
	}
	root := projRes.Root

	var targetRuntimes []string
	if allRuntimes {
		targetRuntimes = adapter.SupportedRuntimes()
	} else {
		prof, pErr := resolveRuntimeProfile(runtimeArg)
		if pErr != nil {
			return fail(pErr)
		}
		targetRuntimes = []string{prof.ID}
	}

	env.DryRun = dryRun
	var statuses []mcp.MCPConfigStatus

	switch sub {
	case "status":
		for _, rt := range targetRuntimes {
			st, iErr := mcp.InspectMCPConfig(root, rt)
			if iErr != nil {
				return fail(iErr)
			}
			statuses = append(statuses, st)
		}
	case "enable":
		if !dryRun && !yes {
			if jsonOutput || !deps.IsInteractive() {
				return fail(apperr.Usage("confirmation_required", "pass --yes to confirm MCP config mutation in non-interactive or --json mode"))
			}
		}
		for _, rt := range targetRuntimes {
			st, eErr := mcp.EnableMCPConfig(root, rt, exePath, dryRun)
			if eErr != nil {
				return fail(eErr)
			}
			statuses = append(statuses, st)
		}
	case "disable":
		if !dryRun && !yes {
			if jsonOutput || !deps.IsInteractive() {
				return fail(apperr.Usage("confirmation_required", "pass --yes to confirm MCP config removal in non-interactive or --json mode"))
			}
		}
		for _, rt := range targetRuntimes {
			st, dErr := mcp.DisableMCPConfig(root, rt, dryRun)
			if dErr != nil {
				return fail(dErr)
			}
			statuses = append(statuses, st)
		}
	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown mcp subcommand %q (expected serve|status|enable|disable)", sub)))
	}

	if jsonOutput {
		env.MCPConfigs = statuses
		return writeEnvelope(deps.Stdout, env)
	}

	for _, st := range statuses {
		stateLabel := "disabled"
		if st.Enabled {
			stateLabel = fmt.Sprintf("enabled (%s %s)", st.Command, strings.Join(st.Args, " "))
		}
		fmt.Fprintf(deps.Stdout, "MCP [%s] %s: %s (peers=%d)\n", st.Runtime, st.ConfigPath, stateLabel, len(st.PeerServers))
	}
	return nil
}
