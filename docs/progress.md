# DTKit Implementation Progress

## Current Status

- **Active Branch:** `feat/release-hardening` (branched from `feat/runtime-cursor-v0.4` `936043f`; `main` currently sits at v0.1 `f81ce6e` with zero tags and zero published GitHub Releases).
- **Latest Code CI Baseline:** SHA `be9e0bd` passed all 4 GitHub Actions jobs (`windows-latest`, `ubuntu-latest`, `macos-latest`, `race`) in [Run 43849330462](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/43849330462). (`936043f` added documentation planning commits on top.)
- **Milestone Context:** P0–P6 (v0.1 Codex), H1 (v0.2 Codex Hooks), D1 (v0.3 Authoring/Archives/Cache/Remote), and R1–R3 (v0.4 Gemini CLI, OpenCode, Cursor) are implemented as dev code and covered by Go automated tests. Remaining stabilization, real-runtime verification, and release-candidate gates are tracked in [DTKIT-NEXT-PHASE-PLAN.md](../DTKIT-NEXT-PHASE-PLAN.md).

### Hardening & Release Backlog Tracker (`N00`–`N12`)

| Task | Status | Branch / Lane | Finding(s) | Evidence / Notes |
|---|---|---|---|---|
| `N00` — Baseline sync & status tracking | `verified` | `feat/release-hardening` | Baseline sync | Recorded `936043f` / run `43849330462`, separated historical snapshots, aligned `README.md`, `compatibility.md`, `release.md` |
| `N01` — Kit version bump & upgrade from `main` | `verified` | `feat/release-hardening` | F01 | Bumped `engineer` `0.1.0`->`0.2.0` + CLI `0.4.0-dev`; tested real `main` `f81ce6e`->HEAD upgrade (`verify-docker.ps1` + live binary smoke) |
| `N02` — Source resolution read-only contract | `verified` | `feat/release-hardening` | F02, F05 | Separated `CacheReadOnly`/`CachePersist`; in-memory `FetchRemoteBytes`; no preview/offline cache writes or `.part` cleanup (`verify-docker.ps1`) |
| `N03` — Remote transport, index strictness, cache safety | `verified` | `feat/release-hardening` | F03, F05, F06 | HTTP timeout/redirect/redaction, strict index, offline `minCliVersion` meta, concurrent cache `Put`, flood cap + fuzz (`verify-docker.ps1`) |
| `N04` — Cross-runtime concurrency & shared `AGENTS.md` | `verified` | `feat/release-hardening` | F04 | Shared target locks (`.dtkit/locks`), atomic recover-under-lock, strict full-state check, barrier & multi-process tests (`verify-docker.ps1`) |
| `N05` — Capability contract & status consistency | `verified` | `feat/release-hardening` | F07 | Added `optionalCapabilities`, pre-write `requiresCapabilities` enforcement, consistent warnings across commands, `integrationStatus` separation (`verify-docker.ps1`) |
| `N06` — Hook JSON strictness & executable lifecycle | `not-started` | `feat/release-hardening` | F12 | Duplicate-key rejection, ambiguous digest guard, special-char quoting, timeout/leak checks |
| `N07` — Native OS edge cases & crash subprocess tests | `not-started` | `feat/release-hardening` | F11 | CI skip inventory, Windows locks/junctions, child-process crash recovery |
| `N08` — Real runtime verification (`codex`, `gemini`, `opencode`, `cursor`) | `not-started` | `feat/release-hardening` | F08 | Discover/task verification per runtime; unverified integrations stay experimental |
| `N09` — Engineer Kit quality baseline evaluation | `not-started` | `feat/release-hardening` | F08 | 8–12 task rubric (`docs/evaluation/`), baseline vs with-kit verification |
| `N10` — Release pipeline & terminal install UX | `not-started` | `feat/release-hardening` | F09, F10 | Sanitize workflow/PS inputs, add GitHub Release draft step, new RC candidate & terminal verify |
| `N11` — Final docs, CLI UX & `main` merge prep | `not-started` | `feat/release-hardening` | F09 | Final audit across docs/changelog/troubleshooting and PR diff review to `main` |
| `N12` — Post-release diagnostics & support policy | `not-started` | `feat/release-hardening` | — | Issue templates, read-only/redacted diagnostics (`--redact`), support & incident guide |

*(Post-release backlog `N13`–`N18` stays out of scope for this candidate wave.)*

---

Task: N00 — Đồng bộ baseline và hồ sơ tiến độ
Status: verified
Baseline/branch/SHA: feat/release-hardening branched from feat/runtime-cursor-v0.4 @ 936043f (code CI baseline be9e0bd @ run 43849330462); main @ f81ce6e
Observed problem and reproduction: README.md still described only v0.1 Codex with hooks/distribution/multi-runtime deferred; docs/compatibility.md was out of sync in working tree; docs/progress.md lacked a top Current Status summary distinguishing historical point-in-time logs from N00–N12 hardening gates; docs/release.md reflected only v0.1 prep. Also confirmed zero tags/releases exist on origin (`git tag --list` empty, GitHub releases API Count=0).
Changed files and behavior: Restored docs/compatibility.md from 936043f and rewrote it with a 4-layer evidence matrix + Feature×Runtime table separating Rendered/CLI/Discovery/Live Task status; updated README.md with accurate implemented vs partial/experimental/unverified status and four-runtime documentation; added Current Status, N00–N12 tracker table, and Historical Snapshots boundary in docs/progress.md; updated docs/release.md to point to feat/release-hardening N00–N12 gates.
Tests actually run: Docs-only check per N00 criteria (`git diff --check`, Markdown link existence check across all relative links in updated docs).
Remaining limitation / external blocker: N01–N12 engineering hardening and real runtime checks remain open before release.
Acceptance checklist result: Pass (consistent status across README, progress, compatibility, release; branch created from full four-runtime baseline 936043f; CI URL and baseline SHA recorded).
Next task: N01 — Version kit, upgrade từ bản cũ và compatibility state.

