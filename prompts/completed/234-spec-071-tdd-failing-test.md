---
status: completed
spec: ["071"]
container: maintainer-pr-verdict-exec-234-spec-060-tdd-failing-test
dark-factory-version: v0.175.0
created: "2026-06-03T15:10:31Z"
queued: "2026-06-03T15:48:33Z"
started: "2026-06-03T15:48:34Z"
completed: "2026-06-03T15:53:01Z"
branch: dark-factory/pr-reviewer-verdict-decides-review-event
---

<summary>
- Add a failing Ginkgo Entry that asserts verdict `approve` with `autoApprove=false` maps to the GitHub review event `APPROVE` (state `APPROVED`) — currently the production code emits `COMMENT`/`COMMENTED`, so the new entry fails on first run. This locks down the regression at the unit boundary BEFORE the production fix lands.
- Add a failing Ginkgo spec in `verifier_test.go` that drives `VerifyReview` with `ExpectedStates=["COMMENTED"]` only and asserts the verifier returns `Found:false` with a phantom-POST failure — the production allow-list drop is verified in a separate unit test rather than baking the assumption into the production code.
- The TDD commit is the first of two for spec 060: a `test:` commit that contains the new failing assertions, with the production fix deliberately absent. The follow-up `fix:` commit is a sibling prompt.
- No production code is changed in this prompt — `poster.go`, `verifier.go`, `config.go`, and `steps_review.go` are read-only references.
- After this prompt ships, `go test ./pkg/githubposter/...` exits non-zero (the new entries fail). The expected-failing exit code is the AC for the TDD step.
</summary>

<objective>
Land the failing-test half of the TDD cycle for spec 060 (pr-reviewer verdict decides review event). The new Entry in the `verdict to event/state mapping` `DescribeTable` asserts the post-fix contract; the new `verifier_test.go` spec asserts the post-fix allow-list behavior. Both must fail against the current production code; both must pass after the sibling fix-prompt lands. No production code changes in this prompt.
</objective>

<context>
Read `CLAUDE.md` at the repo root AND `agent/pr-reviewer/CLAUDE.md` (if it exists; otherwise the project-level CLAUDE.md is the only one).

Read these files fully BEFORE editing (anchors and patterns, not line numbers — lines go stale):

- `agent/pr-reviewer/pkg/githubposter/poster.go` — the `mapVerdictAndSummary` function (the target of the production fix in the sibling prompt) and the existing `verdict to event/state mapping` `DescribeTable` in `poster_test.go`. The current Entry "approve+autoApprove:false → COMMENT" is what the new Entry replaces; the helper sequence for `happySpecs(state)` and `writeYAML(autoApprove)` is the scaffolding to mirror.
- `agent/pr-reviewer/pkg/githubposter/verifier_test.go` — the existing `ReviewVerifier` Describe and the `req(states ...string)` helper (line ~43). The new spec is added inside the same `Describe`.
- `agent/pr-reviewer/pkg/githubposter/poster_test.go` — full file. The new Entry is added to the `DescribeTable("verdict to event/state mapping", ...)` block; the existing Entries for `approve+autoApprove:true → APPROVE` and `request-changes → REQUEST_CHANGES` are kept verbatim (sibling fix-prompt removes the COMMENT Entry).
- `agent/pr-reviewer/pkg/steps_review.go` — read ONLY for context. The `ExpectedStates: []string{"APPROVED", "CHANGES_REQUESTED", "COMMENTED"}` literal at the call site (around line 241) is what the sibling fix-prompt edits. Do NOT edit it in this prompt.
- `agent/pr-reviewer/pkg/verdict.go` — read for the `Verdict` type and the two constants `VerdictApprove` ("approve") and `VerdictRequestChanges` ("request-changes"). Do NOT add new constants.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2/Gomega conventions, `DescribeTable` + `Entry` shape, `BeforeEach` setup, `seqStub` usage.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — counterfeiter / mock pattern (the existing tests use `mocks.HTTPClient`).

