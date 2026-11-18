# DTKit

DTKit is a single-binary Go CLI toolkit for packaging, validating, and safely installing reusable AI engineering skills, rules, workflows, and templates across coding-agent runtimes (`codex`, `gemini`, `opencode`, `cursor`, `claude`).

## Current Status

- **Release Stream:** `main` integrates release-hardening (`N00`–`N12`, `A01`–`A04`), v0.5 engineering (`B01`–`B08`, `engineer@0.4.0`), and Milestone C (`C01`–`C06`), tagged for **`v0.6.0`** (`release.yml` builds Windows/Linux/macOS archives + `SHA256SUMS` and creates a Draft GitHub Release on tag push).
- **Implemented (automated-tested in Go / Docker):**
  - Core CLI & safety engine (`version`, `help`, `list`, `info`, `validate`, `doctor`, `install`, `status`, `update`, `remove`, `recover`, `completion`) with `--dry-run`, `--diff`, `--yes`, `--json`, `--preset` (`minimal`, `review-only`, `engineering`), `--components`, state v3 multi-kit registry (`v1`/`v2` backward-compatible), managed blocks, PID/token locks, transaction journals, and conditional rollback.
  - Project profile, quality runner & workflows (`dtkit project inspect|init`, `dtkit check [--changed|--all] [--allow-autofix]`, `dtkit diff`, `dtkit fork`, `dtkit lock <generate|verify|diff|sign|keygen>`, `dtkit sync`, `dtkit team <export|import|template>`, `dtkit compat <snapshot|diff>`) using strict argv-only `.dtkit/project.json`, portable `dtkit.lock`, Ed25519 trust policies, and `.dtkit/local/check-trust.json`.
  - Opt-in runtime hooks & plugins (`dtkit hooks list|status|enable|disable`) for `codex` (`SessionStart` `session-context`), `claude` (`PostToolUse` `check-changed`, `PreToolUse` `guard-edit`), `gemini` (`AfterTool` `check-changed`, `BeforeTool` `guard-edit`), `cursor` (`afterFileEdit` `check-changed`), and `opencode` (`.opencode/plugins/dtkit-quality.js` `tool.execute.after`).
  - Independent authoring & packaging (`dtkit create kit|skill`, `dtkit pack`, `--from <dir|archive>`, `--source <url>`, `--sha256`, `--offline`, digest cache) and `engineer@0.4.0` repository-neutral engineering kit.
  - Five runtime adapters (`codex`, `gemini`, `opencode`, `cursor`, `claude`) — 5-runtime coexistence, migration, preset switching, and state separation tested in automation.
- **Verification & Benchmark Truthfulness Boundary:**
  - **Codex CLI (`0.169.0`, `project` + `$CODEX_HOME`):** Partial — project skill/instruction discovery & isolated `$CODEX_HOME/AGENTS.md` observed in [docs/verification/codex-0.169.0-2026-11-16.md](docs/verification/codex-0.169.0-2026-11-16.md); global layout kept per [docs/verification/codex-global-windows-decision-2026-11-17.md](docs/verification/codex-global-windows-decision-2026-11-17.md); sandboxed PS subcommands blocked by host policy; live session hook invocation unverified.
  - **OpenCode (`1.19.5`, `project` + `global`):** Partial — isolated project-only, global-only, combined 14-skill discovery alongside other runtimes, and clean removal observed in [docs/verification/opencode-1.19.5-2026-11-16.md](docs/verification/opencode-1.19.5-2026-11-16.md); live LLM turn blocked by absent isolated credentials.
  - **Claude Code (`claude`), Gemini CLI (`gemini`), & Cursor (`cursor`):** Experimental (`unverified` live) — automated adapter, hook-merge, lifecycle, and coexistence tests pass in Go; live `claude 2.1.319` `--no-session-persistence --setting-sources project` probe returned `401 Invalid or expired API key` (`total_cost_usd: 0`), while `gemini` and `cursor` CLIs were absent on the verification host ([docs/verification/gemini-cursor-external-blocker-2026-11-16.md](docs/verification/gemini-cursor-external-blocker-2026-11-16.md)).
  - **Evaluation harness (`internal/eval`):** Automated Go fixture grading (`engineer@0.3.0` frozen manifest in [docs/evaluation/manifest.json](docs/evaluation/manifest.json) and [docs/evaluation/pilot-report-2026-11-17.md](docs/evaluation/pilot-report-2026-11-17.md)) passes locally; live multi-run A/B model benchmarks (`engineer@0.3.0` vs. `engineer@0.4.0`) remain `blocked` pending valid isolated credentials and budget.

See [docs/compatibility.md](docs/compatibility.md) for the per-runtime 4-layer evidence matrix, [docs/cli.md](docs/cli.md) for CLI contracts, [docs/release.md](docs/release.md) for terminal installation, changelog, and troubleshooting, [docs/support.md](docs/support.md) for support & incident response policy, and [docs/progress.md](docs/progress.md) for verified task records.

## Terminal Installation (Pinned Release Binary)

