---
status: completed
spec: [067-cqrs-trigger-github-release]
summary: Added TriggerReleaseCheckCommand payload, CQRS sender (with counterfeiter mock), and in-memory libkv.DB offset store to github-release watcher; tests + mocks + CHANGELOG entry all generated; make precommit exits 0
container: maintainer-cqrs-trigger-release-exec-252-trigger-release-check-command-and-sender
dark-factory-version: v0.175.0
created: "2026-06-09T00:00:00Z"
queued: "2026-06-09T10:57:42Z"
started: "2026-06-09T10:57:44Z"
completed: "2026-06-09T11:10:10Z"
branch: dark-factory/cqrs-trigger-github-release
---

<summary>

- Creates a new `TriggerReleaseCheckCommand` payload with `Scope string` and `Force bool` fields (both reserved-unread for now) and a `Validate` method that accepts the empty payload `{}` so the wire format is forward-compatible.
- Defines the operation constant `TriggerReleaseCheckCommandOperation` as the wire string `"trigger-release-check"`, matching the `^[a-z][a-z-]*$` cqrs regex.
- Adds a `TriggerReleaseCheckCommandSender` interface, a `NewTriggerReleaseCheckCommandSender` constructor that takes `base.CommandCreator` + `cqrsiam.Initiator` + `cdb.CommandObjectSender` injected once at construction, and a generated counterfeiter mock.
- Mirrors `watcher/github-pr/pkg/memdb.go` verbatim at `watcher/github-release/pkg/memdb.go` (not in `pkg/factory/`) so the future command consumer can use it as a session-scoped offset store.
- Ships counterfeiter mocks for both the sender and (re-affirms) the existing `Watcher` interface; existing tests in `pkg/command/` and `pkg/` keep passing.

</summary>

<objective>
Add the CQRS primitives the github-release watcher needs to split `/trigger` from a goroutine into a Kafka publish + in-pod consumer: the command payload, its typed sender with a counterfeiter mock, and an in-memory `libkv.DB` for the consumer's offset store. Nothing in this prompt wires the HTTP handler or the consumer — those ship in prompts 3 and 4.
</objective>

<context>

- Reference implementation (verbatim mirror): `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_command.go`, `.../trigger_pr_review_command_sender.go`.
- Reference memdb: `/workspace/watcher/github-pr/pkg/memdb.go` (lives in `pkg/`, NOT `pkg/factory/`).
- Existing Watcher interface (unchanged by this spec, reused in prompt 2): `/workspace/watcher/github-release/pkg/watcher.go` — `Watcher.Poll(ctx) error`.
- Schema ID (frozen, used by sender + future executor): `lib.GithubReleaserV1SchemaID` from `/workspace/lib/maintainer_cdb-schema.go` (already at line 29-33 of that file).
- Existing `pkg/command/` does not exist yet — create it fresh, mirror the github-pr layout (`doc.go`, `suite_test.go`, then the new files).
- Existing `pkg/memdb.go` does not exist — create it at `/workspace/watcher/github-release/pkg/memdb.go`.
- `mocks/` already exists at `/workspace/watcher/github-release/mocks/` with `watcher.go`, `github_client.go`, etc. The new sender mock will be generated into `mocks/trigger_release_check_command_sender.go` via the `counterfeiter:generate` directive in the source file.

In-container docs (YOLO reads these at `/home/node/.claude/plugins/marketplaces/coding/docs/`):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — rules `skipped-not-nil-for-non-retryable` and `auto-tx-wrapper-no-manual-wrap`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — counterfeiter-directive-above-type-declaration rule.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter pattern, external test packages.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — `Create*` for factories, `New*` for constructors, zero-logic rule.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — `## Unreleased` entry format.

</context>

<ac-coverage>

This prompt covers these spec 067 acceptance criteria:

