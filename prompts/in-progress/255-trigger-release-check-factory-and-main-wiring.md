---
status: approved
spec: [067-cqrs-trigger-github-release]
created: "2026-06-09T00:00:00Z"
queued: "2026-06-09T10:57:42Z"
branch: dark-factory/cqrs-trigger-github-release
---

<summary>

- Adds `factory.CreateTriggerReleaseCheckCommandSender(ctx, syncProducer, branch)` — builds `base.CommandCreator` + `cqrsiam.Initiator("watcher-github-release")` + `cdb.NewCommandObjectSender(...)` ONCE at construction and passes them to `command.NewTriggerReleaseCheckCommandSender` (per the `cqrs/docs/producing-commands.md` "Factory Wiring" pattern).
- Adds `factory.CreateCommandConsumer(...)` that returns a `run.Func` consuming `TriggerReleaseCheckCommand` messages via `cdb.RunCommandConsumerTxDefault(..., lib.GithubReleaserV1SchemaID, ..., executors)` (auto-tx-wrapper, no manual middleware).
- Updates `main.go` to register the new command consumer as the third `run.Func` inside `run.CancelOnFirstFinish`, replacing the `/trigger` route handler with the thin publisher from prompt 3. The watcher instance is built once in `Run` and shared between the poll-interval loop and the new consumer.
- Adds a `pkg.NewMemDB()` session-scoped offset store so the consumer can survive a pod crash + restart and replay the request topic from `OffsetOldest` (safe because the downstream `CreateTaskCommand` is idempotent via derived task_id).
- Clean-shutdown integration test: cancelling the parent context shuts down all three loops within the framework's grace period.
- Crash-recovery integration test: simulating a killed invocation (context cancellation mid-Poll) followed by a fresh invocation proves the executor re-runs `Poll(ctx)` from scratch on a redelivered command — `fakeWatcher.PollCallCount() == 2` within 30s.

</summary>

<objective>
This is the wiring prompt. It connects the leaf pieces from prompts 1-3 (command, sender, executor, handler) into a runnable github-release pod that uses Kafka redelivery for `/trigger` durability. After this prompt lands, `cd watcher/github-release && make precommit` exits 0 and the manual smoke (curl `/trigger` → 202 → consumer logs show poll cycle) works against a deployed dev pod.
</objective>

<context>

- Reference factory: `/workspace/watcher/github-pr/pkg/factory/factory.go` (mirror structurally — `CreateGitHubAppClient`, `CreateKafkaSender`, `CreateWatcher`, `CreateTriggerPRReviewCommandSender`, `CreateCommandConsumer`).
- Reference factory tests: `/workspace/watcher/github-pr/pkg/factory/command_consumer_test.go`, `.../integration_test.go`, `.../single_pr_test.go`.
- Reference main.go: `/workspace/watcher/github-pr/main.go` (mirror the `run.CancelOnFirstFinish` three-loop wiring, the `pkg.NewMemDB()` offset store, the `factory.CreateCommandConsumer` call site, the order: poll → HTTP → command consumer).
- Existing factory (to extend): `/workspace/watcher/github-release/pkg/factory/factory.go` — `CreateKafkaSender`, `CreateStaticFilters`, `CreateWatcher` are present.
- Existing main.go (to modify): `/workspace/watcher/github-release/main.go` — already wires `w := factory.CreateWatcher(...)` and `run.CancelOnFirstFinish(ctx, pollLoop, httpServer)`. Replace the second argument to also include the command consumer; replace the `/trigger` route handler; build a `pkg.NewMemDB()` offset store; share the watcher between the poll loop and the consumer.
- Existing run-once main: `/workspace/watcher/github-release/cmd/run-once/main.go` — has its own `Run` method that calls `a.CreateWatcher(...)` and `w.Poll(ctx)` directly. OUT OF SCOPE for this prompt (run-once is a one-shot CLI, not a daemon). Do NOT touch it. The watcher in run-once is a fresh, in-process instance; there is no Kafka in the run-once path.
- Schema ID: `lib.GithubReleaserV1SchemaID` from `/workspace/lib/maintainer_cdb-schema.go`.
- cqrs types (verified):
  - `cdb.RunCommandConsumerTxDefault(saramaClientProvider, syncProducer, db, schemaID, branch, ignoreUnsupported, executors, options...)` — at `cdb_run-command-consumer-tx.go:19`.
  - `cdb.CommandObjectExecutorTxs []CommandObjectExecutorTx` — at `cdb_command-object-executor-tx.go:15`.
  - `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)` — at `cdb_command-object-sender.go:28`.
  - `base.NewCommandCreator(<-chan base.RequestID) base.CommandCreator` — at `base/base_command-creator.go:29`.
  - `base.RequestIDChannel(ctx) <-chan base.RequestID` — at `base/base_request-id.go:40`.
  - `cqrsiam.Initiator("watcher-github-release")` — string-typed initiator.
