# Release Readiness Checklist

> **Current Release Stream (`main`):** All Mốc A (`N00`–`N12`, `A01`–`A04`), Mốc B (`B01`–`B08`), and Mốc C (`C01`–`C06`) milestones are merged into `main`, tagged **`v0.6.0`** (creating a Draft non-prerelease GitHub Release via [release.yml](../.github/workflows/release.yml) for maintainer review/publication, while preserving truthful `partial`/`experimental` live-runtime labels in [compatibility.md](./compatibility.md)).

## Owner Decisions Already Confirmed

1. Canonical Go module path: `github.com/truong-newbie/Agent_Kit_Impl`.
2. Repository license: `MIT License` ([LICENSE](../LICENSE)).
3. Initial v0.1 release prep (`f81ce6e`): merged to `main`; public tag deferred pending full verification and explicit owner release confirmation.

## Pre-Release Gates on `feat/release-hardening` (`N00`–`N12`)

See [DTKIT-NEXT-PHASE-PLAN.md](../DTKIT-NEXT-PHASE-PLAN.md) for detailed acceptance criteria per task:

- [x] `N00` — Baseline sync, truthful README/compatibility matrix, and progress tracking.
- [x] `N01` — Bump bundled `engineer` kit version (`0.1.0` -> `0.2.0`) for multi-runtime manifest changes and test real upgrade path from `main` (`f81ce6e`).
- [x] `N02` — Enforce strict read-only no-write guarantees for remote/cached `--dry-run`, `info`, and `validate`.
- [x] `N03` — Harden remote HTTP timeouts/redirects, registry index validation, offline `minCliVersion`, and multi-process cache idempotency.
- [x] `N04` — Reproduce and verify concurrent cross-runtime `AGENTS.md` mutations, shared lock ordering, and recovery safety.
- [x] `N05` — Formalize required vs optional/fallback capability enforcement (including Cursor global) and consistent pre/post-install warnings.
- [x] `N06` — Harden hook JSON duplicate-key parsing, ambiguous-digest handling, special-character executable quoting, and timeout/leak isolation.
- [x] `N07` — Document CI skip reasons and add native Windows lock/junction and child-process crash recovery tests.
- [x] `N08` — Run full isolated runtime discovery + real task smoke checks across `codex`, `gemini`, `opencode`, and `cursor` (marking unverified integrations experimental).
- [x] `N09` — Establish Engineer Kit evaluation rubric and baseline vs with-kit task results (`docs/evaluation/`).
- [x] `N10` — Sanitize release workflow/scripts inputs, add GitHub Release publishing flow, generate next RC candidate (e.g., `0.4.0-rc.1`), and verify clean-machine terminal install.
- [x] `N11` — Final CLI/doc/changelog/troubleshooting audit and PR merge readiness to `main`.
- [x] `N12` — Issue templates, redacted `--redact` diagnostic summary support, support policy, and incident handling procedure.

## Upgrade Notes (`0.1.0` -> `0.2.0` Engineer Kit & Dev Snapshots)

- Upgrading an existing `main` (`f81ce6e`, `engineer@0.1.0`) Codex installation to `engineer@0.2.0`:
  - Run `dtkit update engineer --project --dry-run` (and/or `--global --dry-run`), then `dtkit update engineer --project --yes` (and/or `--global --yes`).
  - DTKit preserves user content outside managed blocks (including UTF-8 BOM and CRLF/LF line endings), preserves registered v2 hooks and `installedAt` timestamps, and increments `generation`.
- Replacing an unreleased dev snapshot installed at the same kit version (`immutable_version` conflict):
  - DTKit intentionally rejects silently overwriting a modified package sharing an already-recorded kit version. Never delete `.dtkit/state.json` manually; instead remove cleanly with ownership protection (`dtkit remove <kit> --project|--global --runtime <rt> --yes`) and re-run `dtkit install <kit> --project|--global --runtime <rt> --yes`.

## Terminal Install & Checksum Verification (Pinned Version, No Admin)

*(Applies once a tagged GitHub Release such as `v0.4.0-rc.1` is published, or against a local/HTTP artifact server built via `.\scripts\package.ps1 -Version 0.4.0-rc.1`. Use the fail-fast installer scripts [scripts/install.sh](../scripts/install.sh) and [scripts/install.ps1](../scripts/install.ps1), which share the verified logic below.)*

### Linux / macOS (`bash`, real OS/arch detection, user-local `~/.local/bin`)

```bash
bash scripts/install.sh --version 0.4.0-rc.1 --bin-dir "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"
dtkit version --json
dtkit doctor
dtkit install engineer --project --dry-run
```

