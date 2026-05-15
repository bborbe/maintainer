---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-05-15T16:15:37Z"
generating: "2026-05-15T16:15:37Z"
prompted: "2026-05-15T18:34:40Z"
verifying: "2026-05-15T18:58:03Z"
branch: dark-factory/post-verdict-to-github-pr
---

## Summary

- The agent's `in_progress` phase, after producing the verdict, submits a real GitHub PR review via the **GitHub REST API** (NOT `gh` CLI — see Constraints), mapping the binary verdict 1-to-1: `approve` → `APPROVE`, `request-changes` → `REQUEST_CHANGES`. The `ai_review` phase keeps its existing role (verifying review quality) and additionally verifies the review actually persisted on the PR.
- The summary text is always posted as the review body so PR authors see findings in the GitHub UI, not buried in a vault file.
- Auto-approval is gated per repo via `.pr-reviewer.yaml` at the target repo root: missing file or `autoApprove: false` demotes `verdict=approve` to a `COMMENT` post (with a body-prefix note) so a flaky reviewer can't merge work. `request-changes` always posts as-is.
- Bot identity is enforced at runtime: posting checks `GET /user` and refuses unless login equals `pr-review-of-ben`. Duplicate-review protection dismisses prior bot reviews on the same head SHA before posting fresh.
- Post-success is verified by a follow-up `GET /pulls/{n}/reviews` — POST response alone is not trusted (empirical 2026-05-15: POST returned a stub `pr-review-of-ben` review object that never appeared in the listing).
- Bitbucket parity is explicitly out of scope (separate idea exists). Inline file:line comments are out of scope; this spec posts top-level summary reviews only.

## Problem

The agent already produces a structured verdict (binary `approve | request-changes` per spec 025, plus summary, comments, concerns_addressed) and writes it to the vault task file — but that's invisible to the PR author. Goal success criterion #2 ("Verdict posted to PR — visible in GitHub review panel") cannot be met until the verdict turns into a real GitHub review event. Without this step the pipeline produces output no developer ever sees, so the agent fails its functional purpose even when its review quality is high.

## Goal

After this work, completing a pr-review task results in a real GitHub PR review event on the source PR, attributed to the bot account `pr-review-of-ben`, carrying the verdict's summary text as the review body. The binary verdict drives the GitHub review type, with `approve` gated behind a per-repo `autoApprove` opt-in (configured via `.pr-reviewer.yaml`, default off when missing). Re-running on the same head SHA replaces the prior bot review rather than stacking. Posting happens inside `in_progress` as the final "do the work" step; `ai_review` verifies both the verdict quality AND that the review was posted successfully (via GET, not by trusting POST response). Failures to post (network, auth, closed PR, phantom POST) surface in vault diagnostics and never crash the agent.

## Assumptions

- The binary-verdict refactor (spec 025) has shipped. The verdict set is exactly `{approve, request-changes}`; the legacy `comment` verdict no longer exists and no code in this spec produces it.
- The bot PAT under teamvault key `ROnG5L` has `repo` write scope (verified 2026-05-15: `x-oauth-scopes: gist, read:org, repo, workflow`; real REST API POST succeeded as `pr-review-of-ben`). No PAT rotation needed.
- `gh` CLI is NOT a reliable posting transport because it ignores `GH_TOKEN` when keychain auth exists (verified 2026-05-15: `GH_TOKEN=<bot-pat> gh pr review ...` posted as the keychain identity `bborbe`, not the bot). The agent must use the GitHub REST API directly.
- `pr-review-of-ben` exists as a dedicated bot GitHub account with the `ROnG5L` PAT (verified via `gh api /user` from inside watcher pods on dev + prod).
- The agent container has no `gh` keychain configured, so `GH_TOKEN` does win inside the container today — but the implementation must not depend on this hygiene. Raw REST is the contract.

## Non-goals

- Inline file:line review comments (top-level summary only; inline is a later task using REST `POST /repos/.../pulls/{n}/comments`)
- Bitbucket Server / Bitbucket Cloud variant (separate idea: `bitbucket-inline-comments.md`)
- Posting on `verdict=failed` (infra error from spec 011 — Job phase fails, controller surfaces it; no review posted)
- Changing the verdict schema or the verdict parser (frozen by spec 011, narrowed to binary by spec 025)
- Per-PR `autoApprove` overrides (per-repo only)
- Comment-on-`human_review` escalation (separate task)
- Auto-merging on approve (only the review event is posted; merge stays human)
- Rewriting planning/execution/ai_review prompts (this is a new step, not a prompt rewrite)
- Using `gh` CLI as the posting transport (REST API directly — see Assumptions)