Verified symbols (do NOT change in this prompt — they are the production anchors):
- `prpkg.VerdictApprove` (`agent/pr-reviewer/pkg/verdict.go:16`) — value `Verdict("approve")`. Used as the first argument to `mapVerdictAndSummary` in the production code.
- `prpkg.VerdictRequestChanges` (`agent/pr-reviewer/pkg/verdict.go:17`) — value `Verdict("request-changes")`. The unchanged existing Entry.
- `githubposter.EventToStateForTest` (`agent/pr-reviewer/pkg/githubposter/export_test.go:22`) — exposes `eventToState` for unit tests. The happy-path `seqStub(happySpecs(wantState))` pattern in `poster_test.go:134-140` already uses this mapping.
- `pkg.PostRequest` and `pkg.PostResult` types in `agent/pr-reviewer/pkg/poster_types.go` — `PostedEvent` field on `PostResult` is the value the new Entry asserts.
- `pkg.VerifyRequest.ExpectedStates` (`agent/pr-reviewer/pkg/poster_types.go:103`) — `[]string` field the new verifier test uses to drive the contract.
- `errPhantomPOST` is the sentinel for the "no matching bot review" path in `verifier.go` — the new verifier spec asserts the same phantom behavior when the allow-list excludes the only state the bot actually wrote.

