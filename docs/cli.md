# DTKit CLI Reference (v0.1)

## Commands

- `dtkit version [--json]` — Print CLI version, commit, and build date.
- `dtkit help [command]` — Show general or command-specific usage.
- `dtkit list [--json]` — List kits in the bundled catalog.
- `dtkit info <kit> [--from <dir|archive>] [--json]` — Show kit metadata, components, and deterministic digest.
- `dtkit validate <dir|archive> [--runtime <codex|gemini|opencode>] [--json]` — Strictly validate a local kit directory or `.tar.gz` archive (schema, paths, DAG, and runtime rendering for declared targets/scopes).
- `dtkit create kit <id> [--dir <parent>] [--name <title>] [--json]` — Scaffold a new standalone kit package passing `dtkit validate`.
- `dtkit create skill <slug> --kit-dir <dir> [--description <text>] [--json]` — Add a templated skill to an existing kit package and re-validate.
- `dtkit pack <dir> [--out <file.tar.gz>] [--json]` — Build a deterministic `.tar.gz` package archive and `.sha256` checksum file.
- `dtkit doctor [--runtime <codex|gemini|opencode>] [--project|--global] [--path <dir>] [--json]` — Read-only environment, root, runtime override (`AGENTS.override.md` / `.gemini/settings.json` / `opencode.json`), `.gitignore`, and state checks (never accesses network).
- `dtkit install <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]` — Fresh install or idempotent no-op from bundled catalog, local dir/archive, HTTPS source, or offline digest cache.
- `dtkit status [--runtime <codex|gemini|opencode>] [--project|--global] [--path <dir>] [--json]` — Inspect installed state and report drift (`healthy`, `modified`, `missing`, `conflicted`, `recovery_required`, `profile_mismatch`, `not_installed`).
- `dtkit update <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]` — Upgrade kit version (semver increasing) or migrate adapter revision while preserving user content outside managed blocks.
- `dtkit remove <kit> --project|--global [--runtime <codex|gemini|opencode>] [--path <dir>] [--dry-run] [--yes] [--json]` — Remove DTKit-owned files and managed blocks based on `state.json`.
- `dtkit recover --project|--global [--runtime <codex|gemini|opencode>] [--path <dir>] [--transaction <id>] [--dry-run] [--yes] [--json]` — Inspect and resolve interrupted transactions or stale dead-PID locks.
- `dtkit hooks <list|status|enable|disable> [options]` — Manage opt-in `session-context` (`SessionStart`) Codex hooks (schema v2, structural JSON merge in `.codex/hooks.json` or `$CODEX_HOME/hooks.json`, cross-scope duplicate check, and `configured_pending_trust` guidance).
- `dtkit hook run <id> --scope project|global --root <dir>` — Native runtime entrypoint for Codex hooks (reads JSON stdin <= 1 MiB, writes `hookSpecificOutput` JSON <= 2 KiB within 3s timeout, read-only).

## Exit Codes

| Code | Meaning |
|---|---|
| `0` | Success / no-op |
| `1` | Internal or I/O error |
| `2` | Usage error, validation error, or missing `--yes` in non-interactive mode |
| `3` | Conflict, drift, profile mismatch, lock held, or recovery required |
| `4` | Runtime capability missing (`codex` not found on `PATH` in `doctor`) |
| `130` | Cancelled by user or interrupted context |

## JSON Output

Passing `--json` emits a single JSON object (`schemaVersion: 1`) to `stdout` on both success and error. Diagnostics go to `stderr`. Mutations with `--json` require `--yes` (except `--dry-run`).
