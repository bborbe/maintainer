---
status: verifying
tags:
    - dark-factory
    - spec
approved: "2026-06-09T15:59:59Z"
generating: "2026-06-09T15:59:59Z"
prompted: "2026-06-09T16:18:27Z"
verifying: "2026-06-09T21:37:54Z"
branch: dark-factory/cqrs-trigger-github-build
---

## Summary

- The github-build watcher's `/trigger` endpoint currently signals an in-process buffered `chan struct{}` (size 1) that a separate goroutine (`runPollLoop`) drains alongside a `time.Ticker`. Multiple `/trigger` calls while a poll is running coalesce into one pending signal. A pod crash mid-poll silently loses the in-flight cycle; redundant triggers are silently dropped.
- Convert this third (and final remaining) maintainer-watcher `/trigger` to the same CQRS pattern already shipped for `watcher/github-pr` (v0.37.0) and `watcher/github-release` (v0.38.0): HTTP handler publishes a `TriggerBuildCheckCommand` to Kafka and returns 202; a new in-pod Kafka consumer (third `run.Func`) drives `Watcher.Poll(ctx)`.
- Drop the in-process buffered `chan struct{}` and the channel-coupled `runPollLoop`. The new executor IS the single-flight gate (`cdb.RunCommandConsumerTxDefault` BatchSize(1) + single-partition request topic). The poll-interval loop continues unchanged, calling `Watcher.Poll(ctx)` directly on each tick.
- Bundle the schema-constant addition into this spec: add `GithubBuildV1SchemaID{Group:"maintainer", Kind:"githubbuild", Version:"v1"}` to `lib/maintainer_cdb-schema.go` and append to `CDBSchemaIDs`. The watcher's `go.mod` uses local `replace ../../lib`, so the constant is immediately available in this worktree.
- Wire-compatible: HTTP still returns 202 immediately. The response body changes from a plain `"trigger fired"` text payload to `{"status":"accepted"}` to match the canonical CQRS shape used by the other two watchers. Operators observe one `Watcher.Poll(ctx)` per consumed command — exact-once-per-trigger semantics instead of today's coalesced behavior.

## Problem

A `POST /trigger` on the github-build watcher today does a non-blocking send into a buffered `chan struct{}` (size 1) and immediately responds. A separate `runPollLoop` goroutine selects on `ticker.C | trigger`. Two consequences:

1. If the pod is killed (rollout, OOM, node drain) after the channel accept but before / during `Watcher.Poll(ctx)`, the in-flight poll is silently lost — no `CreateTaskCommand`s emitted for the repos that had not yet been processed, no operator-visible signal, no retry.
2. Multiple triggers fired in quick succession coalesce into one pending signal (channel size 1). Operators expecting "I just fired three triggers" to mean "three polls will run" instead get one. This is not documented in any user-visible runbook.

`watcher/github-pr` (v0.37.0) and `watcher/github-release` (v0.38.0) have both already migrated their `/trigger` to the CQRS pattern; `watcher/github-build` is now the only `/trigger` in the maintainer stack still relying on goroutine survival rather than Kafka redelivery for durability. This inconsistency blocks the planned spec-4 move of `/trigger` out of `/admin`, which requires uniform publish-only semantics across all watchers.

## Goal

A trigger request that the HTTP layer accepts (202) is guaranteed to either run `Watcher.Poll(ctx)` end-to-end or be retried via Kafka redelivery, even if the pod crashes mid-execution. Each accepted command results in exactly one `Watcher.Poll(ctx)` invocation (no coalescing). The github-build watcher pod runs three coordinated loops (poll interval, HTTP, command consumer) inside a single `run.CancelOnFirstFinish`. The HTTP handler is reduced to publish + 202, with no reference to the watcher object on the request path. The executor is a standard `cdb.CommandObjectExecutorTx` that invokes `Watcher.Poll(ctx)` and follows the project's go-cqrs exit-path rules: malformed payload / validate-fail return `ErrCommandObjectSkipped`; poll-cycle transient errors return wrapped errors so the framework emits Failure on the result topic and Kafka redelivers.

## Non-goals

