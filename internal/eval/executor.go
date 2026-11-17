package eval

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CLIExecutor invokes a local coding-agent CLI binary (claude, codex, opencode) inside
// an isolated home/config environment. Unless AllowHostAuth is true, no host credential
// files or API keys are forwarded so missing isolated auth fails closed as environment_blocked.
type CLIExecutor struct {
	ExecutablePath string
	AllowHostAuth  bool
	ExtraEnv       []string
}

func (c CLIExecutor) Execute(ctx context.Context, req ExecRequest) (ExecResult, error) {
	bin := strings.TrimSpace(c.ExecutablePath)
	if bin == "" {
		bin = req.Runtime
	}
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return ExecResult{
			Blocked:       true,
			BlockedReason: fmt.Sprintf("runtime executable %q not found on PATH: %v", bin, err),
		}, nil
	}

	args, authEnvNeeded, err := buildCLIArgs(req)
	if err != nil {
		return ExecResult{Blocked: true, BlockedReason: err.Error()}, nil
	}

	if !c.AllowHostAuth && !hasEnvKey(c.ExtraEnv, authEnvNeeded) && !hasIsolatedAuth(req.Runtime, req.HomeDir) {
		return ExecResult{
			Blocked:       true,
			BlockedReason: fmt.Sprintf("isolated auth not configured for %s under %s (no %s passed)", req.Runtime, req.HomeDir, strings.Join(authEnvNeeded, "/")),
		}, nil
	}

	cmd := exec.CommandContext(ctx, resolved, args...)
	cmd.Dir = req.WorkspaceDir
	cmd.Env = isolatedEnv(req.HomeDir, c.ExtraEnv, c.AllowHostAuth, authEnvNeeded)

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	runErr := cmd.Run()
	out := outBuf.String()
	if errors.Is(ctx.Err(), context.Canceled) {
		return ExecResult{Cancelled: true, Output: out}, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ExecResult{TimedOut: true, Output: out}, nil
	}
	if looksAuthOrQuotaBlocked(out) {
		return ExecResult{
			ExitCode:      exitCode(runErr),
			Output:        out,
			Blocked:       true,
			BlockedReason: "runtime reported missing authentication, consent, or quota",
		}, nil
	}
	return ExecResult{
		ExitCode: exitCode(runErr),
		Output:   out,
	}, nil
}

func buildCLIArgs(req ExecRequest) ([]string, []string, error) {
	switch req.Runtime {
	case "claude":
		args := []string{"-p", req.Scenario.Prompt, "--output-format", "json"}
		if req.ModelID != "" {
			args = append(args, "--model", req.ModelID)
		}
		if req.MaxBudgetUSD != nil && *req.MaxBudgetUSD > 0 {
			args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", *req.MaxBudgetUSD))
		}
		return args, []string{"ANTHROPIC_API_KEY"}, nil
	case "codex":
		args := []string{"exec", req.Scenario.Prompt, "--json"}
		if req.ModelID != "" {
			args = append(args, "--model", req.ModelID)
		}
		return args, []string{"OPENAI_API_KEY"}, nil
	case "opencode":
		args := []string{"run", req.Scenario.Prompt, "--format", "json"}
		if req.ModelID != "" {
			args = append(args, "--model", req.ModelID)
		}
		return args, []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENCODE_API_KEY"}, nil
	default:
		return nil, nil, fmt.Errorf("CLIExecutor does not support headless runtime %q", req.Runtime)
	}
}

func isolatedEnv(homeDir string, extraEnv []string, allowHostAuth bool, authKeys []string) []string {
	deny := map[string]bool{
		"HOME": true, "USERPROFILE": true, "HOMEDRIVE": true, "HOMEPATH": true,
		"XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true, "XDG_STATE_HOME": true,
		"CODEX_HOME": true, "CLAUDE_CONFIG_DIR": true,
	}
	if !allowHostAuth {
		for _, k := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "OPENCODE_API_KEY", "GEMINI_API_KEY"} {
			deny[k] = true
		}
		for _, k := range authKeys {
			deny[k] = true
		}
	}

	var env []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || deny[strings.ToUpper(name)] {
			continue
		}
		env = append(env, kv)
	}
	configHome := filepath.Join(homeDir, ".config")
	dataHome := filepath.Join(homeDir, ".local", "share")
	stateHome := filepath.Join(homeDir, ".local", "state")
	codexHome := filepath.Join(homeDir, ".codex")
	claudeHome := filepath.Join(homeDir, ".claude")
	env = append(env,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+dataHome,
		"XDG_STATE_HOME="+stateHome,
		"CODEX_HOME="+codexHome,
		"CLAUDE_CONFIG_DIR="+claudeHome,
	)
	env = append(env, extraEnv...)
	return env
}

func hasEnvKey(env []string, keys []string) bool {
	for _, kv := range env {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || strings.TrimSpace(val) == "" {
			continue
		}
		for _, k := range keys {
			if strings.EqualFold(name, k) {
				return true
			}
		}
	}
	return false
}

func hasIsolatedAuth(runtime, homeDir string) bool {
	switch runtime {
	case "claude":
		_, err := os.Stat(filepath.Join(homeDir, ".claude", ".credentials.json"))
		return err == nil
	case "codex":
		_, err := os.Stat(filepath.Join(homeDir, ".codex", "auth.json"))
		return err == nil
	default:
		return false
	}
}

func looksAuthOrQuotaBlocked(out string) bool {
	lower := strings.ToLower(out)
	for _, needle := range []string{
		"not logged in",
		"please run `claude auth login`",
		"please log in",
		"invalid api key",
		"missing api key",
		"insufficient quota",
		"exceeded budget",
		"rate limit",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}
