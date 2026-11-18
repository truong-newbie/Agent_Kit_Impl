# Implementation Plan Template

## 1. Goal & Clarified Assumptions
- Requirement / spec reference (`docs/specs/<slug>.md` if present, listing mapped `FR-xxx` / `P1` stories):
- Explicit assumptions / confirmed decisions:

## 2. Scope & Guard Boundaries
- **In scope:** (target zones, modules, contracts)
- **Out of scope:** (explicitly untouched modules)
- **Recommended `dtkit guard` mode:** (`tdd-red` -> `tdd-green` or `scoped`)

## 3. Phased Execution Breakdown
Break work into small, independently verifiable phases (for multi-service tasks, order Contract/Schema -> Backend/Worker -> Frontend -> Integration):

### Phase 1: <Title>
- Files to modify/create:
- Key invariants & edge cases:
- Verification command(s):

### Phase 2: <Title>
- Files to modify/create:
- Key invariants & edge cases:
- Verification command(s):

## 4. Acceptance Criteria & Verification Plan
- Automated tests to add/update:
- Commands to run after final edit (`dtkit check`, `dtkit guard verify`, or zone-native test runners):
- Documentation sync needed (`docs/codebase-summary.md`, `docs/system-architecture.md`, `docs/code-standards.md`):
- Risks / rollback notes:
