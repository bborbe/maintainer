---
status: pending
spec: [067-cqrs-trigger-github-release]
created: "2026-06-09T16:20:25Z"
branch: dark-factory/cqrs-trigger-github-release
---

<summary>
- Adds a new `pkg/command` package containing the `TriggerReleaseCheckCommand` struct, its operation constant, the `TriggerReleaseCheckCommandSender` interface, and the `NewTriggerReleaseCheckCommandSender` constructor.
- The struct carries `Scope string` + `Force bool` (both reserved-unread by this spec — Scope is for a future per-repo filter UX, Force is for the prerequisite Force-flag task; both are plumbed on the wire so the schema does not need to change later).
- `Validate(ctx)` accepts the empty payload `{}` because both fields are reserved-unread today (no per-request field with meaning). A future spec will add per-repo or per-stage validation.
- `TriggerReleaseCheckCommandOperation` is the frozen wire string `"trigger-release-check"`.
- A counterfeiter mock for the sender interface is generated and committed under `mocks/trigger_release_check_command_sender.go`.
- A new `pkg/memdb.go` (mirrored from `watcher/github-pr/pkg/memdb.go`) provides the in-process `libkv.DB` the consumer needs for its offset store — replays the request topic from `OffsetOldest` on pod restart, safe because the downstream `CreateTaskCommand` is idempotent via derived task_id.
- Pure unit tests cover JSON round-trip, Validate (empty + populated), operation constant value, and a stub-based `SendCommand` test that asserts one `cdb.CommandObject` is published with the correct SchemaID and operation.

This is prompt 1 of 4 for spec 067. It is a leaf: it ships the command/sender types, the mock, and the in-memory DB so prompts 2 (executor), 3 (HTTP handler shrink), and 4 (consumer wiring) can import them in parallel.
</summary>

<objective>
Ship the `TriggerReleaseCheckCommand` payload, its `cdb.CommandObjectSender`-backed sender, the operation constant, the empty-payload `Validate`, the counterfeiter mock, and the in-memory `libkv.DB` (`pkg/memdb.go`). The downstream executor (prompt 2), the HTTP handler (prompt 3), and the consumer wiring (prompt 4) all depend on this package landing first.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these source files in full BEFORE editing — every change is anchored by real symbols, not by line number:

- `/workspace/watcher/github-release/go.mod` — confirms `github.com/bborbe/cqrs v0.5.1` and `github.com/bborbe/agent/lib v0.65.0` are direct deps; `github.com/bborbe/maintainer/lib` is replaced by `../../lib`.
- `/workspace/watcher/github-release/pkg/handler/trigger_handler.go` (read in full) — the existing `BackgroundRunHandler`-style trigger path. The new HTTP handler (prompt 3) shrinks this to a publish+202 shell; for now the file is unchanged.
- `/workspace/watcher/github-release/pkg/suite_test.go` — the Ginkgo suite. The new `pkg/command/suite_test.go` follows the same shape (60s timeout, `format.TruncatedDiff = false`, `go:generate counterfeiter` directive).
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_command.go` and `trigger_pr_review_command_sender.go` — the reference shapes from the spec-066 sibling. The new `TriggerReleaseCheckCommand` has the SAME structural shape as `TriggerPRReviewCommand` (struct + `Validate` + counterfeiter-annotated sender interface + `New*` constructor) but with two important differences: (a) the `Validate` body is empty (no URL parse, no per-field check), and (b) the schema ID is `lib.GithubReleaserV1SchemaID` instead of `lib.GithubPRReviewV1SchemaID`.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_command_sender.go` — the reference sender. The new sender has the SAME constructor signature (taking `base.CommandCreator`, `cqrsiam.Initiator`, `cdb.CommandObjectSender`) and the SAME `SendCommand` body, just keyed on the new operation and schema. **CRITICAL:** the new sender does NOT take a `branch` parameter — the branch is built into the underlying `cdb.CommandObjectSender` by the factory, not by the constructor.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_command_test.go` and `trigger_pr_review_command_sender_test.go` — the reference tests. Mirror their layout (operation constant value, operation regex validation, JSON round-trip, `omitempty` body, sender publishes one `CommandObject` with correct `SchemaID` and operation). The `Validate` test for the new command is simpler (only two entries: empty-payload-accepted, populated-payload-accepted) because no fields are validated today.
- `/workspace/watcher/github-pr/pkg/memdb.go` (the reference in-process `libkv.DB`) — the new `watcher/github-release/pkg/memdb.go` is a verbatim port. Read the file fully and reproduce it; only the package comments change (replace "github-pr" with "github-release" and remove any cross-references to spec 066 / PR #51).
- `/workspace/watcher/github-pr/pkg/memdb_test.go` (or the equivalent in `pkg/factory/command_consumer_test.go`) — the reference test for the in-memory DB. The new test for github-release lives at `/workspace/watcher/github-release/pkg/memdb_test.go` (external test package `pkg_test`) and adds the race-detector witness that the github-pr version has (the spec requires the lock+copy contract to survive `go test -race`).
- `/workspace/watcher/github-release/mocks/` — the existing counterfeiter output style. Verify the `mocks.go` aggregator file and one existing generated mock (e.g. `cursor_reader.go`) for the BSD license header + `Code generated by counterfeiter. DO NOT EDIT.` banner.
- `/workspace/lib/maintainer_cdb-schema.go` — the canonical `lib.GithubReleaserV1SchemaID` definition (the maintainer/lib dep is `replace`d to `../../lib` in the watcher's go.mod, so the live source is the mounted lib dir, not the module cache). **Do not edit this file** — the schema is frozen. Import via `github.com/bborbe/maintainer/lib` and reference `lib.GithubReleaserV1SchemaID`. Verify the symbol exists:

  ```
  grep -n 'GithubReleaserV1SchemaID' /home/node/go/pkg/mod/github.com/bborbe/maintainer/lib@*/maintainer_cdb-schema.go
  ```

Reference the agent/lib sibling command patterns (mirror their shape — read these to copy the exact import + struct + sender pattern):

- `/home/node/go/pkg/mod/github.com/bborbe/agent/lib@v0.65.0/command/task/create-command.go` — `CreateCommand` struct, `CreateCommandOperation` constant, `Validate` using `validation.All` + `validation.Name`. The new command is structurally identical (different field set: `Scope string` + `Force bool`; same JSON tag convention: lowercase, `omitempty`). The new `Validate` is EMPTY (no `validation.All{}` body) because both fields are reserved-unread.
- `/home/node/go/pkg/mod/github.com/bborbe/agent/lib@v0.65.0/command/task/create-command-sender.go` — `CreateCommandSender` interface, `NewCreateCommandSender(cdb.CommandObjectSender)` constructor, the `base.ParseEvent → base.NewCommandCreator → cdb.CommandObject{Sender.NewCommandObject}` flow. The new sender is identical in shape, just typed for `TriggerReleaseCheckCommand` and keyed on `lib.GithubReleaserV1SchemaID` instead of `lib.TaskV1SchemaID`.
- `/home/node/go/pkg/mod/github.com/bborbe/agent/lib@v0.65.0/command/task/increment-frontmatter-command-sender.go` — second reference; note the constructor signature takes `commandCreator base.CommandCreator` + `initiator cqrsiam.Initiator` + `commandObjectSender cdb.CommandObjectSender` — built once at construction, NOT per-call. Mirror this exact pattern.

Coding plugin docs (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — public interface + private struct + `New*` constructor + counterfeiter annotation.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrapf(ctx, err, "...")`, never `fmt.Errorf`, never `context.Background()` in `pkg/`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter; external test package `command_test`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-licensing-guide.md` — every new file must have the BSD license header dated 2026 (the project's `addlicense` Make target will add it; if `make precommit` is not run here, the agent adds it manually).
</context>

<requirements>

1. **Create the package `pkg/command/`** in `/workspace/watcher/github-release/pkg/command/` with a `doc.go` file (BSD header, `// Package command defines the TriggerReleaseCheckCommand payload and its Kafka sender.`) and a `suite_test.go` file that registers the new tests with the Ginkgo suite. Mirror the suite shape from `/workspace/watcher/github-pr/pkg/command/suite_test.go` (60s timeout, `format.TruncatedDiff = false`, `go:generate counterfeiter` directive) and put the file in the `command_test` external test package.

