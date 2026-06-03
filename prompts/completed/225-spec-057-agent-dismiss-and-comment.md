---
status: completed
spec: [057-github-releaser-ai-review-phase]
container: maintainer-ai-review-actionable-exec-219-spec-056-agent-dismiss-and-comment
dark-factory-version: v0.173.0
created: "2026-06-01T00:00:00Z"
queued: "2026-05-31T22:41:32Z"
started: "2026-05-31T22:41:34Z"
completed: "2026-05-31T23:06:13Z"
branch: dark-factory/ai-review-actionable
---

<summary>
- When `ai_review` flags its own review as hallucinated (`verdict: fail` with `hallucinations` non-empty), the agent dismisses that review on GitHub and posts a follow-up COMMENT citing each hallucination.
- After dismissal, `gh pr view <n> --json reviewDecision` returns `REVIEW_REQUIRED` (not `CHANGES_REQUESTED`), unblocking the merge gate without operator admin-merge.
- Clean reviews and `verdict: fail` with empty hallucinations behave exactly as today — no dismissal, no COMMENT.
- The `## Diagnostics` section records every dismissal attempt (success or failure) as a YAML block with `step:` and `http_status:` keys.
</summary>

<objective>
Implement the dismiss-and-comment behavior in the pr-reviewer agent. When `ai_review` returns `verdict: fail` with at least one hallucination, dismiss the bot's review at the current head SHA via GitHub REST and post a follow-up COMMENT citing the hallucinations. Route to `human_review` regardless of dismissal outcome. All other paths are unchanged.
</objective>

<context>
Read these files before making changes:
- `/workspace/agent/pr-reviewer/pkg/poster_types.go` — defines `PrPoster` interface + `PostResult` + `ErrorClass`
- `/workspace/agent/pr-reviewer/pkg/githubposter/poster.go` — existing dismiss path: `listBotReviews`, `dismissPriorReviews`, `dismissOne`, `doRequest`, `retryCall`, `buildFailedResult`, `maxGitHubCommentBody`
- `/workspace/agent/pr-reviewer/pkg/githubposter/poster_test.go` — existing httptest-driven test patterns (the model for the new contract tests)
- `/workspace/agent/pr-reviewer/pkg/steps_review.go` — `reviewStep.Run`, `verdictPayload`, `extractVerdict`, `appendVerifyDiagnostic`, `githubPRURLPattern`
- `/workspace/agent/pr-reviewer/pkg/steps_review_test.go` — existing reviewStep test structure (mocked `PrPoster`)
- `/workspace/agent/pr-reviewer/pkg/factory/factory.go` — factory wiring; `NewReviewStep` call site
- `/workspace/agent/pr-reviewer/pkg/export_test.go` — export-for-test pattern
- `/workspace/agent/pr-reviewer/mocks/pr-poster.go` — counterfeiter mock regenerated via `go generate`

Project-wide patterns to follow (already pervasive in this codebase — verify by `rg` if unsure):
- Error wrapping: `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors`. Never `fmt.Errorf`.
- Factory: `Create*` in `pkg/factory/factory.go` calls `New*` constructors. Factory holds no logic and returns no `error`.
- Tests: Ginkgo v2 + Gomega + Counterfeiter mocks. Boundary HTTP tests use `httptest.Server`.
- HTTP: all GitHub REST calls go through `doRequest` + `retryCall`. No raw `http.DefaultClient`.
</context>

<requirements>

1. **Add `Hallucination` struct and `DismissCurrentReview` method to `pkg/poster_types.go`.**

   Add the `Hallucination` struct (this lives at the poster boundary because the poster is what renders it on the wire):

   ```go
   // Hallucination represents a single hallucination entry from the
   // ai_review verdict payload. Rendered verbatim in the dismissal
   // COMMENT body and in the Parked-Because section spawned downstream.
   type Hallucination struct {
       File  string `json:"file"`
       Line  int    `json:"line"`
       Issue string `json:"issue"`
   }
   ```

   Add the new method to the `PrPoster` interface (after `Post` and `PostLGTM`):

   ```go
   // DismissCurrentReview dismisses the bot's APPROVED or CHANGES_REQUESTED
   // review at the current head SHA, then posts a follow-up COMMENT review
   // citing each hallucination. A no-matching-review case is a non-error
   // no-op (returns success with FailureStep="dismiss-current-noop"). A
   // dismissal failure returns a failed PostResult; a COMMENT-post failure
   // after a successful dismissal still returns success — the merge gate
   // is already cleared.
   DismissCurrentReview(
       ctx context.Context,
       pr prurl.PRInfo,
       headSHA, botLogin string,
       hallucinations []Hallucination,
   ) PostResult
   ```

