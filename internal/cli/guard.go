package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/guard"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runGuard(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("guard")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit guard <status|lock|unlock|verify> [options]"))
	}
	sub := args[0]
	env.Command = "guard " + sub

	var targetPath, modeArg string
	var allowGlobs, denyGlobs []string
	protectTests := false

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
			targetPath = args[i]
		case "--mode":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--mode requires read-only|tdd-red|tdd-green|scoped"))
			}
			i++
			modeArg = args[i]
		case "--allow":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--allow requires a glob pattern"))
			}
			i++
			allowGlobs = append(allowGlobs, args[i])
		case "--deny":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--deny requires a glob pattern"))
			}
			i++
			denyGlobs = append(denyGlobs, args[i])
		case "--protect-tests":
			protectTests = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q for dtkit guard %s", args[i], sub)))
		}
	}

	projRes, err := paths.ResolveProject(ctx, deps.Env, targetPath)
	if err != nil {
		return fail(err)
	}
	root := projRes.Root

	switch sub {
	case "status":
		pol, pErr := guard.LoadPolicy(root)
		if pErr != nil {
			return fail(pErr)
		}
		ver, vErr := guard.VerifyWorkspace(root)
		if vErr != nil {
			return fail(vErr)
		}
		st, _ := guard.LoadStreak(root)
		rep := guard.StatusReport{
			Root:         root,
			Policy:       pol,
			Verification: ver,
			Streak:       st,
		}
		if jsonOutput {
			env.GuardStatus = &rep
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Guard root: %s\n", rep.Root)
		fmt.Fprintf(deps.Stdout, "Mode: %s (protectTests=%v)\n", rep.Policy.Mode, rep.Policy.ProtectTests)
		if len(rep.Policy.AllowGlobs) > 0 {
			fmt.Fprintf(deps.Stdout, "Allow globs: %s\n", strings.Join(rep.Policy.AllowGlobs, ", "))
		}
		if len(rep.Policy.DenyGlobs) > 0 {
			fmt.Fprintf(deps.Stdout, "Deny globs: %s\n", strings.Join(rep.Policy.DenyGlobs, ", "))
		}
		fmt.Fprintf(deps.Stdout, "Verified receipt: %v (digest=%s)\n", rep.Verification.Verified, rep.Verification.CurrentDigest)
		if rep.Verification.Reason != "" {
			fmt.Fprintf(deps.Stdout, "Receipt status: %s\n", rep.Verification.Reason)
		}
		if rep.Streak.ConsecutiveFailures > 0 {
			fmt.Fprintf(deps.Stdout, "Consecutive check failures: %d\n", rep.Streak.ConsecutiveFailures)
		}
		return nil

	case "lock":
		if strings.TrimSpace(modeArg) == "" {
			return fail(apperr.Usage("missing_mode", "dtkit guard lock requires --mode <read-only|tdd-red|tdd-green|scoped>"))
		}
		pol, lErr := guard.SavePolicy(root, guard.Policy{
			Mode:         guard.Mode(modeArg),
			AllowGlobs:   allowGlobs,
			DenyGlobs:    denyGlobs,
			ProtectTests: protectTests,
		}, time.Now())
		if lErr != nil {
			return fail(lErr)
		}
		ver, _ := guard.VerifyWorkspace(root)
		st, _ := guard.LoadStreak(root)
		rep := guard.StatusReport{Root: root, Policy: pol, Verification: ver, Streak: st}
		if jsonOutput {
			env.GuardStatus = &rep
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Locked guard mode %q at %s\n", pol.Mode, root)
		return nil

	case "unlock":
		if uErr := guard.ClearPolicy(root); uErr != nil {
			return fail(uErr)
		}
		ver, _ := guard.VerifyWorkspace(root)
		st, _ := guard.LoadStreak(root)
		rep := guard.StatusReport{
			Root:         root,
			Policy:       guard.Policy{SchemaVersion: guard.SchemaVersion1, Mode: guard.ModeUnlocked},
			Verification: ver,
			Streak:       st,
		}
		if jsonOutput {
			env.GuardStatus = &rep
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Unlocked guard firewall at %s\n", root)
		return nil

	case "verify":
		pol, pErr := guard.LoadPolicy(root)
		if pErr != nil {
			return fail(pErr)
		}
		ver, vErr := guard.VerifyWorkspace(root)
		if vErr != nil {
			return fail(vErr)
		}
		st, _ := guard.LoadStreak(root)
		rep := guard.StatusReport{
			Root:         root,
			Policy:       pol,
			Verification: ver,
			Streak:       st,
		}
		if !ver.Verified {
			err := apperr.WithHint(
				apperr.Conflict("verification_failed", ver.Reason),
				"run 'dtkit check --all' after final edits and resolve any test tampering findings",
			)
			if jsonOutput {
				env.GuardStatus = &rep
				return failJSON(deps.Stdout, env, err)
			}
			return err
		}
		if jsonOutput {
			env.GuardStatus = &rep
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Verified receipt matches current workspace digest (%s)\n", ver.CurrentDigest)
		return nil

	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown guard subcommand %q (expected status|lock|unlock|verify)", sub)))
	}
}