Equivalent standalone fail-fast steps (stops immediately on unsupported OS/arch, missing/duplicate `SHA256SUMS` entry, checksum mismatch, or extraction failure without overwriting an existing binary):

```bash
VERSION="0.4.0-rc.1"
case "$(uname -s):$(uname -m)" in
  Linux:x86_64|Linux:amd64) ARCHIVE="dtkit_${VERSION}_linux_amd64.tar.gz" ;;
  Darwin:arm64|Darwin:aarch64) ARCHIVE="dtkit_${VERSION}_darwin_arm64.tar.gz" ;;
  *) echo "Unsupported target $(uname -s)/$(uname -m); supported Unix archives: linux/amd64, darwin/arm64 (build from source otherwise)." >&2; exit 1 ;;
esac
BASE_URL="https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v${VERSION}"
SHA_TOOL="$(command -v sha256sum || command -v shasum)"
[ -n "$SHA_TOOL" ] || { echo "sha256sum or shasum required" >&2; exit 1; }

TMP=$(mktemp -d "${TMPDIR:-/tmp}/dtkit-install.XXXXXX")
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "$BASE_URL/$ARCHIVE" -o "$TMP/$ARCHIVE"
curl -fsSL "$BASE_URL/SHA256SUMS" -o "$TMP/SHA256SUMS"
[ "$(grep -Ec "^[0-9a-fA-F]{64}[[:space:]]+(\./)?${ARCHIVE}$" "$TMP/SHA256SUMS" || true)" -eq 1 ] || exit 1
EXPECTED=$(grep -E "^[0-9a-fA-F]{64}[[:space:]]+(\./)?${ARCHIVE}$" "$TMP/SHA256SUMS" | awk '{print tolower($1)}')
if [ "${SHA_TOOL##*/}" = "sha256sum" ]; then
  ACTUAL=$(sha256sum "$TMP/$ARCHIVE" | awk '{print tolower($1)}')
else
  ACTUAL=$(shasum -a 256 "$TMP/$ARCHIVE" | awk '{print tolower($1)}')
fi
[ "$ACTUAL" = "$EXPECTED" ] || { echo "SHA256 mismatch for $ARCHIVE" >&2; exit 1; }

mkdir -p "$TMP/extracted" "$HOME/.local/bin"
tar -xzf "$TMP/$ARCHIVE" -C "$TMP/extracted"
"$TMP/extracted/dtkit" version --json | grep -q "\"version\":\"${VERSION}\""
mv -f "$TMP/extracted/dtkit" "$HOME/.local/bin/dtkit"
```

### Windows PowerShell (`%LOCALAPPDATA%\Programs\dtkit\bin`, X64 verified)

```powershell
.\scripts\install.ps1 -Version "0.4.0-rc.1"
dtkit version --json
dtkit doctor
dtkit install engineer --project --dry-run
```

[scripts/install.ps1](../scripts/install.ps1) checks `OSArchitecture -in @("X64","amd64","AMD64")`, downloads to a unique GUID temp dir, enforces an exact-1 `SHA256SUMS` match before extracting, validates `dtkit.exe version --json`, swaps via `.bak` rollback so locked/failed upgrades preserve the previous working `dtkit.exe`, and cleans temp files in `finally`.

- **Fail-closed checksum rule:** Never skip SHA256 verification, fall back across checksum tools on a mismatch, or bypass OS security policies (`Device Guard` / Gatekeeper).
- **Upgrading CLI while hooks are active:** Replacing the same stable `dtkit` binary path in place keeps registered hooks valid (`configured_pending_trust`). Never move or delete the `dtkit` binary while active hooks reference it without running `dtkit hooks disable` first.
- **Uninstalling a kit vs. uninstalling the `dtkit` binary:**
  1. Remove installed kit artifacts safely: `dtkit remove engineer --project --yes` (and `--global` / `--runtime <rt>` where installed).
  2. If Codex hooks were enabled, disable them **before** deleting or moving the `dtkit` binary: `dtkit hooks disable session-context --project --yes` (or `--global`).
  3. Only then delete `~/.local/bin/dtkit` or `%LOCALAPPDATA%\Programs\dtkit\bin\dtkit.exe`.

## Changelog (`0.6.0` Release vs. `v0.5.0-rc.1` / `0.4.0-rc.1`)

