# DTKit

DTKit is a single-binary Go CLI toolkit for packaging, validating, and safely installing reusable AI engineering skills, rules, workflows, and templates across coding-agent runtimes (`codex`, `gemini`, `opencode`, `cursor`).

## Current Status

- **Branches:** `main` (`f81ce6e`) holds the Codex v0.1 baseline with zero published GitHub Releases or tags. Active multi-runtime hardening (`N00`–`N12`) lives on `feat/release-hardening` (branched from `feat/runtime-cursor-v0.4` `936043f`).
- **Implemented (automated-tested in Go / Docker + 3-OS CI):**
  - Core CLI & safety engine (`version`, `help`, `list`, `info`, `validate`, `doctor`, `install`, `status`, `update`, `remove`, `recover`) with `--dry-run`, `--yes`, `--json`, managed blocks, PID/token locks, transaction journals, and conditional rollback.
  - Opt-in Codex hooks (`dtkit hooks list|status|enable|disable`, `dtkit hook run session-context`).
  - Independent authoring & packaging (`dtkit create kit|skill`, `dtkit pack`, `--from <dir|archive>`, `--source <url>`, `--sha256`, `--offline`, digest cache).
  - Four runtime adapters (`codex`, `gemini`, `opencode`, `cursor`) — multi-runtime CLI, coexistence, and state separation tested in automation.
- **Runtime Verification Status (Truthfulness Boundary):**
  - **Codex CLI (`0.169.0`, `project` + `$CODEX_HOME`):** Partial — project skill/instruction discovery & isolated `$CODEX_HOME/AGENTS.md` verified in [docs/verification/codex-0.169.0-2026-11-16.md](docs/verification/codex-0.169.0-2026-11-16.md); sandboxed PS subcommands blocked by host policy; hook trust & OS-profile `~/.agents/skills` live smoke pending.
  - **OpenCode (`1.19.5`, `project` + `global`):** Partial — isolated project-only, global-only, combined 14-skill discovery alongside Codex/Gemini/Cursor, and clean removal verified in [docs/verification/opencode-1.19.5-2026-11-16.md](docs/verification/opencode-1.19.5-2026-11-16.md); live LLM turn blocked by absent isolated credentials (`0 credentials`).
  - **Gemini CLI & Cursor:** Experimental (`unverified`) — automated adapter/CLI/coexistence tests pass; `gemini` and `cursor` CLIs absent on verification host ([docs/verification/gemini-cursor-external-blocker-2026-11-16.md](docs/verification/gemini-cursor-external-blocker-2026-11-16.md)).
- **Release Readiness Status (`N00`–`N11` complete on `feat/release-hardening`):** Core hardening (`N01`–`N07`), isolated runtime discovery (`N08`), `engineer` evaluation rubric/suite (`N09`), hardened RC packaging workflow & terminal checksum instructions (`N10`), and final CLI/troubleshooting/merge prep (`N11`) are complete; remaining items are post-release diagnostics (`N12`), owner review/tag approval for publishing `0.4.0-rc.1`, and live A/B model benchmark execution under an unblocked workspace sandbox.

See [docs/compatibility.md](docs/compatibility.md) for the per-runtime 4-layer evidence matrix, [docs/cli.md](docs/cli.md) for CLI contracts, [docs/release.md](docs/release.md) for terminal installation, changelog, and troubleshooting, and [docs/progress.md](docs/progress.md) for verified task records.

## Terminal Installation (Pinned Release Binary)