- Do NOT add Force flag behavior — that ships from the prerequisite task `Add Force Flag to Maintainer Watcher Trigger Endpoints`. This spec only plumbs a `Force bool` field through `TriggerBuildCheckCommand` with no executor branch on it yet. Do NOT add an opt-out flag to keep the legacy channel-based path — if `/trigger` exists, it goes through Kafka. An escape hatch on the Goal is itself a regression.
- Do NOT add per-repo filtering UX (`?repo=` query param, executor-side single-repo branch). The `TriggerBuildCheckCommand` carries a `Scope string` field reserved for a future spec, but the executor MUST ignore it and always invoke the full `Watcher.Poll(ctx)` cycle — matching today's wire behavior. An unused field is invariant; behavior change is a separate spec.
- Do NOT keep the in-process `chan struct{}` trigger plumbing or the channel-coupled `runPollLoop` signature. The executor is already the single-flight gate; the channel becomes a redundant in-process queue ahead of a Kafka-backed queue already in line. Per YAGNI / dev-guide, the channel comes out.
- Do NOT move `/trigger` out of `/admin` — that is the next spec in the series.
- Do NOT touch the github-pr or github-release watchers — they already shipped as v0.37.0 / v0.38.0.
- Do NOT change the existing poll-interval loop's externally visible behavior (same `POLL_INTERVAL` env, same `Watcher.Poll(ctx)` body, same watcher object). The watcher object is now shared between the poll-interval loop and the executor (single instance built once in `Run`).
- Do NOT change the existing `/resetcursor`, `/healthz`, `/readiness`, `/metrics`, `/setloglevel` admin endpoints.
- Do NOT enable `SendResultEnabled` on the consumer — fire-and-forget; HTTP returns 202 and nothing reads the result topic.
- Do NOT register the new `GithubBuildV1SchemaID` in `trading/strimzi/topic-controller/topics.go` from this spec — that is a sibling PR in a different repo, delegated via the task. This spec only adds the constant to `maintainer/lib`.
- Do NOT change `triggerBufferSize` to anything other than removing it; the constant is deleted with the channel.
- Do NOT introduce HA / replica-count > 1 in this spec. Single-instance only, matching the other two watchers.

## Desired Behavior

1. `POST /trigger` returns HTTP 202 immediately after publishing one `TriggerBuildCheckCommand` to Kafka. The response body matches the canonical CQRS shape: `{"status":"accepted"}`.
2. The HTTP handler does not reference the watcher object on the request path. Its only Kafka interaction is publishing one `TriggerBuildCheckCommand`. Verified by a test that injects a watcher whose `Poll(ctx)` panics and confirms 202 is still returned without invoking the watcher.
3. A consumer in the same pod consumes `TriggerBuildCheckCommand` messages from the request topic `<branch>-maintainer-githubbuild-v1-request` and invokes `Watcher.Poll(ctx)` on the same watcher instance the poll-interval loop uses.
4. The executor maps exit paths per `docs/coding/go-cqrs.md`: malformed payload (cannot unmarshal) and `cmd.Validate(ctx)` failure → `ErrCommandObjectSkipped` (non-retryable, deliberate). `Watcher.Poll(ctx)` returning a non-nil error → wrapped error (transient, framework emits Failure, Kafka redelivers). `Watcher.Poll(ctx)` returning nil → `nil, nil, nil` (success).
5. The pod's `Run` orchestrates three `run.Func`s under `run.CancelOnFirstFinish`: the (reshaped) poll-interval loop with no `trigger` channel parameter, the existing HTTP server, the new command consumer. Cancelling the parent context shuts all three down cleanly without leaked goroutines.
6. `TriggerBuildCheckCommand` carries `Scope string` and `Force bool` fields, both with `omitempty` JSON tags. The executor reads neither today — both are reserved for follow-on specs and plumbed through the wire format so the schema does not need to change later.
7. The HTTP-side sender (`TriggerBuildCheckCommandSender`) is constructed once at factory wiring with `base.CommandCreator` + `cqrsiam.Initiator` + `cdb.CommandObjectSender` injected, reused across every `SendCommand` call. No per-call drift.
8. `lib/maintainer_cdb-schema.go` exposes a new exported `GithubBuildV1SchemaID = cdb.SchemaID{Group:"maintainer", Kind:"githubbuild", Version:"v1"}` and the existing `CDBSchemaIDs` slice appends it after `GithubReleaserV1SchemaID`.