2. **Implement `DismissCurrentReview` in `pkg/githubposter/poster.go`.**

   Behavior (the test in step 7 is the binding contract):

   (a) List the bot's reviews at the PR. Filter to `r.User.Login == botLogin AND r.CommitID == headSHA AND r.State in {"APPROVED", "CHANGES_REQUESTED"}`. (Do NOT modify `listBotReviews` — its existing filter rejects current-head-SHA reviews on purpose. Use a new package-private helper, e.g. `listBotReviewsAtHead`, or inline the filter in this method.)

   (b) If `headSHA` is empty OR no matching review is found, return `PostResult{Outcome: "success", FailureStep: "dismiss-current-noop"}`. No HTTP call.

   (c) Call `PUT /repos/{owner}/{repo}/pulls/{n}/reviews/{review_id}/dismissals` with JSON body `{"message":"hallucinated review — see follow-up COMMENT for evidence"}`. Use `doRequest` + `retryCall`. On non-2xx return `buildFailedResult(...)` with `FailureStep="PUT /pulls/N/reviews/M/dismissals"`. Do NOT proceed to the COMMENT POST on dismissal failure.

   (d) On 2xx dismissal, post a follow-up COMMENT review via `POST /repos/{owner}/{repo}/pulls/{n}/reviews` with body:
   ```json
   {"event": "COMMENT", "commit_id": "<headSHA>", "body": "<formatted body>"}
   ```
   Build the body via the new package-private helper `buildHallucinationCommentBody(hallucinations []prpkg.Hallucination) string` (where `prpkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"` — the existing import alias). Truncate to `maxGitHubCommentBody` using the existing truncation pattern visible in `poster.go`.

   On COMMENT POST non-2xx after a successful dismissal: return `PostResult{Outcome: "success", FailureStep: "POST /pulls/N/reviews (comment-after-dismiss)", HTTPStatus: <status>, ErrorMessage: <truncated body>}`. `Outcome=success` is intentional — the dismissal already cleared the merge gate and the COMMENT failure is logged in `## Diagnostics` for the operator.

   (e) On full success return `PostResult{Outcome: "success", FailureStep: "", HTTPStatus: 200}`.

   `buildHallucinationCommentBody` (package-private in `poster.go`):

   ```go
   func buildHallucinationCommentBody(hallucinations []prpkg.Hallucination) string {
       if len(hallucinations) == 0 {
           return ""
       }
       var sb strings.Builder
       sb.WriteString("hallucinated review — see follow-up COMMENT for evidence\n\n")
       for _, h := range hallucinations {
           sb.WriteString(fmt.Sprintf("- %s:%d — %s\n", h.File, h.Line, h.Issue))
       }
       return sb.String()
   }
   ```

   This is the ONLY definition of the body-formatting helper. Do NOT duplicate it in `pkg/steps_review.go` — the poster owns the wire format end-to-end.