2. **Create `pkg/command/trigger_release_check_command.go`** with the struct, operation constant, and `Validate`. The exact shape (anchored by symbol name, not line number — line numbers are unstable across the agent's iteration):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package command

   import (
       "context"

       "github.com/bborbe/cqrs/base"
   )

   // TriggerReleaseCheckCommandOperation is the Kafka command operation for
   // triggering a github-release poll cycle. Wire string: "trigger-release-check".
   const TriggerReleaseCheckCommandOperation base.CommandOperation = "trigger-release-check"

   // TriggerReleaseCheckCommand is the payload for TriggerReleaseCheckCommandOperation.
   // It is published to the github-release watcher's request topic by the /trigger
   // HTTP handler and consumed by the in-pod command consumer.
   //
   // Scope is reserved for a future per-repo filter UX; this spec plumbs the
   // field but the executor ignores it. Force is reserved for the prerequisite
   // Force-flag task; this spec plumbs the field but the executor does not
   // branch on it. Both fields are present on the wire so the schema does
   // not need to change later.
   type TriggerReleaseCheckCommand struct {
       Scope string `json:"scope,omitempty"`
       Force bool   `json:"force,omitempty"`
   }

   // Validate enforces the command's schema rules. The empty payload {} is
   // accepted because both fields are reserved-unread — there's no
   // per-request field with meaning today. A future spec will add
   // per-repo or per-stage validation here.
   func (cmd TriggerReleaseCheckCommand) Validate(_ context.Context) error {
       return nil
   }
   ```

   The struct's JSON tags are `scope,omitempty` and `force,omitempty` (lowercase, `omitempty` per the project convention). The unused `context.Context` parameter in `Validate` is intentional (matches the `base.Command.Validate(ctx)` interface signature).

3. **Create `pkg/command/trigger_release_check_command_sender.go`** with the sender interface, counterfeiter annotation, and constructor. Mirror the spec-066 sibling shape EXACTLY (the only difference is the schema ID and operation constant):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package command

   import (
       "context"

       "github.com/bborbe/cqrs/base"
       cdb "github.com/bborbe/cqrs/cdb"
       cqrsiam "github.com/bborbe/cqrs/iam"
       "github.com/bborbe/errors"
       "github.com/golang/glog"

       "github.com/bborbe/maintainer/lib"
   )

   //counterfeiter:generate -o ../../mocks/trigger_release_check_command_sender.go --fake-name TriggerReleaseCheckCommandSender . TriggerReleaseCheckCommandSender

   // TriggerReleaseCheckCommandSender sends TriggerReleaseCheckCommand payloads to
   // Kafka. Calls Validate before publishing — a validation error is
   // returned without touching Kafka.
   type TriggerReleaseCheckCommandSender interface {
       SendCommand(ctx context.Context, cmd TriggerReleaseCheckCommand) error
   }

   // NewTriggerReleaseCheckCommandSender creates a TriggerReleaseCheckCommandSender.
   // The commandCreator and initiator are injected at construction time per
   // the cqrs/docs/producing-commands.md "Factory Wiring" pattern (matches
   // trading/frontend/command's reference impl) — built once at wiring, reused
   // across every SendCommand call. The commandObjectSender wraps the Kafka
   // sync producer.
   func NewTriggerReleaseCheckCommandSender(
       commandCreator base.CommandCreator,
       initiator cqrsiam.Initiator,
       commandObjectSender cdb.CommandObjectSender,
   ) TriggerReleaseCheckCommandSender {
       return &triggerReleaseCheckCommandSender{
           commandCreator:      commandCreator,
           initiator:           initiator,
           commandObjectSender: commandObjectSender,
       }
   }

   type triggerReleaseCheckCommandSender struct {
       commandCreator      base.CommandCreator
       initiator           cqrsiam.Initiator
       commandObjectSender cdb.CommandObjectSender
   }

   func (s *triggerReleaseCheckCommandSender) SendCommand(
       ctx context.Context,
       cmd TriggerReleaseCheckCommand,
   ) error {
       if err := cmd.Validate(ctx); err != nil {
           return errors.Wrapf(ctx, err, "validate TriggerReleaseCheckCommand")
       }
       event, err := base.ParseEvent(ctx, cmd)
       if err != nil {
           return errors.Wrapf(ctx, err, "parse TriggerReleaseCheckCommand event")
       }
       commandObject := cdb.CommandObject{
           Command: s.commandCreator.NewCommand(
               TriggerReleaseCheckCommandOperation,
               s.initiator,
               "",
               event,
           ),
           SchemaID: lib.GithubReleaserV1SchemaID,
       }
       if err := s.commandObjectSender.SendCommandObject(ctx, commandObject); err != nil {
           return errors.Wrapf(ctx, err, "send TriggerReleaseCheckCommand to Kafka")
       }
       glog.V(2).
           Infof("trigger sender: published op=%s scope=%q force=%t", TriggerReleaseCheckCommandOperation, cmd.Scope, cmd.Force)
       return nil
   }
   ```

   The `glog.V(2).Infof(...)` call sits AFTER `s.commandObjectSender.SendCommandObject(...)` returns nil (per the go-cqrs/heartbeat logging rule). The `cqrsiam.Initiator("watcher-github-release")` string is the producer-name in the audit trail — set by the FACTORY, not the constructor. The constructor takes `initiator` as an arg so the factory can choose the value.

4. **Regenerate the counterfeiter mock.** Run from `/workspace/watcher/github-release/pkg/command/`:

   ```
   cd /workspace/watcher/github-release && go generate -mod=mod ./pkg/command/...
   ```

   This regenerates `mocks/trigger_release_check_command_sender.go` (and any other generated mocks) using the directives in the source. Verify the new file exists:

   ```
   ls /workspace/watcher/github-release/mocks/trigger_release_check_command_sender.go
   ```

   The file MUST contain a `TriggerReleaseCheckCommandSender` struct with a `SendCommandStub` field, a `SendCommandCallCount()` method, and the `package mocks` declaration with the standard counterfeiter header. If counterfeiter is unavailable locally, run via `go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate` from the package directory.

5. **Create `pkg/command/trigger_release_check_command_test.go`** (external test package `command_test`) with these test cases — mirror the structure of `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_command_test.go` but with the simpler `Validate` test (no per-field validation today):

   a. `Describe("TriggerReleaseCheckCommandOperation", ...)` — two `It`s:
   - asserting the constant equals `base.CommandOperation("trigger-release-check")`
   - **boundary test (canonical DoD example):** asserting `TriggerReleaseCheckCommandOperation.Validate(context.Background())` returns `nil`. This catches the case where someone changes the wire string to a value that fails the cqrs `^[a-z][a-z-]*$` regex (e.g. with underscores or a leading digit). Per the agent/lib precedent and `coding/docs/go-cqrs.md`, every CommandOperation constant in a new package gets this check.

   b. `Describe("TriggerReleaseCheckCommand", ...)` — three `It`s:
   - JSON round-trip: marshal a fully-populated `TriggerReleaseCheckCommand{Scope: "bborbe/repo", Force: true}` to JSON, unmarshal back, assert both fields equal. The `omitempty` convention means a zero-value `Scope` and `Force: false` are OMITTED from the JSON output — assert that explicitly: `Expect(jsonStr).NotTo(ContainSubstring("\"scope\""))` and `Expect(jsonStr).NotTo(ContainSubstring("\"force\""))` for a zero-value instance.
   - JSON keys present: assert the marshaled output contains the literal strings `"scope"` and `"force"` when both are set.
   - Empty struct accepted by `Validate`.

   c. `Describe("TriggerReleaseCheckCommand.Validate", ...)` — two `It`s:
   - Empty payload `{}` returns `nil`.
   - Populated payload (Scope and Force both set) returns `nil`.

6. **Create `pkg/command/trigger_release_check_command_sender_test.go`** (external test package `command_test`) — mirror the structure of `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_command_sender_test.go` (the `newTestCommandCreator` helper, the `Context("valid command")` / `Context("Kafka publish fails")` / `Context("downstream is fed the correct command bytes")` blocks). Replace `lib.GithubPRReviewV1SchemaID` with `lib.GithubReleaserV1SchemaID` and `command.TriggerPRReviewCommand` with `command.TriggerReleaseCheckCommand`. The `validCmd` fixture is `command.TriggerReleaseCheckCommand{Scope: "bborbe/repo", Force: false}`. The "validation fails" Context block is OMITTED (the command's `Validate` is always-nil today — no validation failures to test). The "downstream is fed the correct command bytes" block asserts that the round-tripped `Scope` and `Force` match the original.

7. **Create `pkg/memdb.go`** in `/workspace/watcher/github-release/pkg/` (NOT in `pkg/factory/` — per spec § Constraints, the file lives in `pkg/`). This is a verbatim port of `/workspace/watcher/github-pr/pkg/memdb.go` with the package comments updated to refer to "github-release" and spec 067 instead of "github-pr" and spec 066. **Do not refactor, rename, or change the implementation** — the file is the same in-memory `libkv.DB` implementation that the spec-066 sibling uses. The struct name is `memDB`, the constructor is `NewMemDB() libkv.DB`. The package is `pkg` (matching the github-pr location).

8. **Create `pkg/memdb_test.go`** (external test package `pkg_test`) — mirror the test layout of `/workspace/watcher/github-pr/pkg/memdb_test.go` with the same five test cases: returns non-nil DB; implements the `libkv.DB` interface (Sync/Close/Remove/Stats return nil); round-trips a value via Update and View; returns `BucketNotFoundError` for non-existent bucket; **race-detector witness** (Stats + Update + View are race-free under concurrent access — this is the test that fails if anyone removes the locks or the value-copy in `Stats`).

9. **Run `make test` in the changed module.** From the github-release watcher dir:

   ```
   cd /workspace/watcher/github-release && make test
   ```

   Expected: exit code 0; the new `command` package tests pass; all pre-existing tests in `pkg/`, `pkg/handler/`, `pkg/factory/`, `pkg/filter/`, `pkg/auth/` continue to pass. The new mock file (`mocks/trigger_release_check_command_sender.go`) is generated and committed alongside.

10. **Do NOT run `make precommit` in this prompt.** Prompt 4 (wiring the third `run.Func`) owns the final precommit gate because the consumer wiring is the last step that touches the live factory + main.go. This prompt only needs `make test` to confirm the package is green in isolation. The auditor will run the full module precommit on the prompt 4 output.

11. **YAGNI guard.** Do NOT add a `Scope` or `Force` handling branch anywhere in this prompt — the executor (prompt 2) reads NEITHER field. Do NOT add a CLI flag, env var, or metric for "Force requested" or "Scope requested" — both are reserved for follow-on specs. Do NOT add a `Schema()` method to the command — the schema is identified by the constant in the sender. Do NOT introduce a new `lib` import for the command itself (the type lives in the watcher's `pkg/command` package, not in `lib`). Do NOT add per-field validation to `Validate` — the spec explicitly says "the empty payload `{}` is accepted" and both fields are reserved-unread. Do NOT add a `branch` parameter to `NewTriggerReleaseCheckCommandSender` — the branch is built into the underlying `cdb.CommandObjectSender` by the factory, not by the constructor.
</requirements>

<constraints>
- Schema is frozen: use `lib.GithubReleaserV1SchemaID` from `github.com/bborbe/maintainer/lib`. Do NOT define a new schema in this prompt. Do NOT use `lib.GithubPRReviewV1SchemaID` (that is the github-pr watcher's schema, not the github-release watcher's).
- Operation string is frozen: `"trigger-release-check"`. Do NOT change the wire string. The string MUST pass the cqrs `^[a-z][a-z-]*$` regex (no underscores, no leading digit, no uppercase) — verify by calling `TriggerReleaseCheckCommandOperation.Validate(ctx)`.
- Error wrapping: `github.com/bborbe/errors` only. Never `fmt.Errorf`. Always pass `ctx` to error constructors. Never `context.Background()` in `pkg/`.
- The new package follows the `pkg/` layout (no `internal/`); the package name is `command` and lives at `pkg/command/`. The in-memory DB lives at `pkg/memdb.go` (NOT `pkg/factory/memdb.go`).
- The struct's JSON tags are `scope,omitempty` and `force,omitempty` (lowercase; zero values are omitted from the wire).
- `Validate` MUST be empty (always return `nil`) — both fields are reserved-unread by this spec. A future spec will add per-repo or per-stage validation here.
- Counterfeiter annotation goes on the interface declaration line, with `-o ../../mocks/...` (two `..` — the package is two levels below `mocks/`).
- The mock file is generated, not hand-written. Verify the file has the `Code generated by counterfeiter. DO NOT EDIT.` header.
- The sender's `glog.V(2).Infof(...)` call sits AFTER `s.commandObjectSender.SendCommandObject(...)` returns nil (per the go-cqrs/heartbeat logging rule). The log line includes the operation constant, `cmd.Scope`, and `cmd.Force`.
- The `NewTriggerReleaseCheckCommandSender` constructor takes `commandCreator base.CommandCreator` + `initiator cqrsiam.Initiator` + `commandObjectSender cdb.CommandObjectSender` — built once at construction, NOT per-call (matches the spec-066 sibling and the cqrs/docs/producing-commands.md "Factory Wiring" pattern).
- Ginkgo v2 + Gomega + counterfeiter. External test package (`command_test` for the command tests, `pkg_test` for the memdb tests). Coverage on the new code must be ≥ 80% per `docs/definition-of-done.md`.
- Do NOT modify any existing file. The new package is purely additive — prompts 2, 3, and 4 wire it in.
- Do NOT commit — dark-factory handles git. The branch is `dark-factory/cqrs-trigger-github-release` (set in the spec frontmatter).
- Do NOT touch the CHANGELOG in this prompt. The CHANGELOG entry for the full spec is owned by the prompt that ships the user-visible behavior change (prompt 4, the consumer wiring — that is the "feat" per dark-factory's prefix rules).
- Build verification: `cd /workspace/watcher/github-release && make test` must exit 0.
</constraints>

<verification>

Verify the new files were created:
```
ls /workspace/watcher/github-release/pkg/command/
ls /workspace/watcher/github-release/pkg/memdb.go
ls /workspace/watcher/github-release/mocks/trigger_release_check_command_sender.go
```

Verify the operation constant is the exact wire string (spec § AC 12):
```
grep -n 'TriggerReleaseCheckCommandOperation' /workspace/watcher/github-release/pkg/command/trigger_release_check_command.go
```
Must show `const TriggerReleaseCheckCommandOperation base.CommandOperation = "trigger-release-check"`.

Verify the counterfeiter mock has the generated-header:
```
head -1 /workspace/watcher/github-release/mocks/trigger_release_check_command_sender.go
```
Must show `// Code generated by counterfeiter. DO NOT EDIT.`

Verify the counterfeiter directive sits ABOVE the type declaration (spec § AC 18):
```
grep -B 1 -A 2 '//counterfeiter:generate' /workspace/watcher/github-release/pkg/command/trigger_release_check_command_sender.go
```
Must show the directive on its own line directly followed by the `type TriggerReleaseCheckCommandSender interface` declaration.

Verify the sender logs AFTER `SendCommandObject` returns nil (spec § AC 18):
```
grep -A 1 'commandObjectSender.SendCommandObject' /workspace/watcher/github-release/pkg/command/trigger_release_check_command_sender.go
```
The `glog.V(2).Infof(...)` call must come AFTER the `SendCommandObject` call in source order.

Verify the memdb lives at the right path (spec § AC 16):
```
ls /workspace/watcher/github-release/pkg/memdb.go
ls /workspace/watcher/github-release/pkg/factory/memdb.go 2>&1
```
First must succeed; second must fail (file should not exist in `pkg/factory/`).

Verify the schema reference uses `lib.GithubReleaserV1SchemaID` (spec § AC 17):
```
grep -n 'lib.GithubReleaserV1SchemaID' /workspace/watcher/github-release/pkg/command/trigger_release_check_command_sender.go
```
Must show exactly one occurrence (the `cdb.CommandObject{SchemaID: lib.GithubReleaserV1SchemaID}` literal).

Run the new package tests:
```
cd /workspace/watcher/github-release && go test -mod=mod -v -count=1 ./pkg/command/...
```
Expected: exit code 0; the `TriggerReleaseCheckCommandOperation`, `TriggerReleaseCheckCommand`, `Validate`, and `NewTriggerReleaseCheckCommandSender` Describes all pass.

Run the memdb tests:
```
cd /workspace/watcher/github-release && go test -mod=mod -v -count=1 -race ./pkg/ -run "MemDB"
```
Expected: exit code 0; the `NewMemDB` Describes all pass; the race-detector witness confirms no data race under concurrent access.

Run the full module test suite to confirm no regression in sibling packages:
```
cd /workspace/watcher/github-release && make test
```
Expected: exit code 0; pre-existing tests in `pkg/`, `pkg/handler/`, `pkg/factory/`, `pkg/filter/`, `pkg/auth/` pass unchanged.

Confirm no part of the existing handler, factory, or main.go was touched (this prompt is purely additive):
```
git diff --stat HEAD -- /workspace/watcher/github-release/pkg/handler/ /workspace/watcher/github-release/pkg/factory/ /workspace/watcher/github-release/main.go
```
Expected: empty output — no changes in `pkg/handler/`, `pkg/factory/`, or `main.go`.
</verification>
