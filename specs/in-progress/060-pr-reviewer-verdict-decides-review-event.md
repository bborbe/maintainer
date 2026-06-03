---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-06-03T15:04:32Z"
generating: "2026-06-03T15:05:58Z"
prompted: "2026-06-03T15:22:31Z"
branch: dark-factory/pr-reviewer-verdict-decides-review-event
---

## Summary

- PR Reviewer's `githubposter` currently posts a GitHub review event of `COMMENT` when the verdict is approve and the per-repo `autoApprove` config is off. GitHub records that as `COMMENTED`, which does not satisfy branch protection's "approving review" requirement.
- Result: docs-only PRs (zero findings → verdict approve) sit at `reviewDecision: REVIEW_REQUIRED` and cannot be merged without admin override.
- Fix: make verdict alone decide the review event. Approve verdict → `APPROVE`. Request-changes verdict → `REQUEST_CHANGES`. No third branch, no `COMMENT` fallback.
- Verifier's allow-list of expected review states is updated to drop `COMMENTED`, since the poster will no longer produce it.
- Scope is intentionally three files inside `agent/pr-reviewer/pkg/githubposter/`. Other callers of `autoApprove` outside this package are out of scope for this spec.

## Problem

When a PR with zero findings is reviewed by the bot and the repository's `autoApprove` config is set to `false`, the bot posts a review with event `COMMENT`. GitHub stores this as a `COMMENTED` review state. Branch protection rules that require an approving review treat `COMMENTED` as non-approving, so the PR's `reviewDecision` stays at `REVIEW_REQUIRED` and the PR cannot be merged through the normal `gh pr merge --merge` path — operators must use `--admin` to bypass protection, which defeats the purpose of having the bot review at all. The verdict already encodes the right answer ("approve"); the mapping layer is silently downgrading it based on a config flag that was meant to gate auto-approval as a feature, not to invalidate the verdict semantics.

## Goal

After this work:

- A verdict of approve from the reviewer always maps to a GitHub review event of `APPROVE`, regardless of the repo's `autoApprove` config value, for any code path that flows through `githubposter`.
- A verdict of request-changes from the reviewer always maps to `REQUEST_CHANGES`.
- The `githubposter` package never produces a review event of `COMMENT` and never produces a stored review state of `COMMENTED` via fresh posts.
- The verifier's expected-state allow-list reflects the above: only `APPROVED` and `CHANGES_REQUESTED` are valid post-states for a fresh bot review at the head SHA.

## Non-goals

- Do NOT remove or rename the `autoApprove` config field, the `AutoApprove` struct field, or its YAML key. Other code paths and operator tooling still read it.
- Do NOT change behavior of the CLI-level `handleGitHubApprove` / `submitBitbucketReview` paths in `agent/pr-reviewer/cmd/cli/main.go`. They have their own gating semantics and are tracked separately.
- Do NOT touch the dismissal logic that filters out *prior* `COMMENTED` reviews in `poster.go` around line 175 (the `r.State != "COMMENTED"` skip). Historical `COMMENTED` reviews from before this fix may still exist on real PRs and must still be skipped during dismissal.
- Do NOT add a new config flag, opt-out, or override that re-enables the old `COMMENT` fallback. The mapping is invariant after this spec; if a future caller needs a comment-only path, that is a separate spec.
- Do NOT alter the `eventToState` helper's `COMMENT → COMMENTED` mapping. It is still referenced by tests that exercise historical state strings, and removing it is out of scope.
- Do NOT modify the body-truncation logic, the empty-summary soft-warning, or the maximum-body constant.

## Desired Behavior

1. When `mapVerdictAndSummary` is called with verdict approve and `autoApprove=false`, the returned event is `APPROVE` and no "auto-approve disabled for this repo" preamble is prepended to the body.
2. When `mapVerdictAndSummary` is called with verdict approve and `autoApprove=true`, the returned event is `APPROVE` (unchanged from current behavior).
3. When `mapVerdictAndSummary` is called with verdict request-changes (any `autoApprove` value), the returned event is `REQUEST_CHANGES` (unchanged from current behavior).
4. No call path through `mapVerdictAndSummary` returns the string `"COMMENT"` as the event.
5. The verifier no longer accepts `"COMMENTED"` as a valid expected state when verifying a freshly-posted bot review at the head SHA. Only `"APPROVED"` and `"CHANGES_REQUESTED"` are accepted in that allow-list.
6. The empty-summary substitution and soft-warning, the 65 536-byte body truncation, and the dismissal-skip-on-`COMMENTED` behavior all remain intact and continue to be exercised by their existing tests.

