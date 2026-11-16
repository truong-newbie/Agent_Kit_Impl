# Bug fix workflow

1. Gather the symptom, reproduction steps, and environment context (`debug`).
2. Trace the code, form falsifiable hypotheses, and isolate the verified root cause.
3. Confirm whether a fix is authorized (stop at diagnosis if diagnose-only).
4. Implement the smallest safe fix (`implement`) and add regression tests (`test`).
5. Review the diff (`review`) and report root cause, fix, and verified test evidence (`completion-report.md`).