- **AC 4** — fake `cdb.CommandObjectSender` confirms the HTTP handler publishes exactly one command with `lib.GithubReleaserV1SchemaID` and `"trigger-release-check"` (also covered here at the sender unit-test level; the handler-level test lives in prompt 3).
- **AC 11** — `TriggerReleaseCheckCommand` has `Scope string` + `Force bool` fields, both reserved-unread, with `Validate(ctx)` accepting the empty payload `{}`.
- **AC 12** — `TriggerReleaseCheckCommandOperation` constant equals `"trigger-release-check"`.
- **AC 15** — counterfeiter mock generated at `mocks/trigger_release_check_command_sender.go`.
- **AC 16** — `pkg/memdb.go` lives at `watcher/github-release/pkg/memdb.go`, not in `pkg/factory/`.
- **AC 17** — counterfeiter directive sits on its own line ABOVE the `type TriggerReleaseCheckCommandSender interface` declaration.
- **AC 18** — `glog.V(2).Infof(...)` log line is placed AFTER `s.commandObjectSender.SendCommandObject(...)` returns nil.
- **AC 20** — coverage on new code in `pkg/command/` and `pkg/` is ≥ 80% (the `pkg/command/` and `pkg/factory/` thresholds from the spec are split across prompts 1-4; this prompt covers the new `pkg/command/` files).
- **AC 21** — schema reference is `lib.GithubReleaserV1SchemaID`; zero literal `"maintainer-githubreleaser-v1"` in `watcher/github-release/` outside test assertions.

</ac-coverage>

<requirements>

1. Create the directory `watcher/github-release/pkg/command/` (BSD copyright header on every new file, dated 2026).

2. Create `watcher/github-release/pkg/command/doc.go` mirroring the github-pr version:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package command defines the TriggerReleaseCheckCommand payload and its
   // Kafka sender for the github-release watcher's request topic.
   package command
   ```

3. Create `watcher/github-release/pkg/command/suite_test.go` mirroring the github-pr version (same Ginkgo v2 + Gomega + format.TruncatedDiff = false setup, 60s timeout, `go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate`).

4. Create `watcher/github-release/pkg/command/trigger_release_check_command.go` with EXACTLY this shape (mirror github-pr exactly; the only differences are `TriggerReleaseCheck` vs `TriggerPRReview`, the field names `Scope string` + `Force bool`, and an empty-payload-accepted `Validate`):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package command

   import (
       "context"

       "github.com/bborbe/cqrs/base"
       "github.com/bborbe/errors"
       "github.com/bborbe/validation"
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
   func (cmd TriggerReleaseCheckCommand) Validate(ctx context.Context) error {
       return validation.All{}.Validate(ctx)
   }
   ```

   Note: this `Validate` deliberately returns no error on empty `{}`. Do NOT add a non-empty check; the spec non-goal "Do NOT add per-repo filtering UX" forbids it.

5. Create `watcher/github-release/pkg/command/trigger_release_check_command_sender.go` with EXACTLY this shape (mirror github-pr verbatim, with `TriggerReleaseCheck` / `TriggerReleaseCheckCommandSender` / `lib.GithubReleaserV1SchemaID`):

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

6. Verify the counterfeiter directive sits on its own line ABOVE the type declaration (not inside the GoDoc block), per `go-patterns.md`.

7. Add `pkg/command/trigger_release_check_command_sender_test.go` mirroring `watcher/github-pr/pkg/command/trigger_pr_review_command_sender_test.go`, with these test cases (substitute `TriggerReleaseCheckCommand` and `lib.GithubReleaserV1SchemaID` for the github-pr equivalents; the constructor and the test command shape are the same — `cmd := command.TriggerReleaseCheckCommand{}` is the valid happy-path command because empty payload is accepted):
   - "publishes one CommandObject with the correct operation and SchemaID" — assert `obj.SchemaID == lib.GithubReleaserV1SchemaID` and `obj.Command.Operation == command.TriggerReleaseCheckCommandOperation`.
   - "validation fails returns a wrapped validation error and does NOT touch Kafka" — actually a no-op for github-release because empty payload passes Validate. SKIP this case; replace with a round-trip test on the JSON shape ("downstream is fed the correct command bytes") mirroring the github-pr test, asserting `Scope == ""` and `Force == false` round-trip.
   - "Kafka publish fails returns a wrapped Kafka error" — same as github-pr, assert error contains "send TriggerReleaseCheckCommand to Kafka" and the cdb sender was called exactly once.
   - Use the same `newTestCommandCreator(10)` helper pattern as github-pr — copy it into a `helper_test.go` (or inline in the test file as the github-pr test does — see `trigger_pr_review_command_sender_test.go:25-31`).
   - The fake `cdb.CommandObjectSender` is `*cqrsmocks.CDBCommandObjectSender` from `github.com/bborbe/cqrs/mocks`.

