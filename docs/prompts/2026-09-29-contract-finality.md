# Contract workflow finality follow-up for #47

Why: runtime investigation distinguished a finite client outcome from backend cleanup. The user
approved returning typed `REQUEST_ABORTED` from ordinary waiting, while still attempting explicit
cancellation and preserving a failed cancellation request. This is not merge/release approval.

- [x] Read PR context; no outstanding review threads. Preserve existing SDK APIs and native tus safety.
- [x] Reproduce the new lifecycle cases against the old implementation before changing it.
- [x] Implement finite ordinary wait, explicit cancel after an aborted connection, and preservation
      of a failed DELETE when a later GET still reports `REQUEST_ABORTED`.
- [x] Confirm latest main is already integrated.
- [x] Regenerate from committed API2 producer `f13c20fe24`; generated contract SHA-256
      `a72c65901795052a96036a2fa6beed7ac1885299fb3d6dfec8a011e2efc6f819`.
- [x] Native and shared workflow race tests, example tests/build, opt-in import guard and `go vet`.
- [ ] Council-review, fix valid findings, rerun checks, push and monitor all five Go CI versions.

Canonical program history and cross-repository receipts remain in API2's
`docs/prompts/2026-07-09-handover-sdks-branch-restructure.md` on `sdk-next`.
No new live credentialed tests, merge or release in this follow-up.

https://github.com/transloadit/go-sdk/pull/47
