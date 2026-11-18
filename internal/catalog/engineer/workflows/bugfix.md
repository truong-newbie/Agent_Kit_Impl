# Bugfix Workflow

1. **Scout (`scout`):** Locate the failing module, callers, cross-service boundaries, and existing tests (`read-only`).
2. **Debug (`debug`):** Collect exact failure logs/symptoms, isolate the root cause with verified evidence, and avoid guessing.
3. **Red Test (`test`):** Add a minimal reproduction test proving the bug fails before editing production logic.
4. **Fix (`implement`):** Apply the minimal safe fix matching local zone conventions without suppressing errors or weakening existing tests.
5. **Review & Verify (`review`):** Audit the diff for regressions/edge cases and run `dtkit check` / `dtkit guard verify` after final edits.
