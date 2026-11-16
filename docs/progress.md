# DTKit implementation progress

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
