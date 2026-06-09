---
status: prompted
tags:
    - dark-factory
    - spec
approved: "2026-06-08T21:03:17Z"
generating: "2026-06-08T21:03:18Z"
prompted: "2026-06-08T21:34:29Z"
branch: dark-factory/cqrs-trigger-github-pr
---

## Summary

- The `/trigger` endpoint on the github-pr watcher currently does all its work synchronously on the HTTP request thread (GitHub API call, filter, trust check, publish CreateTaskCommand). A pod crash mid-trigger silently loses the trigger.
- Split the endpoint into a CQRS pair: HTTP handler validates the PR URL and publishes a new `TriggerPRReviewCommand` to Kafka, returning 202 immediately. A new in-pod Kafka consumer (third `run.Func`) executes the GitHub fetch, filter, trust, and downstream publish.
- Kafka redelivery guarantees that a trigger survives a pod crash between fetch and publish.
- HTTP wire compatibility shifts from `200 + task_id JSON` to `202 + {status,url}`. Filter-skip and trust-reject become silent in the HTTP response (visible in metrics and logs). Schema, topic name, and the `/admin/trigger` mount path are unchanged.
- Scope is strictly the github-pr watcher. The symmetric github-release change, the gateway flip out of `/admin`, and any sync-result UX restoration are separate specs.

## Problem

A `POST /trigger?url=...` on the github-pr watcher today is a long synchronous HTTP request that touches the GitHub API, runs filter and trust logic, and publishes a `CreateTaskCommand` to Kafka — all inside one HTTP handler. If the pod is killed (rollout, OOM, node drain) between the GitHub fetch and the Kafka publish, the trigger is silently lost: the caller saw 5xx or a connection drop, no `CreateTaskCommand` was emitted, and there is no retry. The current design also blocks moving `/trigger` out of `/admin` because the handler has opinions (filter, trust) that operators outside `/admin` should not be able to influence directly. Finally, every other producer/consumer in this stack is CQRS — github-pr is the odd one out, which makes the operational model inconsistent and the failure surface harder to reason about.

## Goal

A trigger request that the HTTP layer accepts (202) is guaranteed to be either executed or retried by Kafka redelivery, even if the pod crashes mid-execution. The github-pr watcher pod runs three coordinated loops (poll, HTTP, command consumer) inside a single `run.CancelOnFirstFinish`. The HTTP handler is reduced to URL parse + validate + publish + 202, with no GitHub API client, no filter, and no trust dependency on the request path. The executor is a standard `cdb.CommandObjectExecutorTx` that follows the project's go-cqrs exit-path rules: deliberate skips (invalid URL, filter-rejected, untrusted author) return `ErrCommandObjectSkipped`; transient failures (GitHub 5xx, Kafka send error, trust infrastructure error) return wrapped errors so the framework emits Failure on the result topic and Kafka redelivers.

## Non-goals

- Do NOT add a Force flag with behavior — that ships from the prerequisite task `Add Force Flag to Maintainer Watcher Trigger Endpoints`. This spec only plumbs a `Force bool` field through `TriggerPRReviewCommand` with no executor branch on it yet.
- Do NOT move `/trigger` out of `/admin` — that is spec 3.
- Do NOT touch the github-release watcher — that is spec 2 (symmetric copy).
- Do NOT restore sync-result UX by reading back from a result topic — that is an optional spec 4.
- Do NOT change the schema (`GithubPRReviewV1SchemaID` already shipped in `github.com/bborbe/maintainer/lib@v0.36.0`), the topic name (`<branch>-maintainer-githubprreview-v1-request`, auto-derived), or the existing `CreateTaskCommand` produced downstream.
- Do NOT enable `SendResultEnabled` — fire-and-forget; HTTP returns 202 and nothing reads the result topic.
- Do NOT add per-request opt-out flags to disable the CQRS path — if `/trigger` exists, it goes through Kafka. An escape hatch on the Goal is itself a regression.

## Desired Behavior

