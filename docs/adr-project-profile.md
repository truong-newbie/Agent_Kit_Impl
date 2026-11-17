# ADR: Strict Project Profile Schema (`.dtkit/project.json`)

## Context

`engineer` v0.3–v0.4 and `dtkit check` need reliable, repository-local knowledge of workspace stacks, root directories, and verification commands without letting untrusted repos execute arbitrary shell strings or leak machine-specific paths/credentials into version control.

## Decision

1. **Workspace-relative configuration only:** `.dtkit/project.json` stores `schemaVersion: 1`, `roots`, `checks`, `include`, `exclude`, `generatedDirs`, and static `evidence`. No user-home paths, credentials, tokens, or transient run logs are permitted.
2. **Structured `executable` + `argv` commands:** Every check declares `executable`, explicit `argv` array, relative `cwd`, bounded `timeoutSec`, `allowEnv`, and `mutatesFiles`. Shell-wrapping invocations (`sh -c`, `bash -c`, `cmd /c`, `powershell -Command`) are rejected by schema validation to prevent string-interpolation injection.
3. **Read-only discovery, explicit opt-in write:** `dtkit project inspect` reads manifests (`go.work`, `go.mod`, `package.json`, `pnpm-workspace.yaml`, `pyproject.toml`, `Makefile`, `.github/workflows/*.yml`) without executing any scripts or package managers. `dtkit project init` requires `--yes` (after `--dry-run` preview) and refuses to overwrite an existing `.dtkit/project.json`.
4. **Trust invalidation digest:** `CommandsTrustDigest` computes a SHA-256 digest over canonicalized `checks` entries so `dtkit check` (`B05`) detects modified repo check commands and requires re-trust before execution.
