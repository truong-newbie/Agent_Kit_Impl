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
- [ ] `N11` — Final CLI/doc/changelog/troubleshooting audit and PR merge readiness to `main`.
- [ ] `N12` — Issue templates, redacted `--redact` diagnostic summary support, support policy, and incident handling procedure.

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
