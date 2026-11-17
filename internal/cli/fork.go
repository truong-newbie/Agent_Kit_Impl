package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
)

func runFork(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("fork")
	jsonOutput := hasJSONFlag(args)
	redact := false
	fail := func(err error) error {
		if redact {
			err = redactAppError(err, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return fail(apperr.Usage("missing_kit", "usage: dtkit fork <kit>[@<ver>] [--as <new-id>] [--dir <parent>] [--version <x.y.z>] [--name <title>] [--same-id] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--redact] [--json]"))
	}
	kitSpec := args[0]

	var targetID, parentDir, versionStr, displayName, fromPath, sourceURL, sha256Hex string
	var sameID, offline bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--same-id":
			sameID = true
		case "--offline":
			offline = true
		case "--redact":
			redact = true
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--as", "--dir", "--version", "--name", "--from", "--source", "--sha256":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--as":
				targetID = args[i]
			case "--dir":
				parentDir = args[i]
			case "--version":
				versionStr = args[i]
			case "--name":
				displayName = args[i]
			case "--from":
				fromPath = args[i]
			case "--source":
				sourceURL = args[i]
			case "--sha256":
				sha256Hex = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "--as=") {
				targetID = strings.TrimPrefix(args[i], "--as=")
			} else if strings.HasPrefix(args[i], "--dir=") {
				parentDir = strings.TrimPrefix(args[i], "--dir=")
			} else if strings.HasPrefix(args[i], "--version=") {
				versionStr = strings.TrimPrefix(args[i], "--version=")
			} else {
				return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown fork option %q", args[i])))
			}
		}
	}

	if parentDir == "" {
		cwd, err := deps.Env.Getwd()
		if err != nil {
			cwd, err = os.Getwd()
		}
		if err != nil {
			return fail(err)
		}
		parentDir = cwd
	}

	resolved, err := resolveKitSource(ctx, deps, kitSpec, fromPath, sourceURL, sha256Hex, offline, dist.CacheReadOnly)
	if err != nil {
		return fail(err)
	}

	res, err := dist.ForkKit(parentDir, resolved.Package, resolved.Source, dist.ForkOptions{
		TargetID:    targetID,
		DisplayName: displayName,
		Version:     versionStr,
		AllowSameID: sameID,
	})
	if err != nil {
		return fail(err)
	}

	env.Fork = &res
	env.Kit = &KitSummary{ID: res.ID, Version: res.Version, Digest: res.Digest}
	if redact {
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}

	fmt.Fprintf(deps.Stdout, "Forked %s %s -> %s %s at %s (%d files, digest %s).\n",
		env.Fork.SourceID, env.Fork.SourceVersion, env.Fork.ID, env.Fork.Version, env.Fork.Dir, env.Fork.FileCount, env.Fork.Digest[:12])
	fmt.Fprintf(deps.Stdout, "Next: edit files in %s, preview with 'dtkit diff %s --from %s --project', then install or update with '--from %s'.\n",
		env.Fork.Dir, env.Fork.ID, env.Fork.Dir, env.Fork.Dir)
	return nil
}
