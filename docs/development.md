# Development Guide

## Requirements

- Go `1.27.4` or newer (no third-party Go dependencies; stdlib only).
- On Windows hosts where Application Control / Device Guard blocks unsigned `go.exe`, use Docker Desktop with `golang:1.27.4` via `.\scripts\verify-docker.ps1`.

## Local Verification

```powershell
# With Docker Desktop on Windows:
.\scripts\verify-docker.ps1

# With an approved native Go toolchain:
gofmt -l cmd internal schemas
go test -race ./...
go vet ./...
go build ./cmd/dtkit
```

## Build & Release Packaging

```powershell
# Build Windows, Linux, and macOS binaries into dist/
.\scripts\build.ps1 -Version 0.1.0

# Build + package zip/tar.gz archives and SHA256SUMS into dist/
.\scripts\package.ps1 -Version 0.1.0
```

## Testing Rules

- Never mutate the developer's real `HOME`, `USERPROFILE`, or `~/.codex` / `~/.agents` in automated tests.
- All filesystem tests use `t.TempDir()`, injected `paths.Environment`, and per-call `installer.TxHooks` instead of global mutable test seams.
