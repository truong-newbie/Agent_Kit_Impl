# Release Readiness Checklist (v0.1 Codex)

## Owner Decisions Confirmed

1. Canonical Go module path: `github.com/truong-newbie/Agent_Kit_Impl`.
2. Repository license: `MIT License` ([LICENSE](../LICENSE)).
3. Release target: merge `feat/dtkit-foundation` to `main` (public tag `v0.1.0` deferred to explicit release step).

## Implemented & Verified in Repository

- [x] Single-binary CLI with embedded `engineer@0.1.0` kit (`cmd/dtkit`, `internal/catalog`).
- [x] Full v0.1 command set: `version`, `help`, `list`, `info`, `validate`, `doctor`, `install`, `status`, `update`, `remove`, `recover`.
- [x] Unified JSON envelope and exit codes (`0`, `1`, `2`, `3`, `4`, `130`).
- [x] Project + global isolation (`AGENTS.md` managed blocks, namespaced `dtkit-project-*` / `dtkit-global-*` skills, `CODEX_HOME` binding).
- [x] Versioned transaction journal, PID/token lock liveness checks, conditional rollback, and `recover`.
- [x] Docker Linux verification (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`, bounded 3s fuzzing across 4 targets, built-binary E2E lifecycle) and GitHub Actions CI across `windows-latest`, `ubuntu-latest`, `macos-latest`, and `race` (run `43839922917`).
- [x] Cross-platform release packaging script (`scripts/package.ps1`) and workflow (`.github/workflows/release.yml`), verified with archive extraction smoke on Windows and Linux.

## Remaining Release Gates before Tagging `v0.1.0`

- [ ] Verify green CI on `main` after merge.
- [ ] Complete isolated global Codex smoke test (separate OS user account) and unblocked workspace tool execution smoke (see [codex-0.169.0-2026-11-16.md](verification/codex-0.169.0-2026-11-16.md)).
- [ ] Create and push tag `v0.1.0` when ready to publish release artifacts.