---

Task: N01 — Version kit, upgrade từ bản cũ và compatibility state
Status: verified
Baseline/branch/SHA: feat/release-hardening @ 2efef15 (upgrade baseline tested against main @ f81ce6e)
Observed problem and reproduction: Between main (`f81ce6e`, `engineer@0.1.0`, digest `88bc2c800d3b610b932b65a8b551e9d3686506d17e7a9c6056a75042da42245b`) and `feat/runtime-cursor-v0.4` (`936043f`), `internal/catalog/engineer/package.json` added `gemini`, `opencode`, `cursor` to `runtimeTargets` without bumping `"version": "0.1.0"`. Upgrading an existing `main` installation hit `immutable_version` (`0.1.0` digest mismatch). Additionally, downgrade/immutable-version errors still carried stale `in v0.1` copy and needed explicit schema/revision error messages plus golden v1/v2 state & journal fixtures across all 4 runtimes.
Changed files and behavior:
- `internal/catalog/engineer/package.json`: bumped version `0.1.0` -> `0.2.0` (digest `db0b5aaf2d0e22b6278a09a91b28b63ad57d794980f3395d02fe3feeea8ae03c`).
- `internal/buildinfo/buildinfo.go`: bumped dev version `"0.1.0-dev"` -> `"0.4.0-dev"`.
- `internal/catalog/catalog.go`, `internal/state/state.go`: clarified explicit `schemaVersion` error messages (supported manifest v1; state v1/v2).
- `internal/installer/update.go`: removed stale `in v0.1` text on downgrade errors and added actionable `apperr.WithHint` on `immutable_version` (bump kit version or clean `remove`+`install` for dev snapshots).
- `internal/state/compat_test.go`, `internal/installer/compat_test.go`: added golden state v1/v2 (`codex` + hooks, `gemini`, `opencode`, `cursor`), unknown `schemaVersion` / future `adapterRevision`, legacy `v1` journal validation, real `main` (`0.1.0`) -> `0.2.0` upgrade preserving UTF-8 BOM/CRLF user `AGENTS.md`, `InstalledAt`, hooks, idempotent re-runs, and `immutable_version` enforcement tests.
- `internal/cli/{cli_test,e2e_test}.go`, `internal/installer/runtime_test.go`: updated custom update test fixtures to `0.3.0`.
- `docs/release.md`: checked off `N01` and documented upgrade notes for `0.1.0` -> `0.2.0` and dev snapshots.
Tests actually run:
- `docker run --rm -v "${PWD}:/src" -w /src golang:1.27.4 /usr/local/go/bin/gofmt -w cmd internal schemas; .\scripts\verify-docker.ps1` -> exit `0`, all Go packages passed under `go test -race ./...`, `go vet ./...`, and `go build`.
- Live binary upgrade smoke inside Docker (`golang:1.27.4` with detached worktree of `main` `f81ce6e` -> current working tree): `dtkit-old install engineer --project|--global --yes --json` (`0.1.0`) -> `dtkit-new update engineer --project|--global --yes --json` (`0.2.0`, generation `2`, 14 files updated, BOM+CRLF user header preserved) -> `status --json` (`healthy: true`) -> repeat `install`/`update` (`noop: 15`) -> tampered `0.2.0` kit copy `--from /work/custom020` returned exit code `3` (`immutable_version` with hint).
Remaining limitation / external blocker: Cross-OS CI (`windows-latest`, `macos-latest`) runs on push; N02–N12 remain open.
Acceptance checklist result: Pass (upgrade from `main` `f81ce6e` `0.1.0` -> `0.2.0` preserves user BOM/CRLF content, hooks, and `InstalledAt`; tampered same-version blocked cleanly; golden state/journal v1/v2 + error paths tested across all 4 runtimes).
Next task: N02 — Source resolution: tách read-only validation khỏi cache writes và cleanup.

---

