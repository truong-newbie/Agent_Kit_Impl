# DTKit

DTKit is a single-binary Go CLI toolkit for packaging, validating, and safely installing reusable AI engineering skills, rules, workflows, and templates. The v0.1 target runtime is **Codex CLI**, supporting both **project** and **global** scopes.

## Status (v0.1 Release Candidate)

- **Implemented & automated-tested:** `version`, `help`, `list`, `info`, `validate`, `doctor`, `install` (fresh + idempotent no-op), `status`, `update`, `remove`, and `recover`, with `--dry-run`, `--yes`, `--json`, `--from <dir>`, managed blocks in `AGENTS.md`, exclusive locks, transaction journals, and conditional rollback.
- **Runtime verification:** Project skill & instruction discovery tested against `codex-cli 0.169.0` (see [docs/verification/codex-0.169.0-2026-11-16.md](docs/verification/codex-0.169.0-2026-11-16.md)). Global isolation smoke and unblocked Codex file-read verification remain pending.
- **Deferred beyond v0.1:** Native hooks (v0.2), remote registries / `pack` (v0.3), and Gemini CLI / OpenCode / Cursor adapters (v0.4+).

## Build and Install Locally

Requires Go `1.27.4` or newer (or Docker Desktop if Windows Application Control / Device Guard blocks unsigned `go.exe`).

```powershell
# Build Windows, Linux, and macOS binaries + release archives + SHA256SUMS
.\scripts\package.ps1 -Version 0.1.0-rc1

# Verify checksum and install dtkit.exe into a user directory on PATH
Get-FileHash .\dist\dtkit_0.1.0-rc1_windows_amd64.zip -Algorithm SHA256
Get-Content .\dist\SHA256SUMS
New-Item -ItemType Directory -Force -Path "$env:LOCALAPPDATA\dtkit\bin" | Out-Null
Expand-Archive -Path .\dist\dtkit_0.1.0-rc1_windows_amd64.zip -DestinationPath "$env:LOCALAPPDATA\dtkit\bin" -Force
$env:Path += ";$env:LOCALAPPDATA\dtkit\bin"
dtkit version --json
```

If you have an approved native Go toolchain:

```powershell
go build -trimpath -o .\dist\dtkit.exe ./cmd/dtkit
```

## Quick Start

```powershell
# Check environment and runtime readiness
dtkit doctor
dtkit list
dtkit info engineer

# Project installation (preview first, then apply)
dtkit install engineer --project --dry-run
dtkit install engineer --project --yes
dtkit status --project

# Global installation (user-level ~/.agents/skills and $CODEX_HOME/AGENTS.md)
dtkit install engineer --global --dry-run
dtkit install engineer --global --yes
dtkit status

# Update from bundled catalog or a local package directory
dtkit update engineer --project --dry-run
dtkit update engineer --project --from .\path\to\kit --yes

# Remove DTKit-managed files/blocks while keeping personal content
dtkit remove engineer --project --dry-run
dtkit remove engineer --project --yes

# Inspect or recover an interrupted transaction
dtkit recover --project --dry-run
dtkit recover --project --yes
```

In Codex, invoke project or global skills explicitly with `$dtkit-project-plan`, `$dtkit-project-implement`, `$dtkit-project-review`, `$dtkit-global-plan`, etc.

## Scopes, Shared Files, and Safety

- **Project scope** writes `<project>/AGENTS.md` (managed block `<!-- dtkit:begin engineer project -->`), `<project>/.agents/skills/dtkit-project-*/SKILL.md`, `<project>/.dtkit/content/engineer/...`, and `<project>/.dtkit/state.json`. Add `.dtkit/local/` to `.gitignore` so local transaction backups are not committed.
- **Global scope** writes `<codex-home>/AGENTS.md` (`CODEX_HOME` or `~/.codex`), `~/.agents/skills/dtkit-global-*/SKILL.md`, `~/.dtkit/content/engineer/...`, and `~/.dtkit/state.json` (with `codexHome` binding).
- **Conflict protection:** Existing `AGENTS.md` content outside the DTKit managed block is preserved byte-for-byte (including BOM and CRLF/LF). Modified managed files, unowned files at skill destinations, or non-empty `AGENTS.override.md` halt mutations with exit code `3`. `--yes` only skips interactive prompts; it never overrides conflicts.

## Verification

```powershell
# Run formatting, race tests, vet, and build via Docker
.\scripts\verify-docker.ps1
```

More documentation: [docs/cli.md](docs/cli.md), [docs/development.md](docs/development.md), [docs/compatibility.md](docs/compatibility.md), [docs/release.md](docs/release.md), [DTKIT-IMPLEMENTATION-PLAN.md](DTKIT-IMPLEMENTATION-PLAN.md).
