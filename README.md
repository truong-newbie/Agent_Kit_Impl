# DTKit

DTKit is a Go-based toolkit for reusable AI engineering skills. The first target is Codex, with project and global installation planned.

This is an early development snapshot, not a usable installer. The CLI provides `version`, `help`, `list`, `info`, basic `validate`, a Codex presence check in `doctor`, and a read-only fresh-install preview. Validation is incomplete and must not be used as a security check for untrusted packages. Actual install, update, status, remove, recovery, hooks and runtime verification are not implemented yet. Do not use this snapshot to modify agent configuration.

## Build and test

Requires Go 1.27 or newer.

```powershell
go test ./...
go run ./cmd/dtkit list
go run ./cmd/dtkit info engineer
go run ./cmd/dtkit install engineer --project --dry-run
```

If Windows Device Guard blocks the Go executable, keep the policy enabled and
run the same checks through Docker Desktop:

```powershell
.\\scripts\\verify-docker.ps1
```

The development roadmap and safety requirements are in [DTKIT-IMPLEMENTATION-PLAN.md](DTKIT-IMPLEMENTATION-PLAN.md). Original product notes are in [ai-agent-toolkit-spec.md](ai-agent-toolkit-spec.md) and [AI-Engineering-Kit-Global.md](AI-Engineering-Kit-Global.md).