Task: N02 — Source resolution không ghi khi preview/read-only
Status: verified
Baseline/branch/SHA: feat/release-hardening @ 8e201ad (N01 CI run 43852808507 green on all 4 jobs)
Observed problem and reproduction: Previously `resolveKitSource` called `cache.CleanStaleDownloads()` unconditionally (deleting any `.part` files even during `--dry-run` or `--offline`), `Cache.DownloadBytes` created `cache/tmp` and temp `.part` files even for preview reads, and `ResolveRemote` called `cache.Put` unconditionally before plan confirmation or no-op detection. `info` and `validate` also lacked read-only `--source`/`--offline` support.
Changed files and behavior:
- `internal/paths/process_{unix,windows}.go`, `internal/installer/lock_liveness.go`: extracted shared `paths.ProcessLiveness` helper so both installer and cache GC check live PIDs without duplicating OS code.
- `internal/dist/cache.go`: replaced disk-touching `DownloadBytes` with pure in-memory `FetchRemoteBytes`; made `Cache.putChecked` no-op cleanly without rewriting/touching timestamps on verified cache hits; added `CacheHooks` boundary observer; protected active downloads via `<part>.lock` PID sidecars and `DefaultStalePartAge` (15m) in `CleanStaleDownloadsOlderThan`.
- `internal/dist/remote.go`: introduced explicit `CacheMode` (`CacheReadOnly` vs `CachePersist`) and `ResolvedPackage.HasPendingCacheWrite()`/`PersistCache(cache)`; `ResolveRemote` reads cache strictly read-only in `CacheReadOnly` mode and retains validated archive bytes for single-snapshot apply.
- `internal/cli/{source,install,update,cli}.go`: removed unconditional `CleanStaleDownloads()` from `resolveKitSource`; passed explicit `dist.CacheReadOnly` for preview, `info`, `validate`, no-op, missing `--yes`, and cancelled prompts; persisted cache via `persistResolvedCache` only on confirmed non-noop mutations using the exact planned `fs.FS` snapshot; added `--source`/`--sha256`/`--offline` support to `info` and `validate`.
- `internal/dist/readonly_test.go`, `internal/cli/source_readonly_test.go`, `internal/dist/dist_test.go`: added full before/after filesystem tree + nanosecond mtime snapshot tests across `--dry-run` (cache miss/hit), `info`/`validate`, `--offline` (with HTTP trap transport), cancelled prompt, missing `--yes`, invalid remote sources, confirmed remote `install`/`update` + `noop` mtime preservation, and active-vs-stale `.part` cleanup.
Tests actually run:
- `docker run --rm -v "${PWD}:/src" -w /src golang:1.27.4 /usr/local/go/bin/gofmt -w cmd internal schemas; .\scripts\verify-docker.ps1` -> exit `0`, all packages passed under `go test -race ./...`, `go vet ./...`, and `go build`.
Remaining limitation / external blocker: Cross-OS CI (`windows-latest`, `macos-latest`) runs on push; N03 HTTP transport/index strictness & parallel cache stress follow next.
Acceptance checklist result: Pass (zero filesystem/mtime mutations on `--dry-run`, `info`, `validate`, `--offline`, cancel, no-`--yes`, and no-op; zero HTTP requests on offline/cache-hit; active `.part` files preserved).
Next task: N03 — Remote transport, registry strictness, `minCliVersion` offline/cached parity, and concurrent cache idempotency.

---

Task: N03 — Remote transport, registry và cache integrity
Status: verified
Baseline/branch/SHA: feat/release-hardening @ b53b764 (N02 CI run 43853976619 green on all 4 jobs)
Observed problem and reproduction: Previously `DefaultDeps` used timeout-less `http.DefaultClient`, `FetchRemoteBytes` validated only the initial URL (allowing unsafe redirect hops/downgrades, leaking query secrets/userinfo via stdlib `*url.Error`, and keeping `Authorization` headers across hosts), `ResolveRemote` parsed registry indexes permissively without duplicate-key/duplicate-version checks or entry caps, `minCliVersion` existed only in index entries and was bypassed in `--offline`/direct archive modes, tar dir headers escaped entry-count limits, and `Cache.Put` lacked cross-process rename contention handling.
Changed files and behavior:
- `schemas/package.schema.json`, `internal/catalog/{catalog,validation}.go`: added optional `minCliVersion` on manifest v1 packages and exported `CheckStrictJSON` / `CheckObjectKeys` / `IsValidID` / `IsValidVersion`.
- `internal/dist/cache.go`: added `DefaultHTTPClient` (30s timeout), bounded `MaxRedirects` (10), per-hop `ValidateRemoteURL`, HTTPS downgrade blocking, cross-host `Authorization`/`Cookie` header stripping, URL credential rejection & `SanitizeURL` scrubbing of `*url.Error` messages, monotonic `<sha>.meta.json` `CacheMetadata` sidecars, and contention-safe idempotent `writeCacheFileIdempotent`.
- `internal/dist/remote.go`: implemented strict `ParseRegistryIndex` (`CheckStrictJSON`, unknown/null field rejection, entry cap `1024`, unique `id@version`, entry field/URL/digest validation, deterministic highest-semver selection); unified `CheckMinCLIVersion`/`SatisfiesMinCLI` (with explicit dev/prerelease base floor policy) across index, direct archive, cached hit, `--offline`, and `--from` (`internal/cli/{deps,source}.go`).
- `internal/dist/archive.go`: added `MaxArchiveEntries` (`2048`) across all tar headers to block header-only floods alongside existing duplicate/case-collision and size limits.
- `internal/dist/{remote_hardening_test,fuzz_test}.go`: added TLS custom CA tests, slow/canceled request redaction tests, redirect hop/downgrade/header-stripping tests, strict index & deterministic semver tests, `minCliVersion` offline/cached parity tests, 16-goroutine `-race` + 4-child-process concurrent `Cache.PutArchive` tests, header-flood/duplicate-tar tests, and `FuzzParseRegistryIndex` / `FuzzReadArchiveBytes`.
- `docs/{cli,release,progress}.md`: documented remote transport/compatibility/SHA-256 policy and marked N03 verified.
Tests actually run:
- `docker run --rm -v "${PWD}:/src" -w /src golang:1.27.4 /usr/local/go/bin/gofmt -w cmd internal schemas; .\scripts\verify-docker.ps1` -> exit `0`, all packages passed under `go test -race ./...`, `go vet ./...`, and `go build`.
- `go test -fuzz=FuzzParseRegistryIndex -fuzztime=3s ./internal/dist` (42,213 execs) and `go test -fuzz=FuzzReadArchiveBytes -fuzztime=3s ./internal/dist` (63,045 execs) -> both `PASS`.
Remaining limitation / external blocker: Cross-OS CI (`windows-latest`, `macos-latest`) runs on push; N04 concurrent cross-runtime `AGENTS.md` / lock / recovery tests follow next.
Acceptance checklist result: Pass (finite timeouts, strict redirect & credential sanitization, TLS custom CA verified, strict index & offline `minCliVersion` parity, idempotent concurrent `Cache.Put`, bounded archive + fuzz smoke green).
Next task: N04 — Đồng thời nhiều runtime và recovery trên file shared (F04).

