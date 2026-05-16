---
status: completed
summary: Wired verdict parser and SubmitReview into main.go with --comment-only flag
container: code-reviewer-016-wire-verdict-into-main
dark-factory-version: v0.16.0
created: "2026-03-04T23:30:30Z"
queued: "2026-03-04T23:30:30Z"
started: "2026-03-04T23:30:30Z"
completed: "2026-03-04T23:35:00Z"
---
<objective>
Wire the verdict parser and SubmitReview into main.go. Add --comment-only flag. Replace plain PostComment with verdict-based review submission.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read ALL markdown files in ~/Documents/workspaces/coding-guidelines/ for Go patterns.
Read main.go for the current wiring and runReviewAndPost function.
Read pkg/verdict/verdict.go for the Parse function and Verdict type.
Read pkg/github/client.go for the SubmitReview method.
Precondition: pkg/verdict has Parse() returning Result with Verdict and Reason.
Precondition: Client has SubmitReview(ctx, owner, repo, number, body, verdict) method.
Precondition: Client has PostComment(ctx, owner, repo, number, body) for fallback.
</context>

<requirements>
1. Add `--comment-only` flag to main.go:
   - `flag.Bool("comment-only", false, "skip verdict, post as plain comment")`
   - Pass through to `run()` and `runReviewAndPost()`
   - Update usage: `"usage: code-reviewer [-v] [--comment-only] <pr-url>"`

2. Update `runReviewAndPost` to use verdict:
   - After getting reviewText, call `verdict.Parse(reviewText)`
   - Log verdict to stderr: `logAlways("verdict: %s (%s)", result.Verdict, result.Reason)`
   - If `--comment-only` flag is set: use PostComment (current behavior, skip verdict)
   - If verdict is `VerdictApprove` or `VerdictRequestChanges`: use `ghClient.SubmitReview()`
   - If verdict is `VerdictComment`: use `PostComment()` (fallback)
   - Log the action: `logAlways("submitting review: %s...", result.Verdict)` or `logAlways("posting comment...")`

3. Still print review text to stdout (unchanged).

4. Update runReviewAndPost signature — add `commentOnly bool` parameter.
</requirements>

<constraints>
- Import verdict package: `github.com/bborbe/maintainer/pkg/verdict`
- --comment-only takes precedence over any verdict
- Do NOT change pkg/ packages — only main.go changes
- Use Ginkgo v2 + Gomega for any new tests
</constraints>

<verification>
Run `make test` -- must pass.
Run `make precommit` -- must pass.
</verification>

<success_criteria>
- --comment-only flag available and documented in usage
- Verdict parsed and logged to stderr
- VerdictApprove → gh pr review --approve
- VerdictRequestChanges → gh pr review --request-changes
- VerdictComment → plain PostComment (fallback)
- --comment-only → always PostComment regardless of verdict
- make precommit passes
</success_criteria>
