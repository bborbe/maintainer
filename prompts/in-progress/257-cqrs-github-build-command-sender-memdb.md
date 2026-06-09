---
status: approved
created: "2026-06-09T16:00:00Z"
queued: "2026-06-09T16:21:18Z"
branch: dark-factory/cqrs-trigger-github-build
---

# Spec 068 Prompt 2 — TriggerBuildCheckCommand + Sender + MemDB

## Context

This is prompt 2 of 5 for spec 068. It builds the CQRS primitives the github-build watcher needs to publish `/trigger` requests: the typed command payload, the typed Kafka sender (with a counterfeiter mock for tests), and a session-scoped in-memory `libkv.DB` for the future command consumer's offset store. Nothing in this prompt wires the HTTP handler or the consumer — those ship in prompts 4 and 5.

**This prompt depends on prompt 1 having landed** — `lib.GithubBuildV1SchemaID` MUST exist. If `lib/maintainer_cdb-schema.go` does not export it, run prompt 1 first.

**Mirror line-for-line** the spec 067 github-release implementation, which lives at:

- `/workspace/watcher/github-release/pkg/command/trigger_release_check_command.go`
- `/workspace/watcher/github-release/pkg/command/trigger_release_check_command_sender.go`
- `/workspace/watcher/github-release/pkg/command/trigger_release_check_command_test.go`
- `/workspace/watcher/github-release/pkg/command/trigger_release_check_command_sender_test.go`
- `/workspace/watcher/github-release/pkg/command/doc.go`
- `/workspace/watcher/github-release/pkg/command/suite_test.go`
- `/workspace/watcher/github-release/pkg/memdb.go`
- `/workspace/watcher/github-release/pkg/memdb_test.go`

Only the operation string (`"trigger-build-check"`), the schema ID reference (`lib.GithubBuildV1SchemaID`), and the type/function names (`TriggerBuildCheck*`) change. Every other line is verbatim. The github-release implementation already has all 10 spec 067 audit lessons applied.

## Goal

- `TriggerBuildCheckCommand` struct with `Scope string`, `Force bool` (both `omitempty`) and a `Validate(ctx) error` that returns `nil` for the empty payload.
- `TriggerBuildCheckCommandOperation = base.CommandOperation("trigger-build-check")` constant.
- `TriggerBuildCheckCommandSender` interface + `NewTriggerBuildCheckCommandSender` constructor (injects `base.CommandCreator` + `cqrsiam.Initiator` + `cdb.CommandObjectSender` once at construction).
- Counterfeiter mock generated at `watcher/github-build/mocks/trigger_build_check_command_sender.go`.
- `pkg.NewMemDB()` (lives at `watcher/github-build/pkg/memdb.go`, NOT `pkg/factory/`) as a session-scoped `libkv.DB` for the future consumer's offset store.
- Race-detector witness test exercising concurrent Update + View + Stats.

## Files to create

- `/workspace/watcher/github-build/pkg/command/trigger_build_check_command.go` — copy `trigger_release_check_command.go` line-for-line, change `Release` → `Build` in all type/const names and the operation wire string, change `github-release` → `github-build` in the comment block. Use `"trigger-build-check"` as the operation constant.
- `/workspace/watcher/github-build/pkg/command/trigger_build_check_command_sender.go` — copy `trigger_release_check_command_sender.go` line-for-line, change `Release` → `Build` throughout, change the schema ID reference from `lib.GithubReleaserV1SchemaID` to `lib.GithubBuildV1SchemaID`, change the counterfeiter directive's `--fake-name` to `TriggerBuildCheckCommandSender` and the output filename to `trigger_build_check_command_sender.go` (relative `-o ../../mocks/...`).
- `/workspace/watcher/github-build/pkg/command/doc.go` — copy `trigger_release_check` doc.go; change "github-release" → "github-build" in the description.
- `/workspace/watcher/github-build/pkg/command/suite_test.go` — verbatim copy of `/workspace/watcher/github-release/pkg/command/suite_test.go`. Title "Command Suite" stays.
- `/workspace/watcher/github-build/pkg/command/trigger_build_check_command_test.go` — copy of `trigger_release_check_command_test.go` with the type name `TriggerBuildCheckCommand` substituted throughout. Wire string is `"trigger-build-check"`. Description block titles: `TriggerBuildCheckCommandOperation`, `TriggerBuildCheckCommand`, `TriggerBuildCheckCommand.Validate`.
- `/workspace/watcher/github-build/pkg/command/trigger_build_check_command_sender_test.go` — copy of `trigger_release_check_command_sender_test.go`, substitute `TriggerBuildCheckCommand` and `TriggerBuildCheckCommandSender` and reference `lib.GithubBuildV1SchemaID` in the test assertion (`Expect(obj.SchemaID).To(Equal(lib.GithubBuildV1SchemaID))`).
- `/workspace/watcher/github-build/pkg/memdb.go` — verbatim port of `/workspace/watcher/github-release/pkg/memdb.go`. Package is `pkg`, NOT `pkg/factory`. No edits to function names, struct names, or comments — the description already says "github-release" but the code is watcher-agnostic; either leave the comment as-is (it documents the origin pattern) or update it. Either is acceptable; consistency is not.
- `/workspace/watcher/github-build/pkg/memdb_test.go` — verbatim port of `/workspace/watcher/github-release/pkg/memdb_test.go`, changing only the import path from `github-release/pkg` to `github-build/pkg`.