---

Task: N04 — Đồng thời nhiều runtime và recovery trên file shared (and N03 Windows contention fix)
Status: verified (Docker race/linux; pushing for 3-OS Windows/macOS/Linux CI)
Baseline/branch/SHA: feat/release-hardening @ 3e9686f (N03 CI run 43894917283 passed `race`, `macos-latest`, `ubuntu-latest`; `windows-latest` failed — fixed Windows `writeCacheFileIdempotent` rename/sharing contention in `internal/dist/cache.go` before N04)
Observed problem and reproduction: Previously each runtime used an isolated lock (`<operational>/install.lock`), leaving a TOCTOU lost-update window between `verifyBeforeState` and `atomicWrite` on shared `<project>/AGENTS.md` (`codex` + `opencode` or aliased global homes). `RecoverForRuntime` evaluated disk outside locks and lacked shared-target lock coordination, while `ExecuteTx` revalidated only `Generation` + `Kit.Digest` under lock instead of full state snapshots.
Changed files and behavior:
- `internal/dist/cache.go` (N03 Windows contention fix): enhanced `writeCacheFileIdempotent` to re-check verified existing blobs before attempting `os.Rename` and increased backoff retries under Windows `MoveFileEx` / `os.Open` sharing contention.
- `internal/installer/lock.go`: added `canonicalTargetPath` (resolving symlinked existing ancestors + lowercase on Windows) and `sharedTargetLockPaths` / `acquireSharedLocks` / `releaseLockList` writing per-canonical-target `.lock` files under `<scopeRoot>/.dtkit/locks/<sha>.lock`. Strict DAG lock order: runtime `install.lock` -> sorted canonical shared target locks (never acquiring runtime lock while holding a shared lock; non-overlapping targets such as Cursor `.cursor/*` proceed without blocking Codex/OpenCode).
- `internal/installer/engine.go`: acquires shared target locks inside `ExecuteTx` before target checks/writes/rollback, and enforces strict canonical JSON state comparison (`state.ParseAndValidate` + `json.Marshal` equality) under lock so mutations to root bindings/artifacts/hooks under unchanged `Generation` + `Kit.Digest` fail as `state_changed`.
- `internal/installer/recover.go`: evaluates disk and executes recovery strictly under runtime + sorted shared-target locks; clears dead shared locks only when owned by that dead transaction/PID; refuses on live, unknown, age-only-old, or corrupt locks; preserves concurrent runtime blocks and journal evidence on `recovery_conflict`.
- External editor limit documented: external non-DTKit editors do not consult `.dtkit/locks/`, so protection against concurrent outside edits remains DTKit's pre-write digest check + conditional rollback, subject to OS rename TOCTOU between read check and `os.Rename`.
- `internal/installer/concurrency_test.go`: added barrier (`install/install`, `update/update` + non-overlapping `cursor`, `remove/update`, crashed-tx + OpenCode `recover`), strict state revalidation under lock, aliased global roots + case-variant + lock age/corrupt/dead policies, and multi-process OS `os/exec` race tests.
Tests actually run:
- `docker run --rm -v "${PWD}:/src" -w /src golang:1.27.4 /usr/local/go/bin/gofmt -w cmd internal schemas; .\scripts\verify-docker.ps1` -> exit `0`, all packages passed under `go test -race ./...`, `go vet ./...`, and `go build`.
Remaining limitation / external blocker: Pushing next to confirm `windows-latest`, `macos-latest`, `ubuntu-latest`, and `race` CI all green for N03 fix + N04 before starting N05.
Acceptance checklist result: Pass (deterministic winner or safe conflict/rollback with zero lost `AGENTS.md` blocks across goroutine barriers and OS child processes; non-overlapping runtimes unblocked; recovery preserves concurrent runtime blocks and backups).
Next task: N05 — Capability contract và status chính xác (F07).

---

