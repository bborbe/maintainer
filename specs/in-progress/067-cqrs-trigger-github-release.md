---
status: generating
tags:
    - dark-factory
    - spec
approved: "2026-06-09T10:07:19Z"
generating: "2026-06-09T09:06:20Z"
prompted: "2026-06-09T10:27:30Z"
branch: dark-factory/cqrs-trigger-github-release
---

## Summary

- The `/trigger` endpoint on the github-release watcher currently kicks off `w.Poll(ctx)` (full org scan across all in-scope repos) inline in a goroutine via `libhttp.NewBackgroundRunHandler` and returns immediately. A pod crash mid-cycle silently loses the trigger; there is no Kafka redelivery guarantee.
- Split the endpoint into a CQRS pair: HTTP handler publishes a new `TriggerReleaseCheckCommand` to Kafka and returns 202. A new in-pod Kafka consumer (third `run.Func`) runs `w.Poll(ctx)`.
- Kafka redelivery guarantees a trigger survives a pod crash between accept and completion.
- Wire-compatible: HTTP still returns 202 immediately (was already non-blocking via `BackgroundRunHandler`); the only externally visible change is the underlying mechanism (publish-a-command vs run-a-goroutine).
- Scope is strictly the github-release watcher. The symmetric github-pr change already shipped as bborbe/maintainer#51 / v0.37.0. The gateway flip out of `/admin`, the Force flag, and any sync-result UX are separate specs.

## Problem