## Files to modify

- `/workspace/watcher/github-build/mocks/mocks.go` — the `//go:generate` directive already exists in the file; counterfeiter picks it up on first `go generate`. No edit required.
- `/workspace/watcher/github-build/go.mod` — should not need a bump (prompt 1 modified the same `lib` module the watcher's `replace` directive points to). If `go mod tidy` complains, run it; otherwise leave untouched.

## Out of scope

- Do NOT create the executor (`trigger_build_check_executor.go`) — that ships in prompt 3.
- Do NOT create the HTTP handler in `pkg/handler/` — that ships in prompt 4.
- Do NOT modify `pkg/factory/factory.go` or `main.go` — those ship in prompt 5.
- Do NOT delete the legacy `pkg/trigger_handler.go` yet. It still serves the old in-process `chan struct{}` flow until prompt 4 ships and wires the new handler. Both will coexist transiently after this prompt; main.go still calls the old one.
- Do NOT change the existing `mocks/watcher.go` — it's reused by the executor in prompt 3 unchanged.

## Implementation

1. Read all eight canonical files listed above fully. The github-release implementation is the contract.

2. In `trigger_build_check_command.go`, the `Validate` function is the load-bearing line per spec 067 lesson 10:

   ```go
   func (cmd TriggerBuildCheckCommand) Validate(_ context.Context) error {
       return nil
   }
   ```

   Do NOT wrap in `validation.All{}.Validate(ctx)`. The function body must be a literal `return nil`.

3. In `trigger_build_check_command_sender.go`, the counterfeiter directive MUST sit on its own line above the type declaration, NOT inside a GoDoc block (spec 067 lesson 6):

   ```go
   //counterfeiter:generate -o ../../mocks/trigger_build_check_command_sender.go --fake-name TriggerBuildCheckCommandSender . TriggerBuildCheckCommandSender

   // TriggerBuildCheckCommandSender sends ...
   type TriggerBuildCheckCommandSender interface {
       SendCommand(ctx context.Context, cmd TriggerBuildCheckCommand) error
   }
   ```

4. The `SendCommand` method body mirrors github-release exactly. Schema ID reference is `lib.GithubBuildV1SchemaID` (per prompt 1's export). The `glog.V(2).Infof(...)` log line MUST come AFTER `commandObjectSender.SendCommandObject(...)` returns nil, NOT before (spec 067 lesson 7).

5. Generate the counterfeiter mock by running (from the watcher dir):

   ```
   cd /workspace/watcher/github-build && go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate
   ```

   Verify the new file `mocks/trigger_build_check_command_sender.go` is present and starts with `// Code generated by counterfeiter. DO NOT EDIT.`. The fake name MUST be `TriggerBuildCheckCommandSender` (the struct type used in tests).

6. `pkg/memdb.go` is package `pkg` (no `factory` import). The github-build memdb test must be in package `pkg_test` (external test package, per project convention).

7. Run `cd /workspace/watcher/github-build && make precommit` to confirm the new files compile, the counterfeiter mock generated cleanly, and all tests pass.

## Tests

- `trigger_build_check_command_test.go` covers:
  - `TriggerBuildCheckCommandOperation` has expected string value AND passes the cqrs operation regex `Validate(ctx)` (boundary test against the `^[a-z][a-z-]*$` rule).
  - `TriggerBuildCheckCommand` JSON round-trips with both fields set; empty `{}` does NOT contain `"scope"` or `"force"` keys (omitempty proof); non-empty contains both keys.
  - `TriggerBuildCheckCommand.Validate(ctx)` accepts the empty payload `{}` AND a populated payload.
- `trigger_build_check_command_sender_test.go` covers:
  - `SendCommand` with a valid command publishes ONE `CommandObject` whose `SchemaID` is `lib.GithubBuildV1SchemaID` and whose `Command.Operation` is `command.TriggerBuildCheckCommandOperation`.
  - On `cdb.CommandObjectSender.SendCommandObject` returning an error, `SendCommand` returns a wrapped error whose message contains both `"send TriggerBuildCheckCommand to Kafka"` and the underlying error string.
  - The published `CommandObject.Command.Data` round-trips through `MarshalInto(ctx, &cmd)` back to the original `Scope` and `Force` values (proves the event payload is correct).
- `memdb_test.go` covers:
  - `NewMemDB()` returns a non-nil `libkv.DB`.
  - `Sync/Close/Remove/Stats` all return nil.
  - `Update` + `View` round-trip a value through a bucket.
  - Reading a non-existent bucket returns `libkv.BucketNotFoundError`.
  - **Race-detector witness test** ("Stats + Update + View are race-free under concurrent access" or similar): 3 goroutines × 50 iterations doing concurrent Update/View/Stats. Test name MUST contain `Race` or `Concurrent` per spec AC. `cd /workspace/watcher/github-build && go test -race ./pkg/...` MUST pass.

## Verification

```
cd /workspace/watcher/github-build && make precommit
echo "exit=$?"
cd /workspace/watcher/github-build && go test -race ./pkg/...
echo "race_exit=$?"
ls /workspace/watcher/github-build/pkg/command/trigger_build_check_command.go
ls /workspace/watcher/github-build/mocks/trigger_build_check_command_sender.go
ls /workspace/watcher/github-build/pkg/memdb.go
```

All three `ls` calls must succeed. Both precommit and `-race` exits must be 0.

## Lessons from spec 067 audit (apply at write time)

1. Sender constructor takes `(base.CommandCreator, cqrsiam.Initiator, cdb.CommandObjectSender)`. Build `CommandCreator` with `base.RequestIDChannel(ctx)` ONCE at factory wiring time (in prompt 5). Do NOT copy the per-call drift from `agent/lib/command/task/`.
2. `Validate(ctx)` returns `nil` directly (lesson 10) — NO `validation.All{}.Validate(ctx)` wrapper. Empty payload is accepted.
3. Counterfeiter directive ABOVE the type declaration, NOT inside the GoDoc block (lesson 6).
4. `glog.V(2).Infof(...)` AFTER `commandObjectSender.SendCommandObject(...)` returns nil (lesson 7). The github-release sender is the reference.
5. memdb lives in `pkg/`, NOT `pkg/factory/` (lesson 8). `sync.RWMutex`; `Stats` / `StatsDetailed` lock+copy.
6. Operation wire string MUST pass the cqrs regex `^[a-z][a-z-]*$`. `"trigger-build-check"` matches. The unit test that calls `command.TriggerBuildCheckCommandOperation.Validate(context.Background())` proves this at the boundary — do NOT skip that test.
7. The sent `cdb.CommandObject.Command.Operation` is set via `s.commandCreator.NewCommand(TriggerBuildCheckCommandOperation, ...)`. Do NOT hard-code a string literal there.
8. Error wrapping: `errors.Wrapf(ctx, err, "send TriggerBuildCheckCommand to Kafka")` — the github-release pattern uses `errors` (github.com/bborbe/errors), never `fmt.Errorf`, never bare `return err`.
9. External test package convention: every new `_test.go` declares `package <x>_test`, not `package <x>`. The `cdb.CommandObject{}` import in the sender test is used as a "silence unused-import" marker at the bottom — keep that line.
10. BSD copyright header `// Copyright (c) 2026 Benjamin Borbe All rights reserved.` on EVERY new file, dated 2026.

## Improvements

(empty — YOLO fills in after running)