3. **Update `pkg/steps_review.go` — extend `verdictPayload`, add `appendDismissDiagnostic`, wire the dismiss-and-comment call.**

   Extend `verdictPayload` (note: this file is `package pkg`, so `Hallucination` resolves unqualified):

   ```go
   type verdictPayload struct {
       Verdict        string          `json:"verdict"`
       Reason         string          `json:"reason"`
       Hallucinations []Hallucination `json:"hallucinations"`
   }
   ```

   Add `poster PrPoster` to `reviewStep`:

   ```go
   type reviewStep struct {
       runner       claudelib.ClaudeRunner
       poster       PrPoster
       instructions claudelib.Instructions
       verifier     ReviewVerifier
       ghToken      string
       botLogin     string
   }
   ```

   Update `NewReviewStep` signature (add `poster PrPoster` as 2nd parameter, right after `runner`):

   ```go
   func NewReviewStep(
       runner claudelib.ClaudeRunner,
       poster PrPoster,
       instructions claudelib.Instructions,
       verifier ReviewVerifier,
       ghToken string,
       botLogin string,
   ) agentlib.Step {
       return &reviewStep{
           runner:       runner,
           poster:       poster,
           instructions: instructions,
           verifier:     verifier,
           ghToken:      ghToken,
           botLogin:     botLogin,
       }
   }
   ```

   In `reviewStep.Run`, after `verdict, err := extractVerdict(...)` succeeds (i.e. after the existing `if err != nil` block at line ~128) AND before the existing `if verdict.Verdict == "pass"` block at line ~137, insert:

   ```go
   if verdict.Verdict == "fail" && len(verdict.Hallucinations) > 0 {
       s.tryDismissHallucinated(ctx, md, verdict.Hallucinations)
   }
   ```

   Add the helper method on `reviewStep` (sibling to `callVerifier`):

   ```go
   // tryDismissHallucinated dismisses the bot's hallucinated review on
   // the current head SHA and posts a follow-up COMMENT. Routing to
   // human_review still happens unconditionally in the caller — this
   // helper only mutates ## Diagnostics with the dismiss outcome.
   func (s *reviewStep) tryDismissHallucinated(
       ctx context.Context,
       md *agentlib.Markdown,
       hallucinations []Hallucination,
   ) {
       prURLStr := githubPRURLPattern.FindString(md.Preamble)
       if prURLStr == "" {
           glog.V(2).Infof("ai_review dismiss: no GitHub PR URL — skipping")
           return
       }
       prInfo, err := prurl.ParsePRURL(ctx, prURLStr)
       if err != nil || prInfo.Platform != prurl.PlatformGitHub {
           glog.V(2).Infof("ai_review dismiss: non-GitHub or unparseable URL — skipping")
           return
       }
       headSHA, _ := md.Frontmatter.String("ref")
       if headSHA == "" {
           glog.Warningf("ai_review dismiss: empty ref in frontmatter — skipping")
           return
       }
       result := s.poster.DismissCurrentReview(ctx, *prInfo, headSHA, s.botLogin, hallucinations)
       appendDismissDiagnostic(md, result)
   }
   ```

   Add `appendDismissDiagnostic` (always emits, regardless of outcome — the spec's AC requires evidence of every attempt):

   ```go
   // appendDismissDiagnostic appends a YAML block describing the dismiss
   // attempt to ## Diagnostics. Called on every attempt (success and
   // failure) so AC verification can grep for the step + http_status.
   func appendDismissDiagnostic(md *agentlib.Markdown, result pkg.PostResult) {
       block := fmt.Sprintf(
           "ai_review dismiss:\n  outcome: %q\n  step: %q\n  http_status: %d\n  error: %q\n",
           result.Outcome,
           result.FailureStep,
           result.HTTPStatus,
           result.ErrorMessage,
       )
       var existingBody string
       if existing, ok := md.FindSection("## Diagnostics"); ok && existing != nil {
           existingBody = existing.Body
       }
       newBody := strings.TrimLeft(existingBody+"\n"+block, "\n")
       md.ReplaceSection(agentlib.Section{Heading: "## Diagnostics", Body: newBody})
   }
   ```

   (Note: `pkg.PostResult` qualifier is incorrect inside `package pkg` — drop the qualifier and write `result PostResult`. The snippet above shows `pkg.PostResult` only because the same code is referenced from `export_test.go` in step 5. Inside `steps_review.go`, the signature is `func appendDismissDiagnostic(md *agentlib.Markdown, result PostResult)`.)

4. **Wire the poster into `NewReviewStep` in `pkg/factory/factory.go`.**

   Locate the existing `prpkg.NewReviewStep(...)` call inside `CreateAgent` and add `prPoster` as the 2nd argument:

   ```go
   reviewStep := prpkg.NewReviewStep(
       CreateClaudeRunner(claudeConfigDir, agentDir, model, env, reviewTools),
       prPoster,
       prompts.BuildReviewInstructions(),
       verifier,
       ghToken,
       botLogin,
   )
   ```

   There is no `pkg/factory/factory_mocks.go` — counterfeiter mocks for `PrPoster` live at `mocks/pr-poster.go` and are regenerated by step 8.

5. **Add export-for-test helper in `pkg/export_test.go`:**

   ```go
   // AppendDismissDiagnosticForTest exposes appendDismissDiagnostic to the
   // _test package.
   func AppendDismissDiagnosticForTest(md *agentlib.Markdown, result PostResult) {
       appendDismissDiagnostic(md, result)
   }
   ```

6. **Extend `pkg/steps_review_test.go` — `reviewStep` test suite.**

   Add a new `Describe` covering the dismiss-and-comment routing through a mocked `PrPoster`:

   (a) **verdict=fail with hallucinations → dismiss is called and diagnostic recorded.**
       Mock runner returns `{"verdict":"fail","reason":"line 99 not in diff","hallucinations":[{"file":"pkg/foo.go","line":99,"issue":"line 99 not in diff"}]}`. Mock `poster.DismissCurrentReview` returns success.
       Assert: poster called exactly once with hallucinations matching. `result.NextPhase == "human_review"`. `## Diagnostics` body contains the substring `step: "dismiss-current-noop"` OR a success step (per the mock's PostResult). The exact substring is `outcome: "success"`.

   (b) **verdict=fail with hallucinations + dismiss returns 404** (review_id stale).
       Mock `DismissCurrentReview` returns `PostResult{Outcome:"failed", FailureStep:"PUT /pulls/N/reviews/M/dismissals", HTTPStatus:404}`.
       Assert: `result.NextPhase == "human_review"`. `## Diagnostics` contains `step: "PUT /pulls/N/reviews/M/dismissals"` AND `http_status: 404`.

   (c) **verdict=fail with hallucinations + dismiss returns 422** (review already COMMENTED).
       Mock returns `PostResult{Outcome:"failed", FailureStep:"PUT /pulls/N/reviews/M/dismissals", HTTPStatus:422}`.
       Assert: `result.NextPhase == "human_review"`. Diagnostics contains `http_status: 422`.

   (d) **verdict=fail with hallucinations + dismiss success + COMMENT POST fails** (partial state).
       Mock returns `PostResult{Outcome:"success", FailureStep:"POST /pulls/N/reviews (comment-after-dismiss)", HTTPStatus:500}`.
       Assert: `result.NextPhase == "human_review"`. Diagnostics contains the partial-state step.

   (e) **verdict=fail with empty hallucinations** (e.g. `verdict_consistency: inconsistent`).
       Mock runner returns `{"verdict":"fail","reason":"inconsistent","hallucinations":[]}`.
       Assert: `poster.DismissCurrentReview` called **zero times** (counterfeiter recorder). `result.NextPhase == "human_review"`. `## Diagnostics` does NOT contain `ai_review dismiss:`.

   (f) **verdict=pass**.
       Mock runner returns `{"verdict":"pass","reason":"looks good","hallucinations":[]}`.
       Assert: poster called zero times. `result.NextPhase == "done"`.

   (g) **Non-GitHub PR URL skips dismissal**.
       Preamble contains a Bitbucket URL; mock runner returns fail+hallucinations.
       Assert: poster called zero times. `result.NextPhase == "human_review"`.

   (h) **Empty `ref` frontmatter skips dismissal**.
       Frontmatter omits `ref`; mock runner returns fail+hallucinations.
       Assert: poster called zero times. `result.NextPhase == "human_review"`.

7. **Add `httptest`-driven contract tests for `DismissCurrentReview` in `pkg/githubposter/poster_test.go`.**

   Follow the existing `httptest.NewServer` + recorded-request pattern visible elsewhere in `poster_test.go`. Each case asserts URL path, HTTP method, request body, and the returned `PostResult`. Cases:

   (a) **Full success.** Reviews endpoint returns one APPROVED review at `headSHA` by `botLogin`. Dismiss endpoint returns 200. Reviews POST returns 201.
       Assert: dismiss request URL is `PUT /repos/{owner}/{repo}/pulls/{n}/reviews/{id}/dismissals`. Request body JSON is `{"message":"hallucinated review — see follow-up COMMENT for evidence"}`. COMMENT POST URL is `POST /repos/{owner}/{repo}/pulls/{n}/reviews`. COMMENT body JSON has `event: "COMMENT"`, `commit_id: headSHA`, and `body` starting with `"hallucinated review"` and containing one `"- pkg/foo.go:99 — ..."` bullet. Return value: `Outcome: "success", HTTPStatus: 200`.

   (b) **No matching review (no-op).** Reviews endpoint returns reviews only at OTHER SHAs (or only COMMENT-state).
       Assert: zero dismiss requests, zero COMMENT requests. Return: `Outcome: "success", FailureStep: "dismiss-current-noop"`.

   (c) **Empty `headSHA`.** Caller passes `headSHA: ""`.
       Assert: zero HTTP requests (early return). Return: `Outcome: "success", FailureStep: "dismiss-current-noop"`.

   (d) **Dismiss returns 404.**
       Reviews returns a match; dismiss endpoint returns 404.
       Assert: zero COMMENT requests. Return: `Outcome: "failed", FailureStep: "PUT /pulls/N/reviews/M/dismissals", HTTPStatus: 404`.

   (e) **Dismiss returns 422.**
       Reviews returns a match; dismiss endpoint returns 422 with body `Can not dismiss a commented pull request review`.
       Assert: zero COMMENT requests. Return: `Outcome: "failed", HTTPStatus: 422`.

   (f) **Dismiss 5xx exhausted.**
       Reviews returns a match; dismiss endpoint returns 503 on every call (let `retryCall` exhaust).
       Assert: zero COMMENT requests after exhaustion. Return: `Outcome: "failed", HTTPStatus: 503`.

   (g) **Dismiss 200, COMMENT POST 500.**
       Reviews returns a match; dismiss 200; COMMENT POST returns 500.
       Assert: Return: `Outcome: "success", FailureStep: "POST /pulls/N/reviews (comment-after-dismiss)", HTTPStatus: 500`. (Success despite COMMENT failure — dismissal already cleared the gate.)

   (h) **Other-bot review at head SHA is NOT dismissed** (security invariant).
       Reviews returns one CHANGES_REQUESTED review at headSHA by `User.Login == "some-other-bot"`.
       Assert: zero dismiss requests. Return: `Outcome: "success", FailureStep: "dismiss-current-noop"`.

8. **Verify the frozen `dismissPriorReviews` invariant.**

   Run `cd /workspace/agent/pr-reviewer && make test` after step 7. The existing test in `poster_test.go` that asserts "review at current head SHA is preserved by `dismissPriorReviews`" must continue to pass without modification. If it fails, you broke the frozen invariant — fix the production code, do NOT edit the test.

9. **Regenerate mocks.**

   Run `cd /workspace/agent/pr-reviewer && go generate ./...` to regenerate `mocks/pr-poster.go` with the new `DismissCurrentReview` method.

10. **Final verification.**

    Run `cd /workspace/agent/pr-reviewer && make precommit && make test`. Exit code must be 0. If any test fails, diagnose and fix.

</requirements>

<constraints>
- **Frozen** `dismissPriorReviews` semantics: reviews at the current head SHA must remain non-dismissable by that path. Do NOT modify `listBotReviews`.
- **Frozen** `PrPoster` interface for existing methods (`Post`, `PostLGTM`). New method is additive.
- **Frozen** JSON schema in `pkg/prompts/review_output-format.md`: the wire format is unchanged. Only the Go-side `verdictPayload` parser gains a field.
- **Frozen** factory invariant: factory contains zero business logic and returns no `error`. New constructors are `New*` in `pkg`, wired by `Create*` in factory.
- **Frozen** error wrapping: `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors`. Never `fmt.Errorf`.
- **Frozen** HTTP plumbing: all GitHub REST calls go through `doRequest` + `retryCall`. No raw `http.DefaultClient`. No new HTTP client construction.
- Other-bot reviews at the current head SHA must NOT be dismissed — the filter `r.User.Login == botLogin` is a security boundary, not a performance optimisation. Step 7 case (h) is the contract test for this.
- Body builder `buildHallucinationCommentBody` exists in exactly ONE location: `pkg/githubposter/poster.go`. Do not duplicate it in `steps_review.go`.
- `NewReviewStep` signature change requires updating the factory call site in the same prompt. Do not leave a compile-broken commit.
</constraints>

<verification>
```bash
cd /workspace/agent/pr-reviewer && go generate ./... && make precommit && make test
```

Expected: exit code 0. Specifically:
- `go generate` regenerates `mocks/pr-poster.go` with `DismissCurrentReview`.
- `go test ./pkg/...` — new `reviewStep` cases (a)-(h) and new `DismissCurrentReview` httptest cases (a)-(h) all pass.
- Existing `dismissPriorReviews` "preserves current head SHA" test still passes (frozen invariant).
- `make precommit` lints clean.
</verification>
