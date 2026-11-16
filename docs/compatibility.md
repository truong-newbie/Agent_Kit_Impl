# DTKit Runtime Compatibility

| Runtime | Scope | Capabilities | Automated Tests | Runtime Verification Status | Notes |
|---|---|---|---|---|---|
| Codex CLI (`0.169.0`) | `project` | `instructions` (`AGENTS.md`), `skills` (`.agents/skills/dtkit-project-*`), resources (`.dtkit/content/engineer/`) | Pass (Linux Docker race + built-binary E2E; Windows/macOS via CI) | Partial (`codex-cli 0.169.0` discovered skills/instructions in [codex-0.169.0-2026-11-16.md](verification/codex-0.169.0-2026-11-16.md); sandboxed PS read blocked by host policy) | Hooks deferred to v0.2 |
| Codex CLI (`0.169.0`) | `global` | `instructions` (`$CODEX_HOME/AGENTS.md`), `skills` (`~/.agents/skills/dtkit-global-*`), resources (`~/.dtkit/content/engineer/`) | Pass (isolated fake `HOME` & `CODEX_HOME`) | Pending (requires isolated OS user account) | Uses `codexHome` binding in `.dtkit/state.json` |
| Gemini CLI / OpenCode / Cursor | — | — | — | Not implemented (v0.4+) | Rejected as unsupported runtime |
