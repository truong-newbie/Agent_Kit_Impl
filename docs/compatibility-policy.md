# DTKit Compatibility Lifecycle, Version Pinning & Regression Policy (`C06`)

## 1. Public CI vs. Live Model Evaluation Boundary

- **Public CI (`.github/workflows/ci.yml`, `.github/workflows/compat-periodic.yml`):**
  - Runs 100% offline-deterministic discovery, schema, path-layout, hook-protocol, and fuzz smoke tests with **zero API keys**.
  - Periodic checks run on a weekly schedule and manual `workflow_dispatch` on dedicated branches or artifacts only: they emit `dtkit compat diff` maintainer reports and **never** auto-upgrade adapter profiles or publish releases.
- **Private Live Model & Performance Evaluation (`internal/eval`):**
  - Executes only with explicit owner credentials and `--max-budget-usd` caps inside isolated `CLIExecutor` workspaces.
  - Logs and artifacts MUST pass `eval.RedactLogText` before sharing (`<HOME>`, URL userinfo/query tokens, `Bearer`, API keys stripped).

## 2. Performance & Regression Budget Policy

Metrics are isolated strictly per `(modelId, runtime, kitId, kitVersion)` cohort (`eval.SummarizeCohorts`). Cross-model or cross-runtime score aggregation is disallowed (`eval.EvaluateCohortRegression` returns `insufficient_data` on mismatched cohorts).

Recommended baseline-anchored release gates when comparing candidate kit versions against a frozen baseline:

- `minValidRuns`: >= `10` non-blocked trials per arm/cohort (fewer valid runs or `environment_blocked` classifies explicitly as `insufficient_data`, never pass).
- `maxPassRateDrop`: `0.05` (no more than 5 percentage points drop vs. verified baseline).
- `maxLatencyMultiplier`: `1.50` (median duration <= `1.5x` baseline).
- `maxTokenMultiplier`: `1.40` (mean `input + output` tokens <= `1.4x` baseline).
- `maxCostMultiplier`: `1.40` (mean USD cost <= `1.4x` baseline).

## 3. Runtime Version Support, Pinning & Deprecation Window

| Runtime | Pinned Baseline | Adapter Revision | Support Status |
|---|---|---|---|
| Codex CLI | `0.169.0` | `1` | Active (`AGENTS.md`, `.agents/skills`, `.codex/hooks.json`) |
| OpenCode | `1.19.5` | `1` | Active (`AGENTS.md` namespaced block, `.opencode/skills`, `.opencode/plugins/dtkit-quality.js`) |
| Claude Code | `2.1.319` | `1` | Active (`CLAUDE.md`, `.claude/skills`, `.claude/settings.json`) |
| Gemini CLI | Schema-pinned | `1` | Active/Experimental (`GEMINI.md`, `.gemini/skills`, `.gemini/settings.json`; live binary pending) |
| Cursor | Schema-pinned | `1` | Active/Experimental (`.cursor/rules/dtkit-*.mdc`, `.cursor/skills`, `.cursor/hooks.json`; live IDE pending) |

- **Deprecation / Layout Stability Rule:** Existing `(runtime, scope)` directory layouts, skill prefixes, and `state.json` schemas are preserved within an `adapterRevision`. Any breaking layout change requires:
  1. An explicit ADR + maintainer diff review (`dtkit compat diff`),
  2. Minimum **N-1 minor release** deprecation notice in [docs/compatibility.md](./compatibility.md) and [docs/release.md](./release.md),
  3. Automated state migration/compatibility tests covering upgrade from the prior `adapterRevision`.
- **Last-Known-Good Snapshot:** [last-known-good-compat.json](./verification/last-known-good-compat.json) stores the committed baseline checked by `dtkit compat diff` and `.github/workflows/compat-periodic.yml`.