- libkafka types (verified at `github.com/bborbe/kafka`):
  - `libkafka.NewSyncProducerWithName(ctx, brokers, name)` — used today.
  - `libkafka.NewSaramaClientProviderNew(brokers)` — used in github-pr main.go:303.
- librun types: `run.Func = func(ctx) error`; `run.CancelOnFirstFinish(ctx, ...funcs) error` — used today.

In-container docs:

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — `Create*` for factories, zero-logic rule, no I/O in factories.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — `auto-tx-wrapper-no-manual-wrap` rule (do NOT manually wrap with `kv.NewTransactionMiddleware`; use `cdb.RunCommandConsumerTxDefault`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` — `run.CancelOnFirstFinish` over `go func()`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — counterfeiter directive above type.

</context>

<ac-coverage>

This prompt covers these spec 067 acceptance criteria:

- **AC 1** — `cd watcher/github-release && make precommit` exits 0. This is the headline AC; the prompt's `<verification>` block runs it.
- **AC 9** — `main.go` registers three `run.Func`s inside `run.CancelOnFirstFinish` (poll-interval loop, HTTP server, command consumer). The req 3f step pins the order: poll → HTTP → command consumer.
- **AC 10** — cancelling the parent context from a startup integration test shuts down all three loops within the framework's standard grace period without leaked goroutines. (Mirrors github-pr's pattern: ctx-cancellation contract verified, goleak not used as a project dep.)
- **AC 13** — the new consumer is wired via `cdb.RunCommandConsumerTxDefault(... lib.GithubReleaserV1SchemaID ...)` (no manual transaction wrapping via `kv.NewTransactionMiddleware`).
- **AC 14** — `factory.CreateTriggerReleaseCheckCommandSender(...)` builds `base.CommandCreator` + `cqrsiam.Initiator` ONCE at construction and passes them to `command.NewTriggerReleaseCheckCommandSender(...)`.
- **AC 18** — the factory + main wiring rows: counterfeiter directive placement, glog.V(2) log position, schema reference.
- **AC 19** — crash-recovery integration test at the consumer-level (using `RunTriggerReleaseCheck` from prompt 2's `_export_test.go`).
- **AC 20** — coverage on new code in `pkg/command/` and `pkg/factory/` is ≥ 80%.
- **AC 21** — schema reference uses `lib.GithubReleaserV1SchemaID` directly; zero literal `"maintainer-githubreleaser-v1"` in `watcher/github-release/`.

</ac-coverage>

<requirements>

1. Extend `watcher/github-release/pkg/factory/factory.go` with two new functions (append to the existing file; do NOT touch the existing `CreateKafkaSender`, `CreateStaticFilters`, `CreateWatcher`):

   ```go
   // CreateTriggerReleaseCheckCommandSender constructs a typed trigger-release-check
   // command sender backed by a Kafka sync producer. This is the HTTP-side
   // sender: the /trigger handler publishes TriggerReleaseCheckCommand messages
   // through it.
   //
   // CommandCreator and Initiator are built once here and reused across every
   // SendCommand call (per cqrs/docs/producing-commands.md "Factory Wiring";
   // matches trading/frontend/command's reference impl).
   func CreateTriggerReleaseCheckCommandSender(
       ctx context.Context,
       syncProducer libkafka.SyncProducer,
       branch base.Branch,
   ) command.TriggerReleaseCheckCommandSender {
       return command.NewTriggerReleaseCheckCommandSender(
           base.NewCommandCreator(base.RequestIDChannel(ctx)),
           cqrsiam.Initiator("watcher-github-release"),
           cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory),
       )
   }

   // CreateCommandConsumer wires a run.Func that consumes TriggerReleaseCheckCommand
   // messages from the github-release watcher's request topic and runs them through
   // the shared Watcher.Poll(ctx) pipeline.
   //
   // The function is pure composition: no business logic, no conditionals.
   // It uses cdb.RunCommandConsumerTxDefault (auto-wraps the transaction) per
   // the go-cqrs/auto-tx-wrapper-no-manual-wrap rule — do NOT manually wrap
   // the executor with kv.NewTransactionMiddleware.
   func CreateCommandConsumer(
       saramaClientProvider libkafka.SaramaClientProvider,
       syncProducer libkafka.SyncProducer,
       db libkv.DB,
       watcher pkg.Watcher,
       branch base.Branch,
   ) run.Func {
       executors := cdb.CommandObjectExecutorTxs{
           command.NewTriggerReleaseCheckCommandExecutor(watcher),
       }
       return cdb.RunCommandConsumerTxDefault(
           saramaClientProvider,
           syncProducer,
           db,
           lib.GithubReleaserV1SchemaID,
           branch,
           false, // ignoreUnsupported
           executors,
       )
   }
   ```

   Add imports as needed: `libkafka "github.com/bborbe/kafka"`, `libkv "github.com/bborbe/kv"`, `"github.com/bborbe/run"`, `lib "github.com/bborbe/maintainer/lib"`, `cqrsiam "github.com/bborbe/cqrs/iam"`. Keep existing imports for `pkg`, `filter`, `task`, `base`, `cdb`, `log`.

2. Add `factory.CreateTriggerReleaseCheckHandler(sender command.TriggerReleaseCheckCommandSender) handler.TriggerReleaseCheckHandler` to the factory (so the wiring in main.go is symmetric with the github-pr `CreateSinglePRTriggerHandler`):

   ```go
   // CreateTriggerReleaseCheckHandler wires the thin CQRS handler that publishes a
   // TriggerReleaseCheckCommand to Kafka for each /trigger request.
   // All poll-cycle work lives in the in-pod command consumer (see
   // pkg/command.NewTriggerReleaseCheckCommandExecutor).
   func CreateTriggerReleaseCheckHandler(
       sender command.TriggerReleaseCheckCommandSender,
   ) handler.TriggerReleaseCheckHandler {
       return handler.NewTriggerReleaseCheckHandler(sender)
   }
   ```

3. Update `watcher/github-release/main.go`:

   a. Add imports:
      ```go
      lib "github.com/bborbe/maintainer/lib"
      "github.com/bborbe/maintainer/watcher/github-release/pkg/handler"
      ```

   b. After `w := factory.CreateWatcher(...)`, add:
      ```go
      // HTTP-side sender backs the /trigger handler.
      triggerReleaseCheckSender := factory.CreateTriggerReleaseCheckCommandSender(ctx, syncProducer, branch)
      triggerHandler := factory.CreateTriggerReleaseCheckHandler(triggerReleaseCheckSender)
      a.TriggerHandler = libhttp.NewJSONErrorHandler(triggerHandler)
      ```
      (Define `branch := base.Branch(a.Stage)` if not already — it is not in the current main.go; add it before the sender call.)

   c. Add the third `run.Func` wiring BEFORE `run.CancelOnFirstFinish`:
      ```go
      // In-pod command consumer: third run.Func alongside poll + HTTP.
      // session-scoped offset store — replays the request topic from OffsetOldest
      // on pod restart; safe because the downstream CreateTaskCommand is idempotent
      // via the derived task_id.
      saramaClientProvider := libkafka.NewSaramaClientProviderNew(a.KafkaBrokers)
      db := pkg.NewMemDB()
      commandConsumer := factory.CreateCommandConsumer(
          saramaClientProvider,
          syncProducer,
          db,
          w, // shared with the poll-interval loop
          branch,
      )
      ```
      Add the application field `TriggerHandler http.Handler` to the `application` struct (mirroring github-pr main.go:138).

   d. Add the `application.TriggerHandler` field. Wrap the `triggerHandler` with `libhttp.NewJSONErrorHandler(triggerHandler)` and store it on `a.TriggerHandler`.

   e. Replace the `/trigger` route in `createHTTPServer`:
      - Before: `router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))`
      - After: `router.Path("/trigger").Handler(a.TriggerHandler)`
      The `poll` closure is now ONLY used by the poll-interval loop (and is no longer needed in `createHTTPServer`); the `createHTTPServer(poll run.Func)` signature can drop the `poll` parameter, OR keep it for symmetry. RECOMMENDATION: drop the parameter — the HTTP server no longer needs `poll`. Update both call sites in `Run`. If `createHTTPServer` becomes parameterless, also update any test that uses it. (No such test exists in github-release today.)

   f. Update `run.CancelOnFirstFinish` to pass three funcs in this order: poll loop, HTTP server, command consumer (spec AC 9: three run.Funcs).

   g. After the wiring, the existing `application` struct gains `TriggerHandler http.Handler`. Add a starting log line that includes the schema ID, mirroring github-pr main.go:321-322:
      ```go
      glog.V(2).Infof(
          "maintainer-watcher-github-release starting stage=%s owner=%s interval=%s listen=%s schema=%s",
          a.Stage, a.Owner, a.PollInterval, a.Listen, lib.GithubReleaserV1SchemaID,
      )
      ```

4. Add `watcher/github-release/pkg/factory/command_consumer_test.go` mirroring `watcher/github-pr/pkg/factory/command_consumer_test.go`, with these test cases (substitute the github-release symbols):

   a. `Describe("CreateTriggerReleaseCheckCommandSender")` — assert it returns a non-nil sender given a mock sync producer + branch.

   b. `Describe("CreateCommandConsumer")`:
      - `It("returns a non-nil run.Func when all dependencies are non-nil", ...)` — wire a mocks.Watcher, mocks.KafkaSyncProducer, mocks.KafkaSaramaClientProvider, mocks.DB; assert non-nil.
      - `It("CreateCommandConsumer body has no control flow", ...)` — use `go/parser` + `go/ast` to walk the function body and `Fail` on any `*ast.IfStmt`, `*ast.ForStmt`, `*ast.RangeStmt`, `*ast.SwitchStmt`, `*ast.TypeSwitchStmt` (spec requirement: factory is pure composition). Mirror github-pr's exact test.

   c. `Describe("NewMemDB")`:
      - `It("returns a non-nil DB", ...)` (from prompt 1) — already exists at `pkg/memdb_test.go` (or wherever prompt 1 placed it); do NOT duplicate. If prompt 1 put it in `pkg/watcher_test.go`, leave it; if in `pkg/memdb_test.go`, leave it.

5. Add `watcher/github-release/pkg/factory/integration_test.go` mirroring `watcher/github-pr/pkg/factory/integration_test.go`, with these test cases:

   a. `Describe("clean shutdown of three run.Funcs (spec 067 AC 10)", ...)`:
      - Construct three local `run.Func` values that all return when their ctx is cancelled (mirroring github-pr's test exactly).
      - Cancel the parent ctx, assert all three `doneCh` receives within 5s.
      - Comment: "goleak: not used here (not a project dep) — rely on the ctx-cancellation contract only." (mirrors github-pr's test comment).

   b. `Describe("end-to-end command flow through wired executor (spec 067 AC 8 + AC 19)", ...)`:
      - Build the executor via `command.NewTriggerReleaseCheckCommandExecutor(watcher)`.
      - Build a `cdb.CommandObject` from `command.TriggerReleaseCheckCommand{}` with the correct schema ID and operation.
      - Call `executor.HandleCommand(ctx, nil, newCommandObject())`.
      - Assert `watcher.PollCallCount() == 1` (the executor invoked the watcher once).
      - This is the "factory composition succeeds and the executor publishes exactly one downstream task" sanity check, adapted: github-release has no downstream `CreateCommandSender` at the executor level (the watcher publishes via `pkg.TaskPublisher`), so the equivalent assertion is `PollCallCount == 1`.

   c. `Describe("crash recovery (spec 067 AC 19 — at-least-once via idempotent Watcher)", ...)`:
      - Use the `RunTriggerReleaseCheck` exported helper (from prompt 2's `_export_test.go`).
      - Round 1: feed a `cdb.CommandObject` to `RunTriggerReleaseCheck` with a killed ctx whose `Watcher.PollStub` cancels the ctx and returns `c.Err()`. Assert non-nil error, NOT `cdb.ErrCommandObjectSkipped`, `PollCallCount == 1` (the killed invocation did invoke Poll once before the cancellation).
      - Round 2: with a fresh ctx + fresh Watcher (PollReturns(nil)) + the SAME `cdb.CommandObject`, call `RunTriggerReleaseCheck` again. Assert nil error and `freshWatcher.PollCallCount() == 1`.
      - Assert the final assertion verbatim: `Eventually(func() int { return fakeWatcher.PollCallCount() }, 30*time.Second, 100*time.Millisecond).Should(BeNumerically(">=", 2))` — the spec's headline durability claim that Kafka redelivery produces at-least-once execution.
      - This mirrors github-pr's crash-recovery test (`trigger_pr_review_executor_test.go:295-386`) exactly, adapted to the github-release domain.

6. Add `watcher/github-release/pkg/factory/single_pr_test.go` mirror for the handler factory function. (The github-pr test file is at `watcher/github-pr/pkg/factory/single_pr_test.go`; mirror the structure with `CreateTriggerReleaseCheckHandler`.)

7. The `cmd/run-once/main.go` is OUT OF SCOPE — do NOT touch it. It uses its own `Application` struct with a `WatcherFactory` and `ProducerFactory`; the new sender/consumer are not in its path. Document this in the prompt's changelog and the prompt's `<verification>` section.

8. Verify the `Run` orchestration in main.go has exactly three arguments to `run.CancelOnFirstFinish` (poll loop, HTTP server, command consumer — spec AC 9). Source-grep the file and confirm.

9. Verify the consumer is wired via `cdb.RunCommandConsumerTxDefault(... lib.GithubReleaserV1SchemaID ...)` — no manual transaction wrapping. Source-grep `pkg/factory/factory.go` and confirm only one mention of `cdb.RunCommandConsumerTxDefault` and ZERO mentions of `kv.NewTransactionMiddleware` in the new code.

10. Verify the `CreateTriggerReleaseCheckCommandSender` body forwards both `commandCreator` and `initiator` to the `command.NewTriggerReleaseCheckCommandSender(...)` constructor call. Source-grep `pkg/factory/factory.go` and confirm (spec AC 14).

11. Verify NO literal `"maintainer-githubreleaser-v1"` appears in `watcher/github-release/` outside of test assertions on the schema struct. The schema reference is `lib.GithubReleaserV1SchemaID` everywhere in production code (spec AC 21).

12. Append to `## Unreleased` in `/workspace/CHANGELOG.md`:
    ```
    - feat: Wire github-release /trigger through Kafka command consumer — third run.Func, factory.CreateCommandConsumer, shared Watcher, MemDB offset store
    - test: Add clean-shutdown and crash-recovery integration tests for the wired command consumer
    ```

