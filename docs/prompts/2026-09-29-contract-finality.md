# Contract workflow finality follow-up for #47

October 6 reader compatibility, still draft and unmerged:

- [x] Reproduce future errors failing generated decoding before the workflow can inspect them.
- [x] Regenerate from API2 producer `98c3d25a8f` with scalar open-string codecs. Share one native
      state reader between waiting/cancellation and tus. Exact values stay structured data;
      stopped-upload diagnostic messages remain generic.
- [x] Pass all 160 shared reader/mode observations, complete contract/shared workflow race
      tests, vet and example builds. Correct the old stop-message assertion to require a generic
      cause while retaining the exact structured code; keep the strict root fixture decoder.
- [ ] Finish council, refreshed producer pins/runtime acceptance and exact-head CI.
      No merge, release or new live-test budget.
- API2's existing `docs/prompts/2026-07-09-handover-sdks-branch-restructure.md` is the canonical
  program record; this file tracks only the native PR's validation.

October 4 final confirmation correction, still draft and unmerged:

- [x] Source-access-verified combined council found invalid confirmation inspection still hid
      the failed DELETE. Both transport and HTTP cases fail before the repair; explicit caller
      cancellation remains a separate control.
- [x] Handle confirmation reading and inspection in the same compound-error boundary. Retain
      cancellation-first `errors.Is`/`errors.As`, direct access to both errors, deadline precedence
      and the Go 1.15 floor. No repeated DELETE or weakened response/destination validation.
- [x] Full native contract race tests pass after the repair.
- [ ] Repeat shared race/vet/example checks, freeze sources and update API2 pins. Repeat actual
      local API2/tusd acceptance, post-fix council and exact-head CI. Final PR receipts supersede
      this pre-push snapshot; no merge/release or additional live-reader budget.

October 4 identity follow-up, still draft and unmerged:

- [x] Main is already an ancestor; regenerate from the integrated API2 contract.
- [x] Reproduce unrelated-empty metadata rejection and noncanonical padding acceptance first.
- [x] Consume owner grammar, strict Base64 bytes and generated receipt accessors, not wire fields.
- [x] All 26 shared cases and native contract/shared race tests, vet and examples build pass.
- [ ] Refresh source pins, repeat actual local API2/tusd proof, complete fresh council and exact-head
      CI. Historical acceptance below does not certify this candidate.

Receipt field names and unrelated empty tus metadata below are historical open items now repaired;
the deployed raw parser, Go 1.15 floor, legacy APIs and live-reader budgets are unchanged.

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
