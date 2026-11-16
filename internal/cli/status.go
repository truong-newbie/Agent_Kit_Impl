package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
	"dtkit.local/dtkit/internal/installer"
)

func status(args []string, out io.Writer) error {
	var scope, projectPath string
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project", "--global":
			if scope != "" {
				return fmt.Errorf("choose exactly one scope")
			}
			scope = strings.TrimPrefix(args[i], "--")
		case "--json":
			jsonOutput = true
		case "--path":
			i++
			if i >= len(args) {
				return fmt.Errorf("--path requires a value")
			}
			projectPath = args[i]
		default:
			return fmt.Errorf("unknown status option %q", args[i])
		}
	}
	if scope == "" {
		return fmt.Errorf("usage: dtkit status --project|--global [--path <dir>] [--json]")
	}
	if scope == "global" && projectPath != "" {
		return fmt.Errorf("--path cannot be used with --global")
	}
	roots, err := statusRoots(scope, projectPath)
	if err != nil {
		return err
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		return err
	}
	artifacts, err := codex.Render(pkg, source, scope)
	if err != nil {
		return err
	}
	report, err := installer.Inspect(scope, roots, artifacts)
	if err != nil {
		return err
	}
	if jsonOutput {
		return json.NewEncoder(out).Encode(report)
	}
	if !report.Installed {
		_, err = fmt.Fprintf(out, "DTKit is not installed for %s.\n", scope)
		return err
	}
	if report.Healthy {
		_, err = fmt.Fprintf(out, "DTKit %s installation is healthy (%d artifacts).\n", scope, len(report.Artifacts))
		return err
	}
	_, err = fmt.Fprintf(out, "DTKit %s installation has drift:\n", scope)
	if err != nil {
		return err
	}
	for _, artifact := range report.Artifacts {
		if artifact.Status != "healthy" {
			if _, err := fmt.Fprintf(out, "%s\t%s:%s\t%s\n", artifact.Status, artifact.Root, artifact.Path, artifact.Detail); err != nil {
				return err
			}
		}
	}
	return nil
}

func statusRoots(scope, projectPath string) (map[string]string, error) {
	if scope == "project" {
		if projectPath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			projectPath = cwd
		}
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("project path is not a directory")
		}
		return map[string]string{"project": abs}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(home, ".codex")
	}
	codexHome, err = filepath.Abs(codexHome)
	if err != nil {
		return nil, err
	}
	return map[string]string{"user-home": home, "codex-home": codexHome}, nil
}
