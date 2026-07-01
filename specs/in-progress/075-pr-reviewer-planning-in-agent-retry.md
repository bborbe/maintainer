---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-07-01T09:19:46Z"
generating: "2026-07-01T09:19:46Z"
prompted: "2026-07-01T09:24:55Z"
verifying: "2026-07-01T09:29:41Z"
branch: dark-factory/spec-075
---

## Summary

- The pr-reviewer planning step sometimes gets malformed JSON from Claude (e.g., MiniMax-M2.7-highspeed emitting `B` from "Based on..." instead of `{`).
- Today (spec 074 shipped) that lands as `AgentStatusFailed` on the very first bad response, and the PR is stuck in silent `REVIEW_REQUIRED` limbo until an operator SHA-bumps the branch.
- This spec adds an in-agent retry loop around the Claude planning call: up to 3 attempts total, no backoff, retry only on JSON parse failure.
- On intermittent bad output (the observed real cases), the retry self-corrects in 1-2 attempts and the review proceeds without operator intervention.
- Only if all 3 attempts return malformed JSON does the step return `AgentStatusFailed` — same failure surface as today, just meaningful instead of spurious.

## Problem

Spec 074 shipped write-time JSON validation in the pr-reviewer planning step: if Claude returns non-parseable output, the agent refuses to persist `## Plan` and returns `AgentStatusFailed`. That prevents corrupt data, but it does not recover: the controller sees a clean Job exit, dedups against the same SHA, and the PR sits in `REVIEW_REQUIRED` with no visible signal. The only operator escape is to push a trivial commit and hope the retried run gets clean JSON — a band-aid. In one recent session two PRs (bborbe/agent-task-controller#2, bborbe/recurring-task-creator#24) hit this via MiniMax emitting `B` as the leading character. Because these failures are intermittent per-call, an in-process retry almost always recovers.

## Goal

The pr-reviewer planning step retries the Claude call on malformed JSON up to 3 times per invocation. Intermittent bad output is transparently recovered; `AgentStatusFailed` is returned only after all 3 attempts produce unparseable JSON. Every other observable behavior (persistence, routing, idempotency, transport-error handling) is unchanged.

## Non-goals

- Do NOT add controller-side requeue on `AgentStatusFailed` — that is Layer 2 in a separate spec against `bborbe/agent-task-controller`.
- Do NOT post a COMMENT review to the PR on exhaustion — Layer 3, controller-side.
- Do NOT append retry entries to the task file `## Progress` section — Layer 3.
- Do NOT change the planning prompt text — Layer 0 already shipped that in spec 074.
- Do NOT add retry to the execution, ai_review, or verdict steps — planning-only.
- Do NOT switch pr-reviewer off MiniMax.
- Do NOT make the attempt count configurable via env or config — invariant; if a future consumer demands variation, that is a separate spec.
- Do NOT add backoff/jitter between attempts — invariant for this spec; failures are per-call sampling artifacts, not rate-limit signals.
- Do NOT retry on Claude transport errors (nil result + err) — those are controller-territory.

## Desired Behavior

1. When the planning step invokes the Claude runner and receives a result whose body parses cleanly as planning-concerns JSON, the step persists `## Plan` and routes exactly as it does today (empty concerns → LGTM/done, non-empty → execution).
2. When the planning step invokes the Claude runner and receives a result whose body FAILS the same parse used at write time, the step calls the runner again with the same prompt, up to a total of 3 attempts.
3. When any attempt returns parseable JSON, that response is the one persisted; earlier malformed responses are discarded and not written to disk.
4. When all 3 attempts return malformed JSON, the step returns `AgentStatusFailed` with a message that names "malformed JSON after 3 attempts" and no `## Plan` section is written.
5. When the Claude runner returns a transport error (nil result, non-nil err) on any attempt, the step returns `AgentStatusFailed` immediately without further retries — transport-layer recovery is out of scope here.
6. When the task file already contains a `## Plan` section (idempotent re-entry), the runner is not called at all and routing proceeds from the persisted body.
7. Each retry attempt emits a V(2) glog line that includes the attempt number, the total (`3`), and the parse error, so an operator can grep for real MiniMax `B`-case recoveries in agent logs.

## Constraints

- The attempt cap is a hardcoded package-level constant `maxPlanningAttempts = 3` in the planning step source file. No env var, no flag, no field on the step struct.
- `parsePlanningConcerns` (existing helper) is reused unchanged as the validity check.
- Routing logic downstream of parse-success is unchanged: empty concerns → LGTM/done, non-empty → execution.
- The `AgentStatusFailed` return type/shape is unchanged — only its message string is new.
- Layer 0 (spec 074) write-time validation stays in place; the retry loop supersedes the first-attempt-only check but the same parse function gates persistence.
- Existing tests in `steps_planning_test.go` continue to pass.
- Real Claude runner is not called in unit tests; a fake `ClaudeRunner` drives the test cases.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Attempt 1 malformed, attempt 2 parseable | Log V(2) attempt 1/3 retry, persist attempt 2 output, route normally | Automatic — no operator action |
| All 3 attempts malformed | Return `AgentStatusFailed`, message names "malformed JSON after 3 attempts", no `## Plan` written | Controller / operator (Layer 2/3, out of scope) |
| Runner returns (nil, err) on attempt 1 | Return `AgentStatusFailed` immediately, runner called exactly once | Controller retries via its own failure path |
| `## Plan` already persisted from prior run | Runner not called, routing from persisted body | N/A (idempotent) |
| Attempt N returns parseable JSON that later fails a downstream check | Out of scope — this spec only gates on `parsePlanningConcerns` | N/A |

## Security / Abuse Cases

Not applicable — this change is internal to a trusted controller/executor loop. No new inputs from users cross a trust boundary; the retry loop consumes only the Claude runner output already trusted by the current step. Cost impact: bounded at 3x the current per-invocation Claude cost in the exhaustion case, which is the same bound as one operator-driven SHA-bump today.

## Acceptance Criteria

- [ ] Package-level constant `maxPlanningAttempts = 3` declared in `agent/pr-reviewer/pkg/steps_planning.go` — evidence: `grep -n 'maxPlanningAttempts' agent/pr-reviewer/pkg/steps_planning.go` returns a line and value is `3`.
- [ ] Ginkgo test case "attempt 1 succeeds" passes: fake runner returns valid JSON once, `runner.Run` called exactly 1 time, `## Plan` persisted — evidence: `go test ./agent/pr-reviewer/pkg/... -run "attempt 1 succeeds"` exits 0.
- [ ] Ginkgo test case "attempt 2 succeeds" passes: fake runner returns `"Based on..."` then valid JSON, `runner.Run` called exactly 2 times, `## Plan` persisted with the second response, status is not `AgentStatusFailed` — evidence: `go test` exit 0 and assertions on call count + persisted body content.
- [ ] Ginkgo test case "all 3 attempts fail" passes: fake runner returns malformed on all 3, `runner.Run` called exactly 3 times, `## Plan` NOT persisted, `Result.Status == AgentStatusFailed`, `Result.Message` contains the substring `malformed JSON after 3 attempts` — evidence: `go test` exit 0 with the substring assertion.
- [ ] Ginkgo test case "runner transport error not retried" passes: fake runner returns `(nil, someErr)` on attempt 1, `Result.Status == AgentStatusFailed`, `runner.Run` called exactly 1 time — evidence: `go test` exit 0 with call-count assertion.
- [ ] Ginkgo test case "idempotent re-entry" passes: task file already contains `## Plan`, runner not called at all, routing proceeds from persisted body — evidence: `go test` exit 0 with `runner.Run` call count assertion at 0.
- [ ] Retry log line emitted on each malformed-but-not-exhausted attempt via `glog.V(2).Infof`, format includes the attempt number, `/3`, and the parse error — evidence: `grep -n 'planning: attempt.*malformed JSON, retrying' agent/pr-reviewer/pkg/steps_planning.go` returns a line.
- [ ] `make precommit` exits 0 in `agent/pr-reviewer/` — evidence: exit code.
- [ ] `CHANGELOG.md` under `## Unreleased` (or the next unreleased heading) contains a bullet mentioning pr-reviewer retry on malformed planning JSON — evidence: `grep -n 'pr-reviewer.*retry.*planning' CHANGELOG.md` returns a line.

Scenario coverage: no new scenario. Unit tests with a fake `ClaudeRunner` reach every branch — retry count, persistence, routing, and log emission — without needing real Claude, real Docker, or real cluster. There is no essential user journey that requires an E2E replay for this behavior.

## Verification

Run in `/Users/bborbe/Documents/workspaces/maintainer-plan-retry/`:

```
cd agent/pr-reviewer && make precommit
grep -n 'maxPlanningAttempts' agent/pr-reviewer/pkg/steps_planning.go
grep -n 'malformed JSON after 3 attempts' agent/pr-reviewer/pkg/steps_planning.go
grep -n 'planning: attempt.*malformed JSON, retrying' agent/pr-reviewer/pkg/steps_planning.go
grep -n 'pr-reviewer.*retry.*planning' CHANGELOG.md
```

Post-deploy dev observation (out of band, not a gate):
- `make buca` from `agent-dev`, redeploy pr-reviewer.
- On the next real MiniMax `B`-case, `kubectl logs` for the agent-pr-reviewer pod should show a `planning: attempt 1/3 malformed JSON, retrying` line followed by a normal LGTM/execution route in the same task run — no `AgentStatusFailed`, no SHA-bump needed.

## Do-Nothing Option

Without this spec, Layer 0 remains in place: malformed JSON returns `AgentStatusFailed` on first bad response, PRs get stuck in `REVIEW_REQUIRED`, operators SHA-bump to recover. Given that the failures are intermittent (observed on 2 PRs in a single session with MiniMax), doing nothing at Layer 1 means Layers 2+3 (controller requeue + PR comment) MUST land before we can call the planning path robust. Adding the in-agent retry now removes >90% of the observed manual interventions with ~15 lines of code and 5 focused unit tests, and shrinks the surface Layers 2/3 have to handle to genuinely-stuck plans.
