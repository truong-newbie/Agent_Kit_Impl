# OpenCode Verification Report — 1.19.5 (2026-11-16)

- **Date:** 2026-11-16 (updated during N08 matrix verification on `feat/release-hardening`)
- **OS:** Windows (`opencode 1.19.5` via `opencode.ps1`; artifacts generated into isolated temp dirs via Docker `dtkit` binary because unsigned binaries in `$env:TEMP` are restricted by Application Control on this host)
- **OpenCode CLI:** `opencode 1.19.5`
- **Scopes tested:** `project-only`, `global-only`, `combined (project + global)`, `4-runtime coexistence (codex + gemini + opencode + cursor)`, and clean uninstall preservation
- **User-state safety:** Isolated `$env:HOME`, `$env:USERPROFILE`, `$env:XDG_CONFIG_HOME`, `$env:XDG_DATA_HOME`, `$env:XDG_STATE_HOME` targeting `$env:TEMP\dtkit-n08-verify\home`; real `%USERPROFILE%` never touched.

## Commands Executed

1. **Project-only (`--runtime opencode --project`):**
   - `opencode debug paths` -> resolved `config` to `<tmpHome>\.config\opencode`.
   - `opencode debug config` -> exit `0` (`https://opencode.ai/config.json` valid schema).
   - `opencode debug skill` -> discovered all **7** `dtkit-opencode-project-*` skills + pre-existing `user-local` skill (`count=7` + `1`).
2. **Global-only (removed project OpenCode, installed `--runtime opencode --global`, ran from empty sibling cwd):**
   - `opencode debug skill` -> discovered all **7** `dtkit-opencode-global-*` skills (`count=7`).
3. **Combined + 4-runtime coexistence (`codex`, `gemini`, `opencode`, `cursor` project + `opencode`, `codex` global):**
   - `dtkit status --path /work/repo --json` -> exit `0`, all 4 runtimes reported `healthy`.
   - `opencode debug skill` -> discovered all **7** `dtkit-opencode-project-*` + **7** `dtkit-opencode-global-*` + **1** `user-local` simultaneously with zero collisions against `.agents/skills`, `.gemini/skills`, or `.cursor/skills`.
   - `AGENTS.md` contained user lines + `<!-- dtkit:begin engineer project -->` + `<!-- dtkit:begin engineer opencode project -->` cleanly.
4. **Clean removal & preservation:**
   - Removed OpenCode `project` and `global` -> `opencode debug skill` dropped cleanly to `dtkit-opencode-*=0`, `user-local=1`; user `AGENTS.md` lines and sibling Codex block remained completely intact.
5. **Model provider check (`opencode providers list` in isolated `HOME`):**
   - Returned `0 credentials` (`~/.local/share/opencode/auth.json` unconfigured in isolated home), preventing live LLM turn execution (`opencode run`) without configured credentials.

## Conclusion

- **OpenCode `1.19.5` project-only, global-only, combined, 4-runtime coexistence, and clean removal:** empirically verified on `opencode 1.19.5` (Status: **Partial**).
- **Remaining blocker:** Live LLM session (`opencode run`) requires isolated model provider credentials (`0 credentials` in clean verification profile).
- **Hooks parity note:** OpenCode uses JS/TS plugins rather than DTKit v0.2 Codex shell hooks (`capabilities.hooks`: `runtimeSupported: true, dtkitSupported: false`).
