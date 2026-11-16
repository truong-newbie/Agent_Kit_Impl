# DTKit implementation progress

## M0 — Audit

Status: in-progress

- Workspace initially contained only the three planning documents and no Git repository.
- Git was available; Go was not usable on PATH. The official Go 1.27.4 archive was downloaded into `.tools/` and its SHA-256 matched the official release checksum. Windows Application Control/Device Guard blocked both the extracted and MSI-installed `go.exe`. Docker Desktop with WSL2 is available and now provides the local Go toolchain without changing that security policy.
- The GitHub remote returned no refs on inspection.
- Actual Codex runtime smoke testing requires an independently available CLI and authenticated test environment.
- Rechecked environment on 2026-11-16: `codex.cmd --version` reports `codex-cli 0.169.0`. This only establishes executable availability, not that DTKit integration has been verified.

## M1–M3 — CLI, catalog and content

Status: in-progress

- Started a Go module, read-only CLI, embedded Engineer Kit, component validation and content. Added a pure Codex renderer, managed-block operations, fresh-install preview, deterministic package digest and installation-state validation.
- Basic tests cover catalog structure, missing/cyclic dependencies and CLI behavior.
- A guarded fresh install is now implemented behind `--yes`. It re-plans under an exclusive lock, journals preimages, preserves an existing `AGENTS.md`, writes state, and conditionally rolls back if an artifact or state write fails. Existing state is rejected by fresh install.
- `dtkit status --project|--global` is read-only: it validates the recorded state and reports missing or modified DTKit-owned bytes while ignoring user content outside the managed `AGENTS.md` block. Update, remove and recovery commands do not yet exist.
- Manifest/schema validation and local package safety require further work before treating packages as untrusted input.
- Local container verification on 2026-11-16 passed formatting, `go test ./...`, `go vet ./...`, and `go build ./cmd/dtkit` using `golang:1.27.4`. Native Windows execution remains blocked by Device Guard. GitHub Actions run 43827322269 passed `go test ./...`, `go vet ./...`, and `go build ./cmd/dtkit` on Windows, Linux and macOS for commit `6bce4dfde0643dd8b97e1621caf047309f3de1a5`.
- GitHub Actions runs 43827864444 and 43828078597 passed those checks for manifest and managed-block changes on all three operating systems. Run 43828303155 passed the same checks for the initial fresh-install preview, commit `c15eaca941b17247a21d3afbfe390654edaed97b`.
- Codex runtime verification has not run.
- Next: use an approved Go toolchain for iterative development, implement strict manifest and path validation, then finish the Codex adapter and transaction-safe installer as described in the plan.

## M4–M12

Status: in-progress (M4 implemented for fresh install; M5 fresh-install lock/journal/state/rollback plus read-only status/drift detection implemented and locally container-tested; update/recover and M6–M12 not complete)
