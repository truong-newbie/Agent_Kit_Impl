# Bootstrap Living Architecture Docs Workflow

Use this workflow to onboard any single-language or polyglot repository:

1. **Scout (`scout`):** Run `dtkit scout --write-summary` and inspect zone manifests, entrypoints, contracts (`openapi.*`, `*.proto`, schemas), and existing CI/test configs.
2. **Author Docs (`docs`):** Generate or update:
   - `docs/codebase-summary.md` (workspace zones, languages, file inventory)
   - `docs/system-architecture.md` (data flows, service boundaries, storage, auth/security model)
   - `docs/code-standards.md` (local idioms, error handling, test/lint commands per zone)
3. **Verify Accuracy:** Confirm every file path, package name, and check command recorded in `docs/` actually exists in the repository.
