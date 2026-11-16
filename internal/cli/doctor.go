package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runDoctor(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("doctor")
	env.Runtime = "codex"
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var scope, runtime, projectPath string
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
			runtime = args[i]
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
	if runtime != "" && runtime != "codex" {
		return fail(apperr.Usage("unsupported_runtime", fmt.Sprintf("unsupported runtime %q", runtime)))
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}
	if scope == "" && projectPath != "" {
		scope = "project"
	}
	env.Scope = scope

	checks := make([]DoctorCheck, 0, 6)
	runtimeFailed := false

	codexPath, lookErr := deps.LookPath("codex")
	if lookErr != nil {
		runtimeFailed = true
		checks = append(checks, DoctorCheck{
			Name:    "runtime_executable",
			Status:  "fail",
			Message: "Codex CLI not found on PATH",
			Hint:    "install Codex CLI and ensure 'codex' is on PATH",
		})
	} else {
		checks = append(checks, DoctorCheck{
			Name:    "runtime_executable",
			Status:  "pass",
			Message: fmt.Sprintf("Codex CLI found at %s", codexPath),
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
		var roots map[string]string
		if sc == "project" {
			res, err := paths.ResolveProject(ctx, deps.Env, projectPath)
			if err != nil {
				if scope == "" {
					var appErr *apperr.Error
					if errors.As(err, &appErr) && (appErr.Code == "home_as_project" || appErr.Code == "filesystem_root") {
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
					Message: err.Error(),
				})
				stateFailed = true
				continue
			}
			roots = map[string]string{"project": res.Root}
			env.Warnings = append(env.Warnings, res.Warnings...)
			checks = append(checks, DoctorCheck{
				Name:    "root_resolution",
				Scope:   sc,
				Status:  "pass",
				Message: fmt.Sprintf("project root: %s", res.Root),
			})
		} else {
			res, err := paths.ResolveGlobal(deps.Env)
			if err != nil {
				checks = append(checks, DoctorCheck{
					Name:    "root_resolution",
					Scope:   sc,
					Status:  "fail",
					Message: err.Error(),
				})
				stateFailed = true
				continue
			}
			roots = map[string]string{"user-home": res.UserHome, "codex-home": res.CodexHome}
			env.Warnings = append(env.Warnings, res.Warnings...)
			checks = append(checks, DoctorCheck{
				Name:    "root_resolution",
				Scope:   sc,
				Status:  "pass",
				Message: fmt.Sprintf("user-home: %s, codex-home: %s", res.UserHome, res.CodexHome),
			})
		}

		instrRoot := roots["project"]
		if sc == "global" {
			instrRoot = roots["codex-home"]
		}
		if overridePath, err := paths.ResolveTarget(instrRoot, "AGENTS.override.md"); err == nil {
			if info, statErr := os.Lstat(overridePath); statErr == nil && info.Size() > 0 {
				checks = append(checks, DoctorCheck{
					Name:    "instruction_override",
					Scope:   sc,
					Status:  "warn",
					Message: fmt.Sprintf("non-empty AGENTS.override.md present at %s", overridePath),
					Hint:    "AGENTS.override.md takes precedence over AGENTS.md in Codex",
				})
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

		artifacts, err := codex.Render(pkg, source, sc)
		if err != nil {
			return fail(err)
		}
		report, err := installer.Inspect(sc, roots, artifacts)
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
				Hint:    fmt.Sprintf("run dtkit recover --%s --dry-run", sc),
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
			return failJSON(deps.Stdout, env, apperr.Runtime("runtime_not_ready", "Codex runtime is not ready"))
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
		return apperr.Runtime("runtime_not_ready", "Codex runtime is not ready")
	}
	return nil
}