Verified existing test scaffolding (copy the pattern, do NOT reinvent):
- `seqStub(happySpecs(state))` from `poster_test.go:71-83, 134-140` — the standard happy-path HTTP call sequence (GET list → POST review → GET list with the bot's review at the right state). Use as-is.
- `writeYAML(autoApprove)` from `poster_test.go:127-132` — writes `.maintainer.yaml` with the desired `prReviewer.autoApprove` value. Use as-is.
- The `req(states ...string) pkg.VerifyRequest` helper in `verifier_test.go:43-49` — pass the desired allow-list as variadic args.
- `mocks.HTTPClient` counterfeiter fake from `agent/pr-reviewer/mocks/http_client.go` — `DoStub = seqStub(...)` is the established stubbing style.
</context>

<requirements>

## Step 1 — Add a new `Entry` to the `verdict to event/state mapping` DescribeTable (RED)

In `agent/pr-reviewer/pkg/githubposter/poster_test.go`, inside the existing `DescribeTable("verdict to event/state mapping", ...)` block, ADD a new Entry. The block is at the file's outer `Describe("PrPoster", ...)` level (lines 142-164). Insert the new Entry immediately AFTER the existing `Entry("approve+autoApprove:false → COMMENT", ...)` Entry (currently lines 159-161). Keep all three existing Entries intact for now — the sibling fix-prompt deletes the COMMENT Entry and edits the post-test YAML. The goal here is to write the failing assertion that the post-fix code must satisfy.

The new Entry:

```go
Entry("approve+autoApprove:false → APPROVE (post-fix contract — spec 060)",
    pkg.VerdictApprove, false, "APPROVE", "APPROVED", ""),
```

Note: the `wantBodyPrefix` fifth argument is `""` for the post-fix contract because the production code no longer prepends the "auto-approve disabled for this repo" preamble. The `DescribeTable` body asserts `Expect(result.Warnings).To(BeEmpty())` when `wantBodyPrefix == ""` (line ~154), which is the correct post-fix invariant — the auto-approve preamble is gone, no soft-warning about it, body is the user-supplied `summary` verbatim.

This Entry calls the production `mapVerdictAndSummary` indirectly via `poster.Post(ctx, req)` and asserts:
- `result.Outcome` is `"success"` (the happy path is preserved).
- `result.PostedEvent` is `"APPROVE"` — load-bearing failing assertion (currently fails: the production code emits `"COMMENT"`).
- `result.ReviewID` is `42` (the happy-path stub ID).

`result.Warnings` is NOT asserted by this Entry — the "auto-approve disabled" preamble is concatenated into `body`, not into `Warnings`, so asserting `Warnings` would not differentiate pre-fix from post-fix behavior. Keep the assertion list minimal and focused on `PostedEvent`.

The expected outcome of running this Entry against today's production code: the test runner reports this Entry as FAILED with the message `Expected <string>: "APPROVE" — got "COMMENT"`. Capture the actual failure message in the completion report.

## Step 2 — Add a failing spec to `verifier_test.go` (RED for the allow-list)

In `agent/pr-reviewer/pkg/githubposter/verifier_test.go`, inside the existing `Describe("ReviewVerifier", ...)` block, ADD a new `Context` that drives the verifier with an allow-list of just `["COMMENTED"]` and asserts the verifier returns `Found:false` with a transient/phantom-POST failure. The intent: lock down the post-fix behavior that the verifier no longer considers `COMMENTED` an acceptable fresh-review state.

Insert AFTER the existing `Context("network error", ...)` block (line ~113-121), before the closing `})` of the outer `Describe`:

```go
Context("allow-list excludes COMMENTED for fresh review (spec 060)", func() {
    It("returns Found:false when only allowed state is COMMENTED", func() {
        // GitHub returns a COMMENTED review (the only kind the pre-fix
        // poster ever produced for verdict approve + autoApprove:false).
        body := reviewListJSON(reviewJSON(42, testBotLogin, testHeadSHA, "COMMENTED"))
        fakeClient.DoStub = func(_ *http.Request) (*http.Response, error) {
            return makeHTTPResp(200, body), nil
        }
        result := verifier.VerifyReview(ctx, req("COMMENTED"))
        Expect(result.Found).To(BeFalse())
        Expect(result.Outcome).To(Equal("failed"))
        Expect(result.Class).To(Equal(pkg.ErrorClassTransient))
        Expect(result.Attempt).To(Equal(2)) // parity with existing "review absent both attempts" — phantom-POST exhausts retries
    })
})
```

The expected outcome against today's production code: PASSES. The verifier today accepts `["COMMENTED"]` as a valid allow-list, and the GET returns a `COMMENTED` review at the head SHA, so `findReview` returns `true` and the verifier reports `Found:true, Outcome:success, FoundState:COMMENTED`. The new spec asserts `Found:false` and the phantom class — it will fail today.

The semantic intent of the spec: after the sibling fix-prompt lands, the production caller (`steps_review.go:241`) drops `"COMMENTED"` from the `ExpectedStates` slice, and the verifier's behavior for a `COMMENTED`-only allow-list is to treat the review as "not a match" — exactly the behavior this spec captures. The spec documents the contract even though the current caller doesn't pass this exact allow-list.

This is intentional TDD-as-documentation: the verifier's contract is "the caller's `ExpectedStates` controls what counts as a match," and the new spec pins the boundary behavior at the unit layer rather than relying on the caller-side edit to prove the drop happened.

## Step 3 — Verify the tests fail (RED) and that nothing else regresses

Run the githubposter tests from the pr-reviewer module root and confirm ONLY the new Entry and the new spec fail; every other pre-existing test continues to pass.

```
cd agent/pr-reviewer && go test ./pkg/githubposter/... 2>&1 | tee /tmp/red-output.txt
```

Acceptable failure signals:
- The new Entry "approve+autoApprove:false → APPROVE (post-fix contract — spec 060)" fails inside the `DescribeTable("verdict to event/state mapping", ...)`. The runner typically prints `Expected <string>: "APPROVE" — got "COMMENT"` (or similar) for that row.
- The new Context "allow-list excludes COMMENTED for fresh review (spec 060)" fails its `Expect(result.Found).To(BeFalse())` assertion (the current production verifier returns `Found:true`).
- All other existing tests pass — including the existing Entry "approve+autoApprove:false → COMMENT" (it is kept in this prompt and will be removed by the sibling fix-prompt).
- `go vet ./...` exits 0; `go build ./...` exits 0; the overall package build is clean.

If any pre-existing test fails, STOP and report the regression in the completion summary before committing. Do NOT modify any pre-existing test in this prompt.

## Step 4 — Commit the failing-test change (TDD trail, part 1)

Commit the change as a single `test:` commit on the spec branch. The dark-factory daemon assigns the branch; do NOT create or switch branches yourself. The commit subject MUST start with `test:` and MUST name the githubposter package and the spec — e.g.:

```
test(agent/pr-reviewer/pkg/githubposter): assert post-fix verdict→APPROVE and allow-list-excludes-COMMENTED contracts (spec 060)
```

The body MUST explain WHY the test is committed without the production fix: TDD red step for spec 060. Name the sibling fix-prompt that ships the production code.

The commit MUST touch only `agent/pr-reviewer/pkg/githubposter/poster_test.go` and `agent/pr-reviewer/pkg/githubposter/verifier_test.go`. No other file in the repo may be modified by this commit. Run `git diff --stat HEAD~1 HEAD` after the commit and confirm only the two test files appear.

</requirements>

<constraints>
- This is the RED step of a TDD cycle. The new tests MUST fail against the current production code. Do NOT modify `poster.go`, `verifier.go`, `config.go`, `steps_review.go`, or any other production file in this prompt. The fix is a sibling prompt.
- Do NOT delete or modify any pre-existing test Entry or `Context` in `poster_test.go` or `verifier_test.go` — the existing `Entry("approve+autoApprove:false → COMMENT", ...)` stays intact (the sibling fix-prompt removes it). The TDD step is purely additive.
- Tests use Ginkgo v2 + Gomega per the project guide (`/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`). No `t.Run`. New specs are added inside the existing `Describe("PrPoster", ...)` and `Describe("ReviewVerifier", ...)` blocks via `Entry` and `Context` — do NOT open a top-level `Describe`.
- Reuse existing helpers verbatim: `seqStub`, `happySpecs(state)`, `writeYAML(autoApprove)`, `req(states ...string)`, `reviewJSON`, `reviewListJSON`, `makeHTTPResp`. Do NOT introduce new helpers.
- The new Entry uses the existing `DescribeTable` signature `func(verdict pkg.Verdict, autoApprove bool, wantEvent, wantState, wantBodyPrefix string)`. Do NOT alter the signature.
- Do NOT commit anything to git. Dark-factory handles the commit (the daemon observes the workdir, generates a commit subject, and pushes). You write the code; the daemon commits.
- Do NOT regenerate counterfeiter mocks — no interface signatures changed.
- Do NOT touch `lib/`, `watcher/`, `agent/github-releaser/`, or any sibling service.
- The DARK-FACTORY-REPORT block, the Fast Feedback Command section, and the Changelog suffix are auto-injected by the daemon. Do NOT include them in this prompt's body.
- CHANGELOG: do NOT edit `CHANGELOG.md` in this prompt — the test-only change is not a user-visible changelog entry. The sibling fix-prompt owns the changelog entry.
- Atomic-batch constraint: this prompt MUST land BEFORE the sibling fix-prompt lands. The daemon's prompt ordering guarantees sequential execution when both are queued; do not assume parallel execution.
</constraints>

<verification>
```
cd agent/pr-reviewer
go test ./pkg/githubposter/... 2>&1 | tee /tmp/red-output.txt
```

Expected (REPORT these explicitly in the completion summary):

1. `go test` exits NON-ZERO (this is the TDD red step — failure is the signal).
2. The failing Entry in the output is `verdict to event/state mapping` row "approve+autoApprove:false → APPROVE (post-fix contract — spec 060)" with an assertion message naming `APPROVE` and the actual returned value `COMMENT` (or similar — quote the actual line verbatim in the completion report).
3. The failing Context in the output is "allow-list excludes COMMENTED for fresh review (spec 060)" (or its `It(...)` description) with an assertion message on `Found:false` (the production verifier currently returns `Found:true`).
4. No pre-existing test fails. Quote the count: "pre-existing tests: N passed, 0 failed" — where N is the count of all githubposter tests minus the two new ones.
5. `go build ./...` and `go vet ./...` both exit 0 (no compile errors, no vet warnings).
6. `git diff --stat HEAD~1 HEAD` (run after the commit lands) shows ONLY the two test files modified — quote the exact output.

If `go test` does NOT fail on the new Entry, STOP and report the finding — the test is not actually exercising the bug, and the TDD red step is invalid. Do NOT proceed.

If pre-existing tests fail, STOP — a regression in the scaffolding, not a TDD red signal. Report and wait.
</verification>
