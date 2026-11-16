# Codex CLI Verification Report — 0.169.0 (2026-11-16)

- **Date:** 2026-11-16 (initial N08 update on `feat/release-hardening`)
- **OS:** Windows (host `codex-cli 0.169.0` via `codex.ps1`; artifacts generated against isolated temp fixture dirs via Docker `dtkit` binary because unsigned binaries in `$env:TEMP` are restricted by Application Control on this host)
- **Codex CLI:** `codex-cli 0.169.0` (`features.hooks = stable true`, `--dangerously-bypass-hook-trust` documented for pre-vetted automation)
- **Scopes tested:** `project`, isolated `$CODEX_HOME` (`global`), `combined` (`project + global`), and 4-runtime coexistence (`codex + gemini + opencode + cursor`)
- **User-state safety:** Never touched real `%USERPROFILE%`, `~/.codex`, or `~/.agents/skills`; all checks used an isolated fixture git repo + temp `HOME`/`USERPROFILE`/`CODEX_HOME` (`$env:TEMP\dtkit-n08-verify`), deleted afterward.

## Commands Executed

1. **Isolated `codex debug prompt-input` matrix (`HOME`, `USERPROFILE`, `CODEX_HOME` pointing to temp dirs):**
   - **Combined (`project` + `global` + 4 runtimes installed):**
     `codex debug prompt-input "hi"` -> exit `0` (`67,943` chars output):
     - Included **both** `<!-- dtkit:begin engineer global -->` (`$CODEX_HOME/AGENTS.md`) and `<!-- dtkit:begin engineer project -->` (`<repo>/AGENTS.md`), preserving user `AGENTS.md` content (`Keep user rules intact.`).
     - Discovered all 7 project skills (`dtkit-project-ask`, `dtkit-project-debug`, `dtkit-project-explain`, `dtkit-project-implement`, `dtkit-project-plan`, `dtkit-project-review`, `dtkit-project-test`) under `<repo>/.agents/skills`.
     - **Important Windows global-skill finding:** Overriding `$env:HOME` / `$env:USERPROFILE` did **not** cause `codex 0.169.0` on Windows to read `<tmpHome>\.agents\skills\dtkit-global-*`; however, placing a probe skill inside `$CODEX_HOME/skills/` was discovered immediately (`r0 = <tmpHome>\.codex\skills`) alongside bundled `.codex/skills/.system`. Thus on Windows, `codex 0.169.0` honors `$CODEX_HOME` for global `AGENTS.md` and `$CODEX_HOME/skills`, whereas `~/.agents/skills` discovery resolves the real OS user profile (`FOLDERID_Profile`) unless tested under an isolated OS user account.
   - **Global-only (empty non-repo cwd with isolated `CODEX_HOME`):**
     `codex debug prompt-input "hi"` -> loaded `<!-- dtkit:begin engineer global -->` cleanly.
2. **Ephemeral `codex exec` runs (`--ephemeral --ignore-user-config`):**
   - `$dtkit-project-plan` / `$dtkit-project-review` (`-s read-only`): accepted skills and referenced `.agents/skills/dtkit-project-*`, `AGENTS.md`, `.dtkit/content/engineer/*`.
   - `$dtkit-project-implement Inspect calc.py and test_calc.py and fix the bug in calc.py if allowed, or report exact blocker.` (`-s workspace-write --json`):
     - Emitted `warning: Untrusted project-local configuration is disabled. Run codex exec --trust ...` (empirically confirming untrusted-repo config/hook trust gating).
     - Announced `I’m using the dtkit-project-implement skill to inspect the code, tests, and project rules...` and attempted reading `AGENTS.md`, `calc.py`, `test_calc.py`, `engineering.md`, `completion-report.md`, `SKILL.md`.
     - Spawned PowerShell commands inside Codex's Windows sandbox failed with `CreateProcess ... powershell.exe ... rejected: blocked by policy`; Codex refused to fabricate file inspection or test results. (Did not bypass with `--dangerously-bypass-approvals-and-sandbox`.)
3. **Clean coexistence & removal:**
   - Removed `codex` (project + global), `opencode`, `gemini`, and `cursor` kits cleanly -> `<repo>/AGENTS.md` reverted byte-for-byte to original `# User Repo Instructions\nKeep user rules intact.\n` and pre-existing `.codex/hooks.json` (`echo user-hook`) remained untouched.

## Conclusion

- **Codex `project` layout & skill/instruction discovery:** observed working on `codex-cli 0.169.0` (Status: **Partial**).
- **Codex `global` `$CODEX_HOME/AGENTS.md` discovery:** verified in isolated `$CODEX_HOME`.
- **Remaining blockers (honest boundaries):**
  1. End-to-end `codex exec` workspace file edits, test execution, and project `SessionStart` hook execution remain blocked/unverified under this host's Windows sandbox policy and untrusted temp-repo gate (`--trust` / interactive trust UI required).
  2. Live `~/.agents/skills` global skill discovery on Windows requires a dedicated isolated OS user account (or optional `$CODEX_HOME/skills` investigation post-release).
