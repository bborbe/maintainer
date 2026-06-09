---
status: completed
summary: Moved /trigger HTTP handler to pkg/handler/trigger_handler.go (CQRS publish + 202), wired factory.CreateTriggerBuildCheckCommandSender+Handler, refactored CreateWatcher to return the sync producer for reuse, deleted legacy pkg/trigger_handler.go + its test, updated run-once and main_poll_loop tests, generated counterfeiter mock at mocks/trigger_handler.go; make precommit passes
container: maintainer-cqrs-trigger-build-exec-259-cqrs-github-build-trigger-handler
dark-factory-version: v0.175.0
created: "2026-06-09T16:00:00Z"
queued: "2026-06-09T16:21:18Z"
started: "2026-06-09T16:43:01Z"
completed: "2026-06-09T16:53:24Z"
branch: dark-factory/cqrs-trigger-github-build
---

# Spec 068 Prompt 4 — Move /trigger Handler to pkg/handler/

## Context

This is prompt 4 of 5 for spec 068. It shrinks the HTTP `/trigger` handler to publish-via-sender + 202 + JSON body `{"status":"accepted"}`, moves it from the legacy `pkg/trigger_handler.go` to the canonical `pkg/handler/trigger_handler.go` (matching the github-release layout), and removes the old in-process `chan struct{}` handler.

**Depends on prompt 2 having landed.** The new handler depends on `command.TriggerBuildCheckCommandSender`, which is built in prompt 2.

**This prompt can ship in parallel with prompt 3** (executor). The handler does not depend on the executor; the executor does not depend on the handler. The two are joined at the factory in prompt 5.

**Mirror line-for-line** the spec 067 github-release implementation, which lives at:

- `/workspace/watcher/github-release/pkg/handler/trigger_handler.go`
- `/workspace/watcher/github-release/pkg/handler/trigger_handler_test.go`
- `/workspace/watcher/github-release/pkg/handler/doc.go`
- `/workspace/watcher/github-release/pkg/handler/suite_test.go`

Only the type/function names (`TriggerBuildCheck*`) and the `command` package reference change. The 502-on-Kafka-error path, the inline comment explaining 502 vs 500/503, the panicking-watcher test, and the JSON response shape are all verbatim.

## Goal

- Move the `/trigger` HTTP handler from `/workspace/watcher/github-build/pkg/trigger_handler.go` (in-process `chan struct{}` style) to `/workspace/watcher/github-build/pkg/handler/trigger_handler.go` (CQRS publish + 202 style).
- The new handler depends only on `command.TriggerBuildCheckCommandSender` — NO `pkg.Watcher` field, NO closure capture of a watcher.
- Kafka send failure → HTTP 502 with inline comment explaining why 502 over 500/503.
- 202 response body is `{"status":"accepted"}` (Content-Type: application/json).
- Delete the old `pkg/trigger_handler.go` and its test file.
- Generate the counterfeiter mock for `TriggerReleaseCheckHandler` (i.e. the `libhttp.WithError` alias) at `mocks/trigger_handler.go` for future test wiring.
- Add a panicking-watcher test that proves the handler has no path to the watcher on the request side (matches spec 067 AC 3 / spec 068 AC 5).

## Files to create

