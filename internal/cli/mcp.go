package cli

import (
	"context"
	"fmt"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/mcp"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runMCP(ctx context.Context, args []string, deps Deps) error {
	if len(args) == 0 || args[0] != "serve" {
		return apperr.Usage("missing_subcommand", "usage: dtkit mcp serve [--path <dir>]")
	}
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
