---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-05-31T22:19:26Z"
generating: "2026-05-31T22:19:57Z"
prompted: "2026-05-31T22:30:36Z"
branch: dark-factory/ai-review-actionable
---

## Summary

- When the agent's verdict-of-verdict (`ai_review`) flags its own posted GitHub review as hallucinated, the agent dismisses that review on GitHub and posts a follow-up COMMENT review citing the hallucinations — so the merge gate clears without operator admin-merge.
- When the watcher parks a re-review at `human_review` because the prior task on the same PR had `ai_review verdict: fail`, the newly-spawned task file carries a `# Parked Because` section listing prior task ID, prior SHA, prior verdict, and the prior hallucinations — readable without opening any other file.
- A clean review (no hallucinations) keeps current behavior unchanged: no dismissal call, no `# Parked Because` section.
- The `ai_review` hallucination check itself is not modified — this spec only acts on the verdict that check already produces.

## Problem

When `ai_review` returns `verdict: fail` with `hallucinations` populated, two operator-unfriendly things happen today:

1. The bot's `CHANGES_REQUESTED` review stays posted on the PR. GitHub branch protection stays `BLOCKED`. The operator must read pod logs to confirm the hallucination, gather evidence locally, and admin-merge — even though the agent already knows the review is wrong.
2. When the watcher re-spawns a task for the next push on the same PR and parks it at `phase: human_review` (current park reasons in the codebase: untrusted author; new reason added by this spec: prior ai_review fail), the task file shows `assignee: ""` with the standard untrusted-author body — no record of "the previous task's ai_review flagged hallucinations on SHA X". The operator has to hunt the prior task and read logs to know why this one is silent.

Surfaced on `bborbe/dotfiles` PR #1 (2026-05-31), admin-merged after the bot hallucinated and the second-pass task parked without explanation.

## Goal

After this work:

- On `ai_review verdict: fail` with at least one hallucination entry, the bot's review at the current head SHA is dismissed via GitHub REST and replaced with a COMMENT review citing each hallucination. `gh pr view <n> --json reviewDecision` returns `REVIEW_REQUIRED` (not `CHANGES_REQUESTED`). The next push can clear the gate by posting a fresh review — no admin merge required.
- When the watcher spawns a `human_review`-phased task because the most recent prior task on the same PR ended with `ai_review verdict: fail`, the spawned task file contains a `# Parked Because` section with prior task ID, prior head SHA, prior verdict string, and the prior hallucinations list (one entry per `{file, line, issue}`).
- All other paths are byte-for-byte unchanged: clean review still posts APPROVE / CHANGES_REQUESTED / COMMENT and does NOT dismiss; untrusted-author parks still produce the existing untrusted-author body with no fake `# Parked Because`.

## Non-goals

- Do NOT change the `ai_review` prompt or its JSON schema — `hallucinations: [{file, line, issue}]` is already required-may-be-empty in `agent/pr-reviewer/pkg/prompts/review_output-format.md`.
- Do NOT auto-merge anything. Dismissal returns the PR to `REVIEW_REQUIRED`; the next push (human or bot) is what clears the gate.
- Do NOT add a config flag to disable the dismissal behavior. If a future consumer demands variation, that's a separate spec. (An opt-out on the Goal is itself a regression.)
- Do NOT change `dismissPriorReviews`' existing invariant for the non-hallucination path (reviews at the current head SHA are never dismissed by it). The new dismissal targets the current head SHA specifically and is a separate call site.
- Do NOT modify the existing untrusted-author park flow — `# Parked Because` is only written for the prior-ai-review-fail reason. Untrusted-author parks continue to use the existing `## Untrusted author` body.
- Do NOT introduce a new task phase. `ai_review` stays the existing phase; new logic lives inside `reviewStep.Run` and the watcher's command-builder.

## Desired Behavior