Once a release tag (such as `v0.4.0-rc.1`) is published—or when building local archives via `.\scripts\package.ps1 -Version 0.4.0-rc.1`—follow the checksum-verified, user-local installation steps in [docs/release.md](docs/release.md#terminal-install--checksum-verification-pinned-version-no-admin) (never skip `SHA256SUMS` verification).

## Build and Verification (Dev Source)

Requires Go `1.27.4` or newer (or Docker Desktop if Windows Application Control / Device Guard blocks unsigned `go.exe`). No published release archive is tagged yet; build from source on `feat/release-hardening`:

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

*(Note: `go install github.com/truong-newbie/Agent_Kit_Impl/cmd/dtkit@latest` resolves `main` (`f81ce6e`, Codex v0.1 only) until multi-runtime changes are merged and tagged.)*

## Quick Start

```powershell
# Check environment and runtime readiness (--runtime defaults to codex)
dtkit doctor
dtkit doctor --runtime gemini
dtkit doctor --runtime opencode
dtkit doctor --runtime cursor

dtkit list
dtkit info engineer

# Preview then install for a runtime
dtkit install engineer --project --runtime codex --dry-run
dtkit install engineer --project --runtime codex --yes
dtkit status --project

# Install / co-exist with other runtimes
dtkit install engineer --project --runtime opencode --yes
dtkit install engineer --project --runtime gemini --yes
dtkit install engineer --project --runtime cursor --yes

# Global installation
dtkit install engineer --global --runtime codex --yes

# Opt-in Codex SessionStart hook
dtkit hooks enable session-context --project --dry-run

# Authoring, packing, updating, removing, and recovery
dtkit create kit my-kit
dtkit pack .\my-kit
dtkit update engineer --project --from .\path\to\kit --yes
dtkit remove engineer --project --runtime codex --dry-run
dtkit recover --project --dry-run
```

## Scopes, Shared Files, and Safety

- **Per-runtime layout & state isolation:**
  - **Codex (`codex`):** `<project>/AGENTS.md` / `$CODEX_HOME/AGENTS.md` (managed block), `.agents/skills/dtkit-<scope>-*/SKILL.md`, `.dtkit/content/<kit>/...`, `.dtkit/state.json`, optional `.codex/hooks.json` or `$CODEX_HOME/hooks.json`.
  - **Gemini CLI (`gemini`):** `<project>/GEMINI.md` / `$GEMINI_CLI_HOME/.gemini/GEMINI.md` (managed block), `.gemini/skills/dtkit-gemini-project-*` / `$GEMINI_CLI_HOME/.gemini/skills/dtkit-gemini-global-*`, `.dtkit/runtimes/gemini/content/<kit>/...`, `.dtkit/runtimes/gemini/state.json`.
  - **OpenCode (`opencode`):** Shared `AGENTS.md` (runtime-namespaced block `<!-- dtkit:begin <kit> opencode <scope> -->`) / `$OPENCODE_CONFIG_DIR/AGENTS.md` (or `~/.config/opencode/AGENTS.md`), `.opencode/skills/dtkit-opencode-project-*` / `<opencode-home>/skills/dtkit-opencode-global-*`, `.dtkit/runtimes/opencode/content/<kit>/...`, `.dtkit/runtimes/opencode/state.json`.
  - **Cursor (`cursor`):** Project rule `.cursor/rules/dtkit-<kit>.mdc` (`alwaysApply: true`), `.cursor/skills/dtkit-cursor-<scope>-*`, `.dtkit/runtimes/cursor/content/<kit>/...`, `.dtkit/runtimes/cursor/state.json`. Global User Rules live in Cursor Settings UI and are not managed on disk (`--global` installs skills + resources only).
- **Operational data:** Add `.dtkit/local/` to `.gitignore` so local lock files, journals, and transaction backups across all runtimes are not committed.
- **Conflict protection:** Unowned content outside DTKit managed blocks is preserved byte-for-byte (including BOM and CRLF/LF). Modified managed files, unowned targets at whole-file paths, or blocking overrides halt mutations with exit code `3`. `--yes` only skips confirmation prompts; it never overrides conflicts.

More documentation: [docs/cli.md](docs/cli.md), [docs/development.md](docs/development.md), [docs/compatibility.md](docs/compatibility.md), [docs/release.md](docs/release.md), [DTKIT-IMPLEMENTATION-PLAN.md](DTKIT-IMPLEMENTATION-PLAN.md), [DTKIT-NEXT-PHASE-PLAN.md](DTKIT-NEXT-PHASE-PLAN.md).
