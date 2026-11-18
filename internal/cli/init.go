package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/mcp"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/scout"
)

// InitSummary summarizes the actions taken (or previewed) by dtkit init.
type InitSummary struct {
	Root            string   `json:"root"`
	Runtimes        []string `json:"runtimes"`
	KitID           string   `json:"kitId"`
	KitVersion      string   `json:"kitVersion"`
	Preset          string   `json:"preset,omitempty"`
	CodebaseSummary string   `json:"codebaseSummary,omitempty"`
	ProjectProfile  string   `json:"projectProfile,omitempty"`
	MCPConfigured   []string `json:"mcpConfigured,omitempty"`
	HooksEnabled    []string `json:"hooksEnabled,omitempty"`
	LockfilePath    string   `json:"lockfilePath,omitempty"`
}

func runInit(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("init")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var targetPath, runtimeArg, presetName string
	var allRuntimes, withMCP, withHooks, withLock, dryRun, yes bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			i++
			targetPath = args[i]
		case "--runtime":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a runtime ID"))
			}
			i++
			runtimeArg = args[i]
		case "--preset":
			if i+1 >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--preset requires a preset name"))
			}
			i++
			presetName = args[i]
		case "--all-runtimes":
			allRuntimes = true
		case "--with-mcp":
			withMCP = true
		case "--with-hooks":
			withHooks = true
		case "--lock":
			withLock = true
		case "--dry-run":
			dryRun = true
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonOutput = true
		default:
			return fail(apperr.Usage("invalid_flag", fmt.Sprintf("unexpected flag %q for dtkit init", args[i])))
		}
	}

	if allRuntimes && runtimeArg != "" {
		return fail(apperr.Usage("conflicting_flags", "choose at most one of --runtime or --all-runtimes"))
	}
	if !dryRun && !yes && (jsonOutput || !deps.IsInteractive()) {
		return fail(apperr.Usage("confirmation_required", "pass --yes to confirm project initialization in non-interactive or --json mode"))
	}

	projRes, err := paths.ResolveProject(ctx, deps.Env, targetPath)
	if err != nil {
		return fail(err)
	}
	root := projRes.Root

	var targetRuntimes []string
	if allRuntimes {
		targetRuntimes = adapter.SupportedRuntimes()
	} else {
		prof, pErr := resolveRuntimeProfile(runtimeArg)
		if pErr != nil {
			return fail(pErr)
		}
		targetRuntimes = []string{prof.ID}
	}

	engPkg, _, err := catalog.Engineer()
	if err != nil {
		return fail(err)
	}

	summary := InitSummary{
		Root:            root,
		Runtimes:        targetRuntimes,
		KitID:           engPkg.ID,
		KitVersion:      engPkg.Version,
		Preset:          presetName,
		CodebaseSummary: "docs/codebase-summary.md",
		ProjectProfile:  project.ProfileRelPath,
	}

	if dryRun {
		if withMCP {
			for _, rt := range targetRuntimes {
				rel, _ := mcp.ConfigRelPath(rt)
				summary.MCPConfigured = append(summary.MCPConfigured, rel)
			}
		}
		if withHooks {
			for _, rt := range targetRuntimes {
				summary.HooksEnabled = append(summary.HooksEnabled, rt+":"+defaultHookForRuntime(rt))
			}
		}
		if withLock {
			summary.LockfilePath = "dtkit.lock"
		}
		env.DryRun = true
		env.InitSummary = &summary
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Preview dtkit init at %s (runtimes=%s, kit=%s@%s)\n", root, strings.Join(targetRuntimes, ","), engPkg.ID, engPkg.Version)
		return nil
	}

	// 1. Write docs/codebase-summary.md if absent
	summaryTarget := filepath.Join(root, "docs", "codebase-summary.md")
	if _, statErr := os.Stat(summaryTarget); os.IsNotExist(statErr) {
		sRep, sErr := scout.Run(ctx, deps.Env, scout.Options{Root: root})
		if sErr != nil {
			return fail(sErr)
		}
		if _, wErr := scout.WriteCodebaseSummary(ctx, deps.Env, root, scout.FormatMarkdownSummary(sRep)); wErr != nil {
			return fail(wErr)
		}
	}

	// 2. Write .dtkit/project.json if absent
	if _, _, exists, lErr := project.LoadProfile(root); lErr != nil {
		return fail(lErr)
	} else if !exists {
		if _, iErr := project.Init(ctx, deps.Env, project.InitOptions{Path: root}); iErr != nil {
			return fail(iErr)
		}
	}

	silentDeps := deps
	silentDeps.Stdout = ioDiscard{}

	// 3. Install engineer@0.5.0 idempotently per target runtime
	for _, rt := range targetRuntimes {
		roots := map[string]string{"project": root}
		reps, _ := installer.InspectRuntimeAll(rt, "project", roots)
		alreadyInstalled := false
		for _, r := range reps {
			if r.Installed && r.KitID == "engineer" {
				alreadyInstalled = true
				break
			}
		}
		if !alreadyInstalled {
			instArgs := []string{"engineer", "--project", "--runtime", rt, "--path", root, "--yes"}
			if presetName != "" {
				instArgs = append(instArgs, "--preset", presetName)
			}
			if iErr := runInstall(ctx, instArgs, silentDeps); iErr != nil {
				return fail(iErr)
			}
		}

		// 4. Optional MCP wiring
		if withMCP {
			st, mErr := mcp.EnableMCPConfig(root, rt, "", false)
			if mErr != nil {
				return fail(mErr)
			}
			summary.MCPConfigured = append(summary.MCPConfigured, st.ConfigPath)
		}

		// 5. Optional default hook wiring
		if withHooks {
			hookName := defaultHookForRuntime(rt)
			hkArgs := []string{"enable", hookName, "--project", "--runtime", rt, "--path", root, "--yes"}
			if hErr := runHooks(ctx, hkArgs, silentDeps); hErr != nil {
				return fail(hErr)
			}
			summary.HooksEnabled = append(summary.HooksEnabled, rt+":"+hookName)
		}
	}

	// 6. Optional lockfile generation
	if withLock {
		lockArgs := []string{"generate", "--path", root, "--yes"}
		if !allRuntimes && len(targetRuntimes) == 1 {
			lockArgs = append(lockArgs, "--runtime", targetRuntimes[0])
		}
		if lErr := runLock(ctx, lockArgs, silentDeps); lErr != nil {
			return fail(lErr)
		}
		summary.LockfilePath = "dtkit.lock"
	}

	env.InitSummary = &summary
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Initialized DTKit at %s (runtimes=%s, kit=%s@%s)\n", root, strings.Join(targetRuntimes, ","), engPkg.ID, engPkg.Version)
	return nil
}

func defaultHookForRuntime(rt string) string {
	if rt == "codex" {
		return "session-context"
	}
	return "check-changed"
}
