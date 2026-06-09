---
status: approved
created: "2026-06-09T16:00:00Z"
queued: "2026-06-09T16:21:18Z"
branch: dark-factory/cqrs-trigger-github-build
---

# Spec 068 Prompt 5 — Factory + main.go Wiring (Final)

## Context

This is prompt 5 of 5 for spec 068. It is the wiring prompt: it adds `factory.CreateCommandConsumer` and `factory.CreateTriggerBuildCheckCommandSender` + `factory.CreateTriggerBuildCheckHandler` in `watcher/github-build/pkg/factory/`, and rewrites `main.go` to drop the legacy `chan struct{}` trigger plumbing, drop the `trigger` parameter from `runPollLoop`, and wire a third `run.Func` for the command consumer alongside the poll-interval loop and the HTTP server.

**Depends on prompts 3 AND 4 having landed.** Specifically:

- `command.NewTriggerBuildCheckCommandExecutor` exists (prompt 3).
- `command.NewTriggerBuildCheckCommandSender` exists (prompt 2).
- `handler.NewTriggerBuildCheckHandler` exists (prompt 4).
- `pkg.NewMemDB` exists (prompt 2).
- `lib.GithubBuildV1SchemaID` exists (prompt 1).

This is the prompt where every prior piece snaps together into the production wiring.

**Mirror line-for-line** the spec 067 github-release implementation, which lives at:

- `/workspace/watcher/github-release/pkg/factory/factory.go` — `CreateTriggerReleaseCheckCommandSender`, `CreateTriggerReleaseCheckHandler`, `CreateCommandConsumer`.
- `/workspace/watcher/github-release/pkg/factory/command_consumer_test.go` — `CreateCommandConsumer` non-nil + AST control-flow assertion.
- `/workspace/watcher/github-release/pkg/factory/integration_test.go` — clean-shutdown test for the three `run.Func`s.
- `/workspace/watcher/github-release/main.go` — three `run.Func`s under `run.CancelOnFirstFinish`.

Only the type/constant names change. The factory's pure-composition rule, the AST control-flow assertion, and the clean-shutdown test pattern are all verbatim.

## Goal

- `factory.CreateTriggerBuildCheckCommandSender(ctx, syncProducer, branch) command.TriggerBuildCheckCommandSender` — wires `base.NewCommandCreator(base.RequestIDChannel(ctx))` + `cqrsiam.Initiator("watcher-github-build")` + `cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)` once at construction.
- `factory.CreateTriggerBuildCheckHandler(sender command.TriggerBuildCheckCommandSender) handler.TriggerBuildCheckHandler` — thin constructor pass-through.
- `factory.CreateCommandConsumer(saramaClientProvider, syncProducer, db, watcher, branch) run.Func` — pure composition, uses `cdb.RunCommandConsumerTxDefault` (NOT `kv.NewTransactionMiddleware`), passes `lib.GithubBuildV1SchemaID` and `cdb.CommandObjectExecutorTxs{command.NewTriggerBuildCheckCommandExecutor(watcher)}`.
- `main.go` rewrites:
  - Drop `triggerBufferSize` constant.
  - Drop the `make(chan struct{}, triggerBufferSize)` channel allocation.
  - Drop the `trigger <-chan struct{}` parameter from `runPollLoop` — new signature is `runPollLoop(poll run.Func, interval time.Duration) run.Func`. The select inside drops the `<-trigger` case.
  - Wire three `run.Func`s inside `run.CancelOnFirstFinish`: poll-interval loop, HTTP server, command consumer.
  - Share the watcher instance `w` with the consumer (the same `w` the poll loop already uses).
  - Build the sender via `factory.CreateTriggerBuildCheckCommandSender(ctx, syncProducer, branch)`, build the handler, assign to `a.TriggerHandler` wrapped in `libhttp.NewJSONErrorHandler(...)`.