## Constraints

- Schema: NEW constant `GithubBuildV1SchemaID{Group:"maintainer", Kind:"githubbuild", Version:"v1"}` added to `lib/maintainer_cdb-schema.go` and appended to `CDBSchemaIDs`. The constant name, group, kind, and version are all frozen as written here so the topic-controller side PR can target the matching identifier.
- Topic name is frozen: `<branch>-maintainer-githubbuild-v1-request`, derived from the schema by the framework. The topic-controller (separate sibling PR in `trading/strimzi/topic-controller`) provisions it; this spec assumes the topic exists at deploy time. If the topic-controller PR has not landed when the watcher is deployed, the consumer's subscribe call will fail and the operator sees a Kafka error — expected behavior, not a code defect.
- Operation string is frozen: `"trigger-build-check"`.
- `SendResultEnabled` is `false` in this spec (matches both sibling watchers).
- The HTTP mount path stays `/trigger` (mounted under `/admin` at the gateway). No routing changes in `main.go` other than dropping the `trigger` channel argument, dropping the channel allocation, replacing the handler-creation line, and adding the third `run.Func` for the command consumer.
- The existing poll-interval ticker behavior is preserved: same `POLL_INTERVAL`, same `Watcher.Poll(ctx)` body. After this spec, the loop's `select` has only `ctx.Done()` and `ticker.C` cases — the `<-trigger` case is removed.
- The `Watcher` interface is NOT extended. The executor invokes the existing `Watcher.Poll(ctx)` method as-is. The `Scope` field on the command is reserved-but-unread.
- The existing `pkg.NewTriggerHandler(chan<- struct{})` function in `watcher/github-build/pkg/trigger_handler.go` is REMOVED; the new handler lives at `watcher/github-build/pkg/handler/trigger_handler.go` matching the spec-2 file layout.
- Reference implementations (post-spec-2, master @ 570c14d): `watcher/github-release/pkg/command/*`, `watcher/github-release/pkg/factory/factory.go`, `watcher/github-release/pkg/handler/trigger_handler.go`, `watcher/github-release/pkg/memdb.go`, `watcher/github-release/main.go`. Cross-reference `watcher/github-pr/pkg/command/` if anything is ambiguous. All ten lessons-learned (see Lessons section below) are already correctly applied there. The github-build implementation MUST mirror those files structurally.
- Canonical pattern doc: `~/Documents/Obsidian/Personal/50 Knowledge Base/CQRS Command Producer Consumer Walkthrough.md`.
- Canonical rule docs: `~/Documents/workspaces/coding/docs/go-cqrs.md` (rules `skipped-not-nil-for-non-retryable`, `auto-tx-wrapper-no-manual-wrap`).
- Cqrs framework doc: `~/Documents/workspaces/cqrs/docs/producing-commands.md` (Factory Wiring section), `~/Documents/workspaces/cqrs/docs/command-consumer.md`.
- Error wrapping uses `github.com/bborbe/errors` exclusively (never `fmt.Errorf`, never bare `return err`).
- Tests use Ginkgo v2 + Gomega + counterfeiter mocks, per repo convention.
- Build is verified per-module: `cd watcher/github-build && make precommit`. Library changes additionally verified at `cd lib && make precommit`.
- Factory functions use `Create*` naming; constructors in `pkg/` use `New*`.
- Counterfeiter directive sits ABOVE the type declaration on its own line, NOT inside any GoDoc block.
- The new `memdb.go` lives at `watcher/github-build/pkg/memdb.go` (NOT in `pkg/factory/`), mirroring `watcher/github-release/pkg/memdb.go`.
- Factory is pure composition — no `if x == nil { panic }` guards; matches sibling factories.
- BSD copyright header on every new file, dated 2026.

### Lessons learned (apply at write time)

