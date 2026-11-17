package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func runCompat(args []string, deps Deps) error {
	env := newEnvelope("compat")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit compat <snapshot|diff> [options]"))
	}
	sub := args[0]
	env.Command = "compat " + sub
	env.DryRun = true

	switch sub {
	case "snapshot":
		var outPath string
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--json":
				jsonOutput = true
			case "--verbose":
				deps.Verbose = true
			case "--out":
				i++
				if i >= len(args) {
					return fail(apperr.Usage("missing_flag_value", "--out requires a value"))
				}
				outPath = args[i]
			default:
				return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown compat snapshot option %q", args[i])))
			}
		}
		rep, err := adapter.BuildCompatibilityReport()
		if err != nil {
			return fail(err)
		}
		env.CompatReport = &rep
		if outPath != "" {
			raw, mErr := json.MarshalIndent(rep, "", "  ")
			if mErr != nil {
				return fail(mErr)
			}
			if err := atomicWriteFile(outPath, append(raw, '\n'), 0644); err != nil {
				return fail(err)
			}
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "DTKit Compatibility Snapshot (%s %s, %d runtime/scope entries):\n", rep.KitID, rep.KitVersion, len(rep.Entries))
		for _, e := range rep.Entries {
			fmt.Fprintf(deps.Stdout, "  %s/%s\trev=%s\thooks=%v\tartifacts=%d\tintegration=%s\n",
				e.Runtime, e.Scope, e.AdapterRevision, e.HooksSupported, e.ArtifactCount, e.Integration.Status)
		}
		return nil

	case "diff":
		var baselinePath string
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--json":
				jsonOutput = true
			case "--verbose":
				deps.Verbose = true
			case "--baseline":
				i++
				if i >= len(args) {
					return fail(apperr.Usage("missing_flag_value", "--baseline requires a value"))
				}
				baselinePath = args[i]
			default:
				if !strings.HasPrefix(args[i], "-") && baselinePath == "" {
					baselinePath = args[i]
				} else {
					return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown compat diff option %q", args[i])))
				}
			}
		}
		if baselinePath == "" {
			baselinePath = "docs/verification/last-known-good-compat.json"
		}
		rawBase, err := os.ReadFile(baselinePath)
		if err != nil {
			return fail(apperr.WithPath(apperr.Usage("baseline_not_found", "unable to read baseline compatibility snapshot"), baselinePath))
		}
		baseline, err := decodeCompatibilitySnapshot(rawBase)
		if err != nil {
			return fail(apperr.Wrap(apperr.ExitUsage, "invalid_compat_baseline", "invalid compatibility baseline JSON", err))
		}
		current, err := adapter.BuildCompatibilityReport()
		if err != nil {
			return fail(err)
		}
		diff := adapter.DiffCompatibilityReports(baseline, current)
		env.CompatReport = &current
		env.CompatDiff = &diff
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		if diff.ChangedCount == 0 {
			fmt.Fprintln(deps.Stdout, "No compatibility changes vs baseline.")
			return nil
		}
		fmt.Fprintf(deps.Stdout, "Compatibility changes (%d, breaking=%v):\n", diff.ChangedCount, diff.HasBreakingChanges)
		for _, ch := range diff.Changes {
			fmt.Fprintf(deps.Stdout, "  [%s] %s/%s (breaking=%v): %s\n", ch.Status, ch.Runtime, ch.Scope, ch.Breaking, ch.Details)
		}
		return nil

	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown compat subcommand %q (expected snapshot or diff)", sub)))
	}
}

func decodeCompatibilitySnapshot(raw []byte) (adapter.CompatibilityReport, error) {
	if rep, err := adapter.ParseCompatibilityReport(raw); err == nil {
		return rep, nil
	}
	// Also accept CLI Envelope wrapper containing compatReport.
	var env struct {
		CompatReport *adapter.CompatibilityReport `json:"compatReport"`
	}
	if err := json.Unmarshal(raw, &env); err == nil && env.CompatReport != nil && env.CompatReport.SchemaVersion == 1 {
		return *env.CompatReport, nil
	}
	return adapter.CompatibilityReport{}, fmt.Errorf("missing valid CompatibilityReport schemaVersion 1")
}
