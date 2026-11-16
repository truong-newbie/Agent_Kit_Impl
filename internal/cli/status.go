package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runStatus(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("status")
	env.Runtime = "codex"
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var scope, projectPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose at most one of --project or --global"))
			}
			scope = "project"
		case "--global":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose at most one of --project or --global"))
			}
			scope = "global"
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a value"))
			}
			projectPath = args[i]
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown status option %q", args[i])))
		}
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}
	if scope == "" && projectPath != "" {
		scope = "project"
	}
	env.Scope = scope

	pkg, source, err := catalog.Engineer()
	if err != nil {
		return fail(err)
	}

	scopesToCheck := []string{scope}
	if scope == "" {
		scopesToCheck = []string{"project", "global"}
	}

	reports := make([]installer.StatusReport, 0, len(scopesToCheck))
	hasDrift := false
	for _, sc := range scopesToCheck {
		var roots map[string]string
		if sc == "project" {
			res, err := paths.ResolveProject(ctx, deps.Env, projectPath)
			if err != nil {
				if scope == "" {
					var appErr *apperr.Error
					if errors.As(err, &appErr) && (appErr.Code == "home_as_project" || appErr.Code == "filesystem_root") {
						env.Warnings = append(env.Warnings, fmt.Sprintf("project status skipped: %s", appErr.Message))
						continue
					}
				}
				return fail(err)
			}
			roots = map[string]string{"project": res.Root}
			env.Warnings = append(env.Warnings, res.Warnings...)
		} else {
			res, err := paths.ResolveGlobal(deps.Env)
			if err != nil {
				return fail(err)
			}
			roots = map[string]string{"user-home": res.UserHome, "codex-home": res.CodexHome}
			env.Warnings = append(env.Warnings, res.Warnings...)
		}

		artifacts, err := codex.Render(pkg, source, sc)
		if err != nil {
			return fail(err)
		}
		report, err := installer.Inspect(sc, roots, artifacts)
		if err != nil {
			return fail(err)
		}
		if report.RecoveryNeeded || (report.Installed && (!report.Healthy || report.ProfileMismatch)) {
			hasDrift = true
		}
		reports = append(reports, report)
	}

	env.Installations = reports
	if len(reports) == 1 {
		installed := reports[0].Installed
		env.Installed = &installed
		env.Roots = reports[0].Roots
	}

	if jsonOutput {
		if hasDrift {
			return failJSON(deps.Stdout, env, apperr.Conflict("installation_drift", "installation drift, profile mismatch, or unfinished transaction detected"))
		}
		return writeEnvelope(deps.Stdout, env)
	}

	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	for _, report := range reports {
		if report.RecoveryNeeded {
			fmt.Fprintf(deps.Stdout, "DTKit %s installation requires recovery: %s\n", report.Scope, report.Detail)
			continue
		}
		if !report.Installed {
			fmt.Fprintf(deps.Stdout, "DTKit is not installed for %s.\n", report.Scope)
			continue
		}
		if report.ProfileMismatch {
			fmt.Fprintf(deps.Stdout, "DTKit %s installation has profile mismatch: %s\n", report.Scope, report.Detail)
			continue
		}
		if report.Healthy {
			fmt.Fprintf(deps.Stdout, "DTKit %s installation is healthy (%s %s, %d artifacts).\n", report.Scope, report.KitID, report.KitVersion, len(report.Artifacts))
			for _, u := range report.UnknownFiles {
				fmt.Fprintf(deps.Stdout, "info\tunowned file in skill dir: %s\n", u)
			}
			continue
		}
		fmt.Fprintf(deps.Stdout, "DTKit %s installation has drift (%s):\n", report.Scope, report.Classification)
		for _, artifact := range report.Artifacts {
			if artifact.Status != "healthy" {
				fmt.Fprintf(deps.Stdout, "%s\t%s:%s\t%s\n", artifact.Status, artifact.Root, artifact.Path, artifact.Detail)
			}
		}
		for _, u := range report.UnknownFiles {
			fmt.Fprintf(deps.Stdout, "info\tunowned file in skill dir: %s\n", u)
		}
	}

	if hasDrift {
		var details []string
		for _, r := range reports {
			if r.RecoveryNeeded || (r.Installed && (!r.Healthy || r.ProfileMismatch)) {
				details = append(details, r.Scope)
			}
		}
		return apperr.Conflict("installation_drift", fmt.Sprintf("drift or recovery required in %s installation", strings.Join(details, ", ")))
	}
	return nil
}