## Constraints

- Frozen: the `prpkg.Verdict` type, the `mapVerdictAndSummary` function name and signature, the `Result.PostedEvent` field, the `AutoApprove` config field and its YAML key, the `eventToState` function.
- Frozen: the dismissal filter `r.State != "COMMENTED"` in the prior-review skip logic. Tests at `poster_test.go` lines around 305, 485, 591, and 845 continue to assert behavior on historical `COMMENTED` reviews and must keep passing.
- Frozen: every other public function and behavior in `githubposter` and the `pkg.PostRequest` / `pkg.VerifyRequest` shapes.
- Tests must use Ginkgo `DescribeTable` / `Entry` (project guide: `~/Documents/workspaces/coding/docs/go-testing-guide.md`). No `t.Run`.
- Error wrapping uses `github.com/bborbe/errors` if any error path is touched. No bare `return err`, no `fmt.Errorf`.
- glog `Info` calls (if any are added) must be gated behind `V(n)`. No new always-on logging.
- TDD: the failing unit test for the bug is written and observed to fail before the production patch lands.

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility |
|---|---|---|---|---|
| Reviewer produces verdict approve on a repo with `autoApprove=false` (the bug scenario) | Verifier sees `APPROVED` state at head SHA; `gh pr view --json reviewDecision` shows `APPROVED` | `mapVerdictAndSummary` returns event `APPROVE`; GitHub stores state `APPROVED`; branch protection's approving-review requirement is satisfied | None needed — happy path | Reversible (re-review or dismiss) |
| Reviewer produces verdict request-changes | Verifier sees `CHANGES_REQUESTED` state at head SHA | `mapVerdictAndSummary` returns `REQUEST_CHANGES`; GitHub stores `CHANGES_REQUESTED` | None — happy path | Reversible (dismiss + re-review) |
| A prior `COMMENTED` review by the bot exists at an older SHA from before this fix | Dismissal loop scans prior reviews | The `COMMENTED` review is skipped during dismissal (GitHub rejects dismissal of comment-state reviews); `APPROVED` / `CHANGES_REQUESTED` priors at older SHAs are dismissed normally | None — historical reviews remain on the PR as comments, harmless | Irreversible (cannot dismiss `COMMENTED` reviews via API) |
| Verifier polls and finds an old `COMMENTED` bot review at the head SHA (e.g. a PR that was reviewed under the old buggy code, then re-triggered after the fix) | Verifier scans reviews, applies allow-list `[APPROVED, CHANGES_REQUESTED]` | The stale `COMMENTED` review does not match the allow-list; verifier returns `errPhantomPOST` and the retry loop continues until the fresh `APPROVED` / `CHANGES_REQUESTED` review appears | Re-trigger reviewer; the next post will produce a matching state | Reversible (new post matches) |
| Unknown verdict value passed to `mapVerdictAndSummary` (defensive) | Caller observes empty event string | Function returns empty event; caller's POST will fail at the GitHub API layer with HTTP 422; retry/escalate path handles it | Operator investigates the bad verdict source | Reversible |

## Security / Abuse Cases

Not applicable beyond existing controls. This change tightens the mapping (removes a state), does not widen any trust boundary, does not accept new input, and does not affect authentication, body content sanitization, or rate-limit handling. The 65 536-byte body cap and empty-summary substitution are unchanged.

## Acceptance Criteria

