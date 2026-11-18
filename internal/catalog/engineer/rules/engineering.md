# Engineering Principles (DTKit Engineer Kit)

- Follow the structured lifecycle: **Brainstorm -> Scout -> Plan -> Implement -> Test -> Converge -> Review -> Docs**. Scale depth to task risk; never jump straight into edits on ambiguous specs.
- Respect project context & constitution authority: inspect `.dtkit/constitution.md` (when present), `.dtkit/project.json`, `docs/codebase-summary.md`, `docs/system-architecture.md`, and `docs/code-standards.md`; treat constitution `MUST`/`NEVER` rules as non-negotiable `CRITICAL` constraints.
- Keep context disciplined: scout relevant directories (`dtkit scout`) and search callers before reading bounded slices; avoid dumping unrelated files.
- Enforce phase separation & approval gates: never modify files during read-only tasks (`brainstorm`, `scout`, `plan`, `converge`, `review`, `explain`, `ask`), and wait for explicit approval between `plan` and `implement` while respecting active `dtkit guard` firewall boundaries.
- Apply YAGNI, KISS, and DRY: keep diffs modular and focused, reuse existing helpers, and avoid unrequested public API or schema breaks.
- Never tamper with test integrity: follow Red/Green verification, never skip or weaken assertions to force a pass, and run real verification (`dtkit check` / `dtkit guard verify`) after final edits.
- Protect secrets and validate untrusted inputs/paths across all service boundaries; report exact executed commands, outcomes, and remaining limitations honestly.
