# DTKit CLI Reference

DTKit (`dtkit`) is a single-binary Go CLI for packaging, validating, and safely installing reusable AI agent kits across `codex` (default), `gemini`, `opencode`, `cursor`, and `claude`.

## Command Summary

```text
dtkit version [--json]
dtkit help [command]
dtkit list [--json]
dtkit info <kit>[@<ver>] [--runtime <codex|gemini|opencode|cursor|claude>] [--project|--global] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--json]
dtkit validate <dir|archive> [--runtime <codex|gemini|opencode|cursor|claude>] [--json]
dtkit validate <kit>[@<ver>] --source <url>|--offline [--sha256 <hex>] [--runtime <codex|gemini|opencode|cursor|claude>] [--json]
dtkit create kit <id> [--dir <parent>] [--name <title>] [--json]
dtkit create skill <slug> --kit-dir <dir> [--description <text>] [--json]
dtkit fork <kit>[@<ver>] [--as <new-id>] [--dir <parent>] [--version <x.y.z>] [--name <title>] [--same-id] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--redact] [--json]
dtkit pack <directory> [--out <file.tar.gz>] [--json]
dtkit doctor [--runtime <codex|gemini|opencode|cursor|claude>] [--project|--global] [--path <dir>] [--redact] [--json]
dtkit diff [<kit>[@<ver>]] --project|--global [--runtime <codex|gemini|opencode|cursor|claude>] [--preset <name>|--components <list>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--redact] [--json]
dtkit install <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor|claude>] [--preset <name>|--components <list>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--diff] [--redact] [--yes] [--json]
dtkit status [--runtime <codex|gemini|opencode|cursor|claude>] [--project|--global] [--path <dir>] [--json]
dtkit update <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor|claude>] [--preset <name>|--components <list>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--diff] [--redact] [--yes] [--json]
dtkit remove <kit> --project|--global [--runtime <codex|gemini|opencode|cursor|claude>] [--path <dir>] [--dry-run] [--yes] [--json]
dtkit recover --project|--global [--runtime <codex|gemini|opencode|cursor|claude>] [--path <dir>] [--transaction <id>] [--dry-run] [--yes] [--json]
dtkit project <inspect|init> [--path <dir>] [--dry-run] [--yes] [--json]
dtkit check [--path <dir>] [--changed|--all] [--dry-run] [--yes] [--allow-autofix] [--json]
dtkit lock <generate|verify|diff|sign|keygen> [--path <dir>] [--runtime <codex|gemini|opencode|cursor|claude>] [--lock <file>] [--trust <file>] [--source-ref <kit=ref>] [--key-file <file>] [--key-id <id>] [--offline] [--require-signature] [--dry-run] [--yes] [--redact] [--json]
dtkit sync [--path <dir>] [--runtime <codex|gemini|opencode|cursor|claude>] [--lock <file>] [--trust <file>] [--require-signature] [--offline] [--dry-run] [--diff] [--redact] [--yes] [--json]
dtkit hooks <list|status|enable|disable> [--runtime <codex|claude>] [--project|--global] [--path <dir>] [--executable <abs>] [--dry-run] [--yes] [--json]
dtkit hook run <id> --scope project|global --root <dir> [--runtime <codex|claude>]
dtkit completion <bash|zsh|powershell>
```

## Core Contracts & Semantics

