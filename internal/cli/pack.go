package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
)

func runPack(args []string, deps Deps) error {
	env := newEnvelope("pack")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var dir, outPath string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--out":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--out requires an output archive path"))
			}
			outPath = args[i]
		default:
			if strings.HasPrefix(args[i], "-") || dir != "" {
				return fail(apperr.Usage("invalid_usage", "usage: dtkit pack <directory> [--out <file.tar.gz>] [--json]"))
			}
			dir = args[i]
		}
	}
	if dir == "" {
		return fail(apperr.Usage("missing_directory", "usage: dtkit pack <directory> [--out <file.tar.gz>] [--json]"))
	}

	if outPath == "" {
		pkg, _, _, err := catalog.LoadDir(dir)
		if err != nil {
			return fail(err)
		}
		cwd, err := deps.Env.Getwd()
		if err != nil {
			return fail(err)
		}
		outPath = filepath.Join(cwd, fmt.Sprintf("%s-%s.tar.gz", pkg.ID, pkg.Version))
	}

	res, err := dist.Pack(dir, outPath)
	if err != nil {
		return fail(err)
	}
	env.Pack = &res
	env.Kit = &KitSummary{
		ID:            res.KitID,
		Version:       res.Version,
		Digest:        res.PackageDigest,
		ArchiveSHA256: res.ArchiveSHA256,
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Packed %s %s -> %s (sha256 %s, packageDigest %s).\n", res.KitID, res.Version, res.ArchivePath, res.ArchiveSHA256[:12], res.PackageDigest[:12])
	return nil
}