- **Multi-Kit State Registry v3 & Shell Completion (`C01`, `C04a`):** Added backward-compatible `schemaVersion: 3` multi-kit state registry, kit-namespaced skill directories (`<prefix><kitID>-<slug>`), cross-kit ownership collision enforcement, transactional rollback, and `dtkit completion <bash|zsh|powershell>`.
- **Preview Diff, Explain & Controlled Kit Fork (`C02`):** Added `dtkit diff`, `dtkit fork`, `--diff`, and `--redact` with bounded/redacted unified diffs isolated to DTKit-managed blocks.
- **Portable `dtkit.lock`, Reproducible Sync & Ed25519 Trust (`C03`):** Added `schemas/lock.schema.json`, `dtkit lock <generate|verify|diff|sign|keygen>`, `dtkit sync [--offline] [--require-signature]`, and stdlib Ed25519 key rotation/revocation/expiration verification before install mutations.
- **Portable Team Onboarding & Multi-Runtime Inspect (`C04`):** Added `dtkit team <export|import|template>`, secret-free team bundles (`schemas/team.schema.json`), and unified `dtkit status --all-runtimes` across all 5 runtimes.
- **Multi-Runtime Quality Hooks & Plugins (`C05`):** Added opt-in quality hooks/plugins for Gemini CLI (`.gemini/settings.json` `AfterTool`/`BeforeTool`), Cursor (`.cursor/hooks.json` `afterFileEdit`), and OpenCode (`.opencode/plugins/dtkit-quality.js` `tool.execute.after`) alongside Codex and Claude Code.
- **Periodic Compatibility Checks & Perf Budgets (`C06`):** Added `dtkit compat <snapshot|diff>`, committed baseline [last-known-good-compat.json](./verification/last-known-good-compat.json), weekly [compat-periodic.yml](../.github/workflows/compat-periodic.yml), [compatibility-policy.md](./compatibility-policy.md), and cohort-isolated `RegressionBudget` evaluation (`internal/eval/perf.go`).
- **Release Gate Honesty:** Maintainer approved cutting **`v0.6.0`** (Draft GitHub Release, non-prerelease) while preserving `partial` / `experimental` (`unverified` live) support boundaries in [compatibility.md](./compatibility.md) until live API credentials (`claude`, `opencode`, `codex` sandbox) and `gemini`/`cursor` CLIs are supplied.

## Changelog (`0.4.0-rc.1` Candidate vs. `v0.1` `f81ce6e` Baseline)

- **Multi-Runtime Architecture (`R1`–`R3`, `N05`):** Added `gemini`, `opencode`, and `cursor` adapters alongside `codex`, strict `requiresCapabilities` / `optionalCapabilities` contract, `integrationStatus` separation (`verified` / `partial` / `unverified`), consistent runtime binary warnings, and shared `AGENTS.md` multi-block namespacing.
- **Opt-In Codex Hooks (`H1`, `N06`):** Added `dtkit hooks list|status|enable|disable` and `dtkit hook run session-context` with strict duplicate-key/size/schema validation, `hook_entry_ambiguous` conflict protection, POSIX/PowerShell quoting execution guarantees, non-ephemeral executable checks, and zero-leak stdin timeout/cancel isolation.
- **Authoring, Packaging & Remote Policy (`D1`, `N02`–`N03`):** Added `dtkit create kit|skill`, `dtkit pack`, `--from`, `--source`, `--sha256`, `--offline`, strict JSON & registry index parsing, `minCliVersion` compatibility checks (including `<sha>.meta.json` offline enforcement), HTTPS/loopback redirect hardening, URL credential scrubbing, and zero-write `dry-run` / `info` / `validate` guarantees.
- **Concurrency, Upgrade Compatibility & Native Recovery (`N01`, `N04`, `N07`):** Bumped bundled `engineer` kit `0.1.0` -> `0.2.0` (`state` v1/v2 compatibility verified via `f81ce6e -> HEAD` upgrade smoke), added sorted canonical shared-target locking across concurrent runtimes, and verified Windows NTFS junctions, exclusive file locks, POSIX `0555` parent confinement, bounded 7-target fuzzing, and 4-boundary OS subprocess `Process.Kill()` crash recovery.
- **Release Hardening & Evaluation (`N08`–`N11`):** Verified isolated Codex/OpenCode layout discovery, created `engineer` 10-task benchmark & structural evaluation tests (`docs/evaluation/`), hardened `.github/workflows/release.yml` + `scripts/*.ps1` with 3-OS native archive smoke, and synced CLI/troubleshooting docs.

## Troubleshooting Guide