1. **Dismiss-and-comment trigger gate.** `ai_review` step performs the dismiss-and-comment action if and only if all of the following hold: `verdict == "fail"`, AND `len(hallucinations) > 0`, AND a bot-authored review exists at the current head SHA in state `APPROVED` or `CHANGES_REQUESTED` (queried via GitHub REST). If any condition fails, no dismiss call is made; routing continues to `human_review` exactly as today.
2. **Dismiss action.** The agent calls `PUT /repos/{owner}/{repo}/pulls/{n}/reviews/{review_id}/dismissals` for the bot's review at the current head SHA, with message `"hallucinated review — see follow-up COMMENT for evidence"`. On 2xx, proceeds to step 3. On 4xx/5xx, the failure is logged to `## Diagnostics` and the step routes to `human_review` with the original verdict reason (no partial state).
3. **Follow-up COMMENT review.** After successful dismissal, the agent posts a new review via `POST /repos/{owner}/{repo}/pulls/{n}/reviews` with `event: "COMMENT"`, `commit_id: <headSHA>`, and body listing the hallucinations — one line per entry in the form `- {file}:{line} — {issue}`, preceded by a one-line preamble identifying this as a hallucination dismissal.
4. **Next-phase routing after dismiss-and-comment.** Whether dismiss-and-comment succeeded or failed, the next phase is `human_review`. A human still owns the final call; the dismissal just unblocks the merge gate.
5. **Watcher carries prior-failure context.** When the watcher decides to spawn a task whose phase is `human_review` because the most recent prior task on the same `(owner, repo, pr_number)` had `ai_review verdict: fail`, the spawned task's body contains a `## Parked Because` section with: prior task ID (string), prior head SHA (string), prior verdict (literal `fail`), and a bulleted list of hallucinations from the prior ai_review payload (one bullet per `{file, line, issue}` entry, in the order they appeared in the payload).
6. **No `# Parked Because` for other reasons.** Untrusted-author parks (`buildUntrustedBody`) do not gain a `# Parked Because` section. Trusted-author normal spawns (`buildTaskBody`) do not gain one either. The section appears only for the prior-ai-review-fail park reason.
7. **Clean review unchanged.** When `ai_review verdict: pass`, OR `verdict: fail` with empty `hallucinations` (e.g. `verdict_consistency: inconsistent`), no dismissal API call is made and no follow-up COMMENT is posted. Routing matches today: `pass → done`, `fail → human_review`.

## Constraints

- **Frozen invariant in `dismissPriorReviews`:** reviews at the current head SHA must remain non-dismissable by the pre-existing dismiss-prior path. This spec adds a separate code path targeting the current head SHA explicitly; it must not loosen the bot-author / head-SHA filter in `listBotReviews` (in `agent/pr-reviewer/pkg/githubposter/poster.go`). Existing `poster_test.go` cases asserting "review at current head SHA is preserved by `dismissPriorReviews`" must continue to pass.
- **Frozen JSON schema** in `agent/pr-reviewer/pkg/prompts/review_output-format.md`. The Go-side parsed shape (`verdictPayload`) gains fields but the wire format is unchanged.
- **Frozen `PrPoster` interface** for existing methods (`Post`, `PostLGTM`). New methods are additive. Mocks in `mocks/pr-poster.go` are regenerated via `go generate`.
- **Frozen factory invariant:** new constructors are `New*` in `pkg`, wired by `Create*` in `pkg/factory/factory.go` per `~/Documents/workspaces/coding/docs/go-factory-pattern.md`. Factory contains no logic and no `error` returns.
- **Frozen state-machine invariant** per `~/Documents/workspaces/coding/docs/go-state-machine-pattern.md`: no new phase; new logic lives inside the existing `ai_review` phase's `reviewStep.Run`.
- **Frozen error-wrapping invariant** per `~/Documents/workspaces/coding/docs/go-error-wrapping-guide.md`: all wraps use `github.com/bborbe/errors` `errors.Wrapf(ctx, err, "...")`. No `fmt.Errorf` introduced.
- **Frozen HTTP invariant** per `~/Documents/workspaces/coding/docs/go-http-handler-refactoring-guide.md`: dismiss and follow-up-comment calls go through the existing `doRequest` + `retryCall` plumbing in `pkg/githubposter/poster.go`. No raw `http.DefaultClient`.
- **GitHub App scope:** the existing `ben-s-pull-request-reviewer` App already has `pull_request: write` (required by current post and dismiss-prior flows). No new scope or app config.
- **`# Parked Because` parser-compat:** the spawned task is consumed by `agentlib.Markdown` section parsing. The new section heading is `## Parked Because` (double-`#`), matching `buildUntrustedBody`'s sibling pattern (`## Untrusted author`). It is the only top-level section in the prior-fail park body — no parent `# PR Review` wrapper.

## Assumptions

- The watcher already has access to the prior task's metadata at spawn time (most recent prior task for the same `(owner, repo, pr_number)`). If this is not true today, the implementation prompt must add a lookup — current spec assumes the controller/watcher boundary either (a) already retains the prior task's ai_review payload on the Kafka trigger message, or (b) can fetch it from the agent's task store. Resolution belongs in the implementation prompt; the spec only contracts the observable output.
- `hallucinations` entries from Claude are trustworthy enough to render verbatim in a public GitHub comment. (Sanitization is out of scope — the agent already trusts the same payload for routing decisions.)

## Failure Modes