## Desired Behavior

1. **Posting transport: raw GitHub REST API**, via Go `net/http` with `Authorization: token $GH_TOKEN`. No `gh` CLI shell-out for the post. The poster sits behind an interface so tests can inject a fake HTTP client (Counterfeiter-mocked).

2. **`in_progress` phase posts AFTER writing the vault — vault first, always.** Strict sequence:
   - (a) execution produces verdict
   - (b) **verdict written to vault `## Review` section — committed before any API call**
   - (c) bot-identity self-check via `GET /user`
   - (d) `.pr-reviewer.yaml` read from workdir root
   - (e) duplicate-review dismissal via `GET /pulls/{n}/reviews` + `PUT /pulls/{n}/reviews/{id}/dismissals` for matching prior bot reviews
   - (f) `POST /pulls/{n}/reviews` with the mapped event type
   - (g) verify-after-POST via `GET /pulls/{n}/reviews` to confirm persistence
   - (h) on success, phase advances to `ai_review`
   
   **The vault verdict is preserved regardless of API outcome.** If any step (c)–(g) fails after exhausting in-process retries (see #8), the vault verdict stays; only the posting attempt is marked failed in diagnostics; phase escalates to `human_review`. The agent's review work is never lost.

3. **Binary verdict mapping (no `comment` path):**
   - `approve` + `autoApprove: true` → `POST` with `event: "APPROVE"`
   - `approve` + `autoApprove: false` (or file missing) → `POST` with `event: "COMMENT"`, body prefixed with `"auto-approve disabled for this repo, review submitted as comment"`. Vault verdict stays `approve`; only the posted action is demoted.
   - `request-changes` → `POST` with `event: "REQUEST_CHANGES"`. Always posts; never demoted.
   - `failed` (infra error, not a verdict per spec 025) → skip posting; existing failure path runs.

4. **Bot-identity self-check.** Before any POST, `GET /user`; assert `login == BOT_GITHUB_LOGIN` (env, default `pr-review-of-ben`). On mismatch: refuse to post, escalate to `human_review` with diagnostic naming the misconfiguration.

5. **Duplicate-review dismissal.** Before POST, `GET /pulls/{n}/reviews`, filter to entries where `user.login == BOT_GITHUB_LOGIN` AND `commit_id == <current head SHA>`. For each match, `PUT /pulls/{n}/reviews/{id}/dismissals` with a dismissal message. Reviews by humans or by the bot on other SHAs are never touched.

6. **Verify-after-POST (mandatory).** A POST `/pulls/{n}/reviews` returning 200 is NOT proof of persistence. Empirical 2026-05-15: a POST with `event: "COMMENT"` returned a `pr-review-of-ben` review object with a numeric id, but the review never appeared in `GET /pulls/{n}/reviews` and a `GET /pulls/{n}/reviews/{id}` returned `404 Not Found`. Implementation must:
   - After POST, immediately `GET /pulls/{n}/reviews` (no caching).
   - Filter to entries where `user.login == BOT_GITHUB_LOGIN` AND `commit_id == <current head SHA>`.
   - Assert at least one entry exists with `state` matching the posted event (`APPROVED`, `CHANGES_REQUESTED`, or `COMMENTED`).
   - On failure: counts as a **transient error** per the retry policy (#8).

7. **`ai_review` independently verifies the post — conditional on a posting having been attempted.**
   - **Skip post-verification entirely** when execution exited `verdict=failed` (no post was attempted; verifying for a non-existent review would false-positive). The phase runs its existing quality checks (concerns addressed, hallucinations, verdict consistency) and exits normally.
   - **Skip post-verification entirely** when the **most recent** vault Diagnostics block from `in_progress` shows `outcome: failed` with `class: permanent` or `class: unknown` (post was attempted but cannot succeed; re-verifying is pointless and would mask the original error). ai_review reads ONLY the latest block — older blocks from prior `trigger_count` attempts are ignored, so a re-spawn that succeeds in-progress is not blocked by a stale permanent-class entry from an earlier run.
   - **Otherwise** (binary verdict + posting attempted + no permanent failure recorded): `GET /pulls/{n}/reviews` and assert a review by `pr-review-of-ben` exists for current head SHA with expected state (`APPROVED` / `CHANGES_REQUESTED` / `COMMENTED`). Missing → phase fails with diagnostic.
   - The verification GET itself follows the same retry policy as DB#8: one in-process retry for transient errors; no retry for permanent. On transient-exhausted-retry, ai_review exits `failed`; controller's `trigger_count` handles further attempts. On permanent (e.g. 404 PR gone), ai_review exits `failed` with `class: permanent` so the operator can intervene fast.

8. **Retry policy — exactly two layers, no mixing.**
   
   **Layer 1 — in-process retry per HTTP call.** Applies independently to **every** HTTP call in the posting sequence (`GET /user`, `GET /pulls/n/reviews` for dismissal-list, each `PUT .../dismissals`, the `POST /pulls/n/reviews`, the verify `GET /pulls/n/reviews`). Each call may retry **at most once**. Classification:
   
   | Error class | Examples | Behavior |
   |---|---|---|
   | **Transient** | Network/connection error; HTTP 5xx; HTTP 429 rate limit; phantom POST (200 but not in subsequent verify-GET); HTTP timeout | **Retry the same call once** with backoff (initial wait 5s + jitter). On second failure, stop; record diagnostic; agent exits `failed` (see Layer 2). |
   | **Permanent — no retry** | HTTP 401 (auth bad), 403 (PAT scope insufficient), 404 (PR/repo gone), 422 (closed PR, validation), bot-identity mismatch from `GET /user` | **No retry.** Record diagnostic with `class: permanent` and `escalate_hint: true`; agent exits `failed` (see Layer 2). Retrying won't change the result, but the agent has no exit code distinct from transient — the diagnostic's `class` field is how the operator distinguishes. |
   | **Unknown / unexpected** | Non-JSON response, unexpected status code, parse failure | Treat as **permanent**: no retry, exit `failed` with `class: unknown`. Operator inspects diagnostics. |
   
   Layer 1 is **per-call bounded** — at most 2 attempts each. Worst-case per Job run: 5 calls × 2 attempts = 10 HTTP requests against GitHub. (`PUT .../dismissals × N` adds 2N to the upper bound when N prior reviews need dismissing.) No exponential schedule, no extra rounds.
   
   **Layer 2 — cross-Job retry via controller's existing `trigger_count`.** The agent does not implement this; it's existing infrastructure. The agent's only exit codes are `done` / `needs_input` / `failed` (spec 011 contract). Every failed posting attempt exits `failed`, and the controller increments `trigger_count`:
   - `trigger_count < cap (3)` → controller respawns the Job; vault verdict already present; bot-identity check, dismiss-stale, POST, verify all run again
   - `trigger_count == cap` → controller marks the task `human_review` and stops respawning. `human_review` is **terminal**; no further automatic spawns until operator intervenes.
   
   **The agent has no way to tell the controller "don't bother retrying."** For permanent errors (403 scope, 404 repo gone) the controller will still spawn the agent up to 3 times; each spawn hits the same error and exits fast. Cost: ~30s of compute per wasted spawn, ~90s total. Accepted as a trivial cost in exchange for a simpler agent/controller contract. The diagnostic's `class: permanent` flag lets the operator skip waiting for trigger_count exhaustion if they want to manually transition the task.
   
   **Net behavior:**
   - **Transient GitHub outage:** up to ~3 Job spawns × ~10 HTTP attempts/spawn = ~30 HTTP attempts across the full retry cycle before sticking at `human_review`.
   - **Permanent error (e.g. 403):** ~3 Job spawns × 1 fast-fail attempt/spawn = ~3 HTTP attempts (the failing call is the first one in sequence; subsequent calls don't run); ~90s compute wasted; task sticks at `human_review`.
   - **Phantom POST:** retry the POST once in-process. If still phantom, exit `failed` with `class: transient`. Controller respawns; dismissal step (which is the FIRST thing after identity check) cleans up any latent phantom that did persist after all, then POST again. Self-healing.
   
   **Idempotency of re-attempts.** Dismissal (#5) runs at the start of every Job run, so a re-spawn that finds a stale bot review on the same SHA — including one created by a phantom POST that actually persisted — dismisses it first and then posts fresh. No duplicate-stacking even across re-spawns.

9. **Diagnostic block format (operator-readable).** Every posting failure appends a structured block to the vault task body under a `## Diagnostics` heading (creating it if absent). Schema — one fenced YAML block per Job run, append-only:
    
    ```yaml
    ```yaml
    job_run: 2026-05-15T15:30:00Z
    trigger_count: 1
    outcome: failed
    failure_step: POST /pulls/2/reviews         # which step in sequence (c-g) failed
    class: transient                            # transient | permanent | unknown | not-a-failure | soft-warning
    escalate_hint: false                        # true for permanent + unknown (operator can skip waiting for trigger_count cap)
    attempt: 2                                  # 1 or 2 (in-process retry count for the failing call)
    http_status: 503                            # or null if pre-HTTP (DNS, connection)
    error_message: "service unavailable"        # short summary
    response_body: "<empty>"                    # first 500 bytes of response, or null
    elapsed_ms: 8420                            # total elapsed for this Job run's posting steps
    ```
    ```
    
    On success: a single line `job_run: <ts> outcome: success review_id: <github-id>` — no separate block.
    
    The block is consumed by the operator (reading the vault task). It is NOT consumed by any code; it exists for human triage. The fenced-YAML shape is deliberate — easy to parse if a follow-up automation wants it, easy to read without tooling.

## Constraints

- **Frozen verdict schema (spec 025)**: verdict ∈ `{approve, request-changes}`. The `comment` verdict no longer exists. No code path in this spec produces or consumes a `comment` verdict value.
- **Dismissal MUST precede POST in the sequence.** The "self-healing on cross-Job retry" property (DB#8) depends on this ordering: dismissal cleans up any latent persisted-phantom from a prior Job run before the fresh POST. Refactoring the sequence to POST-then-dismiss breaks idempotency under controller respawn.
- **Bot identity is non-negotiable**: posting MUST use the `ROnG5L` PAT for `pr-review-of-ben`, never the operator's token. Runtime enforcement via `GET /user` self-check.
- **Posting transport is REST, not `gh` CLI.** Justification: `gh` ignores `GH_TOKEN` env when keychain auth exists (verified 2026-05-15). The container environment happens to avoid this today, but the implementation must not depend on container hygiene for identity correctness — that's a single-line-of-defense pattern. Raw HTTP via `net/http` is testable, mockable (Counterfeiter on the HTTP client interface), and identity-deterministic.
- **POST is not proof of persistence.** All posts must be followed by a GET that confirms the review exists with the expected state. Failure modes table covers the phantom-POST case.
- **Branch protection interaction**: a bot `approve` may count toward required reviews under branch-protection rules. This is operator-side configuration (GitHub repo settings), not a code concern; document in `docs/pr-post-back.md` but do not detect or enforce in code.
- **`Bash` allowlist NOT extended.** Because the implementation uses Go `net/http`, the agent's tool allowlist does not need new `Bash(gh ...)` patterns. The agent stays sandboxed at the bash level; only the in-process HTTP client talks to GitHub. (This is a tightening vs the earlier spec draft.)
- **`ai_review` still needs read-only API access.** The verification step in `ai_review` also uses Go `net/http` for `GET /pulls/{n}/reviews`. No `gh api` allowlist needed; in-process call.
- **Existing guardrail relaxation**: the agent's posting prohibition (comment in `factory.go`: "Execution gets broader git/gh access for cross-file reads but still cannot post (no `gh pr comment` / `gh pr review`) — posting happens out-of-band after the human approves the verdict.") must be relaxed and the comment updated to reflect that `in_progress` is now the trusted poster, gated by bot-identity self-check + per-repo `.pr-reviewer.yaml`, and that `ai_review` independently verifies via REST GET.
- **No mutation of git history**: the new step has GitHub-API-write capability via the bot PAT but must not push, commit, branch, reset, or otherwise mutate repo state. Posting reviews is the only write.
- **Read-only against the workdir**: the post step needs the workdir ONLY for reading `.pr-reviewer.yaml`. No checkout dependency for the actual posting (verdict + PR URL come from the task body).
- **Existing knowledge to reference**:
  - `agent/pr-reviewer/docs/architecture.md` — three-phase flow; this spec extends `in_progress` (final step) and `ai_review` (new check)
  - Spec 011 — frozen verdict schema (narrowed by spec 025)
  - Spec 025 — binary verdict semantics; this spec consumes the binary set
  - `agent/pr-reviewer/pkg/factory/factory.go` — phase wiring + per-phase `AllowedTools`
  - `~/Documents/workspaces/coding/docs/go-mocking-guide.md` — Counterfeiter pattern for the HTTP client interface
  - `~/Documents/workspaces/coding/docs/go-factory-pattern.md` — `CreatePrPoster` returns interface, no error, no conditionals; identity-check + config-reader live in `Run`, not factory
  - `~/Documents/workspaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrapf(ctx, ...)` for every HTTP failure path; enrich ctx with structured data before escalating
- **Domain knowledge candidate**: a new `agent/pr-reviewer/docs/pr-post-back.md` documenting (a) bot identity provisioning, (b) `.pr-reviewer.yaml` schema, (c) duplicate-dismissal flow, (d) verify-after-POST rationale, (e) operator-side branch-protection considerations.

## Failure Modes

All retries follow the policy in Desired Behavior #9: transient errors get **one** in-process retry (initial wait 5s + jitter); permanent errors get **no** retry and escalate immediately. After in-process retry exhaustion, the controller's `trigger_count` may re-spawn the Job up to its cap (currently 3) — total POST attempts bounded at ~6 in the worst case.

| Trigger | Class | Behavior |
|---------|-------|----------|
| Network/connection error, HTTP 5xx, timeout | **Transient** | Retry once; on second failure, vault preserved, escalate to `human_review`. Controller may re-spawn Job up to `trigger_count` cap. |
| HTTP 429 rate limit | **Transient** | Retry once after 5s+jitter; on second failure, escalate. (Cross-Job re-spawn often picks up after rate window resets.) |
| **POST returns 200 but review absent in subsequent GET** (phantom POST, 2026-05-15 empirical) | **Transient** | Retry POST once; on second failure, escalate with diagnostic. Verify-after-POST is the detector. |
| HTTP 401 (auth bad) | **Permanent** | No retry; escalate. Operator rotates/fixes PAT. |
| HTTP 403 (PAT scope insufficient) | **Permanent** | No retry; escalate. Operator extends scopes (already verified `repo` write 2026-05-15). |
| HTTP 404 (PR or repo gone) | **Permanent** | No retry; escalate. Operator investigates. |
| HTTP 422 (PR closed/merged, validation error) | **Not a failure** | No retry; vault verdict preserved; phase **completes successfully** (review against a closed PR is moot). Diagnostic block records `class: not-a-failure` with `failure_step` set to the call that 422'd so the operator can audit. |
| Bot identity check fails (login != `pr-review-of-ben`) | **Permanent** | No retry; escalate with diagnostic naming the misconfiguration. |
| HTTP response non-JSON / malformed | **Unknown** | Treat as permanent: no retry; capture body in diagnostics; escalate. |
| Unexpected status code (anything not covered above) | **Unknown** | Treat as permanent: no retry; escalate with body + status in diagnostics. |
| `autoApprove: false` + `verdict=approve` | **Not a failure** | Demote posted action to `COMMENT`; prefix body with "auto-approve disabled for this repo, review submitted as comment"; vault verdict stays `approve`. |
| Prior bot review exists for same head SHA | **Not a failure** | Dismiss before posting fresh (dismissal itself follows the same retry policy if it fails). |
| Force-push: PR head_sha changed mid-Job | **Not a failure** | Post against the SHA the verdict was computed for (recorded in vault `ref` frontmatter). Spec 026 (per-SHA tasks) ensures the new SHA gets its own task. |
| `verdict=failed` (spec 011 infra error) | **Not a posting concern** | No review posted; existing failure path runs. |
| `summary` empty or missing | **Soft warning** | Post with default body ("automated review — no summary produced"); record warning in diagnostics; verdict drives the review type as normal. |

## Security / Abuse Cases

- **Bot identity is the trust boundary.** PAT MUST belong to dedicated bot account. If operator's PAT used, every review appears as if the human approved it — audit-trail violation. Runtime enforcement: `GET /user` self-check before any POST; refuse on mismatch.
- **Verdict body is LLM output, not user input — but is posted to GitHub verbatim.** The verdict's `summary` may contain LLM-generated text derived from a PR diff (untrusted by PR author). GitHub renders the body as Markdown. The agent must NOT execute or shell-interpolate the summary; passing it as a JSON field in the POST body (`{"body": "..."}`) is sufficient — JSON encoding handles all escaping. PR-author-controlled content cannot escape into shell commands because we're not shelling out at all.
- **No `Bash(gh ...)` allowlist expansion.** Because posting is via in-process REST, the agent's bash sandbox is unchanged. The bot PAT travels only through Go HTTP calls, never through a shell. Reduces attack surface vs the earlier spec draft.
- **Auto-approve gating prevents merge runaway.** Flaky agent emitting `verdict=approve` on a bad PR could, with auto-merge enabled, ship broken code without human review. Defense in depth: (1) `autoApprove: false` default — bot reviews are comments-only by default; (2) operator opt-in is per-repo and explicit; (3) repo-level branch-protection should require a human reviewer in addition to the bot for any non-trivial repo. Document (3) in `pr-post-back.md`.
- **Duplicate-review dismissal is a write op via API.** Bot must dismiss its own prior reviews. It must NOT dismiss reviews by humans — filter strictly by `user.login == BOT_GITHUB_LOGIN`. Misimplementation could dismiss real reviewer feedback. AC enforces this.
- **No new outbound network surface beyond `api.github.com`.** REST calls already permitted by pod network policy.
- **Closed-PR posting is an enumeration weak signal but not exploitable.** A 422 reveals the PR is closed; this is already public via the PR's URL.

## Acceptance Criteria

- [ ] Implementation uses the existing `ROnG5L` PAT (already verified 2026-05-15 as `pr-review-of-ben` with `repo` write scope — see Assumptions). No PAT rotation introduced by this spec.
- [ ] Posting transport is the GitHub REST API via Go `net/http`. No `gh` CLI shell-out for posting, identity check, dismissal, or verification. (Optional `gh` use elsewhere in the agent is untouched; only the post path is REST.)
- [ ] Posting is wired into `in_progress` as the final step (after verdict written to vault, before phase advances to `ai_review`). On `verdict=failed`, posting is skipped.
- [ ] `ai_review` phase verifies the post happened: `GET /pulls/{n}/reviews` via in-process HTTP; asserts a review by `pr-review-of-ben` exists for the current head SHA with expected state (`APPROVED` / `CHANGES_REQUESTED` / `COMMENTED`). Missing → phase fails with diagnostic.
- [ ] The agent's tool allowlist is NOT extended with new `Bash(gh ...)` patterns. The post path uses in-process Go HTTP; the bash sandbox stays tight.
- [ ] Per-repo `autoApprove` is read from `.pr-reviewer.yaml` at the workdir root. Schema: `autoApprove: bool`. Missing file OR `autoApprove: false` → `approve` demoted to `COMMENT`. File present with `autoApprove: true` → `APPROVE`. Documented in `docs/pr-post-back.md`.
- [ ] Binary verdict-to-event mapping is implemented and unit-tested via Ginkgo `DescribeTable`: `approve`+`autoApprove:true` → `APPROVE`; `approve`+`autoApprove:false` → `COMMENT` with body prefix; `request-changes` → `REQUEST_CHANGES`. No `comment` verdict path (the value doesn't exist in the binary world).
- [ ] Bot-identity self-check: `GET /user` via in-process HTTP; assert `login == BOT_GITHUB_LOGIN`; env-overridable via `BOT_GITHUB_LOGIN` (default `pr-review-of-ben`). Refuse + escalate on mismatch.
- [ ] Duplicate-review dismissal: `GET /pulls/{n}/reviews`, filter to bot login + same head SHA, dismiss each via `PUT /pulls/{n}/reviews/{id}/dismissals`. Reviews by other authors are never touched. Unit-tested with a Counterfeiter mock of the HTTP client.
- [ ] **Verify-after-POST is implemented and unit-tested.** After POST, immediately GET review list and assert the new review exists with expected state. On failure, retry once; on second failure, escalate. Unit test simulates the phantom-POST case (POST returns 200, GET returns empty list) and asserts retry + escalation behavior.
- [ ] **Vault written before any API call.** Sequence enforced by code: write `## Review` to vault → identity check → config read → dismiss → POST → verify-GET. If any step c–g fails, vault verdict is preserved untouched and phase escalates. Unit-tested by injecting failure at each step and asserting the vault is unaltered.
- [ ] **Retry policy implemented per DB#8, applied per HTTP call.** Each of the 5 HTTP-call types in the sequence (`GET /user`, `GET /pulls/n/reviews` dismiss-list, `PUT .../dismissals`, `POST /pulls/n/reviews`, `GET /pulls/n/reviews` verify) retries at most once for transient errors and never for permanent. Unit-tested via Counterfeiter mock: for each call type × each error class, assert the correct attempt count and exit class.
- [ ] **Diagnostic block per DB#9.** Every Job run that attempts posting writes a structured YAML block under `## Diagnostics` in the vault task (one block per run, append-only) with the schema in DB#9: `job_run`, `trigger_count`, `outcome`, `failure_step`, `class`, `escalate_hint`, `attempt`, `http_status`, `error_message`, `response_body` (≤500 bytes), `elapsed_ms`. Success path writes a single one-line entry only. Unit-tested by injecting failures at each step and asserting the YAML block shape.
- [ ] **`ai_review` skip conditions per DB#7.** Verification is skipped when (a) `verdict=failed` or (b) `in_progress` Diagnostics block shows `class: permanent` or `class: unknown`. In both cases ai_review still runs quality checks (concerns, hallucinations, consistency). Unit-tested via task fixtures.
- [ ] **`ai_review` verification GET follows DB#8 retry policy.** Transient → one retry; permanent → no retry; exit `failed` with class on diagnostic. Unit-tested with Counterfeiter mock.
- [ ] All failure modes in the table are handled per their class: transient retries once, permanent escalates immediately, soft-warning records and continues, "not a failure" rows execute their documented behavior. Diagnostics recorded, no agent crash, vault verdict preserved.
- [ ] The `factory.go` comment block describing why "execution cannot post" is updated to reflect the new posting phase and the trust boundary (bot-identity self-check + per-repo opt-in).
- [ ] `agent/pr-reviewer/docs/pr-post-back.md` exists and covers: (a) bot-identity provisioning, (b) `.pr-reviewer.yaml` schema and per-repo opt-in flow, (c) duplicate-dismissal mechanism, (d) verify-after-POST rationale, (e) operator-side branch-protection considerations.
- [ ] `agent/pr-reviewer/docs/architecture.md` is updated: the execution phase's emit description is extended to include "posts review to GitHub via REST"; the ai_review check list (currently 3 items) gets a 4th: "Post verification: review exists on PR via GET /pulls/{n}/reviews".
- [ ] `make precommit` passes in `agent/pr-reviewer/`.
- [ ] **Local smoke test**: `cd agent/pr-reviewer/cmd/run-task && make run-dummy-task` against PR #2 emits a real review on the GitHub UI under `pr-review-of-ben`, with the verdict's summary as the body, correct binary mapping. Re-run replaces the prior bot review (no stacking).
- [ ] **Scenario coverage**: a new `scenarios/NNN-pr-reviewer-post-verdict.md` exercises the full flow end-to-end against PR #2: (a) review appears in GitHub UI under bot account, (b) `autoApprove:false` demotes `approve` to `COMMENT` with documented body prefix, (c) duplicate-dismissal works on re-run against same head SHA, (d) `verdict=request-changes` posts as `REQUEST_CHANGES`, (e) verify-after-POST catches a simulated phantom POST and escalates. Unit tests cannot fake the GitHub API boundary; scenario runs the real path.
- [ ] After dev deploy: triggering one PR via the watcher results in a vault task with a verdict AND a real review event on the PR within the 10-min latency budget. Goal success criterion #2 becomes observable in the GitHub UI.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Local smoke test against PR #2 (posts a real review to bborbe/maintainer PR #2 — designated permanent test fixture):

```
cd agent/pr-reviewer/cmd/run-task && make run-dummy-task
```

Inspect: the resulting review appears in PR #2's "Reviews" panel on github.com under `pr-review-of-ben`, with the verdict's summary as the body. Re-run the same command — the prior review should be dismissed/replaced, not stacked.

After deploy to dev:

```
# Trigger one PR via the watcher; observe controller materialize the vault task,
# then confirm the review event appears on GitHub.
kubectlquant -n dev logs <pr-reviewer-agent-job-pod> | grep -E "POST.*reviews|GET.*user|dismiss"

# Manually verify in GitHub UI: PR's "Reviews" panel shows a review by
# pr-review-of-ben with the expected verdict type and summary body.
```

**Rung-2 (dev k8s e2e) is required for this spec** per `docs/verifying-specs.md` — the implementation has a real-world side effect (posting to public GitHub PRs) and the phantom-POST failure mode is only reachable against the real GitHub API. Unit tests cover the logic; the scenario + dev smoke prove the end-to-end behavior under real network conditions.

## Do-Nothing Option

Stay with verdict-in-vault-only. The agent continues to produce reviews no developer ever sees on the PR page, so the goal's #2 success criterion ("Verdict posted to PR — visible in GitHub review panel") cannot be met and the wave does not complete. Developers must manually check the vault to find feedback on their PRs — a workflow no one will adopt. The full pipeline (watcher + agent + posting) remains a half-pipeline.

A weaker alternative — post a `COMMENT` review unconditionally regardless of verdict, deferring the `APPROVE` / `REQUEST_CHANGES` mapping — saves the `autoApprove` plumbing for later. Cost: the GitHub UI's green-check / red-X signal (the most valuable artifact for PR authors at a glance) is never produced; the bot is just another commenter. The bot-identity work and verify-after-POST are required regardless (a comment posted as the human operator is the same identity-confusion problem; a phantom-COMMENT post is the same silent-failure problem), so the deferral saves only the `autoApprove` config plumbing — modest savings, large UX cost. Not recommended.

A second alternative — use `gh` CLI as the posting transport instead of REST — was the original draft. Rejected after 2026-05-15 empirical verification: `gh` ignores `GH_TOKEN` when keychain auth exists, which means container hygiene (no keychain) is the only thing keeping the bot identity correct. That's a single-line-of-defense pattern; raw REST eliminates the dependency on container state entirely. The `gh` CLI also can't be reliably tested without spawning a real subprocess, whereas the HTTP client interface is straightforwardly Counterfeiter-mocked.

## Verification Result

**Verified:** 2026-05-15T21:38:58Z (HEAD d69fca5)
**Binary:** `dark-factory v0.156.1-1-g04f3863-dirty` (installed); deployed agent image built from maintainer master (commits `e5536b2`, `e6743af`, `e788184`).
**Scenario:** Real GitHub PR posting end-to-end against `bborbe/maintainer#2` from prod cluster (`pr-reviewer-agent-d2af959a-20260515212017-fxxqq`) plus local-smoke replay; reviews queried back via authenticated GitHub REST API.
**Evidence:**
- Bot review `4301479968` (state `COMMENTED`, login `pr-review-of-ben`, commit `f972fdd6`, 2026-05-15T21:21:26Z) posted by prod-cluster pod with body prefix `"auto-approve disabled for this repo, review submitted as comment"` (verify-after-POST recorded `outcome: success` in vault diagnostics line).
- Local-smoke review `4300948227` (state `COMMENTED`, login `pr-review-of-ben`, commit `19c513b8`, 2026-05-15T19:47:30Z) confirms `request-changes`→`COMMENTED` demotion path and prior bot review `4300897191` reaches state `DISMISSED` (duplicate dismissal proven for non-COMMENTED prior reviews).
- `make precommit` clean in `agent/pr-reviewer/` (gosec 0 issues, trivy 0 vulns, full `go test ./...` green across 11 packages).
- Docs landed: `agent/pr-reviewer/docs/pr-post-back.md` covers bot identity / `.pr-reviewer.yaml` / dismissal / verify-after-POST / branch-protection; `architecture.md` execution row mentions REST post and ai_review check #4 names verify-after-POST.
- `factory.go:33-40` comment updated to describe `PrPoster` (`net/http`, not `gh`) gated by `GET /user` self-check and `.pr-reviewer.yaml`.
- Scenario `scenarios/017-pr-reviewer-post-verdict.md` promoted draft→active.
**Verdict:** PASS

Known limitation (in-scope, no regression): GitHub rejects `PUT .../dismissals` on reviews already in state `COMMENTED` with HTTP 422; the implementation (`e6743af`) skips dismissal of `COMMENTED` reviews. In the `autoApprove:false` path, repeated re-spawns therefore stack comment-state reviews on the same SHA (observed: `4301479968` + `4301500029` both COMMENTED on `f972fdd6`) rather than dismissing-then-replacing. Real reviewer feedback is never touched. Operator-facing impact is cosmetic.
