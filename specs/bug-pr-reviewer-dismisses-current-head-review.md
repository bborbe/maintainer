---
status: draft
tags:
    - dark-factory
    - spec
---

## Summary

- A re-spawned PR-reviewer task on the same head SHA wiped its own freshly-posted GitHub review because the dismissal filter compares `r.CommitID == headSHA` — it dismisses the very review the pod just left at the current head, not prior reviews at superseded SHAs.
- The function `dismissPriorReviews` is meant to clear stale reviews at *earlier* commits as new commits accrue on a PR. The filter condition is inverted: `==` where it must be `!=`.
- The triggering incident left `bborbe/maintainer` PR #5 head SHA `d04d349` with zero GitHub reviews even though the reviewer-agent ran successfully end-to-end and the vault task carries a complete `## Review` section.
- The fix inverts the SHA filter, plants an invariant comment naming the rule, adds Ginkgo test rows covering the multi-SHA history matrix, and documents the dismissal contract in `pr-post-back.md`.
- After the fix, re-running a reviewer task on a PR that already has a bot review at the current head leaves that review untouched; only reviews at superseded SHAs are dismissed.

## Problem

The PR-reviewer agent for `bborbe/maintainer` PR #5 `d04d349` was spawned twice on the same head SHA on 2026-05-16 (pod 1 completed, pod 2 spawned ~5 min later as a re-run triggered by an unrelated controller bug). Pod 2's `dismissPriorReviews` removed pod 1's freshly-posted GitHub review `4303450851`, then the downstream idempotency short-circuit (`## Review` already present in the vault) skipped the re-POST. Net result: PR #5 ended with zero reviews on GitHub despite a successful agent run. Any operator-triggered retry or controller-triggered re-spawn reproduces the same wipe, so the bug is not specific to the originating trigger — it is a latent defect in the dismissal filter that fires on every multi-pod-per-SHA scenario. The PR's review state on GitHub does not reflect the agent's actual verdict, which silently undermines the trust contract of the entire reviewer pipeline.

## Goal

After this fix, the dismissal step never removes a bot review whose `commit_id` equals the PR's current head SHA. Only bot reviews at SHAs strictly older than (or different from) the current head are dismissed. The source code contains an invariant comment naming the rule so a future reader cannot re-invert it without first arguing against the comment. The dismissal contract is documented in `agent/pr-reviewer/docs/pr-post-back.md`. The triggering scenario — two pods successfully posting at the same head SHA — leaves exactly one bot review at that SHA on GitHub.

## Non-goals

(Bug spec — scope is bounded by the reproduction. Listed for clarity.)

- Not fixing Bug 1 (verdict schema mismatch at `verdict.go:50`) — covered by spec 030.
- Not fixing Bug 3 (controller spawn-on-terminal-phase that triggered the second pod) — separate spec.
- Not changing the dismissal HTTP behavior (`PUT .../dismissals` endpoint, retry policy, dismissal message body).
- Not changing how the verifier at `verifier.go:76` recognises the just-posted review — it continues to match on `CommitID == headSHA`.
- Not introducing multi-bot-identity dismissal logic — the existing `botLogin == p.botLogin` filter stands.
- Not relaxing the `COMMENTED`-state skip — GitHub's API rejects dismissal of comment reviews with HTTP 422; the current skip is correct.

## Reproduction

**Triggering incident (verbatim evidence on file):**

- PR: `bborbe/maintainer` #5, head SHA `d04d349`
- Pod 1: `pr-reviewer-agent-22fda7e7-…` posted review_id `4303450851` (APPROVED — incorrect verdict due to Bug 1, but irrelevant to this filter bug)
- Pod 2: `pr-reviewer-agent-22fda7e7-20260516093019` (spawned ~5 min after pod 1) ran `dismissPriorReviews` which selected review `4303450851` (because its `commit_id == d04d349` matched the head SHA), called `PUT .../reviews/4303450851/dismissals`, then short-circuited on the vault-`## Review`-present idempotency check and did not re-POST.
- Final GitHub state: `gh pr view 5 --repo bborbe/maintainer --json reviews | jq '.reviews | map(select(.author.login=="pr-review-of-ben"))'` → empty array.
- Vault task page: `~/Documents/Obsidian/OpenClaw/tasks/PR Review github - bborbe-maintainer - 5 - d04d349a - confirm-new-env-vars-are-documented-in-help.md` shows a complete `## Review` section.

