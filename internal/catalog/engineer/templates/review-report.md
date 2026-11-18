# Code & Security Review Report Template

## 1. Review Scope
- Target branch / diff / files inspected:
- Base assumption or areas not inspected:

## 2. Prioritized Findings
For each confirmed defect or vulnerability, include:
- **Severity:** Critical | High | Medium | Low
- **Category:** Correctness | Concurrency | Cross-Service Contract | Security | Test Integrity
- **Location:** `<path>:<line>`
- **Evidence:** Observed behavior vs expected invariant
- **Impact & Remediation:** Concrete fix direction

## 3. Cross-Zone & Security Checklist
- [ ] Input validation, path traversal, injection (SQL/Cmd/XSS/SSRF), and secret redaction checked
- [ ] AuthN/AuthZ boundaries enforced at entrypoints and server actions/handlers
- [ ] Cross-service schemas/types (FE <-> BE <-> AI/Worker) remain compatible
- [ ] Concurrency, timeouts, error propagation, and resource cleanup verified
- [ ] Tests cover normal + failure/edge cases without skipped/weakened assertions

## 4. Testing Gaps & Verdict
- Remaining missing checks or environment limitations:
- Verdict: Approve | Request Changes
