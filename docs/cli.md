# DTKit CLI Reference

DTKit (`dtkit`) is a single-binary Go CLI for packaging, validating, and safely installing reusable AI agent kits across `codex` (default), `gemini`, `opencode`, and `cursor`.

## Command Summary

```text
dtkit version [--json]
dtkit help [command]
dtkit list [--json]
dtkit info <kit>[@<ver>] [--runtime <codex|gemini|opencode|cursor>] [--project|--global] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--json]
dtkit validate <dir|archive> [--runtime <codex|gemini|opencode|cursor>] [--json]
dtkit validate <kit>[@<ver>] --source <url>|--offline [--sha256 <hex>] [--runtime <codex|gemini|opencode|cursor>] [--json]
dtkit create kit <id> [--dir <parent>] [--name <title>] [--json]
dtkit create skill <slug> --kit-dir <dir> [--description <text>] [--json]
dtkit pack <directory> [--out <file.tar.gz>] [--json]
dtkit doctor [--runtime <codex|gemini|opencode|cursor>] [--project|--global] [--path <dir>] [--redact] [--json]
dtkit install <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]
dtkit status [--runtime <codex|gemini|opencode|cursor>] [--project|--global] [--path <dir>] [--json]
dtkit update <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]
dtkit remove <kit> --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--dry-run] [--yes] [--json]
dtkit recover --project|--global [--runtime <codex|gemini|opencode|cursor>] [--path <dir>] [--transaction <id>] [--dry-run] [--yes] [--json]
dtkit hooks <list|status|enable|disable> [--project|--global] [--path <dir>] [--executable <abs>] [--dry-run] [--yes] [--json]
dtkit hook run <id> --scope project|global --root <dir>
```

## Core Contracts & Semantics

- **One kit per `(runtime, scope)`:** Each runtime and scope pair (`codex/project`, `codex/global`, `opencode/project`, etc.) tracks one active installed kit in its isolated `state.json`. Different runtimes can coexist in the same workspace or home directory without clobbering each other's state.
- **Read-only guarantees (`info`, `validate`, `doctor`, `status`, `--dry-run`, cancelled prompt):** Never write project files, user config, state, transaction backups, or persistent archive cache blobs. Pass `dtkit doctor --redact` (optionally with `--json`) to scrub local home paths (`<HOME>`) and credential tokens before sharing diagnostics.
- **Capabilities fallback (`requiresCapabilities` vs. `optionalCapabilities`):** Packages requiring unsupported capabilities for a `(runtime, scope)` fail pre-write with `unsupported_capability`. Optional capabilities (for example `instructions` on `cursor/global`, where global User Rules live in Cursor Settings UI) emit an explicit warning and install supported artifacts (`skills`, resources).
- **Pre-flight CLI warnings:** Missing target runtime binaries (`codex`, `gemini`, `opencode`, `cursor`) on `PATH` emit consistent non-blocking warnings across `info`, `install`, `update`, `status`, and `doctor` (with `doctor` also flagging `runtime_executable` exit `4`).
- **Codex hooks (`dtkit hooks`):** Opt-in only and explicitly scoped to Codex (`SessionStart` via `.codex/hooks.json` or `$CODEX_HOME/hooks.json`). Project and global `SessionStart` hooks cannot be enabled simultaneously (`hook_scope_conflict`). Pass `--executable <abs>` if registering a specific stable non-ephemeral binary, and always run `dtkit hooks disable ...` before moving or deleting the `dtkit` binary.
- **Kit upgrades vs. binary upgrades:** `dtkit update` updates the **installed kit package** using DTKit's atomic transaction engine while preserving user content outside managed blocks. Upgrading the **`dtkit` CLI binary** follows checksum-verified terminal replacement (see [docs/release.md](release.md)).

## Exit Codes

| Code | Constant | Meaning |
|---|---|---|
| `0` | `ExitOK` | Success (or clean no-op recovery) |
| `1` | `ExitInternal` | Unexpected internal or I/O error |
| `2` | `ExitUsage` | Invalid CLI usage, bad flags, or validation error |
| `3` | `ExitConflict` | Ownership conflict, corrupted/incompatible state, unrecovered transaction, hook ambiguity, or blocked/unverified remote policy |
| `4` | `ExitRuntime` | Target runtime readiness error (`doctor` runtime missing on `PATH`) |
| `130` | `ExitCancelled` | Cancelled interactively (`[y/N]`), timeout, or `SIGINT` |

## Remote Source, Compatibility & Integrity Policy

- **Supported remote schemes:** `--source` requires HTTPS (including private/internal registries trusted by system/custom CA roots); plain HTTP is allowed only for loopback hosts (`localhost`, `127.0.0.1`, `::1`) for local tests/registries.
- **Redirects & credentials:** Bounded to 30s request timeout and 10 redirects, blocking HTTPS->HTTP downgrades, stripping `Authorization`/`Cookie` headers on cross-host redirects, rejecting embedded URL `userinfo`, and redacting credentials and query tokens from errors.
- **Registry index & `minCliVersion`:** Indexes reject duplicate JSON keys, invalid entries, and duplicate `id@version` pairs, selecting the highest valid semver deterministically. Dev/prerelease CLI builds (`X.Y.Z-dev`, `X.Y.Z-rc.N`) satisfy `minCliVersion <= X.Y.Z`. Verified `minCliVersion` requirements are cached in sidecar `<sha>.meta.json` and enforced equally across online, direct archive, `--from`, cached, and `--offline` paths.
- **Archive integrity (`--sha256`):** SHA-256 checksums verify downloaded byte integrity against expected digests and do **not** provide cryptographic publisher authenticity or provenance signatures.