**Minimal in-process reproduction:**

1. Construct a fake GitHub `/pulls/N/reviews` response containing two bot reviews: one at `SHA-A` (older), one at `SHA-B` (current head).
2. Call the bot-review filter with `headSHA = "SHA-B"`.
3. Observe today: the filter returns the `SHA-B` review for dismissal (and not the `SHA-A` review).
4. Expected: the filter returns only the `SHA-A` review for dismissal; the `SHA-B` review is preserved.

**Live reproduction (post-deploy gate):**

1. On a dev test PR, create two synthetic bot reviews from `pr-review-of-ben`: one at an older SHA, one at the current head SHA (via `gh api`).
2. Trigger the reviewer-agent task for that PR.
3. Observe today: both reviews dismissed (or current-head one dismissed and older one untouched if the older is `COMMENTED`).
4. After fix: older review dismissed via `PUT .../dismissals`; current-head review preserved; verifier sees the current-head review and exits success.

## Expected vs Actual

**Expected** (per the function name `dismissPriorReviews` and the spec 027 dismissal feature intent): the dismissal step removes bot reviews left at earlier commits as the PR accrues new commits, so the PR's review state at any moment reflects only the latest head. A bot review at the current head is the one the pipeline is preserving — it is the verifier's evidence that the just-completed POST succeeded.

