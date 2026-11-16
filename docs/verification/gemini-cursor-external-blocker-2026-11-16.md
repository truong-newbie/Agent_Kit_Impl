# Gemini CLI & Cursor Verification Blocker Report (2026-11-16)

- **Date:** 2026-11-16 (N08 verification audit on `feat/release-hardening`)
- **Host check (`Get-Command gemini,cursor,agent`):** Neither `gemini`, `cursor`, nor `agent` CLI executables are installed on the verification host.

## What Was Verified Locally

- **Artifact layout & DTKit lifecycle:** Verified in Go automated tests (`internal/adapter/gemini`, `internal/adapter/cursor`, `internal/cli`, `internal/installer`) and live 4-runtime coexistence smoke (`dtkit install` + `dtkit status` + `dtkit remove` alongside `codex` and `opencode`):
  - **Gemini CLI:** `GEMINI.md` managed block, `.gemini/skills/dtkit-gemini-<scope>-*`, `.dtkit/runtimes/gemini/state.json`, optional `.gemini/settings.json` `context.fileName` awareness, and clean coexistence without colliding with `.agents/skills`.
  - **Cursor:** `.cursor/rules/dtkit-engineer.mdc` (`alwaysApply: true` project rule), `.cursor/skills/dtkit-cursor-<scope>-*`, `.dtkit/runtimes/cursor/state.json`, explicit `--global` `instructions` fallback (`runtimeSupported: true, dtkitSupported: false`, UI-only global User Rules), and clean coexistence/removal.
- **CLI pre-flight warning:** `dtkit` emits `warning: gemini executable not found on PATH; files will still be installed` and `warning: cursor executable not found on PATH; files will still be installed` consistently across `info`, `install`, `update`, `status`, and `doctor` when missing on `PATH`.

## External Blocker & Honesty Status

- **Gemini CLI (`project` & `global`):** **`unverified`** — `gemini` CLI binary absent on host; live CLI skill/instruction discovery and real model task execution could not be executed.
- **Cursor (`project` & `global`):** **`unverified`** — `cursor` CLI/IDE absent on host; live Cursor rule/skill loading and agent task execution could not be executed.
