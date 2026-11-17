# Codex Global Windows Layout Decision (`A03` — 2026-11-17)

- **Decision:** Keep the existing Codex global layout (`$CODEX_HOME/AGENTS.md` + `~/.agents/skills/dtkit-global-*`, state bound via `codexHome` in `~/.dtkit/state.json`) unchanged for `0.4.0-rc.1`, and keep **`codex / global` explicitly labeled `partial`** in RC release notes and [docs/compatibility.md](../compatibility.md).
- **Observed Evidence ([codex-0.169.0-2026-11-16.md](codex-0.169.0-2026-11-16.md)):**
  1. In isolated `$CODEX_HOME`, `codex debug prompt-input "hi"` loaded `<!-- dtkit:begin engineer global -->` cleanly across both `global-only` and `combined (project + global)` setups.
  2. Overriding `$env:HOME` / `$env:USERPROFILE` in a temporary directory on Windows did **not** redirect `codex 0.169.0` away from Windows `FOLDERID_Profile\.agents\skills` (real OS user profile, untouched for safety).
  3. Placing a probe skill in isolated `$CODEX_HOME/skills/` was discovered directly (`r0 = <tmpHome>\.codex\skills`) alongside bundled `.system` skills.
- **Why No Layout Change in RC (`0.4.0-rc.1`):**
  - Per [DTKIT-RC-AND-V05-PLAN.md](../../DTKIT-RC-AND-V05-PLAN.md) §4 (`A03`), changing owned paths requires a clear migration contract, `adapterRevision` bump, old/new transaction migration, and must never work around discovery limits by copying the same global skills into two directories.
  - Because this verification host lacks a separate disposable OS user account to test real Windows `FOLDERID_Profile\.agents\skills` safely without mutating the owner's account, keeping the existing layout in RC with an honest **`partial`** boundary preserves backward compatibility with `main` (`f81ce6e`) while documenting exact Windows global behavior.
- **Follow-up (v0.5+ optional):** Either verify `~/.agents/skills` under an isolated OS user account or evaluate an explicit optional `$CODEX_HOME/skills` layout binding with full `state` migration + `adapterRevision` increment.