**Actual** (observed 2026-05-16 on PR #5 `d04d349`): the filter dismisses reviews whose `commit_id` equals the current head SHA — the exact opposite of the intended behavior. Reviews at older SHAs are never selected for dismissal. When two pods run on the same head SHA, pod 2 wipes pod 1's review.

## Why this is a bug

Three independent invariants are broken:

1. **Function name vs implementation disagreement.** `dismissPriorReviews` promises dismissal of *prior* reviews; the implementation dismisses *current* reviews. Reading the function name leads the reader to the wrong mental model.
2. **Re-spawn safety broken.** The post-back design (`docs/pr-post-back.md` Vault-First Invariant) assumes pod-level idempotency: if pod 2 re-runs after pod 1 succeeded, the vault is already correct and GitHub is already correct, so nothing should change. The inverted filter introduces a destructive side effect on every re-spawn, breaking the idempotency contract.
3. **Verifier contract violated.** `verifier.go:76` confirms POST success by looking for `r.CommitID == headSHA && r.State ∈ ExpectedStates`. The dismissal filter actively removes the artifact the verifier depends on, creating a state where the verifier — if it ran after dismissal — would report POST failure even though a POST happened.

The combination produces a silent wipe-on-retry failure mode for a system whose entire job is to leave a durable review trace on the PR.

## Desired Behavior

1. The bot-review filter inside `listBotReviews` returns only reviews whose `commit_id` is **not** equal to the current head SHA (in addition to the existing `botLogin` and non-`COMMENTED` predicates).
2. A bot review at the current head SHA is never returned for dismissal, regardless of state (`APPROVED`, `CHANGES_REQUESTED`, `DISMISSED`, etc.).
3. A bot review at any SHA different from the current head is returned for dismissal, subject to the existing `COMMENTED` skip and the existing `DISMISSED` skip in the caller.
4. Two simultaneous bot reviews at the current head (rare race) are both preserved; the verifier picks whichever matches `ExpectedStates`.
5. Reviews authored by users other than `p.botLogin` are never returned for dismissal (unchanged).
6. `COMMENTED`-state reviews at any SHA are never returned for dismissal (unchanged — GitHub API rejects with 422).
7. An empty review list returns an empty dismissal list (unchanged).
8. A code-level invariant comment immediately above the filter names the rule: "reviews at the current head SHA are never dismissed here". The comment references `docs/pr-post-back.md §Dismissal Contract` and this spec.
9. `agent/pr-reviewer/docs/pr-post-back.md` contains a "Dismissal Contract" subsection that states the SHA invariant in prose.
10. Re-spawning a reviewer-agent task on a PR that already has a bot review at the current head leaves that review on GitHub untouched.

## Constraints

- The exported `prpkg.PrPoster` interface MUST NOT change. Callers (`controller`, `executor`) depend on the existing signature.
- `verifier.go` MUST NOT change — its `CommitID == headSHA` predicate is correct and load-bearing. This spec only changes `poster.go`'s filter inside `listBotReviews`.
- The `COMMENTED`-state skip MUST be preserved verbatim (with the same justification comment about HTTP 422).
- The `DISMISSED`-state skip in the caller (`dismissPriorReviews` loop at `poster.go:150-152`) MUST be preserved.
- The dismissal HTTP call (`PUT /repos/.../pulls/N/reviews/M/dismissals`), its payload (`{"message":"superseded by new automated review"}`), and its retry policy MUST NOT change.
- Existing passing tests in `poster_test.go` and `verifier_test.go` that do not depend on the inverted filter MUST continue to pass. Any test row that asserted the old "dismiss current-head review" behavior must be re-pointed to the corrected behavior or deleted.
- Domain rules referenced: `specs/completed/027-post-verdict-to-github-pr.md` (the dismissal feature shipped here; this is a regression in its filter logic).
- Verification ladder per `docs/verifying-specs.md`: pure code correctness, so Rung-1 (local `make precommit` + new Ginkgo rows) is the primary gate; Rung-2 (dev deploy + simulated multi-SHA history) is the live-evidence gate; Rung-3 (prod) only after Rung-2 passes.
- This spec assumes Bug 1 (spec 030) lands first: live verification requires correct verdicts to be posted in the first place, otherwise the dismissed review under test is wrong-direction noise.

## Failure Modes

| Trigger | Expected behavior | Detection | Recovery |
|---|---|---|---|
| Two bot reviews exist, one at older `SHA-A`, one at current head `SHA-B` | Filter returns only the `SHA-A` review; `SHA-B` review is preserved | Ginkgo unit test row; runtime log line `dismiss-list size=1 sha=SHA-A` | None needed — handled inline |
| One bot review at current head `SHA-X`, no others | Filter returns empty list; nothing dismissed | Ginkgo unit test row; runtime log shows empty dismissal | n/a |
| Two bot reviews both at current head `SHA-Y` (race: two pods both POST-succeeded) | Filter returns empty list; both preserved; verifier picks whichever matches `ExpectedStates` | Ginkgo unit test row; verifier success log | If the two reviews disagree on state, operator inspects manually; out of scope for this fix |
| Bot `COMMENTED` review at older SHA + bot `CHANGES_REQUESTED` at older SHA | Filter returns only the `CHANGES_REQUESTED` review (COMMENTED skipped); both at older SHA so both eligible by the new SHA predicate, but COMMENTED is filtered out independently | Ginkgo unit test row | n/a |
| Non-bot review at older SHA | Filter does not return it; non-bot reviews are never dismissed | Ginkgo unit test row | n/a |
| Empty review list from GitHub | Filter returns empty; dismissal loop is a no-op | Ginkgo unit test row | n/a |
| GitHub `GET /pulls/N/reviews` returns 5xx | Existing retry policy in `retryCall` applies; on exhaustion `dismissPriorReviews` returns a failed `PostResult` with `Class=Transient` | Pod log shows retries; failed `PostResult` surfaces in vault diagnostic block | Controller re-spawns task on next reconcile; reversible |
| Two pods re-spawn on the same head SHA (the triggering scenario) | Pod 2's filter returns empty (pod 1's review is at the current head); pod 2 short-circuits on vault idempotency; PR ends with pod 1's review intact | `gh pr view N --json reviews` shows exactly one bot review at head; pod 2 logs show empty dismissal list | None needed — idempotency restored |
| Force-pushed new head SHA between pod 1 and pod 2 | Pod 1's review is now at a superseded SHA; pod 2's filter returns it for dismissal (correct behavior — that is the intended dismissal path) | Pod 2 logs show dismissal of pod 1's review by ID; new POST follows | n/a; this is the intended use-case of `dismissPriorReviews` |

