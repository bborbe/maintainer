---
status: completed
summary: Added TriggerBuildCheckCommandExecutor (constructor, export-test, table-driven + crash-recovery tests) mirroring the github-release spec-067 implementation; uses lib.GithubBuildV1SchemaID and the github-build pkg.Watcher mock; precommit exits 0 and coverage on runTriggerBuildCheck is 100%.
container: maintainer-cqrs-trigger-build-exec-258-cqrs-github-build-executor
dark-factory-version: v0.175.0
created: "2026-06-09T16:00:00Z"
queued: "2026-06-09T16:21:18Z"
started: "2026-06-09T16:38:27Z"
completed: "2026-06-09T16:43:00Z"
branch: dark-factory/cqrs-trigger-github-build
---

# Spec 068 Prompt 3 — TriggerBuildCheckCommandExecutor

## Context

This is prompt 3 of 5 for spec 068. It builds the executor that consumes `TriggerBuildCheckCommand` messages from the future Kafka request topic and drives `Watcher.Poll(ctx)` on the github-build watcher's shared `pkg.Watcher` instance.

**Depends on prompts 1 and 2 having landed.** Specifically:

- `lib.GithubBuildV1SchemaID` exists.
- `command.TriggerBuildCheckCommand` (with `Validate`, `TriggerBuildCheckCommandOperation`, `Scope`/`Force` fields).
- `command.RunTriggerReleaseCheck` is NOT relevant — the github-build parallel is `command.RunTriggerBuildCheck` (built in this prompt).
- The existing `pkg.Watcher` interface (Poll only) and its counterfeiter mock at `/workspace/watcher/github-build/mocks/watcher.go` are reused unchanged.

**This prompt can ship in parallel with prompt 4** (handler). They don't depend on each other.

**Mirror line-for-line** the spec 067 github-release implementation, which lives at:

- `/workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go`
- `/workspace/watcher/github-release/pkg/command/trigger_release_check_executor_export_test.go`
- `/workspace/watcher/github-release/pkg/command/trigger_release_check_executor_test.go`

Only the operation string, schema ID, type names, and import path of the `pkg.Watcher` package change. The exit-path mapping (skipped / wrapped / nil) and the auto-tx-wrapper pattern are verbatim.

## Goal

- `command.NewTriggerBuildCheckCommandExecutor(watcher pkg.Watcher) cdb.CommandObjectExecutorTx` — the constructor returns a `cdb.CommandObjectExecutorTxFunc` wrapping a closure that calls `runTriggerBuildCheck` per consumed message.
- `command.runTriggerBuildCheck(ctx, tx, commandObject, watcher) (*base.EventID, base.Event, error)` — the work-loop, exposed via `command.RunTriggerBuildCheck` in an `_export_test.go` for testability.
- `command.unmarshalAndValidate(ctx, commandObject) (TriggerBuildCheckCommand, error)` — defense-in-depth decoder; maps malformed-payload and validate-fail to `cdb.ErrCommandObjectSkipped`.
- Exit-path mapping per spec 068 § Desired Behavior 4:
  - malformed payload (MarshalInto fails) → `cdb.ErrCommandObjectSkipped` (non-retryable)
  - `cmd.Validate(ctx)` failure → `cdb.ErrCommandObjectSkipped` (non-retryable)
  - `watcher.Poll(ctx)` returns non-nil → wrapped error (transient, retried)
  - `watcher.Poll(ctx)` returns nil → `nil, nil, nil` (success)

## Files to create

