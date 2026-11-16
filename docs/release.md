# Release Readiness Checklist

> **Current Repository Split:** `main` (`f81ce6e`) contains the v0.1 Codex baseline with **0 published tags / 0 GitHub Releases**. The complete multi-runtime codebase (`codex`, `gemini`, `opencode`, `cursor`), hooks, distribution commands, and release-hardening backlog (`N00`–`N12`) live on branch **`feat/release-hardening`** (branched from `feat/runtime-cursor-v0.4` `936043f`).

## Owner Decisions Already Confirmed

1. Canonical Go module path: `github.com/truong-newbie/Agent_Kit_Impl`.
2. Repository license: `MIT License` ([LICENSE](../LICENSE)).
3. Initial v0.1 release prep (`f81ce6e`): merged to `main`; public tag deferred pending full verification and explicit owner release confirmation.

## Pre-Release Gates on `feat/release-hardening` (`N00`–`N12`)

See [DTKIT-NEXT-PHASE-PLAN.md](../DTKIT-NEXT-PHASE-PLAN.md) for detailed acceptance criteria per task:

- [x] `N00` — Baseline sync, truthful README/compatibility matrix, and progress tracking.
- [ ] `N01` — Bump bundled `engineer` kit version for multi-runtime manifest changes and test real upgrade path from `main` (`f81ce6e`).
- [ ] `N02` — Enforce strict read-only no-write guarantees for remote/cached `--dry-run`, `info`, and `validate`.
- [ ] `N03` — Harden remote HTTP timeouts/redirects, registry index validation, offline `minCliVersion`, and multi-process cache idempotency.
- [ ] `N04` — Reproduce and verify concurrent cross-runtime `AGENTS.md` mutations, shared lock ordering, and recovery safety.
- [ ] `N05` — Formalize required vs optional/fallback capability enforcement (including Cursor global) and consistent pre/post-install warnings.
- [ ] `N06` — Harden hook JSON duplicate-key parsing, ambiguous-digest handling, special-character executable quoting, and timeout/leak isolation.
- [ ] `N07` — Document CI skip reasons and add native Windows lock/junction and child-process crash recovery tests.
- [ ] `N08` — Run full isolated runtime discovery + real task smoke checks across `codex`, `gemini`, `opencode`, and `cursor` (marking unverified integrations experimental).
- [ ] `N09` — Establish Engineer Kit evaluation rubric and baseline vs with-kit task results (`docs/evaluation/`).
- [ ] `N10` — Sanitize release workflow/scripts inputs, add GitHub Release publishing flow, generate next RC candidate (e.g., `0.4.0-rc.1`), and verify clean-machine terminal install.
- [ ] `N11` — Final CLI/doc/changelog/troubleshooting audit and PR merge readiness to `main`.
- [ ] `N12` — Issue templates, redacted `--redact` diagnostic summary support, support policy, and incident handling procedure.
