# Feature Specification: <Feature Name>

- **Slug:** `<feature-slug>`
- **Status:** `Draft` <!-- Draft | Clarified | Approved -->
- **Constitution:** `.dtkit/constitution.md` (or `docs/code-standards.md`)

## 1. Problem & User Scenarios (Prioritized)

Order stories by user value; each story must be independently testable as a standalone slice.

### User Story 1 — <Primary Flow> (Priority: P1)
- **Why this priority:** <Business / user impact>
- **Independent Test:** <How P1 can be tested and verified standalone>
- **Acceptance Scenarios:**
  1. **Given** <initial state>, **When** <user/system action>, **Then** <observable outcome>.

### User Story 2 — <Secondary Flow> (Priority: P2)
- **Why this priority:** <Value>
- **Independent Test:** <Standalone verification>
- **Acceptance Scenarios:**
  1. **Given** <initial state>, **When** <user/system action>, **Then** <observable outcome>.

## 2. Edge Cases & Boundaries
- Boundary input / empty / large payload behavior:
- Error, timeout, concurrency, or rollback behavior:

## 3. Functional Requirements (`What`, Tech-Agnostic)
Mark material ambiguities with `[NEEDS CLARIFICATION: <specific question>]` (limit to at most 3 critical questions; resolve all before planning):

- **FR-001:** System MUST <verifiable behavior>.
- **FR-002:** System MUST <verifiable behavior>.

## 4. Non-Goals / Out of Scope
- Explicitly excluded behavior or deferred modules:

## 5. Measurable Success Criteria
- **SC-001:** <Measurable pass condition, latency/correctness/UX metric>.
- **SC-002:** <Measurable pass condition>.
