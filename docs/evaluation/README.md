# DTKit Agent Quality Evaluation Harness (`B02`)

This directory stores the frozen evaluation manifest and reports comparing **baseline** (fresh isolated runtime without DTKit) against **with-kit** (fresh isolated runtime with DTKit installed) on external, non-DTKit multi-stack repositories.

## Architecture & Leakage Controls

- **Package:** [`internal/eval`](../../internal/eval) (`types.go`, `runner.go`, `executor.go`).
- **Fixtures & Private Evaluators:** Stored under `testdata/evaluation/`:
  - `go-service` (`snapshot/` + private `evaluator/`)
  - `ts-web` (`snapshot/` + private `evaluator/`)
  - `python-cli` (`snapshot/` + private `evaluator/`)
- **Anti-leak separation:** Only `snapshot/` is copied into the isolated trial workspace before execution. Private `evaluator/` tests are overlaid and executed by `RunTrial` **after** the agent finishes, preventing the agent from gaming or weakening evaluation assertions.
- **Home/auth isolation:** `CLIExecutor` redirects `HOME`, `USERPROFILE`, `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `CODEX_HOME`, and `CLAUDE_CONFIG_DIR` inside the trial temp home, strips host API keys unless explicitly permitted, and fails closed with `environment_blocked` when isolated credentials/tools are missing.
- **Pairing & accounting:** `PlanPairs` generates a deterministic randomized baseline vs. with-kit sequence per repeat. `SummarizeSuite` separates `passed`, `task_failed`, `environment_blocked`, `timeout`, `cancelled`, and `not_run` outcomes without treating blocked runs as passes or failures, and serializes unavailable token/cost telemetry as JSON `null`.
- **Frozen baseline:** `manifest.json` pins `engineer` at `0.3.0` (digest `fa2c417d743d2014ff035de268e135b190cba808ac0f1db5ecb936b60fcd81f5`) prior to `B04` prompt/kit revisions.