- [ ] A Ginkgo `Entry` exists in `poster_test.go` that calls the production code with `verdict=VerdictApprove`, `autoApprove=false`, and asserts the resulting event is `"APPROVE"` and the resulting GitHub review state is `"APPROVED"` — evidence: `go test ./agent/pr-reviewer/pkg/githubposter/...` exit code 0 and `grep -n 'autoApprove:false.*APPROVE' poster_test.go` returns a matching line.
- [ ] The existing `Entry` for `VerdictRequestChanges → "REQUEST_CHANGES"` still passes — evidence: `go test ./agent/pr-reviewer/pkg/githubposter/...` exit code 0.
- [ ] The existing `Entry` for `VerdictApprove + autoApprove=true → "APPROVE"` still passes — evidence: `go test ./agent/pr-reviewer/pkg/githubposter/...` exit code 0.
- [ ] No `Entry` in `poster_test.go` expects event `"COMMENT"` for any verdict — evidence: `grep -n '"COMMENT"' agent/pr-reviewer/pkg/githubposter/poster_test.go` returns no line that is inside the `verdict to event/state mapping` `DescribeTable` (other occurrences such as `EventToStateForTest("COMMENT")` may remain).
- [ ] `mapVerdictAndSummary` in `poster.go` contains no branch that assigns `"COMMENT"` to `event` — evidence: `grep -n 'event = "COMMENT"' agent/pr-reviewer/pkg/githubposter/poster.go` returns zero lines.
- [ ] `mapVerdictAndSummary` in `poster.go` contains no string `"auto-approve disabled for this repo"` — evidence: `grep -n 'auto-approve disabled' agent/pr-reviewer/pkg/githubposter/poster.go` returns zero lines.
- [ ] The verifier's expected-state allow-list for fresh bot reviews at the head SHA does not include `"COMMENTED"` — evidence: `grep -n 'COMMENTED' agent/pr-reviewer/pkg/githubposter/verifier.go` returns zero lines, OR every remaining occurrence is inside a comment that explains exclusion (reviewer judgement call at implementation time on how to express this — agent decides at impl time).
- [ ] Verifier tests still pass and assert that `"APPROVED"` and `"CHANGES_REQUESTED"` are accepted while `"COMMENTED"` is rejected at the head SHA — evidence: `go test ./agent/pr-reviewer/pkg/githubposter/...` exit code 0.
- [ ] The dismissal-skip-on-`COMMENTED` tests in `poster_test.go` (around lines 305, 485, 591, 845) still pass — evidence: `go test ./agent/pr-reviewer/pkg/githubposter/...` exit code 0.
- [ ] `make precommit` run from `agent/pr-reviewer/` exits 0 — evidence: exit code.
- [ ] TDD trail visible: the new failing test is committed in a separate commit before the production patch — evidence: `git log --oneline feature/pr-verdict-mapping ^origin/master -- agent/pr-reviewer/pkg/githubposter/` shows the `poster_test.go`-touching commit with a subject containing `test` preceding the `poster.go`-touching commit.

Scenario coverage: no new scenario. Behavior is reachable from a single-package unit test against `mapVerdictAndSummary` and from the existing `DescribeTable` for the post path. No E2E layer is required for this fix.

## Verification

```
cd agent/pr-reviewer
make precommit
go test ./pkg/githubposter/...
grep -n 'event = "COMMENT"' pkg/githubposter/poster.go
grep -n 'auto-approve disabled' pkg/githubposter/poster.go
grep -n 'COMMENTED' pkg/githubposter/verifier.go
```

Expected:
- `make precommit`: exit 0.
- `go test`: exit 0.
- `grep -n 'event = "COMMENT"'`: zero lines.
- `grep -n 'auto-approve disabled'`: zero lines.
- `grep -n 'COMMENTED' verifier.go`: zero lines (or comment-only mentions explaining exclusion).

Manual post-merge sanity (dev environment, operator-driven, not a gating AC): a docs-only PR built on master after merge gets an `APPROVED` review from the bot, `gh pr view --json reviewDecision` returns `APPROVED`, and `gh pr merge --merge` succeeds without `--admin`.

## Do-Nothing Option

If we do not fix this: every docs-only PR (zero findings) on a repo with `autoApprove=false` stays at `reviewDecision: REVIEW_REQUIRED` and requires either a manual human review or an admin merge. This is the current state and is the immediate trigger for this spec. It is not acceptable because (a) it forces admin-merge habits that bypass branch protection generally, and (b) it makes the bot's "approve" verdict effectively meaningless for the most common merge path. There is no reasonable wait-and-see alternative — the only way to surface the bug fewer times is to stop merging docs-only PRs, which is not a real option.