Task: N05 — Capability contract và status chính xác
Status: verified
Baseline/branch/SHA: feat/release-hardening @ baa0a69 (N03+N04 CI run 43899396900 green across race, ubuntu-latest, macos-latest, windows-latest)
Observed problem and reproduction: Previously `RenderKit` silently omitted `instructions` on Cursor global despite `engineer/package.json` listing `"instructions"` in `requiresCapabilities`, `install` appended absent-binary and Cursor-global fallback warnings only late in apply (omitted in `--dry-run`, `noop`, `update`, `info`, and `status`), and `status`/`doctor` did not separate file integrity (`healthy`) from runtime integration verification (`unverified`/`partial`/`verified`).
Changed files and behavior:
- `schemas/package.schema.json`, `internal/catalog/{catalog,validation}.go`: added optional `optionalCapabilities` field, allowed `hooks` in capability vocabulary cleanly, and rejected overlapping or unknown capabilities.
- `internal/catalog/engineer/package.json`, `internal/dist/scaffold.go`: declared `"requiresCapabilities": ["skills"], "optionalCapabilities": ["instructions"]` explicitly so skill-only Cursor global fallback is contractual rather than silent bypass.
- `internal/adapter/{adapter,render}.go`: added `IntegrationEvidence` (`unverified`/`partial`/`verified`) per runtime/scope and `EvaluateCapabilities`, called by `RenderKit` (`validate`/`pack`/`install`/`update`) so unsatisfied `requiresCapabilities` (such as strict `instructions` on Cursor global or `hooks` on package install) fail pre-write with `unsupported_capability` while `optionalCapabilities` emit consistent fallback warnings.
- `internal/installer/status.go`: added `IntegrationStatus` and `IntegrationNote` to `StatusReport` alongside artifact health (pure static evidence metadata, zero network calls).
- `internal/cli/{envelope,runtime,install,update,status,doctor,cli}.go`: unified `applyScopeCapabilitiesAndBinaryWarnings` across `info`, `install --dry-run`/apply/noop, `update --dry-run`/apply/noop, `status`, and `doctor` (text + JSON), separating artifact file health from runtime integration status.
- `internal/adapter/capabilities_test.go`, `internal/cli/capabilities_test.go`: tested all 8 runtime/scope pairs for strict vs optional capabilities, hooks/unknown capability failures, zero pre-write leakage, cross-command warning/capability parity, and status/doctor integration distinction.
Tests actually run:
- `docker run --rm -v "${PWD}:/src" -w /src golang:1.27.4 /usr/local/go/bin/gofmt -w cmd internal schemas; .\scripts\verify-docker.ps1` -> exit `0`, all packages passed under `go test -race ./...`, `go vet ./...`, and `go build`.
Remaining limitation / external blocker: Cross-OS CI runs on push; runtime `integrationStatus` reflects real observed evidence prior to N08 full runtime verification.
Acceptance checklist result: Pass (required capabilities strictly fail pre-write; optional fallback explicitly declared and warned identically across preview, apply, noop, info, status, and doctor; artifact health cleanly separated from runtime integration evidence).
Next task: N06 — Hook ownership, parser và executable lifecycle (F12).

---

## Historical Implementation Log (Point-in-Time Snapshots)

> The entries below were recorded at the time each dev milestone completed and are kept intact for historical context; refer to **Current Status** above for live release-gate truth.

## M0 — Audit

Status: verified (baseline environment recorded)

- Workspace initially contained the planning documents; Git branch `feat/dtkit-foundation` connects to `origin` (`https://github.com/truong-newbie/Agent_Kit_Impl.git`).
- Native Windows `go.exe` remains blocked by Application Control/Device Guard; `scripts/verify-docker.ps1` runs `gofmt`, `go test ./...`, `go vet ./...`, and `go build ./cmd/dtkit` inside `golang:1.27.4`.
- `codex.cmd --version` reports `codex-cli 0.169.0` (executable presence only; Codex runtime verification is pending).

## P0 — Baseline, root resolution, error/JSON contract, and strict validation

Status: implemented and local-container verified (CI re-run pending push)

- Audited GitHub Actions run `43834146403` (commit `8223446`, `https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/43834146403`): `windows-latest` and `ubuntu-latest` passed, while `macos-latest` failed `go test ./...` because `ResolveTarget` walked above `t.TempDir()` and hit macOS `/var` -> `/private/var` symlink.
- Added `internal/paths` (`ResolveProject`, `ResolveGlobal`, `ResolveTarget`, `ValidateRelativePath`, `IsReparseOrSymlink`): unified root resolution across `install`, `status`, and `doctor`; stops ancestor link traversal at the resolved root (rejecting symlink/reparse roots and symlink/reparse descendants beneath root).
- Added `internal/apperr` and `internal/buildinfo`, mapped exit codes (`0`, `1`, `2`, `3`, `4`, `130`) in `cmd/dtkit/main.go`, unified JSON `Envelope` (`internal/cli/envelope.go`), added TTY `[y/N]` confirmation prompts, dual-scope `status`/`doctor` when scope is omitted, `validate <dir>` rendering across package scopes, and machine-readable JSON schemas in `schemas/`.
- Tightened `internal/catalog` (`Snapshot`, `LoadDir`, `ResolveOrder` deterministic topological sort with cycle paths, disallowed executable extensions/magic headers/directories, `SKILL.md` vs `SKILL.md.tmpl` exclusivity) and `internal/installer/status.go` (`CodexHome` mismatch detection, 4 MiB state / 8 MiB artifact read caps, regular-file checks).
- Verification: `.\scripts\verify-docker.ps1` passed (`gofmt`, `go test ./...`, `go vet ./...`, `go build ./cmd/dtkit` in `golang:1.27.4` Linux container).
- Known limitations at P0: `update`, `remove`, `recover` are not implemented yet; `FreshInstall` still needs the P1 transaction/journal/lock/recovery overhaul; native Windows/macOS CI for the P0 commit has not run yet.
- Next: P1 — Transaction engine, lock metadata, conditional rollback, and `recover`.

## P1 — Transaction engine and recovery

Status: implemented and local-container verified (CI pending)

- Added versioned transaction journal (`internal/installer/journal.go`), exclusive PID/token lock metadata with OS liveness inspection (`internal/installer/lock.go`, `lock_unix.go`, `lock_windows.go`), general transaction engine (`internal/installer/engine.go`), and `Recover` (`internal/installer/recover.go`, CLI `dtkit recover`).
- Replaced global mutable writer test seams with per-call `TxHooks`, removed `O_TRUNC` write fallback, tracked DTKit-created directories for safe empty-dir pruning, and retained up to 10 completed transactions while preserving unresolved journals.
- Updated `Inspect` (`status` and `doctor`) to flag unresolved transactions and lock presence as `recoveryNeeded` with exit code `3`.
- Verification: `.\scripts\verify-docker.ps1` passed (including fault injection across backup/prepared/action/state-commit boundaries, external editor rollback conflict, forward/rollback recovery, live vs dead PID lock handling, and journal allowlist validation).
- Known limitations: `update` and `remove` CLI commands, unified state+disk+desired planner, and idempotent `install` are next (P2–P4); Windows native locked-file/junction tests remain to run on Windows CI/native runner.
- Next: P2 — Unified planner (`state + disk + desired`), strict state loader, and idempotent `install`.

