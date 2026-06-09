---
status: completed
spec: [066-cqrs-trigger-github-pr]
summary: Wired the third run.Func (CreateCommandConsumer) into run.CancelOnFirstFinish, added CreateTriggerPRReviewCommandSender + NewMemDB factories, deleted the legacy handler adapter, added clean-shutdown + end-to-end integration tests, and updated CHANGELOG.md. make precommit passes with exit code 0.
container: maintainer-cqrs-trigger-exec-251-spec-066-wire-command-consumer
dark-factory-version: v0.175.0
created: "2026-06-08T21:11:59Z"
queued: "2026-06-08T21:49:51Z"
started: "2026-06-09T05:49:01Z"
completed: "2026-06-09T06:13:36Z"
branch: dark-factory/cqrs-trigger-github-pr
---

<summary>
- `pkg/factory/factory.go` gains a new `CreateCommandConsumer(syncProducer, branch, ghClient, createSender, taskCreationFilter, trustDecision, stage, maxSlugLen, maxTitleLen, taskSuffix, metrics, db, saramaClientProvider) run.Func` that wires `cdb.RunCommandConsumerTxDefault(... lib.GithubPRReviewV1SchemaID ...)` with the prompt-2 executor.
- `main.go` builds a `cdb.CommandObjectSender` from the existing `syncProducer` and constructs both `factory.NewTriggerPRReviewCommandSender(sender)` (HTTP-side) and `factory.CreateCommandConsumer(...)` (consumer-side). The `run.CancelOnFirstFinish(ctx, ...)` call grows from two `run.Func`s to three.
- Two integration tests cover the consumer wiring: (a) a clean-shutdown test that cancels the parent context and asserts `goleak.VerifyTestMain` reports zero goroutine leaks across the three `run.Func`s; (b) an end-to-end test that publishes one `TriggerPRReviewCommand` to the request topic against the wired-up consumer and observes the `github_pr_published` counter delta ≥ 1 within 30s.
- The CHANGELOG entry for the full spec lands in this prompt (`feat: split /trigger into CQRS pair — HTTP handler publishes TriggerPRReviewCommand; in-pod consumer runs the GitHub fetch + filter + trust + downstream CreateTaskCommand publish`).
- A `factory.CreateCommandConsumer` test (in `pkg/factory/command_consumer_test.go`) asserts: zero-business-logic rule (factory has no `for`/`switch`/conditional, only composition), the returned `run.Func` is non-nil, and a nil-dependency causes a panic with the right message.

This is prompt 4 of 4 for spec 066. It depends on prompts 1, 2, and 3 (the command type, the executor, and the shrunk HTTP handler). It owns the final `make precommit` gate, the CHANGELOG entry, and the integration tests.
</summary>

<objective>
Wire the new `TriggerPRReviewCommand` consumer as the third `run.Func` inside the existing `run.CancelOnFirstFinish`, complete the main.go + factory wiring that prompt 3 deferred, add the integration tests that prove the three-loop orchestration works, and ship the CHANGELOG entry. After this prompt, the spec is end-to-end complete: a `POST /trigger?url=<valid>` returns 202, the consumer picks up the command, runs the GitHub fetch + filter + trust + publish, and `github_pr_published` increments exactly once per outcome.

The load-bearing invariants from this prompt:
1. `run.CancelOnFirstFinish(ctx, ...)` has exactly three arguments (poll, HTTP, command consumer). The spec § AC 9.
2. `factory.CreateCommandConsumer` is pure composition (no loops, no conditionals). The factory-pattern guide is non-negotiable.
3. The consumer uses `cdb.RunCommandConsumerTxDefault` (NOT `RunCommandConsumerTx` with manual tx wrapping). The `auto-tx-wrapper-no-manual-wrap` rule in `go-cqrs.md` is non-negotiable.
4. The clean-shutdown integration test proves the three loops stop within the framework's grace period with no leaked goroutines.
5. The end-to-end integration test proves the third `run.Func` is not a no-op stub.
</objective>