A `POST /trigger` on the github-release watcher today schedules `w.Poll(ctx)` in a background goroutine inside the same pod and returns 202. If the pod is killed (rollout, OOM, node drain) before the goroutine finishes, the in-flight poll is silently lost — no `CreateTaskCommand` was emitted for the repos that had not yet been processed, the operator has no signal that the trigger was dropped, and there is no retry. The github-pr watcher just shipped CQRS for the symmetric endpoint (bborbe/maintainer#51 → v0.37.0); github-release is now the only `/trigger` in this stack that depends on goroutine survival rather than Kafka redelivery for durability. This inconsistency also blocks the planned next spec (move `/trigger` out of `/admin`) because that spec requires every `/trigger` in the stack to already be publish-only.

## Goal

A trigger request that the HTTP layer accepts (202) is guaranteed to either run `w.Poll(ctx)` end-to-end or be retried by Kafka redelivery, even if the pod crashes mid-execution. The github-release watcher pod runs three coordinated loops (poll interval, HTTP, command consumer) inside a single `run.CancelOnFirstFinish`. The HTTP handler is reduced to publish + 202, with no reference to the watcher object on the request path. The executor is a standard `cdb.CommandObjectExecutorTx` that invokes `w.Poll(ctx)` and follows the project's go-cqrs exit-path rules: malformed payload / validate-fail return `ErrCommandObjectSkipped`; poll-cycle transient errors return wrapped errors so the framework emits Failure on the result topic and Kafka redelivers.

## Non-goals

- Do NOT add a Force flag with behavior — that ships from the prerequisite task `Add Force Flag to Maintainer Watcher Trigger Endpoints`. This spec only plumbs a `Force bool` field through `TriggerReleaseCheckCommand` with no executor branch on it yet.
- Do NOT add per-repo filtering UX (`?repo=` query param, executor-side single-repo branch). The `TriggerReleaseCheckCommand` carries a `Scope string` field reserved for a future spec, but the executor MUST ignore it and always invoke the full `w.Poll(ctx)` cycle — matching today's wire behavior. An unused field is invariant; behavior change is a separate spec.
- Do NOT move `/trigger` out of `/admin` — that is the next spec in the series.
- Do NOT touch the github-pr watcher — it already shipped as v0.37.0.
- Do NOT change the schema (`GithubReleaserV1SchemaID` already shipped in `github.com/bborbe/maintainer/lib`), the topic name (`<branch>-maintainer-githubreleaser-v1-request`, auto-derived), or the existing `CreateTaskCommand` produced downstream by the watcher's publisher.
- Do NOT enable `SendResultEnabled` — fire-and-forget; HTTP returns 202 and nothing reads the result topic.
- Do NOT add per-request opt-out flags to disable the CQRS path — if `/trigger` exists, it goes through Kafka. An escape hatch on the Goal is itself a regression.
- Do NOT change the existing poll-interval loop or the existing `/resetcursor`, `/setcursor` admin endpoints.

## Desired Behavior

1. `POST /trigger` returns HTTP 202 immediately after publishing one `TriggerReleaseCheckCommand` to Kafka. The response body matches the github-pr watcher's spec-066 shape: `{"status":"accepted"}`.
2. The HTTP handler does not reference the watcher object on the request path. The handler's only Kafka interaction is publishing a `TriggerReleaseCheckCommand`. Verified by a test that injects a panicking watcher and confirms 202 is still returned.
3. A consumer in the same pod consumes `TriggerReleaseCheckCommand` messages from the request topic and invokes `w.Poll(ctx)` on the same watcher instance the poll-interval loop uses.
4. The executor maps exit paths: malformed payload (cannot unmarshal) and `cmd.Validate(ctx)` failure → `ErrCommandObjectSkipped` (non-retryable, deliberate). `w.Poll(ctx)` returning a non-nil error → wrapped error (transient, framework emits Failure, Kafka redelivers). `w.Poll(ctx)` returning nil → `nil, nil, nil` (success).
5. The pod's `Run` orchestrates three `run.Func`s under `run.CancelOnFirstFinish`: existing poll-interval loop, existing HTTP server, new command consumer. Cancelling the parent context shuts all three down cleanly without leaked goroutines.
6. `TriggerReleaseCheckCommand` carries `Scope string` and `Force bool` fields. The executor reads neither today — both are reserved for follow-on specs and plumbed through the wire format so the schema does not need to change later.
7. The HTTP-side sender (`TriggerReleaseCheckCommandSender`) is constructed once at factory wiring with `base.CommandCreator` + `cqrsiam.Initiator` + `cdb.CommandObjectSender` injected, reused across every `SendCommand` call. No per-call drift.

## Constraints

- Schema is frozen: `GithubReleaserV1SchemaID` from `github.com/bborbe/maintainer/lib` (already present at `lib/maintainer_cdb-schema.go`). No schema changes in this spec.
- Topic name is frozen: `<branch>-maintainer-githubreleaser-v1-request`, derived from the schema by the framework. The topic-controller (PR #158) provisions it; verify it has landed before consuming.
- Operation string is frozen: `"trigger-release-check"`.
- `SendResultEnabled` is `false` in this spec.
- The HTTP mount path stays `/trigger` (mounted under `/admin` at the gateway). No routing changes in `main.go` other than adding the third `run.Func` and replacing the handler line.
- The existing poll-interval loop behavior is untouched: same `pollInterval`, same `w.Poll(ctx)` body, same watcher object. The watcher object is now shared with the executor (single instance built once in `Run`).
- The Watcher interface is NOT extended. The executor invokes the existing `Watcher.Poll(ctx)` method as-is. The `Scope` field on the command is reserved-but-unread.
- Reference implementation: `watcher/github-pr/pkg/command/*` and `watcher/github-pr/pkg/factory/factory.go` as they exist on master at v0.37.0 (PR #51 just merged). All eight lessons-learned (CommandCreator/Initiator injection at construction; `skipped-not-nil-for-non-retryable` exit mapping; pure-composition factories with no nil-checks; `RunCommandConsumerTxDefault` auto-tx wrap; `Create*` factory naming vs `New*` constructors; counterfeiter directive above the type declaration; glog.V(2) success log after `SendCommandObject`; memdb in `pkg/` not `pkg/factory/`) are already correctly applied there. The github-release implementation MUST mirror those files structurally.
- Canonical pattern doc: `~/Documents/Obsidian/Personal/50 Knowledge Base/CQRS Command Producer Consumer Walkthrough.md`.
- Canonical rule docs: `~/Documents/workspaces/coding/docs/go-cqrs.md` (rules `skipped-not-nil-for-non-retryable`, `auto-tx-wrapper-no-manual-wrap`).
- Cqrs framework doc: `~/Documents/workspaces/cqrs/docs/producing-commands.md` (Factory Wiring section) and `~/Documents/workspaces/cqrs/docs/command-consumer.md`.
- Error wrapping uses `github.com/bborbe/errors` exclusively (never `fmt.Errorf`, never bare `return err`).
- Tests use Ginkgo v2 + Gomega + counterfeiter mocks, per repo convention.
- Build is verified per-module: `cd watcher/github-release && make precommit`.
- Factory functions use `Create*` naming; constructors in `pkg/` use `New*`.
- Counterfeiter directive sits ABOVE the type declaration, NOT inside the GoDoc block.
- The new `memdb.go` lives at `watcher/github-release/pkg/memdb.go` (NOT in `pkg/factory/`), mirroring `watcher/github-pr/pkg/memdb.go`.
- Factory is pure composition — no `if x == nil { panic }` guards; matches sibling factories.
- BSD copyright header on every new file, dated 2026.

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---------|-------------------|----------|-----------|---------------|-------------|
| Pod crash between HTTP 202 ack and command consumer pickup | Kafka holds the command at the committed offset; on restart the consumer resumes and the executor runs | Automatic — next consumer start | Lag on consumer group | Reversible | Single consumer per partition; redelivery is at-least-once |
| Pod crash mid-`w.Poll(ctx)` (after partial repo scan, before cursor save) | Kafka offset not committed; on restart the same `TriggerReleaseCheckCommand` is redelivered and the executor re-runs `w.Poll(ctx)` end-to-end | Automatic — Kafka redelivery | Consumer lag; duplicate downstream `CreateTaskCommand` deduped by github-releaser-agent's task-id derivation (already idempotent today; same property as pre-spec poll-interval cycles) | Reversible | At-least-once redelivery; downstream task creation is idempotent |
| `w.Poll(ctx)` returns wrapped error (rate-limited, GitHub 5xx, cursor read error) | Executor returns wrapped error; framework emits Failure on result topic (no one reads it); Kafka redelivers per consumer policy | Automatic — Kafka redelivery; transient errors clear when GitHub recovers | Executor error logged; consumer lag grows during outage; existing `poll_cycle` metric label (`rate_limited` / `github_error`) records the underlying cause | Reversible | Multiple in-flight retries possible if redelivery policy is aggressive — acceptable, downstream task creation is idempotent |
| `w.Poll(ctx)` returns nil but per-repo prunes occurred (today's "log and continue" path) | Executor returns `nil, nil, nil`; framework commits offset; per-repo prunes remain log-only (existing behavior, unchanged) | None — by design; operator monitors `poll_cycle` and per-repo glog.V(2) lines | `poll_cycle{result="success"}` increments; pruned-repo log lines available via grep | N/A | N/A |
| Malformed `TriggerReleaseCheckCommand` payload (unmarshal fails) | Executor wraps with `ErrCommandObjectSkipped`; framework commits offset (non-retryable) | None — operator must investigate the rogue producer | Executor log line names the unmarshal error; offset committed | Irreversible | Single message, single skip |
| `cmd.Validate(ctx)` fails on a payload that did unmarshal | Executor wraps with `ErrCommandObjectSkipped`; framework commits offset | None — operator must investigate the rogue producer | Executor log line names the validation error | Irreversible | Single message, single skip |
| Kafka request topic unavailable on HTTP publish | HTTP handler returns 5xx (sender error); caller retries | Operator/caller retry once Kafka is reachable | HTTP 5xx; sender error in logs | Reversible | No partial state — message either published or not |
| Pod starts before topic-controller has provisioned the request topic | Consumer fails to subscribe; HTTP publisher fails on first send | Topic-controller provisions topic (PR #158); pod retries on next reconnection | Consumer subscribe error log; HTTP 5xx on `/trigger` | Reversible | N/A |
| Two pod replicas (HA scale-out — **forward-looking, NOT in this spec**) | Both consume from the same consumer group; partition assignment guarantees one consumer per partition per command | Automatic — Kafka consumer-group semantics | Consumer-group rebalance logs | Reversible | Today single-instance only (deliberate, see Constraints); HA is a separate follow-on. Row kept to document Kafka semantics if/when replicas land. |
| Concurrent `w.Poll(ctx)` invocations (poll-interval ticks while executor is running) | Both invocations run; no in-process serialization is added by this spec (existing poll loop already has no serialization against external invocations) | None — by design | Possible duplicate transient API calls to GitHub; downstream task creation idempotent | Reversible | At-least-once semantics already exist in this stack; no new race introduced |

## Security / Abuse Cases

- Endpoint is mounted under `/admin/trigger` — gateway-level admin auth gates access. This spec does not change that.
- The HTTP request body and query string are NOT consumed (the command has no per-request fields with meaning yet — `Scope` is reserved-unread, `Force` is reserved-unread). The handler builds a `TriggerReleaseCheckCommand{}` from defaults and publishes it. Attacker-controllable input on `/trigger` is therefore an empty surface for this spec.
- The HTTP path no longer holds the request open while doing GitHub API work (it was already returning early via `BackgroundRunHandler`, but the goroutine was in-process and could be killed by a pod restart). Maximum HTTP-handler work per request is one Kafka send.
- The consumer is the only thing that invokes `w.Poll(ctx)` from a trigger. It runs on the pod's normal GitHub App credentials — unchanged from today.
- The `TriggerReleaseCheckCommand` is published on a stage-scoped topic (`<branch>-maintainer-githubreleaser-v1-request`). Cross-stage contamination is impossible by topic naming.
- No new data crosses a trust boundary.

## Acceptance Criteria

- [ ] `cd watcher/github-release && make precommit` exits 0 — evidence: exit code
- [ ] `POST /trigger` returns HTTP 202 with body containing `"status":"accepted"` — evidence: HTTP status + response body
- [ ] A test injecting a `pkg.Watcher` whose `Poll(ctx)` panics into the HTTP handler still observes HTTP 202 from `POST /trigger` (proves the watcher is not on the request path) — evidence: test passes, no panic propagates
- [ ] A test using a fake `cdb.CommandObjectSender` confirms the HTTP handler publishes exactly one command with schema ID `lib.GithubReleaserV1SchemaID` and operation `"trigger-release-check"` per `POST /trigger` — evidence: fake sender captures exactly one `CommandObject` with matching `SchemaID` and `Command.Operation`
- [ ] Table-driven test on the executor returns `ErrCommandObjectSkipped` (verified via `errors.Is(err, cdb.ErrCommandObjectSkipped)`) for each of: malformed payload (e.g. unmarshal error), `cmd.Validate(ctx)` failure — evidence: assertion result
- [ ] Executor returns a wrapped error (NOT `ErrCommandObjectSkipped`) when the injected `pkg.Watcher.Poll(ctx)` returns a non-nil error — evidence: assertion result with `errors.Is(err, cdb.ErrCommandObjectSkipped) == false` and the watcher error is in the wrap chain
- [ ] Executor returns `nil, nil, nil` when the injected `pkg.Watcher.Poll(ctx)` returns nil — evidence: assertion result
- [ ] Executor invokes `Watcher.Poll(ctx)` exactly once per valid command — evidence: counterfeiter fake's `PollCallCount()` equals 1
- [ ] `main.go` registers three `run.Func`s inside `run.CancelOnFirstFinish` (poll-interval loop, HTTP server, command consumer) — evidence: source grep for `factory.CreateCommandConsumer(` reference inside `Run`; `run.CancelOnFirstFinish` call site shows three arguments
- [ ] Cancelling the parent context from a startup integration test shuts down all three loops within the framework's standard grace period without leaked goroutines — evidence: test passes; `goleak` (or equivalent) reports zero leaks
- [ ] The `TriggerReleaseCheckCommand` struct has fields `Scope string` and `Force bool` (both reserved-unread by this spec) with a `Validate(ctx)` method that accepts the empty payload `{}` — evidence: unit test on `Validate` shows `nil` for empty payload
- [ ] `TriggerReleaseCheckCommandOperation` is the constant `"trigger-release-check"` — evidence: source grep
- [ ] The new consumer is wired via `cdb.RunCommandConsumerTxDefault(... GithubReleaserV1SchemaID ...)` (no manual transaction wrapping via `kv.NewTransactionMiddleware`) — evidence: source grep against `watcher/github-release/pkg/factory/`
- [ ] `factory.CreateTriggerReleaseCheckCommandSender(...)` builds `base.CommandCreator` + `cqrsiam.Initiator` ONCE at construction and passes them to `command.NewTriggerReleaseCheckCommandSender(...)` — evidence: source grep showing the constructor call inside the factory, with both values forwarded as constructor args (not built per-call inside `SendCommand`)
- [ ] Counterfeiter mock for `TriggerReleaseCheckCommandSender` is generated and committed — evidence: file present at `watcher/github-release/mocks/trigger_release_check_command_sender.go`
- [ ] The new `memdb.go` lives at `watcher/github-release/pkg/memdb.go` (NOT `pkg/factory/memdb.go`) — evidence: `ls watcher/github-release/pkg/memdb.go` succeeds, `ls watcher/github-release/pkg/factory/memdb.go` fails
- [ ] The counterfeiter directive sits on its own line ABOVE the type declaration in `trigger_release_check_command_sender.go`, NOT inside any GoDoc block — evidence: source grep `^//counterfeiter:generate` is directly followed (modulo blank line) by `type TriggerReleaseCheckCommandSender interface {`
- [ ] The sender logs `glog.V(2).Infof(...)` AFTER `commandObjectSender.SendCommandObject(...)` returns nil — evidence: source grep for the log line position
- [ ] Crash-recovery test (the spec's headline durability claim): wire the real consumer against an in-memory libkv DB; publish one `TriggerReleaseCheckCommand`; let the executor begin invoking the fake watcher's `Poll(ctx)`; before the executor's offset commit, cancel and restart the consumer goroutine; on restart, observe the fake watcher's `PollCallCount()` reaches 2 within 30s (proves Kafka redelivery → at-least-once execution) — evidence: `Eventually(func() int { return fakeWatcher.PollCallCount() }, 30*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2))` passes
- [ ] Coverage on new code in `watcher/github-release/pkg/command/` and `watcher/github-release/pkg/factory/` is ≥ 80% — evidence: `cd watcher/github-release && go test -coverprofile=coverage.out ./pkg/command/... ./pkg/factory/... && go tool cover -func=coverage.out | awk '/total:/ {print $3}'` ≥ `80.0%`
- [ ] Schema reference uses `lib.GithubReleaserV1SchemaID` directly (no string literal duplication of `"maintainer-githubreleaser-v1"`) — evidence: source grep finds zero literal `"maintainer-githubreleaser-v1"` in `watcher/github-release/` outside test assertions on the schema struct itself

## Verification

```
cd ~/Documents/workspaces/maintainer-cqrs-trigger-release/watcher/github-release && make precommit
```

Manual smoke (deployed pod, dev stage):

```
curl -sS -o /dev/null -w '%{http_code}\n' -X POST \
  "https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-release/trigger"
# Expect: 202
# Then watch consumer logs:
kubectldev -n maintainer logs deploy/maintainer-watcher-github-release -f | grep -E 'trigger-release-check|poll cycle'
# Expect to see the executor pick up the command and the watcher log "poll cycle start ...".
```

Crash-recovery smoke:

```
# 1. Send a trigger (curl above).
# 2. Immediately kill the pod: kubectlquant -n <stage> delete pod <pod>
# 3. Wait for pod to come back.
# 4. Verify the executor re-ran w.Poll(ctx) after restart (poll cycle start log line dated after pod restart timestamp).
```

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Define `TriggerReleaseCheckCommand` (struct + `Validate` + operation constant) and `TriggerReleaseCheckCommandSender` with counterfeiter mock; unit tests for `Validate` (empty payload accepted), operation constant, and sender (publishes one `CommandObject` with correct schema ID via fake `cdb.CommandObjectSender`). Add `watcher/github-release/pkg/memdb.go` as a verbatim port of `watcher/github-pr/pkg/memdb.go` with its tests. | 6, 7 | 5 (Validate row), 11, 12, 15, 17, 18 (in part: log line position on sender), 21 | — |
| 2 | Build the executor (`trigger_release_check_executor.go`) that invokes the injected `pkg.Watcher.Poll(ctx)`. Apply exit-path mapping (skipped vs wrapped vs nil). Table-driven tests cover malformed payload, validate-fail, watcher returns nil, watcher returns error. Counterfeiter fake for `pkg.Watcher` (extend existing mock if present, generate if not). | 3, 4 | 5 (executor rows), 6, 7, 8 | 1 |
| 3 | Shrink the HTTP `/trigger` handler to publish-via-sender + 202 + JSON body `{"status":"accepted"}`. Update or add handler tests including the panicking-watcher test (AC 3). Confirm the handler has no `pkg.Watcher` dependency at all. | 1, 2 | 2, 3, 4 | 1 |
| 4 | Add `factory.CreateCommandConsumer` and `factory.CreateTriggerReleaseCheckCommandSender` in `watcher/github-release/pkg/factory/`. Wire the third `run.Func` in `main.go`: build the watcher once, share it between the existing poll-interval loop and the new command consumer. Integration test for clean shutdown (goleak). Crash-recovery integration test (AC 19). | 5 | 1, 9, 10, 13, 14, 16, 18 (factory + main wiring rows), 19, 20 | 2, 3 |

Rationale: command + sender + memdb (1) are leaves with no internal cycle — ship first so executor (2) and handler (3) can import them in parallel. Consumer wiring + main (4) comes last because it depends on the executor and the new factory-injected sender both existing. Each prompt verifies with `cd watcher/github-release && make precommit`. No cycles. Matches the spec-066 decomposition shape so the prompt-creator can reuse the same reasoning.

## Do-Nothing Option

Leaving `/trigger` as a `BackgroundRunHandler` means: (a) pod crashes mid-cycle continue to silently lose the in-flight poll, with no operator-visible signal beyond the missing downstream tasks the next operator notices days later; (b) `/trigger` cannot be moved out of `/admin` because two watchers' triggers behave differently (github-pr is CQRS, github-release is goroutine) and the gateway-flip spec needs uniform semantics across all `/trigger` endpoints; (c) the inconsistency between the two watchers accumulates as the stack adds more triggered services. None of these are fatal today — trigger volume is low and pod crashes are rare — but they directly block the next spec in this series and contradict the just-shipped CQRS pattern on github-pr. Acceptable only if the spec-3 gateway flip is abandoned.
