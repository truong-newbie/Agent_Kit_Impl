# Feature Delivery Workflow

Follow this end-to-end lifecycle across any stack or multi-service repository:

1. **Brainstorm / Spec (`brainstorm`):** If requirements or architecture choices are open-ended, clarify unknowns, compare 2-3 trade-off options (`read-only`), and for substantial features record the agreed specification in `docs/specs/<slug>.md` (resolve all `[NEEDS CLARIFICATION]` markers first).
2. **Scout (`scout`):** Run `dtkit scout` and read `.dtkit/constitution.md` (when present), `docs/codebase-summary.md`, `docs/system-architecture.md`, and `docs/code-standards.md` to locate affected zones, entrypoints, contracts, and test suites (`read-only`).
3. **Plan (`plan`):** Produce a phased plan in `implementation-plan.md` mapping `FR-xxx`/`P1` stories and ordering Contract/Schema -> Backend/Worker -> Frontend -> Integration; keep `dtkit guard lock --mode read-only` until the user approves the plan.
4. **Implement / Cook (`implement` + `test`):** Execute phase by phase within approved scope (`dtkit guard lock --mode tdd-red` -> failing test -> `tdd-green` -> implementation -> phase checks).
5. **Converge (`converge`):** Run `dtkit converge` (`--spec <file>` when a feature spec exists) / `dtkit_converge` to verify all spec requirements, plan items, referenced files, constitution rules, and `dtkit guard verify` receipts align before review.
6. **Review & Security (`review`):** Audit the diff for correctness, cross-zone contract parity, security risks, and test integrity.
7. **Docs & Verify (`docs`):** Sync affected `docs/` architecture notes, verify via `dtkit check --all` / `dtkit guard verify`, and deliver `completion-report.md`.