- `main_poll_loop_test.go` updates:
  - First test (the "continues loop when poll returns an error" test) drops the third `make(chan struct{})` argument.
  - **Second test ("runs a poll when signalled via the trigger channel") is DELETED** — the trigger channel is gone. The repo no longer has any signal-driven poll path inside the pod; triggers come through Kafka.

  <!-- AUDIT-OPEN: main_poll_loop_test.go second test. The spec's Suggested Decomposition table says "AST control-flow assertion on `CreateCommandConsumer` body" and "clean-shutdown test" for prompt 5. The test file `main_poll_loop_test.go` is in the spec's reference list (item 23) as "WILL BREAK with the change". Deleting the trigger-channel test is the natural outcome, but a stricter reading might keep the test file as a "continues the loop when poll returns an error" test only — i.e. delete the second `It` block. The prompt body takes the deletion path; if audit disagrees, the fallback is to add a new `It("exits cleanly on ctx cancel", ...)` that uses the new `runPollLoop(poll, interval)` signature. -->

## Files to modify

- `/workspace/watcher/github-build/pkg/factory/factory.go` — add `CreateTriggerBuildCheckCommandSender`, `CreateTriggerBuildCheckHandler`, `CreateCommandConsumer`. Mirror github-release factory.go lines 85-142. Import `libkv "github.com/bborbe/kv"`, `lib "github.com/bborbe/maintainer/lib"`, `libkafka "github.com/bborbe/kafka"`, `cdb "github.com/bborbe/cqrs/cdb"`, `base "github.com/bborbe/cqrs/base"`, `cqrsiam "github.com/bborbe/cqrs/iam"`, `command "github.com/bborbe/maintainer/watcher/github-build/pkg/command"`, `handler "github.com/bborbe/maintainer/watcher/github-build/pkg/handler"`. The existing `CreateWatcher` and `CreateKafkaCreateSender` from prompts 4's refactor stay — main.go continues to call them, but the new `CreateTriggerBuildCheckCommandSender` accepts a pre-built sync producer (reused, not re-created).
- `/workspace/watcher/github-build/pkg/factory/command_consumer_test.go` — copy of github-release's. Substitutions:
  - Import: `github.com/bborbe/maintainer/watcher/github-build/mocks`.
  - The `CreateCommandConsumer` body AST test: assert (a) zero `if/for/range/switch` statements in the body, (b) the body contains a call to `cdb.RunCommandConsumerTxDefault`, (c) the identifier `GithubBuildV1SchemaID` appears in the call's argument list. (Mirror the github-release test exactly — it already does all three.)
  - The "returns a non-nil run.Func when all dependencies are non-nil" test: pass `*mocks.Watcher` (the github-build watcher mock, which has the same Poll-only shape).
  - The `CreateTriggerBuildCheckCommandSender` non-nil test mirrors the github-release one.