- `/workspace/watcher/github-build/pkg/command/trigger_build_check_executor.go` — copy of `trigger_release_check_executor.go`. Rename `Release` → `Build` throughout. Change the import of the watcher package from `github.com/bborbe/maintainer/watcher/github-release/pkg` to `github.com/bborbe/maintainer/watcher/github-build/pkg`. Change the operation string to `TriggerBuildCheckCommandOperation`. Update the GoDoc on the executor and the comments referencing "github-release" to "github-build".
- `/workspace/watcher/github-build/pkg/command/trigger_build_check_executor_export_test.go` — copy of `trigger_release_check_executor_export_test.go` with all `Release` → `Build` and import path swap.
- `/workspace/watcher/github-build/pkg/command/trigger_build_check_executor_test.go` — copy of `trigger_release_check_executor_test.go`. Substitutions:
  - All `Release` → `Build`.
  - Import paths: `github.com/bborbe/maintainer/watcher/github-release/...` → `github.com/bborbe/maintainer/watcher/github-build/...` (mocks, pkg/command).
  - `lib.GithubReleaserV1SchemaID` → `lib.GithubBuildV1SchemaID`.
  - The "spec 067 AC 19" comment in the second Describe block becomes "spec 068 AC 26" (per the spec's crash-recovery test AC).

## Out of scope

- Do NOT wire the consumer in `pkg/factory/factory.go` — that ships in prompt 5. This prompt only builds the executor function and its tests.
- Do NOT touch the HTTP handler, `main.go`, or the factory. The executor returned here is consumed only by `factory.CreateCommandConsumer` in prompt 5.
- Do NOT change the `pkg.Watcher` interface or its mock. The mock at `/workspace/watcher/github-build/mocks/watcher.go` has the right shape already.
- Do NOT enable `SendResultEnabled` (kept `false` to match spec 067 and the spec's Non-goal).
- Do NOT add metric increments inside the executor — the existing `Watcher.Poll(ctx)` already owns them (per the github-release comment).

## Implementation

1. Read `/workspace/watcher/github-release/pkg/command/trigger_release_check_executor.go` fully. The constructor wraps a closure; the closure calls the top-level `runTriggerBuildCheck`; `unmarshalAndValidate` is a private helper. This three-function shape is the pattern.

2. The constructor must be pure composition — no business logic, no I/O, no early returns:

   ```go
   func NewTriggerBuildCheckCommandExecutor(
       watcher pkg.Watcher,
   ) cdb.CommandObjectExecutorTx {
       return cdb.CommandObjectExecutorTxFunc(
           TriggerBuildCheckCommandOperation,
           false, // SendResultEnabled = false
           func(ctx context.Context, tx libkv.Tx, commandObject cdb.CommandObject) (*base.EventID, base.Event, error) {
               return runTriggerBuildCheck(ctx, tx, commandObject, watcher)
           },
       )
   }
   ```

3. `runTriggerBuildCheck` calls `unmarshalAndValidate` first. On any error from the validator, return `(nil, nil, err)`. On a successful validation, call `watcher.Poll(ctx)`:
   - Non-nil error → wrap with `errors.Wrapf(ctx, err, "poll cycle from trigger scope=%q force=%t", cmd.Scope, cmd.Force)`. Return `(nil, nil, wrappedErr)`. The framework emits Failure on the result topic, Kafka redelivers.
   - Nil → log `glog.V(2).Infof("trigger executor: poll cycle complete scope=%q force=%t", cmd.Scope, cmd.Force)` and return `(nil, nil, nil)`.

4. `unmarshalAndValidate` does two checks; both failures wrap `cdb.ErrCommandObjectSkipped`:

   ```go
   if err := commandObject.Command.Data.MarshalInto(ctx, &cmd); err != nil {
       return cmd, errors.Wrapf(
           ctx,
           cdb.ErrCommandObjectSkipped,
           "malformed TriggerBuildCheckCommand: %v",
           err,
       )
   }
   if err := cmd.Validate(ctx); err != nil {
       return cmd, errors.Wrapf(
           ctx,
           cdb.ErrCommandObjectSkipped,
           "validate TriggerBuildCheckCommand: %v",
           err,
       )
   }
   return cmd, nil
   ```

5. The `_export_test.go` file re-exports `runTriggerBuildCheck` as `command.RunTriggerBuildCheck` so the external test package can drive it directly. Use the same shape as github-release's `trigger_release_check_executor_export_test.go`. Include the compile-time guard at the bottom that binds the signature.

6. Tests in `trigger_build_check_executor_test.go` mirror the github-release test file. Keep the same `outcome` enum (`outcomeSuccess | outcomeSkipped | outcomeWrappedErr`) and the same `DescribeTable` shape. Keep the malformed-payload entry that uses `base.Event{"scope": map[string]interface{}{"unexpected": "object"}}` to force `MarshalInto` to fail.

7. The "executor crash recovery" Describe block at the bottom (currently `Describe("executor crash recovery (spec 067 AC 19)", ...)`) is the load-bearing test for spec 068 AC 26. The comments inside reference "spec 067 AC 19" — update to "spec 068 AC 26" for the github-build context. Test logic stays identical.

## Tests

- `Describe("NewTriggerBuildCheckCommandExecutor", ...)` with a `DescribeTable("exit-path mapping", ...)`:
  - Entry "valid command → success + PollCallCount==1": happy path, watcher returns nil, `errors.Is(err, cdb.ErrCommandObjectSkipped)` is false, err is nil, PollCallCount == 1.
  - Entry "malformed payload → skipped": force `MarshalInto` to fail with `base.Event{"scope": map[string]interface{}{"unexpected": "object"}}`, assert `errors.Is(err, cdb.ErrCommandObjectSkipped) == true`, PollCallCount == 0.
  - Entry "watcher returns error → wrapped err (not skipped)": `watcher.PollReturns(errors.Errorf(ctx, "rate limited"))`, assert err is non-nil, `errors.Is(err, cdb.ErrCommandObjectSkipped) == false`, error message contains `"poll cycle from trigger"`, PollCallCount == 1.
- `Describe("executor crash recovery (spec 068 AC 26)", ...)`:
  - "a killed invocation can be retried and Poll runs once on the fresh watcher": simulate a killed watcher that cancels ctx and returns ctx.Err(); assert err is non-nil, NOT skipped, message contains `"poll cycle from trigger"`, PollCallCount == 1.
  - Round 2: fresh watcher with `PollReturns(nil)`, retry the same commandObject, assert no error, freshWatcher.PollCallCount() == 1.

## Verification

```
cd /workspace/watcher/github-build && make precommit
echo "exit=$?"
ls /workspace/watcher/github-build/pkg/command/trigger_build_check_executor.go
ls /workspace/watcher/github-build/pkg/command/trigger_build_check_executor_test.go
ls /workspace/watcher/github-build/pkg/command/trigger_build_check_executor_export_test.go
```

Precommit exits 0. All three `ls` calls succeed.

## Lessons from spec 067 audit (apply at write time)

1. Exit-path mapping uses `errors.Wrapf(ctx, cdb.ErrCommandObjectSkipped, ...)` for non-retryable, bare wrapped err for transient (lesson 3). NEVER `return nil, nil, nil` as a skip signal — that is success and the framework would commit the offset.
2. Use `cdb.CommandObjectExecutorTxFunc` (the spec 067 executor does NOT use `kv.NewTransactionMiddleware` — that's prompt 5's rule, but the executor itself is also expected to NOT manually wrap transactions). The closure receives a `libkv.Tx` from the auto-wrapper.
3. Split the work-loop from the constructor (lesson from github-release: "keeps the constructor's closure short and makes the function directly testable"). The export-test file exists specifically to expose `runTriggerBuildCheck` to the external test package.
4. The crash-recovery test stays at the executor layer (per spec 068 AC 26: "single source of truth, NOT duplicated in factory integration_test"). Prompt 5 will NOT repeat this test — the comment block at the bottom of `trigger_release_check_executor_test.go` makes that explicit; mirror that comment in the github-build copy.
5. Errors are wrapped with `github.com/bborbe/errors.Wrapf` (lesson 7). Never `fmt.Errorf`, never bare `return err`.
6. `glog.V(2).Infof` for the success log line; do NOT use `glog.V(3)` or `V(4)`. Per project convention, V(2) is heartbeat / per-cycle events.
7. The export-test file's compile-time guard (`var _ = func(...) { return runTriggerBuildCheck(...) }`) MUST stay. If `runTriggerBuildCheck`'s signature ever drifts, the export test fails to build and the breakage is local.
8. The malformed-payload test entry uses `base.Event{"scope": map[string]interface{}{"unexpected": "object"}}`. The executor unmarshals into `TriggerBuildCheckCommand` (where `Scope` is a `string`); an object into a string field fails the `MarshalInto` step. Do NOT change this entry.
9. The `outcome` enum type and the `mocks.Watcher` reset (`*watcher = mocks.Watcher{}`) per Entry are both load-bearing for the table-driven test. Keep them.
10. BSD copyright header on every new file, dated 2026.

## Improvements

(empty — YOLO fills in after running)
