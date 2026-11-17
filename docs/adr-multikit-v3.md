# ADR: Multi-Kit State Registry v3, Skill Namespacing, and Cross-Kit Ownership (`C01`)

## Context

Through DTKit v0.5 (`main` @ `b78a6b3`), each `(runtime, scope)` pair stores a single active `Installation` object in `.dtkit/state.json` (Codex) or `.dtkit/runtimes/<rt>/state.json` (`gemini`, `opencode`, `cursor`, `claude`). While content directories (`.dtkit/content/<kit>/...`), Cursor rules (`.cursor/rules/dtkit-<kit>.mdc`), and instruction managed blocks (`<!-- dtkit:begin <kit> ... -->`) already isolate by `kitID`, installing a second kit in the same `(runtime, scope)` currently fails with `different_kit_installed` (`ExitConflict`).

To support composing multiple kits (for example `engineer` alongside a domain kit such as `security-review` or a language pack) without breaking existing `v0.1`–`v0.5` installations, DTKit requires an explicit multi-kit state contract, deterministic skill namespacing, cross-kit ownership checks, and atomic migration/recovery.

## Decision

### 1. Backward-Compatible State Schema `v3` (`schemas/state.schema.json`)

- **Single-kit persistence (`schemaVersion` 1 or 2):** Continues to write the single top-level `Installation` JSON object when only one kit is installed, keeping full backward compatibility with `v0.1`–`v0.5` fixtures and consumers.
- **Multi-kit persistence (`schemaVersion` 3 `Registry`):** When two or more kits are installed in the same `(runtime, scope)`, `state.json` promotes transactionally inside `ExecuteTx` to a `Registry` document:
  - `schemaVersion`: `3`
  - `managedBy`: `"dtkit"`
  - `scope`: `"project" | "global"`
  - `runtime`: runtime profile ID
  - `adapterRevision`: runtime adapter revision
  - `codexHome` / `runtimeHome`: single shared global-home binding for the `(runtime, scope)`
  - `kits`: object map keyed by `<kitID>` containing per-kit `Installation` records (requiring `>= 2` kits in serialized v3 form)
  - `hooks`: optional runtime/scope hook registrations (`[]HookRegistration`) deduplicated per `(runtime, scope)` and shared across installed kits
  - `lastTransactionID`, `updatedAt`
- **Canonical migration & rollback:**
  - `state.LoadRegistryForRuntime` reads `v1`, `v2`, and `v3` transparently into a normalized in-memory `Registry`.
  - Promoting `v1/v2 -> v3` (installing a second kit) and demoting `v3 -> v1/v2` (removing one of two remaining kits) happen inside `ExecuteTx` using the existing `state-before.bin` byte-exact backup, so a crash at any transaction boundary rolls back cleanly across schema versions.
  - Runtime hooks (`hooks`) recorded on a single-kit `v2` installation are preserved when promoting to `v3` (stored in `Registry.Hooks`) and carried cleanly across `remove` of any kit that does not own the remaining installation, only being disabled/unmerged when the final hook-bearing scope is removed or explicitly disabled.

### 2. Logical Ownership & Cross-Kit Conflict Rules (`internal/planner`)

- Each artifact belongs logically to exactly one `<kitID>` inside `(runtime, scope)`.
- Before planning `install` or `update` for `<kitID>`, the planner builds `peerOwned` from all other kits currently recorded in the `(runtime, scope)` `Registry`:
  - `whole-file` artifacts (skills, content files, `.cursor/rules/dtkit-<kit>.mdc`): **must be strictly disjoint** across kits. Any path collision with a peer kit fails pre-write with `ExitConflict` (`artifact_owned_by_other_kit`), never silently overwriting or picking a winner.
  - `managed-block` artifacts (`AGENTS.md`, `GEMINI.md`, `CLAUDE.md`): multiple kits may target the same shared instruction file because each kit owns its own fenced block (`<!-- dtkit:begin <kitID> ... -->`).
- Removing `<kitA>` (`dtkit remove <kitA>`) removes only `<kitA>`'s artifacts and `<kitA>`'s managed block, prunes empty directories not needed by `<kitB>`, and leaves `<kitB>`'s files, state, and user content untouched.

### 3. Skill Namespacing & Legacy Alias Rule (`internal/adapter`)

- Historical `engineer` installations use `dtkit-<scope>-<slug>` (Codex) or `dtkit-<runtime>-<scope>-<slug>` (non-Codex) so existing user prompts/scripts (`$dtkit-project-plan`) do not break.
- To prevent collisions when a second kit also defines `skill/review` (or any identically named skill):
  - All kits other than `engineer` automatically render skills under the kit-aware namespace `<prefix><kitID>-<slug>` (for example `dtkit-project-security-review-review` on Codex or `dtkit-claude-project-security-review-review` on Claude Code).
  - Callers/packages can also request explicit kit-namespacing without renaming legacy `engineer` installations silently behind the user's back.

### 4. Lock Ordering (`internal/installer`)

- Every multi-kit transaction acquires locks in deterministic order:
  1. Per-`(runtime, scope)` operational lock (`.dtkit/local/install.lock` or `.dtkit/local/runtimes/<rt>/install.lock`) — serializing concurrent kit operations inside that `(runtime, scope)`.
  2. Lexicographically sorted shared target locks (`.dtkit/locks/<sha256>.lock`) for shared files (`AGENTS.md`, `CLAUDE.md`, `hooks.json`, `settings.json`) — serializing concurrent multi-runtime mutations without deadlocks.