- **Multi-kit per `(runtime, scope)` (`state.json` v1/v2 & v3):** Each `(runtime, scope)` pair (`codex/project`, `opencode/project`, `claude/project`, etc.) tracks one or more installed kits in its isolated `state.json`. Installing a single kit keeps the `schemaVersion` v1/v2 layout for backward compatibility; installing two or more kits in the same `(runtime, scope)` atomically promotes `state.json` to `schemaVersion: 3` (`kits` registry + shared top-level `hooks`) and demotes cleanly on `dtkit remove`. Peer kits coexist safely across shared `managed-block` instructions and skill directories (`<prefix><kit>-<slug>`, preserving `<prefix><slug>` compatibility for `engineer`), reject cross-kit `whole-file` collisions (`artifact_owned_by_<kit>`), and preserve shared runtime hooks while peer kits remain installed. All five supported runtimes also remain isolated from each other.
- **Selective install & update (`--preset` / `--components`):** Pass `--preset <name>` (e.g., `minimal`, `review-only`, `engineering` for `engineer@0.4.0`) or `--components <id1,id2,...>` on `dtkit install` or `dtkit update` (mutually exclusive). DTKit expands transitive `requires` dependencies deterministically, records `selection` in `state.json`, preserves existing selection on subsequent `dtkit update` unless overridden, and removes orphaned artifacts cleanly when switching to a smaller selection.
- **Read-only guarantees (`info`, `validate`, `doctor`, `status`, `diff`, `project inspect`, `--dry-run`, cancelled prompt):** Never write project files, user config, state, transaction backups, or persistent archive cache blobs, and never execute untrusted repository scripts. Pass `--redact` on `doctor`, `diff`, `fork`, `install --diff`, or `update --diff` (with or without `--json`) to scrub local home paths (`<HOME>`) and credential tokens before sharing diagnostics or previews.
- **Controlled customization & preview diffs (`dtkit diff`, `dtkit fork`):** `dtkit diff` (and `--diff` on `install`/`update`) emits bounded, binary-safe unified diffs (`16 KiB` per-file, `64 KiB` total) plus source/version/selector/ownership explanations without ever exposing unowned files or user text outside DTKit's managed block, and never auto-adopting direct edits to managed artifacts. `dtkit fork` exports a verified kit cleanly into `<dir>/<id>` (without host path or remote credential leakage), guarding duplicate IDs unless `--same-id` bumps the version above upstream so subsequent `dtkit update --from <dir>` cycles work cleanly.
- **Portable lockfile & trust policy (`dtkit lock`, `dtkit sync`):** `dtkit.lock` (`schemas/lock.schema.json`) pins kit IDs, versions, `packageDigest`/`archiveSha256`, portable relative/HTTPS sources, runtime targets/adapter revisions, selections, and optional Ed25519 `provenance` signatures without host paths, user tokens, or installation state. Optional `.dtkit/trust.json` defines active/revoked/expiring Ed25519 publisher keys; `dtkit lock verify` and `dtkit sync` verify digests, adapter compatibility, and provenance before any mutation, falling back cleanly to verified cache in `--offline` mode and failing closed on revoked/expired keys, missing signatures when required, or digest mismatches. SHA-256 hashes verify byte integrity and Ed25519 signatures verify publisher key ownership; neither proves skill/hook prompt safety on its own.
- **Project profile & quality checks (`dtkit project`, `dtkit check`):** `dtkit project init` generates `.dtkit/project.json` (never overwriting existing files) with strict argv-only check definitions (`schemas/project.schema.json`). `dtkit check` requires explicit trust confirmation recorded in `.dtkit/local/check-trust.json` (automatically invalidated whenever `checks` change), orders runs fast-to-slow (`format -> lint -> typecheck -> build -> test -> custom`), supports `--changed` monorepo root filtering via Git porcelain status, gates autofix commands behind `--allow-autofix`, and returns exit `1`/`4`/`130` for check failures, missing tools, or timeouts/cancellations.
- **Capabilities fallback (`requiresCapabilities` vs. `optionalCapabilities`):** Packages requiring unsupported capabilities for a `(runtime, scope)` fail pre-write with `unsupported_capability`. Optional capabilities (for example `instructions` on `cursor/global`, where global User Rules live in Cursor Settings UI) emit an explicit warning and install supported artifacts (`skills`, resources).
- **Pre-flight CLI warnings:** Missing target runtime binaries (`codex`, `gemini`, `opencode`, `cursor`, `claude`) on `PATH` emit consistent non-blocking warnings across `info`, `install`, `update`, `status`, and `doctor` (with `doctor` also flagging `runtime_executable` exit `4`).
- **Opt-in runtime hooks (`dtkit hooks`):** Opt-in only, supporting `--runtime codex` (`session-context` on `SessionStart` via `.codex/hooks.json` or `$CODEX_HOME/hooks.json`) and `--runtime claude` (`check-changed` `PostToolUse` on `Edit|Write|MultiEdit` running `dtkit check --changed` with `DTKIT_HOOK_ACTIVE=1` loop prevention and 2s debounce, plus `guard-edit` `PreToolUse` blocking `.env*`, `.dtkit/local/`, `.git/`, and traversal paths, via `.claude/settings.json` or `$CLAUDE_CONFIG_DIR/settings.json`). `check-changed` never bypasses `.dtkit/local/check-trust.json`. Pass `--executable <abs>` if registering a specific stable binary, and always run `dtkit hooks disable ...` before moving or deleting `dtkit`.
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
