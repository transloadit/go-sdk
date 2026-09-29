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
- [x] Exact-head CI on `49ccde560e3ec9ed3d730030eab36135b8a8eddb`: all five Go versions
      (1.15, 1.20, 1.24, 1.25 and 1.26) pass in run `36624253223`.
- [x] Triage the completed Opus report: document additive public-uploader admission and the complete
      upload deadline. Call out the existing Smart CDN helper's changed canonical URL/signature
      output in the PR release notes, not just the README.
- [ ] Complete multi-model council. This attempt's Codex reviewer and arbiter hit provider quota;
      the Opus report alone is not council approval. Retry when capacity is available.
- [ ] Post-documentation checks and exact-head CI.

Review dispositions: the overall context deadline intentionally wins over a longer `Retry-After`;
returning a retryable response error instead would obscure the caller's exhausted budget. A local
closure naming preference is not a correctness defect. Maintainer checklists are not customer
schema prose. Root credentialed integration tests already require credentials on main; do not
borrow those credentials or turn their absence into a fake passing test.

Follow up separately on accepting unrelated empty tus metadata entries and deterministic multipart
map ordering. These do not explain the finality defect or occur in the canary's generated metadata;
retain them as interoperability questions to test before releasing the draft SDK.

Canonical program history and cross-repository receipts remain in API2's
`docs/prompts/2026-07-09-handover-sdks-branch-restructure.md` on `sdk-next`.
No new live credentialed tests, merge or release in this follow-up.

https://github.com/transloadit/go-sdk/pull/47