- `/workspace/watcher/github-build/pkg/handler/trigger_handler.go` — copy of `/workspace/watcher/github-release/pkg/handler/trigger_handler.go`. Substitutions:
  - Type alias: `type TriggerBuildCheckHandler = libhttp.WithError`.
  - Constructor: `NewTriggerBuildCheckHandler(sender command.TriggerBuildCheckCommandSender) TriggerBuildCheckHandler`.
  - Concrete struct: `triggerBuildCheckHandler{ sender command.TriggerBuildCheckCommandSender }`.
  - Counterfeiter directive line: `//counterfeiter:generate -o ../../mocks/trigger_handler.go --fake-name TriggerBuildCheckHandler . TriggerBuildCheckHandler`. Note: `--fake-name` is the EXTERNAL alias (`TriggerBuildCheckHandler`) and the source type is also `TriggerBuildCheckHandler` (it's a `type … = libhttp.WithError` alias). Mirror the github-release directive format exactly — same flag positions.
  - 502 error message: `errors.Wrap(ctx, err, "send TriggerBuildCheckCommand")` — matches the github-release wrap-text.
  - Success log: `glog.V(2).Infof("trigger accepted op=%s", command.TriggerBuildCheckCommandOperation)`.
- `/workspace/watcher/github-build/pkg/handler/trigger_handler_test.go` — copy of `/workspace/watcher/github-release/pkg/handler/trigger_handler_test.go`. Substitutions:
  - Import: `github.com/bborbe/maintainer/watcher/github-build/mocks` and `github.com/bborbe/maintainer/watcher/github-build/pkg` and `github.com/bborbe/maintainer/watcher/github-build/pkg/handler`.
  - Sender mock field: `*mocks.TriggerBuildCheckCommandSender`.
  - Handler build: `libhttp.NewErrorHandler(handler.NewTriggerBuildCheckHandler(sender))`.
  - `panickingWatcher` is unchanged — it implements `pkg.Watcher` (the github-build `pkg.Watcher` interface, which is the same shape as github-release's: `Poll(ctx context.Context) error`).
  - Test descriptions reference "spec 067 AC 3" — update to "spec 068 AC 5" for context.
- `/workspace/watcher/github-build/pkg/handler/doc.go` — copy of github-release `pkg/handler/doc.go`. Change "github-release" → "github-build".
- `/workspace/watcher/github-build/pkg/handler/suite_test.go` — verbatim copy of `/workspace/watcher/github-release/pkg/handler/suite_test.go`. Title stays "Handler Suite".

## Files to modify

- `/workspace/watcher/github-build/mocks/mocks.go` — the `//go:generate` directive already exists; counterfeiter picks it up on first `go generate`. No edit required.
- `/workspace/watcher/github-build/go.mod` — should not need a bump. If `go mod tidy` complains, run it; otherwise leave untouched.

## Files to delete

- `/workspace/watcher/github-build/pkg/trigger_handler.go` — the legacy in-process `chan struct{}` handler.
- `/workspace/watcher/github-build/pkg/trigger_handler_test.go` — the legacy handler test that depends on the deleted handler.

**Do NOT touch `main.go` in this prompt.** Main.go still references `pkg.NewTriggerHandler(trigger)` until prompt 5 lands. Without prompt 5, deleting the old handler breaks the build — but the new `pkg/handler/trigger_handler.go` and the deleted `pkg/trigger_handler.go` can coexist in the same compile unit if and only if main.go is updated to call the new path. **Therefore this prompt must update `main.go` to use the new handler** (a minimal, reversible patch: replace `pkg.NewTriggerHandler(trigger)` with the new wiring).

Concretely, the `main.go` edit (still prompt 4 — minimal):

- Remove the `trigger chan<- struct{}` parameter from `a.createHTTPServer` (it will go away entirely in prompt 5; for now, just remove the parameter and the `pkg.NewTriggerHandler` registration, replace with `a.TriggerHandler`).
- Add a `TriggerHandler http.Handler` field to `application` (mirror github-release's main.go line 59).
- Add a new factory call: `triggerHandler := factory.CreateTriggerBuildCheckHandler(factory.CreateTriggerBuildCheckCommandSender(ctx, syncProducer, branch))` and `a.TriggerHandler = libhttp.NewJSONErrorHandler(triggerHandler)`.
- Add a `base.Branch(a.Stage)` variable (named `branch`) — needed for the sender factory. (github-release main.go line 98).
- Add a sync producer dependency. Look at how the existing `CreateKafkaCreateSender` builds one — it returns `(sender, cleanup, error)`. The new sender needs a sync producer too. **Re-use the same sync producer** the existing CreateKafkaCreateSender builds, OR build a second one. Looking at github-release main.go (lines 86-96), it builds ONE sync producer and reuses it for both the create-task sender AND the trigger sender. To minimize main.go churn in this prompt, pass the existing sync producer into the new factory call. This requires a small refactor of `CreateKafkaCreateSender` to also return the sync producer — see Implementation step 4.

  Alternative: build a second sync producer for the trigger sender. This is wasteful (two connections to Kafka from one pod) but avoids touching `CreateKafkaCreateSender`. **Decision: do not add a second sync producer** — refactor `CreateKafkaCreateSender` to return the sync producer (or accept a pre-built one) and reuse. This matches github-release's main.go pattern.

  <!-- AUDIT-OPEN: Sync-producer refactor scope. The cleanest path is to refactor CreateKafkaCreateSender to accept a pre-built sync producer (matching the github-release pattern where main.go owns the sync producer lifecycle). However, this expands the prompt's surface area. The alternative — building a second sync producer for the trigger sender — works but adds a Kafka connection. The prompt body chooses the refactor path; if the executor finds this too invasive, the safer fallback is to build a second sync producer in main.go and document the dual-connection cost. -->

## Out of scope

- Do NOT touch the executor (prompt 3) or the factory (prompt 5). The new handler is a self-contained replacement for the old one.
- Do NOT update `main_poll_loop_test.go`. That test is bound to the OLD `runPollLoop(poll, interval, trigger)` signature, which main.go keeps in this prompt (prompt 5 is the one that drops the trigger parameter and rewrites that test). The test still passes in this prompt because `runPollLoop`'s signature has not changed.
- Do NOT enable `SendResultEnabled` (false everywhere).
- Do NOT change the HTTP mount path (`/trigger` under `/admin` at the gateway).

## Implementation

1. Read `/workspace/watcher/github-release/pkg/handler/trigger_handler.go` fully. The handler struct has exactly one field (`sender command.TriggerReleaseCheckCommandSender`). The `ServeHTTP` method builds a zero-value `TriggerReleaseCheckCommand{}` and calls `h.sender.SendCommand(ctx, ...)`. On error, wrap with `libhttp.WrapWithStatusCode(..., http.StatusBadGateway)`. On success, return `writeAccepted(resp)`.

2. The inline comment in the handler explaining 502 vs 500/503 is REQUIRED (spec 068 AC 7). Keep it verbatim — same wording, same structure as github-release. This is a load-bearing audit AC.

3. The 502 wrap path uses `libhttp.WrapWithStatusCode` (verified at `/home/node/go/pkg/mod/github.com/bborbe/http@v1.26.10/http_error-handler.go:32`). The handler is wrapped by `libhttp.NewErrorHandler` at the call site (in main.go / test), which translates the `ErrorWithStatusCode` into an HTTP response with that status code.

4. For the main.go refactor (see AUDIT-OPEN note above): change `CreateKafkaCreateSender` to accept a pre-built `syncProducer libkafka.SyncProducer` and `branch base.Branch`, returning `(task.CreateCommandSender, error)` (no cleanup — the caller owns the producer now). Move the producer construction into main.go (mirror github-release main.go lines 86-96). Then construct the create-task sender AND the trigger-build-check sender AND the trigger-build-check command sender all from the same `syncProducer`. Mirror github-release main.go lines 86-152.

   After this refactor, main.go's `createHTTPServer` no longer takes a `chan<- struct{}` parameter (since the new handler doesn't need it). For this prompt, pass `nil` or remove the parameter entirely — the choice depends on whether the prompt 5 final shape retains the parameter. **Decision: remove the parameter entirely in this prompt.** The clean shutdown refactor in prompt 5 will then only need to remove the `trigger` channel allocation and the `runPollLoop` trigger parameter.

5. Generate the counterfeiter mock for the new handler:

   ```
   cd /workspace/watcher/github-build && go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate
   ```

   Verify `mocks/trigger_handler.go` is present and starts with `// Code generated by counterfeiter. DO NOT EDIT.`. The fake struct MUST be named `TriggerBuildCheckHandler`.

## Tests

`trigger_handler_test.go` covers:

- "happy path":
  - "returns 202 with `{status:accepted}` body": POST `/trigger`, assert 202, Content-Type `application/json`, body decodes to `{"status":"accepted"}` (one key, value "accepted").
  - "publishes exactly one zero-value TriggerBuildCheckCommand": assert `sender.SendCommandCallCount() == 1` and the sent command's `Scope == ""` and `Force == false`.
- "Kafka send failure":
  - "returns 502": with `sender.SendCommandReturns(errors.Errorf(ctx, "kafka error"))`, POST `/trigger`, assert 502.
- "handler struct has no Watcher-typed field (spec 068 AC 5)":
  - "handler struct has no Watcher-typed field": build a concrete handler via `handler.NewTriggerBuildCheckHandler(nil)`, use `reflect.TypeOf` to assert no field's type implements `pkg.Watcher`.
  - "request completes with 202 (no Watcher wired anywhere)": POST `/trigger`, assert 202 — proves the handler has no global watcher reference.
  - "request completes with 202 even when a panicking Watcher is constructed alongside": construct a `panickingWatcher` (a minimal `pkg.Watcher` whose `Poll` panics) in test scope, POST `/trigger`, assert 202 — proves the handler does not capture any watcher through closure or package-level state.

The `panickingWatcher` type is defined at the bottom of the test file (matches the github-release pattern). It implements `pkg.Watcher` (Poll only — same as the github-build `pkg.Watcher` interface).

## Verification

```
cd /workspace/watcher/github-build && make precommit
echo "exit=$?"
ls /workspace/watcher/github-build/pkg/handler/trigger_handler.go
ls /workspace/watcher/github-build/mocks/trigger_handler.go
test -f /workspace/watcher/github-build/pkg/trigger_handler.go && echo "OLD HANDLER STILL EXISTS" || echo "old handler removed"
test -f /workspace/watcher/github-build/pkg/trigger_handler_test.go && echo "OLD TEST STILL EXISTS" || echo "old test removed"
```

Precommit exits 0. The two `ls` calls succeed. The two `test -f` checks print "removed" for both legacy files.

## Lessons from spec 067 audit (apply at write time)

1. Handler struct must have exactly one field: the sender. NO `watcher pkg.Watcher` field. The reflection test (`reflect.TypeOf(handler).NumField()`) will catch any regression here — keep it.
2. The 502 BadGateway response has an inline comment explaining 502 vs 500/503 (lesson 9). Verbatim copy from github-release. Operators + observability tools depend on this distinction.
3. Counterfeiter directive ABOVE the type declaration (lesson 6). For a `type X = libhttp.WithError` alias, the directive still works because counterfeiter treats the alias as a source type.
4. The 202 response body is exactly `{"status":"accepted"}` — Content-Length 21, Content-Type `application/json`. The test asserts both.
5. `libhttp.NewErrorHandler` wraps the handler at the call site (main.go and test). The `TriggerHandler http.Handler` field on `application` receives the wrapped form (`libhttp.NewJSONErrorHandler(...)`).
6. `panickingWatcher` is a load-bearing test artifact. It is never injected into the handler; it exists only to prove the handler has no path to a Watcher instance. Keep it.
7. The handler must not be parameterized by request body or query string (spec 068 Security section: "attacker-controllable input on `/trigger` is therefore an empty surface for this spec"). The handler builds `command.TriggerBuildCheckCommand{}` from defaults — never reads from the request.
8. The handler's success log is `glog.V(2).Infof("trigger accepted op=%s", command.TriggerBuildCheckCommandOperation)`. Do NOT log the body, headers, or path. Operators read the V(2) log line; it must not contain sensitive data.
9. The handler test uses `httptest.NewRequest("POST", "/trigger", nil)` — explicit method, explicit path, nil body. Do NOT switch to `http.MethodPost` (the github-release test uses the literal `"POST"`).
10. BSD copyright header on every new file, dated 2026.

## Improvements

(empty — YOLO fills in after running)
