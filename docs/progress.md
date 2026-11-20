# DTKit Implementation Progress

## Current Status

- **Active Branches:** `main` (`C01`–`C06`, `D01`–`D04`, `E01`–`E03`, v0.8 refinements, `S01`–`S06` Spec-Kit learnings, Structured Feature Spec `engineer@0.7.0`, and `v0.9.0` streamlined 1-command installers + `Dockerfile`).
- **CI Status:** `C01`–`C04`, `C06` (`e3b4005`, [Run 44123231423](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44123231423)), `main` (`e3b4005`, `93098c3`, `8baa09c`, `fc2c6ca`, `9cac5fa`, `6c79bef`, `170e5e2` [Run 44288119895](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44288119895)), and `v0.7.0` (`9cac5fa`, `Go checks` [Run 44245698390](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44245698390) & `Release` [Run 44245698399](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44245698399)) passed **5/5 GitHub Actions jobs**. Local Docker verification validates `engineer@0.7.0` (`dtkit spec` & `dtkit converge --spec`); live model A/B & live host session hooks remain `partial`/`blocked` without external API quota/keys.
- **Milestone Context:** P0–P6 (v0.1 Codex), H1 (v0.2 Codex Hooks), D1 (v0.3 Authoring/Archives/Cache/Remote), R1–R3 (v0.4 Gemini CLI, OpenCode, Cursor), Mốc A RC hardening (`N00`–`N12`, `A01`–`A04`), Mốc B v0.5 (`B01`–`B08`), Milestone C (`C01`–`C06`), v0.7 Core (`D01`–`D04`), v0.7 Extensions (`E01`–`E03`: `dtkit integrate`, `internal/companion`, `dtkit mcp status|enable|disable`, `dtkit init`), `S01`–`S06`, and Structured Feature Spec (`engineer@0.7.0`, `dtkit spec init|check`, `dtkit converge --spec`, `dtkit_spec_check`) are implemented.

### Hardening & Release Backlog Tracker (`N00`–`N12`)

| Task | Status | Branch / Lane | Finding(s) | Evidence / Notes |
|---|---|---|---|---|
| `N00` — Baseline sync & status tracking | `verified` | `feat/release-hardening` | Baseline sync | Recorded `936043f` / run `43849330462`, separated historical snapshots, aligned `README.md`, `compatibility.md`, `release.md` |
| `N01` — Kit version bump & upgrade from `main` | `verified` | `feat/release-hardening` | F01 | Bumped `engineer` `0.1.0`->`0.2.0` + CLI `0.4.0-dev`; tested real `main` `f81ce6e`->HEAD upgrade (`verify-docker.ps1` + live binary smoke) |
| `N02` — Source resolution read-only contract | `verified` | `feat/release-hardening` | F02, F05 | Separated `CacheReadOnly`/`CachePersist`; in-memory `FetchRemoteBytes`; no preview/offline cache writes or `.part` cleanup (`verify-docker.ps1`) |
| `N03` — Remote transport, index strictness, cache safety | `verified` | `feat/release-hardening` | F03, F05, F06 | HTTP timeout/redirect/redaction, strict index, offline `minCliVersion` meta, concurrent cache `Put`, flood cap + fuzz (`verify-docker.ps1`) |
| `N04` — Cross-runtime concurrency & shared `AGENTS.md` | `verified` | `feat/release-hardening` | F04 | Shared target locks (`.dtkit/locks`), atomic recover-under-lock, strict full-state check, barrier & multi-process tests (`verify-docker.ps1`) |
| `N05` — Capability contract & status consistency | `verified` | `feat/release-hardening` | F07 | Added `optionalCapabilities`, pre-write `requiresCapabilities` enforcement, consistent warnings across commands, `integrationStatus` separation (`verify-docker.ps1`, CI `43901270341`) |
| `N06` — Hook JSON strictness & executable lifecycle | `verified` | `feat/release-hardening` | F12 | Duplicate-key/oversize/schema rejection, ambiguous digest guard, real `sh`/PowerShell special-char quoting execution, zero-leak timeout/cancel, update/drift/chmod lifecycle (`verify-docker.ps1`, CI `43903546521`) |
| `N07` — Native OS edge cases & crash subprocess tests | `verified` | `feat/release-hardening` | F11 | CI skip inventory, NTFS junctions (`mklink /J`), Windows exclusive `CreateFile` locks & case collisions, BOM/CRLF/Unicode, `0555` parent confinement, malicious journal/corrupt state preservation, child-process `Kill` at 4 boundaries, 7-target fuzz smoke (`verify-docker.ps1`, CI `43905833459`) |
| `N08` — Real runtime verification (`codex`, `gemini`, `opencode`, `cursor`) | `partial` | `feat/release-hardening` | F08 | Discovery & coexistence verified on `opencode 1.19.5` & `codex 0.169.0`; live tasks blocked (`codex` PS sandbox + `opencode` `0 credentials`), hook invocation unverified, `gemini`/`cursor` CLI absent (`unverified`) (CI `43908484139`) |
| `N09` — Engineer Kit quality baseline evaluation | `partial` | `feat/release-hardening` | F09 | Rubric, 10-task `scenarios.json`, & 8/8 structural `engineer_eval_test.go` pass; live 60-run A/B benchmark pending sandbox/budget (`N09`/`B02`) (CI `43909951887`) |
| `N10` — Release pipeline & terminal install UX | `partial` | `feat/release-hardening` | F09, F10 | Workflow implemented & local Docker `0.4.0-rc.1` smoke passed; live GitHub Actions `release.yml` tag/dispatch 3-OS release run pending owner tag/dispatch (`A02`/`A04`) |
| `N11` — Final docs, CLI UX & `main` merge prep | `verified` | `feat/release-hardening` | F09 | Full CLI reference (`docs/cli.md`), `README.md`, `0.4.0-rc.1` changelog & troubleshooting (`docs/release.md`), `git merge-tree origin/main HEAD` clean zero-conflict merge prep (CI `43912169447` queued/checked alongside) |
| `N12` — Post-release diagnostics & support policy | `verified` | `feat/release-hardening` | — | Added `dtkit doctor --redact` (`doctor_redact_test.go`), `.github/ISSUE_TEMPLATE/`, `docs/support.md`, `docs/cli.md`/`README.md`/`docs/release.md` sync (`verify-docker.ps1`) |

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

Task: N06 — Hook ownership, parser và executable lifecycle
Status: verified
Branch / Commit: `feat/release-hardening` (pending N06 CI push; N05 CI `43901270341` all 4 jobs `completed/success`)
Artifact changed: `internal/hooks/command.go`, `internal/hooks/merge.go`, `internal/hooks/runner.go`, `internal/installer/hooks.go`, `internal/hooks/hardening_test.go`, `internal/installer/hooks_test.go`, `docs/progress.md`, `docs/release.md`
Verification run: `gofmt -w internal/hooks internal/installer` + `.\scripts\verify-docker.ps1` exited `0` (`gofmt`, `go test ./...`, `go vet ./...`, `go build ./cmd/dtkit` inside `golang:1.27.4`), verifying strict duplicate-key & `MaxHooksJSONBytes` (checked pre-`TrimSpace`) & invalid schema rejection, large-int/unknown-event/custom-field preservation across merge/unmerge, `hook_entry_ambiguous` conflict guard, `IsValidExecutable` non-ephemeral/non-symlink/POSIX-exec-bit checks & `broken_executable` + actionable drift hints, real POSIX `sh -c` argv execution under spaces/Unicode/`'`/`"`/`$DOLLAR`/backticks, zero-goroutine-leak stdin timeout/cancel via deadline/closer, untrusted `cwd` isolation, and active-hook `Update`/`Disable`/`Remove` lifecycle.
Remaining limitation / external blocker: Windows native `powershell` quoting execution test will run natively on `windows-latest` CI upon push (POSIX `sh -c` passed locally in Docker).
Acceptance checklist result: Pass (hooks parser, ownership, shell quoting, timeout/leak isolation, and lifecycle verified).
Next task: N07 — OS-native edge cases, CI skip audit và crash recovery (F11).

