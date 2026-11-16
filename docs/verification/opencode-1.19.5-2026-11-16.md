# OpenCode Verification Report — 1.19.5 (2026-11-16)

- **Date:** 2026-11-16
- **OS:** Windows (isolated temp project + temp `HOME` / `USERPROFILE` with `.config\opencode`; artifacts created by Docker-built `dtkit` binary from branch `feat/runtime-opencode-v0.4`)
- **OpenCode CLI:** `opencode 1.19.5` (`opencode.ps1`)
- **Scopes tested:** `project`, `global` (isolated `$env:HOME` / `$env:USERPROFILE`), and project coexistence with `codex`
- **Limitation note:** Native Windows execution of an un-signed `dtkit.exe` inside `$env:TEMP` is blocked by Windows Application Control on this host, so `dtkit` commands ran against the temp directories via Docker `golang:1.27.4` while native `opencode 1.19.5` inspected the generated files on Windows; live LLM turn execution (`opencode run`) was not invoked (no model credentials spent).

## Commands Executed

1. Inside Docker container targeting temp workspace `/smoke/repo` and isolated temp `/smoke/home`:
   - `dtkit install engineer --project --path /smoke/repo --runtime codex --yes` -> exit `0` (`15 artifacts`)
   - `dtkit install engineer --project --path /smoke/repo --runtime opencode --yes` -> exit `0` (`15 artifacts`, appended runtime-namespaced block `<!-- dtkit:begin engineer opencode project -->` into shared `AGENTS.md`)
   - `dtkit install engineer --global --runtime opencode --yes` -> exit `0` (`15 artifacts` under `~/.config/opencode/` and `~/.dtkit/runtimes/opencode/`)
   - `dtkit status --path /smoke/repo --json` -> exit `0`, reported `codex` (project) and `opencode` (project + global) all `healthy`
2. On native Windows targeting the exact same isolated `$homeDir` and `$proj`:
   - `opencode debug paths` -> resolved `config` to `<isolated-home>\.config\opencode`
   - `opencode debug config` -> exit `0`, valid config state
   - `opencode debug skill` -> discovered and parsed all 7 `dtkit-opencode-project-*` skills (`.opencode\skills\dtkit-opencode-project-*\SKILL.md`) and all 7 `dtkit-opencode-global-*` skills (`<isolated-home>\.config\opencode\skills\dtkit-opencode-global-*\SKILL.md`) without name collisions against Codex's `.agents\skills\dtkit-project-*`

## Conclusion

- **OpenCode `1.19.5` project, global, and Codex-coexistence skill/instruction layout & config discovery:** verified empirically on `opencode 1.19.5`.
- **Hooks capability parity:** OpenCode uses JS/TS plugins (`.opencode/plugins/`, `~/.config/opencode/plugins/`) rather than DTKit v0.2 Codex shell hooks; `capabilities.hooks` explicitly reports `runtimeSupported: true, dtkitSupported: false`.
