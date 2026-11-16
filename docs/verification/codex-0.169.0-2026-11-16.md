# Codex CLI Verification Report — 0.169.0 (2026-11-16)

- **Date:** 2026-11-16
- **OS:** Windows (native `dtkit.exe` built from the P5 Go source tree `3885d82` via `golang:1.27.4` with `-Version 0.1.0-rc1`)
- **Codex CLI:** `codex-cli 0.169.0` (`codex.cmd`)
- **Scope tested:** `project` (isolated temp Git repository `$env:TEMP\dtkit-codex-smoke`)
- **Global scope status:** `pending` (not executed against real `%USERPROFILE%\.agents\skills` to protect user configuration; requires an isolated OS user account)

## Setup and Commands Executed

1. Extracted `dist/dtkit_0.1.0-rc1_windows_amd64.zip` and ran natively:
   - `dtkit.exe version --json` -> exit `0`, `version: "0.1.0-rc1"`
   - `dtkit.exe doctor --project --path <fixture>` -> exit `0`, detected `codex.cmd`
   - `dtkit.exe install engineer --project --path <fixture> --yes` -> exit `0`, created 15 artifacts (`AGENTS.md`, 7 `.agents/skills/dtkit-project-*` skills, 7 `.dtkit/content/engineer/*` files, `.dtkit/state.json`)
   - `dtkit.exe status --project --path <fixture>` -> exit `0`, `healthy`
2. Invoked Codex CLI read-only ephemeral sessions against `<fixture>`:
   - `'$dtkit-project-plan Create a short plan to fix the bug in calc.py and add a unit test. Do not edit any files.' | codex.cmd exec --ephemeral --ignore-user-config -s read-only -C <fixture> -`
   - `'$dtkit-project-review Review calc.py for bugs without editing any files.' | codex.cmd exec --ephemeral --ignore-user-config -s read-only --json -C <fixture> -`

## Observed Results

- **Skill & instruction discovery (verified):**
  - Codex accepted `$dtkit-project-plan` and `$dtkit-project-review` and announced using those skills.
  - From injected `AGENTS.md` and skill metadata, Codex targeted the exact DTKit paths:
    - `.agents/skills/dtkit-project-review/SKILL.md`
    - `.dtkit/content/engineer/rules/engineering.md`
    - `.dtkit/content/engineer/templates/review-report.md`
  - Codex respected read-only boundaries and explicitly refused to fabricate file inspection results when its command execution was blocked.
- **Sandbox command execution limitation (environment-specific):**
  - Under `codex.cmd exec -s read-only` on this Windows host, `powershell.exe` subcommands were rejected by the local Windows/Codex execution policy (`CreateProcess ... rejected: blocked by policy`), preventing end-to-end tool execution on `calc.py` within that sandbox mode.

## Conclusion

- **Project instruction & skill discovery:** observed working on `codex-cli 0.169.0`.
- **Full interactive/workspace-read execution & global isolation smoke:** `pending` (complete in a separate OS user account with allowed Codex workspace read tools).
