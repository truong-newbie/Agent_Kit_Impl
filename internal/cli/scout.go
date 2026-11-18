package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/scout"
)

func runScout(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("scout")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var targetPath, query string
	var zones []string
	var maxBytes int
	pack := false
	writeSummary := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
			targetPath = args[i]
		case "--query":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--query requires a search query"))
			}
			i++
			query = args[i]
		case "--zone":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--zone requires a subdirectory path"))
			}
			i++
			zones = append(zones, args[i])
		case "--max-bytes":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--max-bytes requires an integer"))
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return fail(apperr.Usage("invalid_flag_value", fmt.Sprintf("invalid --max-bytes %q", args[i])))
			}
			maxBytes = n
		case "--pack":
			pack = true
		case "--write-summary":
			writeSummary = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected argument %q in scout", args[i])))
		}
	}

	rep, err := scout.Run(ctx, deps.Env, scout.Options{
		Root:           targetPath,
		Query:          query,
		Paths:          zones,
		MaxBytes:       maxBytes,
		IncludeContent: pack,
	})
	if err != nil {
		return fail(err)
	}

	var summaryPath string
	if writeSummary {
		md := scout.FormatMarkdownSummary(rep)
		summaryPath, err = scout.WriteCodebaseSummary(ctx, deps.Env, rep.Root, md)
		if err != nil {
			return fail(err)
		}
	}

	if jsonOutput {
		env.Scout = &rep
		if summaryPath != "" {
			env.Roots = map[string]string{"codebaseSummary": summaryPath}
		}
		return writeEnvelope(deps.Stdout, env)
	}

	if writeSummary {
		fmt.Fprintf(deps.Stdout, "Wrote %s\n", summaryPath)
	}
	fmt.Fprint(deps.Stdout, scout.FormatMarkdownSummary(rep))
	if pack {
		for _, f := range rep.MatchedFiles {
			if f.Snippet != "" {
				fmt.Fprintf(deps.Stdout, "\n--- %s (%s) ---\n%s\n", f.RelPath, f.Language, f.Snippet)
			}
		}
	}
	return nil
}
