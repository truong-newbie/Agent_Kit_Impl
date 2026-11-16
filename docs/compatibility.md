# DTKit Runtime Compatibility and Evidence Matrix

> **Honesty rule:** Automated tests and runtime discovery do **not** equal end-to-end live task verification. Until `N08` and `N09` pass on real runtimes with real tool/model execution, no runtime/scope below is labeled `verified (live task)`.

## 1. Runtime × Scope Four-Layer Evidence Matrix

| Runtime | Scope | Rendered Artifacts | CLI Lifecycle Tests | Runtime Discovery Smoke | Live Model Task Execution | Status / Fallback Notes |
|---|---|---|---|---|---|---|
| Codex CLI (`0.169.0`) | `project` | Pass (`AGENTS.md` block, `.agents/skills/dtkit-project-*`, `.dtkit/content/engineer/`) | Pass (Linux `-race`, built-binary E2E, Windows/macOS CI) | **Pass** (`codex debug prompt-input` & `codex exec` discovered all 7 skills + `AGENTS.md` + `$dtkit-project-implement`; [codex-0.169.0-2026-11-16.md](verification/codex-0.169.0-2026-11-16.md)) | **Partial / Blocked** (host Windows sandbox policy rejected child `powershell.exe` commands cleanly; untrusted repo config warning observed) | **Partial.** Managed hooks supported via `.codex/hooks.json` (`--trust`/UI trust required) |
| Codex CLI (`0.169.0`) | `global` | Pass (`$CODEX_HOME/AGENTS.md`, `~/.agents/skills/dtkit-global-*`, `~/.dtkit/content/engineer/`) | Pass (isolated fake `HOME` & `CODEX_HOME`) | **Partial** (`codex debug prompt-input` loaded isolated `$CODEX_HOME/AGENTS.md`; Windows `FOLDERID_Profile\.agents\skills` requires separate OS user account) | Pending (sandboxed PS & OS profile isolation) | **Partial.** Uses `codexHome` binding in `.dtkit/state.json` |
| Gemini CLI | `project` | Pass (`GEMINI.md` block, `.gemini/skills/dtkit-gemini-project-*`, `.dtkit/runtimes/gemini/content/engineer/`) | Pass (Linux `-race`, 4-runtime coexistence & rollback tests, Windows/macOS CI) | Blocked (`gemini` CLI absent on host — [gemini-cursor-external-blocker-2026-11-16.md](verification/gemini-cursor-external-blocker-2026-11-16.md)) | Pending (`gemini` CLI absent) | **Experimental / `unverified`.** Inspects `.gemini/settings.json` `context.fileName`; uses `dtkit-gemini-project-*` |
| Gemini CLI | `global` | Pass (`$GEMINI_CLI_HOME/.gemini/GEMINI.md`, `$GEMINI_CLI_HOME/.gemini/skills/dtkit-gemini-global-*`, `~/.dtkit/runtimes/gemini/content/engineer/`) | Pass (isolated fake `HOME` & `GEMINI_CLI_HOME`) | Blocked (`gemini` CLI absent on host) | Pending (`gemini` CLI absent) | **Experimental / `unverified`.** Uses `runtimeHome` binding in `.dtkit/runtimes/gemini/state.json` |
| OpenCode (`1.19.5`) | `project` | Pass (`AGENTS.md` block `engineer opencode project`, `.opencode/skills/dtkit-opencode-project-*`, `.dtkit/runtimes/opencode/content/engineer/`) | Pass (Linux `-race`, shared `AGENTS.md` multi-block & 4-way coexistence) | **Pass** (`opencode debug skill` + `debug config` discovered all 7 project skills + user skill + clean remove alongside Codex/Gemini/Cursor — [opencode-1.19.5-2026-11-16.md](verification/opencode-1.19.5-2026-11-16.md)) | Blocked (`0 credentials` in isolated `HOME`; live `opencode run` not executed) | **Partial.** `N04` shared `AGENTS.md` lock parity verified |
| OpenCode (`1.19.5`) | `global` | Pass (`$OPENCODE_CONFIG_DIR/AGENTS.md` or `~/.config/opencode/AGENTS.md`, `<opencode-home>/skills/dtkit-opencode-global-*`, `~/.dtkit/runtimes/opencode/content/engineer/`) | Pass (isolated `OPENCODE_CONFIG_DIR`, `XDG_CONFIG_HOME`, `~/.config/opencode`) | **Pass** (`opencode debug skill` discovered global-only & combined 14 DTKit skills from empty/project cwds cleanly — [opencode-1.19.5-2026-11-16.md](verification/opencode-1.19.5-2026-11-16.md)) | Blocked (`0 credentials` in isolated `HOME`) | **Partial.** Uses `runtimeHome` binding in `.dtkit/runtimes/opencode/state.json` |
| Cursor | `project` | Pass (`.cursor/rules/dtkit-engineer.mdc`, `.cursor/skills/dtkit-cursor-project-*`, `.dtkit/runtimes/cursor/content/engineer/`) | Pass (Linux `-race`, 4-way coexistence & `.mdc` preservation tests) | Blocked (`cursor` CLI/IDE absent on host — [gemini-cursor-external-blocker-2026-11-16.md](verification/gemini-cursor-external-blocker-2026-11-16.md)) | Pending (`cursor` CLI absent) | **Experimental / `unverified`.** `.cursor/rules/*.mdc` whole-file rule avoids colliding with `AGENTS.md` or user rules |
| Cursor | `global` | Pass (`~/.cursor/skills/dtkit-cursor-global-*`, `~/.dtkit/runtimes/cursor/content/engineer/`; no global instruction file emitted) | Pass (isolated fake `HOME`, fallback warning emitted) | Blocked (`cursor` CLI/IDE absent on host) | Pending (`cursor` CLI absent) | **Experimental / `unverified` (Fallback).** Global User Rules live in Cursor Settings UI (`instructions` has `runtimeSupported: true, dtkitSupported: false`); capability contract enforced (`N05`) |

