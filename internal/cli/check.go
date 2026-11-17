package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/check"
)

func runCheck(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("check")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var projectPath string
	var dryRun, changedOnly, all, yes, allowAutofix bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a value"))
			}
			projectPath = args[i]
		case "--dry-run":
			dryRun = true
		case "--changed":
			changedOnly = true
		case "--all":
			all = true
		case "--yes", "-y":
			yes = true
		case "--allow-autofix":
			allowAutofix = true
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown check option %q", args[i])))
		}
	}

	if changedOnly && all {
		return fail(apperr.Usage("conflicting_flags", "choose at most one of --changed or --all"))
	}
	if !changedOnly && !all {
		changedOnly = true
	}
	env.DryRun = dryRun

	rep, err := check.Run(ctx, deps.Env, check.Options{
		Path:         projectPath,
		DryRun:       dryRun,
		ChangedOnly:  changedOnly,
		All:          all,
		Yes:          yes,
		AllowAutofix: allowAutofix,
		LookPath:     deps.LookPath,
		Now:          deps.Now,
	})
	if err != nil {
		return fail(err)
	}
	env.CheckReport = &rep
	env.Warnings = append(env.Warnings, rep.Warnings...)

	if !rep.Ok {
		checkErr := apperr.Conflict("check_failed", fmt.Sprintf("checks failed (fail=%d, missingTool=%d, timeout=%d, cancelled=%d)", rep.Summary.Fail, rep.Summary.MissingTool, rep.Summary.Timeout, rep.Summary.Cancelled))
		if jsonOutput {
			return failJSON(deps.Stdout, env, checkErr)
		}
		printCheckText(deps, rep)
		return checkErr
	}

	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	printCheckText(deps, rep)
	return nil
}

func printCheckText(deps Deps, rep check.Report) {
	for _, w := range rep.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	fmt.Fprintf(deps.Stdout, "Check root=%s mode=%s dryRun=%v trusted=%v ok=%v\n", rep.Root, rep.Mode, rep.DryRun, rep.Trusted, rep.Ok)
	for _, r := range rep.Results {
		fmt.Fprintf(deps.Stdout, "  [%s] %s (%s cwd=%s): %s %s (%dms)\n",
			r.Status, r.ID, r.Kind, r.Cwd, r.Executable, strings.Join(r.Argv, " "), r.DurationMs)
	}
	fmt.Fprintf(deps.Stdout, "Summary: total=%d pass=%d fail=%d missing_tool=%d timeout=%d cancelled=%d skipped=%d\n",
		rep.Summary.Total, rep.Summary.Pass, rep.Summary.Fail, rep.Summary.MissingTool, rep.Summary.Timeout, rep.Summary.Cancelled, rep.Summary.Skipped)
}