8. Add `pkg/command/trigger_release_check_command_test.go` mirroring `watcher/github-pr/pkg/command/trigger_pr_review_command_test.go`, with these test cases:
   - `Describe("TriggerReleaseCheckCommandOperation")` — assert string is `"trigger-release-check"`; assert `Validate(ctx)` succeeds (passes cqrs regex).
   - `Describe("TriggerReleaseCheckCommand")` — JSON round-trip with `Scope: "bborbe/repo"` + `Force: true`; assert `omitempty` works (JSON does not contain `"scope"` or `"force"` when zero); assert JSON contains both keys when set.
   - `Describe("TriggerReleaseCheckCommand.Validate")` — assert `Validate` returns nil for empty payload `{}` AND for `Scope: "bborbe"` / `Force: true` (acceptance — the spec deliberately accepts everything today).

9. Create `watcher/github-release/pkg/memdb.go` by VERBATIM-copying `/workspace/watcher/github-pr/pkg/memdb.go` to `/workspace/watcher/github-release/pkg/memdb.go`. The file lives in `pkg/`, NOT in `pkg/factory/` — this is a hard requirement from spec 067 constraints. Keep the same GoDoc, same fields, same methods (`Update`, `View`, `Sync`, `Close`, `Remove`, `Stats`, `StatsDetailed`).

