# DTKit Support Policy & Incident Procedure (`N12`)

## 1. Runtime Support Policy

DTKit tracks four runtime adapters with strict, evidence-based support tiers (no telemetry is collected by default):

| Tier | Runtimes & Observed Versions | Policy |
|---|---|---|
| **Partial (Discovery Tested)** | `codex` (`codex-cli 0.169.0`), `opencode` (`1.19.5`) | Layout, instruction precedence, skill discovery, and coexistence are verified on real runtimes; promotion to full **Verified / Stable** requires unblocked workspace task & benchmark evidence (`N08`/`N09`). |
| **Experimental (`unverified`)** | `gemini`, `cursor` | Adapter rendering, ownership, rollback, and pre-flight binary warnings pass automated tests; live CLI verification awaits runtime binary availability. |

### Re-checking Runtime Compatibility Safely

- Adapter layouts should be re-verified when target runtimes ship minor/major CLI changes (`.codex/hooks.json`, `AGENTS.md`, `SKILL.md` paths, `OPENCODE_CONFIG_DIR`, `GEMINI_CLI_HOME`).
- Compatibility re-checks remain manual or optional scheduled automation using isolated temporary directories and **must never require live paid model credentials inside public CI workflows**.

## 2. Diagnostic Collection (`dtkit doctor --redact`)

- Run `dtkit doctor --redact --json` (or omit `--json` for text) when gathering diagnostics for bug reports.
- `--redact` replaces user home paths with `<HOME>` and strips HTTP URL credentials/query tokens and `Bearer`/`token=` patterns from warnings, check messages, hints, and error fields without touching state files or reading `auth.json` secrets.
- Never share raw `.dtkit/state.json`, `.dtkit/local/` backups, credentials, or private URLs in issues.

## 3. Security & Release Incident Procedure

If a published release asset, bundled kit template, or adapter contract is found to have a safety, corruption, or checksum defect:

1. **Halt bad asset distribution immediately:** Mark the affected GitHub Release as draft/prerelease or annotate known issue warnings in release notes and `docs/release.md`; verify `SHA256SUMS` mismatches are never bypassed.
2. **Preserve forensic/recovery evidence:** Instruct affected users **not** to delete `.dtkit/state.json` or `.dtkit/local/transactions/`. Prefer `dtkit recover --dry-run` -> `dtkit recover --yes` (or `dtkit remove --dry-run` -> `dtkit remove --yes`) to keep backups and user content outside managed blocks intact.
3. **Never auto-downgrade silently:** DTKit intentionally avoids silently downgrading kit state schemas or installed kit versions without an explicit migration contract.
4. **Ship patched version with regression coverage:** Bump the kit version (`engineer` if kit content changed) or CLI semver tag, add failing-first regression tests reproducing the incident, verify across `windows-latest`, `ubuntu-latest`, `macos-latest`, and Linux `-race`, and publish updated `SHA256SUMS` + release notes referencing the superseded version.
