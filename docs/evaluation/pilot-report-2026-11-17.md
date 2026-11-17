# B02 Evaluation Pilot Report — 2026-11-17

## Scope & Integrity Statement

This pilot verifies the `internal/eval` harness, fixture isolation, out-of-workspace private evaluator overlay, scope/tampering/false-completion detectors, and frozen `engineer@0.3.0` baseline manifest using deterministic fake executors and live isolated negative checks.

> **Live Model Execution Status:** `blocked` (honest boundary — no live LLM benchmark runs were executed because isolated `CLAUDE_CONFIG_DIR` / `CODEX_HOME` credentials are unset and unbudgeted host tokens are not used automatically).

## Frozen Baseline & Suite Inventory

- **Manifest:** [`docs/evaluation/manifest.json`](./manifest.json) (`schemaVersion: 1`)
- **Frozen kit baseline:** `engineer@0.3.0` (`digest`: `fa2c417d743d2014ff035de268e135b190cba808ac0f1db5ecb936b60fcd81f5`)
- **Development scenarios:** `2` (`D01-go-discount-dev`, `D02-ts-format-dev`)
- **Held-out scenarios:** `10` (`H01`–`H10`) spanning `go`, `ts`, and `python` across `implement`, `debug`, `review`, `plan`, `test`, `explain`, `ask`, and prompt-injection `scope-guard` tasks.

## Pilot Harness Verification (`internal/eval/eval_test.go`)

- `TestManifestFixturesValid`: validates manifest structure, exact `engineer@0.3.0` digest parity, `>=2` dev and `>=10` held-out scenarios, `go`/`ts`/`python` coverage, and verifies zero private evaluator test leakage into `snapshot/`.
- `TestEvalRunnerIsolationScopeTamperingAndPrivateEvaluator`:
  - Baseline arm verifies no `CLAUDE.md` injection; unfixed `go-service` snapshot fails private `evaluator/eval_test.go`; false success text triggers `FalseCompletion=true`; unknown token/cost stays JSON `null`.
  - With-kit arm installs `engineer@0.3.0` in isolated workspace (`with_kit`), runs fix, overlays private Go evaluator (`go test ./...`), and records `OutcomePassed` with `ScopeViolation=false`, `TestTampering=false`.
  - Protected test modification triggers both `ScopeViolation=true` and `TestTampering=true`.
  - Read-only task file creation triggers `ScopeViolation=true`.
  - `environment_blocked` and `timeout` runs stay cleanly separated in `SummarizeSuite` (`ValidPassRate` excludes blocked/not_run without treating them as pass).
- `TestCLIExecutorIsolatedFailureModes`:
  - Missing runtime executable returns `Blocked=true`.
  - Empty isolated `HomeDir` without explicit `AllowHostAuth` fails closed as `environment_blocked` without reading host `~/.claude` or host API keys.

## Live Run Prerequisites (For Owner Execution)

To execute the 60-run held-out matrix (`10 scenarios * 3 repeats * 2 arms = 60` pairs per runtime/model) with explicit owner budget:

1. Provide isolated auth inside an isolated directory or pass explicit API key + `--max-budget-usd` via `CLIExecutor{ExtraEnv: ..., MaxBudgetUSD: ...}`.
2. Ensure `go`, `node` (`>= 18` for `node --test`), and `python`/`python3` exist on the runner `PATH`.
3. Archive raw per-run JSON records alongside `manifest.json` before comparing `engineer@0.3.0` vs. `engineer@0.4.0`.