10. Add unit tests for `NewMemDB` (at `pkg/memdb_test.go` external test package, OR in `pkg/watcher_test.go` — pick one location and don't duplicate) covering:
    - `NewMemDB()` returns non-nil.
    - Implements the `libkv.DB` interface: `Sync()`, `Close()`, `Remove()`, and `Stats(ctx)` are callable and return nil error.
    - `Update` + `View` round-trip: write a value via Update into a bucket, read it back via View.
    - `BucketNotFoundError` is returned when reading a non-existent bucket inside a `View` callback.

11. After creating all source files, regenerate counterfeiter mocks:
    ```bash
    cd watcher/github-release && go generate ./pkg/command/... ./pkg/...
    ```
    Verify `mocks/trigger_release_check_command_sender.go` is generated and contains a fake with `SendCommandStub func(context.Context, command.TriggerReleaseCheckCommand) error`.

12. Append a `## Unreleased` entry to `/workspace/CHANGELOG.md` per `changelog-guide.md`:
    ```
    - feat: Add TriggerReleaseCheckCommand payload, sender, and in-memory offset store for github-release /trigger CQRS split
    ```

13. Do NOT touch `pkg/handler/`, `pkg/factory/factory.go`, `main.go`, `cmd/run-once/main.go`, or the Watcher implementation in `pkg/watcher.go`. Those land in prompts 2, 3, 4.

14. Do NOT add a `CreateTriggerReleaseCheckCommandSender` factory function — that ships in prompt 4 alongside the consumer wiring. The constructor `NewTriggerReleaseCheckCommandSender` is what this prompt exposes.

15. Do NOT add any executor, HTTP handler, or factory wiring. This prompt is intentionally minimal: payload + sender + memdb + their tests + their mocks.

</requirements>

<constraints>

- The `Validate` method on `TriggerReleaseCheckCommand` MUST accept the empty payload `{}` — both `Scope` and `Force` are reserved-unread for this spec. Do NOT add a non-empty check, do NOT validate Scope format. An unused field is invariant; behavior change is a separate spec.
- Operation string is frozen at `"trigger-release-check"` (matches the cqrs `^[a-z][a-z-]*$` regex). Do NOT use underscores, uppercase, or a different word.
- Schema reference uses `lib.GithubReleaserV1SchemaID` from `github.com/bborbe/maintainer/lib` directly. Do NOT duplicate the string literal `"maintainer-githubreleaser-v1"` anywhere in `watcher/github-release/` (spec AC 21).
- Counterfeiter directive must sit on its own line ABOVE the type declaration, NOT inside any GoDoc block. The github-pr pattern uses `//counterfeiter:generate -o ../../mocks/<name>.go --fake-name <FakeName> . <InterfaceName>` — mirror that.
- `glog.V(2).Infof(...)` MUST be called AFTER `s.commandObjectSender.SendCommandObject(...)` returns nil, on the success path (spec AC 18). The line above is the success log; the failure path returns the wrapped error before the log.
- `pkg/memdb.go` lives at `watcher/github-release/pkg/memdb.go`, NOT at `watcher/github-release/pkg/factory/memdb.go`. The github-pr reference is at `watcher/github-pr/pkg/memdb.go` for the same reason (spec AC 16).
- Counterfeiter mock must be committed to `watcher/github-release/mocks/trigger_release_check_command_sender.go` (spec AC 15).
- BSD copyright header on every new file, dated 2026.
- Error wrapping uses `github.com/bborbe/errors` exclusively (never `fmt.Errorf`, never bare `return err`).
- Tests use Ginkgo v2 + Gomega + counterfeiter mocks, per repo convention.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

```bash
cd watcher/github-release && make test
```

Must pass with all existing tests + new tests. Then:

```bash
cd watcher/github-release && make precommit
```

Must exit 0.

Additional manual checks the executor should perform:

- `grep -rn '"maintainer-githubreleaser-v1"' watcher/github-release/` returns ONLY matches inside test files that assert on the schema struct itself (e.g. `lib/maintainer_cdb-schema.go` reference, if any). No literal duplication in production code.
- `grep -rn 'TriggerReleaseCheckCommandSender' watcher/github-release/pkg/command/` shows the interface declaration, the constructor, and the type definition.
- `ls watcher/github-release/pkg/memdb.go` succeeds; `ls watcher/github-release/pkg/factory/memdb.go` fails.
- `head -1 watcher/github-release/mocks/trigger_release_check_command_sender.go` is the counterfeiter-generated `// Code generated by counterfeiter. DO NOT EDIT.` line.
- The counterfeiter directive in the source file is followed (modulo blank line) by `type TriggerReleaseCheckCommandSender interface {`.
- `cd watcher/github-release && go test -coverprofile=/tmp/cover.out ./pkg/command/... ./pkg/... && go tool cover -func=/tmp/cover.out | awk '/total:/ {print $3}'` reports `>= 80.0%` for changed code.

Manual smoke (deployed dev pod, post-prompt-4):

```bash
curl -sS -o /dev/null -w '%{http_code}\n' -X POST \
  "https://dev.quant.benjamin-borbe.de/admin/maintainer-watcher-github-release/trigger"
# Expect: 202
kubectldev -n maintainer logs deploy/maintainer-watcher-github-release -f | grep -E 'trigger-release-check|poll cycle'
# Expect: trigger sender: published op=trigger-release-check AND poll cycle start
```

</verification>

## Improvements

- [PROMPT] Frontmatter mixes `name:`/`description:`/`tags:` with the spec's required `spec:`/`status:`/`created:` shape; an audit should normalise this. The current prompt is content-correct but the dark-factory frontmatter schema wants only `spec`, `status`, `created`. The `name`/`description`/`tags` fields are noise to the daemon.
- [PROMPT] The `pkg/memdb_test.go` location is left to the executor's discretion (req 10 says "OR in `pkg/watcher_test.go` — pick one"). Pin to a single location to avoid drift.
- [PROMPT] The Validate-fail test case is explicitly skipped (req 7 acknowledges the empty `Validate` makes this unreachable as a unit test). Audit should confirm the prompt 4 integration test fills the AC 5 row.
- [GUIDE] `go-cqrs.md` should add a "validate-fail in an empty-Validate spec is a no-op for unit tests; cover it end-to-end in the consumer" note — this gap bit this prompt.
- [GLOBAL] When a spec says "mirror github-pr X", add an explicit note about which sub-behaviors are reachable in unit tests vs require integration tests — the github-pr precedent's table entry (validate-fail) does NOT port 1:1 when the new command's Validate is empty.
