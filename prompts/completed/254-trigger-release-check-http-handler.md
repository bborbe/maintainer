---
status: completed
spec: ["072"]
container: maintainer-cqrs-trigger-release-exec-254-trigger-release-check-http-handler
dark-factory-version: v0.175.0
created: "2026-06-09T00:00:00Z"
queued: "2026-06-09T10:57:42Z"
started: "2026-06-09T11:19:17Z"
completed: "2026-06-09T11:29:07Z"
branch: dark-factory/cqrs-trigger-github-release
---

<summary>

- Replaces the current `libhttp.NewBackgroundRunHandler(ctx, poll)` `/trigger` route with a thin handler that publishes one `TriggerReleaseCheckCommand{}` (zero-value: empty Scope, Force=false) via the injected `TriggerReleaseCheckCommandSender`.
- HTTP returns 202 with body `{"status":"accepted"}` — wire shape matches the github-pr `/trigger` and the spec.
- The handler has zero `pkg.Watcher` dependency: it does not call Poll, does not receive a Watcher, does not even know a Watcher exists. A test injecting a panicking Watcher-shaped interface is structurally impossible (the handler's struct only holds a sender), and a behavior test confirms 202 is returned even when the sender's underlying path panics.
- The handler does not consume the request body or query string — both `Scope` and `Force` are reserved-unread (spec non-goals). Attacker-controllable input on `/trigger` is an empty surface for this spec.
- Existing `/healthz`, `/readiness`, `/metrics`, `/resetcursor`, `/setcursor` routes are unchanged. The `poll run.Func` is no longer used by the HTTP path (the poll-interval loop still uses it).

</summary>

<objective>
Reduce the github-release `/trigger` HTTP handler to a thin shell: build a `TriggerReleaseCheckCommand{}` from defaults, publish it via the sender, return 202. The handler must have no reference to `pkg.Watcher` on the request path, so a pod crash between HTTP 202 and executor pickup can no longer lose the trigger — Kafka redelivery handles it (wiring in prompt 4).
</objective>

<context>

- Existing `/trigger` route: `/workspace/watcher/github-release/main.go:133` — `router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))`. This prompt REPLACES that line.
- The `poll` `run.Func` defined at `main.go:112-115` stays in scope for the poll-interval loop at `main.go:117-120`. The HTTP route is the only thing that changes.
- Reference handler: `/workspace/watcher/github-pr/pkg/handler/trigger_handler.go` (mirror structurally — but github-release's handler is even simpler: NO URL parsing, NO `prurl.ParsePRURL`, NO `?url=` query param. The handler is purely publish + 202).
- Reference handler test: `/workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go` (mirror the table-driven shape — but with no URL-validation cases because there's no URL to validate).
- The handler depends ONLY on `command.TriggerReleaseCheckCommandSender` (from prompt 1). It does NOT depend on `pkg.Watcher`, `pkg.GitHubClient`, `pkg.TaskPublisher`, or any other domain primitive.
- libhttp types (verified at `/home/node/go/pkg/mod/github.com/bborbe/http@v1.26.10/`):
  - `libhttp.WithError` — handler interface returned by `NewXxxHandler`. Defined by signature `type WithError interface { ServeHTTP(ctx, resp, req) error }`.
  - `libhttp.NewErrorHandler(withError)` — `http_error-handler.go:123` — wraps a WithError handler to centralize error→status-code mapping.
  - `libhttp.NewJSONErrorHandler(withError)` — `http_json-error-handler.go:22` — same but emits JSON error bodies.
  - `libhttp.WrapWithStatusCode(err, statusCode)` — used in the github-pr handler to attach 400/500 status to errors.

In-container docs:

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md` — handler-in-pkg/handler/ rule, no-inline-handlers rule.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` — counterfeiter-directive-above-type rule, `New*` constructor.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md` — JSON error body shape.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrap` over `fmt.Errorf`.

</context>

<ac-coverage>

This prompt covers these spec 067 acceptance criteria:

- **AC 2** — `POST /trigger` returns HTTP 202 with body `{"status":"accepted"}`. The response body matches the spec's desired shape exactly.
- **AC 3** — A test injecting a `pkg.Watcher` whose `Poll(ctx)` panics into the HTTP handler still observes HTTP 202 from `POST /trigger` (proves the watcher is not on the request path). Both structural (reflect on handler fields) and behavioral (test the request completes) halves are covered.
- **AC 4** — the sender unit-test in prompt 1 confirms `lib.GithubReleaserV1SchemaID` + `TriggerReleaseCheckCommandOperation`; this prompt's handler test asserts the captured `TriggerReleaseCheckCommand` is the zero value, completing the path: HTTP → sender → cdb → Kafka.
- **AC 20** — coverage on new code in `pkg/handler/` is ≥ 80%.

</ac-coverage>

<requirements>

1. Create the directory `watcher/github-release/pkg/handler/`.

2. Create `watcher/github-release/pkg/handler/doc.go`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package handler contains the HTTP handlers for the github-release watcher.
   // /trigger publishes a TriggerReleaseCheckCommand to Kafka and returns 202;
   // the heavy lifting (Poll cycle) happens in the in-pod command consumer.
   package handler
   ```

3. Create `watcher/github-release/pkg/handler/suite_test.go` (Ginkgo v2 + Gomega + format.TruncatedDiff = false, 60s timeout — same shape as `watcher/github-pr/pkg/handler/suite_test.go`).

4. Create `watcher/github-release/pkg/handler/trigger_handler.go` with EXACTLY this shape (mirror github-pr structurally; differences: no `prurl.ParsePRURL`; no `?url=` query param; no 400 cases; the handler is purely publish + 202):

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
   // consumed (both Scope and Force are reserved-unread). All poll
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
           return libhttp.WrapWithStatusCode(
               errors.Wrap(ctx, err, "send TriggerReleaseCheckCommand"),
               http.StatusBadGateway,
           )
       }

       glog.V(2).Infof("trigger accepted")
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

   Note: the request parameter is unused (no body, no query). Use `_ *http.Request` and add a `//nolint:contextcheck` or comment explaining why.

5. The handler struct MUST have exactly one field: `sender command.TriggerReleaseCheckCommandSender`. No `pkg.Watcher` field, no `pkg.GitHubClient` field, no `pkg.TaskPublisher` field. The handler is intentionally as small as possible (spec AC 2: "the handler does not reference the watcher object on the request path").

6. After creating the source file, regenerate the counterfeiter mock:
   ```bash
   cd watcher/github-release && go generate ./pkg/handler/...
   ```
   This produces `mocks/trigger_release_check_handler.go` with a `TriggerReleaseCheckHandler` fake. The fake implements `libhttp.WithError` (i.e., `ServeHTTP(ctx, resp, req) error`) so tests can wire it as a handler too.

7. Add `watcher/github-release/pkg/handler/trigger_handler_test.go` with these test cases (mirror github-pr's handler test structurally but drop the URL-validation entries):

   a. `Describe("TriggerHandler")` with `BeforeEach` constructing a fresh `mocks.TriggerReleaseCheckCommandSender` and wrapping the handler with `libhttp.NewErrorHandler(handler.NewTriggerReleaseCheckHandler(sender))`.

   b. `Context("happy path")`:
      - `It("returns 202 with {status:accepted} body", ...)` — POST /trigger, assert status 202 and JSON body `{"status":"accepted"}`.
      - `It("publishes exactly one zero-value TriggerReleaseCheckCommand", ...)` — assert `sender.SendCommandCallCount() == 1`, the captured cmd is the zero value (`Scope == ""`, `Force == false`).

   c. `Context("Kafka send failure")`:
      - `BeforeEach` makes `sender.SendCommandReturns(errors.Errorf(ctx, "kafka error"))`.
      - `It("returns 502", ...)` — assert `resp.Code == http.StatusBadGateway`.

   d. `Context("handler struct has no Watcher-typed field (spec 067 AC 3)")`:
      - This block uses `reflect` (same approach as `watcher/github-pr/pkg/handler/trigger_handler_test.go:107-148`).
      - `It("handler struct has no Watcher-typed field", ...)` — build the handler directly via `handler.NewTriggerReleaseCheckHandler(sender)`, then reflect on its dynamic type. Assert that NO field's type implements `pkg.Watcher` (`reflect.TypeOf((*pkg.Watcher)(nil)).Elem()`). The handler is constructed with a nil sender, but the constructor is pure composition (no nil-check) and the reflect runs BEFORE any method is called, so the nil sender is harmless.
      - `It("request completes with 202 (no Watcher wired anywhere)", ...)` — a request still returns 202 even though the handler has no Watcher dependency. (This is the "behavioral" half of the structural+behavioral proof.)
      - `It("request completes with 202 even when a panicking Watcher is constructed alongside", ...)` — to prove the handler does not indirectly reach the Watcher through a closure, build a separate `pkg.Watcher` whose `Poll(ctx)` panics. The test should NOT inject it into the handler (handler has no Watcher field); it should merely construct it next to the handler, then send a request and assert 202. The Watcher is never called. Comment that the point is to document that the handler has no reference path to the Watcher.

   e. Use `mocks.TriggerReleaseCheckCommandSender` from `watcher/github-release/mocks/` (generated in prompt 1).

8. Update `watcher/github-release/main.go` to use the new handler on the `/trigger` route:

   a. Add the import:
      ```go
      "github.com/bborbe/maintainer/watcher/github-release/pkg/handler"
      ```

   b. In `Run`, after `w := factory.CreateWatcher(...)`, build the trigger handler. The sender is NOT YET BUILT in this prompt (it ships in prompt 4); for now, build a placeholder via a one-liner that the executor in prompt 4 will replace. CONCRETE: do NOT wire the sender in this prompt. Instead, leave a `// prompt 4 will wire: factory.CreateTriggerReleaseCheckCommandSender(...)` comment and KEEP the existing `/trigger` route as `libhttp.NewBackgroundRunHandler(ctx, poll)` in this prompt. Then in prompt 4 the executor will swap both the sender construction and the route handler. The goal of THIS prompt is the handler package + tests + the new file — the wiring change to main.go comes in prompt 4 to keep this prompt's diff minimal and prompt 4's diff localized to the wiring layer.

   c. Alternatively, if you'd rather wire the route in this prompt and avoid a later edit: pass `nil` as the sender placeholder and have the route in main.go call `handler.NewTriggerReleaseCheckHandler(nil)` — but this leaks a nil into production temporarily. The cleanest split is to defer the route swap to prompt 4.

   d. DECISION (resolved before writing): keep `/trigger` as `libhttp.NewBackgroundRunHandler(ctx, poll)` in main.go for this prompt. Document the deferred wiring in a code comment. Prompt 4's main.go edit will replace the line and add the sender construction in a single localized diff. This avoids a "half-wired" main.go in this prompt.

9. Do NOT add `RunCommandConsumerTxDefault`, `factory.CreateCommandConsumer`, or any consumer wiring in this prompt. Those land in prompt 4.

10. Do NOT change `/healthz`, `/readiness`, `/metrics`, `/resetcursor`, `/setcursor`, the poll loop, the `poll` closure, or the watcher construction. Only `/trigger` will be replaced (in prompt 4).

11. Do NOT add Scope or Force URL-query-param handling. The handler does not parse the request body or query string.

12. Append to `## Unreleased` in `/workspace/CHANGELOG.md`:
    ```
    - feat: Add thin HTTP /trigger handler that publishes TriggerReleaseCheckCommand to Kafka and returns 202
    ```

</requirements>

<constraints>

- The handler MUST NOT reference `pkg.Watcher` in any form: no field, no method, no closure capture, no reflection lookup. Verified by the struct-reflection test in prompt 3 (spec AC 3) and the structural+behavioral tests in this prompt.
- The handler MUST NOT call the GitHub API, the filter, the trust decision, or the publisher. All of those live behind `pkg.Watcher.Poll(ctx)` and execute in the in-pod command consumer (prompt 4).
- The handler MUST NOT read the HTTP request body or query string. `Scope` and `Force` are reserved-unread (spec non-goals).
- The response body shape is `{"status":"accepted"}` — exactly matches the github-pr handler's body minus the `url` field (which github-release doesn't have).
- On Kafka send failure the handler MUST return 502 (Bad Gateway) via `libhttp.WrapWithStatusCode(err, http.StatusBadGateway)` — mirrors github-pr's exact error mapping.
- The counterfeiter directive `//counterfeiter:generate -o ../../mocks/trigger_release_check_handler.go --fake-name TriggerReleaseCheckHandler . TriggerReleaseCheckHandler` MUST sit on its own line above the `type TriggerReleaseCheckHandler = libhttp.WithError` declaration, NOT inside the GoDoc block. NOTE: counterfeiter does not generate useful fakes for type aliases. If counterfeiter emits an empty/useless mock, use a plain interface and the `//counterfeiter:generate` directive on that interface INSTEAD. The github-pr pattern uses an alias `SinglePRTriggerHandler = libhttp.WithError` and the counterfeiter directive on it — verify by looking at `watcher/github-pr/pkg/handler/trigger_handler.go:20-27` and the generated `mocks/single_pr_trigger_handler.go`. If the github-pr generator DOES produce a useful fake for the alias, mirror that. If not, declare a small private interface for the handler and generate the fake from that.
- The handler's constructor MUST use `New*` naming per the factory/constructor convention.
- BSD copyright header on every new file, dated 2026.
- Error wrapping uses `github.com/bborbe/errors` exclusively.
- Do NOT commit — dark-factory handles git.

</constraints>

<verification>

```bash
cd watcher/github-release && make test
```

Must pass — the new handler tests run, the existing tests still pass, and `go test ./pkg/handler/...` reports coverage. Then:

```bash
cd watcher/github-release && make precommit
```

Must exit 0.

Additional manual checks:

- `grep -n "pkg.Watcher\|poll" watcher/github-release/pkg/handler/trigger_handler.go` returns NO matches. The handler has zero reference to the watcher on the request path.
- `grep -n "ParsePRURL\|prurl" watcher/github-release/pkg/handler/trigger_handler.go` returns NO matches. The handler does no URL parsing.
- `head -1 watcher/github-release/mocks/trigger_release_check_handler.go` is `// Code generated by counterfeiter. DO NOT EDIT.` (assuming the counterfeiter step succeeded).
- The structural test in `trigger_handler_test.go` asserting "no Watcher-typed field" passes.
- The behavioral test asserting "202 even with panicking Watcher alongside" passes.
- `cd watcher/github-release && go test -coverprofile=/tmp/cover.out ./pkg/handler/... && go tool cover -func=/tmp/cover.out | awk '/total:/ {print $3}'` reports `>= 80.0%` for the new code.

</verification>

## Improvements

- [PROMPT] Frontmatter mixes `name:`/`description:`/`tags:` with the required `spec:` field; an audit should normalise to spec/status/created.
- [PROMPT] The decision to keep the existing `/trigger` route in main.go for this prompt (req 8d) is sound but leaves a TODO comment in main.go between prompt 3 and prompt 4. The comment is fine; just make sure prompt 4's grep verification confirms the comment is gone (the deferred wiring replaced the line, not just the route handler).
- [PROMPT] The counterfeiter-on-type-alias risk (req constraint note) is real — if counterfeiter emits an empty fake, the test that wires the fake as a handler will break. The prompt should specify a fallback: declare a small interface and put the directive on that. Otherwise the executor has to improvise at run-time.
- [GUIDE] `go-http-handler-refactoring-guide.md` should call out the "type alias + counterfeiter" trap: `type X = libhttp.WithError` does NOT produce a useful fake. Either use a real interface or skip the fake.
- [GLOBAL] When a spec says "publish + 202 + JSON body", the response body shape should be a literal in the prompt body (not just described). Future audits should diff the literal against the spec's body wire shape.