</requirements>

<constraints>

- The factory functions MUST be pure composition — no conditionals, no I/O, no `context.Background()` calls, no `if x == nil { panic }` guards (spec factory rule).
- The consumer MUST use `cdb.RunCommandConsumerTxDefault` (auto-tx wrapper). Do NOT use `kv.NewTransactionMiddleware` (go-cqrs `auto-tx-wrapper-no-manual-wrap` rule).
- The watcher is built ONCE in `Run` and passed BOTH to the poll-interval loop and to `CreateCommandConsumer`. Sharing the same `*watcher` pointer means `Watcher.Poll(ctx)` from the executor and from the poll loop are the same method on the same instance — concurrent calls already exist today (the poll loop and the `/check` endpoint can race) and the spec non-goal "no in-process serialization is added by this spec" is preserved.
- `branch := base.Branch(a.Stage)` MUST be defined exactly once in `Run` and reused for `factory.CreateKafkaSender`, `factory.CreateTriggerReleaseCheckCommandSender`, and `factory.CreateCommandConsumer`. Do not compute it in three places.
- The `application` struct MUST add the `TriggerHandler http.Handler` field. Wrap with `libhttp.NewJSONErrorHandler` so JSON error bodies are emitted (consistent with github-pr).
- `pkg.NewMemDB()` is session-scoped — it does NOT persist across pod restarts. On restart the consumer replays from `OffsetOldest`, which is safe because the downstream `CreateTaskCommand` is idempotent via derived task_id (see github-pr's memdb.go docstring for the same reasoning).
- The integration test for crash recovery uses the exported `RunTriggerReleaseCheck` helper from prompt 2's `_export_test.go`. Do NOT bypass that export to test through `executor.HandleCommand` — `RunTriggerReleaseCheck` is the seam that mirrors the test pattern in github-pr.
- The factory-test "no control flow" test uses `go/parser` + `go/ast` to walk `CreateCommandConsumer`'s body and fail on any if/for/range/switch/type-switch statements. This is a hard guardrail from spec 067 constraints.
- BSD copyright header on every new/modified file, dated 2026.
- Error wrapping uses `github.com/bborbe/errors` exclusively.
- Do NOT modify `cmd/run-once/main.go`. It is a one-shot CLI; the spec scope is the daemon (`main.go`).
- Do NOT add a `Force` executor branch, a `Scope` executor branch, or a per-repo filter. Spec non-goals forbid all three.
- Do NOT enable `SendResultEnabled` — it stays `false` (set in the executor in prompt 2).
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

```bash
cd watcher/github-release && make test
```

Must pass — all existing tests, prompt 1-3 tests, and the new factory + integration tests in this prompt run green. Then:

```bash
cd watcher/github-release && make precommit
```

Must exit 0. (This is the headline AC: spec AC 1.)

Additional manual checks the executor should perform:

- `grep -n "run.CancelOnFirstFinish" watcher/github-release/main.go` shows exactly one call site with three arguments.
- `grep -n "factory.CreateCommandConsumer" watcher/github-release/main.go` shows the call site.
- `grep -rn "kv.NewTransactionMiddleware" watcher/github-release/` returns NO matches (auto-tx-wrapper rule).
- `grep -n 'RunCommandConsumerTxDefault' watcher/github-release/pkg/factory/factory.go` shows the call.
- `grep -n 'GithubReleaserV1SchemaID' watcher/github-release/pkg/factory/factory.go` shows the schema reference (no string literal).
- `grep -rn '"maintainer-githubreleaser-v1"' watcher/github-release/` returns NO matches in production code (only test assertions on the schema struct, if any).
- The crash-recovery integration test in `pkg/factory/integration_test.go` passes: `Eventually(fakeWatcher.PollCallCount()).Should(BeNumerically(">=", 2))` within 30s.
- The clean-shutdown integration test passes: all three `doneCh` receives within 5s.
- `cd watcher/github-release && go test -coverprofile=/tmp/cover.out ./pkg/command/... ./pkg/factory/... ./pkg/handler/... && go tool cover -func=/tmp/cover.out | awk '/total:/ {print $3}'` reports `>= 80.0%` for new code (spec AC 20).
- `cmd/run-once/main.go` is unchanged (`git diff` shows zero lines in that file).
- The CHANGELOG has new entries under `## Unreleased`.

Manual smoke (deployed dev pod):

```bash
curl -sS -o /dev/null -w '%{http_code}\n' -X POST \
  "https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-release/trigger"
# Expect: 202
kubectldev -n maintainer logs deploy/maintainer-watcher-github-release -f | grep -E 'trigger-release-check|poll cycle'
# Expect: trigger sender: published op=trigger-release-check AND poll cycle start
```

Crash-recovery smoke:

```bash
# 1. Send a trigger (curl above).
# 2. Immediately kill the pod: kubectlquant -n <stage> delete pod <pod>
# 3. Wait for pod to come back.
# 4. Verify the executor re-ran w.Poll(ctx) after restart (poll cycle start log line dated after pod restart timestamp).
```

</verification>

## Improvements

- [PROMPT] Frontmatter mixes `name:`/`description:`/`tags:` with the required `spec:` field; an audit should normalise to spec/status/created.
- [PROMPT] The "createHTTPServer drops the `poll` parameter" recommendation (req 3e) is a small refactor. The original line passes `poll` only for the `/check` endpoint which the spec also doesn't ship — verify that `poll` is not used anywhere else in `createHTTPServer` before dropping it. If the github-pr reference has any other consumer of the `poll` param, mirror the github-pr pattern instead.
- [PROMPT] The `createHTTPServer` parameter-drop is a load-bearing change that should appear in the prompt's diff manifest, not as a side comment. Audit can miss it.
- [PROMPT] The crash-recovery test in req 5c uses `RunTriggerReleaseCheck` directly — this is the executor-level test, NOT a real Kafka-consumer test. The spec AC 19 verbatim "Kafka redelivery → at-least-once execution" requires a real `cdb.RunCommandConsumerTxDefault` consumer to assert. The `Eventually(... PollCallCount() >= 2)` assertion can technically pass at the executor level (two direct calls in the same test) — it does NOT prove Kafka redelivery. Audit should require a real libkv-backed consumer in this integration test, not just the exported helper. (This is the single biggest gap in the prompt.)
- [GUIDE] `go-factory-pattern.md` should add a "factory no-control-flow test" snippet — the `go/parser`/`go/ast` walker is non-obvious and worth capturing in the guide.
- [GLOBAL] The spec language "crash-recovery" is ambiguous between (a) the executor's idempotency-under-retry and (b) Kafka consumer redelivery. Future specs should pick a primary meaning and disambiguate in the AC list.