| Trigger | Detection | Expected behavior | Recovery | Reversibility |
|---------|-----------|-------------------|----------|---------------|
| GitHub `PUT .../dismissals` returns 404 (review_id stale / dismissed by another actor) | HTTP 404 in `## Diagnostics` | Skip follow-up COMMENT; route to `human_review` with verdict reason | Operator manually reviews; next push triggers fresh review cycle | Irreversible (dismissal not attempted again this run) |
| GitHub `PUT .../dismissals` returns 422 ("Can not dismiss a commented pull request review") | HTTP 422 in `## Diagnostics` | No-op (the review we'd dismiss is COMMENT-state, doesn't block merge); skip follow-up; route to `human_review` | None needed — merge gate was never blocked | Irreversible per run |
| GitHub `PUT .../dismissals` returns 5xx | HTTP 5xx in `## Diagnostics` | Retry via existing `retryCall` plumbing; if final attempt fails, log + route to `human_review` (CHANGES_REQUESTED stays posted) | Operator falls back to today's admin-merge path | Reversible on next trigger |
| GitHub `POST .../reviews` (the follow-up COMMENT) fails after a successful dismissal | HTTP non-2xx in `## Diagnostics` | Dismissal stands (merge gate cleared); log the COMMENT failure; route to `human_review` | Operator sees dismissed-without-explanation review; can read `## Diagnostics` for the hallucination list | Partial — dismissal succeeded, comment did not |
| `ai_review` returns `verdict: fail` with `hallucinations: []` (e.g. `verdict_consistency: inconsistent`) | Verdict parse | No dismiss, no follow-up; route to `human_review` exactly as today | None — this is the existing behavior | N/A |
| Pod crashes between dismissal and follow-up COMMENT | Re-spawned task sees `## Verdict` already present, short-circuits to `done` (existing idempotency in `reviewStep.Run` line 77-83) | Dismissal stands; follow-up COMMENT never posted | Operator reads `## Diagnostics` from the original task | Partial — same shape as "COMMENT failure" above |
| Watcher spawns `human_review` task but prior task's ai_review payload is unavailable (lookup failure) | Watcher log line | Spawn task with no `# Parked Because` section (silent park, current behavior) — do NOT block spawn on missing payload | Operator falls back to today's "find the prior task" workflow | Reversible — next trigger may have payload |
| Two concurrent ai_review runs on the same PR (race) | Both attempt dismissal of the same review_id | First wins (200); second gets 404 → treated as "stale" above, routes to `human_review`. No state corruption. | None | N/A |
| GitHub rate-limit (403 secondary) on dismiss or COMMENT POST | HTTP 403 + `X-RateLimit-Remaining: 0` | Retried by existing `retryCall`; on exhaustion, route to `human_review` with diagnostic logged | Operator falls back to admin-merge; next poll cycle gets fresh quota | Reversible on next trigger |

## Security / Abuse Cases

- **Attacker controls `hallucinations` field?** No — the field is generated by Claude on a trusted instructions payload. The agent already trusts the same field for routing.
- **`file` / `line` / `issue` rendered in a public GitHub COMMENT.** Pathological content (markdown injection, HTML, very long strings) is rendered as-is. The list is bounded by the size of the agent's review payload (typically <20 hallucinations). No per-entry length cap is added; if Claude emits 10MB the COMMENT POST will fail with 422 and fall through the failure-mode table.
- **`PUT .../dismissals` on a review that isn't ours.** The implementation must filter `r.User.Login == p.botLogin` before dismissing (same invariant as `listBotReviews`). Skipping this filter would let one bot dismiss another's review on the same PR.
- **No new trust boundary crossed.** The dismiss call uses the same GitHub App token as the existing post path.

## Acceptance Criteria