- `/workspace/watcher/github-build/pkg/factory/integration_test.go` — copy of github-release's, with substitutions for `TriggerBuildCheck*` types and `lib.GithubBuildV1SchemaID`. The crash-recovery test (the second Describe block) is NOT duplicated here — the spec 068 audit explicitly says "single source of truth at the executor layer". Replace it with a comment block pointing at `pkg/command/trigger_build_check_executor_test.go` for the crash-recovery AC 26.
- `/workspace/watcher/github-build/main.go` — full rewrite of the `Run` method and the helpers:
  - Delete `triggerBufferSize` constant.
  - Delete `trigger := make(chan struct{}, triggerBufferSize)`.
  - `runPollLoop` signature becomes `func (a *application) runPollLoop(poll run.Func, interval time.Duration) run.Func`. The select has only `ctx.Done()` and `ticker.C` cases.
  - `createHTTPServer` no longer takes a `chan<- struct{}` parameter. It mounts `a.TriggerHandler` on `/trigger` (matching github-release main.go line 164).
  - `application` gets a `TriggerHandler http.Handler` field.
  - The `Run` method ends with `return run.CancelOnFirstFinish(ctx, a.runPollLoop(pollOnce, pollInterval), a.createHTTPServer(), commandConsumer)`. If `refreshTask != nil`, append it after the consumer (mirror github-release's `if refreshTask != nil { tasks = append(tasks, refreshTask) }` pattern adapted for the `run.CancelOnFirstFinish` variadic).
  - The sync producer is now reused: build it once in main.go and pass it to both `factory.CreateKafkaCreateSender` (or its refactored form accepting a pre-built producer) and `factory.CreateTriggerBuildCheckCommandSender`.
- `/workspace/watcher/github-build/main_poll_loop_test.go` — update:
  - First test: `loopFunc := app.runPollLoop(pollFunc, 10*time.Millisecond)` (drop the third arg).
  - Second test ("runs a poll when signalled via the trigger channel"): delete the entire `It("runs a poll when signalled via the trigger channel", ...)` block. The trigger channel does not exist anymore; this test has no replacement at the main package layer.
  - Add a new `It("exits cleanly when context is cancelled", ...)` test that calls `loopFunc(ctx)`, cancels ctx, and asserts the function returns within 500ms (i.e. proves the new `runPollLoop(poll, interval)` signature handles ctx cancellation correctly).
- `/workspace/watcher/github-build/main_test.go` — may not exist (the github-build module uses a `main_test.go` for the compile check per github-release precedent). If it exists, no edit. If it doesn't, do not add one in this prompt (out of scope).

## Out of scope

- Do NOT change the executor (prompt 3), the sender (prompt 2), or the handler (prompt 4). The factory wires their constructors; it does not redefine them.
- Do NOT change the `pkg.Watcher` interface, its mock, or the `Watcher.Poll(ctx)` semantics.
- Do NOT add metric increments in the factory. The executor and watcher already own their metrics.
- Do NOT add per-repo filter UX. The Scope field is reserved-unread.
- Do NOT change the Force flag behavior (plumbed but unused).
- Do NOT enable `SendResultEnabled` (kept `false` per spec Non-goal).
- Do NOT change the HTTP mount path. The route stays `/trigger` under `/admin`.
- Do NOT change the k8s manifests. The CRD/secret/PVC shape stays the same.

## Implementation

1. Read `/workspace/watcher/github-release/pkg/factory/factory.go` fully. The `CreateCommandConsumer` body is the load-bearing AST target. It MUST be:
   - Pure composition: no `if x == nil { panic(…) }` (spec 067 lesson 2). The github-release version is already correct; mirror it.
   - One call to `cdb.RunCommandConsumerTxDefault` with the schema ID in the argument list.
   - Zero `if/for/range/switch` statements (the AST test in `command_consumer_test.go` enforces this).

2. In the github-build factory:
   - `CreateTriggerBuildCheckCommandSender` mirrors github-release's signature but takes the `syncProducer` (already built in main.go) instead of building it internally.
   - `CreateTriggerBuildCheckHandler` is a thin pass-through: `return handler.NewTriggerBuildCheckHandler(sender)`.
   - `CreateCommandConsumer` wraps `command.NewTriggerBuildCheckCommandExecutor(watcher)` in a `cdb.CommandObjectExecutorTxs{...}` and passes that into `cdb.RunCommandConsumerTxDefault` with `lib.GithubBuildV1SchemaID`. `ignoreUnsupported` is `false` (matching the github-release value).

3. `main.go` rewrites the `Run` method. The structure follows github-release's main.go lines 86-152:

   ```go
   syncProducer, err := libkafka.NewSyncProducerWithName(
       ctx, a.KafkaBrokers, "maintainer-watcher-github-build",
   )
   if err != nil {
       return errors.Wrap(ctx, err, "create sync producer")
   }
   defer func() {
       if cerr := syncProducer.Close(); cerr != nil {
           glog.Warningf("close kafka sync producer: %v", cerr)
       }
   }()

   branch := base.Branch(a.Stage)

   // Build the create-task sender (existing path).
   createSender, _, err := factory.CreateKafkaCreateSenderFromProducer(
       syncProducer, branch,
   )
   if err != nil {
       return errors.Wrap(ctx, err, "create kafka create sender")
   }

   // ... (build the watcher, see existing CreateWatcher wiring)

   // Build the trigger-build-check sender + handler.
   triggerSender := factory.CreateTriggerBuildCheckCommandSender(ctx, syncProducer, branch)
   a.TriggerHandler = libhttp.NewJSONErrorHandler(
       factory.CreateTriggerBuildCheckHandler(triggerSender),
   )

   // Build the in-pod command consumer (third run.Func).
   saramaClientProvider := libkafka.NewSaramaClientProviderNew(a.KafkaBrokers)
   db := pkg.NewMemDB()
   commandConsumer := factory.CreateCommandConsumer(
       saramaClientProvider,
       syncProducer,
       db,
       w, // shared with the poll-interval loop
       branch,
   )

   // Wire the three (or four with refreshTask) run.Funcs.
   pollOnce := a.pollOnce(w)
   tasks := []run.Func{
       a.runPollLoop(pollOnce, pollInterval),
       a.createHTTPServer(),
       commandConsumer,
   }
   if refreshTask != nil {
       tasks = append(tasks, refreshTask)
   }
   return run.CancelOnFirstFinish(ctx, tasks...)
   ```

   Note: the create-task sender refactor from prompt 4 may have already changed `CreateKafkaCreateSender` to take a pre-built `syncProducer`. If not, this prompt includes that refactor. **The end state is: main.go owns the sync producer lifecycle (matching github-release).**

4. `runPollLoop` body (after the rewrite):
   ```go
   func (a *application) runPollLoop(
       poll run.Func,
       interval time.Duration,
   ) run.Func {
       return func(ctx context.Context) error {
           ticker := time.NewTicker(interval)
           defer ticker.Stop()
           if err := poll(ctx); err != nil {
               glog.Errorf("initial poll: %v", err)
           }
           for {
               select {
               case <-ctx.Done():
                   glog.V(2).Infof("poll loop: context cancelled, exiting cleanly")
                   return nil
               case <-ticker.C:
                   if err := poll(ctx); err != nil {
                       glog.Errorf("poll cycle error: %v", err)
                   }
               }
           }
       }
   }
   ```

5. `createHTTPServer` body:
   ```go
   func (a *application) createHTTPServer() run.Func {
       return func(ctx context.Context) error {
           router := mux.NewRouter()
           router.Path("/healthz").Handler(libhttp.NewPrintHandler("OK"))
           router.Path("/readiness").Handler(libhttp.NewPrintHandler("OK"))
           router.Path("/metrics").Handler(promhttp.Handler())
           router.Path("/setloglevel/{level}").
               Handler(log.NewSetLoglevelHandler(ctx, log.NewLogLevelSetter(2, 5*time.Minute)))
           router.Path("/resetcursor/{repo:.+}").
               Handler(libhttp.NewDangerousHandlerWrapper(pkg.NewResetCursorHandler(pkg.DefaultCursorPath)))
           router.Path("/trigger").Handler(a.TriggerHandler)
           glog.V(2).Infof("http server listening on %s", a.Listen)
           return libhttp.NewServer(a.Listen, router).Run(ctx)
       }
   }
   ```

6. The `TriggerHandler http.Handler` field on `application` is a struct field, not a config flag. Default-initialize it to `nil` (the assignment in `Run` sets it before the HTTP server starts). If `application` is constructed without `Run` being called, the field is nil — that's fine, the test code paths that build the application directly (like the integration test) don't call `createHTTPServer`.

## Tests

`pkg/factory/command_consumer_test.go` covers:

- "CreateTriggerBuildCheckCommandSender returns a non-nil sender": call the factory with a fake sync producer + `base.Branch("dev")`, assert non-nil.
- "CreateCommandConsumer returns a non-nil run.Func when all dependencies are non-nil": pass fake sarama client provider, fake sync producer, fake `libkv.DB`, fake `*mocks.Watcher`, `base.Branch("dev")`. Assert non-nil.
- **AST control-flow assertion on `CreateCommandConsumer` body**:
  - Resolve factory.go path via `runtime.Caller(0)` (NO hardcoded path).
  - Parse the file, find the `CreateCommandConsumer` `*ast.FuncDecl`.
  - `ast.Inspect` the body for `*ast.IfStmt | *ast.ForStmt | *ast.RangeStmt | *ast.SwitchStmt | *ast.TypeSwitchStmt` — assert ZERO matches (Fail on any).
  - Bonus: assert the body contains a call to `cdb.RunCommandConsumerTxDefault` (find an `*ast.CallExpr` whose `Fun` resolves to that selector).
  - Bonus: assert the identifier `GithubBuildV1SchemaID` appears somewhere in the body.

`pkg/factory/integration_test.go` covers:

- "clean shutdown of three run.Funcs (spec 068 AC 15)": start three goroutines with run.Func closures that wait on `<-ctx.Done()`. Cancel ctx. Assert all three exit within 5s. (This is the goleak-equivalent: proves the three run.Funcs respond to ctx cancellation without leaking.)
- "factory composition succeeds and the executor invokes Watcher.Poll exactly once": build the `run.Func` via `factory.CreateCommandConsumer(...)` with mocks, then drive the executor directly with a real `cdb.CommandObject` and assert the watcher's `PollCallCount() == 1`.
- A trailing comment block explicitly noting that the crash-recovery test (AC 26) is NOT duplicated here — it lives at `pkg/command/trigger_build_check_executor_test.go`.

`main_poll_loop_test.go` covers:

- (Modified) "continues the loop when poll returns an error" — drops the third arg to `runPollLoop`.
- (New) "exits cleanly when context is cancelled" — uses the new signature, cancels ctx, asserts the function returns within 500ms.
- (Deleted) "runs a poll when signalled via the trigger channel" — this test's path no longer exists.

## Verification

```
cd /workspace/watcher/github-build && make precommit
echo "exit=$?"
grep -n 'triggerBufferSize\|make(chan struct{}' /workspace/watcher/github-build/main.go
echo "expect_zero_lines_above"
grep -n 'CreateCommandConsumer' /workspace/watcher/github-build/main.go
echo "expect_at_least_one_line"
grep -n 'RunCommandConsumerTxDefault' /workspace/watcher/github-build/pkg/factory/factory.go
echo "expect_at_least_one_line"
grep -n 'GithubBuildV1SchemaID' /workspace/watcher/github-build/pkg/factory/factory.go
echo "expect_at_least_two_lines"
ls /workspace/watcher/github-build/pkg/trigger_handler.go
echo "expect_no_such_file"
ls /workspace/watcher/github-build/pkg/handler/trigger_handler.go
echo "expect_such_file"
```

The first two grep commands must return zero lines (legacy plumbing removed). The next three must return ≥ 1 line each. The last two `ls` calls confirm the handler file is at the new location and the old location is gone.

## Lessons from spec 067 audit (apply at write time)

1. Factory is pure composition (lesson 2): no `if x == nil { panic }` guards in `CreateCommandConsumer` or any of the new factory functions. Sibling factories don't nil-check; NPE-on-misuse is the codebase style.
2. Use `cdb.RunCommandConsumerTxDefault` (lesson 4 — auto-wraps the transaction). NEVER manual `RunCommandConsumerTx` + `kv.NewTransactionMiddleware`. The grep at the end of the Verification section enforces this — `RunCommandConsumerTxDefault` must appear, `kv.NewTransactionMiddleware` must NOT.
3. Counterfeiter directive ABOVE the type declaration (lesson 6). The mock for `TriggerBuildCheckCommandSender` and the mock for `TriggerBuildCheckHandler` are generated by `go generate` in earlier prompts; this prompt does not touch them.
4. The AST control-flow assertion (in `command_consumer_test.go`) is the load-bearing test for the factory's pure-composition rule. Per the spec audit, the AST approach is brittle if a future refactor builds the run.Func slice via a helper — but since the spec explicitly requires the AST check, keep it. Do not weaken to "just check non-nil run.Func".
5. `glog.V(2).Infof` for the success log line on `/trigger` accept; `glog.Errorf` for the initial-poll error (matches github-release main.go line 180).
6. The `createHTTPServer` no longer takes `chan<- struct{}` (per spec 068). The `TriggerHandler http.Handler` field on `application` is the seam. Mirror github-release main.go's pattern exactly.
7. The `runPollLoop` body: fire one initial poll (best-effort, log on error), then select on `ctx.Done()` and `ticker.C`. NO `<-trigger` case. Mirror github-release main.go lines 174-193.
8. The `createHTTPServer` mounts `a.TriggerHandler` on `/trigger` (matches github-release main.go line 164). All other admin endpoints (`/resetcursor`, `/setloglevel`, `/healthz`, `/readiness`, `/metrics`) are unchanged.
9. The `RefreshTask` (from `factory.CreateAllowlistSnapshot`) appends to the tasks slice AFTER the consumer (matching github-release's order: poll → HTTP → consumer → refresh). Operators see consistent log ordering.
10. BSD copyright header on every new/modified file. The factory's `factory.go` already has one; verify it's intact.

## Improvements

(empty — YOLO fills in after running)