<context>
Read `/workspace/CLAUDE.md` and `/workspace/watcher/github-pr/CLAUDE.md` (if present) for project conventions.

Read these source files in full BEFORE editing:

- `/workspace/watcher/github-pr/main.go` — the file being completed. The edits in this prompt are the natural follow-through of prompt 3's placeholder. Key sections:
  - Lines 250-265: `branch := base.Branch(a.Stage)` and the `syncProducer` construction + `defer Close()`. The `syncProducer` is REUSED — both the HTTP-side sender and the consumer-side sender back onto the same `syncProducer` (sharing a single Kafka connection is more efficient than spinning up two).
  - Line 265: `createSender := factory.CreateKafkaSender(syncProducer, branch)` — the existing downstream `CreateCommandSender`. The consumer executor (prompt 2) takes this as its `createSender` arg. Keep it as-is.
  - Line 289-300: the `triggerHandler := factory.CreateSinglePRTriggerHandler(...)` + `a.TriggerHandler = libhttp.NewJSONErrorHandler(...)` block. Replace the placeholder with the real call, passing the new `triggerPRReviewSender` (built in this prompt).
  - Line 307-310: `return run.CancelOnFirstFinish(ctx, a.runPollLoop(...), a.createHTTPServer(...))` — grows to three args.