For a published release/pre-release tag (such as `v0.6.0`)—or when building local archives via `.\scripts\package.ps1 -Version 0.6.0`—follow the checksum-verified, user-local installation steps in [docs/release.md](docs/release.md#terminal-install--checksum-verification-pinned-version-no-admin) using [scripts/install.sh](scripts/install.sh) or [scripts/install.ps1](scripts/install.ps1) (never skip `SHA256SUMS` verification).

## Build and Verification (Dev Source)

Requires Go `1.27.4` or newer (or Docker Desktop if Windows Application Control / Device Guard blocks unsigned `go.exe`):

```powershell
# Docker-based formatting, race tests, vet, and Linux binary build:
.\scripts\verify-docker.ps1
```

With an approved native Go `1.27.4+` toolchain:

```powershell
gofmt -l cmd internal schemas
go test -race ./...
go vet ./...
go build -trimpath -o .\dist\dtkit ./cmd/dtkit
```

*(Note: `go install github.com/truong-newbie/Agent_Kit_Impl/cmd/dtkit@latest` resolves `main`; for pinned reproducible builds use the checksum-verified archives attached to the GitHub Release.)*

## Quick Start

```powershell
# Check environment and runtime readiness (--runtime defaults to codex)
dtkit doctor
dtkit doctor --runtime claude
dtkit doctor --runtime gemini
dtkit doctor --runtime opencode
dtkit doctor --runtime cursor

dtkit list
dtkit info engineer

# Inspect & initialize stack-agnostic project quality profile (.dtkit/project.json)
dtkit project inspect
dtkit project init --yes
dtkit check --all --yes

# Preview then install for a runtime (full kit, preset, or explicit components)
dtkit install engineer --project --runtime claude --dry-run
dtkit install engineer --project --runtime claude --preset minimal --yes
dtkit status --project --runtime claude

# Install / co-exist with other runtimes
dtkit install engineer --project --runtime codex --yes
dtkit install engineer --project --runtime opencode --yes
dtkit install engineer --project --runtime gemini --yes
dtkit install engineer --project --runtime cursor --yes

# Global installation
dtkit install engineer --global --runtime claude --yes

# Opt-in hooks (Codex SessionStart; Claude PostToolUse check-changed & PreToolUse guard-edit)
dtkit hooks enable session-context --runtime codex --project --dry-run
dtkit hooks enable check-changed --runtime claude --project --dry-run

# Authoring, packing, updating, removing, and recovery
dtkit create kit my-kit
dtkit pack .\my-kit
dtkit update engineer --project --runtime claude --preset engineering --yes
dtkit remove engineer --project --runtime codex --dry-run
dtkit recover --project --dry-run
```

## Scopes, Shared Files, and Safety

- **Per-runtime layout & state isolation:**
  - **Codex (`codex`):** `<project>/AGENTS.md` / `$CODEX_HOME/AGENTS.md` (managed block), `.agents/skills/dtkit-<scope>-*/SKILL.md`, `.dtkit/content/<kit>/...`, `.dtkit/state.json`, optional `.codex/hooks.json` or `$CODEX_HOME/hooks.json`.
  - **Gemini CLI (`gemini`):** `<project>/GEMINI.md` / `$GEMINI_CLI_HOME/.gemini/GEMINI.md` (managed block), `.gemini/skills/dtkit-gemini-project-*` / `$GEMINI_CLI_HOME/.gemini/skills/dtkit-gemini-global-*`, `.dtkit/runtimes/gemini/content/<kit>/...`, `.dtkit/runtimes/gemini/state.json`.
  - **OpenCode (`opencode`):** Shared `AGENTS.md` (runtime-namespaced block `<!-- dtkit:begin <kit> opencode <scope> -->`) / `$OPENCODE_CONFIG_DIR/AGENTS.md` (or `~/.config/opencode/AGENTS.md`), `.opencode/skills/dtkit-opencode-project-*` / `<opencode-home>/skills/dtkit-opencode-global-*`, `.dtkit/runtimes/opencode/content/<kit>/...`, `.dtkit/runtimes/opencode/state.json`.
  - **Cursor (`cursor`):** Project rule `.cursor/rules/dtkit-<kit>.mdc` (`alwaysApply: true`), `.cursor/skills/dtkit-cursor-<scope>-*`, `.dtkit/runtimes/cursor/content/<kit>/...`, `.dtkit/runtimes/cursor/state.json`. Global User Rules live in Cursor Settings UI and are not managed on disk (`--global` installs skills + resources only).
  - **Claude Code (`claude`):** `<project>/CLAUDE.md` / `$CLAUDE_CONFIG_DIR/CLAUDE.md` (or `~/.claude/CLAUDE.md`, managed block), `.claude/skills/dtkit-claude-project-*` / `<claude-home>/skills/dtkit-claude-global-*`, `.dtkit/runtimes/claude/content/<kit>/...`, `.dtkit/runtimes/claude/state.json`, optional `.claude/settings.json` or `<claude-home>/settings.json`.
- **Operational data:** Add `.dtkit/local/` to `.gitignore` so local lock files, journals, and transaction backups across all runtimes are not committed.
- **Conflict protection:** Unowned content outside DTKit managed blocks is preserved byte-for-byte (including BOM and CRLF/LF). Modified managed files, unowned targets at whole-file paths, or blocking overrides halt mutations with exit code `3`. `--yes` only skips confirmation prompts; it never overrides conflicts.

Next implementation handoff: [RC and v0.5 development plan](DTKIT-RC-AND-V05-PLAN.md) — release gates, Claude integration, reproducible evaluation, project profiles, quality checks/hooks, presets, and the subsequent roadmap.

More documentation: [docs/cli.md](docs/cli.md), [docs/development.md](docs/development.md), [docs/compatibility.md](docs/compatibility.md), [docs/release.md](docs/release.md), [DTKIT-IMPLEMENTATION-PLAN.md](DTKIT-IMPLEMENTATION-PLAN.md), [DTKIT-NEXT-PHASE-PLAN.md](DTKIT-NEXT-PHASE-PLAN.md).