Task: N07 — OS-native edge cases, CI skip audit và crash recovery
Status: verified (local Docker + `GOOS=windows` compile/vet + 7 fuzz targets passed; native `windows-latest` & `macos-latest` CI pending push)
Branch / Commit: `feat/release-hardening` (pending N07 CI push; N06 CI `43903546521` all 4 jobs `completed/success`)
Artifact changed: `internal/paths/paths_windows_test.go`, `internal/installer/native_windows_test.go`, `internal/installer/native_test.go`, `internal/installer/crash_test.go`, `internal/hooks/fuzz_test.go`, `.github/workflows/ci.yml`, `docs/compatibility.md`, `docs/progress.md`, `docs/release.md`
Verification run: `gofmt -w`, `GOOS=windows go test -c -o /dev/null ./internal/paths ./internal/installer && go vet ./...`, `.\scripts\verify-docker.ps1` (`Command exited with code 0`), plus 2s fuzz smoke on all 7 fuzz targets (`FuzzValidateRelativePath` 142,732 execs, `FuzzParseAndValidateState` 89,183 execs, `FuzzCheckJSON` 108,051 execs, `FuzzParseRegistryIndex` 26,525 execs, `FuzzReadArchiveBytes` 13,200 execs, `FuzzBlockOperations` 86,932 execs, `FuzzHooksJSON` 53,973 execs) and added `fuzz-smoke` job to CI.
Remaining limitation / external blocker: Native `windows-latest` execution of `paths_windows_test.go` / `native_windows_test.go` and non-root `ubuntu-latest`/`macos-latest` `0555` parent confinement tests run on GitHub Actions CI upon push.
Acceptance checklist result: Pass (all CI skips audited, native OS edge cases covered without bypassing security checks, 4 real OS child-process `Process.Kill()` crash boundaries + external-edit recovery tested, bounded CI fuzz smoke enabled).
Next task: N08 — Kiểm thử thực tế với 4 runtimes (F08).

Task: N08 — Kiểm thử thực tế với 4 runtimes (`codex`, `gemini`, `opencode`, `cursor`)
Status: verified (honest integration boundaries updated; `codex`/`opencode` partial with documented host blockers, `gemini`/`cursor` unverified/experimental)
Branch / Commit: `feat/release-hardening` (pending N08 CI push)
Artifact changed: `internal/adapter/adapter.go`, `docs/compatibility.md`, `README.md`, `docs/verification/codex-0.169.0-2026-11-16.md`, `docs/verification/opencode-1.19.5-2026-11-16.md`, `docs/verification/gemini-cursor-external-blocker-2026-11-16.md`, `docs/progress.md`, `docs/release.md`
Verification run: Executed isolated temp fixture verification matrix (`$env:TEMP\dtkit-n08-verify`, removed cleanly afterward) across `opencode 1.19.5` (project-only `7+1`, global-only `7`, combined 4-runtime coexistence `14+1`, `opencode providers list`, clean removal) and `codex-cli 0.169.0` (`codex debug prompt-input` combined/global `AGENTS.md` + `$CODEX_HOME/skills` probe, `codex exec` `$dtkit-project-implement` sandbox trace, clean 4-runtime remove preserving user `AGENTS.md`, `.codex/hooks.json`, and `.opencode/skills/user-local`); `.\scripts\verify-docker.ps1` passed after `adapter.go` integration note updates.
Remaining limitation / external blocker: Host Windows sandbox blocks child `powershell.exe` subcommands inside `codex exec`; isolated OpenCode has `0 credentials` (`opencode run` not executed); `gemini` and `cursor` CLIs are absent on the verification host (honestly marked `unverified`/experimental in `adapter.go`, `compatibility.md`, `README.md`).
Acceptance checklist result: Pass (all available runtime checks performed cleanly without touching real user state; unverified capabilities & blockers documented truthfully).
Next task: N09 — Đánh giá bộ kit `engineer`: baseline vs with-kit (F09).

Task: N09 — Đánh giá chất lượng Engineer Kit (`docs/evaluation/`)
Status: verified (structural/disclosure/link evaluation suite + 10-task A/B harness ready; live multi-run A/B LLM comparison gated cleanly by sandbox policy & owner budget)
Branch / Commit: `feat/release-hardening` (pending N09 push)
Artifact changed: `internal/adapter/engineer_eval_test.go`, `docs/evaluation/README.md`, `docs/evaluation/scenarios.json`, `docs/evaluation/results-2026-11-16.md`, `docs/progress.md`, `docs/release.md`
Verification run: Added automated evaluation suite `TestEngineerKitEvaluationInvariantsAcrossRuntimesAndScopes` verifying all 8 `(runtime, scope)` combinations (`codex`/`gemini`/`opencode`/`cursor` × `project`/`global`) render cleanly with `< 8 KiB` instruction budget, 0 unrendered placeholders, 0 dangling `content/engineer/` resource paths, and explicit read-only/non-fabrication guardrails; recorded measured `opencode 1.19.5` & `codex 0.169.0` context-discovery deltas and observed `$dtkit-project-implement` blocked-policy honesty trace; verified via `.\scripts\verify-docker.ps1`.
Remaining limitation / external blocker: Live baseline vs. with-kit repeated `T01`–`T10` LLM execution (`N >= 3`) requires an unblocked workspace shell sandbox (`codex` PS child execution allowed) and owner-approved LLM API credentials/budget. No speculative quality uplift percentages claimed.
Acceptance checklist result: Pass (kit template integrity, progressive disclosure, guardrail checks, 10-task benchmark suite, and honest evaluation boundaries established).
Next task: N10 — Cứng hóa release pipeline và trải nghiệm cài qua terminal (F09/F10).

Task: N10 — Release pipeline & trải nghiệm cài qua terminal
Status: verified (workflow/script hardening, `0.4.0-rc.1` archive build + checksum + Docker archive lifecycle smoke passed; native macOS/Windows `native-smoke` executes via GitHub Actions on dispatch/tag)
Branch / Commit: `feat/release-hardening` (pending N10 push; N09 CI `43909951887` checked alongside)
Artifact changed: `.github/workflows/release.yml`, `scripts/validate-release-args.ps1`, `scripts/build.ps1`, `scripts/package.ps1`, `internal/cli/release_workflow_test.go`, `docs/release.md`, `docs/progress.md`
Verification run: Negative `build.ps1` semver & `OutDir` traversal checks rejected malicious inputs (`"0.4.0;calc"`, `"../bad"`, `"../outside"`, `".git/hooks"`); `.\scripts\package.ps1 -Version 0.4.0-rc.1 -OutDir dist` generated 3 archives + `SHA256SUMS`; Docker smoke verified `sha256sum -c SHA256SUMS`, `VERSION`/`README.md`/`LICENSE`, `dtkit version --json` (`0.4.0-rc.1`), `info`, `install --project --yes`, `status --json` (`"healthy":true`), and `remove --yes`, followed by `dist/` cleanup and `.\scripts\verify-docker.ps1`.
Remaining limitation / external blocker: Proposed RC version `0.4.0-rc.1` is staged as a candidate workflow default (`publish_draft: false` unless owner approves creating a tag/draft release). Native `darwin_arm64` binary requires macOS runner (`native-smoke` matrix in `release.yml`).
Acceptance checklist result: Pass (shell injection fixed, strict semver/path validation enforced, clean RC archive & terminal checksum-verified install/uninstall flows documented and tested).
Next task: N11 — Tài liệu cuối, UX và chuẩn bị merge (`README.md`, `docs/cli.md`, `docs/release.md` changelog/troubleshooting, CLI consistency audit, PR summary).

