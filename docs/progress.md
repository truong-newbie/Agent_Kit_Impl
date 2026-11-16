# DTKit implementation progress

## M0 — Audit

Status: in-progress

- Workspace initially contained only the three planning documents and no Git repository.
- Git was available; Go and Codex CLI were not on PATH. The official Go 1.27.4 archive was downloaded into `.tools/` and its SHA-256 matched the official release checksum. Windows Application Control blocked running the extracted `go.exe`, both in the sandbox and outside it. The toolchain is excluded from Git.
- The GitHub remote returned no refs on inspection.
- Actual Codex runtime smoke testing requires an independently available CLI and authenticated test environment.

## M1–M3 — CLI, catalog and content

Status: in-progress

- Started a Go module, read-only CLI, embedded Engineer Kit, component validation and content. Added a pure Codex renderer and scope/determinism test.
- Basic tests cover catalog structure, missing/cyclic dependencies and CLI behavior.
- The CLI is intentionally incomplete: installation, update, status, remove, dry-run, state and recovery do not yet exist.
- Manifest/schema validation and local package safety require further work before treating packages as untrusted input.
- Build, test, vet and runtime verification have not run because Windows Application Control blocked Go. A CI workflow is provided for an independent runner but its results are not yet known.
- Next: use an approved Go toolchain or inspect CI, fix any build/test errors, implement strict manifest and path validation, then finish the Codex adapter and transaction-safe installer as described in the plan.

## M4–M12

Status: in-progress (M4 renderer only; M5–M12 not started)