## Security / Abuse Cases

- The filter takes `headSHA` and `botLogin` from internal trusted sources (controller-provided PR state, resolved `/user` identity). No external input crosses a trust boundary inside `listBotReviews`.
- The `commit_id` field in the GitHub response is attacker-controllable only if the attacker can post a review with a forged `commit_id` — GitHub does not permit this; `commit_id` is set server-side by the API.
- No injection vector: `commit_id` and `headSHA` are compared as opaque strings via `!=`; no shell, SQL, or template substitution.
- No retry-forever path: existing `retryCall` policy bounds attempts; this fix does not change retry semantics.

## Acceptance Criteria

- [ ] The filter at `agent/pr-reviewer/pkg/githubposter/poster.go` inside `listBotReviews` uses `r.CommitID != headSHA` (not `==`) — evidence: `grep -nE 'r\.CommitID\s*(==|!=)\s*headSHA' agent/pr-reviewer/pkg/githubposter/poster.go` returns exactly one match and that match uses `!=`.
- [ ] An invariant comment is present immediately above the filter naming the SHA rule — evidence: reading `poster.go`, the lines immediately above the `if r.User.Login == p.botLogin && r.CommitID != headSHA && ...` line contain the phrase `current head SHA` and the phrase `never dismissed` (or equivalent wording asserting non-dismissal of current-head reviews) and reference `docs/pr-post-back.md` and this spec.
- [ ] A Ginkgo `DescribeTable` (or equivalent table-style `Context`/`Entry` set) in `agent/pr-reviewer/pkg/githubposter/poster_test.go` (external test package `package githubposter_test`) covers the six rows below — evidence: `grep -nE 'DescribeTable|Entry\(' agent/pr-reviewer/pkg/githubposter/poster_test.go` returns ≥6 lines, and the table contains entries asserting:
  - [ ] Row A: two bot reviews `{SHA-A, SHA-B}`, head=`SHA-B` → dismissal list contains only the `SHA-A` review by ID
  - [ ] Row B: one bot review at head `SHA-X`, head=`SHA-X` → dismissal list is empty
  - [ ] Row C: two bot reviews both at head `SHA-Y`, head=`SHA-Y` → dismissal list is empty (neither dismissed)
  - [ ] Row D: one bot `COMMENTED` at older SHA + one bot `CHANGES_REQUESTED` at older SHA → dismissal list contains only the `CHANGES_REQUESTED` review (COMMENTED filtered out independently)
  - [ ] Row E: one non-bot review at older SHA → dismissal list is empty
  - [ ] Row F: empty review list from the fake → dismissal list is empty
