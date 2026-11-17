# Claude Code Verification Report — 2.1.319 (2026-11-17)

- **Date:** 2026-11-17 (on branch `feat/v0.5`)
- **OS:** Windows host (`claude.exe` `2.1.319 (Claude Code)` at `C:\Users\HP\.local\bin\claude.exe`) + Docker `golang:1.27.4` Linux container (`.\scripts\verify-docker.ps1`)
- **Engineer Kit Baseline:** `0.3.0` (`runtimeTargets` adds `claude`; instructions/skills content intentionally kept identical to `0.2.0` as the frozen `B02` baseline)

## Observed CLI Probes (`claude 2.1.319`)

1. **`claude --version`:** `2.1.319 (Claude Code)`
2. **`claude auth status` (default user profile, credential values not inspected):** `loggedIn: true`, `authMethod: oauth_token`, `configDirectory: C:\Users\HP\.claude`.
3. **`CLAUDE_CONFIG_DIR` isolated probe (`$env:TEMP\dtkit-claude-probe`):**
   - `configDirectory` resolved directly to `<temp>\dtkit-claude-probe` (not `<temp>\.claude`), confirming `$CLAUDE_CONFIG_DIR` overrides `~/.claude` directly (`ResolveClaudeGlobal` matches this behavior).
   - Reported `loggedIn: false`, `authMethod: none` (`ANTHROPIC_API_KEY` not set in process env).
4. **Automated tests (`.\scripts\verify-docker.ps1`):**
   - `internal/paths/claude_test.go`: `ResolveClaudeGlobal` default `~/.claude`, `CLAUDE_CONFIG_DIR` override, relative path warning, symlink rejection.
   - `internal/adapter/claude/render_test.go`: `CLAUDE.md` (`<!-- dtkit:begin engineer claude <scope> -->`), `.claude/skills/dtkit-claude-project-*`, `~/.claude/skills/dtkit-claude-global-*`, `.dtkit/runtimes/claude/content/engineer/`.
   - `internal/cli/claude_test.go`: 5-runtime coexistence (`codex`, `gemini`, `opencode`, `cursor`, `claude`), `CLAUDE_CONFIG_DIR` `profile_mismatch`, monorepo subdir & Git worktree root resolution, clean removal preserving personal `CLAUDE.md`, user skills, and `.claude/settings.json`.

## Live Task Status & Honest Blocker

- **Current tier:** `automated-tested` (`IntegrationUnverified` retained in `internal/adapter/adapter.go`).
- **Remaining blocker for `live-task-verified` (`B01`):**
  - Isolated `$CLAUDE_CONFIG_DIR` has `loggedIn: false` (`ANTHROPIC_API_KEY` unset), and running against the owner's real `~/.claude` OAuth session requires explicit owner budget/permission per `DTKIT-RC-AND-V05-PLAN.md` §1.6 & §5 `B01`.
  - To execute the bounded `B01` live fixture task cleanly later, set `ANTHROPIC_API_KEY` (with owner budget approval) + isolated `CLAUDE_CONFIG_DIR` and run `claude --print --max-budget-usd <cap> --no-session-persistence ...`.
