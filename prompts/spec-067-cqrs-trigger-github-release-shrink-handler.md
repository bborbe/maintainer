---
status: pending
spec: [067-cqrs-trigger-github-release]
created: "2026-06-09T16:20:25Z"
branch: dark-factory/cqrs-trigger-github-release
---

<summary>
- `pkg/handler/trigger_handler.go` is rewritten to be a thin CQRS shell: publish a zero-value `TriggerReleaseCheckCommand` to Kafka via the injected sender, and return HTTP 202 with body `{"status":"accepted"}`. The handler consumes no request body or query string (both `Scope` and `Force` are reserved-unread).
- The handler no longer depends on `pkg.Watcher`, `libhttp.NewBackgroundRunHandler`, or any GitHub API access on the request path. Its only outbound call is `Sender.SendCommand(ctx, command.TriggerReleaseCheckCommand{})` for the Kafka publish.
- A "panicking Watcher" test wires a `pkg.Watcher` whose `Poll(ctx)` panics into a sibling object constructed alongside the handler. The test asserts that `POST /trigger` STILL returns HTTP 202 — the Watcher is not on the request path, so the panic must not propagate. This is spec § AC 3.
- The wire shape on the success path is `{"status":"accepted"}` (HTTP 202). The error path is HTTP 502 if the Kafka publish fails (the upstream Kafka broker is the proximate cause, not this service).
- A new `TriggerReleaseCheckHandler` counterfeiter mock is generated and committed at `mocks/trigger_release_check_handler.go` for use by `pkg/factory/command_consumer_test.go` (added in prompt 4).
- The factory function `factory.CreateTriggerReleaseCheckHandler` (added in prompt 4) is updated: its parameter list becomes `(sender command.TriggerReleaseCheckCommandSender) handler.TriggerReleaseCheckHandler`. The factory's existing test is updated to match the new signature.
- No CHANGELOG entry in this prompt (prompt 4 owns the spec-level CHANGELOG).

This is prompt 3 of 4 for spec 067. It depends on prompt 1 (the sender type). It does NOT depend on prompt 2 (the executor) — the HTTP handler shrink is independent of the executor's internals, only its public type. The factory rewiring happens in prompt 4.
</summary>

<objective>
Reduce the HTTP handler to its minimum viable behavior: build a zero-value `TriggerReleaseCheckCommand`, publish it to Kafka via the injected sender, and return HTTP 202. The handler must hold no Watcher, GitHub, or scan-cycle dependencies on the request path. This is the user-visible wire-compatibility change (background-goroutine → 202-ack-and-return) that operators will see on `POST /trigger`.

After this prompt, `POST /trigger` returns 202 in well under 100 ms regardless of how slow GitHub is — the work moves to the consumer (prompt 4). A panicking Watcher constructed alongside the handler does NOT affect the HTTP response — proving the Watcher is not on the request path.
</objective>

<context>
Read `/workspace/CLAUDE.md` for project conventions.

Read these source files in full BEFORE editing:

