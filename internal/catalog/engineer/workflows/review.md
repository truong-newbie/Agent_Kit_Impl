# Review Workflow

1. Confirm target diff, commit base, or file scope (`read-only`).
2. Inspect modified code alongside callers, cross-zone contracts, and test files.
3. Prioritize verified defects: correctness, regressions, security boundaries (injection, traversal, auth, secrets), and missing tests.
4. Report evidence-backed findings using `review-report.md` without modifying repository files.
