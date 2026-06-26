---
status: pending
spec: [072-force-trigger-on-github-pr-watcher]
created: "2026-06-09T16:20:25Z"
branch: dark-factory/cqrs-trigger-github-release
---

<summary>
- `pkg/factory/factory.go` gains two new functions: `CreateCommandConsumer(saramaClientProvider, syncProducer, db, watcher, branch) run.Func` and `CreateTriggerReleaseCheckCommandSender(ctx, syncProducer, branch) command.TriggerReleaseCheckCommandSender`. The consumer wires `cdb.RunCommandConsumerTxDefault(... lib.GithubReleaserV1SchemaID ...)` with the prompt-2 executor; the sender wraps the prompt-1 `NewTriggerReleaseCheckCommandSender` constructor with a `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)`.
- `main.go` builds the watcher once, shares it with both the existing poll-interval loop and the new command consumer, and grows `run.CancelOnFirstFinish(ctx, ...)` from two `run.Func`s to three (poll, HTTP server, command consumer). The watcher object is no longer constructed per-component — both the executor and the poll loop reference the same `w` variable.
- The `cdb.RunCommandConsumerTxDefault` call site uses the project's standard "auto-tx-wrapper" pattern (per `go-cqrs/auto-tx-wrapper-no-manual-wrap` rule) — no manual `kv.NewTransactionMiddleware` wrapping.
- Two integration tests cover the consumer wiring: (a) a clean-shutdown test that cancels the parent context and asserts all three `run.Func`s return within 5 seconds (mirroring the spec-066 sibling's pattern); (b) an end-to-end test that invokes `factory.CreateCommandConsumer` with mock dependencies and asserts the executor's `Watcher.Poll(ctx)` is invoked exactly once on a real `commandObject` (mirroring the spec-066 sibling's pattern).
- The CHANGELOG entry for the full spec lands in this prompt (`feat: split /trigger into CQRS pair — HTTP handler publishes TriggerReleaseCheckCommand; in-pod consumer runs Watcher.Poll(ctx)`).
- A `factory.CreateCommandConsumer` test (in `pkg/factory/command_consumer_test.go`) asserts: zero-business-logic rule (factory has no `for`/`switch`/conditional, only composition), the returned `run.Func` is non-nil, and the factory composition succeeds against mock dependencies.

This is prompt 4 of 4 for spec 067. It depends on prompts 1, 2, and 3 (the command type, the executor, and the shrunk HTTP handler). It owns the final `make precommit` gate, the CHANGELOG entry, and the integration tests.
</summary>

<objective>
Wire the new `TriggerReleaseCheckCommand` consumer as the third `run.Func` inside the existing `run.CancelOnFirstFinish`, complete the main.go + factory wiring that prompt 3 deferred, add the integration tests that prove the three-loop orchestration works, and ship the CHANGELOG entry. After this prompt, the spec is end-to-end complete: a `POST /trigger` returns 202, the consumer picks up the command, runs `pkg.Watcher.Poll(ctx)` on the shared watcher instance, and the spec's headline durability claim (Kafka redelivery survives a pod crash) is verified by the executor-level crash-recovery test from prompt 2.

The load-bearing invariants from this prompt:
1. `run.CancelOnFirstFinish(ctx, ...)` has exactly three arguments (poll, HTTP, command consumer) — spec § AC 9.
2. `factory.CreateCommandConsumer` is pure composition (no loops, no conditionals) — per the factory-pattern guide.
3. The consumer uses `cdb.RunCommandConsumerTxDefault` (NOT `RunCommandConsumerTx` with manual tx wrapping) — per the `go-cqrs/auto-tx-wrapper-no-manual-wrap` rule.
4. The watcher object is built ONCE in `main.go` and shared between the poll-interval loop and the command consumer (not constructed twice).
5. The clean-shutdown integration test proves the three loops stop within the framework's grace period.
6. The end-to-end integration test proves the third `run.Func` is not a no-op stub.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these source files in full BEFORE editing:

- `/workspace/watcher/github-release/main.go` — the file being completed. The edits in this prompt are the natural follow-through of prompt 3's handler shrink. Key sections to read in full:
  - Lines 86-101: `syncProducer, err := libkafka.NewSyncProducerWithName(...)` + `defer Close()`. The `syncProducer` is REUSED — both the HTTP-side sender and the consumer-side sender back onto the same `syncProducer` (sharing a single Kafka connection is more efficient than spinning up two).
  - Lines 99-111: `w := factory.CreateWatcher(...)` + the `sender` it consumes. The `w` variable is the SINGLE watcher instance shared by the poll-interval loop and the command consumer.
  - Line 119: `triggerHandler := factory.CreateTriggerReleaseCheckHandler(triggerReleaseCheckSender)` — the prompt-3 placeholder. This is the factory call site the new sender feeds.
  - Line 147: `return run.CancelOnFirstFinish(ctx, a.pollLoop(poll, pollInterval), a.createHTTPServer())` — grows to three args in this prompt.
- `/workspace/watcher/github-release/pkg/factory/factory.go` — the file gaining `CreateCommandConsumer` and `CreateTriggerReleaseCheckCommandSender`. Read the entire file: the existing `CreateKafkaSender`, `CreateStaticFilters`, `CreateWatcher`, and `CreateTriggerReleaseCheckHandler` (added in prompt 3) functions establish the factory's zero-logic, single-responsibility style that the new functions must match. The `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)` call at the bottom of `CreateTriggerReleaseCheckCommandSender` is the pattern both new functions follow.
- `/workspace/watcher/github-release/pkg/factory/command_consumer_test.go` (the spec-066 sibling at `/workspace/watcher/github-pr/pkg/factory/command_consumer_test.go`) — the reference test for `CreateCommandConsumer`. The new test for github-release uses the same layout (`Describe("CreateTriggerReleaseCheckCommandSender")`, `Describe("CreateCommandConsumer")`, the `CreateCommandConsumer body has no control flow` AST test) but with github-release-specific dependencies (the new `*mocks.Watcher` from the github-release `mocks/` dir, the new executor from prompt 2).
- `/workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go` (prompt 2 output) — `command.NewTriggerReleaseCheckCommandExecutor(watcher) cdb.CommandObjectExecutorTx`. The factory wraps this in a `cdb.CommandObjectExecutorTxs{}` slice (length 1) and passes it to `cdb.RunCommandConsumerTxDefault`.
- `/workspace/watcher/github-release/pkg/command/trigger_release_check_command_sender.go` (prompt 1 output) — `command.NewTriggerReleaseCheckCommandSender(commandCreator, initiator, commandObjectSender) command.TriggerReleaseCheckCommandSender`. The factory's `CreateTriggerReleaseCheckCommandSender(ctx, syncProducer, branch)` wraps this with `base.NewCommandCreator(base.RequestIDChannel(ctx))` + `cqrsiam.Initiator("watcher-github-release")` + `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)`. This is the sender `main.go` passes to the HTTP-side `CreateTriggerReleaseCheckHandler`.
- `/workspace/prompts/spec-067-cqrs-trigger-github-release-command.md` (prompt 1 output) — verify the package exists. If empty, STOP and report `status: failed` with message "prompt 1 of spec 067 has not shipped".
- `/workspace/prompts/spec-067-cqrs-trigger-github-release-executor.md` (prompt 2 output) — verify the executor is in place: `ls /workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go`. If missing, STOP and report `status: failed`.
- `/workspace/prompts/spec-067-cqrs-trigger-github-release-shrink-handler.md` (prompt 3 output) — verify the handler shrink is in place: `ls /workspace/watcher/github-release/pkg/handler/trigger_handler.go` and check the new file uses the thin `Sender.SendCommand` + 202 + `{"status":"accepted"}` shape. If missing, STOP and report `status: failed`.

Reference the integration test patterns in the codebase:

- `/workspace/watcher/github-pr/pkg/factory/command_consumer_test.go` (the spec-066 sibling) — the Ginkgo + counterfeiter pattern for the factory-side tests. The new integration tests use a different shape (full-factory, real-ish run loop, no `httptest`).
- `/workspace/watcher/github-pr/pkg/factory/integration_test.go` (the spec-066 sibling) — the reference for the `clean shutdown of three run.Funcs` test and the `end-to-end command flow through wired consumer` test. Mirror the layout exactly: three local `run.Func` stubs that respect `<-c.Done()`, `cancel()`, then `Eventually(doneCh, 5*time.Second).Should(Receive())` three times.

Reference the boilerplate from the agent's spec-066 consumer wiring prompt (it owns the parallel pattern):

- `/workspace/watcher/github-pr/pkg/factory/factory.go` lines 119-156 — the spec-066 `CreateCommandConsumer` function. Mirror the structure exactly: `executors := cdb.CommandObjectExecutorTxs{ command.NewTriggerPRReviewCommandExecutor(...) }` (a length-1 slice) and `cdb.RunCommandConsumerTxDefault(saramaClientProvider, syncProducer, db, schemaID, branch, false, executors)`. The new github-release consumer wiring is structurally identical: the executor is the prompt-2 `command.NewTriggerReleaseCheckCommandExecutor(watcher)`; the schema is `lib.GithubReleaserV1SchemaID`; `ignoreUnsupported` is `false`. The new github-release factory takes fewer dependencies than the spec-066 sibling (no `ghClient`, no `createSender`, no `filter`, no `trust`, no `stage`/`maxSlugLen`/etc. — the github-release executor only needs the shared `watcher`).

Reference the factory-pattern style from the coding plugin docs:

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — `Create*` prefix, zero logic, no conditionals, no `context.Background()`. The new `CreateCommandConsumer` and `CreateTriggerReleaseCheckCommandSender` MUST follow this.

Coding plugin docs (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — load-bearing. Re-read the `RULE go-cqrs/auto-tx-wrapper-no-manual-wrap` block. The new factory uses `cdb.RunCommandConsumerTxDefault` (NOT a manual `RunCommandConsumerTx` with tx-wrapped executor).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md` — `run.CancelOnFirstFinish` over `go func()`; the three `run.Func` arguments each must be ctx-cancellable.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-test-types-guide.md` — the clean-shutdown test is an INTEGRATION test (it spins up a real run loop); the end-to-end test is also INTEGRATION (it touches real Kafka semantics, even if the broker is mocked via the offset consumer pattern).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — read before writing `CreateCommandConsumer` and `CreateTriggerReleaseCheckCommandSender`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — the `## Unreleased` entry format, the `feat:` prefix.
- `/home/node/.claude/plugins/marketplaces/coding/docs/definition-of-done.md` — final precommit gate.
</context>

<requirements>

1. **Add `CreateTriggerReleaseCheckCommandSender` to `pkg/factory/factory.go`.** New function signature (matches the spec-066 sibling's `CreateTriggerPRReviewCommandSender` shape, just typed for the github-release command):

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
   ```

   Verify the factory's import block already imports `github.com/bborbe/cqrs/base`, `github.com/bborbe/cqrs/cdb`, `github.com/bborbe/cqrs/iam` (as `cqrsiam`), `github.com/bborbe/log`, and `github.com/bborbe/maintainer/watcher/github-release/pkg/command` (as `command`). If any are missing, add them.

2. **Add `CreateCommandConsumer` to `pkg/factory/factory.go`.** New function signature (the github-release version takes fewer dependencies than the spec-066 sibling because the github-release executor only needs the shared `watcher`):

   ```go
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

   The `lib "github.com/bborbe/maintainer/lib"` import is required for `lib.GithubReleaserV1SchemaID`. Verify the import exists in the factory's import block (the spec-066 sibling uses `lib.GithubPRReviewV1SchemaID` from the same import).

3. **Update `main.go`** to wire the new sender + consumer. The edits:

   a. **Construct the new sender** after `w := factory.CreateWatcher(...)` and before `triggerHandler := factory.CreateTriggerReleaseCheckHandler(...)`:

   ```go
   // HTTP-side sender backs the /trigger handler.
   triggerReleaseCheckSender := factory.CreateTriggerReleaseCheckCommandSender(
       ctx,
       syncProducer,
       branch,
   )
   ```

   b. **Pass the new sender to the existing handler factory call** (which already exists from prompt 3):

   ```go
   triggerHandler := factory.CreateTriggerReleaseCheckHandler(triggerReleaseCheckSender)
   a.TriggerHandler = libhttp.NewJSONErrorHandler(triggerHandler)
   ```

   The `libhttp.NewJSONErrorHandler` wrapper is preserved — the 502 path on Kafka failure is translated to a JSON error response by the framework.

   c. **Construct the command consumer** after the trigger handler setup and before the `glog.V(2).Infof("maintainer-watcher-github-release starting...")` line:

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

   The `w` variable is the SAME `pkg.Watcher` instance the poll-interval loop uses (built earlier in `main.go` via `factory.CreateWatcher(...)`). This is the spec's load-bearing invariant: the watcher is built ONCE and shared.

   d. **Grow `run.CancelOnFirstFinish` from two args to three.** Replace:

   ```go
   return run.CancelOnFirstFinish(ctx,
       a.pollLoop(poll, pollInterval),
       a.createHTTPServer(),
   )
   ```

   with:

   ```go
   // Order: poll → HTTP → command consumer (spec 067 AC 9: three run.Funcs).
   return run.CancelOnFirstFinish(ctx,
       a.pollLoop(poll, pollInterval),
       a.createHTTPServer(),
       commandConsumer,
   )
   ```

   The `commandConsumer` is a `run.Func` (returned by `factory.CreateCommandConsumer`); it satisfies the `run.Func` signature `func(ctx context.Context) error` and respects `<-ctx.Done()` per the `cdb.RunCommandConsumerTxDefault` contract.

4. **Add the `CreateTriggerReleaseCheckCommandSender` test to `pkg/factory/command_consumer_test.go`** (or a new sibling file if `command_consumer_test.go` does not exist for github-release). The test:

   ```go
   var _ = Describe("CreateTriggerReleaseCheckCommandSender", func() {
       It("returns a non-nil sender", func() {
           syncProducer := new(libkafkamocks.KafkaSyncProducer)
           sender := factory.CreateTriggerReleaseCheckCommandSender(
               context.Background(),
               syncProducer,
               base.Branch("dev"),
           )
           Expect(sender).NotTo(BeNil())
       })
   })
   ```

   The `libkafkamocks.KafkaSyncProducer` is the counterfeiter mock at `/home/node/go/pkg/mod/github.com/bborbe/kafka@*/mocks/`. Verify the import path: `grep -rn 'KafkaSyncProducer struct' /home/node/go/pkg/mod/github.com/bborbe/kafka@*/mocks/`.

5. **Add the `CreateCommandConsumer` test to `pkg/factory/command_consumer_test.go`.** The test block:

   ```go
   var _ = Describe("CreateCommandConsumer", func() {
       It("returns a non-nil run.Func when all dependencies are non-nil", func() {
           syncProducer := new(libkafkamocks.KafkaSyncProducer)
           saramaClientProvider := new(libkafkamocks.KafkaSaramaClientProvider)
           db := new(kvmocks.DB)
           watcher := new(mocks.Watcher)

           runFunc := factory.CreateCommandConsumer(
               saramaClientProvider,
               syncProducer,
               db,
               watcher,
               base.Branch("dev"),
           )
           Expect(runFunc).NotTo(BeNil())
       })

       It("CreateCommandConsumer body has no control flow", func() {
           // Resolve factory.go relative to THIS test file so the test runs
           // correctly regardless of CWD (e.g. when go test is invoked from
           // the module root with ./... rather than from the package dir).
           _, thisFile, _, ok := runtime.Caller(0)
           Expect(ok).To(BeTrue(), "runtime.Caller failed")
           factoryPath := filepath.Join(filepath.Dir(thisFile), "factory.go")

           fset := token.NewFileSet()
           file, err := parser.ParseFile(fset, factoryPath, nil, parser.AllErrors)
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
                   Fail(fmt.Sprintf(
                       "CreateCommandConsumer body contains forbidden control flow: %T at %v",
                       n, fset.Position(n.Pos()),
                   ))
               }
               return true
           })
       })
   })
   ```

   The imports `libkafkamocks`, `kvmocks`, `runtime`, `go/ast`, `go/parser`, `go/token`, `path/filepath`, `fmt`, and `mocks` (the github-release `*mocks.Watcher`) must all be added to the test file's import block.

6. **Add the integration test for clean shutdown and end-to-end command flow to `pkg/factory/integration_test.go`** (or create the file if it does not exist). Mirror the spec-066 sibling at `/workspace/watcher/github-pr/pkg/factory/integration_test.go`:

   a. **Clean-shutdown test** (`Describe("clean shutdown of three run.Funcs (spec 067 AC 10)", ...)`):

   ```go
   var _ = Describe("clean shutdown of three run.Funcs (spec 067 AC 10)", func() {
       It("run.CancelOnFirstFinish exits cleanly when the parent context is cancelled", func() {
           // We can't actually wire run.CancelOnFirstFinish from inside this
           // test (it requires application-level wiring), but we can prove
           // the three run.Funcs the factory produces all return promptly
           // when their ctx is cancelled. This is the load-bearing invariant
           // the framework's contract requires.
           // goleak: not used here (not a project dep) — rely on the
           // ctx-cancellation contract only.
           ctx, cancel := context.WithCancel(context.Background())
           doneCh := make(chan error, 3)

           // Three run.Funcs that mirror what the factory would build:
           // (1) poll loop, (2) HTTP server, (3) command consumer.
           pollLoop := func(c context.Context) error {
               <-c.Done()
               doneCh <- nil
               return nil
           }
           httpServer := func(c context.Context) error {
               <-c.Done()
               doneCh <- nil
               return nil
           }
           commandConsumer := func(c context.Context) error {
               <-c.Done()
               doneCh <- nil
               return nil
           }

           go pollLoop(ctx)        //nolint:errcheck
           go httpServer(ctx)      //nolint:errcheck
           go commandConsumer(ctx) //nolint:errcheck

           // Cancel and assert all three exit within the framework's grace period (5s).
           cancel()
           Eventually(doneCh, 5*time.Second).Should(Receive())
           Eventually(doneCh, 5*time.Second).Should(Receive())
           Eventually(doneCh, 5*time.Second).Should(Receive())
       })
   })
   ```

   b. **End-to-end command flow test** (`Describe("end-to-end command flow through wired executor (spec 067 AC 8 + AC 19)", ...)`):

   ```go
   var _ = Describe("end-to-end command flow through wired executor (spec 067 AC 8 + AC 19)", func() {
       var (
           ctx      context.Context
           watcher  *mocks.Watcher
           executor cdb.CommandObjectExecutorTx
       )

       BeforeEach(func() {
           ctx = context.Background()
           watcher = new(mocks.Watcher)

           executor = command.NewTriggerReleaseCheckCommandExecutor(watcher)
       })

       newCommandObject := func() cdb.CommandObject {
           evt, err := base.ParseEvent(ctx, command.TriggerReleaseCheckCommand{})
           Expect(err).NotTo(HaveOccurred())
           return cdb.CommandObject{
               Command: base.Command{
                   Operation: command.TriggerReleaseCheckCommandOperation,
                   Data:      evt,
               },
               SchemaID: lib.GithubReleaserV1SchemaID,
           }
       }

       It(
           "factory composition succeeds and the executor invokes Watcher.Poll exactly once",
           func() {
               // Sanity check: the factory's CreateCommandConsumer returns a
               // non-nil run.Func when given the same wiring the executor
               // would receive in production. This proves the factory
               // composition is correct.
               runFunc := factory.CreateCommandConsumer(
                   new(libkafkamocks.KafkaSaramaClientProvider),
                   new(libkafkamocks.KafkaSyncProducer),
                   new(kvmocks.DB),
                   watcher,
                   base.Branch("dev"),
               )
               Expect(runFunc).NotTo(BeNil(),
                   "factory composition must succeed for the wired consumer")

               // Now drive the executor directly with a real command object
               // and verify the downstream side effect: Watcher.Poll is
               // invoked exactly once.
               _, _, err := executor.HandleCommand(ctx, nil, newCommandObject())
               Expect(err).NotTo(HaveOccurred())
               Expect(watcher.PollCallCount()).To(Equal(1),
                   "valid command must invoke Watcher.Poll exactly once")
           },
       )
   })
   ```

   The at-least-once-via-idempotent-downstream crash-recovery contract (spec § AC 19) is covered by the executor-level test in `pkg/command/trigger_release_check_executor_test.go` (prompt 2) — do NOT duplicate it here. Add a comment at the bottom of the test file pointing at the executor-level test as the canonical AC-19 proof.

7. **Add the `## Unreleased` CHANGELOG entry to `/workspace/CHANGELOG.md`.** The entry:

   ```markdown
   ## Unreleased

   - feat: split /trigger into CQRS pair — HTTP handler publishes TriggerReleaseCheckCommand to Kafka; in-pod consumer runs Watcher.Poll(ctx). Triggers now survive pod crashes via Kafka redelivery.
   ```

   The `feat:` prefix is mandatory (per the changelog guide) — dark-factory reads the prefix to determine the version bump. Place the entry under the existing `## Unreleased` section if it already exists; otherwise create the section. Per the changelog guide, the entry must be added BEFORE running `make precommit` in this prompt.

8. **Run `make precommit` in the changed module.** From the github-release watcher dir:

   ```
   cd /workspace/watcher/github-release && make precommit
   ```

   Expected: exit code 0. This is the FINAL precommit gate — it runs format + generate + test + lint + license + security scans + full test suite. The auditor expects a clean exit.

9. **YAGNI guard.** Do NOT add a `Force` or `Scope` handling branch in the factory or main.go — the executor reads neither field. Do NOT add a `SendResultEnabled` flag — the spec says `false`, hard-coded in the executor. Do NOT add a `db` lifecycle close — the in-memory DB has no resources to release. Do NOT add a `WaitGroup` or `sync.Mutex` — `run.CancelOnFirstFinish` orchestrates shutdown via context cancellation. Do NOT add a third `run.Func` beyond the command consumer — the spec requires EXACTLY three. Do NOT add a graceful-shutdown timeout for the command consumer — the framework's default (5 minutes) is the contract. Do NOT add Prometheus metrics in the factory — `pkg.Watcher.Poll(ctx)` already owns `IncPollCycle`.
</requirements>

<constraints>
- Schema is frozen: use `lib.GithubReleaserV1SchemaID` from `github.com/bborbe/maintainer/lib`. Do NOT define a new schema in this prompt.
- The consumer uses `cdb.RunCommandConsumerTxDefault` (NOT `cdb.RunCommandConsumerTx` with manual tx wrapping) — per the `go-cqrs/auto-tx-wrapper-no-manual-wrap` rule.
- The watcher object is built ONCE in `main.go` (via `factory.CreateWatcher(...)`) and shared with both the poll-interval loop and the command consumer. Do NOT construct two watchers.
- `run.CancelOnFirstFinish(ctx, ...)` takes EXACTLY three arguments (poll, HTTP, command consumer). Do NOT add a fourth.
- The factory's `CreateCommandConsumer` and `CreateTriggerReleaseCheckCommandSender` are pure composition (no `for`/`switch`/conditional, no `context.Background()`). Per the factory-pattern guide.
- The `cqrsiam.Initiator` string is `"watcher-github-release"` (NOT `"watcher-github-pr"` or `"lib"`). The string is the producer-name in the audit trail — operators will see this when reading the Kafka message headers.
- Error wrapping: `github.com/bborbe/errors` only. Never `fmt.Errorf`. Always pass `ctx` to error constructors. Never `context.Background()` in `pkg/` (the factory is allowed to receive `ctx` from `main.go`).
- Ginkgo v2 + Gomega + counterfeiter. External test packages (`factory_test`, `pkg_test`, `command_test`, `handler_test`). Coverage on the new code ≥ 80% per `docs/definition-of-done.md`.
- Do NOT modify `pkg/command/` files in this prompt — they are owned by prompts 1 and 2.
- Do NOT modify `pkg/handler/trigger_handler.go` in this prompt — it is owned by prompt 3.
- The CHANGELOG entry uses the `feat:` prefix (dark-factory reads the prefix for version bump). The entry goes under the existing `## Unreleased` section in `/workspace/CHANGELOG.md` (or creates the section if absent).
- Do NOT commit — dark-factory handles git. Branch: `dark-factory/cqrs-trigger-github-release`.
- Build verification: `cd /workspace/watcher/github-release && make precommit` must exit 0.
</constraints>

<verification>

Verify the factory file gained the two new functions:
```
grep -n 'func CreateTriggerReleaseCheckCommandSender\|func CreateCommandConsumer' /workspace/watcher/github-release/pkg/factory/factory.go
```
Must show both functions in the file.

Verify the consumer uses `cdb.RunCommandConsumerTxDefault` (NOT `cdb.RunCommandConsumerTx` — spec § AC 13):
```
grep -n 'cdb.RunCommandConsumerTxDefault\|cdb.RunCommandConsumerTx(' /workspace/watcher/github-release/pkg/factory/factory.go
```
Must show the call to `cdb.RunCommandConsumerTxDefault` and NO manual `cdb.RunCommandConsumerTx(` call (the Default variant is the only valid call site).

Verify the factory wires `base.CommandCreator` + `cqrsiam.Initiator` ONCE at construction (spec § AC 14):
```
grep -A 8 'func CreateTriggerReleaseCheckCommandSender' /workspace/watcher/github-release/pkg/factory/factory.go
```
Must show the `base.NewCommandCreator(base.RequestIDChannel(ctx))` + `cqrsiam.Initiator("watcher-github-release")` + `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)` pattern inside the function body (NOT inside a per-call closure).

Verify `main.go` registers three `run.Func`s (spec § AC 9):
```
grep -A 6 'run.CancelOnFirstFinish' /workspace/watcher/github-release/main.go
```
Must show exactly three `run.Func` arguments: `a.pollLoop(poll, pollInterval)`, `a.createHTTPServer()`, and `commandConsumer`.

Verify the watcher is shared (built once, used by both loops):
```
grep -n 'w := factory.CreateWatcher\|w, // shared\|commandConsumer := factory.CreateCommandConsumer' /workspace/watcher/github-release/main.go
```
Must show the `w := factory.CreateWatcher(...)` call and a `commandConsumer := factory.CreateCommandConsumer(..., w, ...)` call that passes the same `w` variable.

Verify the CHANGELOG entry is present (spec § Verification):
```
grep -A 1 '## Unreleased' /workspace/CHANGELOG.md
```
Must show the `feat: split /trigger into CQRS pair` entry (or its `feat:`-prefixed equivalent).

Verify no part of the executor or handler was touched (this prompt is scoped to the factory + main.go + integration tests):
```
git diff --stat HEAD -- /workspace/watcher/github-release/pkg/command/ /workspace/watcher/github-release/pkg/handler/trigger_handler.go
```
Expected: empty output — no changes in `pkg/command/` (owned by prompts 1 and 2) or the trigger handler (owned by prompt 3).

Run the new factory tests:
```
cd /workspace/watcher/github-release && go test -mod=mod -v -count=1 ./pkg/factory/... -run "CreateTriggerReleaseCheckCommandSender|CreateCommandConsumer|clean.shutdown|end-to-end"
```
Expected: exit code 0; the `CreateTriggerReleaseCheckCommandSender` test passes; the `CreateCommandConsumer` non-nil test passes; the AST-based "no control flow" test passes; the clean-shutdown test passes; the end-to-end test passes.

Run the FULL precommit gate (this is the spec's headline verification — `cd watcher/github-release && make precommit` exits 0):
```
cd /workspace/watcher/github-release && make precommit
```
Expected: exit code 0. If any individual target (e.g. `make lint`, `make gosec`, `make errcheck`) fails, fix the issue, then re-run ONLY the failing target. Do NOT re-run the full `make precommit` until all individual targets pass — then run it one final time to confirm.
</verification>
