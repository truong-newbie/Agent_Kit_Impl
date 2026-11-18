# Documentation & Living Context Management

Maintain three concise, accurate architecture documents in `docs/` so any polyglot or multi-service repository stays immediately navigable across sessions:

1. `docs/codebase-summary.md`: Workspace zones (frontend, backend, workers, AI, DB), languages, manifests, API contracts, and entrypoints (refreshable via `dtkit scout --write-summary`).
2. `docs/system-architecture.md`: Component boundaries, data/control flows, cross-service API/event schemas, storage, and security boundaries.
3. `docs/code-standards.md`: Local naming, error handling, layering, testing, and linting conventions observed per zone.

## Rules
- Inspect these docs during `scout` and `plan` before proposing cross-cutting changes.
- Update affected sections after completing a feature or architectural change; never document planned behavior as implemented unless verified.
- Keep secrets, host-specific paths, and credentials out of documentation.