1. `POST /trigger?url=<pr_url>` returns HTTP 202 with body `{"status":"accepted","url":"<pr_url>"}` whenever URL validation succeeds, regardless of whether the PR will ultimately be filter-skipped, untrusted, or 404 in GitHub.
2. `POST /trigger?url=<bad_url>` returns HTTP 400 with the validation error (empty URL, unparseable, non-github platform). No Kafka message is published.
3. The HTTP handler does not call the GitHub API, the filter, or the trust decision on the request path. The handler's only Kafka interaction is publishing a `TriggerPRReviewCommand`.
4. A consumer in the same pod consumes `TriggerPRReviewCommand` messages from the request topic and executes the work the old handler used to do: fetch PR details, apply filter, check trust, build and publish `CreateTaskCommand`.
5. The executor maps exit paths exactly as: invalid URL / filter-rejected / untrusted-author → `ErrCommandObjectSkipped` (non-retryable, deliberate). GitHub 5xx / network error / trust infrastructure error / downstream Kafka publish error → wrapped error (transient, framework emits Failure, Kafka redelivers). Successful publish → `nil, nil, nil`.
6. The executor does NOT use the `return nil, nil, nil` "idempotent skip" anti-pattern from `agent/task/controller/pkg/command/task_create_task_executor.go:61`. Filter-skip and trust-reject MUST be `ErrCommandObjectSkipped`.
7. The `github_pr_published` metric is emitted from the executor (not the HTTP handler) with the same label set as today: `create`, `skipped`, `kafka_error`, `trust_error`.
8. The pod's `Run` orchestrates three `run.Func`s under `run.CancelOnFirstFinish`: existing poll loop, existing HTTP server, new command consumer. Cancelling the parent context shuts all three down cleanly.
9. `TriggerPRReviewCommand` carries `URL string` and `Force bool`. The executor reads `URL` for behavior; `Force` is plumbed but unused (reserved for the Force-flag task).

## Constraints

