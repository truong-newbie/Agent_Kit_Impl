package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runStatus(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("status")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var scope, projectPath, runtimeArg string
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
		case "--runtime":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a value"))
			}
			runtimeArg = args[i]
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

	primaryProfile, rtErr := resolveRuntimeProfile(runtimeArg)
	if rtErr != nil {
		return fail(rtErr)
	}
	env.Runtime = primaryProfile.ID
	env.Scope = scope

	pkg, source, err := catalog.Engineer()
	if err != nil {
		return fail(err)
	}

	scopesToCheck := []string{scope}
	if scope == "" {
		scopesToCheck = []string{"project", "global"}
	}

	var targetRuntimes []adapter.RuntimeProfile
	if runtimeArg != "" {
		targetRuntimes = []adapter.RuntimeProfile{primaryProfile}
	} else {
		for _, rtID := range adapter.SupportedRuntimes() {
			if p, ok := adapter.Profile(rtID); ok {
				targetRuntimes = append(targetRuntimes, p)
			}
		}
	}

	reports := make([]installer.StatusReport, 0, len(scopesToCheck))
	hasDrift := false
	for _, rtProfile := range targetRuntimes {
		isPrimary := rtProfile.ID == primaryProfile.ID
		for _, sc := range scopesToCheck {
			roots, rootWarnings, rErr := resolveScopeRoots(ctx, deps, rtProfile, sc, projectPath)
			if rErr != nil {
				if !isPrimary {
					continue
				}
				if scope == "" && sc == "project" {
					var appErr *apperr.Error
					if errors.As(rErr, &appErr) && (appErr.Code == "home_as_project" || appErr.Code == "filesystem_root") {
						env.Warnings = append(env.Warnings, fmt.Sprintf("project status skipped: %s", appErr.Message))
						continue
					}
				}
				return fail(rErr)
			}
			if isPrimary {
				env.Warnings = append(env.Warnings, rootWarnings...)
			}
			artifacts, rRenderErr := adapter.RenderForRuntime(rtProfile.ID, pkg, source, sc)
			if rRenderErr != nil {
				return fail(rRenderErr)
			}
			report, inspectErr := installer.InspectRuntime(rtProfile.ID, sc, roots, artifacts)
			if inspectErr != nil {
				return fail(inspectErr)
			}
			if !isPrimary && !report.Installed && !report.RecoveryNeeded {
				continue
			}
			if runtimeArg == rtProfile.ID || report.Installed {
				if report.Capabilities != nil && !report.Capabilities.Instructions.DTKitSupported && report.Capabilities.Instructions.Note != "" {
					appendUniqueWarning(&env.Warnings, report.Capabilities.Instructions.Note)
				}
				if _, lookErr := deps.LookPath(rtProfile.Binary); lookErr != nil {
					appendUniqueWarning(&env.Warnings, fmt.Sprintf("%s executable not found on PATH; files will still be installed", rtProfile.Binary))
				}
			}
			if report.RecoveryNeeded || (report.Installed && (!report.Healthy || report.ProfileMismatch)) {
				hasDrift = true
			}
			reports = append(reports, report)
		}
	}

	// Ensure paths remains used if needed or let Go check clean imports.
	_ = paths.DefaultEnvironment

	env.Installations = reports
	if len(reports) == 1 {
		installed := reports[0].Installed
		env.Installed = &installed
		env.Roots = reports[0].Roots
		env.Capabilities = reports[0].Capabilities
		env.Selection = reports[0].Selection
		if reports[0].IntegrationStatus != "" {
			env.Integration = &adapter.IntegrationEvidence{Status: reports[0].IntegrationStatus, Note: reports[0].IntegrationNote}
		}
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
		rtLabel := ""
		if report.Runtime != "" && (report.Runtime != "codex" || len(reports) > len(scopesToCheck)) {
			rtLabel = " (" + report.Runtime + ")"
		}
		if report.RecoveryNeeded {
			fmt.Fprintf(deps.Stdout, "DTKit %s%s installation requires recovery: %s\n", report.Scope, rtLabel, report.Detail)
			continue
		}
		if !report.Installed {
			fmt.Fprintf(deps.Stdout, "DTKit%s is not installed for %s.\n", rtLabel, report.Scope)
			continue
		}
		if report.ProfileMismatch {
			fmt.Fprintf(deps.Stdout, "DTKit %s%s installation has profile mismatch: %s\n", report.Scope, rtLabel, report.Detail)
			continue
		}
		if report.Healthy {
			integ := report.IntegrationStatus
			if integ == "" {
				integ = adapter.IntegrationUnverified
			}
			fmt.Fprintf(deps.Stdout, "DTKit %s%s artifact files are healthy (%s %s, %d artifacts; integration: %s).\n", report.Scope, rtLabel, report.KitID, report.KitVersion, len(report.Artifacts), integ)
			for _, u := range report.UnknownFiles {
				fmt.Fprintf(deps.Stdout, "info\tunowned file in skill dir: %s\n", u)
			}
			continue
		}
		fmt.Fprintf(deps.Stdout, "DTKit %s%s installation has drift (%s):\n", report.Scope, rtLabel, report.Classification)
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