## P2 — Unified planner, strict state loader, status, and idempotent install

Status: implemented and local-container verified (CI pending)

- Added strict state loader (`internal/state/load.go`) with duplicate-key, unknown-field, size-cap, and allowlist enforcement independent of the bundled catalog.
- Added unified `PlanDesired` / `PlanRemove` (`internal/planner/plan.go`) reconciling `state + disk + desired` according to the decision matrix.
- Updated `Inspect` (`internal/installer/status.go`) to classify `not_installed`, `healthy`, `modified`, `missing`, `conflicted`, `recovery_required`, and `profile_mismatch`, and report unknown files inside DTKit skill directories without claiming ownership.
- Made `dtkit install` idempotent when matching kit/version/digest/adapter and healthy (absolute no-op without lock, journal, or state rewrite), and verified project + global coexistence.
- Verification: `.\scripts\verify-docker.ps1` passed (`internal/state`, `internal/planner`, `internal/installer`, `internal/cli` tests including BOM/CRLF preservation, state corruption, profile mismatch, and idempotent no-op).
- Known limitations: `remove` and `update` commands are next (P3–P4).
- Next: P3 — Safe `dtkit remove` using state + transaction engine.

## P3 — Safe removal (`dtkit remove`)

Status: implemented and local-container verified (CI pending)

- Implemented `PlanRemove` / `Remove` (`internal/installer/remove.go`) and `dtkit remove` (`internal/cli/remove.go`) using the P1 transaction engine and P2 planner.
- Removal operates purely from `state.json` (no catalog dependency), forgets already-missing whole-files, preserves pre-existing `AGENTS.md` (including empty files) and personal files in skill directories, rejects modified managed files with exit code `3`, and prunes only empty DTKit-owned directories while retaining operational journal history.
- Verification: `.\scripts\verify-docker.ps1` passed (`internal/installer/remove_test.go` and `internal/cli/cli_test.go` covering empty `AGENTS.md`, personal skill files, drift conflict, fault-injection rollback, kit ID mismatch, and project/global isolation).
- Known limitations: `update` command is next (P4).
- Next: P4 — `dtkit update` with semver checks, immutable-version enforcement, artifact migration, and adapter revision support.

## P4 — Update and version/adapter migration

Status: implemented and local-container verified (CI pending)

- Implemented `PlanUpdate` / `Update` (`internal/installer/update.go`) and `dtkit update` (`internal/cli/update.go`) reusing the P1 transaction engine and P2 `PlanDesired` matrix.
- Enforces semver comparison (rejects downgrades), immutable-version digest verification (same version with different digest fails), no-op on identical healthy version/digest/adapter, artifact addition/update/removal across kit versions, `Generation` increment, `InstalledAt` preservation, and `AGENTS.md` BOM/CRLF/outside-content preservation.
- Verification: `.\scripts\verify-docker.ps1` passed (`internal/installer/update_test.go` and `internal/cli/cli_test.go` covering v0.1->v0.2 migration with added/removed skills, immutable-version conflict, downgrade rejection, drift conflict, fault-injection rollback, adapter migration detection, and CLI `--from` update).
- Known limitations: hardening/fuzz/race/built-binary integration tests, Engineer Kit content enrichment (P5), Codex runtime smoke verification, and release readiness (P6) remain.
- Next: P5 — Hardening, fuzzing, race tests, built-binary E2E script, and Engineer Kit content review.

## P5 — Hardening, fuzzing, race tests, and Engineer Kit content

Status: implemented and local-container verified (Windows/macOS CI & Codex smoke pending)