1. Sender takes `(base.CommandCreator, cqrsiam.Initiator, cdb.CommandObjectSender)` at construction. Build `CommandCreator` with `base.RequestIDChannel(ctx)` ONCE at factory wiring time. Do NOT copy the per-call drift from `agent/lib/command/task/`.
2. Factory `CreateCommandConsumer` is PURE COMPOSITION — no `if x == nil { panic }`. Sibling factories don't nil-check; NPE-on-misuse is the codebase style.
3. Exit paths: `errors.Wrapf(ctx, cdb.ErrCommandObjectSkipped, ...)` for non-retryable; bare wrapped err for transient. NEVER `return nil, nil, nil` as a skip signal.
4. Use `cdb.RunCommandConsumerTxDefault` (auto-wraps tx). NEVER manual `RunCommandConsumerTx` + `kv.NewTransactionMiddleware`.
5. Factory function naming: `Create*` for factories, `New*` for constructors in `pkg/`.
6. Counterfeiter directive ABOVE the type declaration, NOT inside the GoDoc block.
7. `glog.V(2)` success log AFTER `SendCommandObject` returns nil in the sender.
8. memdb in `pkg/`, NOT `pkg/factory/`. `sync.RWMutex`; `Stats` / `StatsDetailed` lock+copy.
9. Handler's 502 BadGateway response (on sender error) has an inline comment explaining 502 vs 500/503 (Kafka publish = upstream cause).
10. `Validate(ctx)` for reserved-unread payload returns `nil` directly (not empty `validation.All{}.Validate(ctx)`).

## Failure Modes