- `/workspace/watcher/github-pr/pkg/factory/factory.go` — the file gaining `CreateCommandConsumer`. Read the entire file: the existing `CreateGitHubAppClient`, `CreateKafkaSender`, and `CreateWatcher` functions establish the factory's zero-logic, single-responsibility style that the new function must match. The `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)` call at line 45 is the pattern the new sender construction follows.
- `/workspace/watcher/github-pr/pkg/factory/factory_suite_test.go` and `single_pr_test.go` — the Ginkgo suite. The new test file `command_consumer_test.go` registers in the same suite.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor.go` (prompt 2 output) — `command.NewTriggerPRReviewCommandExecutor(ghClient, createSender, taskCreationFilter, trustDecision, stage, maxSlugLen, maxTitleLen, taskSuffix, metrics) cdb.CommandObjectExecutorTx`. The factory wraps this in a `cdb.CommandObjectExecutorTxs{}` slice (length 1) and passes it to `cdb.RunCommandConsumerTxDefault`.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_command_sender.go` (prompt 1 output) — `command.NewTriggerPRReviewCommandSender(commandObjectSender) command.TriggerPRReviewCommandSender`. The factory ALSO exports a `CreateTriggerPRReviewCommandSender(syncProducer, branch)` convenience constructor that wraps `cdb.NewCommandObjectSender` + `command.NewTriggerPRReviewCommandSender`. This is the sender `main.go` passes to the HTTP-side `CreateSinglePRTriggerHandler`.
- `/workspace/prompts/3-spec-066-shrink-trigger-handler.md` — the prompt 3 output. Verify the placeholder in `main.go` is in place: `grep -n 'triggerPRReviewSender\|placeholder; prompt 4' /workspace/watcher/github-pr/main.go`. If the placeholder is absent (prompt 3 didn't run), STOP and report `status: failed` with message "prompt 3 of spec 066 has not shipped".
- `/workspace/prompts/2-spec-066-trigger-pr-review-executor.md` — the prompt 2 output. Verify the executor is in place: `ls /workspace/watcher/github-pr/pkg/command/`. The factory's `CreateCommandConsumer` references `command.NewTriggerPRReviewCommandExecutor` by name.
- `/workspace/prompts/1-spec-066-trigger-pr-review-command.md` — the prompt 1 output. Verify the sender type and operation constant are exported.

Reference the integration test patterns in the codebase:

- `/workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go` (post-prompt-3 rewrite) — the Ginkgo + counterfeiter pattern for the HTTP-side tests. The new integration tests use a different shape (full-factory, real-ish run loop, no `httptest`).
- `/home/node/go/pkg/mod/github.com/bborbe/kafka@v1.23.2/kafka_consumer-offset_internal_test.go` lines 240-241 — a `testSaramaClientProvider` fake. The integration test can use a similar pattern.
- `/home/node/go/pkg/mod/github.com/bborbe/agent@v0.65.0/prompts/completed/083-spec-017-controller-create-task-executor.md` test pattern — counterfeiter-based executor tests. The new clean-shutdown test does NOT use counterfeiter (it spins up a real `run.CancelOnFirstFinish`); the end-to-end test does NOT use counterfeiter (it observes real Kafka traffic).

Reference the boilerplate from the agent's spec-017 consumer wiring prompt (it owns the parallel pattern):

- `/home/node/go/pkg/mod/github.com/bborbe/agent@v0.65.0/prompts/completed/083-spec-017-controller-create-task-executor.md` requirement #3 — `executors := cdb.CommandObjectExecutorTxs{ ... }` and `cdb.RunCommandConsumerTxDefault(saramaClientProvider, syncProducer, db, schemaID, branch, false, executors)`. The new github-pr consumer wiring is structurally identical: the executor is the prompt-2 `command.NewTriggerPRReviewCommandExecutor(...)`; the schema is `lib.GithubPRReviewV1SchemaID`; `ignoreUnsupported` is `false`.

Reference the factory-pattern style from the coding plugin docs:

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — `Create*` prefix, zero logic, no conditionals, no `context.Background()`. The new `CreateCommandConsumer` MUST follow this.

Coding plugin docs (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — load-bearing. Re-read the `RULE go-cqrs/auto-tx-wrapper-no-manual-wrap` block. The new factory uses `cdb.RunCommandConsumerTxDefault` (NOT a manual `RunCommandConsumerTx` with tx-wrapped executor).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` — `run.CancelOnFirstFinish` over `go func()`; the three `run.Func` arguments each must be ctx-cancellable.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-test-types-guide.md` — the clean-shutdown test is an INTEGRATION test (it spins up a real run loop); the end-to-end test is also INTEGRATION (it touches real Kafka semantics, even if the broker is mocked via the offset consumer pattern).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — read before writing `CreateCommandConsumer`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — the `## Unreleased` entry format, the `feat:` prefix.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — final precommit gate.
</context>

<requirements>

1. **Add `CreateTriggerPRReviewCommandSender` to `pkg/factory/factory.go`.** New function signature:

   ```go
   // CreateTriggerPRReviewCommandSender constructs a typed trigger-PR-review
   // command sender backed by a Kafka sync producer. This is the HTTP-side
   // sender: the /trigger handler publishes TriggerPRReviewCommand messages
   // through it.
   func CreateTriggerPRReviewCommandSender(
       syncProducer libkafka.SyncProducer,
       branch base.Branch,
   ) command.TriggerPRReviewCommandSender {
       sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
       return command.NewTriggerPRReviewCommandSender(sender)
   }
   ```

   Add the import `github.com/bborbe/maintainer/watcher/github-pr/pkg/command` to the import block (already covered by the existing factory imports — verify by re-reading the file; if the `command` import is missing, add it).

2. **Add `CreateCommandConsumer` to `pkg/factory/factory.go`.** New function signature:

   ```go
   // CreateCommandConsumer wires a run.Func that consumes TriggerPRReviewCommand
   // messages from the github-pr watcher's request topic and runs them through
   // the single-PR review pipeline (GitHub fetch → filter → trust → publish).
   //
   // The function is pure composition: no business logic, no conditionals.
   // It uses cdb.RunCommandConsumerTxDefault (auto-wraps the transaction) per
   // the go-cqrs/auto-tx-wrapper-no-manual-wrap rule — do NOT manually wrap
   // the executor with kv.NewTransactionMiddleware.
   func CreateCommandConsumer(
       saramaClientProvider libkafka.SaramaClientProvider,
       syncProducer libkafka.SyncProducer,
       db libkv.DB,
       ghClient pkg.GitHubClient,
       createSender task.CreateCommandSender,
       taskCreationFilter filter.TaskCreationFilter,
       trustDecision trust.Trust,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       metrics pkg.Metrics,
       branch base.Branch,
   ) run.Func {
       executors := cdb.CommandObjectExecutorTxs{
           command.NewTriggerPRReviewCommandExecutor(
               ghClient,
               createSender,
               taskCreationFilter,
               trustDecision,
               stage,
               maxSlugLen,
               maxTitleLen,
               taskSuffix,
               metrics,
           ),
       }
       return cdb.RunCommandConsumerTxDefault(
           saramaClientProvider,
           syncProducer,
           db,
           lib.GithubPRReviewV1SchemaID,
           branch,
           false, // ignoreUnsupported
           executors,
       )
   }
   ```

   Add the new imports to the file's import block:
   - `"github.com/bborbe/run"` (for `run.Func` return type)
   - `libkv "github.com/bborbe/kv"` (for `libkv.DB` parameter)
   - `"github.com/bborbe/maintainer/lib"` (for `lib.GithubPRReviewV1SchemaID`)

   **Zero-logic factory — no nil-checks.** The existing sibling factories (`CreateGitHubAppClient`, `CreateKafkaSender`, `CreateWatcher`) do NOT panic-on-nil — they compose and return. Caller passes nil → NPE at first use, which is the codebase's actual style. Do NOT add `if x == nil { panic(...) }` guards; they violate the zero-logic invariant the test enforces.

3. **Rewire `main.go` — swap the legacy call site, delete the adapter, share deps.** Three changes:

   **(a) Extract shared `ghClient` and `metrics` BEFORE the `factory.CreateWatcher` call** so both the watcher and the new consumer use the same instances (no double GitHub-rate-limit bucket, no double Prometheus registration):

   ```go
   // Shared instances — both the poll-loop watcher and the command consumer use them.
   ghClient := pkg.NewGitHubClient(httpClient)
   metrics := pkg.NewMetrics()
   ```

   Then update the existing `factory.CreateWatcher` call site to pass `ghClient` and `metrics` instead of constructing them inline (refactor `CreateWatcher`'s signature if it currently constructs them internally — the precondition is "share, don't duplicate").

   **(b) Replace the legacy `factory.CreateSinglePRTriggerHandler(...)` call** (lines 289-300, 9 args) with the new single-arg form:

   ```go
   // HTTP-side sender backs the /trigger handler.
   triggerPRReviewSender := factory.CreateTriggerPRReviewCommandSender(syncProducer, branch)
   triggerHandler := factory.NewSinglePRTriggerHandler(triggerPRReviewSender)
   a.TriggerHandler = libhttp.NewJSONErrorHandler(triggerHandler)

   // In-pod command consumer: third run.Func alongside poll + HTTP.
   saramaClientProvider := libkafka.NewSaramaClientProviderNew(a.KafkaBrokers)
   db := libkv.NewMemDB() // session-scoped offset store; replay-from-oldest on restart is safe (downstream task_id is idempotent)
   commandConsumer := factory.CreateCommandConsumer(
       saramaClientProvider,
       syncProducer,
       db,
       ghClient,          // shared with the watcher
       createSender,
       taskCreationFilter,
       trustDecision,
       a.Stage,
       a.MaxSlugLen,
       a.MaxTitleLen,
       a.TaskSuffix,
       metrics,           // shared with the watcher
       branch,
   )
   ```

   **(c) Delete the legacy `CreateSinglePRTriggerHandler` adapter from `pkg/factory/single_pr.go` and its corresponding test entry from `single_pr_test.go`.** The adapter was only retained for prompt 3's mid-rollout greenness; this prompt finishes the swap. Verify with: `grep -n 'CreateSinglePRTriggerHandler' /workspace/watcher/github-pr/pkg/factory/single_pr.go` → must show zero matches after this prompt.

   The `db := libkv.NewMemDB()` is the session-scoped offset store — it lives in-process and is rebuilt on pod restart, so the consumer starts from `OffsetOldest` (framework default in `RunCommandConsumerTxDefault`) and replays the request topic from the beginning. Acceptable for a fire-and-forget consumer where redelivery is the durability mechanism, NOT the offset store. (Alternative — boltdb-backed offsets — adds a PVC requirement that is out of scope for this spec.)

   Add the imports to `main.go`:
   - `libkv "github.com/bborbe/kv"`
   - `libkafka "github.com/bborbe/kafka"` (already present)
   - `lib "github.com/bborbe/maintainer/lib"` (only if not already imported — `main.go` already imports `repoallowlist "github.com/bborbe/maintainer/lib/repoallowlist"`; add a second import for the bare `lib` package)
   - `"github.com/bborbe/run"` (already present at line 23)

4. **Add the third `run.Func` to `run.CancelOnFirstFinish`.** Replace the two-arg call at line 307-310 with the three-arg call:

   ```go
   return run.CancelOnFirstFinish(ctx,
       a.runPollLoop(pollOnce, pollInterval),
       a.createHTTPServer(pollOnce),
       commandConsumer,
   )
   ```

   The order of the three `run.Func`s is load-bearing for the spec § AC 9 evidence. Document the order in a one-line comment: `// Order: poll → HTTP → command consumer (spec 066 AC 9: three run.Funcs)`.

5. **Create `pkg/factory/command_consumer_test.go`** (external test package `factory_test`) with two test cases:

   a. **Factory wiring: non-nil dependencies return a non-nil `run.Func`.** Pass all real dependencies (use `libkafka.NewSaramaClientProviderNew(a.KafkaBrokers)` with a dummy broker, `libkv.NewMemDB()`, real `*mocks.GitHubClient`, `*taskmocks.TaskCreateCommandSender`, `*mocks.TaskCreationFilter`, `*mocks.Trust`, `pkg.NewMetrics()`). Assert the returned `run.Func` is non-nil. **Do not** invoke the returned function — invoking it would try to connect to Kafka.

   b. **Factory wiring: zero-logic invariant via AST walk.** Use `go/ast` + `go/parser` to parse `factory.go`, find the `CreateCommandConsumer` function declaration, walk its body, and assert it contains zero `*ast.IfStmt`, zero `*ast.ForStmt`, zero `*ast.RangeStmt`, zero `*ast.SwitchStmt`, zero `*ast.TypeSwitchStmt`. Composite literals (e.g. `cdb.CommandObjectExecutorTxs{...}`) and function calls are fine. This is robust to whitespace/formatting drift unlike a text grep. Sketch:

   ```go
   It("CreateCommandConsumer body has no control flow", func() {
       fset := token.NewFileSet()
       file, err := parser.ParseFile(fset, "factory.go", nil, parser.AllErrors)
       Expect(err).NotTo(HaveOccurred())
       var fn *ast.FuncDecl
       for _, decl := range file.Decls {
           if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "CreateCommandConsumer" {
               fn = f
               break
           }
       }
       Expect(fn).NotTo(BeNil(), "CreateCommandConsumer not found")
       ast.Inspect(fn.Body, func(n ast.Node) bool {
           switch n.(type) {
           case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt:
               Fail(fmt.Sprintf("CreateCommandConsumer body contains forbidden control flow: %T at %v", n, fset.Position(n.Pos())))
           }
           return true
       })
   })
   ```

   No nil-check panic-table test — `CreateCommandConsumer` does not nil-check (per req 2 zero-logic), so there is no panic message to assert against.

6. **Create `pkg/factory/integration_test.go`** (external test package `factory_test`) with two INTEGRATION tests. These are the spec's headline durability + wiring tests (AC 9, 10, 11, 13, 15). They use a real `run.Func` invocation but mock the Kafka layer with a counterfeiter-friendly boundary (the `cdb.CommandObjectSender` is the seam).

   a. **Clean-shutdown test (AC 10).** Wire the three `run.Func`s (poll, HTTP, command consumer) into a `run.CancelOnFirstFinish`, then cancel the parent context. Assert:
   - All three `run.Func`s return within the framework's standard grace period (5s in the default `RunCommandConsumerTx`).
   - `goleak.VerifyTestMain` (or the equivalent `goleak` import: `go.uber.org/goleak`) reports zero leaked goroutines.

   If `goleak` is not already a project dep, skip the explicit goroutine-leak assertion and rely on the framework's own ctx-cancellation contract: the test asserts that `run.CancelOnFirstFinish` returns within 5s after `cancel()`, and that the returned error is `context.Canceled` (or nil — depends on the `run.Func` exit semantics). Document the absence of the `goleak` assertion in a comment.

   b. **End-to-end command flow test (AC 11).** Construct the full consumer pipeline: real `libkafka.NewSaramaClientProviderNew(...)` with a TEST broker (use `kafka:9092` if a docker-compose is available, otherwise use a stub broker address — the test will fail to subscribe but the executor's `HandleCommand` is the unit under test, not Kafka), the prompt-2 executor, and a `*mocks.GitHubClient` whose `GetPRDetailsReturns` provides a stubbed `PRDetails`. Then:
   - Pre-create a `TriggerPRReviewCommand` as a `cdb.CommandObject` (use `base.ParseEvent` + `cdb.CommandObject{...}`).
   - Capture the baseline `github_pr_published{result="create"}` counter value via `testutil.ToFloat64` (from `github.com/prometheus/client_golang/prometheus/testutil`).
   - Invoke the executor's `HandleCommand` directly via the `CommandObjectExecutorTxFunc` constructor (the inner closure).
   - Assert the metric delta is exactly 1.
   - Assert the captured `createSender.SendCommandArgsForCall(0)` payload contains the expected `TaskIdentifier` (derived from the same `prInfo.Owner`, `prInfo.Repo`, `prInfo.Number`, `details.HeadSHA`).

   This is NOT a Kafka-roundtrip test — it is a "wired-up consumer invocation" test that proves the factory composition is correct without requiring a live Kafka broker. The test name MUST reflect this: `Context("end-to-end command flow through wired consumer (spec 066 AC 11)", ...)`.

7. **Update `CHANGELOG.md` at the repo root.** Append to `## Unreleased` (create the section above the latest `## vX.Y.Z` heading if absent — the latest is `## v0.36.0`):

   ```markdown
   ## Unreleased

   - feat(watcher/github-pr): split /trigger into CQRS pair — HTTP handler validates the PR URL and publishes a `TriggerPRReviewCommand` to Kafka (returns 202), an in-pod command consumer (third `run.Func` alongside the poll loop and HTTP server) runs the GitHub fetch + filter + trust + downstream `CreateTaskCommand` publish. Pod crashes mid-trigger survive via Kafka redelivery (downstream task_id is derived and idempotent). HTTP wire shape changes from `200 + {status,task_id,repo,pr_number,head_sha}` to `202 + {status,url}`; filter-skip and trust-reject become silent in the HTTP response (visible in `github_pr_published{result="skipped"|"kafka_error"|"trust_error"}` metrics). The `/admin/trigger` mount path and the `GithubPRReviewV1SchemaID` are unchanged.
   - test(watcher/github-pr): add `TriggerPRReviewCommand` operation constant, sender, executor, byte-identical payload parity, crash-recovery, panicking-GitHub-client, clean-shutdown, and end-to-end command flow tests (spec 066)
   ```

   Two bullets: the `feat:` (the user-visible behavior change) and the `test:` (the new test coverage). The `feat:` prefix triggers a minor bump; the `test:` prefix triggers a patch bump; the minor bump wins per dark-factory's prefix precedence rules.

8. **Run `make precommit` in the changed module.** From the github-pr watcher dir:

   ```
   cd /workspace/watcher/github-pr && make precommit
   ```

   Expected: exit code 0. The `precommit` target chains `ensure + format + generate + test + check + addlicense` — all six must pass. The `lint` step (`golangci-lint`) catches any unused-import or shadow-variable issue. The `addlicense` step adds the BSD license header to any new Go file that is missing it (it skips the generated counterfeiter mocks and the `mocks.go` package marker).

   If `make precommit` fails on a specific target, fix the issue and re-run ONLY that target. Do NOT re-run the full `make precommit` until all individual targets pass. See `CLAUDE.md` for the fix-loop pattern.

9. **Verify the three-arg evidence (spec § AC 9):**

   ```
   grep -n 'run.CancelOnFirstFinish' /workspace/watcher/github-pr/main.go
   ```

   Must show a 3-argument call. The auditor will grep for the `commandConsumer` variable name in the same line range.

   ```
   grep -n 'CreateCommandConsumer' /workspace/watcher/github-pr/main.go
   ```

   Must show exactly one call site (the wiring in step 3).

10. **YAGNI guard.** Do NOT add a `boltdb`-backed `db` — the in-process `libkv.NewMemDB()` is the spec's deliberate choice (session-scoped offsets, replay-from-oldest on restart is safe because the downstream `CreateTaskCommand` is idempotent). Do NOT add a configurable offset-reset policy — `OffsetOldest` is hard-coded in `RunCommandConsumerTxDefault` and is the correct default. Do NOT add a `ReadOnly` flag for the executor — the executor is always invoked by the framework. Do NOT add a `MaxInflight` knob — the framework's `BatchSize(1)` is the correct default for a single-PR executor. Do NOT add a `RetryBackoff` knob — the framework's `commandExpireDuration` of 5 minutes (the `RunCommandConsumerTxDefault` default) is the spec's deliberate choice.
</requirements>

<constraints>
- The consumer uses `cdb.RunCommandConsumerTxDefault` (NOT a manual `RunCommandConsumerTx` with a tx-wrapped executor). The `auto-tx-wrapper-no-manual-wrap` rule from `go-cqrs.md` is non-negotiable.
- The factory `CreateCommandConsumer` is zero-logic except for nil-checks. No `for` loops, no `switch` statements, no business logic. The optional AST-based assertion in the test enforces this.
- The factory's `CreateTriggerPRReviewCommandSender` is also zero-logic — it composes `cdb.NewCommandObjectSender` + `command.NewTriggerPRReviewCommandSender` and returns the typed interface. No branching.
- `run.CancelOnFirstFinish` in `main.go` has exactly three arguments. The order (poll, HTTP, command consumer) is the spec's AC 9 evidence.
- The `db := libkv.NewMemDB()` is session-scoped. The pod replays the request topic from `OffsetOldest` on restart — this is safe because the downstream `CreateTaskCommand` is idempotent via derived `task_id`. Do NOT add a persistent offset store in this spec.
- `saramaClientProvider` is built fresh per pod via `libkafka.NewSaramaClientProviderNew(a.KafkaBrokers)`. Do NOT reuse the HTTP-side sender's underlying Sarama client (the consumer needs its own connection lifecycle).
- `lib.GithubPRReviewV1SchemaID` is imported via `lib "github.com/bborbe/maintainer/lib"`. Do NOT define a new schema in this prompt.
- Error wrapping: `github.com/bborbe/errors` only. Never `fmt.Errorf`. Always pass `ctx` to error constructors. Never `context.Background()` in `pkg/`.
- The integration tests are INTEGRATION tests (per `go-test-types-guide.md`): they touch a real `run.Func` invocation, they observe real metric deltas, they may use mocked Kafka. They live in `pkg/factory/integration_test.go` and register in the same Ginkgo suite.
- The clean-shutdown test does not require a live Kafka broker — it tests the `run.Func` exit semantics. The end-to-end test invokes the executor's `HandleCommand` directly (not via Kafka) and observes the metric delta + downstream sender call count.
- The CHANGELOG entry has the `feat:` prefix (the user-visible behavior change is the headline) and a `test:` prefix (the new test coverage). Both bullets are required.
- Ginkgo v2 + Gomega + counterfeiter. External test package (`factory_test`). Coverage on the new code ≥ 80% per `docs/definition-of-done.md`.
- Do NOT commit — dark-factory handles git. Branch: `dark-factory/cqrs-trigger-github-pr`.
- Build verification: `cd /workspace/watcher/github-pr && make precommit` must exit 0.
</constraints>

<verification>

Verify the factory has the two new functions:
```
grep -n 'func CreateCommandConsumer\|func CreateTriggerPRReviewCommandSender' /workspace/watcher/github-pr/pkg/factory/factory.go
```
Must show both function declarations with the expected signatures.

Verify the consumer uses `cdb.RunCommandConsumerTxDefault` (not the manual-tx variant):
```
grep -n 'cdb.RunCommandConsumerTxDefault\|cdb.RunCommandConsumerTx ' /workspace/watcher/github-pr/pkg/factory/factory.go
```
Must show `cdb.RunCommandConsumerTxDefault` exactly once. Zero matches for `cdb.RunCommandConsumerTx ` (with a trailing space — that's the manual-tx variant).

Verify the main.go has the three-arg `run.CancelOnFirstFinish`:
```
grep -A 5 'run.CancelOnFirstFinish' /workspace/watcher/github-pr/main.go
```
Must show three arguments: `a.runPollLoop(...)`, `a.createHTTPServer(...)`, `commandConsumer`.

Verify the CHANGELOG entry landed:
```
grep -B 1 -A 4 '## Unreleased' /workspace/CHANGELOG.md | head -20
```
Must show the two new bullets (the `feat(watcher/github-pr):` and the `test(watcher/github-pr):` lines). The `## Unreleased` heading must exist (create it if absent — there is no `## Unreleased` section today).

Run the full precommit (the final gate):
```
cd /workspace/watcher/github-pr && make precommit
```
Expected: exit code 0. All six chained targets (ensure, format, generate, test, check, addlicense) pass.

Run the new factory tests in isolation:
```
cd /workspace/watcher/github-pr && go test -mod=mod -v -count=1 ./pkg/factory/...
```
Expected: exit code 0; the `CreateCommandConsumer` nil-panic cases pass; the zero-logic invariant test passes; the integration tests pass (or skip with a documented reason if `goleak` is unavailable).

Spot-check the metric increments in the integration test:
```
cd /workspace/watcher/github-pr && go test -mod=mod -v -count=1 -run "end-to-end command flow" ./pkg/factory/...
```
Expected: the test passes; the metric delta is exactly 1 for the `create` label; the captured `CreateCommand` payload has the expected `TaskIdentifier`.

Audit evidence the auditor will collect:
- `grep -n 'CreateCommandConsumer' /workspace/watcher/github-pr/main.go` → 1 call site
- `grep -n 'commandConsumer' /workspace/watcher/github-pr/main.go` → 1 reference inside `run.CancelOnFirstFinish`
- `grep -c 'cdb.RunCommandConsumerTxDefault' /workspace/watcher/github-pr/pkg/factory/factory.go` → 1
- `grep -n '## Unreleased' /workspace/CHANGELOG.md` → 1 (the new section header)
</verification>
