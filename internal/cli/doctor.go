package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
)

func runDoctor(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("doctor")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var scope, runtimeArg, projectPath string
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
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown doctor option %q", args[i])))
		}
	}
	profile, rtErr := resolveRuntimeProfile(runtimeArg)
	if rtErr != nil {
		return fail(rtErr)
	}
	env.Runtime = profile.ID
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}
	if scope == "" && projectPath != "" {
		scope = "project"
	}
	env.Scope = scope

	checks := make([]DoctorCheck, 0, 8)
	runtimeFailed := false

	rtPath, lookErr := deps.LookPath(profile.Binary)
	if lookErr != nil {
		runtimeFailed = true
		checks = append(checks, DoctorCheck{
			Name:    "runtime_executable",
			Status:  "fail",
			Message: fmt.Sprintf("%s not found on PATH", profile.DisplayName),
			Hint:    fmt.Sprintf("install %s and ensure '%s' is on PATH", profile.DisplayName, profile.Binary),
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:    "runtime_executable",
			Status:  "pass",
			Message: fmt.Sprintf("%s found at %s", profile.DisplayName, rtPath),
		})
	}

	pkg, source, err := catalog.Engineer()
	if err != nil {
		return fail(err)
	}

	scopesToCheck := []string{scope}
	if scope == "" {
		scopesToCheck = []string{"project", "global"}
	}
	stateFailed := false
	for _, sc := range scopesToCheck {
		roots, rootWarnings, rErr := resolveScopeRoots(ctx, deps, profile, sc, projectPath)
		if rErr != nil {
			if scope == "" && sc == "project" {
				var appErr *apperr.Error
				if errors.As(rErr, &appErr) && (appErr.Code == "home_as_project" || appErr.Code == "filesystem_root") {
					checks = append(checks, DoctorCheck{
						Name:    "root_resolution",
						Scope:   sc,
						Status:  "warn",
						Message: appErr.Message,
					})
					continue
				}
			}
			checks = append(checks, DoctorCheck{
				Name:    "root_resolution",
				Scope:   sc,
				Status:  "fail",
				Message: rErr.Error(),
			})
			stateFailed = true
			continue
		}
		env.Warnings = append(env.Warnings, rootWarnings...)
		checks = append(checks, DoctorCheck{
			Name:    "root_resolution",
			Scope:   sc,
			Status:  "pass",
			Message: describeRoots(roots),
		})

		checks = append(checks, doctorRuntimeSpecificChecks(profile, sc, roots)...)
		checks = append(checks, runtimeIntegrationDoctorCheck(profile, sc))
		if capPlan, cErr := adapter.EvaluateCapabilities(profile, sc, nil); cErr == nil {
			for _, w := range capPlan.Warnings {
				appendUniqueWarning(&env.Warnings, w)
			}
			if len(scopesToCheck) == 1 {
				capCopy := capPlan.Capabilities
				integCopy := capPlan.Integration
				env.Capabilities = &capCopy
				env.Integration = &integCopy
			}
		}

		if sc == "project" {
			if _, gitStatErr := os.Lstat(filepath.Join(roots["project"], ".git")); gitStatErr == nil {
				cmd := exec.CommandContext(ctx, "git", "-C", roots["project"], "check-ignore", "-q", ".dtkit/local")
				if ignoreErr := cmd.Run(); ignoreErr != nil {
					checks = append(checks, DoctorCheck{
						Name:    "operational_data_ignore",
						Scope:   sc,
						Status:  "warn",
						Message: ".dtkit/local/ is not ignored by Git",
						Hint:    "add '.dtkit/local/' to .gitignore so local journals and backups are not committed",
					})
				}
			}
		}

		artifacts, err := adapter.RenderForRuntime(profile.ID, pkg, source, sc)
		if err != nil {
			return fail(err)
		}
		report, err := installer.InspectRuntime(profile.ID, sc, roots, artifacts)
		if err != nil {
			stateFailed = true
			checks = append(checks, DoctorCheck{
				Name:    "installation_state",
				Scope:   sc,
				Status:  "fail",
				Message: err.Error(),
			})
			continue
		}
		if report.RecoveryNeeded {
			stateFailed = true
			checks = append(checks, DoctorCheck{
				Name:    "interrupted_transaction",
				Scope:   sc,
				Status:  "fail",
				Message: report.Detail,
				Hint:    fmt.Sprintf("run dtkit recover --%s --runtime %s --dry-run", sc, profile.ID),
			})
		} else if !report.Installed {
			checks = append(checks, DoctorCheck{
				Name:    "installation_state",
				Scope:   sc,
				Status:  "pass",
				Message: "not installed",
			})
		} else if report.ProfileMismatch || !report.Healthy {
			stateFailed = true
			checks = append(checks, DoctorCheck{
				Name:    "installation_state",
				Scope:   sc,
				Status:  "fail",
				Message: fmt.Sprintf("installed %s %s has drift or profile mismatch", report.KitID, report.KitVersion),
			})
		} else {
			checks = append(checks, DoctorCheck{
				Name:    "installation_state",
				Scope:   sc,
				Status:  "pass",
				Message: fmt.Sprintf("healthy (%s %s)", report.KitID, report.KitVersion),
			})
		}
	}

	env.Checks = checks
	if jsonOutput {
		if stateFailed {
			return failJSON(deps.Stdout, env, apperr.Conflict("doctor_state_failed", "doctor found installation or root issues"))
		}
		if runtimeFailed {
			return failJSON(deps.Stdout, env, apperr.Runtime("runtime_not_ready", fmt.Sprintf("%s runtime is not ready", profile.DisplayName)))
		}
		return writeEnvelope(deps.Stdout, env)
	}

	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	for _, c := range checks {
		prefix := c.Scope
		if prefix != "" {
			prefix += " "
		}
		fmt.Fprintf(deps.Stdout, "[%s] %s%s: %s\n", c.Status, prefix, c.Name, c.Message)
	}
	if stateFailed {
		return apperr.Conflict("doctor_state_failed", "doctor found installation or root issues")
	}
	if runtimeFailed {
		return apperr.Runtime("runtime_not_ready", fmt.Sprintf("%s runtime is not ready", profile.DisplayName))
	}
	return nil
}
