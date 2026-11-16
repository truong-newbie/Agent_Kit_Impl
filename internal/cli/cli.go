package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

const helpText = `DTKit — AI engineering toolkit

Usage:
  dtkit version [--json]
  dtkit help [command]
  dtkit list [--json]
  dtkit info <kit> [--from <dir>] [--json]
  dtkit validate <directory> [--json]
  dtkit doctor [--runtime codex] [--project|--global] [--path <dir>] [--json]
  dtkit install <kit> --project|--global [--path <dir>] [--from <dir>] [--runtime codex] [--dry-run] [--yes] [--json]
  dtkit status [--project|--global] [--path <dir>] [--json]
  dtkit update <kit> --project|--global [--path <dir>] [--from <dir>] [--runtime codex] [--dry-run] [--yes] [--json]
  dtkit remove <kit> --project|--global [--path <dir>] [--dry-run] [--yes] [--json]
  dtkit recover --project|--global [--path <dir>] [--transaction <id>] [--dry-run] [--yes] [--json]

Examples:
  dtkit doctor
  dtkit install engineer --project --dry-run
  dtkit install engineer --project --yes
  dtkit status --project
  dtkit update engineer --project --dry-run
  dtkit remove engineer --project --dry-run
  dtkit recover --project --dry-run
`

func Run(args []string, stdout, stderr io.Writer) error {
	return RunWithDeps(context.Background(), args, DefaultDeps(nil, stdout, stderr))
}

func RunWithDeps(ctx context.Context, args []string, deps Deps) error {
	deps = deps.withDefaults()
	if err := ctx.Err(); err != nil {
		return apperr.Cancelled("operation cancelled")
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if len(args) > 1 && args[0] == "help" {
			return printCommandHelp(args[1], deps.Stdout)
		}
		_, err := io.WriteString(deps.Stdout, helpText)
		return err
	}

	switch args[0] {
	case "--version", "version":
		return runVersion(args[1:], deps)
	case "list":
		return runList(args[1:], deps)
	case "info":
		return runInfo(args[1:], deps)
	case "validate":
		return runValidate(args[1:], deps)
	case "doctor":
		return runDoctor(ctx, args[1:], deps)
	case "install":
		return runInstall(ctx, args[1:], deps)
	case "status":
		return runStatus(ctx, args[1:], deps)
	case "update":
		return runUpdate(ctx, args[1:], deps)
	case "remove":
		return runRemove(ctx, args[1:], deps)
	case "recover":
		return runRecover(ctx, args[1:], deps)
	default:
		if strings.HasPrefix(args[0], "-") {
			err := apperr.Usage("unknown_option", fmt.Sprintf("unknown option %q", args[0]))
			if hasJSONFlag(args[1:]) {
				return failJSON(deps.Stdout, newEnvelope("dtkit"), err)
			}
			return err
		}
		err := apperr.Usage("unknown_command", fmt.Sprintf("command %q is not available; run dtkit help", args[0]))
		if hasJSONFlag(args[1:]) {
			return failJSON(deps.Stdout, newEnvelope(args[0]), err)
		}
		return err
	}
}

func printCommandHelp(cmd string, out io.Writer) error {
	switch cmd {
	case "install":
		_, err := io.WriteString(out, "Usage: dtkit install <kit> --project|--global [--path <dir>] [--from <dir>] [--runtime codex] [--dry-run] [--yes] [--json]\n")
		return err
	case "status":
		_, err := io.WriteString(out, "Usage: dtkit status [--project|--global] [--path <dir>] [--json]\n")
		return err
	case "doctor":
		_, err := io.WriteString(out, "Usage: dtkit doctor [--runtime codex] [--project|--global] [--path <dir>] [--json]\n")
		return err
	case "validate":
		_, err := io.WriteString(out, "Usage: dtkit validate <directory> [--json]\n")
		return err
	case "info":
		_, err := io.WriteString(out, "Usage: dtkit info <kit> [--from <dir>] [--json]\n")
		return err
	case "update":
		_, err := io.WriteString(out, "Usage: dtkit update <kit> --project|--global [--path <dir>] [--from <dir>] [--runtime codex] [--dry-run] [--yes] [--json]\n")
		return err
	case "remove":
		_, err := io.WriteString(out, "Usage: dtkit remove <kit> --project|--global [--path <dir>] [--dry-run] [--yes] [--json]\n")
		return err
	case "recover":
		_, err := io.WriteString(out, "Usage: dtkit recover --project|--global [--path <dir>] [--transaction <id>] [--dry-run] [--yes] [--json]\n")
		return err
	default:
		_, err := io.WriteString(out, helpText)
		return err
	}
}

func hasJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--json" {
			return true
		}
	}
	return false
}

