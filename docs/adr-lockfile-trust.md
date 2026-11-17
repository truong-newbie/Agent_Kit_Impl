# ADR: Portable Lockfile (`dtkit.lock`), Reproducible Sync, and Ed25519 Trust Policy (`C03`)

## Status

Accepted for `C03` on branch `feat/c03-lockfile`.

## Context

`.dtkit/state.json` (v1/v2/v3) tracks machine-local installation state, home bindings, transaction IDs, and artifact digests. It is non-portable across team workstations or CI clean checkouts. Teams need a committed project declaration (`dtkit.lock`) and optional publisher trust policy (`.dtkit/trust.json`) so two machines install byte-identical verified kits deterministically and fail closed on tampering, revoked keys, or unavailable sources.

## Decision

1. **Strict Separation (`dtkit.lock` vs `state.json`):**
   - `dtkit.lock` lives at project root (`schemaVersion: 1`) and records only portable kit declarations: `id`, `version`, `packageDigest`, optional `archiveSha256`, `source` (`bundled` | `local` | `archive` | `remote` with relative POSIX path or sanitized HTTPS/loopback URL), `runtimeTargets`, `adapterRevisions`, `selection`, and optional `provenance`.
   - `ValidateLockfile` rejects absolute filesystem paths, Windows drive letters/UNC paths, parent traversal (`..`), embedded URL credentials/userinfo, duplicate kit IDs, and unknown JSON keys.

2. **Reproducible Resolution & Offline Semantics:**
   - Before any filesystem mutation, `dtkit sync` and `dtkit lock verify` resolve each locked entry:
     - `bundled`: verified against bundled `engineer` digest/version.
     - `local` / `archive`: resolved relative to project root (never crossing symlinks/traversal), verified against exact `version` and `packageDigest` (plus `archiveSha256` for archives).
     - `remote`: requires `archiveSha256`. Checks verified digest cache (`dist.Cache`) first; fetches over HTTPS/loopback only if not cached and `--offline` is false; fails closed with `offline_cache_miss` / `source_unavailable` when unreachable or missing in offline mode.
   - Runtime compatibility: verifies target `--runtime` appears in `runtimeTargets` and matches `adapterRevisions[runtime]` when specified (`adapter_revision_mismatch`).

3. **Publisher Provenance & Trust Policy (`.dtkit/trust.json`, `crypto/ed25519`):**
   - Hash integrity (`sha256`) and publisher identity (`ed25519` signature over `"dtkit-provenance-v1\n<id>\n<version>\n<archiveSha256-or-packageDigest>\n"`) serve distinct roles: neither proves skill/hook prompt safety on its own.
   - Standard library `crypto/ed25519` is used exclusively (no custom cryptography).
   - Trust policy supports multiple active keys (key rotation), explicit `status: "revoked"` (key revocation), `notAfter` expiration, `allowedKits` scoping, and `requireSignature`. Verification always executes before mutating workspace files.

4. **Error Codes & Recovery:**
   - Usage errors (`ExitUsage`, `2`): malformed schema, non-portable paths, duplicate kit IDs.
   - Conflict/Integrity/Trust errors (`ExitConflict`, `3`): `lock_digest_mismatch`, `archive_digest_mismatch`, `version_mismatch`, `adapter_revision_mismatch`, `offline_cache_miss`, `source_unavailable`, `signature_required`, `untrusted_key`, `key_revoked`, `key_expired`, `invalid_signature`.
   - `dtkit sync` executes each kit update/install through DTKit's existing `ExecuteTx` atomic transaction engine so crash recovery (`dtkit recover`) applies unchanged.
