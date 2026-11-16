package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
	"dtkit.local/dtkit/internal/planner"
)

// previewInstall exposes only a read-only fresh installation preview.
// The write operation is deliberately unavailable until the installer is verified.
func previewInstall(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "engineer" {
		return fmt.Errorf("usage: dtkit install engineer --project|--global --dry-run")
	}
	var scope, projectPath, runtime string
	var dryRun, jsonOutput bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--project", "--global":
			if scope != "" {
				return fmt.Errorf("choose exactly one scope")
			}
			scope = strings.TrimPrefix(args[i], "--")
		case "--dry-run":
			dryRun = true
		case "--json":
			jsonOutput = true
		case "--runtime", "--path":
			flag := args[i]
			i++
			if i >= len(args) {
				return fmt.Errorf("%s requires a value", flag)
			}
			if flag == "--runtime" {
				runtime = args[i]
			} else {
				projectPath = args[i]
			}
		default:
			return fmt.Errorf("unknown install option %q", args[i])
		}
	}
	if !dryRun {
		return fmt.Errorf("installation is not implemented; use --dry-run for a read-only preview")
	}
	if scope == "" {
		return fmt.Errorf("choose --project or --global")
	}
	if runtime != "" && runtime != "codex" {
		return fmt.Errorf("unsupported runtime %q", runtime)
	}
	if scope == "global" && projectPath != "" {
		return fmt.Errorf("--path cannot be used with --global")
	}
	roots := map[string]string{}
	if scope == "project" {
		if projectPath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			projectPath = cwd
			if gitRoot, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil && strings.TrimSpace(string(gitRoot)) != "" {
				projectPath = strings.TrimSpace(string(gitRoot))
			}
		}
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("project path is not a directory")
		}
		if filepath.Dir(abs) == abs {
			return fmt.Errorf("filesystem root cannot be a project")
		}
		if home, err := os.UserHomeDir(); err == nil && strings.EqualFold(filepath.Clean(abs), filepath.Clean(home)) {
			return fmt.Errorf("home directory cannot be a project")
		}
		roots["project"] = abs
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		roots["user-home"] = home
		codexHome := os.Getenv("CODEX_HOME")
		if codexHome == "" {
			codexHome = filepath.Join(home, ".codex")
		}
		roots["codex-home"], err = filepath.Abs(codexHome)
		if err != nil {
			return err
		}
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		return err
	}
	artifacts, err := codex.Render(pkg, source, scope)
	if err != nil {
		return err
	}
	plan, err := planner.InstallPreview(scope, roots, artifacts)
	if err != nil {
		return err
	}
	if jsonOutput {
		result := struct {
			Scope   string              `json:"scope"`
			Runtime string              `json:"runtime"`
			Roots   map[string]string   `json:"roots"`
			DryRun  bool                `json:"dryRun"`
			Plan    planner.InstallPlan `json:"plan"`
		}{scope, "codex", roots, true, plan}
		if err := json.NewEncoder(out).Encode(result); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "Read-only preview: engineer %s (%s)\n", scope, roots[scopeRoot(scope)])
		for _, action := range plan.Actions {
			fmt.Fprintf(out, "%s\t%s:%s\t%s\n", action.Kind, action.Root, action.Path, action.Reason)
		}
		fmt.Fprintf(out, "Conflicts: %d. Nothing was written.\n", plan.Conflicts)
	}
	if plan.Conflicts != 0 {
		return fmt.Errorf("preview found %d conflict(s)", plan.Conflicts)
	}
	return nil
}

func scopeRoot(scope string) string {
	if scope == "global" {
		return "user-home"
	}
	return "project"
}
