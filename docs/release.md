# Release Readiness Checklist (v0.1 Codex)

## Pre-Release Decisions Required from Owner

Before tagging or publishing a public release, the repository owner must confirm:
1. Canonical Go module path (`dtkit.local/dtkit` vs `github.com/truong-newbie/Agent_Kit_Impl` in `go.mod` and imports).
2. Official repository license file (`LICENSE`).
3. Release visibility (public vs internal) and target OS/architecture support commitments.

## Implemented & Verified in Repository

- [x] Single-binary CLI with embedded `engineer@0.1.0` kit (`cmd/dtkit`, `internal/catalog`).
- [x] Full v0.1 command set: `version`, `help`, `list`, `info`, `validate`, `doctor`, `install`, `status`, `update`, `remove`, `recover`.
- [x] Unified JSON envelope and exit codes (`0`, `1`, `2`, `3`, `4`, `130`).
- [x] Project + global isolation (`AGENTS.md` managed blocks, namespaced `dtkit-project-*` / `dtkit-global-*` skills, `CODEX_HOME` binding).
- [x] Versioned transaction journal, PID/token lock liveness checks, conditional rollback, and `recover`.
- [x] Docker Linux verification (`gofmt`, `go test -race ./...`, `go vet ./...`, `go build`, bounded 3s fuzzing across 4 targets, built-binary E2E lifecycle).
- [x] Cross-platform release packaging script (`scripts/package.ps1`) and workflow (`.github/workflows/release.yml`), verified with archive extraction smoke on Windows and Linux.

## Remaining Release Gates

- [ ] Push `feat/dtkit-foundation` and confirm green CI across `windows-latest`, `ubuntu-latest`, and `macos-latest` (plus `race` job) for the release candidate SHA.
- [ ] Complete isolated global Codex smoke test and unblocked Windows/Linux Codex file-read verification (see [codex-0.169.0-2026-11-16.md](verification/codex-0.169.0-2026-11-16.md)).
- [ ] Owner confirmation of module path, `LICENSE`, and release publication.