- `/workspace/watcher/github-release/pkg/handler/trigger_handler.go` — the file being rewritten. The new file SHARES the same package name (`handler`) and exports a counterfeiter-mockable interface alias `TriggerReleaseCheckHandler = libhttp.WithError` (so the wiring in `main.go` stays byte-identical at the type level). The struct's fields shrink to one: `sender command.TriggerReleaseCheckCommandSender`. The `ServeHTTP` body becomes ~25 lines.
- `/workspace/watcher/github-release/pkg/handler/trigger_handler_test.go` (read in full, even if it does not exist yet — it WILL be created/rewritten in this prompt) — the existing test file. The new test file is structured around (a) happy-path 202+body assertions, (b) Kafka-failure 502 assertion, (c) a structural assertion that the handler's struct has NO `pkg.Watcher`-typed field (using `reflect`), and (d) a panicking-Watcher behavioral assertion.
- `/workspace/watcher/github-release/pkg/handler/doc.go` — the existing package doc. Mirror the github-pr sibling's pattern (BSD header + `// Package handler defines the HTTP handlers for the github-release watcher's admin endpoints.`).
- `/workspace/watcher/github-release/pkg/handler/suite_test.go` — the Ginkgo suite. The `go:generate counterfeiter` directive at the top runs counterfeiter for any `//counterfeiter:generate` annotation. The new test file uses the `mocks.TriggerReleaseCheckCommandSender` (prompt 1) as the new HTTP-side mock.
- `/workspace/watcher/github-pr/pkg/handler/trigger_handler.go` (the spec-066 sibling) — the EXACT structural shape the new handler mirrors. Differences: the new handler does NOT parse a `url` query parameter (it builds a zero-value `TriggerReleaseCheckCommand`), the new handler returns 502 (not 400 / 500) on Kafka publish failure, and the new handler's body is shorter (no `validateTriggerURL` call, no `cmd.URL`/`cmd.Force` field assignments).
- `/workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go` (the spec-066 reference) — mirror the layout: Ginkgo Describe blocks for `happy path`, `Kafka send failure`, and `handler struct has no Watcher-typed field (spec 067 AC 3)`. The github-release version adapts: the "happy path" assertion checks the body is exactly `{"status":"accepted"}` (no `"url"` field — the command has no per-request field with meaning).
- `/workspace/watcher/github-release/pkg/watcher.go` (lines covering the `Watcher` interface) — the `Watcher` interface with `Poll(ctx context.Context) error`. The new handler's structural test uses `reflect.TypeOf((*pkg.Watcher)(nil)).Elem()` to assert no handler field implements this interface.
- `/workspace/watcher/github-release/mocks/watcher.go` — the existing counterfeiter mock for `pkg.Watcher`. The new test file imports `*mocks.Watcher` to construct a panicking fixture (defined inline as a struct that implements `pkg.Watcher` and panics on `Poll`).
- `/workspace/lib/http` — `libhttp.NewErrorHandler`, `libhttp.NewJSONErrorHandler`, `libhttp.WrapWithStatusCode`. The handler's 202 success path is plain `ServeHTTP` (no error handler wrapper); the 502 Kafka-failure path uses `libhttp.WrapWithStatusCode`.
- `/workspace/prompts/spec-067-cqrs-trigger-github-release-command.md` — the prompt 1 output. The new HTTP handler imports `command.TriggerReleaseCheckCommand`, `command.TriggerReleaseCheckCommandSender`, `command.TriggerReleaseCheckCommandOperation` from this package. Verify the package exists before editing: `ls /workspace/watcher/github-release/pkg/command/`. If empty, STOP and report `status: failed` with message "prompt 1 of spec 067 has not shipped".

Reference the HTTP handler patterns in the agent project:

- `/home/node/go/pkg/mod/github.com/bborbe/agent/lib@v0.65.0/command/task/create-command-sender.go` lines 37-65 — the `SendCommand` method shape. The HTTP handler calls `sender.SendCommand(ctx, command.TriggerReleaseCheckCommand{})` exactly once with a zero-value command (no `Scope`, no `Force`).
- `/home/node/go/pkg/mod/github.com/bborbe/cqrs@v0.5.1/cdb/cdb_command-object-sender.go` — the `cdb.CommandObjectSender.SendCommandObject` method. The handler does NOT call this directly — it calls the `TriggerReleaseCheckCommandSender` (the typed wrapper from prompt 1), which in turn calls `SendCommandObject`. This indirection is intentional and matches the spec-066 sibling.