| Trigger | Expected behavior | Recovery | Detection | Reversibility | Concurrency |
|---------|-------------------|----------|-----------|---------------|-------------|
| Pod crash between HTTP 202 ack and command consumer pickup | Kafka holds the command at the committed offset; on restart the consumer resumes and the executor runs | Automatic — next consumer start | Lag on consumer group | Reversible | Single consumer per partition; redelivery is at-least-once |
| Pod crash mid-`Watcher.Poll(ctx)` (after partial repo scan, before cursor save) | Kafka offset not committed; on restart the same `TriggerBuildCheckCommand` is redelivered and the executor re-runs `Watcher.Poll(ctx)` end-to-end | Automatic — Kafka redelivery | Consumer lag; duplicate downstream `CreateTaskCommand` deduped by build-fixer-agent's task-id derivation (already idempotent today; same property as pre-spec poll-interval cycles) | Reversible | At-least-once redelivery; downstream task creation is idempotent |
| `Watcher.Poll(ctx)` returns wrapped error (rate-limited, GitHub 5xx, cursor read error) | Executor returns wrapped error; framework emits Failure on result topic (no one reads it); Kafka redelivers per consumer policy | Automatic — Kafka redelivery; transient errors clear when GitHub recovers | Executor error logged; consumer lag grows during outage | Reversible | Multiple in-flight retries possible if redelivery policy is aggressive — acceptable, downstream task creation is idempotent |
| Malformed `TriggerBuildCheckCommand` payload (unmarshal fails) | Executor wraps with `ErrCommandObjectSkipped`; framework commits offset (non-retryable) | None — operator must investigate the rogue producer | Executor log line names the unmarshal error; offset committed | Irreversible | Single message, single skip |
| `cmd.Validate(ctx)` fails on a payload that did unmarshal | Executor wraps with `ErrCommandObjectSkipped`; framework commits offset | None — operator must investigate the rogue producer | Executor log line names the validation error | Irreversible | Single message, single skip |
| Kafka request topic unavailable on HTTP publish | HTTP handler returns 502 (sender error); caller retries | Operator/caller retry once Kafka is reachable | HTTP 502; sender error in logs | Reversible | No partial state — message either published or not |
| Pod starts before topic-controller has provisioned the request topic | Consumer fails to subscribe; HTTP publisher fails on first send | Topic-controller provisions topic (sibling PR registers `GithubBuildV1SchemaID`); pod retries on next reconnection | Consumer subscribe error log; HTTP 502 on `/trigger` | Reversible | N/A |
| Schema-constant collision (operator hand-types `"maintainer-githubbuild-v1"` somewhere instead of using `lib.GithubBuildV1SchemaID`) | Source grep AC fails precommit; CI blocks merge | None — fix at PR time | Source grep AC | Irreversible at runtime if shipped | N/A |
| Burst of N `/trigger` calls in a short window | N commands published to Kafka; N `Watcher.Poll(ctx)` invocations run sequentially (BatchSize(1)) — NOT coalesced into one | None — by design; operators get one poll per trigger (exact-once-per-trigger) | Consumer lag temporarily grows; each invocation logs `trigger-build-check` operation start | Reversible | At-least-once redelivery; serialized via BatchSize(1) — never two `Watcher.Poll(ctx)` runs from the consumer overlap |
| Poll-interval tick fires while the executor is mid-`Watcher.Poll(ctx)` | Both invocations run concurrently; no in-process serialization between the poll-interval loop and the executor (matches today's behavior, where the poll-interval loop did not coordinate with `/trigger`-driven runs either) | None — by design | Possible duplicate transient API calls to GitHub; downstream task creation idempotent | Reversible | No new race introduced vs pre-spec |
| Schema-constant added to `lib/` but topic-controller side PR not yet merged | Pod boot fails on consumer subscribe; `/trigger` publishes fail | Roll back watcher deploy OR land topic-controller PR; on next pod start the topic exists and consumer subscribes | Pod CrashLoopBackOff; consumer subscribe error log line | Reversible | N/A |

## Security / Abuse Cases

- Endpoint is mounted under `/admin/trigger` — gateway-level admin auth gates access. This spec does not change that.
- The HTTP request body and query string are NOT consumed (the command has no per-request fields with meaning yet — `Scope` is reserved-unread, `Force` is reserved-unread). The handler builds a `TriggerBuildCheckCommand{}` from defaults and publishes it. Attacker-controllable input on `/trigger` is therefore an empty surface for this spec.
- The HTTP path no longer holds the request open or blocks on a buffered channel send. Maximum HTTP-handler work per request is one Kafka send.
- The consumer is the only thing that invokes `Watcher.Poll(ctx)` from a trigger. It runs on the pod's normal GitHub App credentials — unchanged from today.
- The `TriggerBuildCheckCommand` is published on a stage-scoped topic (`<branch>-maintainer-githubbuild-v1-request`). Cross-stage contamination is impossible by topic naming.
- A burst of `/trigger` calls now translates to N Kafka publishes (vs today's coalesce-to-1). Trigger volume is gated by `/admin` auth and is operationally low; no new DoS surface beyond what the existing CQRS watchers already accept.
- No new data crosses a trust boundary.

## Acceptance Criteria

- [ ] `cd watcher/github-build && make precommit` exits 0 — evidence: exit code
- [ ] `cd lib && make precommit` exits 0 after adding `GithubBuildV1SchemaID` and appending to `CDBSchemaIDs` — evidence: exit code
- [ ] `lib/maintainer_cdb-schema.go` exports `GithubBuildV1SchemaID = cdb.SchemaID{Group:"maintainer", Kind:"githubbuild", Version:"v1"}` and the `CDBSchemaIDs` slice contains it — evidence: `grep -n 'GithubBuildV1SchemaID' lib/maintainer_cdb-schema.go` returns ≥ 2 lines (definition + slice append); a unit test in `lib` asserts `CDBSchemaIDs` contains an entry with `Kind == "githubbuild"`
- [ ] `POST /trigger` returns HTTP 202 with body containing `"status":"accepted"` — evidence: HTTP status + response body in handler unit test
- [ ] A test injecting a `pkg.Watcher` whose `Poll(ctx)` panics into the HTTP handler still observes HTTP 202 from `POST /trigger` (proves the watcher is not on the request path) — evidence: test passes; handler test does not even hold a reference to a `pkg.Watcher`
- [ ] A test using a fake `cdb.CommandObjectSender` confirms the HTTP handler publishes exactly one command with schema ID `lib.GithubBuildV1SchemaID` and operation `"trigger-build-check"` per `POST /trigger` — evidence: fake sender captures exactly one `CommandObject` with matching `SchemaID` and `Command.Operation`
- [ ] HTTP handler returns HTTP 502 (Bad Gateway) when the injected sender returns an error, with an inline `// 502 because Kafka publish is an upstream cause ...` comment — evidence: handler test asserts 502; source grep for the comment
- [ ] Table-driven test on the executor returns `ErrCommandObjectSkipped` (verified via `errors.Is(err, cdb.ErrCommandObjectSkipped)`) for each of: malformed payload (e.g. unmarshal error), `cmd.Validate(ctx)` failure — evidence: assertion result
- [ ] Executor returns a wrapped error (NOT `ErrCommandObjectSkipped`) when the injected `pkg.Watcher.Poll(ctx)` returns a non-nil error — evidence: assertion result with `errors.Is(err, cdb.ErrCommandObjectSkipped) == false` and the watcher error is in the wrap chain
- [ ] Executor returns `nil, nil, nil` when the injected `pkg.Watcher.Poll(ctx)` returns nil — evidence: assertion result
- [ ] Executor invokes `Watcher.Poll(ctx)` exactly once per valid command — evidence: counterfeiter fake's `PollCallCount()` equals 1
- [ ] `main.go` registers three `run.Func`s inside `run.CancelOnFirstFinish` (poll-interval loop, HTTP server, command consumer) — evidence: source grep for `factory.CreateCommandConsumer(` reference inside `Run` AND for `run.CancelOnFirstFinish(`; goleak-driven clean-shutdown integration test (AC 15) is the runtime gate that all three loops are wired and stop cleanly. Per spec 067 audit lesson: a strict AST arity check is brittle if a future refactor builds the run.Func slice via a helper; grep + goleak combo is robust without pinning syntactic form.
- [ ] `main.go` no longer declares `triggerBufferSize`, no longer allocates `make(chan struct{}, ...)`, and the `runPollLoop` function signature has no `trigger` parameter — evidence: `grep -n 'triggerBufferSize\|make(chan struct{}' watcher/github-build/main.go` returns zero lines; `runPollLoop` (or its replacement) takes only `(poll run.Func, interval time.Duration)`
- [ ] `watcher/github-build/pkg/trigger_handler.go` is removed; the new handler lives at `watcher/github-build/pkg/handler/trigger_handler.go` — evidence: `ls watcher/github-build/pkg/trigger_handler.go` fails; `ls watcher/github-build/pkg/handler/trigger_handler.go` succeeds
- [ ] Cancelling the parent context from a startup integration test shuts down all three loops within the framework's standard grace period without leaked goroutines — evidence: test passes; `goleak` (or equivalent) reports zero leaks
- [ ] The `TriggerBuildCheckCommand` struct has fields `Scope string` and `Force bool` (both with `omitempty` JSON tags, both reserved-unread by this spec) with a `Validate(ctx)` method that accepts the empty payload `{}` — evidence: unit test on `Validate` shows `nil` for empty payload; JSON marshalling of `TriggerBuildCheckCommand{}` produces `{}`
- [ ] `TriggerBuildCheckCommandOperation` is the constant `"trigger-build-check"` — evidence: source grep
- [ ] The new consumer is wired via `cdb.RunCommandConsumerTxDefault(... GithubBuildV1SchemaID ...)` (no manual transaction wrapping via `kv.NewTransactionMiddleware`) — evidence: source grep against `watcher/github-build/pkg/factory/` finds the `RunCommandConsumerTxDefault` call and zero `kv.NewTransactionMiddleware` references
- [ ] `factory.CreateTriggerBuildCheckCommandSender(...)` builds `base.CommandCreator` + `cqrsiam.Initiator` ONCE at construction and passes them to `command.NewTriggerBuildCheckCommandSender(...)` — evidence: source grep showing the constructor call inside the factory, with both values forwarded as constructor args (not built per-call inside `SendCommand`)
- [ ] Counterfeiter mock for `TriggerBuildCheckCommandSender` is generated and committed — evidence: file present at `watcher/github-build/mocks/trigger_build_check_command_sender.go`
- [ ] The new `memdb.go` lives at `watcher/github-build/pkg/memdb.go` (NOT `pkg/factory/memdb.go`) — evidence: `ls watcher/github-build/pkg/memdb.go` succeeds, `ls watcher/github-build/pkg/factory/memdb.go` fails
- [ ] `memdb` race-detector witness test: spawn N goroutines writing + reading + calling `Stats()` concurrently; `go test -race` passes — evidence: test exists in `memdb_test.go` and the test name contains `Race` or `Concurrent`; `cd watcher/github-build && go test -race ./pkg/...` exits 0
- [ ] The counterfeiter directive sits on its own line ABOVE the type declaration in `trigger_build_check_command_sender.go`, NOT inside any GoDoc block — evidence: source grep `^//counterfeiter:generate` is directly followed (modulo blank line) by `type TriggerBuildCheckCommandSender interface {`
- [ ] The sender logs `glog.V(2).Infof(...)` AFTER `commandObjectSender.SendCommandObject(...)` returns nil — evidence: source grep for the log line position inside `trigger_build_check_command_sender.go`
- [ ] `Validate(ctx)` for `TriggerBuildCheckCommand` returns `nil` directly (no empty `validation.All{}.Validate(ctx)` wrapper) — evidence: source grep on the `Validate` function body shows literal `return nil` and no `validation.All`
- [ ] Crash-recovery test at the EXECUTOR layer (single source of truth, NOT duplicated in factory integration_test): wire the real consumer against an in-memory libkv DB; publish one `TriggerBuildCheckCommand`; let the executor begin invoking the fake watcher's `Poll(ctx)`; before the executor's offset commit, cancel and restart the consumer goroutine; on restart, observe the fake watcher's `PollCallCount()` reaches ≥ 2 within 30s (proves Kafka redelivery → at-least-once execution) — evidence: `Eventually(func() int { return fakeWatcher.PollCallCount() }, 30*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2))` passes
- [ ] AST control-flow assertion on `factory.CreateCommandConsumer` body: test resolves the source path via `runtime.Caller(0)` (no hardcoded path); parses the function body; asserts (a) zero `if … == nil { panic(…) }` statements, (b) at least one call to `cdb.RunCommandConsumerTxDefault`, (c) the `GithubBuildV1SchemaID` identifier appears in the call's argument list — evidence: test passes; AST visit emits the three assertions
- [ ] Coverage on new code in `watcher/github-build/pkg/command/`, `watcher/github-build/pkg/handler/`, and `watcher/github-build/pkg/factory/` is ≥ 80% — evidence: `cd watcher/github-build && go test -coverprofile=coverage.out ./pkg/command/... ./pkg/handler/... ./pkg/factory/... && go tool cover -func=coverage.out | awk '/total:/ {print $3}'` ≥ `80.0%`
- [ ] Schema reference uses `lib.GithubBuildV1SchemaID` directly — evidence: source grep finds zero literal `"maintainer-githubbuild-v1"` and zero literal `"githubbuild"` in `watcher/github-build/` outside test assertions on the schema struct itself

## Verification

```
cd ~/Documents/workspaces/maintainer-cqrs-trigger-build/lib && make precommit
cd ~/Documents/workspaces/maintainer-cqrs-trigger-build/watcher/github-build && make precommit
cd ~/Documents/workspaces/maintainer-cqrs-trigger-build/watcher/github-build && go test -race ./pkg/...
```

Manual smoke (deployed pod, dev stage — assumes topic-controller PR has already registered `GithubBuildV1SchemaID` and been redeployed):

```
curl -sS -o /dev/null -w '%{http_code}\n' -X POST \
  "https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-build/trigger"
# Expect: 202

kubectldev -n maintainer logs deploy/maintainer-watcher-github-build -f | grep -E 'trigger-build-check|poll cycle'
# Expect: executor pick up; "poll cycle start ..." log line.
```

Crash-recovery smoke:

```
# 1. Send a trigger (curl above).
# 2. Immediately kill the pod: kubectldev -n maintainer delete pod <pod>
# 3. Wait for pod to come back.
# 4. Verify the executor re-ran Watcher.Poll(ctx) after restart (poll cycle start log line dated after pod restart timestamp).
```

Burst-vs-coalesce smoke (proves exact-once-per-trigger):

```
# Fire 3 triggers in rapid succession:
for i in 1 2 3; do curl -sS -o /dev/null -X POST \
  "https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-build/trigger"; done

# Watch logs:
kubectldev -n maintainer logs deploy/maintainer-watcher-github-build -f | grep -c 'poll cycle start'
# Expect: 3 (not 1 — coalescing is gone)
```

## Suggested Decomposition

| # | Prompt focus | Covers DBs | Covers ACs | Depends on |
|---|---|---|---|---|
| 1 | Add `GithubBuildV1SchemaID` + `CDBSchemaIDs` append in `lib/maintainer_cdb-schema.go`; unit test asserts the new entry is present in the slice. Verify `cd lib && make precommit`. | 8 | 2, 3 | — |
| 2 | Define `TriggerBuildCheckCommand` (struct + `Validate` + operation constant) and `TriggerBuildCheckCommandSender` with counterfeiter mock; unit tests for `Validate` (empty payload accepted, JSON marshal of empty is `{}`), operation constant, and sender (publishes one `CommandObject` with correct schema ID via fake `cdb.CommandObjectSender`; logs `glog.V(2)` after success). Add `watcher/github-build/pkg/memdb.go` as verbatim port of `watcher/github-release/pkg/memdb.go` + race-detector witness test. | 6, 7 | 6 (sender publish), 16, 17, 20, 21, 22, 23, 24, 25 | 1 |
| 3 | Build the executor (`pkg/command/trigger_build_check_executor.go`) that invokes the injected `pkg.Watcher.Poll(ctx)`. Apply exit-path mapping (skipped vs wrapped vs nil). Table-driven tests cover malformed payload, validate-fail, watcher returns nil, watcher returns error. Counterfeiter fake for `pkg.Watcher` (extend existing mock if present, generate if not). Includes the crash-recovery test at the executor layer (AC 26). | 3, 4 | 8, 9, 10, 11, 26 | 2 |
| 4 | Shrink the HTTP `/trigger` handler to publish-via-sender + 202 + JSON body `{"status":"accepted"}`. Move handler to `pkg/handler/trigger_handler.go`; delete `pkg/trigger_handler.go`. Handler tests including the panicking-watcher test (AC 5), the sender-error-→-502 test with inline comment (AC 7), and the schema-ID/operation assertion (AC 6). | 1, 2 | 4, 5, 6, 7, 14 | 2 |
| 5 | Add `factory.CreateCommandConsumer` and `factory.CreateTriggerBuildCheckCommandSender` in `watcher/github-build/pkg/factory/`. Update `main.go`: drop `triggerBufferSize` constant, drop the `trigger` channel allocation, drop the `trigger` parameter from the poll loop signature, wire the third `run.Func` for the command consumer, share watcher instance. Integration test for clean shutdown (goleak). AST control-flow assertion on `CreateCommandConsumer` body. | 5 | 1, 12, 13, 15, 18, 19, 27, 28, 29 | 3, 4 |

Rationale: schema-lib change (1) is the leaf — every later prompt imports it. Command + sender + memdb (2) are the next layer with no internal cycle. Executor (3) and handler (4) can ship in parallel after 2 — they don't depend on each other. Factory wiring + `main.go` (5) lands last because it depends on the executor, the new sender factory, and the new handler all existing. Each prompt verifies with `cd watcher/github-build && make precommit`. No cycles. Matches the spec-067 decomposition shape so the prompt-creator can reuse the same reasoning.

## Do-Nothing Option

Leaving the github-build `/trigger` on the in-process `chan struct{}` + `runPollLoop` means: (a) pod crashes mid-cycle continue to silently lose the in-flight poll, with no operator-visible signal beyond the missing downstream tasks the next operator notices days later; (b) burst-of-N triggers continue to coalesce into a single poll, contradicting the operator mental model "one trigger = one poll" already established by the other two watchers; (c) `/trigger` cannot be moved out of `/admin` because two-out-of-three watchers are CQRS and the third is not, and the gateway-flip spec needs uniform semantics; (d) the inconsistency between the three watchers permanently solidifies into "two patterns to learn" instead of "one pattern, three watchers" — every future contributor pays the cognitive tax. None of these are fatal today, but together they directly block spec-4 (gateway flip) and contradict the just-shipped CQRS pattern on the two sibling watchers. Acceptable only if the spec-4 gateway flip is abandoned and operators are willing to permanently document the github-build coalescing quirk.
