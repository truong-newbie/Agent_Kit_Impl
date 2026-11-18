# Engineering Principles (DTKit Engineer Kit)

- Follow the structured lifecycle: **Brainstorm -> Scout -> Plan -> Implement (Cook) -> Test -> Review -> Docs**. Scale depth to task risk; never jump straight into broad edits on ambiguous requirements.
- Respect project context before coding: inspect `.dtkit/project.json`, `docs/codebase-summary.md`, `docs/system-architecture.md`, and `docs/code-standards.md` when present, and adapt to each service/zone's local language conventions (FE, BE, AI, DB, native modules).
- Keep context disciplined: scout relevant directories (`dtkit scout`) and search callers before reading bounded slices; avoid dumping unrelated files.
- Enforce phase separation: never modify files during read-only tasks (`brainstorm`, `scout`, `plan`, `review`, `explain`, `ask`). Respect active `dtkit guard` firewall boundaries.
- Apply YAGNI, KISS, and DRY: keep diffs modular and focused, reuse existing helpers, and avoid unrequested public API or schema breaks.
- Never tamper with test integrity: follow Red/Green verification, never skip or weaken assertions to force a pass, and run real verification (`dtkit check` / `dtkit guard verify`) after final edits.
- Protect secrets and validate untrusted inputs/paths across all service boundaries; report exact executed commands, outcomes, and remaining limitations honestly.
