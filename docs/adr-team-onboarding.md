# ADR: Portable Team Onboarding Bundle & Cross-Runtime Inspect (`C04`)

## Status

Accepted for `C04` on branch `feat/c04-onboarding`.

## Context

New contributors and CI environments need a single reproducible way to onboard onto a repository's DTKit configuration (pinned kits, optional publisher trust policy, project verification profile, and recommended opt-in hooks) across two different workstations/homes without inheriting machine-local state, home bindings, or executing untrusted commands automatically during setup.

## Decision

1. **Portable Bundle (`TeamBundle`, `schemaVersion: 1`, `schemas/team.schema.json`):**
   - `TeamBundle` aggregates portable declarations only: `schemaVersion`, optional `name`/`defaultRuntime`, `runtimes`, required `lockfile` (`dist.Lockfile`), optional `trustPolicy` (`dist.TrustPolicy` public keys only), optional `projectProfile` (`project.Profile`), and optional allowlisted `hookRecommendations` (`session-context`, `check-changed`, `guard-edit`).
   - `export` strictly excludes `.dtkit/state.json`, `.dtkit/local/`, backups, transaction journals, `check-trust.json`, private keys, home directories, and URL tokens/userinfo.

2. **Safe Import (`dtkit team import`):**
   - Validates bundle schema, runtime capabilities, lockfile digests, and Ed25519 provenance **before** any filesystem mutation.
   - If `projectProfile` is present, writes `.dtkit/project.json` only if missing (or identical; differing existing `.dtkit/project.json` fails closed without overwriting) and **never** writes `.dtkit/local/check-trust.json` or executes project commands automatically.
   - Hook recommendations remain informational until the user explicitly opts in via `dtkit hooks enable ...`.

3. **Unified Cross-Runtime Inspect (`dtkit status --all-runtimes`) & Repo Template (`dtkit team template`):**
   - `dtkit status --all-runtimes` inspects all 5 supported runtimes (`codex`, `gemini`, `opencode`, `cursor`, `claude`) in one report (read-only), with optional `--redact`.
   - `dtkit team template` scaffolds `dtkit.lock`, read-only discovered `.dtkit/project.json`, and `.dtkit/ONBOARDING.md` without overwriting existing files or running repository scripts.