- Enriched all 7 Engineer Kit skills (`plan`, `implement`, `debug`, `review`, `test`, `explain`, `ask`), `rules/engineering.md`, 3 workflows, and 3 templates with explicit scope constraints, resource template references, examples, and missing-information handling; added automated content/link/line-limit tests in `internal/adapter/codex/render_test.go`.
- Added `.dtkit/local/` `.gitignore` check to `dtkit doctor` and tip output on `dtkit install --project`.
- Added bounded fuzz tests (`FuzzBlockOperations`, `FuzzValidateRelativePath`, `FuzzParseAndValidateState`, `FuzzCheckJSON`), built-binary CLI E2E lifecycle test (`internal/cli/e2e_test.go`), and `go test -race ./...` in both `scripts/verify-docker.ps1` and `.github/workflows/ci.yml`.
- Verification: `.\scripts\verify-docker.ps1` (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`) passed; bounded 3s fuzz runs passed across all 4 targets (~99k–132k execs each, 0 failures).
- Known limitations: Windows-native junction/locked-file tests and macOS CI re-run depend on GitHub Actions; real Codex CLI runtime verification and release packaging belong to P6.
- Next: P6 — Codex runtime smoke check (if authenticated), docs/README, release packaging scripts/workflow, and release readiness checklist.

## P6 — Release packaging, documentation, and Codex smoke check

Status: implemented and local-container verified (release candidate ready for CI & owner gate; global Codex smoke pending)

- Added cross-platform build/archive scripts (`scripts/build.ps1`, `scripts/package.ps1`) and `.github/workflows/release.yml` (ready for review; no automatic publish).
- Built and verified `0.1.0-rc1` archives + `SHA256SUMS` for `windows_amd64`, `linux_amd64`, and `darwin_arm64`; verified extracted `dtkit.exe` runs natively on Windows and extracted Linux binary runs in `golang:1.27.4` container.
- Executed isolated project-scope Codex CLI smoke (`codex-cli 0.169.0`) documented in `docs/verification/codex-0.169.0-2026-11-16.md`: Codex discovered `$dtkit-project-plan` and `$dtkit-project-review`, referenced `.agents/skills/...`, `.dtkit/content/engineer/rules/engineering.md`, and `.dtkit/content/engineer/templates/review-report.md`, and refused to fabricate unverified file contents when Windows read-only sandbox policy blocked PowerShell reads.
- Updated `README.md`, `docs/cli.md`, `docs/development.md`, `docs/compatibility.md`, and `docs/release.md` with accurate capabilities, terminal install/checksum verification, and remaining release gates.
- Next: Push branch to verify Windows/Linux/macOS + race CI on GitHub Actions, run isolated global Codex smoke in a separate OS user account, and confirm owner decisions (`go.mod` module path, `LICENSE`, visibility).

## v0.1 Release Prep — Module path, MIT License, and CI verification

Status: implemented and verified (ready to merge `feat/dtkit-foundation` -> `main`; `v0.1.0` tag deferred per owner decision)

- Confirmed GitHub Actions run `43839922917` (`c178120`) passed across `windows-latest`, `ubuntu-latest`, `macos-latest`, and `race`.
- Added MIT `LICENSE`, migrated Go module path and `-ldflags` from `dtkit.local/dtkit` to `github.com/truong-newbie/Agent_Kit_Impl`, and included `LICENSE` in `scripts/package.ps1` and `.github/workflows/release.yml`.
- Recorded additional `codex-cli 0.169.0` `-s workspace-write` observation in `docs/verification/codex-0.169.0-2026-11-16.md` (host sandbox policy blocks nested `powershell.exe` execution; discovery and path resolution confirmed).
- Next: Verify `main` CI after merge, then begin **H1 (v0.2 Hooks opt-in)** on a dedicated feature branch.

## H1 — v0.2 Opt-in Hooks (`session-context`)

Status: implemented and local-container verified (CI & real Codex `/hooks` trust smoke pending)

- Added `internal/hooks` (`command.go`, `merge.go`, `runner.go`) implementing:
  - Stable executable resolution (rejects `go-build` / ephemeral temp binaries), POSIX single-quoting, and Windows PowerShell `& '...'` quoting (`commandWindows`).
  - Structural JSON merge/unmerge (`MergeHooksJSON`, `UnmergeHooksJSON`, `InspectHooksJSON`) for Codex `hooks.json` (`SessionStart`) using canonical entry SHA-256 digests in `state.json` without injecting non-standard keys into `hooks.json`.
  - Read-only `RunSessionContext` handler (`dtkit hook run session-context`) bounded to 1 MiB stdin, 3s timeout, and 2 KiB `additionalContext`, ignoring payload `cwd`.
- Upgraded state schema (`internal/state`, `schemas/state.schema.json`) to support v1 -> v2 migration with `Hooks` registrations; integrated `hooks enable/disable/status/list` with `ExecuteTx`, cross-scope duplicate guard, `dtkit remove` cleanup, and `dtkit update` preservation.
- Verification: `.\scripts\verify-docker.ps1` passed (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`), including `internal/hooks/hooks_test.go`, `internal/installer/hooks_test.go`, `internal/cli/hooks_test.go`, and built-binary `sh -c` hook execution in `internal/cli/e2e_test.go`.
- Next: Push `feat/hooks-v0.2` and verify Windows/Linux/macOS + `-race` CI (including PowerShell `commandWindows` execution in `e2e_test.go`), test interactive `/hooks` trust review in Codex CLI, then move to D1 (v0.3 packaging/remote).

## D1 — v0.3 Authoring, deterministic archives, digest cache, and remote sources

Status: implemented and local-container verified (CI pending)

- Added `internal/dist` (`scaffold.go`, `archive.go`, `cache.go`, `remote.go`):
  - `dtkit create kit` & `dtkit create skill` scaffolding standalone kits validated by `catalog.LoadDir` + Codex render.
  - `dtkit pack` generating deterministic reproducible `.tar.gz` archives + `.sha256` files, and `ReadArchiveBytes` / `ReadArchiveFile` validating archives purely in memory (rejecting traversal, symlinks, hardlinks, FIFOs/devices, case collisions, scripts/executables, and >1 MiB/10 MiB payloads).
  - Digest-keyed cache (`Cache`) with atomic `.part` download cleanup, cache corruption detection on read, HTTPS-only enforcement (plus loopback HTTP for test servers), `<id>@<version>` resolution, registry index JSON support, `minCliVersion` check, and `--offline --sha256` reinstall.