func runVersion(args []string, deps Deps) error {
	jsonOutput := false
	for _, a := range args {
		switch a {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		default:
			err := apperr.Usage("invalid_flag", fmt.Sprintf("version does not accept %q", a))
			if jsonOutput || hasJSONFlag(args) {
				return failJSON(deps.Stdout, newEnvelope("version"), err)
			}
			return err
		}
	}
	info := buildinfo.Current()
	if jsonOutput {
		env := newEnvelope("version")
		env.BuildInfo = &info
		return writeEnvelope(deps.Stdout, env)
	}
	_, err := fmt.Fprintf(deps.Stdout, "dtkit %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date)
	return err
}

func runList(args []string, deps Deps) error {
	jsonOutput := false
	for _, a := range args {
		switch a {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		default:
			err := apperr.Usage("invalid_flag", fmt.Sprintf("usage: dtkit list [--json] (unexpected %q)", a))
			if jsonOutput || hasJSONFlag(args) {
				return failJSON(deps.Stdout, newEnvelope("list"), err)
			}
			return err
		}
	}
	p, _, err := catalog.Engineer()
	if err != nil {
		if jsonOutput {
			return failJSON(deps.Stdout, newEnvelope("list"), err)
		}
		return err
	}
	if jsonOutput {
		env := newEnvelope("list")
		env.Packages = []PackageSummary{{ID: p.ID, Name: p.Name, Version: p.Version, Description: p.Description}}
		return writeEnvelope(deps.Stdout, env)
	}
	_, err = fmt.Fprintf(deps.Stdout, "%s\t%s\t%s\n", p.ID, p.Version, p.Description)
	return err
}

func runInfo(args []string, deps Deps) error {
	var kitID, fromDir string
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--from":
			i++
			if i >= len(args) {
				err := apperr.Usage("missing_flag_value", "--from requires a directory")
				if jsonOutput || hasJSONFlag(args) {
					return failJSON(deps.Stdout, newEnvelope("info"), err)
				}
				return err
			}
			fromDir = args[i]
		default:
			if strings.HasPrefix(args[i], "-") || kitID != "" {
				err := apperr.Usage("invalid_usage", "usage: dtkit info <kit> [--from <dir>] [--json]")
				if jsonOutput || hasJSONFlag(args) {
					return failJSON(deps.Stdout, newEnvelope("info"), err)
				}
				return err
			}
			kitID = args[i]
		}
	}
	if kitID == "" {
		err := apperr.Usage("missing_kit", "usage: dtkit info <kit> [--from <dir>] [--json]")
		if jsonOutput {
			return failJSON(deps.Stdout, newEnvelope("info"), err)
		}
		return err
	}
	var p catalog.Package
	var digest string
	var err error
	if fromDir != "" {
		p, _, digest, err = catalog.LoadDir(fromDir)
		if err == nil && p.ID != kitID {
			err = apperr.Usage("kit_id_mismatch", fmt.Sprintf("local package ID %q does not match requested %q", p.ID, kitID))
		}
	} else {
		if kitID != "engineer" {
			err = apperr.Usage("unknown_kit", fmt.Sprintf("kit %q is not in the bundled catalog", kitID))
		} else {
			var src interface{ Open(string) (os.File, error) }
			_ = src
			pkg, fsSrc, loadErr := catalog.Engineer()
			p = pkg
			err = loadErr
			if err == nil {
				digest, err = catalog.Digest(fsSrc)
			}
		}
	}
	if err != nil {
		if jsonOutput {
			return failJSON(deps.Stdout, newEnvelope("info"), err)
		}
		return err
	}
	if jsonOutput {
		env := newEnvelope("info")
		env.Kit = &KitSummary{ID: p.ID, Version: p.Version, Digest: digest}
		env.Package = &p
		return writeEnvelope(deps.Stdout, env)
	}
	_, err = fmt.Fprintf(deps.Stdout, "%s %s (%s)\n%s\nRuntimes: %s\nScopes: %s\nComponents: %d\nDigest: %s\n",
		p.ID, p.Version, p.Name, p.Description,
		strings.Join(p.RuntimeTargets, ", "),
		strings.Join(p.Scopes, ", "),
		len(p.Components), digest)
	return err
}

func runValidate(args []string, deps Deps) error {
	var dir string
	jsonOutput := false
	for _, a := range args {
		switch a {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		default:
			if strings.HasPrefix(a, "-") || dir != "" {
				err := apperr.Usage("invalid_usage", "usage: dtkit validate <directory> [--json]")
				if jsonOutput || hasJSONFlag(args) {
					return failJSON(deps.Stdout, newEnvelope("validate"), err)
				}
				return err
			}
			dir = a
		}
	}
	if dir == "" {
		err := apperr.Usage("missing_directory", "usage: dtkit validate <directory> [--json]")
		if jsonOutput {
			return failJSON(deps.Stdout, newEnvelope("validate"), err)
		}
		return err
	}
	env := newEnvelope("validate")
	p, snap, digest, err := catalog.LoadDir(dir)
	if err != nil {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}
	for _, scope := range p.Scopes {
		if _, err := codex.Render(p, snap, scope); err != nil {
			if jsonOutput {
				return failJSON(deps.Stdout, env, err)
			}
			return err
		}
	}
	if jsonOutput {
		env.Kit = &KitSummary{ID: p.ID, Version: p.Version, Digest: digest}
		env.Package = &p
		return writeEnvelope(deps.Stdout, env)
	}
	_, err = fmt.Fprintf(deps.Stdout, "Validation passed: %s %s (digest %s; local package check is not a security audit for untrusted input)\n", p.ID, p.Version, digest[:12])
	return err
}
