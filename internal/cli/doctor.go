package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
)

var (
	redactURLRe    = regexp.MustCompile(`https?://[^\s"'<>]+`)
	redactBearerRe = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/-]+=*`)
	redactKVRe     = regexp.MustCompile(`(?i)\b(token|secret|password|passwd|apikey|api_key|authorization)=([^\s&"';]+)`)
)

func collectHomeCandidates(deps Deps) []string {
	raw := make([]string, 0, 6)
	if deps.Env.UserHomeDir != nil {
		if h, err := deps.Env.UserHomeDir(); err == nil && h != "" {
			raw = append(raw, h)
		}
	} else if h, err := os.UserHomeDir(); err == nil && h != "" {
		raw = append(raw, h)
	}
	getEnv := os.Getenv
	if deps.Env.Getenv != nil {
		getEnv = deps.Env.Getenv
	}
	for _, k := range []string{"HOME", "USERPROFILE"} {
		if v := getEnv(k); v != "" {
			raw = append(raw, v)
		}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw)*2)
	for _, h := range raw {
		for _, variant := range []string{filepath.Clean(h), filepath.ToSlash(filepath.Clean(h))} {
			variant = strings.TrimRight(variant, `/\`)
			if variant == "" || variant == "/" {
				continue
			}
			key := strings.ToLower(variant)
			if !seen[key] {
				seen[key] = true
				out = append(out, variant)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

func replaceFoldLiteral(s, target, repl string) string {
	if target == "" || len(target) > len(s) {
		return s
	}
	lowerS := strings.ToLower(s)
	lowerT := strings.ToLower(target)
	var b strings.Builder
	i := 0
	for i < len(s) {
		idx := strings.Index(lowerS[i:], lowerT)
		if idx < 0 {
			b.WriteString(s[i:])
			break
		}
		b.WriteString(s[i : i+idx])
		b.WriteString(repl)
		i += idx + len(target)
	}
	return b.String()
}

func redactDiagnosticText(s string, homes []string) string {
	if s == "" {
		return s
	}
	out := s
	for _, h := range homes {
		out = replaceFoldLiteral(out, h, "<HOME>")
	}
	out = redactURLRe.ReplaceAllStringFunc(out, func(raw string) string {
		return dist.SanitizeURL(raw)
	})
	out = redactBearerRe.ReplaceAllString(out, `${1}<REDACTED>`)
	out = redactKVRe.ReplaceAllString(out, `${1}=<REDACTED>`)
	return out
}

func redactEnvelope(env *Envelope, homes []string) {
	for k, v := range env.Roots {
		env.Roots[k] = redactDiagnosticText(v, homes)
	}
	if env.Explain != nil {
		env.Explain.SourceLocation = redactDiagnosticText(env.Explain.SourceLocation, homes)
	}
	if env.Diff != nil {
		for i := range env.Diff.Changes {
			env.Diff.Changes[i].Patch = redactDiagnosticText(env.Diff.Changes[i].Patch, homes)
		}
	}
	if env.Fork != nil {
		env.Fork.Dir = redactDiagnosticText(env.Fork.Dir, homes)
	}
	for i := range env.Warnings {
		env.Warnings[i] = redactDiagnosticText(env.Warnings[i], homes)
	}
	for i := range env.Checks {
		env.Checks[i].Message = redactDiagnosticText(env.Checks[i].Message, homes)
		env.Checks[i].Hint = redactDiagnosticText(env.Checks[i].Hint, homes)
	}
	for i := range env.Errors {
		env.Errors[i].Message = redactDiagnosticText(env.Errors[i].Message, homes)
		env.Errors[i].Path = redactDiagnosticText(env.Errors[i].Path, homes)
		env.Errors[i].Hint = redactDiagnosticText(env.Errors[i].Hint, homes)
	}
}

func redactAppError(err error, homes []string) error {
	if err == nil {
		return nil
	}
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		cp := *appErr
		cp.Message = redactDiagnosticText(cp.Message, homes)
		cp.Path = redactDiagnosticText(cp.Path, homes)
		cp.Hint = redactDiagnosticText(cp.Hint, homes)
		return &cp
	}
	return errors.New(redactDiagnosticText(err.Error(), homes))
}

func runDoctor(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("doctor")
	jsonOutput := hasJSONFlag(args)
	var scope, runtimeArg, projectPath string
	redact := false
	fail := func(err error) error {
		if redact {
			homes := collectHomeCandidates(deps)
			redactEnvelope(&env, homes)
			err = redactAppError(err, homes)
		}
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

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
		case "--redact":
			redact = true
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

		reports, err := installer.InspectRuntimeAll(profile.ID, sc, roots)
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
		for _, report := range reports {
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
	}

	env.Checks = checks
	if redact {
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		if stateFailed {
			return fail(apperr.Conflict("doctor_state_failed", "doctor found installation or root issues"))
		}
		if runtimeFailed {
			return fail(apperr.Runtime("runtime_not_ready", fmt.Sprintf("%s runtime is not ready", profile.DisplayName)))
		}
		return writeEnvelope(deps.Stdout, env)
	}

	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	for _, c := range env.Checks {
		prefix := c.Scope
		if prefix != "" {
			prefix += " "
		}
		fmt.Fprintf(deps.Stdout, "[%s] %s%s: %s\n", c.Status, prefix, c.Name, c.Message)
	}
	if stateFailed {
		return fail(apperr.Conflict("doctor_state_failed", "doctor found installation or root issues"))
	}
	if runtimeFailed {
		return fail(apperr.Runtime("runtime_not_ready", fmt.Sprintf("%s runtime is not ready", profile.DisplayName)))
	}
	return nil
}