Coding plugin docs (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md` — handlers in `pkg/handler/`, factory in `pkg/factory/`, no inline handlers in `main.go`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md` — JSON error responses; the 502 path uses `libhttp.WrapWithStatusCode`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrapf` over `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter; external test package.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — `//counterfeiter:generate` directive ABOVE the type declaration, NOT inside the GoDoc block.
</context>

<requirements>

1. **Rewrite `pkg/handler/trigger_handler.go`** end-to-end. The new file shape:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package handler

   import (
       "context"
       "encoding/json"
       "net/http"

       "github.com/bborbe/errors"
       libhttp "github.com/bborbe/http"
       "github.com/golang/glog"

       "github.com/bborbe/maintainer/watcher/github-release/pkg/command"
   )

   //counterfeiter:generate -o ../../mocks/trigger_release_check_handler.go --fake-name TriggerReleaseCheckHandler . TriggerReleaseCheckHandler

   // TriggerReleaseCheckHandler handles POST /trigger.
   // The handler is intentionally minimal: build a zero-value
   // TriggerReleaseCheckCommand, publish it to Kafka via the injected
   // sender, and return HTTP 202. No request body or query string is
   // consumed (both Scope and Force are reserved-unread). All scan
   // cycle work is owned by the in-pod command consumer.
   type TriggerReleaseCheckHandler = libhttp.WithError

   // NewTriggerReleaseCheckHandler returns a handler that publishes a
   // TriggerReleaseCheckCommand to Kafka for each /trigger request and
   // returns 202. The sender is the only collaborator.
   func NewTriggerReleaseCheckHandler(
       sender command.TriggerReleaseCheckCommandSender,
   ) TriggerReleaseCheckHandler {
       return &triggerReleaseCheckHandler{
           sender: sender,
       }
   }

   type triggerReleaseCheckHandler struct {
       sender command.TriggerReleaseCheckCommandSender
   }

   func (h *triggerReleaseCheckHandler) ServeHTTP(
       ctx context.Context,
       resp http.ResponseWriter,
       _ *http.Request,
   ) error {
       // Both fields are reserved-unread; build a zero-value command.
       if err := h.sender.SendCommand(ctx, command.TriggerReleaseCheckCommand{}); err != nil {
           // 502 BadGateway over 500/503: upstream Kafka is the proximate cause,
           // not this service. 500 implies an unexpected handler bug; 503 implies
           // this service is unhealthy. Kafka publish failure is neither — it's
           // an upstream gateway dependency, so 502 is the most accurate signal
           // for operators + observability tools.
           return libhttp.WrapWithStatusCode(
               errors.Wrap(ctx, err, "send TriggerReleaseCheckCommand"),
               http.StatusBadGateway,
           )
       }

       glog.V(2).Infof("trigger accepted op=%s", command.TriggerReleaseCheckCommandOperation)
       return writeAccepted(resp)
   }

   // writeAccepted emits the 202 response with body {"status":"accepted"}.
   func writeAccepted(resp http.ResponseWriter) error {
       resp.Header().Set("Content-Type", "application/json")
       resp.WriteHeader(http.StatusAccepted)
       return json.NewEncoder(resp).Encode(map[string]interface{}{
           "status": "accepted",
       })
   }
   ```

   The counterfeiter annotation sits ABOVE the `type TriggerReleaseCheckHandler` declaration, on its own line, with `-o ../../mocks/trigger_release_check_handler.go --fake-name TriggerReleaseCheckHandler . TriggerReleaseCheckHandler` (matches the spec-066 sibling's annotation). The `_ *http.Request` parameter is intentionally unused — the handler does not read the request body or query string.

2. **Create `pkg/handler/doc.go`** (BSD header + `// Package handler defines the HTTP handlers for the github-release watcher's admin endpoints.`) if it does not already exist. If it already exists with different content, leave it untouched.

3. **Create `pkg/handler/suite_test.go`** (BSD header, external test package `handler_test`, `go:generate counterfeiter` directive at the top, Ginkgo suite registration with 60s timeout and `format.TruncatedDiff = false`) if it does not already exist. If it already exists with different content, leave it untouched.

4. **Create `pkg/handler/trigger_handler_test.go`** (external test package `handler_test`) — mirror the layout of `/workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go` (post-spec-066 rewrite) but with the github-release specifics:

   a. **Test fixtures at the top of the file:**
   - `panickingWatcher` struct (NOT in the test package, defined at file scope) with a `Poll(_ context.Context) error` method that panics. The struct MUST implement `pkg.Watcher` (compile-time guard: `var _ pkg.Watcher = (*panickingWatcher)(nil)`).
   - Imports: `mocks "github.com/bborbe/maintainer/watcher/github-release/mocks"`, `pkg "github.com/bborbe/maintainer/watcher/github-release/pkg"`, `handler "github.com/bborbe/maintainer/watcher/github-release/pkg/handler"`, `libhttp "github.com/bborbe/http"`, standard Ginkgo/Gomega.

   b. **Happy-path tests** (Context "happy path"):
   - `It("returns 202 with {status:accepted} body", ...)` — POST to `/trigger`, assert `resp.Code == http.StatusAccepted`, assert `Content-Type: application/json`, unmarshal the body into `map[string]interface{}`, assert `body == {"status": "accepted"}` (length 1, key "status" == "accepted").
   - `It("publishes exactly one zero-value TriggerReleaseCheckCommand", ...)` — assert `sender.SendCommandCallCount() == 1`, assert the captured command has `Scope == ""` and `Force == false`.

   c. **Kafka send failure tests** (Context "Kafka send failure"):
   - `BeforeEach` sets `sender.SendCommandReturns(errors.Errorf(ctx, "kafka error"))`.
   - `It("returns 502", ...)` — assert `resp.Code == http.StatusBadGateway`.

   d. **No-Watcher-on-request-path tests** (Context "handler struct has no Watcher-typed field (spec 067 AC 3)"):
   - `It("handler struct has no Watcher-typed field", ...)` — use `reflect.TypeOf(concrete).Elem()` to walk the handler's struct fields, assert no field's type implements `pkg.Watcher`. Build the handler via `handler.NewTriggerReleaseCheckHandler(nil)` (the constructor is pure composition with no nil-check, so passing nil is safe for the reflect-only test).
   - `It("request completes with 202 (no Watcher wired anywhere)", ...)` — POST to `/trigger` with the standard `BeforeEach` setup (which has a `*mocks.TriggerReleaseCheckCommandSender`), assert `resp.Code == http.StatusAccepted`. This proves the request path does not require a Watcher.
   - `It("request completes with 202 even when a panicking Watcher is constructed alongside", ...)` — construct a `&panickingWatcher{}` (the panic is never invoked, but the type exists in the test's scope as proof the handler has no indirect reference), POST to `/trigger`, assert `resp.Code == http.StatusAccepted`.

   The test's standard `BeforeEach` builds: `ctx = context.Background()`, `sender = new(mocks.TriggerReleaseCheckCommandSender)`, `h = libhttp.NewErrorHandler(handler.NewTriggerReleaseCheckHandler(sender))`. The handler is wrapped in `libhttp.NewErrorHandler` so the 502 path (which returns an error from `ServeHTTP`) is translated to the correct HTTP status by the framework.

5. **Update the `pkg/handler` factory function in `pkg/factory/factory.go`** — `CreateTriggerReleaseCheckHandler` is added/updated to use the new thin handler. The new factory function signature:

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

   Add the import `handler "github.com/bborbe/maintainer/watcher/github-release/pkg/handler"` to the factory's import block (verify the existing import block; if `handler` is not imported, add it). If `CreateTriggerReleaseCheckHandler` does not yet exist in `factory.go`, add it. If it exists with a different signature, replace it (the project is mid-rollout, so a partial factory helper from a prior spec may exist).

6. **Regenerate the counterfeiter mocks.** Run from `/workspace/watcher/github-release/pkg/handler/`:

   ```
   cd /workspace/watcher/github-release && go generate -mod=mod ./pkg/handler/...
   ```

   This regenerates `mocks/trigger_release_check_handler.go` (and any other generated mocks in this package). Verify the new file exists:

   ```
   ls /workspace/watcher/github-release/mocks/trigger_release_check_handler.go
   ```

   The file MUST contain a `TriggerReleaseCheckHandler` struct with a `ServeHTTPStub` field, a `ServeHTTPCallCount()` method, and the `package mocks` declaration with the standard counterfeiter header.

7. **Do NOT modify `main.go` in this prompt** — prompt 4 owns the full main.go rewiring (the new sender variable, the third `run.Func`, and the `cdb.RunCommandConsumerTxDefault` setup all land together). To keep `main.go` compilable end-of-prompt-3 (so each commit is green), this prompt DOES update the factory helper `CreateTriggerReleaseCheckHandler` to its new single-arg form — the existing `main.go` call site (which passes more args, if a partial factory exists from a prior spec) will need to be updated in prompt 4. If `main.go` fails to compile after this prompt, document the issue in the completion report's `## Improvements` section but DO NOT fix it in this prompt.

8. **Run `make test` in the changed module.** From the github-release watcher dir:

   ```
   cd /workspace/watcher/github-release && make test
   ```

   Expected: exit code 0; the new `pkg/handler/` tests pass; all pre-existing tests pass unchanged.

9. **YAGNI guard.** Do NOT add a `Scope` or `Force` query parameter to the handler — both fields are reserved-unread. Do NOT add request-body parsing — the spec says "no per-request fields with meaning today". Do NOT add an HTTP-side metric increment — `pkg.Watcher.Poll(ctx)` owns all metrics. Do NOT add a `Force bool` argument to `NewTriggerReleaseCheckHandler` — Force is reserved for a follow-on spec. Do NOT add a `Scope string` argument — Scope is reserved for a follow-on spec. Do NOT add a 400-validation path — the command's `Validate` is empty today, so no input is invalid at the command level; the 502 path covers the only failure mode (Kafka unavailable).
</requirements>

<constraints>
- Schema is frozen: use `lib.GithubReleaserV1SchemaID` from `github.com/bborbe/maintainer/lib`. Do NOT define a new schema in this prompt.
- Operation string is frozen: `"trigger-release-check"`. The handler logs this constant in the `glog.V(2).Infof` call after a successful publish.
- The handler's success-path body is `{"status":"accepted"}` (HTTP 202). No additional fields — the command has no per-request field with meaning today.
- The handler's Kafka-failure path is HTTP 502 (NOT 500 or 503). The error wrap is `errors.Wrap(ctx, err, "send TriggerReleaseCheckCommand")` translated via `libhttp.WrapWithStatusCode(err, http.StatusBadGateway)`.
- Error wrapping: `github.com/bborbe/errors` only. Never `fmt.Errorf`. Always pass `ctx` to error constructors. Never `context.Background()` in `pkg/`.
- Counterfeiter annotation goes ABOVE the `type TriggerReleaseCheckHandler` line, on its own line, with `-o ../../mocks/...` (two `..` — the package is two levels below `mocks/`). The annotation MUST NOT be inside the GoDoc block.
- The mock file is generated, not hand-written. Verify the file has the `Code generated by counterfeiter. DO NOT EDIT.` header.
- Ginkgo v2 + Gomega + counterfeiter. External test package (`handler_test`). Coverage on the new code ≥ 80% per `docs/definition-of-done.md`.
- Do NOT modify `main.go` in this prompt. The factory rewiring (`CreateTriggerReleaseCheckHandler`) is owned by this prompt; the call-site update in `main.go` is owned by prompt 4.
- Do NOT modify `pkg/command/` files in this prompt — they are owned by prompts 1 and 2.
- Do NOT commit — dark-factory handles git. Branch: `dark-factory/cqrs-trigger-github-release`.
- Do NOT touch the CHANGELOG in this prompt. The CHANGELOG entry for the full spec is owned by prompt 4.
- Build verification: `cd /workspace/watcher/github-release && make test` must exit 0. If `main.go` fails to compile (because the factory's `CreateTriggerReleaseCheckHandler` signature changed and `main.go` has not been updated), document this in the completion report's `## Improvements` section but DO NOT fix it — prompt 4 owns the main.go update.
</constraints>

<verification>

Verify the handler file was rewritten and exports the expected public constructor:
```
grep -n 'func NewTriggerReleaseCheckHandler' /workspace/watcher/github-release/pkg/handler/trigger_handler.go
```
Must show the constructor signature: `(sender command.TriggerReleaseCheckCommandSender) TriggerReleaseCheckHandler`.

Verify the counterfeiter directive sits ABOVE the type declaration (spec § AC 18):
```
grep -B 1 -A 1 '//counterfeiter:generate' /workspace/watcher/github-release/pkg/handler/trigger_handler.go
```
Must show the directive on its own line directly followed (modulo blank line) by `type TriggerReleaseCheckHandler = libhttp.WithError`.

Verify the success body is exactly `{"status":"accepted"}` (spec § AC 2):
```
grep -A 4 'func writeAccepted' /workspace/watcher/github-release/pkg/handler/trigger_handler.go
```
Must show `map[string]interface{}{"status": "accepted"}`.

Verify the Kafka-failure path is 502 (NOT 500/503):
```
grep -n 'http.StatusBadGateway' /workspace/watcher/github-release/pkg/handler/trigger_handler.go
```
Must show exactly one match in the `libhttp.WrapWithStatusCode` call.

Verify the handler has no `Watcher`-typed field (spec § AC 3):
```
grep -n 'pkg.Watcher\|pkg\\.Watcher' /workspace/watcher/github-release/pkg/handler/trigger_handler.go
```
Expected: NO matches — the handler struct must have only the `sender` field.

Verify the counterfeiter mock has the generated-header:
```
head -1 /workspace/watcher/github-release/mocks/trigger_release_check_handler.go
```
Must show `// Code generated by counterfeiter. DO NOT EDIT.`

Run the new handler tests:
```
cd /workspace/watcher/github-release && go test -mod=mod -v -count=1 ./pkg/handler/...
```
Expected: exit code 0; the `TriggerHandler` Describe's `happy path`, `Kafka send failure`, and `handler struct has no Watcher-typed field` Contexts all pass.

Run the full module test suite to confirm no regression in `pkg/command/`, `pkg/factory/`, or other sibling tests (none of which are touched in this prompt):
```
cd /workspace/watcher/github-release && make test
```
Expected: exit code 0; the new `pkg/handler/` tests pass; all pre-existing tests in `pkg/command/`, `pkg/factory/`, `pkg/filter/`, `pkg/auth/` pass unchanged.

Confirm no part of the executor or main.go was touched (this prompt is scoped to the handler + factory helper):
```
git diff --stat HEAD -- /workspace/watcher/github-release/pkg/command/ /workspace/watcher/github-release/main.go
```
Expected: empty output — no changes in `pkg/command/` (owned by prompts 1 and 2) or `main.go` (owned by prompt 4).
</verification>
