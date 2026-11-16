# Release Readiness Checklist

> **Current Repository Split:** `main` (`f81ce6e`) contains the v0.1 Codex baseline with **0 published tags / 0 GitHub Releases**. The complete multi-runtime codebase (`codex`, `gemini`, `opencode`, `cursor`), hooks, distribution commands, and release-hardening backlog (`N00`–`N12`) live on branch **`feat/release-hardening`** (branched from `feat/runtime-cursor-v0.4` `936043f`).

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

*(Applies once a tagged GitHub Release such as `v0.4.0-rc.1` is published, or after building archives locally with `.\scripts\package.ps1 -Version 0.4.0-rc.1`.)*

### Linux / macOS (`bash` / `zsh`, user-local `~/.local/bin`)

```bash
VERSION="0.4.0-rc.1"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"   # linux | darwin
ARCH="amd64"
[ "$OS" = "darwin" ] && ARCH="arm64"
ARCHIVE="dtkit_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v${VERSION}"

TMP=$(mktemp -d)
curl -fsSL "$BASE_URL/$ARCHIVE" -o "$TMP/$ARCHIVE"
curl -fsSL "$BASE_URL/SHA256SUMS" -o "$TMP/SHA256SUMS"
(cd "$TMP" && (sha256sum -c <(grep " ${ARCHIVE}\$" SHA256SUMS) || shasum -a 256 -c <(grep " ${ARCHIVE}\$" SHA256SUMS)))

mkdir -p "$HOME/.local/bin"
tar -xzf "$TMP/$ARCHIVE" -C "$TMP"
install -m 0755 "$TMP/dtkit" "$HOME/.local/bin/dtkit"
rm -rf "$TMP"

export PATH="$HOME/.local/bin:$PATH"
dtkit version --json
dtkit doctor
dtkit install engineer --project --dry-run
```

### Windows PowerShell (`%LOCALAPPDATA%\Programs\dtkit\bin`)

```powershell
$Version = "0.4.0-rc.1"
$Archive = "dtkit_${Version}_windows_amd64.zip"
$BaseUrl = "https://github.com/truong-newbie/Agent_Kit_Impl/releases/download/v$Version"
$Tmp = Join-Path $env:TEMP "dtkit-install-$Version"
New-Item -ItemType Directory -Force -Path $Tmp | Out-Null

Invoke-WebRequest "$BaseUrl/$Archive" -OutFile (Join-Path $Tmp $Archive)
Invoke-WebRequest "$BaseUrl/SHA256SUMS" -OutFile (Join-Path $Tmp "SHA256SUMS")

$Actual = (Get-FileHash (Join-Path $Tmp $Archive) -Algorithm SHA256).Hash.ToLowerInvariant()
$ExpectedLine = (Get-Content (Join-Path $Tmp "SHA256SUMS") | Where-Object { $_ -match "\s+$Archive$" })
if (-not $ExpectedLine -or ($ExpectedLine.Split(" ", [System.StringSplitOptions]::RemoveEmptyEntries)[0].ToLowerInvariant() -ne $Actual)) {
    throw "SHA256 checksum mismatch for $Archive; aborting installation."
}

$BinDir = Join-Path $env:LOCALAPPDATA "Programs\dtkit\bin"
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
Expand-Archive -Path (Join-Path $Tmp $Archive) -DestinationPath $Tmp -Force
Copy-Item (Join-Path $Tmp "dtkit.exe") (Join-Path $BinDir "dtkit.exe") -Force
Remove-Item -Recurse -Force $Tmp

# Add $BinDir to user PATH (without admin):
[Environment]::SetEnvironmentVariable("Path", "$BinDir;" + [Environment]::GetEnvironmentVariable("Path", "User"), "User")
$env:Path = "$BinDir;$env:Path"
dtkit version --json
dtkit doctor
dtkit install engineer --project --dry-run
```

- **Fail-closed checksum rule:** Never skip SHA256 verification or bypass OS security policies (`Device Guard` / Gatekeeper) to run an unverified binary.
- **Uninstalling a kit vs. uninstalling the `dtkit` binary:**
  1. Remove installed kit artifacts safely: `dtkit remove engineer --project --yes` (and `--global` / `--runtime <rt>` where installed).
  2. If Codex hooks were enabled, disable them **before** deleting or moving the `dtkit` binary: `dtkit hooks disable session-context --project --yes` (or `--global`).
  3. Only then delete `~/.local/bin/dtkit` or `%LOCALAPPDATA%\Programs\dtkit\bin\dtkit.exe`.

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

## PR Merge Readiness (`feat/release-hardening` -> `main`)

- `git merge-tree --write-tree origin/main HEAD` confirmed zero merge conflicts cleanly against `origin/main` (`f81ce6e`).
- **Owner gates remaining before merge/tag/publish:** Approve PR merge into `main`, approve creating release tag (candidate default `0.4.0-rc.1`), and (if desired before declaring runtimes `stable`) run live A/B `T01`–`T10` LLM tasks cleanly in an unblocked tool sandbox (`docs/evaluation/README.md`).