- Extended `state.Kit` (`archiveSha256`, `archive`/`remote` source kinds) and `paths.Environment.UserCacheDir`, and updated `install`, `update`, `info`, `validate` to share `resolveKitSource`.
- Added unit/integration tests in `internal/dist/dist_test.go` and `internal/cli/dist_test.go` covering deterministic pack, hostile archives, partial download `.part` cleanup, cache corruption, `minCliVersion`, offline reinstall, and end-to-end authoring -> remote install -> update -> remove -> offline cache reinstall.
- Verification: `.\scripts\verify-docker.ps1` passed (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`); GitHub Actions run `43844758850` passed all 4 jobs (`windows-latest`, `ubuntu-latest`, `macos-latest`, `race`).
- Next: Implement R1–R3 sequentially (`gemini`, `opencode`, `cursor`).

## R1 — v0.4 Gemini CLI Adapter

Status: implemented and local-container verified (CI pending; `gemini` CLI not installed on host)

- Extracted shared multi-runtime core in `internal/adapter` (`adapter.go`, `render.go`) with `RuntimeProfile`, `CapabilityStatus` (`RuntimeSupported` vs `DTKitSupported`), shared template rendering/frontmatter validation, and per-runtime state/lock separation (`.dtkit/state.json` for `codex`, `.dtkit/runtimes/<runtime>/state.json` + `.dtkit/local/runtimes/<runtime>` for new runtimes).
- Generalized `internal/state`, `internal/planner`, `internal/installer`, `internal/dist`, `schemas/`, and CLI commands (`validate`, `doctor`, `install`, `status`, `update`, `remove`, `recover`, `hooks`) to accept `--runtime` while keeping existing v1/v2 `codex` state files backward compatible.
- Added `internal/adapter/gemini` (`GEMINI.md` managed block, `.gemini/skills/dtkit-gemini-project-*`, `~/.gemini/skills/dtkit-gemini-global-*` via `$GEMINI_CLI_HOME/.gemini` or `~/.gemini`, `.dtkit/runtimes/gemini/content/<kit>/...`, and `.gemini/settings.json` `context.fileName` override inspection in `dtkit doctor`).
- Expanded `internal/catalog/engineer/package.json` (`runtimeTargets: ["codex", "gemini"]`) and added isolation/coexistence/lifecycle tests across `internal/adapter`, `internal/paths`, `internal/installer`, and `internal/cli`.
- Verification: `.\scripts\verify-docker.ps1` passed (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`); GitHub Actions run `43847448522` passed all 4 jobs (`windows-latest`, `ubuntu-latest`, `macos-latest`, `race`); real Gemini CLI runtime smoke remains pending (`gemini` executable not present on host).
- Next: **R2 (OpenCode adapter)**.

## R2 — v0.4 OpenCode Adapter

Status: implemented, local-container verified, and `opencode 1.19.5` discovery-smoked (CI pending)

- Added `paths.ResolveOpenCodeGlobal` (`$OPENCODE_CONFIG_DIR` -> `$XDG_CONFIG_HOME/opencode` -> `~/.config/opencode`), verified against live `opencode 1.19.5` `opencode debug paths` on Windows.
- Extended `internal/planner/block.go` (`AppendBlockForRuntime`, `ReplaceBlockForRuntime`, `BlockDigestForRuntime`, `RemoveBlockForRuntime`) with runtime-namespaced managed block markers (`<!-- dtkit:begin <kit> opencode <scope> -->`) and closed sibling-block parsing so Codex and OpenCode share `AGENTS.md` without ownership collisions while malformed/unclosed/nested blocks fail closed.
- Added `internal/adapter/opencode` (`.opencode/skills/dtkit-opencode-project-*`, `~/.config/opencode/skills/dtkit-opencode-global-*`, `AGENTS.md` / `<opencode-home>/AGENTS.md`, `.dtkit/runtimes/opencode/content/<kit>/...`, strict `opencode.json` syntax warning in `dtkit doctor`), opened `--runtime opencode` in CLI, `catalog`, and `schemas/`, and added 3-way coexistence (`codex` + `gemini` + `opencode`) tests.
- Verification: `.\scripts\verify-docker.ps1` passed (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`); isolated `opencode 1.19.5` `debug skill` + `debug config` smoke verified all 14 project+global skills alongside Codex in `docs/verification/opencode-1.19.5-2026-11-16.md`; GitHub Actions run `43848615457` passed all 4 jobs (`windows-latest`, `ubuntu-latest`, `macos-latest`, `race`).
- Next: **R3 (Cursor adapter)**.

## R3 — v0.4 Cursor Adapter

Status: implemented and local-container verified (CI pending; `cursor` CLI not installed on host)

- Generalized `InstructionLocation` (`Strategy`, `PathTemplate`, `RelPath`, `MatchesWholeFile`) and `paths.ResolveUserHome` cleanly inside `internal/adapter` (`adapter.go`, `render.go`) and `internal/planner/install.go`, supporting both `managed-block` instructions and whole-file rule artifacts plus optional global instruction capability fallback.
- Added `internal/adapter/cursor` with:
  - Project scope: `.cursor/rules/dtkit-<kit>.mdc` (`alwaysApply: true`, whole-file), `.cursor/skills/dtkit-cursor-project-*`, `.dtkit/runtimes/cursor/content/<kit>/...`
  - Global scope: `~/.cursor/skills/dtkit-cursor-global-*` and `~/.dtkit/runtimes/cursor/content/<kit>/...` under `user-home`, plus honest fallback (`Capabilities.Instructions = {runtimeSupported: true, dtkitSupported: false}`) surfacing a clear warning for Cursor Settings UI User Rules rather than faking an unsupported global rules file.
- Opened `--runtime cursor` across `catalog`, `schemas/`, `internal/cli` (`runtime.go`, `cli.go`, `install.go`, `update.go`, `remove.go`, `doctor`, `status`), and added `internal/cli/cursor_test.go` verifying 4-way coexistence (`codex` + `gemini` + `opencode` + `cursor`), personal `.cursor/rules/*.mdc` preservation, and global fallback warnings.
- Verification: `.\scripts\verify-docker.ps1` passed (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`); real Cursor runtime smoke remains pending (`cursor` executable not present on host).
- Next: Push `feat/runtime-cursor-v0.4` and verify Windows/Linux/macOS + `-race` CI across the complete v0.1–v0.4+ implementation roadmap.
