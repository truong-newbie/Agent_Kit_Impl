# Feature Delivery Workflow

Follow this end-to-end lifecycle across any stack or multi-service repository:

1. **Brainstorm (`brainstorm`):** If requirements or architecture choices are open-ended, clarify unknowns and compare 2-3 trade-off options (`read-only`).
2. **Scout (`scout`):** Run `dtkit scout` and read `docs/codebase-summary.md`, `docs/system-architecture.md`, and `docs/code-standards.md` to locate affected zones, entrypoints, contracts, and test suites (`read-only`).
3. **Plan (`plan`):** Produce a phased plan in `implementation-plan.md` ordering Contract/Schema -> Backend/Worker -> Frontend -> Integration.
4. **Implement / Cook (`implement` + `test`):** Execute phase by phase within approved scope (`dtkit guard lock --mode tdd-red` -> failing test -> `tdd-green` -> implementation -> phase checks).
5. **Review & Security (`review`):** Audit the diff for correctness, cross-zone contract parity, concurrency, security risks, and test integrity.
6. **Docs & Verify (`docs`):** Sync affected `docs/` architecture notes, verify via `dtkit check --all` / `dtkit guard verify`, and deliver `completion-report.md`.