- Schema is frozen: `GithubPRReviewV1SchemaID` from `github.com/bborbe/maintainer/lib@v0.36.0`. No schema changes in this spec.
- Topic name is frozen: `<branch>-maintainer-githubprreview-v1-request`, derived from the schema by the framework. The topic-controller (PR #158) provisions it.
- Operation string is frozen: `"trigger-pr-review"`.
- `SendResultEnabled` is `false` in this spec.
- The HTTP mount path stays `/admin/trigger`. No changes to routing in `main.go` other than adding the third `run.Func`.
- The downstream `CreateTaskCommand` payload built inside the executor MUST be byte-identical to what `singlePRTriggerHandler.ServeHTTP` produces today (same `pkg.BuildCreateCommand` call with same args).
- Existing poll-loop watcher behavior is untouched: same filter set, same trust decision, same `createSender`, same metrics — they are now shared between the poll watcher and the executor, not duplicated.
- Error wrapping uses `github.com/bborbe/errors` exclusively (never `fmt.Errorf`, never bare `return err`).
- Tests use Ginkgo v2 + Gomega + counterfeiter mocks, per repo convention.
- Build is verified per-module: `cd watcher/github-pr && make precommit`.
- Follow go-cqrs guide rules `skipped-not-nil-for-non-retryable` and `auto-tx-wrapper-no-manual-wrap` from `~/Documents/workspaces/coding/docs/go-cqrs.md`.
- Architecture reference: `docs/architecture.md` (Watcher component, pipeline).
- Pattern references: `~/Documents/workspaces/agent/lib/command/task/create-command.go` (sender shape), `~/Documents/workspaces/agent/task/controller/pkg/command/task_create_task_executor.go` (executor shape — DO NOT copy its skip semantics), `~/Documents/workspaces/agent/task/controller/pkg/factory/factory.go` (factory wiring), `~/Documents/workspaces/cqrs/docs/command-consumer.md` (consumer wiring).

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---------|-------------------|----------|-----------|---------------|-------------|
| Pod crash between HTTP 202 ack and command consumer pickup | Kafka holds the command at the committed offset; on restart the consumer resumes and the executor runs | Automatic — next consumer start | Lag on consumer group | Reversible | Single consumer per partition; redelivery is at-least-once |
| Pod crash mid-executor (after GitHub fetch, before `CreateTaskCommand` publish) | Kafka offset not committed; on restart the same `TriggerPRReviewCommand` is redelivered and the executor re-runs end-to-end | Automatic — Kafka redelivery | Consumer lag; duplicate downstream `CreateTaskCommand` deduped by `task_id` derivation (already idempotent: `DeriveTaskID(owner, repo, number, head_sha)`) | Reversible | At-least-once redelivery; downstream task creation is idempotent via derived task_id |
| GitHub API returns 5xx or network error | Executor returns wrapped error; framework emits Failure on result topic (no one reads it); Kafka redelivers per consumer policy | Automatic — Kafka redelivery; transient errors clear when GitHub recovers | `github_pr_published{result="kafka_error"}` does NOT increment (this is GH error not kafka); executor error logged; consumer lag grows during outage | Reversible | Multiple in-flight retries possible if redelivery policy is aggressive — acceptable, downstream is idempotent |
| GitHub returns 404 for the PR URL | Executor returns `ErrCommandObjectSkipped` (deliberate: URL parsed OK but PR does not exist) | None — operator must re-submit with correct URL | `github_pr_published{result="skipped"}` increments; executor log line names the 404 sub-case | Irreversible without operator re-submit | Single message, single skip |
| Filter rejects PR (draft, bot author, WIP title, out-of-scope repo) | Executor returns `ErrCommandObjectSkipped` | None — by design | `github_pr_published{result="skipped"}` increments | Irreversible without filter config change | Single message, single skip |
| Trust check returns false (untrusted author) | Executor returns `ErrCommandObjectSkipped` | None — by design | `github_pr_published{result="skipped"}` increments; operator must update `TRUSTED_AUTHORS` and re-submit | Irreversible without config change | Single message, single skip |
| Trust check infrastructure error (e.g. allowlist lookup fails) | Executor returns wrapped error; Kafka redelivers | Automatic | `github_pr_published{result="trust_error"}` increments | Reversible | Redelivery is safe |
| Downstream `CreateTaskCommand` Kafka publish fails | Executor returns wrapped error; Kafka redelivers the trigger | Automatic | `github_pr_published{result="kafka_error"}` increments | Reversible | Redelivery is safe (downstream task_id is derived → idempotent) |
| Invalid URL submitted via HTTP | HTTP handler returns 400; no Kafka message | None needed — request never entered the queue | HTTP 400 in access log | N/A | N/A |
| Invalid URL somehow reaches the executor (e.g. enqueued by a buggy client) | Executor's `Validate(ctx)` returns error wrapped with `ErrCommandObjectSkipped` | None | Executor log line; offset committed | Irreversible | Single message, single skip |
| Kafka request topic unavailable on HTTP publish | HTTP handler returns 5xx (sender error); caller retries | Operator/caller retry once Kafka is reachable | HTTP 5xx; sender error in logs | Reversible | No partial state — message either published or not |
| Pod starts before topic-controller has provisioned the request topic | Consumer fails to subscribe; HTTP publisher fails on first send | Topic-controller provisions topic (PR #158); pod retries on next reconnection | Consumer subscribe error log; HTTP 5xx on `/trigger` | Reversible | N/A |
| Two pod replicas (HA scale-out) | Both consume from the same consumer group; partition assignment guarantees one consumer per partition per command | Automatic — Kafka consumer-group semantics | Consumer-group rebalance logs | Reversible | At-least-once across replicas is fine; downstream idempotent |

## Security / Abuse Cases

- Endpoint is mounted under `/admin/trigger` — gateway-level admin auth gates access. This spec does not change that.
- The attacker-controllable input is the `url` query parameter. The HTTP handler validates it with `prurl.ParsePRURL` and the platform-must-be-github check before publishing. Malformed URLs cannot create Kafka messages.
- The HTTP path no longer holds open a TCP connection while doing GitHub API work, so an attacker cannot stall the HTTP server by hammering `/trigger` with URLs that point at slow-to-respond GitHub repos. Maximum HTTP-handler work per request is one Kafka send.
- The consumer is the only thing that talks to GitHub on behalf of triggers. It runs on the pod's normal GitHub App credentials — unchanged from today.
- The `TriggerPRReviewCommand` is published on a stage-scoped topic (`<branch>-...`). Cross-stage contamination is impossible by topic naming.
- No new data crosses a trust boundary. The trust decision still applies inside the executor before `CreateTaskCommand` is published downstream.

## Acceptance Criteria

- [ ] `cd watcher/github-pr && make precommit` exits 0 — evidence: exit code
- [ ] `POST /trigger?url=<valid_github_pr_url>` returns HTTP 202 with body containing `"status":"accepted"` — evidence: HTTP status + response body
- [ ] `POST /trigger?url=` (empty) returns HTTP 400 — evidence: HTTP status
- [ ] `POST /trigger?url=<non-github-url>` returns HTTP 400 — evidence: HTTP status
- [ ] A test injecting a panicking `pkg.GitHubClient` into the HTTP handler still observes HTTP 202 from `POST /trigger?url=<valid>` (proves the GitHub client is not on the request path) — evidence: test passes, no panic propagates
- [ ] A test using a fake `CommandObjectSender` confirms the executor emits one `CreateTaskCommand` to the agent task topic when fed a valid PR URL — evidence: fake sender captures exactly one command with the expected schema ID and payload
- [ ] Table-driven test on the executor returns `ErrCommandObjectSkipped` (verified via `errors.Is(err, cdb.ErrCommandObjectSkipped)`) for each of: invalid URL, filter-rejected PR, untrusted author — evidence: assertion result
- [ ] Table-driven test on the executor returns a wrapped error (NOT `ErrCommandObjectSkipped`) for each of: GitHub 5xx, downstream Kafka send error, trust infrastructure error — evidence: assertion result
- [ ] `main.go` registers three `run.Func`s inside `run.CancelOnFirstFinish` (poll, HTTP, command consumer) — evidence: source grep for `CreateCommandConsumer` reference inside `Run`; `run.CancelOnFirstFinish` call has three arguments
- [ ] Integration test publishes one `TriggerPRReviewCommand` to the request topic against the wired-up consumer and observes the executor's `github_pr_published` counter delta ≥ 1 within 30s (proves the third `run.Func` is not a no-op stub) — evidence: test passes, metric delta asserted
- [ ] Cancelling the parent context from a startup integration test shuts down all three loops within the framework's standard grace period without leaked goroutines — evidence: test passes; `goleak` (or equivalent) reports zero leaks
- [ ] The Prometheus metric `github_pr_published` is incremented from inside the executor (not the HTTP handler) with labels `create`, `skipped`, `kafka_error`, `trust_error` for the corresponding exit paths — evidence: unit test asserts metric increment via the test registry
- [ ] The `TriggerPRReviewCommand` struct has fields `URL string` and `Force bool` with a `Validate(ctx)` method that rejects empty URL and non-github PR URLs — evidence: unit test on `Validate`
- [ ] `TriggerPRReviewCommandOperation` is the constant `"trigger-pr-review"` — evidence: source grep
- [ ] The new consumer is wired via `cdb.RunCommandConsumerTxDefault(... GithubPRReviewV1SchemaID ...)` (no manual transaction wrapping) — evidence: source grep
- [ ] Counterfeiter mock for `TriggerPRReviewCommandSender` is generated and committed — evidence: file present under `watcher/github-pr/mocks/`
- [ ] Crash-recovery test (the spec's headline durability claim): publish one `TriggerPRReviewCommand`, kill the executor goroutine before it commits the offset, restart the consumer, observe exactly one downstream `CreateTaskCommand` emitted via fake `CommandObjectSender` (proves Kafka redelivery → exactly-once-effective-via-idempotent-downstream) — evidence: test passes, fake sender captures exactly one downstream command

## Verification

```
cd ~/Documents/workspaces/maintainer-cqrs-trigger/watcher/github-pr && make precommit
```

Manual smoke (deployed pod, dev stage):

```
curl -sS -o /dev/body -w '%{http_code}\n' -X POST \
  "https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-pr/trigger?url=https://github.com/bborbe/maintainer/pull/1"
# Expect: 202
# Then watch consumer logs:
kubectldev -n maintainer logs deploy/maintainer-watcher-github-pr -f | grep -E 'trigger-pr-review|github_pr_published'
# Expect to see executor pick up the command and emit either create, skipped, kafka_error, or trust_error.
```

Crash-recovery smoke:

```
# 1. Send a trigger.
# 2. Immediately kill the pod (kubectl delete pod ...).
# 3. Wait for pod to come back.
# 4. Verify the downstream CreateTaskCommand was published (consumer-side log or downstream task file).
```

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Define the command + sender + counterfeiter mock + unit tests for `Validate` and operation constant | 9 | 11, 12, 14 | — |
| 2 | Build the executor by lifting `singlePRTriggerHandler.ServeHTTP` body; apply the go-cqrs exit-path mapping; table-driven tests; metric ownership; crash-recovery test | 4, 5, 6, 7 | 6, 7, 8, 10, 16 | 1 |
| 3 | Shrink the HTTP handler to parse + validate + publish + 202; update handler tests including the panicking-GitHub-client test | 1, 2, 3 | 2, 3, 4, 5 | 1 |
| 4 | Add `factory.CreateCommandConsumer` and wire it as the third `run.Func` in `main.go`; integration test for clean shutdown; integration test asserting end-to-end command flow through wired consumer | 8 | 9, 13, 15 | 2 |

Rationale: command/sender (1) is a leaf with no dependencies — ship first so both the executor (2) and the handler (3) can import it independently and proceed in parallel. The consumer wiring (4) is last because it depends on the executor existing. No cycles. Each prompt is independently testable with `make precommit` in `watcher/github-pr/`.

## Do-Nothing Option

Leaving `/trigger` synchronous means: (a) pod crashes mid-trigger continue to silently lose work, with no operator-visible signal beyond a 5xx in the caller's logs; (b) `/trigger` cannot be moved out of `/admin` because the handler does heavy, opinionated work that admin-only access currently guards; (c) the github-pr watcher remains the only non-CQRS producer in the stack, forcing operators to maintain two mental models for "how triggers propagate." None of these are fatal today — trigger volume is low and pod crashes are rare — but they block the next two specs in this series (gateway move, symmetric github-release) and accumulate as the stack adds more triggered services. Acceptable only if the planned follow-ups are abandoned.