- [ ] On `ai_review verdict: fail` with `len(hallucinations) > 0` and a bot review in state `CHANGES_REQUESTED` at the current head SHA, the agent emits HTTP `PUT /repos/{owner}/{repo}/pulls/{n}/reviews/{review_id}/dismissals` — evidence: `## Diagnostics` block contains a YAML entry with `step: "PUT .../dismissals"` and `http_status: 200` (or 2xx). Synthetic-PR test asserts via `gh pr view <n> --json reviews` that the prior review's state transitioned to `DISMISSED`.
- [ ] After successful dismissal, `gh pr view <n> --json reviewDecision` returns `REVIEW_REQUIRED` (not `CHANGES_REQUESTED`) — evidence: `gh` CLI JSON output captured in the synthetic-PR scenario.
- [ ] After successful dismissal, a follow-up COMMENT review exists on the PR at the current head SHA, authored by the bot, with body matching regex `(?s)hallucinated review.*\n- .+:\d+ — .+` — evidence: `gh pr view <n> --json reviews` JSON shows one review with `state: "COMMENTED"`, `commit_id == headSHA`, and body matching the regex.
- [ ] On `ai_review verdict: fail` with `len(hallucinations) == 0`, the agent emits zero `PUT .../dismissals` requests — evidence: counterfeiter mock recorder shows zero invocations of the new dismissal method; routing returns `NextPhase: "human_review"` (existing test extended).
- [ ] On `ai_review verdict: pass`, the agent emits zero `PUT .../dismissals` requests — evidence: existing `steps_review_test.go` pass-path test extended with mock-invocation assertion.
- [ ] Existing test `poster_test.go` "dismissPriorReviews preserves the review at the current head SHA" still passes — evidence: `make test` exit code 0 in `agent/pr-reviewer/pkg/githubposter/`.
- [ ] When watcher spawns a `human_review`-phased task because the most recent prior task on the same PR had `ai_review verdict: fail`, the spawned task's body contains a section starting with `## Parked Because` — evidence: integration test asserts `strings.Contains(spawnedCmd.Body, "## Parked Because")` AND `strings.Contains(spawnedCmd.Body, priorTaskID)` AND each `{file, line, issue}` triple from the prior payload appears as a bullet AND the bullets' relative order matches the prior payload's array order (`strings.Index` of bullet N is less than `strings.Index` of bullet N+1).
- [ ] When watcher spawns a `human_review`-phased task for the untrusted-author reason, the spawned task's body does NOT contain `Parked Because` — evidence: existing `buildUntrustedBody` test extended with `Expect(body).NotTo(ContainSubstring("Parked Because"))`.
- [ ] When watcher spawns a normal trusted-author task (planning phase), the body does NOT contain `Parked Because` — evidence: existing `buildTaskBody` test extended likewise.
- [ ] `[[GitHub PR Reviewer Agent]]` Obsidian page (`/Users/bborbe/Documents/Obsidian/Personal/50 Knowledge Base/GitHub PR Reviewer Agent.md`) contains a new subsection documenting (a) the dismiss-and-comment behavior with its trigger gate, and (b) the `## Parked Because` section's meaning and fields — evidence: `grep -nE "Parked Because|hallucinated review" "/Users/bborbe/Documents/Obsidian/Personal/50 Knowledge Base/GitHub PR Reviewer Agent.md"` returns ≥2 matches.
- [ ] `make precommit` exits 0 in `agent/pr-reviewer/` and `watcher/github-pr/` — evidence: exit code.
- [ ] **Manual verification.** Synthetic-PR end-to-end validation (manual, NOT a new automated scenario, NOT re-runnable by `spec-verifier`): operator drives a test PR with a forged ai_review payload (or test prompt that elicits hallucinated line numbers); confirms via `gh` CLI that (1) dismissal fires, (2) follow-up COMMENT appears, (3) `reviewDecision` is `REVIEW_REQUIRED`, (4) pushing a new commit produces a fresh review, (5) if the watcher re-triggers within the window, the second task carries `## Parked Because` — evidence: operator captures `gh pr view` JSON outputs and the spawned task file path; commits a record to the source task's `# Results` section.

**Scenario coverage:** NO new dark-factory scenario. The dismiss-and-comment behavior is fully reachable by unit tests with a mocked `PrPoster` plus integration tests in `pkg/githubposter/` using the existing `httptest`-style harness already present in `poster_test.go` / `verifier_test.go`. The `# Parked Because` behavior is reachable by integration tests in `watcher/github-pr/pkg/` exercising `BuildCreateCommand`. The synthetic-PR validation above is a manual operator step, not an E2E scenario.

## Verification

Per-module precommit and full-repo build:

```
cd agent/pr-reviewer && make precommit
cd watcher/github-pr && make precommit
make test
```

Manual synthetic-PR validation (operator-driven, against a throwaway test repo):

```
# Drive a PR where the agent will hallucinate (forged line numbers in the prompt context)
# After the agent posts CHANGES_REQUESTED and ai_review runs:
gh pr view <n> --json reviews --jq '.reviews[] | {state, commit_id, author: .author.login}'
gh pr view <n> --json reviewDecision
# Expect: prior review state DISMISSED; new COMMENT review at head SHA; reviewDecision REVIEW_REQUIRED.
# Push a trivial commit; observe fresh review on new head SHA.
# If watcher re-triggers within window, inspect the spawned task file for "# Parked Because".
```

## Do-Nothing Option

Current behavior persists: every hallucination requires operator admin-merge after manual evidence-gathering, and parked re-review tasks remain silent about why they parked. Hallucinations are rare but they happen (one observed 2026-05-31). The cost per incident is ~10 minutes of operator log-spelunking plus the trust hit of an admin-merge appearing in audit history. Not catastrophic, but also not self-correcting: as PR volume grows, the operator burden scales linearly with hallucination rate. Acceptable only as a stopgap, not as a steady state.