Task: N11 — Tài liệu cuối, UX và chuẩn bị merge
Status: verified
Branch / Commit: `feat/release-hardening` (pending N11 push; N10 CI `43910935931` checked alongside)
Artifact changed: `docs/cli.md`, `README.md`, `docs/release.md`, `docs/progress.md`
Verification run: Verified live `go run ./cmd/dtkit help`, `help hooks`, `help doctor`, `internal/apperr/apperr.go` exit codes (`0/1/2/3/4/130`), git ancestry above `origin/main` (`f81ce6e`), and `git merge-tree --write-tree origin/main HEAD` (`6920268ae23c290f852d8f6df00866f996e9534f`, exit `0`, zero merge conflicts); synced `docs/cli.md`, `README.md`, and `docs/release.md` (changelog, troubleshooting, PR readiness).
Remaining limitation / external blocker: Merge into `main`, tag creation (`v0.4.0-rc.1`), and public GitHub Release publication remain gated on explicit repository owner approval.
Acceptance checklist result: Pass (docs, help text, exit codes, changelog, troubleshooting, and clean merge ancestry aligned).
Next task: N12 — Theo dõi sau phát hành: issue templates, `doctor --redact` / diagnostic redaction, support policy và incident procedure.

Task: N12 — Theo dõi sau phát hành & diagnostics (`--redact`, issue templates, support/incident policy)
Status: verified
Branch / Commit: `feat/release-hardening` (final N12 push pending)
Artifact changed: `internal/cli/cli.go`, `internal/cli/doctor.go`, `internal/cli/doctor_redact_test.go`, `.github/ISSUE_TEMPLATE/bug_report.yml`, `.github/ISSUE_TEMPLATE/runtime_compatibility.yml`, `.github/ISSUE_TEMPLATE/config.yml`, `docs/support.md`, `docs/cli.md`, `README.md`, `docs/release.md`, `docs/progress.md`
Verification run: Added `--redact` flag to `dtkit doctor` scrubbing home directories (`<HOME>`, Windows/POSIX slash variants), credential URLs, Bearer tokens, and `token=`/`api_key=` secrets across checks, warnings, hints, and error envelopes; added `TestDoctorRedactMasksHomeDirectoriesAndTokens`; added GitHub issue forms & support/incident policy (`docs/support.md`); verified cleanly via `.\scripts\verify-docker.ps1`.
Remaining limitation / external blocker: All mandatory release-hardening tasks (`N00`–`N12`) are complete. Post-release features (`N13`–`N18`), live A/B LLM benchmarks under unblocked workspace tooling, PR merge into `main`, and tagging/publishing `v0.4.0-rc.1` await repository owner authorization.
Acceptance checklist result: Pass (all mandatory release gates N00–N12 complete, documented, and verified).
Next task: Owner review & PR merge / release candidate decision (`N13`–`N18` post-release backlog deferred out of scope).

---

ID: A01–A04 (RC Gate Finalization on `feat/release-hardening`)
Status: partial (code, tests, fail-fast terminal installer scripts, A03 decision, and PR merge prep verified; remote tag/release run awaits owner merge/tag permission)
Baseline SHA / branch: `681529d` (`c100b86` CI `43912929680` & `681529d` CI `44002558059` passed 5/5 jobs) on `feat/release-hardening`
User-visible behavior and schema decisions: Recalibrated `N08`/`N09`/`N10` status labels truthfully (`partial` where live tasks, 60-run A/B evaluation, or remote tag release remain gated); recorded `A03` decision keeping existing Codex global layout with explicit `partial` boundary in RC; added fail-fast `scripts/install.sh` and `scripts/install.ps1` with real OS/arch rejection, single checksum tool selection, exact-1 `SHA256SUMS` validation, pre-extract hash verification, atomic rollback on locked/failed upgrades, and hook path preservation.
Files changed: `scripts/install.sh`, `scripts/install.ps1`, `.gitattributes`, `.github/workflows/release.yml`, `internal/cli/terminal_install_unix_test.go`, `internal/cli/terminal_install_windows_test.go`, `docs/verification/codex-global-windows-decision-2026-11-17.md`, `docs/compatibility.md`, `docs/release.md`, `README.md`, `docs/progress.md`
Tests completed: `GOOS=windows go test -c ./internal/cli`, `.\scripts\verify-docker.ps1` (including real `httptest.NewServer` HTTP tests for `scripts/install.sh` good install, hook-preserving upgrade, 404, checksum mismatch, missing/duplicate `SHA256SUMS`, unsupported OS/arch `Darwin/x86_64`, corrupt archive). Windows `terminal_install_windows_test.go` executes natively on `windows-latest` CI.
Acceptance still missing / external blocker: Owner approval for PR merge -> `main` and pushing/dispatching tag `v0.4.0-rc.1` on `.github/workflows/release.yml` (remote tags = `0`, GitHub Releases = `0`); live Codex/OpenCode/Gemini/Cursor task execution tracked under Mốc B (`v0.5`).
Next independent work: Branch `feat/v0.5` cleanly from `feat/release-hardening` to execute `B01` (Claude adapter) -> `B02` (repeatable benchmark runner & frozen baseline) -> `B03`–`B08`.

---

ID: B01 (Claude Code Adapter & Engineer `0.3.0` Baseline)
Status: partial (implemented and `verify-docker.ps1` passed; live task execution blocked on isolated `CLAUDE_CONFIG_DIR` auth & owner model budget)
Baseline SHA / branch: `84ad6ad` -> `feat/v0.5` (`buildinfo.Version` `"0.5.0-dev"`, `engineer` `"0.3.0"`)
User-visible behavior and schema decisions: Added `--runtime claude` (`CLAUDE.md` managed blocks, `.claude/skills/dtkit-claude-project-*`, `$CLAUDE_CONFIG_DIR` / `~/.claude` global resolution with `claude-home` binding, `.dtkit/runtimes/claude/state.json`); verified `CLAUDE_CONFIG_DIR` replaces `~/.claude` directly via live `claude 2.1.319 auth status`; bumped `engineer` `0.2.0` -> `0.3.0` adding `claude` target while keeping kit content frozen for `B02` baseline; updated package/state JSON schemas and CLI usage/help.
Files changed: `internal/buildinfo/buildinfo.go`, `internal/paths/paths.go`, `internal/paths/claude_test.go`, `internal/adapter/adapter.go`, `internal/adapter/claude/render.go`, `internal/adapter/claude/render_test.go`, `internal/adapter/engineer_eval_test.go`, `internal/catalog/catalog.go`, `internal/catalog/engineer/package.json`, `schemas/package.schema.json`, `schemas/state.schema.json`, `internal/cli/cli.go`, `internal/cli/runtime.go`, `internal/cli/claude_test.go`, fixture version updates in `internal/cli/{capabilities,cli,e2e}_test.go` & `internal/installer/{compat,crash,runtime}_test.go`, `docs/verification/claude-2.1.319-2026-11-17.md`, `docs/progress.md`
Tests completed: `claude --version` (`2.1.319`), isolated `CLAUDE_CONFIG_DIR` `claude auth status` probe, `.\scripts\verify-docker.ps1` exited `0` (all 17 packages pass including 5-runtime coexistence, monorepo/worktree resolution, and user `.claude/settings.json` preservation).
Live evidence: `claude auth status` confirmed `CLAUDE_CONFIG_DIR` isolation (`loggedIn: false`); real `~/.claude` remains untouched and no unbudgeted model tokens were spent.
Acceptance still missing / external blocker: Live isolated `claude --print` read+edit+test fixture execution (needs owner-approved `ANTHROPIC_API_KEY` or isolated auth + `--max-budget-usd` cap).
Next independent work: Fix `windows-latest` CI on `feat/release-hardening` (`84ad6ad`), then implement `B02` (repeatable benchmark runner with fake runtime tests + 10 non-DTKit multi-stack fixture repos) and `B03` (`dtkit project inspect/init`).