## 2. Feature × Runtime Support Matrix

| Feature | Codex | Gemini CLI | OpenCode | Cursor | Notes |
|---|---|---|---|---|---|
| Project instructions | DTKit-managed (`AGENTS.md`) | DTKit-managed (`GEMINI.md`) | DTKit-managed (`AGENTS.md` namespaced block) | DTKit-managed (`.cursor/rules/dtkit-*.mdc`) | Only Codex checks `AGENTS.override.md`; Gemini checks `.gemini/settings.json` |
| Global instructions | DTKit-managed (`$CODEX_HOME/AGENTS.md`) | DTKit-managed (`~/.gemini/GEMINI.md`) | DTKit-managed (`~/.config/opencode/AGENTS.md`) | **UI-only fallback** (`runtimeSupported: true, dtkitSupported: false`) | DTKit warns clearly on Cursor global and installs skills/resources only (`N05` formalizes required vs optional capability enforcement) |
| Project & global skills | DTKit-managed (`.agents/skills`) | DTKit-managed (`.gemini/skills`, `~/.gemini/skills`) | DTKit-managed (`.opencode/skills`, `<opencode-home>/skills`) | DTKit-managed (`.cursor/skills`, `~/.cursor/skills`) | Non-Codex runtimes namespace skill IDs (`dtkit-<runtime>-<scope>-*`) to avoid collisions |
| Hooks / plugins | **DTKit opt-in `SessionStart`** (`.codex/hooks.json` / `$CODEX_HOME/hooks.json`) | Native `.gemini/settings.json` hooks (**not DTKit-managed**) | Native JS/TS `.opencode/plugins/` (**not DTKit-managed**) | Native `.cursor/hooks.json` (**not DTKit-managed**) | `dtkit hooks` explicitly targets `--runtime codex` only; cross-runtime hook management is post-release (`N16`) |
| Authoring / pack / remote / offline | Yes | Yes | Yes | Yes | Policy hardening (`dry-run` no-write, HTTP timeouts/redirects, strict index validation) tracked in `N02`–`N03` |

## 3. CI Skip Audit

Every conditional `t.Skip` / `t.Skipf` in the test suite has an explicit, documented boundary condition:

| Location | Skip Condition | Covered Elsewhere / Why Safe |
|---|---|---|
| `internal/cli/e2e_test.go` | `testing.Short()` | Full CI & `verify-docker.ps1` run without `-short`, executing built-binary E2E |
| `internal/cli/e2e_test.go` | Missing `go` binary on `PATH` | Only triggers for standalone precompiled test binaries; CI & Docker always provide Go |
| `internal/hooks/hardening_test.go` | Missing `powershell` on `PATH` on Windows | `windows-latest` CI runner provides `powershell.exe`; POSIX tests `sh -c` directly |
| `internal/installer/recover_test.go` | Helper exit command fails to launch | Standard OS fallback guard (`true` / `cmd /c exit 0`) |
| `internal/paths/paths_test.go` | Unprivileged `os.Symlink` denied on Windows | Compensated by `internal/paths/paths_windows_test.go` using unprivileged `cmd /c mklink /J` NTFS junctions, plus POSIX CI coverage |
| `internal/installer/native_test.go` | `os.Geteuid() == 0` in POSIX `0555` parent test | Containerized Linux runs as `uid=0` (bypasses DAC); non-root `ubuntu-latest` and `macos-latest` CI runners execute `0555` confinement live |