- [ ] Reverting the filter to `==` causes Row A and Row B to fail — evidence: on a scratch branch with the `!=` reverted back to `==`, `go test ./agent/pr-reviewer/pkg/githubposter/...` exits non-zero and the failure output names Row A or Row B (or their entry text).
- [ ] `make precommit` in `agent/pr-reviewer/` exits 0 — evidence: exit code.
- [ ] `agent/pr-reviewer/docs/pr-post-back.md` contains a "Dismissal Contract" subsection naming the SHA invariant — evidence: `grep -nE '^##+\s+Dismissal Contract' agent/pr-reviewer/docs/pr-post-back.md` returns ≥1 line, and the prose immediately following contains both the strings `current head` and `superseded` (or equivalent wording stating: dismiss only reviews at superseded SHAs; never dismiss the review at the current head).
- [ ] No existing test asserts the old "current-head review is dismissed" behavior — evidence: `grep -rn 'CommitID:\s*headSHA\|commit_id.*head' agent/pr-reviewer/pkg/githubposter/poster_test.go` does not contain any assertion that a current-head review appears in the dismissal output.
- [ ] Live replay (Rung-2): on a dev test PR seeded with two synthetic bot reviews from `pr-review-of-ben` (one at an older SHA, one at the current head SHA), triggering the reviewer-agent task results in exactly one bot review remaining at the current head — evidence: before the agent runs, `gh api /repos/<owner>/<repo>/pulls/<N>/reviews | jq '[.[] | select(.user.login=="pr-review-of-ben")] | length'` returns 2; after the pod completes, the same query returns 1 and the remaining review's `commit_id` equals the PR's current head SHA; the dismissed older review appears in pod logs as a `PUT .../dismissals` step with HTTP 200.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Expected: exit 0. The new Ginkgo table reports ≥6 entries passing.

Revert-test (confirms the table actually exercises the inversion):

```
cd agent/pr-reviewer
# temporarily change `!=` back to `==` in pkg/githubposter/poster.go listBotReviews filter
go test ./pkg/githubposter/...
# Expected: non-zero exit, output names Row A and/or Row B
# revert the change before continuing
```

Rung-2 live replay (after dev deploy):

```
# Pick a dev test PR (e.g., a PR on bborbe/maintainer-sandbox); identify head SHA = $HEAD.
# Pick an older SHA from the PR's commit history = $OLD.
# Seed two synthetic bot reviews via gh api (as the bot identity pr-review-of-ben):
gh api -X POST /repos/$OWNER/$REPO/pulls/$N/reviews \
  -f commit_id=$OLD -f event=REQUEST_CHANGES -f body="synthetic older review for spec verification"
gh api -X POST /repos/$OWNER/$REPO/pulls/$N/reviews \
  -f commit_id=$HEAD -f event=REQUEST_CHANGES -f body="synthetic current-head review for spec verification"

# Confirm seed:
gh api /repos/$OWNER/$REPO/pulls/$N/reviews \
  | jq '[.[] | select(.user.login=="pr-review-of-ben")] | length'
# Expected: 2

# Reset the vault task for this PR (frontmatter: phase=in_progress, status=in_progress,
# drop current_job + job_started_at, bump trigger_count). obsidian-git auto-commits;
# task-controller picks it up within ~30s.

# Wait for pod completion:
kubectlquant -n dev get pods -l app=pr-reviewer-agent --sort-by=.metadata.creationTimestamp

# Confirm post-state:
gh api /repos/$OWNER/$REPO/pulls/$N/reviews \
  | jq '[.[] | select(.user.login=="pr-review-of-ben" and .state != "DISMISSED")] | length'
# Expected: 1 (the current-head review preserved)

gh api /repos/$OWNER/$REPO/pulls/$N/reviews \
  | jq '[.[] | select(.user.login=="pr-review-of-ben" and .state != "DISMISSED")] | .[0].commit_id'
# Expected: $HEAD

# Also confirm dismissal log line in pod logs:
kubectlquant -n dev logs <pod-name> | grep -E 'PUT .*dismissals'
# Expected: ≥1 line referencing the older review's ID
```

## Do-Nothing Option

Not viable. Every multi-pod-per-SHA scenario (operator-triggered retry, controller-triggered re-spawn, network-flake-triggered replay) silently wipes the just-posted review. The triggering incident already produced a PR on `bborbe/maintainer` with zero reviews despite a successful agent run. Until fixed, the reviewer pipeline cannot be trusted to leave durable evidence on a PR, which defeats specs 025 and 027. The configured workaround would be to disable re-spawns entirely, which negates Bug 3's recovery path and other legitimate retry scenarios.