---

ID: B02 (Repeatable Multi-Stack Evaluation Harness & Frozen `0.3.0` Baseline)
Status: partial (harness, isolated `CLIExecutor`, 3 multi-stack fixture repos, 10 held-out + 2 dev scenarios, frozen `engineer@0.3.0` manifest, and unit/Go-evaluator tests verified; 60-run live model comparison awaits owner-budgeted isolated runtime auth)
Baseline SHA / branch: `aceddd4` + `d7cc1ec` merge on `feat/v0.5`
User-visible behavior and schema decisions: Created `internal/eval` (`Manifest`, `Scenario`, `ExecRequest`/`ExecResult`, `RunRecord`, `SuiteSummary`, `AgentExecutor`, `FakeExecutor`, `CLIExecutor`, `PlanPairs`, `RunTrial`, `SummarizeSuite`) with strict snapshot-only workspace setup, post-run private evaluator overlay, `TestTampering`/`ScopeViolation`/`FalseCompletion` detection, JSON `null` for missing token/cost metrics, `environment_blocked`/`timeout`/`cancelled`/`not_run` classification, and frozen `engineer@0.3.0` baseline manifest (`432c742e12f7f24ecd9ad8e6ac8fc8f787a76d3ea7094d351625db17059e844e`).
Files changed: `internal/eval/types.go`, `internal/eval/runner.go`, `internal/eval/executor.go`, `internal/eval/eval_test.go`, `testdata/evaluation/{go-service,ts-web,python-cli}/...`, `docs/evaluation/{README.md,manifest.json,pilot-report-2026-11-17.md}`, `docs/progress.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (all tests in `internal/eval` including private Go evaluator pass/fail, tampering, read-only violation, isolated `CLIExecutor` fail-closed checks, and `manifest.json` anti-leak + `engineer@0.3.0` digest verification).
Live evidence: `docs/evaluation/pilot-report-2026-11-17.md` documents verified Go fixture evaluation inside `golang:1.27.4` Docker; live model suite explicitly marked `blocked` (zero fabricated runs, zero host auth leakage).
Acceptance still missing / external blocker: Live isolated `claude`/`codex` 60-run baseline vs. with-kit comparison requires owner-provided isolated credentials and model budget cap; `windows-latest` CI failure on `feat/release-hardening` (`d7cc1ec`) tracked for immediate resolution.
Next independent work: Fix `windows-latest` CI on `feat/release-hardening` and merge into `feat/v0.5`, then implement `B03` (`dtkit project inspect` & `dtkit project init` with strict `.dtkit/project.json` schema v1).

---

ID: B03 (Project Profile Schema & `dtkit project inspect|init`)
Status: verified (local Docker `verify-docker.ps1`; CI pending push)
Baseline SHA / branch: `e5b4b0f` on `feat/v0.5`
User-visible behavior and schema decisions: Added strict Project Profile v1 schema ([`schemas/project.schema.json`](../schemas/project.schema.json), ADR in [`docs/adr-project-profile.md`](./adr-project-profile.md)) forbidding unknown fields, absolute/traversal paths, shell wrappers (`sh -c`/`cmd /c`/`powershell`), and secret env vars, while computing canonical `CommandsTrustDigest` over `checks`. Added `dtkit project inspect [--path] [--json]` (strictly read-only discovery across Go `go.work`/`go.mod`, Node/TS `package.json` workspaces & lockfiles, Python `pyproject.toml`, `Makefile`, and `.github/workflows/*.yml`) and `dtkit project init [--path] [--dry-run|--yes] [--json]` (never overwrites existing `.dtkit/project.json`).
Files changed: `schemas/project.schema.json`, `schemas/embed_test.go`, `docs/adr-project-profile.md`, `internal/project/{schema,inspect,init,project_test}.go`, `internal/cli/{cli,envelope,project,project_test}.go`, `docs/progress.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (`internal/project` malformed/security/monorepo/symlink/no-execute-canary tests, `internal/cli` `TestCLIProjectInspectAndInitWorkflow`, `schemas` 4-schema validation).
Live evidence: Local read-only inspection confirmed zero script execution against canary `Makefile` target and non-clobber preservation of user-edited `.dtkit/project.json`.
Acceptance still missing / external blocker: Multi-OS CI run for `B03` commit (`windows-latest` installer fix also queued on `feat/release-hardening`).
Next independent work: Harden `TestTerminalInstallScriptWindows` on `feat/release-hardening` & verify 5/5 CI, then execute `B04` (`engineer` `0.4.0` repository-neutral content revision).

---

ID: B04 (Engineer Kit `0.4.0` Repository-Neutral & Project-Profile-Aware Revision)
Status: partial (kit content revised to `0.4.0`, repo-neutrality & `0.3.0 -> 0.4.0` upgrade tests passing locally; live 60-run A/B model comparison `0.3.0` vs `0.4.0` remains gated on owner auth/budget)
Baseline SHA / branch: `a726efd` + `76445cc` merge on `feat/v0.5`
User-visible behavior and schema decisions: Bumped `engineer` `0.3.0` -> `0.4.0`. Removed DTKit-internal examples (`internal/apperr`, `AppendBlock`, `ReplaceBlock`, `RemoveBlock`, `AGENTS.md`, `Docker`) from `skills/{ask,explain,review,test}/SKILL.md.tmpl`; updated `rules/engineering.md`, `skills/test/SKILL.md.tmpl`, and `templates/review-report.md` with `.dtkit/project.json` discovery, targeted-read context discipline, untrusted-repo-text guards, and review checklist while preserving `< 8 KiB` always-loaded instruction footprint. Frozen baseline manifest in `docs/evaluation/manifest.json` stays pinned at `engineer@0.3.0`.
Files changed: `internal/catalog/engineer/{package.json,rules/engineering.md,skills/ask/SKILL.md.tmpl,skills/explain/SKILL.md.tmpl,skills/review/SKILL.md.tmpl,skills/test/SKILL.md.tmpl,templates/review-report.md}`, `internal/adapter/engineer_eval_test.go`, `internal/eval/eval_test.go`, `internal/installer/compat_test.go`, `docs/progress.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (validates template rendering across all 5 runtimes x 2 scopes, `< 8 KiB` instruction budget, repo-neutrality assertions, and `0.1.0`/`0.3.0` -> `0.4.0` upgrades across Codex & Claude).
Live evidence: Offline invariant & upgrade checks verified; live A/B comparison `0.3.0` vs `0.4.0` awaits isolated credentials + model budget (`B02` gate).
Acceptance still missing / external blocker: Live isolated multi-stack A/B comparison (`engineer@0.3.0` vs `engineer@0.4.0`); multi-OS CI run for `B04`.
Next independent work: Implement `B05` (`dtkit check --path <repo> --dry-run|--changed|--all --json`) on `feat/v0.5`.

---

ID: B05 (Independent Quality Runner `dtkit check`)
Status: verified (local Docker `verify-docker.ps1`; CI pending push)
Baseline SHA / branch: `f44acff` on `feat/v0.5`
User-visible behavior and schema decisions: Added `internal/check` and `dtkit check [--path <dir>] [--changed|--all] [--dry-run] [--yes] [--allow-autofix] [--json]` consuming `.dtkit/project.json`. Enforces explicit trust confirmation in `.dtkit/local/check-trust.json` (invalidated automatically when `checks` change in `.dtkit/project.json`), fast-to-slow ordering (`format -> lint -> typecheck -> build -> test -> custom`), strict argv-only execution (no shell), filtered environment, Git porcelain `-z` monorepo changed-root mapping with explicit non-Git/shared-config fallback, process-tree timeout termination, 64 KiB output cap, overlap lock (`.dtkit/local/check.lock`), and explicit non-pass statuses (`pass`, `fail`, `missing_tool`, `timeout`, `cancelled`, `skipped`).
Files changed: `internal/check/{changed,process_unix,process_windows,runner,trust,check_test}.go`, `internal/cli/{check,check_test,cli,envelope}.go`, `docs/progress.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (verifying `missing_project_profile`, `untrusted_project_checks` invalidation, Go pass, exit-code fail, `missing_tool` failure, 64 KiB output truncation, 1s process-tree kill, autofix gate, overlap lock, and Git monorepo `--changed` filtering).
Live evidence: Local Docker Go + Python check execution verified real command execution, process-tree cleanup, and JSON envelope output.
Acceptance still missing / external blocker: Multi-OS CI run for `B05`.
Next independent work: Implement `B06` (opt-in Claude Code `.claude/settings.json` quality hooks + `dtkit hook run check-changed`).

---

ID: B06 (Opt-In Claude Code Quality Hooks)
Status: partial (unit, structural merge/unmerge, recursion/debounce/trust/lock/guardrail tests & local `verify-docker.ps1` passing; live Claude Code CLI hook trigger execution remains gated on isolated OAuth/API key)
Baseline SHA / branch: `d8468ce` on `feat/v0.5`
User-visible behavior and schema decisions: Enabled `HooksSupported: true` for `claude` (`HookProjectPath: ".claude/settings.json"`, `HookGlobalRoot: "claude-home"`, `HookGlobalPath: "settings.json"`). Generalized `internal/hooks/merge.go`, `internal/hooks/command.go`, `internal/state/state.go`, `schemas/state.schema.json`, `internal/installer/hooks.go`, and `internal/cli/hooks.go` across `codex` and `claude`. Added `PostToolUse` (`check-changed` on `Edit|Write|MultiEdit` calling `dtkit check --changed` with `DTKIT_HOOK_ACTIVE=1` loop prevention, 2s debounce window, lock safety, non-bypassable `.dtkit/local/check-trust.json` verification, and 2 KiB output cap) and optional `PreToolUse` (`guard-edit` blocking secret `.env*`, `.dtkit/local/`, `.git/`, and traversal edits).
Files changed: `internal/adapter/adapter.go`, `internal/check/runner.go`, `internal/hooks/{merge,command,claude_runner,claude_hooks_test}.go`, `internal/state/state.go`, `schemas/state.schema.json`, `internal/installer/hooks.go`, `internal/cli/{cli,hooks,claude_hooks_test}.go`, `docs/progress.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (testing `.claude/settings.json` merge/unmerge preserving `permissions`/`mcpServers`/`env` and user hooks, `DTKIT_HOOK_ACTIVE=1`, 2s debounce, untrusted-check reminder vs trusted execution, `guard-edit` deny cases, `$CLAUDE_CONFIG_DIR/settings.json` global lifecycle, and clean `remove` unmerge).
Live evidence: Offline structural settings merge, CLI hook handlers, and rollback invariants verified; live Claude Code interactive hook firing awaits isolated authenticated Claude session (`claude auth status` unauthenticated in isolated `CLAUDE_CONFIG_DIR`).
Acceptance still missing / external blocker: Live `claude` tool-use session firing `.claude/settings.json` hook end-to-end (needs isolated Claude auth + owner model budget); multi-OS CI run for `B06`.
Next independent work: Implement `B07` (`--preset minimal|review-only|engineering` & `--components` selection with state migration/update).

---

ID: B07 (Kit Profiles & Selective Component Install `--preset` / `--components`)
Status: verified (local Docker `verify-docker.ps1`; CI pending push)
Baseline SHA / branch: `521ce83` on `feat/v0.5`
User-visible behavior and schema decisions: Added optional `"presets"` in `schemas/package.schema.json` and `internal/catalog/engineer/package.json` (`minimal`, `review-only`, `engineering`) and optional `"selection"` (`preset`, `requestedComponents`, `resolvedComponents`) in `schemas/state.schema.json` and `internal/state/state.go`. Added dependency-closure resolution (`catalog.ResolveComponentSelection`, `catalog.SelectionsEqual`), selective rendering (`adapter.RenderForRuntimeSelection`, preserving full `SkillNames` lookup context so partial presets never fail missingkey templates), transactional switch/preservation in `dtkit install` & `dtkit update` (removing orphaned artifacts cleanly and preserving `Selection` on subsequent `dtkit update`), and `dtkit status` reporting of active selection.
Files changed: `schemas/{package,state}.schema.json`, `internal/catalog/{catalog.go,validation.go,engineer/package.json}`, `internal/adapter/render.go`, `internal/state/state.go`, `internal/installer/{install,update,status,selection_test}.go`, `internal/cli/{cli,envelope,install,runtime,status,update,selection_test}.go`, `docs/progress.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (preset closure validation for `minimal`/`review-only`/`engineering`, cross-runtime partial rendering, preset switch add/remove lifecycle, crash rollback during switch, legacy state upgrade with component filter, and CLI `--preset`/`--components` validation, preservation across version bump `0.4.0 -> 0.5.0`, and switching).
Live evidence: Offline schema, adapter, transaction engine, and CLI selection tests passing locally in `golang:1.27.4` Docker.
Acceptance still missing / external blocker: Multi-OS CI run for `B07` (`windows-latest` installer fix also queued on `feat/release-hardening`).
Next independent work: Fix `TestTerminalInstallScriptWindows` cleanup on `feat/release-hardening` for 5/5 CI, merge into `feat/v0.5`, then run `B08` final readiness & docs truthfulness pass.

---

ID: B08 (v0.5 Final Readiness, Docs & Evidence Reconciliation)
Status: verified (offline/CI 5/5 green on `feat/v0.5` `2512004`; live model A/B & live Claude session hooks blocked on `401` expired auth)
Baseline SHA / branch: `fc76ac8` -> `2512004` on `feat/v0.5`
User-visible behavior and schema decisions: Synced `README.md`, `docs/cli.md`, `docs/compatibility.md`, `docs/progress.md`, and `.github/workflows/release.yml` to match the full v0.5 CLI & schema surface (`claude` runtime, `.claude/settings.json` hooks `check-changed`/`guard-edit`, `dtkit project inspect|init`, `dtkit check`, `engineer@0.4.0`, and `--preset`/`--components` selection). Fixed cross-OS CI failures (`Get-FileHash` replaced by pure .NET `SHA256` in `scripts/install.ps1`, `LOCALAPPDATA`/`APPDATA`/`SYSTEMDRIVE`/`COMSPEC`/`GOTOOLCHAIN` preserved in `internal/check/runner.go`, POSIX+Windows absolute/path-separator executable rejection in `internal/project/schema.go`, fast `go version` hook test in `internal/hooks/claude_hooks_test.go`).
Files changed: `README.md`, `docs/{cli,compatibility,progress}.md`, `.github/workflows/release.yml`, `scripts/install.ps1`, `internal/check/runner.go`, `internal/project/{schema,project_test}.go`, `internal/hooks/claude_hooks_test.go`, `internal/cli/terminal_install_windows_test.go`
Tests completed: `.\scripts\verify-docker.ps1` (exit `0`, all 20 packages pass with `-race`), Windows 11 PowerShell `.NET SHA256` `install.ps1` end-to-end check (`NET_SHA256_OK=True`), and GitHub Actions Run `44018736691` passing all **5/5** jobs (`windows-latest`, `ubuntu-latest`, `macos-latest`, `race`, `fuzz-smoke`).
Live evidence: Attempted capped isolated live `claude 2.1.319` run (`-p --no-session-persistence --setting-sources project --strict-mcp-config --max-budget-usd 0.30`) against `testdata/evaluation/go-service` + `engineer@0.4.0` + `.dtkit/project.json`; failed closed cleanly with `api_error_status: 401 ("Invalid or expired API key")`, `total_cost_usd: 0`.
Acceptance still missing / external blocker: Live isolated `claude`/`codex` 60-run A/B benchmark (`engineer@0.3.0` vs. `engineer@0.4.0`) and live Claude hook session firing await valid authenticated runtime credentials & budget.
Next independent work: Merge `feat/v0.5` into `main` and push `v0.5.0-rc.1` tag to trigger `release.yml` pre-release packaging/publication per owner Phương án B approval.

---

ID: C01 & C04a (Multi-Kit State Registry v3 & Shell Completion)
Status: verified (`verify-docker.ps1` + GitHub Actions CI [Run 44025529720](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44025529720) 5/5 green)
Baseline SHA / branch: `b78a6b3` -> `feat/c01-multikit` (`9b5fe7b`)
User-visible behavior and schema decisions: Implemented [`docs/adr-multikit-v3.md`](./adr-multikit-v3.md) and [`schemas/state.schema.json`](../schemas/state.schema.json) `oneOf` v1/v2 `installation` and v3 `registryV3` (`kits` registry + shared top-level `hooks`). Single-kit installs/updates/demotions keep v1/v2 JSON compatibility while 2+ installed kits in one `(runtime, scope)` promote atomically via `ExecuteTx` to `schemaVersion: 3` with byte-exact crash rollback (`state-before.bin`). Added `adapter.SkillDirName` (`<prefix><kitID>-<slug>` for non-`engineer` kits, keeping `<prefix><slug>` compatibility for `engineer`), cross-kit disjoint `whole-file` ownership enforcement (`artifact_owned_by_<kit>` in `planner.PlanDesiredWithPeersForRuntime`), shared `hooks` preservation when removing one kit while peers remain, and `installer.InspectRuntimeAll` (`dtkit status` / `dtkit doctor` listing all kits). Added `C04a` `dtkit completion <bash|zsh|powershell>`.
Files changed: `docs/adr-multikit-v3.md`, `schemas/state.schema.json`, `internal/state/{state,load,multikit_test}.go`, `internal/adapter/render.go`, `internal/planner/plan.go`, `internal/installer/{engine,install,update,remove,hooks,status,multikit_test}.go`, `internal/cli/{cli,completion,completion_test,doctor,install,status,update,dist_test,multikit_test}.go`, `docs/{cli,progress}.md`
Tests completed: `.\scripts\verify-docker.ps1`, `GOMAXPROCS=1 go test -count=20 ./internal/installer -run TestMultiKit`, and GitHub Actions CI Run `44025529720` (5/5 jobs `success`).
Acceptance still missing / external blocker: None for `C01`/`C04a`.
Next independent work: Implement `C02` (`dtkit diff`, `dtkit fork`, `--diff`/`--redact`).

---

ID: C02 (Preview Content Diff, Explain & Controlled Kit Fork)
Status: verified (`verify-docker.ps1` + GitHub Actions CI [Run 44034433728](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44034433728) 5/5 green)
Baseline SHA / branch: `9b5fe7b` -> `feat/c02-diff-fork` (`772b522`)
User-visible behavior and schema decisions: Added `planner.ExtractBlockForRuntime` (`internal/planner/block.go`) and `planner.ComputePlanDiff` / `planner.UnifiedDiff` (`internal/planner/diff.go`) generating bounded (`16 KiB`/file, `64 KiB` total), binary-safe unified diffs that isolate DTKit's own managed block without exposing user instructions outside managed blocks or unowned/conflicting files, and without auto-adopting direct drift edits. Added `dist.ForkKit` (`internal/dist/fork.go`) exporting verified kits into clean local directories (`package.json` with non-null slices, zero host/URL leakage, `duplicate_kit_id` guard unless `--same-id` with bumped semver, validated via `catalog.LoadDir` and multi-runtime render). Added `dtkit diff`, `dtkit fork`, `--diff`/`--redact` on `install` and `update`, and updated `dtkit completion` + `Envelope` (`Explain`, `Diff`, `Fork`).
Files changed: `internal/planner/{block,diff,diff_test}.go`, `internal/dist/{fork,fork_test}.go`, `internal/cli/{cli,completion,diff,diff_fork_test,doctor,envelope,fork,install,update}.go`, `docs/{cli,progress}.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (20/20 packages with `-race`), plus GitHub Actions CI Run `44034433728` (5/5 jobs `success`).
Acceptance still missing / external blocker: None for `C02`.
Next independent work: Implement `C03` (`dtkit.lock`, reproducible sync & trust policy).

---

ID: C03 (Portable `dtkit.lock`, Reproducible Sync & Ed25519 Trust Policy)
Status: verified (`verify-docker.ps1` + GitHub Actions CI [Run 44039514749](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44039514749) 5/5 green)
Baseline SHA / branch: `772b522` -> `feat/c03-lockfile` (`462ab49`)
User-visible behavior and schema decisions: Added [`docs/adr-lockfile-trust.md`](./adr-lockfile-trust.md) and [`schemas/lock.schema.json`](../schemas/lock.schema.json) (`schemaVersion: 1`) separating portable project declarations from machine-local state/journals. Rejects absolute/home/traversal paths, URL query secrets/userinfo, duplicate kits, and unknown fields (`dist.ParseAndValidateLockfile`). Added pure Go stdlib `crypto/ed25519` trust root & provenance helpers (`internal/dist/provenance.go`) supporting key generation, signing, multiple active keys (rotation), explicit `revoked` keys, RFC3339 `notAfter` expiration, `allowedKits` scoping, and verify-before-mutation (`VerifyProvenance`). Added `dtkit lock <generate|verify|diff|sign|keygen>` and `dtkit sync [--offline] [--require-signature] [--dry-run] [--diff]` installing/updating multi-kit locked projects deterministically from verified cache/relative/remote sources via `ExecuteTx`.
Files changed: `docs/adr-lockfile-trust.md`, `schemas/{lock.schema.json,embed_test.go}`, `internal/dist/{lockfile,lockfile_test,provenance}.go`, `internal/cli/{cli,completion,envelope,lock,lock_sync_test,sync}.go`, `docs/{cli,progress}.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (`20/20` packages with `-race`), plus GitHub Actions CI Run `44039514749` (5/5 jobs `success`).
Acceptance still missing / external blocker: None for `C03`.
Next independent work: Implement `C04` (`dtkit team export|import|template`, unified `dtkit status --all-runtimes`).

---

ID: C04 (Team Onboarding Bundles, Repo Template & Unified Cross-Runtime Inspect)
Status: verified (`verify-docker.ps1` + GitHub Actions CI [Run 44041946395](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44041946395) 5/5 green)
Baseline SHA / branch: `462ab49` -> `feat/c04-onboarding` (`70df858`)
User-visible behavior and schema decisions: Added [`docs/adr-team-onboarding.md`](./adr-team-onboarding.md) and [`schemas/team.schema.json`](../schemas/team.schema.json) (`schemaVersion: 1`) plus `dist.TeamBundle` (`internal/dist/team.go`) strictly excluding machine-local state, home bindings, backups, private keys, and `check-trust.json`. Added `dtkit team export`, `dtkit team import` (running capability, runtime, lockfile digest, provenance, and non-clobber `.dtkit/project.json` checks before mutating, never executing repo commands or auto-trusting checks), `dtkit team template` (scaffolding `dtkit.lock`, read-only `.dtkit/project.json`, and `.dtkit/ONBOARDING.md` without clobbering existing files), and `dtkit status --all-runtimes [--redact]`. Extracted shared `prepareSyncPlan` / `applySyncPlan` in `internal/cli/sync.go` so `sync` and `team import` share one verified transaction path.
Files changed: `docs/adr-team-onboarding.md`, `schemas/{team.schema.json,embed_test.go}`, `internal/dist/{team,team_test}.go`, `internal/cli/{cli,completion,doctor,envelope,status,sync,team,team_test}.go`, `docs/{cli,progress}.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (`20/20` packages with `-race`), plus GitHub Actions CI Run `44041946395` (5/5 jobs `success`).
Acceptance still missing / external blocker: None for `C04`.
Next independent work: Implement `C05` (`gemini`, `cursor`, and `opencode` runtime-native quality hook/plugin adapters).

---

ID: C05 (Multi-Runtime Quality Hook & Plugin Integrations: Gemini CLI, Cursor, OpenCode)
Status: partial (`verify-docker.ps1` passed 20/20; CI [Run 44115734821](https://github.com/truong-newbie/Agent_Kit_Impl/actions/runs/44115734821) 4/5 `success` with `macos-latest` cancelled in queue; live Gemini/Cursor/OpenCode runtime hook firing remains `unverified`/`blocked`)
Baseline SHA / branch: `70df858` -> `feat/c05-runtime-hooks` (`b68f159`)
User-visible behavior and schema decisions: Extended opt-in DTKit hook integrations to **Gemini CLI** (`.gemini/settings.json` / `~/.gemini/settings.json`, `AfterTool` `check-changed` and `BeforeTool` `guard-edit` with ms timeout and official `allow`/`deny` JSON), **Cursor** (`.cursor/hooks.json` / `~/.cursor/hooks.json` enforcing `"version": 1` on `afterFileEdit` `check-changed`), and **OpenCode** (standalone ESM JS plugin `.opencode/plugins/dtkit-quality.js` / `<opencode-home>/plugins/dtkit-quality.js` invoking `dtkit hook run check-changed --runtime opencode` via `execFileSync` argv array on `tool.execute.after`, preserving `opencode.json` and unrelated user plugins). Generalized `hooks.RunCheckChangedForRuntime`, `hooks.RunGuardEditForRuntime`, `schemas/state.schema.json`, `internal/state/state.go`, and `internal/installer/hooks.go` cleanly over `ExecuteTx` while keeping `.dtkit/local/check-trust.json`, `DTKIT_HOOK_ACTIVE=1`, 2s debounce, and project/global dedupe intact.
Files changed: `schemas/state.schema.json`, `internal/adapter/{adapter,adapter_test}.go`, `internal/state/state.go`, `internal/hooks/{command,merge,claude_runner,runtime_plugins,hardening_test,multiruntime_hooks_test}.go`, `internal/installer/hooks.go`, `internal/cli/{cli,gemini_test,hooks,hooks_multiruntime_test}.go`, `docs/{cli,compatibility,progress}.md`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (`20/20` packages with `-race`), including `TestGeminiCursorOpenCodeHooksAndSharedRunner` and `TestMultiRuntimeQualityHookLifecycles` verifying `AfterTool`/`BeforeTool`, `afterFileEdit` `version: 1`, `execFileSync` JS plugin rendering/tamper-fail/removal, project/global dedupe, and user settings preservation.
Acceptance still missing / external blocker: Live interactive hook firing in authenticated `gemini`, `cursor`, and `opencode` sessions (`gemini` and `cursor` CLI absent on host; `opencode` lacks provider credentials).
Next independent work: Implement `C06` (compatibility snapshot verification & performance regression budget).

---

ID: C06 (Periodic Compatibility Checks, Deprecation Policy & Cohort Performance Regression Budgets)
Status: verified offline/CI automation (local Docker `verify-docker.ps1`; GitHub Actions Run `44123231423` on `feat/c06-compat-perf` & Run `44125604359` on `main` 5/5 `success`; live model regression suite gated on private credentials/budget)
Baseline SHA / branch: `b68f159` -> `feat/c06-compat-perf` (`e3b4005`) -> merged fast-forward into `main`
User-visible behavior and schema decisions: Added offline compatibility snapshot & maintainer diff (`internal/adapter/compat.go`, `dtkit compat <snapshot|diff>`) capturing deterministic 5-runtime × 2-scope layouts, hook paths, capability matrix, and `engineer` rendered digests (`docs/verification/last-known-good-compat.json`), plus scheduled/manual GitHub Actions workflow [`.github/workflows/compat-periodic.yml`](../.github/workflows/compat-periodic.yml) emitting maintainer reports/artifacts without API keys or auto-upgrading adapters. Added per-`(modelId, runtime, kitId, kitVersion)` performance cohort aggregation, baseline-anchored `RegressionBudget` checks (`pass`, `regressed`, `insufficient_data`, refusing cross-cohort averaging), and `RedactLogText` (`internal/eval/perf.go`), plus runtime version support & N-1 deprecation policy ([`docs/compatibility-policy.md`](./compatibility-policy.md)).
Files changed: `internal/adapter/{compat,compat_test}.go`, `internal/eval/{perf,perf_test}.go`, `internal/cli/{cli,compat,compat_test,completion,envelope}.go`, `.github/workflows/compat-periodic.yml`, `docs/{cli,compatibility,compatibility-policy,progress}.md`, `docs/verification/last-known-good-compat.json`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (`20/20` packages with `-race`), `dtkit compat diff --baseline docs/verification/last-known-good-compat.json`, and GitHub Actions Runs `44123231423` & `44125604359` (5/5 jobs `success`).
Acceptance still missing / external blocker: Live model A/B regression run under private credentials/budget (`B02`/`N09` gate) and live `compat-periodic.yml` run on GitHub Actions default branch.
Next independent work: Cut `v0.6.0-rc.1` release tag, clean up merged `feat/*` branches, and switch GitHub repository default branch to `main` so scheduled workflows execute on `main`.

---

ID: D01–D04 (Core v0.7: Polyglot Scout, Guard & Verification Receipts, `engineer@0.5.0` Skill Mesh, STDIO MCP Server)
Status: verified offline/automated (`.\scripts\verify-docker.ps1` passing with `-race` across all packages)
Baseline SHA / branch: `01ab596` -> `main` (`CORE-V07-PLAN.md`)
User-visible behavior and schema decisions: Extended `internal/project` + `schemas/project.schema.json` to discover Java/Kotlin (`pom.xml`, `build.gradle*`), C/C++ (`CMakeLists.txt`), Rust (`Cargo.toml`), and .NET (`*.sln`, `*.csproj`) alongside Go/Node/TS/Python. Added `internal/scout` (`dtkit scout [--query] [--zone] [--pack] [--write-summary]`) for secret-redacted polyglot reconnaissance and `docs/codebase-summary.md` generation. Added `internal/guard` (`dtkit guard status|lock|unlock|verify`) implementing `.dtkit/local/guard.json` phase firewalls (`read-only`, `tdd-red`, `tdd-green`, `scoped` with `**` globs), polyglot test-tampering detection (`added_skip`, `removed_test_case`, `reduced_assertions`), cryptographic `.dtkit/local/verify-receipt.json` SHA-256 check proofs updated by `check.Run`, and 3+ failure retry-loop warnings in `hooks`. Upgraded `engineer` to `0.5.0` (19 components / 10 skills following `Brainstorm -> Scout -> Plan -> Implement -> Test -> Review -> Docs` + `documentation-management` rule while keeping `minimal` and `review-only` presets backward compatible). Added zero-dependency STDIO JSON-RPC 2.0 `internal/mcp` (`dtkit mcp serve`) exposing `dtkit_scout`, `dtkit_project_inspect`, `dtkit_guard_status`, and `dtkit_guard_verify`.
Files changed: `docs/{CORE-V07-PLAN,cli,progress}.md`, `docs/verification/last-known-good-compat.json`, `README.md`, `schemas/project.schema.json`, `internal/project/{inspect,schema,project_test}.go`, `internal/scout/{scout,scout_test}.go`, `internal/guard/{guard,tamper,receipt,guard_test}.go`, `internal/check/runner.go`, `internal/hooks/claude_runner.go`, `internal/mcp/{server,server_test}.go`, `internal/catalog/engineer/...`, `internal/cli/{cli,completion,envelope,guard,mcp,scout,core_v07_test,cli_test,diff_fork_test,selection_test}.go`, `internal/adapter/{codex,cursor,gemini,opencode}/render_test.go`, `internal/adapter/engineer_eval_test.go`, `internal/catalog/catalog_test.go`, `internal/dist/fork_test.go`, `internal/eval/eval_test.go`, `internal/installer/runtime_test.go`, `internal/planner/{install,plan}_test.go`
Tests completed: `gofmt` + `dtkit compat snapshot` + `.\scripts\verify-docker.ps1` (`go test -race ./...`, `go vet ./...`, `gofmt -l`, `go build ./cmd/dtkit`).
Acceptance still missing / external blocker: Live interactive MCP & hook firing inside live `claude`/`gemini`/`cursor`/`opencode` sessions remains gated on runtime CLIs/API credentials.
Next independent work: Proceed to Extension Milestone (`dtkit integrate` for community repos/tools like Superpowers, Repomix, Graphify) or run multi-OS GitHub Actions CI upon push.

---

ID: E01–E03 (Extensions v0.7: Universal Community Importer, External Companion Bridge & Cache, Auto-MCP Wiring & `dtkit init`)
Status: verified offline/automated (`.\scripts\verify-docker.ps1` passing with `-race` across all 23 test packages)
Baseline SHA / branch: `8baa09c` -> `main` (`EXTENSIONS-V07-PLAN.md`)
User-visible behavior and schema decisions: Added `dist.ImportKit` (`internal/dist/importer.go`) transpiling community skill/rule repositories (`skills/*/SKILL.md` such as Superpowers, `.claude/commands/*.md`, `.cursor/rules/*.mdc`, `.cursorrules`, `CLAUDE.md`/`AGENTS.md`, `workflows/*.md`, `templates/*.md`) into validated 5-runtime DTKit packages with bridge skill synthesis for rule-only packs. Added `internal/companion` (`repomix`, `graphify`, `ast-grep`, `semgrep`) with argv-only execution, `.dtkit/local/companion-cache/` SHA-256 workspace-digest caching, secret redaction, and automatic `dtkit scout` fallback when missing on `PATH`. Added `internal/mcp/config.go` (`dtkit mcp <status|enable|disable>`) merging/unmerging `dtkit mcp serve` entries across `.mcp.json` (`claude`), `.cursor/mcp.json` (`cursor`), `.gemini/settings.json` (`gemini`), `opencode.json` (`opencode`), and `.codex/mcp.json` (`codex`) while preserving peer MCP servers. Added `dtkit integrate <list|import|tool|run>` and one-command `dtkit init [--runtime|--all-runtimes] [--preset] [--with-mcp] [--with-hooks] [--lock]`.
Files changed: `docs/{EXTENSIONS-V07-PLAN,cli,progress}.md`, `README.md`, `internal/dist/{importer,importer_test}.go`, `internal/companion/{companion,companion_test}.go`, `internal/mcp/{config,config_test}.go`, `internal/cli/{cli,completion,envelope,init,integrate,mcp,extensions_v07_test}.go`
Tests completed: `gofmt` + `.\scripts\verify-docker.ps1` (`go test -race ./...`, `go vet ./...`, `gofmt -l`, `go build ./cmd/dtkit`).
Acceptance still missing / external blocker: Live interactive execution with external `repomix`/`graphify`/`ast-grep`/`semgrep` binaries when installed on user workstations, and live runtime sessions.
Next independent work: Push to `main`, verify 5/5 GitHub Actions CI, and tag `v0.7.0` release when owner approves.

---

ID: F01–F04 (v0.8 Refinements: Codex `.codex/config.toml` MCP, Remote Git Import + `--update`, Import/Include Impact Graph, Homebrew/Scoop Packaging)
Status: verified offline/automated (`.\scripts\verify-docker.ps1` passing with `-race` across all 23 test packages)
Baseline SHA / branch: `87fe69f` -> `main`
User-visible behavior and schema decisions: Switched Codex MCP wiring from `.codex/mcp.json` to `.codex/config.toml` (`# dtkit:begin mcp` ... `# dtkit:end mcp` managed block preserving user TOML lines & peer `[mcp_servers.*]` tables, rejecting unmanaged `[mcp_servers.dtkit]` conflicts). Extended `dist.ImportKit` and `dtkit integrate import` to accept HTTPS Git URLs and GitHub `owner/repo` shorthands (enforcing `ValidateRemoteURL` and shallow `git clone --depth 1` into temp dir) plus atomic `--update` replacement. Added `scout.AnalyzeImpact` (`internal/scout/impact.go`), `dtkit scout --impact <file>`, 5th MCP tool `dtkit_context_impact`, and hook `check-changed` `[impact dependents=[...] tests=[...]]` injection across Go (`go/parser`), TS/JS, Python, Java/Kotlin, and C/C++. Added `scripts/render-pkg-manifests.sh`, `packaging/homebrew/dtkit.rb`, `packaging/scoop/dtkit.json`, and automated release attachment generation in `.github/workflows/release.yml`.
Files changed: `.github/workflows/release.yml`, `README.md`, `docs/{cli,guide,guide.vi,progress}.md`, `packaging/{homebrew/dtkit.rb,scoop/dtkit.json}`, `scripts/render-pkg-manifests.sh`, `internal/mcp/{config,config_test,server,server_test}.go`, `internal/dist/{importer,importer_test}.go`, `internal/scout/{impact,impact_test}.go`, `internal/hooks/claude_runner.go`, `internal/cli/{cli,completion,envelope,integrate,scout,extensions_v07_test}.go`
Tests completed: `gofmt`, bash syntax & manifest generation dry check in Docker, and `.\scripts\verify-docker.ps1` (`go test -race ./...`, `go vet ./...`, `gofmt -l`, `go build ./cmd/dtkit`).
Acceptance still missing / external blocker: Live `codex` CLI execution of `.codex/config.toml` `[mcp_servers.dtkit]` and live GitHub tarball Homebrew/Scoop tap install.
Next independent work: Push to `main`, verify 5/5 GitHub Actions CI, and tag `v0.8.0` when owner approves.

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
