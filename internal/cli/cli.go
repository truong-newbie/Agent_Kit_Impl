package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
)

const helpText = `DTKit — AI engineering toolkit

Usage:
  dtkit version [--json]
  dtkit help [command]
  dtkit list [--json]
  dtkit info <kit>[@<ver>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--json]
  dtkit validate <dir|archive> [--runtime <codex|gemini|opencode|cursor>] [--json]
  dtkit validate <kit>[@<ver>] --source <url>|--offline [--sha256 <hex>] [--runtime <codex|gemini|opencode|cursor>] [--json]
  dtkit create kit <id> [--dir <parent>] [--name <title>] [--json]
  dtkit create skill <slug> --kit-dir <dir> [--description <text>] [--json]
  dtkit pack <directory> [--out <file.tar.gz>] [--json]
  dtkit doctor [--runtime <codex|gemini|opencode|cursor>] [--project|--global] [--path <dir>] [--json]
  dtkit install <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]
  dtkit status [--runtime <codex|gemini|opencode|cursor>] [--project|--global] [--path <dir>] [--json]
  dtkit update <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]
  dtkit remove <kit> --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--dry-run] [--yes] [--json]
  dtkit recover --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--transaction <id>] [--dry-run] [--yes] [--json]
  dtkit hooks <list|status|enable|disable> [args]
  dtkit hook run <id> --scope project|global --root <dir>

Examples:
  dtkit doctor
  dtkit create kit my-kit
  dtkit pack ./my-kit
  dtkit install engineer --project --dry-run
  dtkit install engineer --project --yes
  dtkit status --project
  dtkit update engineer --project --dry-run
  dtkit remove engineer --project --dry-run
  dtkit recover --project --dry-run
  dtkit hooks enable session-context --project --dry-run
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
		return runInfo(ctx, args[1:], deps)
	case "validate":
		return runValidate(ctx, args[1:], deps)
	case "create":
		return runCreate(ctx, args[1:], deps)
	case "pack":
		return runPack(args[1:], deps)
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
	case "hooks":
		return runHooks(ctx, args[1:], deps)
	case "hook":
		return runHookExec(ctx, args[1:], deps)
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
		_, err := io.WriteString(out, "Usage: dtkit install <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]\n")
		return err
	case "status":
		_, err := io.WriteString(out, "Usage: dtkit status [--runtime <codex|gemini|opencode|cursor>] [--project|--global] [--path <dir>] [--json]\n")
		return err
	case "doctor":
		_, err := io.WriteString(out, "Usage: dtkit doctor [--runtime <codex|gemini|opencode|cursor>] [--project|--global] [--path <dir>] [--json]\n")
		return err
	case "validate":
		_, err := io.WriteString(out, "Usage: dtkit validate <dir|archive> [--runtime <codex|gemini|opencode|cursor>] [--json]\n       dtkit validate <kit>[@<ver>] --source <url>|--offline [--sha256 <hex>] [--runtime <codex|gemini|opencode|cursor>] [--json]\n")
		return err
	case "info":
		_, err := io.WriteString(out, "Usage: dtkit info <kit>[@<ver>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--json]\n")
		return err
	case "create":
		_, err := io.WriteString(out, "Usage: dtkit create <kit|skill> [options]\n")
		return err
	case "pack":
		_, err := io.WriteString(out, "Usage: dtkit pack <directory> [--out <file.tar.gz>] [--json]\n")
		return err
	case "update":
		_, err := io.WriteString(out, "Usage: dtkit update <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]\n")
		return err
	case "remove":
		_, err := io.WriteString(out, "Usage: dtkit remove <kit> --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--dry-run] [--yes] [--json]\n")
		return err
	case "recover":
		_, err := io.WriteString(out, "Usage: dtkit recover --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--transaction <id>] [--dry-run] [--yes] [--json]\n")
		return err
	case "hooks":
		_, err := io.WriteString(out, "Usage: dtkit hooks <list|status|enable|disable> [--project|--global] [--path <dir>] [--executable <abs>] [--dry-run] [--yes] [--json]\n")
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

func runInfo(ctx context.Context, args []string, deps Deps) error {
	var kitID, fromPath, sourceURL, sha256Hex string
	var offline bool
	jsonOutput := hasJSONFlag(args)
	env := newEnvelope("info")
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--offline":
			offline = true
		case "--from", "--source", "--sha256":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--from":
				fromPath = args[i]
			case "--source":
				sourceURL = args[i]
			case "--sha256":
				sha256Hex = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") || kitID != "" {
				return fail(apperr.Usage("invalid_usage", "usage: dtkit info <kit>[@<ver>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--json]"))
			}
			kitID = args[i]
		}
	}
	if kitID == "" {
		return fail(apperr.Usage("missing_kit", "usage: dtkit info <kit>[@<ver>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--json]"))
	}
	res, err := resolveKitSource(ctx, deps, kitID, fromPath, sourceURL, sha256Hex, offline, dist.CacheReadOnly)
	if err != nil {
		return fail(err)
	}
	p, digest, archiveSHA := res.Package, res.PackageDigest, res.ArchiveSHA256
	if jsonOutput {
		env.Kit = &KitSummary{ID: p.ID, Version: p.Version, Digest: digest, ArchiveSHA256: archiveSHA}
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

func runValidate(ctx context.Context, args []string, deps Deps) error {
	var targetPath, runtimeArg, fromPath, sourceURL, sha256Hex string
	var offline bool
	jsonOutput := hasJSONFlag(args)
	env := newEnvelope("validate")
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--offline":
			offline = true
		case "--runtime", "--from", "--source", "--sha256":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--runtime":
				runtimeArg = args[i]
			case "--from":
				fromPath = args[i]
			case "--source":
				sourceURL = args[i]
			case "--sha256":
				sha256Hex = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") || targetPath != "" {
				return fail(apperr.Usage("invalid_usage", "usage: dtkit validate <dir|archive> [--runtime <codex|gemini|opencode|cursor>] [--json]"))
			}
			targetPath = args[i]
		}
	}
	if runtimeArg != "" {
		profile, err := resolveRuntimeProfile(runtimeArg)
		if err != nil {
			return fail(err)
		}
		env.Runtime = profile.ID
	}

	var p catalog.Package
	var memFS fs.FS
	var digest, archiveSHA string
	if sourceURL != "" || offline || fromPath != "" {
		if targetPath == "" {
			return fail(apperr.Usage("missing_kit", "kit identifier is required with --from, --source, or --offline"))
		}
		res, err := resolveKitSource(ctx, deps, targetPath, fromPath, sourceURL, sha256Hex, offline, dist.CacheReadOnly)
		if err != nil {
			return fail(err)
		}
		p, memFS, digest, archiveSHA = res.Package, res.Source, res.PackageDigest, res.ArchiveSHA256
	} else {
		if targetPath == "" {
			return fail(apperr.Usage("missing_directory", "usage: dtkit validate <dir|archive> [--runtime <codex|gemini|opencode|cursor>] [--json]"))
		}
		info, statErr := os.Lstat(targetPath)
		if statErr != nil {
			return fail(statErr)
		}
		if info.IsDir() {
			if sha256Hex != "" {
				return fail(apperr.Usage("invalid_flag_combination", "--sha256 applies to archives, not directories"))
			}
			pkg, snapFS, dig, err := catalog.LoadDir(targetPath)
			if err != nil {
				return fail(err)
			}
			p, memFS, digest = pkg, snapFS, dig
		} else {
			pkg, snapFS, dig, archSHA, err := dist.ReadArchiveFile(targetPath, sha256Hex)
			if err != nil {
				return fail(err)
			}
			p, memFS, digest, archiveSHA = pkg, snapFS, dig, archSHA
		}
	}

	targets := p.RuntimeTargets
	if runtimeArg != "" {
		targets = []string{env.Runtime}
	}
	for _, rt := range targets {
		for _, scope := range p.Scopes {
			if _, err := adapter.RenderForRuntime(rt, p, memFS, scope); err != nil {
				return fail(err)
			}
		}
	}
	if jsonOutput {
		env.Kit = &KitSummary{ID: p.ID, Version: p.Version, Digest: digest, ArchiveSHA256: archiveSHA}
		env.Package = &p
		return writeEnvelope(deps.Stdout, env)
	}
	_, err := fmt.Fprintf(deps.Stdout, "Validation passed: %s %s (digest %s; local package check is not a security audit for untrusted input)\n", p.ID, p.Version, digest[:12])
	return err
}
