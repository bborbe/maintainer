---
status: pending
spec: ["069"]
created: "2026-06-09T16:20:25Z"
branch: dark-factory/cqrs-trigger-github-release
---

<summary>
- Adds `pkg/command/trigger_release_check_executor.go` that consumes `TriggerReleaseCheckCommand` messages from the in-pod Kafka topic and invokes the shared `pkg.Watcher.Poll(ctx)` on the same watcher instance the poll-interval loop uses.
- The executor uses the project's standard `cdb.CommandObjectExecutorTxFunc(...)` shape; `SendResultEnabled` is `false` (spec Non-goal: fire-and-forget, no result topic).
- Exit paths follow `go-cqrs.md` rules strictly: malformed payload (MarshalInto fails) and `cmd.Validate(ctx)` failure → `cdb.ErrCommandObjectSkipped` (non-retryable, deliberate). `w.Poll(ctx)` returning a non-nil error → wrapped error (transient, framework emits Failure on the result topic, Kafka redelivers). `w.Poll(ctx)` returning nil → `nil, nil, nil` (success).
- The executor does NOT increment any metrics — `pkg.Watcher.Poll(ctx)` already owns `IncPollCycle` / `IncPublished` / `IncReposScanned` / `IncFilterSkipped` (spec § Desired Behavior: "executor reads neither Scope nor Force — both are reserved-unread").
- Table-driven tests cover all 3 exit-path branches (valid, malformed, watcher-returns-error). A dedicated crash-recovery test simulates a pod kill mid-`w.Poll(ctx)`: cancel the consumer's context during Poll, verify the executor returns a wrapped error (NOT `ErrCommandObjectSkipped`), then re-run on a fresh context and a fresh watcher, verify the second invocation succeeds and `PollCallCount == 1` on the fresh watcher (proving at-least-once-via-Kafka-redelivery).

This is prompt 2 of 4 for spec 067. It depends on prompt 1 (the command + sender types). Prompt 4 (consumer wiring) depends on this prompt.
</summary>

<objective>
Build the executor that performs the actual github-release trigger work on the consumer side. After this prompt, a `TriggerReleaseCheckCommand` message on the request topic is processed end-to-end: the shared `pkg.Watcher.Poll(ctx)` runs once. The HTTP handler (prompt 3) is reduced to publish+202, and the consumer (prompt 4) is wired as the third `run.Func`.

The critical correctness invariants:
1. The executor invokes `pkg.Watcher.Poll(ctx)` EXACTLY ONCE per valid command (spec § AC 8). Skipped payloads (malformed / Validate-fail) MUST NOT call `Poll`.
2. The executor must NOT use the `return nil, nil, nil` "idempotent skip" anti-pattern. Malformed and Validate-fail MUST be `cdb.ErrCommandObjectSkipped` (spec § Desired Behavior 4).
3. `w.Poll(ctx)` returning a non-nil error MUST be wrapped (not `ErrCommandObjectSkipped`) so Kafka redelivers. The executor's metric ownership is unchanged — the watcher's own `IncPollCycle` increments are the single source of truth.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these source files in full BEFORE editing:

- `/workspace/watcher/github-release/pkg/watcher.go` — the existing `Watcher` interface and `Watcher.Poll(ctx)` method. The executor calls ONLY `pkg.Watcher.Poll(ctx)` — that single method encapsulates all scan-cycle work (changelog fetch, filter, task publish, cursor save). Do not call any private helpers; do not duplicate the scan logic. Anchor by symbol name: `grep -n 'func.*Poll(ctx' /workspace/watcher/github-release/pkg/watcher.go`.
- `/workspace/watcher/github-release/pkg/metrics.go` — the `Metrics` interface and the `poll_cycle` counter. **The executor does NOT increment any metric** — `pkg.Watcher.Poll(ctx)` already calls `metrics.IncPollCycle(...)` with labels `success` / `rate_limited` / `github_error`. Do not duplicate that ownership.
- `/workspace/watcher/github-release/pkg/cursor.go` — the `Cursor` reader/writer. The executor does NOT touch the cursor directly — `pkg.Watcher.Poll(ctx)` owns the cursor lifecycle.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor.go` (the spec-066 reference) — the EXACT structural shape the new executor mirrors: `cdb.CommandObjectExecutorTxFunc(operation, false, func(ctx, tx, commandObject) (*base.EventID, base.Event, error) { return runTriggerReleaseCheck(...) })` constructor; `runTriggerReleaseCheck` split into a private helper; `unmarshalAndValidate` shared helper that returns `cdb.ErrCommandObjectSkipped` for both MarshalInto and Validate failures; the `*_export_test.go` re-export pattern. The new executor is structurally identical but with a much simpler body: instead of GitHub fetch + filter + trust + downstream publish, the body is one line — `watcher.Poll(ctx)`.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor_test.go` (the spec-066 reference) — the EXACT test layout the new executor mirrors: `outcome` enum (`outcomeSuccess` / `outcomeSkipped` / `outcomeWrappedErr`); `mustParseEvent` test helper; `newCommandObject` test helper; table-driven `DescribeTable("exit-path mapping", ...)`; `Describe("executor crash recovery (spec 066 AC 16)", ...)` block. The new test layout is structurally identical but with simpler table entries (no filter, no trust, no downstream publish) and a simpler crash-recovery round (just the executor's Poll + retry).
- `/workspace/prompts/spec-067-cqrs-trigger-github-release-command.md` (the prompt 1 output) — the `pkg/command` package with `TriggerReleaseCheckCommand`, `TriggerReleaseCheckCommandSender`, `TriggerReleaseCheckCommandOperation`, and the counterfeiter mock. Verify the package exists before editing: `ls /workspace/watcher/github-release/pkg/command/`. If empty, STOP and report `status: failed` with message "prompt 1 of spec 067 has not shipped".

Reference the canonical executor pattern in the agent project:

- `/home/node/go/pkg/mod/github.com/bborbe/cqrs@v0.5.1/cdb/cdb_command-object-executor-tx-func.go` — the `cdb.CommandObjectExecutorTxFunc` factory and the `HandleCommandFunc` signature: `func(ctx, tx, commandObject) (*base.EventID, base.Event, error)`. The executor's closure MUST match this signature exactly. `tx` is unused for the trigger executor (no kv writes) but the parameter is mandatory.
- `/home/node/go/pkg/mod/github.com/bborbe/cqrs@v0.5.1/cdb/cdb_command-object-executor-result-sender.go` — the `cdb.ErrCommandObjectSkipped` sentinel at line 19. The spec says: "wrap with `ErrCommandObjectSkipped`, not the bare sentinel" — use `errors.Wrapf(ctx, cdb.ErrCommandObjectSkipped, "...")` to attach context. The framework's `NewCommandObjectExecutorTxResultSender` recognizes the wrapped error via `errors.Is` (see `cdb_command-object-executor-tx-result-sender.go` line 37: `if resultErr != nil && errors.Is(resultErr, CommandObjectSkippedError) {`).

Coding plugin docs (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — load-bearing. Read the entire file. The "Skipping Invalid Commands" section and the two RULE blocks (`go-cqrs/auto-tx-wrapper-no-manual-wrap`, `go-cqrs/skipped-not-nil-for-non-retryable`) are the rules the executor's error mapping MUST obey.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrapf` over `fmt.Errorf`; pass `ctx` to every constructor.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter; external test package.
</context>

<requirements>

1. **Create `pkg/command/trigger_release_check_executor.go`** in the same `pkg/command` package as prompt 1 (do NOT create a new package — the executor is a sibling of the sender, both logically part of "the trigger command's CQRS plumbing"). Add the executor to the same external-test-package Ginkgo suite that prompt 1 created.

   The executor file shape (anchored by symbol name, not line number):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package command

   import (
       "context"

       "github.com/bborbe/cqrs/base"
       cdb "github.com/bborbe/cqrs/cdb"
       "github.com/bborbe/errors"
       libkv "github.com/bborbe/kv"
       "github.com/golang/glog"

       "github.com/bborbe/maintainer/watcher/github-release/pkg"
   )

   // NewTriggerReleaseCheckCommandExecutor creates a cdb.CommandObjectExecutorTx that
   // consumes TriggerReleaseCheckCommand messages and drives the github-release
   // watcher: unmarshal → validate → invoke w.Poll(ctx) on the shared watcher
   // instance.
   //
   // Exit-path mapping (per spec 067 § Desired Behavior 4):
   //   - malformed payload (MarshalInto fails)    → cdb.ErrCommandObjectSkipped
   //   - cmd.Validate(ctx) failure                → cdb.ErrCommandObjectSkipped
   //   - w.Poll(ctx) returns non-nil error        → wrapped error (transient, retried)
   //   - w.Poll(ctx) returns nil                  → nil, nil, nil (success)
   //
   // SendResultEnabled is false (spec Non-goal: fire-and-forget, no result topic).
   // The executor does NOT increment any metrics — the Watcher.Poll(ctx) call
   // already owns IncPollCycle / IncPublished / IncReposScanned / IncFilterSkipped.
   // The executor does NOT read Scope or Force — both are reserved-unread
   // (spec Non-goal: per-repo filter UX and Force flag are separate specs).
   func NewTriggerReleaseCheckCommandExecutor(
       watcher pkg.Watcher,
   ) cdb.CommandObjectExecutorTx {
       return cdb.CommandObjectExecutorTxFunc(
           TriggerReleaseCheckCommandOperation,
           false, // SendResultEnabled = false
           func(ctx context.Context, tx libkv.Tx, commandObject cdb.CommandObject) (*base.EventID, base.Event, error) {
               return runTriggerReleaseCheck(ctx, tx, commandObject, watcher)
           },
       )
   }

   // runTriggerReleaseCheck is the work-loop for a single TriggerReleaseCheckCommand.
   // Splitting it out from the constructor (a) keeps the constructor's
   // closure short and (b) makes the function directly testable from
   // the package's external _test.go (the constructor returns an interface,
   // not a closure).
   //
   // cmd.Validate is invoked here as defense-in-depth: the sender already
   // validates before publishing, but a buggy client that bypasses the
   // HTTP handler could otherwise inject garbage. The framework's
   // CommandObject.Validate only checks the wrapper (SchemaID + base.Command),
   // not the typed payload.
   func runTriggerReleaseCheck(
       ctx context.Context,
       _ libkv.Tx,
       commandObject cdb.CommandObject,
       watcher pkg.Watcher,
   ) (*base.EventID, base.Event, error) {
       cmd, err := unmarshalAndValidate(ctx, commandObject)
       if err != nil {
           return nil, nil, err
       }
       if err := watcher.Poll(ctx); err != nil {
           // Transient: rate-limited, GitHub 5xx, cursor read error, etc.
           // Framework emits Failure on the result topic, Kafka redelivers.
           // The Watcher already logged per-cycle state; we just propagate.
           return nil, nil, errors.Wrapf(
               ctx, err, "poll cycle from trigger scope=%q force=%t", cmd.Scope, cmd.Force,
           )
       }
       glog.V(2).Infof(
           "trigger executor: poll cycle complete scope=%q force=%t",
           cmd.Scope, cmd.Force,
       )
       return nil, nil, nil
   }

   // unmarshalAndValidate decodes the CommandObject payload into a typed
   // TriggerReleaseCheckCommand and runs Validate as defense-in-depth. Any
   // failure here is a deliberate, non-retryable skip.
   func unmarshalAndValidate(
       ctx context.Context,
       commandObject cdb.CommandObject,
   ) (TriggerReleaseCheckCommand, error) {
       var cmd TriggerReleaseCheckCommand
       if err := commandObject.Command.Data.MarshalInto(ctx, &cmd); err != nil {
           return cmd, errors.Wrapf(
               ctx,
               cdb.ErrCommandObjectSkipped,
               "malformed TriggerReleaseCheckCommand: %v",
               err,
           )
       }
       if err := cmd.Validate(ctx); err != nil {
           return cmd, errors.Wrapf(
               ctx,
               cdb.ErrCommandObjectSkipped,
               "validate TriggerReleaseCheckCommand: %v",
               err,
           )
       }
       return cmd, nil
   }
   ```

   The `pkg "github.com/bborbe/maintainer/watcher/github-release/pkg"` import is mandatory — `pkg.Watcher` is the existing watcher interface. The `cmd` variable in `runTriggerReleaseCheck` is returned by `unmarshalAndValidate` even on the wrapped-error path because the error message interpolates `cmd.Scope` and `cmd.Force` (for observability of which fields were present in the failed command).

2. **Create `pkg/command/trigger_release_check_executor_export_test.go`** (lives in `package command` so it is excluded from production builds via the `_test.go` suffix). This re-exports the private `runTriggerReleaseCheck` to the external test package. The file shape:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package command

   import (
       "context"

       "github.com/bborbe/cqrs/base"
       cdb "github.com/bborbe/cqrs/cdb"
       libkv "github.com/bborbe/kv"

       "github.com/bborbe/maintainer/watcher/github-release/pkg"
   )

   // RunTriggerReleaseCheck re-exports the private runTriggerReleaseCheck for
   // the external test package. The _test.go suffix keeps this file
   // out of production builds.
   var RunTriggerReleaseCheck = runTriggerReleaseCheck

   // Compile-time guard: keep the public surface tightly aligned with
   // the internal helper. If runTriggerReleaseCheck's signature ever drifts,
   // this file fails to build and the test breakage is local.
   var _ = func(
       ctx context.Context,
       tx libkv.Tx,
       obj cdb.CommandObject,
       watcher pkg.Watcher,
   ) (*base.EventID, base.Event, error) {
       return runTriggerReleaseCheck(ctx, tx, obj, watcher)
   }
   ```

3. **Create `pkg/command/trigger_release_check_executor_test.go`** (external test package `command_test`). Required test cases — mirror the layout of `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor_test.go` but with the simpler github-release semantics:

   a. **Test helpers** (top of file, after imports):
   - `outcome` enum: `outcomeSuccess`, `outcomeSkipped`, `outcomeWrappedErr` (matching the spec-066 sibling).
   - `mustParseEvent(cmd command.TriggerReleaseCheckCommand) base.Event` — wraps `base.ParseEvent(ctx, cmd)`.
   - `newCommandObject(cmd command.TriggerReleaseCheckCommand) cdb.CommandObject` — builds a `cdb.CommandObject` with `Operation: command.TriggerReleaseCheckCommandOperation`, `Data: mustParseEvent(cmd)`, `SchemaID: lib.GithubReleaserV1SchemaID`.

   b. **Table-driven exit-path test.** Build a `Describe("NewTriggerReleaseCheckCommandExecutor", ...)` with `BeforeEach` that resets a `*mocks.Watcher` fixture and a `DescribeTable("exit-path mapping", ...)` with the following entries:

   ```go
   DescribeTable("exit-path mapping",
       func(
           configure func(w *mocks.Watcher),
           obj cdb.CommandObject,
           expectOutcome outcome, // skipped | wrappedErr | success
       ) {
           // Reset the watcher between entries — the table shares a
           // single fixture so we need to clear per-Entry state.
           *watcher = mocks.Watcher{}
           configure(watcher)

           _, _, err := command.RunTriggerReleaseCheck(ctx, nil, obj, watcher)

           switch expectOutcome {
           case outcomeSkipped:
               Expect(err).To(HaveOccurred(), "expected ErrCommandObjectSkipped")
               Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeTrue(),
                   "expected ErrCommandObjectSkipped, got %v", err)
           case outcomeWrappedErr:
               Expect(err).To(HaveOccurred(), "expected wrapped (transient) error")
               Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse(),
                   "transient errors must NOT be classified as Skipped, got %v", err)
               Expect(err.Error()).To(ContainSubstring("poll cycle from trigger"),
                   "transient errors must be wrapped with poll-cycle context, got %v", err)
           case outcomeSuccess:
               Expect(err).NotTo(HaveOccurred(), "unexpected error: %v", err)
           }
           // spec AC 8: Watcher.Poll must be invoked exactly once per
           // valid command. The malformed-payload path short-circuits
           // in unmarshalAndValidate, so for outcomeSkipped Poll must
           // NOT have been called (0 == the default zero call count).
           if expectOutcome == outcomeSkipped {
               Expect(watcher.PollCallCount()).To(Equal(0),
                   "skipped payloads must not invoke Watcher.Poll")
           } else {
               Expect(watcher.PollCallCount()).To(Equal(1),
                   "valid payloads must invoke Watcher.Poll exactly once")
           }
       },
       Entry("valid command → success + PollCallCount==1",
           func(_ *mocks.Watcher) {},
           newCommandObject(command.TriggerReleaseCheckCommand{Scope: "bborbe/repo", Force: true}),
           outcomeSuccess,
       ),
       Entry("malformed payload → skipped",
           // Force MarshalInto to fail: feed a CommandObject whose Data
           // is an Event with `scope` set to a JSON object — the
           // unmarshal step rejects "object into string".
           func(_ *mocks.Watcher) {},
           cdb.CommandObject{
               Command: base.Command{
                   Operation: command.TriggerReleaseCheckCommandOperation,
                   Data: base.Event{
                       "scope": map[string]interface{}{"unexpected": "object"},
                   },
               },
               SchemaID: lib.GithubReleaserV1SchemaID,
           },
           outcomeSkipped,
       ),
       Entry("watcher returns error → wrapped err (not skipped)",
           func(w *mocks.Watcher) {
               w.PollReturns(errors.Errorf(ctx, "rate limited"))
           },
           newCommandObject(command.TriggerReleaseCheckCommand{}),
           outcomeWrappedErr,
       ),
   )
   ```

   The `mocks.Watcher` is the existing counterfeiter mock for `pkg.Watcher` at `/workspace/watcher/github-release/mocks/watcher.go` (verify the file exists: `ls /workspace/watcher/github-release/mocks/watcher.go`). If the mock is missing, STOP and report `status: failed` with message "pkg.Watcher counterfeiter mock not found at /workspace/watcher/github-release/mocks/watcher.go" — adding a new mock is out of scope for this prompt and is owned by the spec that adds it.

   c. **NOTE on the validate-fail case.** The github-release command's `Validate` is empty (both `Scope` and `Force` are reserved-unread, so there is nothing to reject today). The only way to force a framework-level "CommandObject.Validate" failure in a unit test is to construct a CommandObject whose `Command.Operation` doesn't match the executor's expected operation — and the executor's `HandleCommand` is invoked directly in this test, so that mismatch is a no-op here. Document this with a comment block at the top of the `Describe("NewTriggerReleaseCheckCommandExecutor", ...)` block: the validate-fail coverage is owned by the integration test in prompt 4 (consumer-level test against a real `cdb.RunCommandConsumerTxDefault` with an out-of-schema message). Do NOT add a validate-fail table entry to this prompt.

   d. **Crash-recovery test (spec § AC 19).** Build a `Describe("executor crash recovery (spec 067 AC 19)", ...)` block. The test:

   ```go
   It("a killed invocation can be retried and Poll runs once on the fresh watcher", func() {
       // Round 1: simulate a real Watcher that respects context
       // cancellation. The stub honours ctx.Err() and returns the
       // context-cancelled error — same shape as a real watcher that
       // gets SIGKILL'd in mid-Poll.
       killedCtx, cancel := context.WithCancel(ctx)
       watcher.PollStub = func(c context.Context) error {
           // Cancel mid-call, then return the context error like a real Watcher would.
           cancel()
           return c.Err()
       }

       cmd := command.TriggerReleaseCheckCommand{Scope: "bborbe/repo"}
       commandObject := newCommandObject(cmd)

       _, _, err := command.RunTriggerReleaseCheck(
           killedCtx, nil, commandObject, watcher,
       )
       Expect(err).To(HaveOccurred(),
           "killed invocation must return a transient error so Kafka redelivers")
       Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse(),
           "killed invocation must NOT be classified as Skipped (transient, not deliberate)")
       Expect(err.Error()).To(ContainSubstring("poll cycle from trigger"),
           "killed invocation must be wrapped with poll-cycle context")
       Expect(watcher.PollCallCount()).To(Equal(1),
           "killed invocation must have called Poll once before failing")

       // Round 2: fresh context, fresh Watcher (PollReturns(nil)).
       // The same commandObject is reused (Kafka would redeliver it as-is).
       freshWatcher := new(mocks.Watcher)
       freshWatcher.PollReturns(nil)

       _, _, err = command.RunTriggerReleaseCheck(
           context.Background(), nil, commandObject, freshWatcher,
       )
       Expect(err).NotTo(HaveOccurred(), "retry must succeed: %v", err)
       Expect(freshWatcher.PollCallCount()).To(Equal(1),
           "retry must invoke Poll on the fresh Watcher exactly once")
   })
   ```

   The point of the test: prove that on retry (which Kafka would do via redelivery), the same `Watcher.Poll(ctx)` call runs again from scratch (i.e. `PollCallCount==1` on the fresh watcher — the framework's redelivery is responsible for the second invocation overall). Use a fresh `mocks.Watcher` for the retry so the call count is unambiguous. goleak is not used (not a project dep) — rely on the ctx-cancellation contract only.

4. **Run `make test` in the changed module.** From the github-release watcher dir:

   ```
   cd /workspace/watcher/github-release && make test
   ```

   Expected: exit code 0; the new executor tests pass; all pre-existing tests pass unchanged (the existing handler is unchanged in this prompt — its tests still pass).

5. **Do NOT run `make precommit` in this prompt.** Prompt 4 (wiring the third `run.Func`) owns the final precommit gate. This prompt only needs `make test`.

6. **YAGNI guard.** Do NOT add a `Scope` or `Force` handling branch — the executor reads `cmd.Scope` and `cmd.Force` only for the log-line interpolation (the fields are reserved-unread in this spec). Do NOT add a new Prometheus metric — `pkg.Watcher.Poll(ctx)` already owns all metric increments. Do NOT add a `Validate` call inside `NewTriggerReleaseCheckCommandExecutor`'s closure wrapper — `runTriggerReleaseCheck` calls it via `unmarshalAndValidate`. Do NOT add a `WaitGroup` or `sync.Mutex` — the executor is single-message-at-a-time (the framework's `MessageHandlerTx` calls it serially per command). Do NOT add a downstream publish path — the watcher's `Poll(ctx)` already calls `TaskPublisher` which publishes the `CreateTaskCommand`.
</requirements>

<constraints>
- The executor uses ONLY `cdb.ErrCommandObjectSkipped` for non-retryable paths and ONLY wrapped `errors.Wrapf(ctx, err, "...")` for transient paths. NEVER `return nil, nil, nil` as a skip signal — the spec explicitly forbids this anti-pattern (spec § Desired Behavior 4).
- `SendResultEnabled` is `false` — hard-coded in the constructor's second argument to `cdb.CommandObjectExecutorTxFunc`. Do NOT add a config field.
- The `tx libkv.Tx` parameter is unused (`_ libkv.Tx`); the executor does not write to kv. Do not call any tx methods. The parameter is required by the `HandleCommandFunc` signature.
- Error wrapping: `github.com/bborbe/errors` only. Never `fmt.Errorf`. Always pass `ctx` to error constructors. Never `context.Background()` in `pkg/`.
- The `runTriggerReleaseCheck` function is package-private. Test access via the `*_export_test.go` re-export pattern (Go convention). Do NOT make it public.
- Ginkgo v2 + Gomega + counterfeiter. External test package (`command_test`). Coverage on the new code ≥ 80% per `docs/definition-of-done.md`.
- Do NOT modify the existing `pkg/handler/trigger_handler.go` in this prompt — prompt 3 shrinks it.
- Do NOT modify `main.go` or the factory in this prompt — prompt 4 wires the third `run.Func`.
- Do NOT add a validate-fail table entry to the executor test — the github-release command's `Validate` is always-nil today, so there is no in-package scenario to test. The end-to-end validate-fail coverage is owned by the integration test in prompt 4.
- Do NOT commit — dark-factory handles git. Branch: `dark-factory/cqrs-trigger-github-release`.
- Do NOT touch the CHANGELOG in this prompt. The CHANGELOG entry for the full spec is owned by prompt 4.
- Build verification: `cd /workspace/watcher/github-release && make test` must exit 0.
</constraints>

<verification>

Verify the executor file was created and exports the expected public constructor:
```
grep -n 'func NewTriggerReleaseCheckCommandExecutor' /workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go
```
Must show the constructor signature: `(watcher pkg.Watcher) cdb.CommandObjectExecutorTx`.

Verify the private helper is re-exported for the external test package:
```
grep -n 'RunTriggerReleaseCheck' /workspace/watcher/github-release/pkg/command/trigger_release_check_executor_export_test.go
```
Must show `var RunTriggerReleaseCheck = runTriggerReleaseCheck`.

Verify the exit-path mapping uses the right sentinels (spec § AC 5, 6, 7):
```
grep -n 'cdb.ErrCommandObjectSkipped' /workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go
```
Must show at least two occurrences (one for MarshalInto fail, one for Validate fail — both in `unmarshalAndValidate`).

```
grep -n 'errors.Wrapf' /workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go
```
Must show at least three wrapped-error returns (MarshalInto fail, Validate fail, watcher-returns-error). Note: the MarshalInto and Validate failures are wrapped with `cdb.ErrCommandObjectSkipped`, the watcher-returns-error is wrapped with the watcher's error.

Verify the executor invokes `Watcher.Poll(ctx)` exactly once per valid command (spec § AC 8):
```
grep -n 'watcher.Poll(ctx)' /workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go
```
Must show exactly one call site (inside `runTriggerReleaseCheck`).

Verify the executor does NOT increment any metrics (spec § Desired Behavior: "executor does NOT increment any metrics — `pkg.Watcher.Poll(ctx)` already owns..."):
```
grep -n 'metrics.Inc' /workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go
```
Expected: NO matches — the executor file must not import or call the metrics interface.

Run the new tests:
```
cd /workspace/watcher/github-release && go test -mod=mod -v -count=1 ./pkg/command/... -run "TriggerReleaseCheck|exit.path|runTriggerReleaseCheck|crash.recovery"
```
Expected: exit code 0; the table-driven exit-path test passes all 3 entries; the crash-recovery test passes.

Run the full module test suite to confirm no regression in handler/factory/main tests (none of which are touched in this prompt):
```
cd /workspace/watcher/github-release && make test
```
Expected: exit code 0; all pre-existing tests pass unchanged; the new `pkg/command/` tests are additive.
</verification>
