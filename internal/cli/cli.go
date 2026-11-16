package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"dtkit.local/dtkit/internal/catalog"
)

const version = "0.1.0-dev"

const help = `DTKit — AI engineering toolkit

Usage:
  dtkit version
  dtkit list
  dtkit info engineer
  dtkit validate <directory>
  dtkit doctor
  dtkit install engineer --project|--global --dry-run

Installation, updates and removal are under development.
`

func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := io.WriteString(stdout, help)
		return err
	}
	switch args[0] {
	case "--version", "version":
		if len(args) == 2 && args[1] == "--json" {
			return json.NewEncoder(stdout).Encode(map[string]string{"version": version})
		}
		if len(args) != 1 {
			return fmt.Errorf("version accepts only --json")
		}
		_, err := fmt.Fprintln(stdout, "dtkit", version)
		return err
	case "list":
		if len(args) != 1 {
			return fmt.Errorf("usage: dtkit list")
		}
		p, _, err := catalog.Engineer()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "%s\t%s\t%s\n", p.ID, p.Version, p.Description)
		return err
	case "info":
		if len(args) != 2 || args[1] != "engineer" {
			return fmt.Errorf("usage: dtkit info engineer")
		}
		p, _, err := catalog.Engineer()
		if err != nil {
			return err
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(p)
	case "validate":
		if len(args) != 2 {
			return fmt.Errorf("usage: dtkit validate <directory>")
		}
		p, err := catalog.Load(os.DirFS(args[1]))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Basic validation passed: %s %s (not a security audit)\n", p.ID, p.Version)
		return err
	case "doctor":
		if len(args) != 1 {
			return fmt.Errorf("usage: dtkit doctor")
		}
		path, err := exec.LookPath("codex")
		if err != nil {
			fmt.Fprintln(stdout, "Codex CLI: not found on PATH")
			return fmt.Errorf("Codex runtime is not ready")
		}
		fmt.Fprintln(stdout, "Codex CLI:", path)
		return nil
	case "install":
		return previewInstall(args[1:], stdout)
	default:
		if strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("unknown option %q", args[0])
		}
		return fmt.Errorf("command %q is not available yet; run dtkit help", args[0])
	}
}