- **Modified kit drift (`target_modified`) or immutable-version rejection (`version_already_installed`):**
  - *Do not* delete `.dtkit/state.json`. Preview first: `dtkit status --project --runtime <rt>` and `dtkit remove engineer --project --runtime <rt> --dry-run`. If intentional local changes must be kept, copy them aside, run `dtkit remove ... --yes`, then `dtkit install ... --yes`.
- **Interrupted transaction (`recoveryNeeded`) or stale/active `install.lock` (exit code `3`):**
  - Check state safely with `dtkit recover --project --runtime <rt> --dry-run`. If the recorded PID is dead and no other `dtkit` instance is running, run `dtkit recover --project --runtime <rt> --yes`. External user edits made after a crash trigger conflict protection instead of silent overwrite.
- **`broken_executable` or drifted hook after moving/upgrading `dtkit`:**
  - Inspect via `dtkit hooks status --project`. Disable cleanly first (`dtkit hooks disable session-context --project --yes`), then re-enable with the new stable path (`dtkit hooks enable session-context --project --yes`). If `.codex/hooks.json` contains duplicate DTKit entries (`hook_entry_ambiguous`), remove duplicate entries manually before disabling.
- **Corrupt archive cache, checksum mismatch, or missing `--offline` artifact (exit code `3`):**
  - Do not disable `--sha256` checks or overwrite cache blobs manually. Re-fetch online with `--source <url> --sha256 <expected>` (matching the published `SHA256SUMS`), or verify connectivity/CA trust for HTTPS registries.
- **`profileMismatch` (`CodexHome` / `runtimeHome` mismatch) or Cursor global instructions fallback:**
  - Verify `CODEX_HOME`, `GEMINI_CLI_HOME`, or `OPENCODE_CONFIG_DIR` / `XDG_CONFIG_HOME` matches the directory bound when installing (`dtkit doctor --global --runtime <rt>`). For Cursor `--global`, instructions remain managed in Cursor Settings UI while skills/resources install to `~/.cursor/skills/`.

## PR Merge Readiness (`feat/release-hardening` -> `main`) & Candidate Evidence (`A01`–`A04`)

- **Proposed candidate version:** `0.4.0-rc.1` (untagged until owner approves merge/tag; remote tags count = `0`, GitHub Releases = `0`, `main` & default branch `feat/dtkit-foundation` sit at `f81ce6e`).
- **Automated lifecycle & hardening (`N01`–`N07`, `N10`–`N12`, `A02`):** Verified on GitHub Actions (`windows-latest`, `ubuntu-latest`, `macos-latest`, `race`, `fuzz-smoke`) — commit `c100b86` ([run 43912929680](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/43912929680)), commit `681529d` ([run 44002558059](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44002558059)); upgrade fixture `main` (`f81ce6e`) -> HEAD verified (`N01`).
- **Support boundary & known issues in this RC:**
  - `codex` (`project` + isolated `$CODEX_HOME/AGENTS.md`): **`partial`** (`codex exec` child PowerShell commands blocked by host Windows sandbox; Codex `global` layout kept per [A03 decision](verification/codex-global-windows-decision-2026-11-17.md); `.codex/hooks.json` `SessionStart` is configured & shell-tested, **live Codex session hook invocation unverified**).
  - `opencode` (`project` + `global`): **`partial`** (14-skill discovery & coexistence verified on `1.19.5`; live LLM turn blocked by `0 credentials` in isolated test profile).
  - `gemini` & `cursor`: **`experimental / unverified`** (CLI binaries absent on verification host).
  - `engineer 0.2.0` evaluation (`N09`): structural/disclosure tests and 10-task harness exist ([docs/evaluation/](evaluation/README.md)), while quantitative 60-run A/B benchmark remains `pending` unblocked tool execution & model budget.
- **Release workflow ([release.yml](../.github/workflows/release.yml)):** Workflow syntax, strict semver input handling, local `0.4.0-rc.1` packaging (`windows_amd64`, `linux_amd64`, `darwin_arm64` + `SHA256SUMS`), and Linux Docker archive smoke are verified; **remote 3-OS GitHub Actions `native-smoke` + `draft-release` run executes upon maintainer dispatch or `v0.4.0-rc.1` tag push**.
- **Merge readiness & owner decisions needed:** `git merge-tree --write-tree origin/main HEAD` exits `0` with zero conflicts. Owner approval is needed to: (1) merge `feat/release-hardening` into `main`, (2) dispatch `release.yml` or push `v0.4.0-rc.1` tag for public prerelease assets + checksums, and (3) approve unblocked sandbox/credentials for full live runtime & A/B benchmarks (`Mốc B` v0.5).
