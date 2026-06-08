---
status: approved
spec: [066-cqrs-trigger-github-pr]
created: "2026-06-08T21:11:59Z"
queued: "2026-06-08T21:49:51Z"
branch: dark-factory/cqrs-trigger-github-pr
---

<summary>
- `pkg/handler/trigger_handler.go` is rewritten to be a thin shell: parse `url` query param → call `TriggerPRReviewCommand.Validate` (prompt 1) → publish via `TriggerPRReviewCommandSender` (prompt 1) → write HTTP 202 with body `{"status":"accepted","url":"<raw_url>"}`.
- All GitHub API access, filter evaluation, and trust decision logic is removed from the request path. The handler no longer depends on `pkg.GitHubClient`, `filter.TaskCreationFilter`, `trust.Trust`, or `pkg.Metrics` — its only outbound calls are `ParsePRURL` (for a duplicate URL parse that drives the 400 check) and `Sender.SendCommand` (for the Kafka publish).
- A new "panicking GitHub client" test wires a `*mocks.GitHubClient` whose `GetPRDetailsStub` calls `panic("boom")` into the HTTP handler's dependency graph. The test asserts that `POST /trigger?url=<valid>` STILL returns HTTP 202 — the GitHub client is not on the request path, so the panic must not propagate to the HTTP response. This is spec § AC 5.
- The wire shape on the success path changes from `{"status":"ok","task_id":"...","repo":"...","pr_number":N,"head_sha":"..."}` (HTTP 200) to `{"status":"accepted","url":"<raw_url>"}` (HTTP 202). The error path stays 400 for invalid URLs (no Kafka publish).
- The factory function `factory.CreateSinglePRTriggerHandler` is updated: its parameter list shrinks to `(sender command.TriggerPRReviewCommandSender)` — no more `httpClient`, `createSender`, `taskCreationFilter`, `trustDecision`, `stage`, `maxSlugLen`, `maxTitleLen`, `taskSuffix`, or `metrics` parameters. The factory's existing test (`pkg/factory/single_pr_test.go`) is updated to match the new signature.
- The `singlePRTriggerHandler.ServeHTTP` body is replaced end-to-end. The helpers `buildFilterPR` and `buildPullRequest` are removed (no longer used in the handler). The `writeSuccess` helper is replaced with a `writeAccepted` helper that emits the 202 shape.
- No CHANGELOG entry in this prompt (prompt 4 owns the spec-level CHANGELOG).

This is prompt 3 of 4 for spec 066. It depends on prompt 1 (the sender type). It does NOT depend on prompt 2 (the executor) — the HTTP handler shrink is independent of the executor's internals, only its public type. The factory rewiring happens in prompt 4.
</summary>

<objective>
Reduce the HTTP handler to its minimum viable behavior: parse the URL, validate it, publish a `TriggerPRReviewCommand` to Kafka, return 202. The handler must hold no GitHub/filter/trust dependencies on the request path. This is the user-visible wire-compatibility change (200 → 202, body shape change) that operators will see on `POST /trigger`.

After this prompt, a `POST /trigger?url=<bad>` still returns 400 (no Kafka message), and a `POST /trigger?url=<valid>` returns 202 in well under 100 ms regardless of how slow GitHub is for the target PR — the work moves to the consumer.
</objective>

<context>
Read `/workspace/CLAUDE.md` and `/workspace/watcher/github-pr/CLAUDE.md` (if present) for project conventions.

Read these source files in full BEFORE editing:

- `/workspace/watcher/github-pr/pkg/handler/trigger_handler.go` — the file being rewritten. The new file SHARES the same package name (`handler`) and the same exported alias `SinglePRTriggerHandler = libhttp.WithError` (so the wiring in `main.go` line 300 stays byte-identical). The struct's fields shrink dramatically: only `sender command.TriggerPRReviewCommandSender` remains. The `ServeHTTP` body becomes ~25 lines.
- `/workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go` — the existing test file. MOST of the existing tests are no longer applicable (they assert behavior the handler no longer has: GitHub 502 paths, filter 422 paths, Kafka 502 paths, trust-branching frontmatter assertions). The new test file retains the table-driven "error cases" (4 entries: missing/empty/invalid/non-github) and adds the panicking-GitHub-client test plus a new happy-path test (asserts 202 + `{"status":"accepted","url":"..."}` body + `sender.SendCommandCallCount() == 1`).
- `/workspace/watcher/github-pr/pkg/handler/suite_test.go` — the Ginkgo suite. The `go:generate counterfeiter` directive at the top runs counterfeiter for any `//counterfeiter:generate` annotation. The new test file uses the `mocks.TriggerPRReviewCommandSender` (prompt 1) as the new HTTP-side mock.
- `/workspace/watcher/github-pr/pkg/factory/single_pr.go` — the existing `CreateSinglePRTriggerHandler` factory. The parameter list shrinks to `(sender command.TriggerPRReviewCommandSender) handler.SinglePRTriggerHandler`. The nil-checks for `httpClient`, `createSender`, `taskCreationFilter`, `trustDecision` are removed; a single nil-check for `sender` is added.
- `/workspace/watcher/github-pr/pkg/factory/single_pr_test.go` — the existing factory test. All four nil-check entries (`httpClient`, `createSender`, `taskCreationFilter`, `trustDecision`) become irrelevant. The "non-nil handler" entry stays but the parameter list shrinks to one argument.
- `/workspace/watcher/github-pr/mocks/single_pr_trigger_handler.go` — the existing counterfeiter mock for `SinglePRTriggerHandler`. This mock is still USED by `pkg/handler/single_pr_trigger_handler_test.go` (if any) — verify the file's callers: `grep -rn 'mocks.SinglePRTriggerHandler' /workspace/watcher/github-pr/`. If the mock has no callers after the handler rewrite, the file can stay (unused, harmless); the next `make generate` run (prompt 4) will keep it. If it does have callers, do NOT break them.
- `/workspace/prompts/1-spec-066-trigger-pr-review-command.md` — the prompt 1 output. The new HTTP handler imports `command.TriggerPRReviewCommand`, `command.TriggerPRReviewCommandSender`, `command.TriggerPRReviewCommandOperation` from this package. Verify the package exists before editing: `ls /workspace/watcher/github-pr/pkg/command/`. If empty, STOP and report `status: failed`.
- `/workspace/watcher/github-pr/main.go` lines 289-300 — the existing `factory.CreateSinglePRTriggerHandler(...)` call site. **DO NOT EDIT `main.go` in this prompt** — prompt 4 owns the full main.go rewiring (the new sender variable, the third `run.Func`, and the `cdb.RunCommandConsumerTxDefault` setup all land together). To keep `main.go` compilable end-of-prompt-3 (so each commit is green), this prompt **adds a backward-compatible adapter** to `factory.CreateSinglePRTriggerHandler` — see Requirements § 3. The legacy 9-arg signature stays callable until prompt 4 swaps the call site to the new single-arg form.
- `/workspace/lib/prurl/prurl.go` — `prurl.ParsePRURL` and `prurl.PlatformGitHub`. The handler still uses these for the 400-on-non-GitHub check. **The HTTP handler does NOT call `TriggerPRReviewCommand.Validate`** for the 400 path — it calls `prurl.ParsePRURL` directly because the spec's response shape (HTTP 400 + specific error string) is different from `Validate`'s error shape (`errors.Wrapf(ctx, validation.Error, "...")`). The handler keeps the 400-on-bad-URL behavior it had before, but builds the response from `prurl.ParsePRURL` errors directly (NOT from `Validate` errors) so the response wording is unchanged for operators.

Reference the HTTP handler patterns in the agent project:

- `/home/node/go/pkg/mod/github.com/bborbe/agent/lib@v0.65.0/command/task/create-command-sender.go` lines 37-65 — the `SendCommand` method shape. The HTTP handler calls `sender.SendCommand(ctx, command.TriggerPRReviewCommand{URL: rawURL, Force: false})` exactly once.

Coding plugin docs (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-http-handler-refactoring-guide.md` — handlers in `pkg/handler/`, factory in `pkg/factory/`, no inline handlers in `main.go`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-json-error-handler-guide.md` — JSON error responses; the 400 path uses `libhttp.WrapWithStatusCode` (already in the file).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrapf` over `fmt.Errorf`.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter; external test package.
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

       "github.com/bborbe/maintainer/lib/prurl"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/command"
   )

   // SinglePRTriggerHandler handles POST /trigger?url=<pr_url>.
   // The handler is intentionally thin: parse the URL, validate it
   // synchronously, publish a TriggerPRReviewCommand to Kafka, and
   // return HTTP 202. All GitHub API access, filter evaluation, and
   // trust decision logic is owned by the in-pod command consumer.
   //
   //counterfeiter:generate -o ../../mocks/single_pr_trigger_handler.go --fake-name SinglePRTriggerHandler . SinglePRTriggerHandler

   type SinglePRTriggerHandler = libhttp.WithError

   // NewSinglePRTriggerHandler returns a handler that publishes a
   // TriggerPRReviewCommand to Kafka for each valid /trigger request.
   func NewSinglePRTriggerHandler(
       sender command.TriggerPRReviewCommandSender,
   ) SinglePRTriggerHandler {
       return &singlePRTriggerHandler{
           sender: sender,
       }
   }

   type singlePRTriggerHandler struct {
       sender command.TriggerPRReviewCommandSender
   }

   func (h *singlePRTriggerHandler) ServeHTTP(
       ctx context.Context,
       resp http.ResponseWriter,
       req *http.Request,
   ) error {
       rawURL := req.URL.Query().Get("url")
       if err := validateTriggerURL(ctx, rawURL); err != nil {
           return err
       }

       if err := h.sender.SendCommand(ctx, command.TriggerPRReviewCommand{
           URL:   rawURL,
           Force: false,
       }); err != nil {
           return libhttp.WrapWithStatusCode(
               errors.Wrap(ctx, err, "send TriggerPRReviewCommand"),
               http.StatusBadGateway,
           )
       }

       glog.V(2).Infof("trigger accepted url=%s", rawURL)
       return writeAccepted(resp, rawURL)
   }

   // validateTriggerURL rejects empty URLs, unparseable URLs, and
   // non-GitHub platforms with HTTP 400. Mirrors the old parseAndValidateURL
   // behavior so the 400 wire shape is unchanged for operators.
   func validateTriggerURL(ctx context.Context, rawURL string) error {
       if rawURL == "" {
           return libhttp.WrapWithStatusCode(
               errors.Errorf(ctx, "url query parameter is required"),
               http.StatusBadRequest,
           )
       }
       prInfo, err := prurl.ParsePRURL(ctx, rawURL)
       if err != nil {
           return libhttp.WrapWithStatusCode(
               errors.Wrap(ctx, err, "parse PR URL"),
               http.StatusBadRequest,
           )
       }
       if prInfo.Platform != prurl.PlatformGitHub {
           return libhttp.WrapWithStatusCode(
               errors.Errorf(ctx, "only github platform is supported, got %s", prInfo.Platform),
               http.StatusBadRequest,
           )
       }
       return nil
   }

   // writeAccepted emits the 202 response with body {"status":"accepted","url":<raw>}.
   func writeAccepted(resp http.ResponseWriter, rawURL string) error {
       resp.Header().Set("Content-Type", "application/json")
       resp.WriteHeader(http.StatusAccepted)
       return json.NewEncoder(resp).Encode(map[string]interface{}{
           "status": "accepted",
           "url":    rawURL,
       })
   }
   ```

   Key shape changes vs the old file:
   - The struct shrinks from 9 fields to 1 field (`sender`).
   - The constructor shrinks from 9 args to 1 arg.
   - `ServeHTTP` body is ~20 lines (was ~75).
   - `buildFilterPR` and `buildPullRequest` are removed (used only by the old body).
   - `writeSuccess` is replaced by `writeAccepted` (different status code, different body shape).
   - `parseAndValidateURL` is replaced by `validateTriggerURL` (returns `libhttp.WrapWithStatusCode`-wrapped errors directly).
   - The `counterfeiter:generate` annotation on `SinglePRTriggerHandler = libhttp.WithError` STAYS — it is the alias declaration, not a real interface, and counterfeiter handles this case (the existing mock file is unchanged).

2. **Rewrite `pkg/handler/trigger_handler_test.go`** end-to-end. The new test file:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package handler_test

   import (
       "context"
       "encoding/json"
       "net/http"
       "net/http/httptest"

       "github.com/bborbe/errors"
       libhttp "github.com/bborbe/http"
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-pr/mocks"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/command"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
   )

   var _ = Describe("TriggerHandler", func() {
       var (
           ctx    context.Context
           sender *mocks.TriggerPRReviewCommandSender
           h      http.Handler
       )

       BeforeEach(func() {
           ctx = context.Background()
           sender = new(mocks.TriggerPRReviewCommandSender)
           h = libhttp.NewErrorHandler(handler.NewSinglePRTriggerHandler(sender))
       })

       DescribeTable("error cases (400, no Kafka publish)",
           func(rawURL string) {
               sender.SendCommandReturns(nil) // should not be called
               req := httptest.NewRequest("POST", "/trigger?"+rawURL, nil)
               resp := httptest.NewRecorder()
               h.ServeHTTP(resp, req)
               Expect(resp.Code).To(Equal(http.StatusBadRequest))
               Expect(sender.SendCommandCallCount()).To(Equal(0),
                   "SendCommand must not be called for invalid URL")
           },
           Entry("missing url returns 400", "foo=bar"),
           Entry("empty url returns 400", "url="),
           Entry("invalid url returns 400", "url=not-a-url"),
           Entry("non-github platform returns 400", "url=https://bitbucket.org/owner/repo/pull-requests/1"),
       )

       Context("happy path: valid GitHub PR URL", func() {
           It("returns 202 with {status,url} body", func() {
               req := httptest.NewRequest(
                   "POST",
                   "/trigger?url=https://github.com/bborbe/repo/pull/42",
                   nil,
               )
               resp := httptest.NewRecorder()
               h.ServeHTTP(resp, req)

               Expect(resp.Code).To(Equal(http.StatusAccepted))
               var body map[string]interface{}
               Expect(json.Unmarshal(resp.Body.Bytes(), &body)).To(Succeed())
               Expect(body["status"]).To(Equal("accepted"))
               Expect(body["url"]).To(Equal("https://github.com/bborbe/repo/pull/42"))
           })

           It("publishes exactly one TriggerPRReviewCommand with the raw URL and Force=false", func() {
               req := httptest.NewRequest(
                   "POST",
                   "/trigger?url=https://github.com/bborbe/repo/pull/42",
                   nil,
               )
               resp := httptest.NewRecorder()
               h.ServeHTTP(resp, req)

               Expect(sender.SendCommandCallCount()).To(Equal(1))
               _, sentCmd := sender.SendCommandArgsForCall(0)
               Expect(sentCmd.URL).To(Equal("https://github.com/bborbe/repo/pull/42"))
               Expect(sentCmd.Force).To(BeFalse())
           })
       })

       Context("Kafka send failure", func() {
           BeforeEach(func() {
               sender.SendCommandReturns(errors.Errorf(ctx, "kafka error"))
           })

           It("returns 502", func() {
               req := httptest.NewRequest(
                   "POST",
                   "/trigger?url=https://github.com/bborbe/repo/pull/42",
                   nil,
               )
               resp := httptest.NewRecorder()
               h.ServeHTTP(resp, req)
               Expect(resp.Code).To(Equal(http.StatusBadGateway))
           })
       })

       Context("GitHub client off the request path (spec 066 AC 5)", func() {
           // The handler must not depend on pkg.GitHubClient on the request
           // path. We assert this two ways:
           //   (a) structural — reflect.TypeOf the handler struct contains
           //       NO field whose type implements pkg.GitHubClient. This
           //       proves the dependency was actually removed (not just
           //       unused in tests).
           //   (b) behavioral — request completes with 202 even when no
           //       GitHubClient is wired anywhere in BeforeEach.
           It("handler struct has no GitHubClient-typed field", func() {
               // Build the handler directly (not via factory) so we can
               // reflect on the concrete struct.
               concrete := handler.NewSinglePRTriggerHandler(sender)
               // concrete is the SinglePRTriggerHandler alias of libhttp.WithError;
               // unwrap to the underlying value via the package's exported test seam.
               // The exported `singlePRTriggerHandler` struct is package-private;
               // we use reflect on the returned interface's dynamic type.
               t := reflect.TypeOf(concrete)
               // The returned value is the interface; get its dynamic type via Elem
               // (it's a pointer to a struct).
               if t.Kind() == reflect.Ptr {
                   t = t.Elem()
               }
               for i := 0; i < t.NumField(); i++ {
                   field := t.Field(i)
                   ghType := reflect.TypeOf((*pkg.GitHubClient)(nil)).Elem()
                   Expect(field.Type.Implements(ghType)).To(BeFalse(),
                       "handler field %q (type %v) must not implement pkg.GitHubClient",
                       field.Name, field.Type)
               }
           })
           It("request completes with 202 (no GitHubClient wired anywhere)", func() {
               req := httptest.NewRequest(
                   "POST",
                   "/trigger?url=https://github.com/bborbe/repo/pull/42",
                   nil,
               )
               resp := httptest.NewRecorder()
               h.ServeHTTP(resp, req)
               Expect(resp.Code).To(Equal(http.StatusAccepted))
           })
       })
   })
   ```

   The reflection-based test is the load-bearing AC 5. It catches a regression where a future refactor re-introduces a `ghClient` field on the handler struct (the test would fail because the new field's type would implement `pkg.GitHubClient`). The "request completes" `It` is a sanity check that the handler still works end-to-end with no GitHub wiring. Add the `reflect` and `pkg` imports to the test file.

   **Test-helper note**: the `mocks.TriggerPRReviewCommandSender` is the prompt 1 mock. Its method is `SendCommand(ctx, command.TriggerPRReviewCommand) error` — the assertion `sender.SendCommandArgsForCall(0)` returns `(_, sentCmd command.TriggerPRReviewCommand)`. Verify the mock's exact `ArgsForCall` signature by reading `/workspace/watcher/github-pr/mocks/trigger_pr_review_command_sender.go` (prompt 1's output).

3. **Update `pkg/factory/single_pr.go` — add the new constructor, KEEP the old one as a legacy adapter so `main.go` stays compilable until prompt 4 swaps it.** The factory exports BOTH after this prompt:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package factory

   import (
       "net/http"

       libhttp "github.com/bborbe/http"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/command"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/handler"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   )

   // NewSinglePRTriggerHandler wires the thin CQRS handler that publishes a
   // TriggerPRReviewCommand to Kafka for each valid /trigger request.
   // All GitHub/filter/trust work lives in the in-pod command consumer
   // (see pkg/command.NewTriggerPRReviewCommandExecutor). This is the
   // signature main.go will use after prompt 4's rewiring.
   func NewSinglePRTriggerHandler(
       sender command.TriggerPRReviewCommandSender,
   ) handler.SinglePRTriggerHandler {
       if sender == nil {
           panic("sender is required")
       }
       return handler.NewSinglePRTriggerHandler(sender)
   }

   // CreateSinglePRTriggerHandler is the LEGACY 9-arg adapter retained for
   // ONE prompt (prompt 3) so main.go stays compilable while the per-prompt
   // commits land sequentially. Prompt 4 swaps the main.go call site to
   // NewSinglePRTriggerHandler and DELETES this adapter. Do NOT add new
   // callers — every parameter except (eventually) sender is now ignored.
   //
   // DEPRECATED: remove in prompt 4. Tracked by spec 066.
   func CreateSinglePRTriggerHandler(
       httpClient *http.Client,
       createSender any, // task.CreateCommandSender — kept loose to avoid the agent/lib import drift in this transition prompt
       taskCreationFilter filter.TaskCreationFilter,
       trustDecision trust.Trust,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       metrics pkg.Metrics,
   ) handler.SinglePRTriggerHandler {
       // All args ignored: the legacy adapter returns a handler that always
       // returns HTTP 503 with "trigger endpoint reconfiguring — see spec 066".
       // This adapter exists only so main.go compiles between prompt 3 and
       // prompt 4 commits; prompt 4 deletes the call site and this function.
       _ = httpClient
       _ = createSender
       _ = taskCreationFilter
       _ = trustDecision
       _ = stage
       _ = maxSlugLen
       _ = maxTitleLen
       _ = taskSuffix
       _ = metrics
       return libhttp.WithErrorFunc(func(_ context.Context, w http.ResponseWriter, _ *http.Request) error {
           return libhttp.WrapWithStatusCode(
               errors.Errorf(context.Background(), "trigger endpoint reconfiguring (spec 066 mid-rollout); retry after prompt 4 lands"),
               http.StatusServiceUnavailable,
           )
       })
   }
   ```

   Why an adapter and not a delete: the spec ships as one PR (all four prompts merged together), but per-prompt commits land sequentially. The adapter keeps every intermediate commit green (the dark-factory contract). The adapter is a 503 stub — it never serves real traffic because the same PR also lands prompt 4 which removes both the adapter and its caller.

4. **Update `pkg/factory/single_pr_test.go`** to cover both shapes during the transition:
   - Keep the existing 4 nil-check entries against `CreateSinglePRTriggerHandler` (the adapter) — they still pass because the adapter ignores those args and never reaches nil-deref logic.
   - Add a new `Describe("NewSinglePRTriggerHandler", ...)` block with: (a) one entry asserting nil-sender panics with `"sender is required"`; (b) one entry asserting a non-nil sender returns a non-nil handler.
   - Add an `It("legacy CreateSinglePRTriggerHandler returns a 503 stub during the transition", ...)` test that invokes the adapter's returned handler with `httptest.NewRecorder` and asserts the body says "reconfiguring" and status is `503`. Prompt 4 deletes both the adapter test and this entry.

5. **DO NOT edit `main.go` in this prompt.** All `main.go` rewiring (new sender variable, third `run.Func`, removal of the legacy `CreateSinglePRTriggerHandler` call site) lands in prompt 4. `main.go` compiles unchanged because the legacy `CreateSinglePRTriggerHandler` signature is retained as an adapter (req #3).

   Verify by grep: `git diff --stat /workspace/watcher/github-pr/main.go` must show **zero changes** after this prompt.

6. **Run `make test` in the changed module — the FULL suite, not a subset.** Because `main.go` is untouched and the legacy `CreateSinglePRTriggerHandler` adapter keeps the call site compiling, the full module test passes after this prompt:

   ```
   cd /workspace/watcher/github-pr && make test
   ```

   Expected: exit code 0; the rewritten handler tests pass; the factory tests cover both the new `NewSinglePRTriggerHandler` and the legacy `CreateSinglePRTriggerHandler` adapter; the prompt 1 + 2 command tests are unaffected; the `main.go` binary still compiles.

7. **Do NOT add a CHANGELOG entry in this prompt.** The CHANGELOG entry for the full spec is owned by prompt 4 (it is the user-visible behavior change: `feat:` for the new CQRS endpoint, plus the test/CHANGELOG/HTTP wire-shape changes ripple there). This prompt is mid-pipeline and prompt 4 is the integration point.

8. **YAGNI guard.** Do NOT add a new field to the request struct (e.g. `actor`, `requested_by`, `priority`) — the spec freezes the wire shape. Do NOT add a `force=true` query param handling branch — the spec is explicit: "Force bool is plumbed but unused". Do NOT add a per-URL dedup check on the HTTP path — Kafka's at-least-once + downstream task_id idempotency is the dedup mechanism. Do NOT add a "trigger accepted at" timestamp field to the response — `status` + `url` is the frozen shape.
</requirements>

<constraints>
- The new HTTP handler depends ONLY on `command.TriggerPRReviewCommandSender`. No `pkg.GitHubClient`, no `filter.TaskCreationFilter`, no `trust.Trust`, no `pkg.Metrics` on the request path. This is the spec § AC 5 load-bearing invariant.
- Wire shape: `POST /trigger?url=<valid>` returns HTTP 202 with body `{"status":"accepted","url":"<raw_url>"}`. JSON field names are `status` and `url` (lowercase, matching the rest of the codebase). The Content-Type header is `application/json`.
- 400 path: same as today. Empty/missing URL, unparseable URL, non-GitHub platform → HTTP 400 with the error wrapped via `libhttp.WrapWithStatusCode`. No Kafka message published.
- 502 path: `sender.SendCommand` returns an error → HTTP 502 with `libhttp.WrapWithStatusCode`. Single message in the log.
- The handler's `SinglePRTriggerHandler = libhttp.WithError` alias is UNCHANGED. The wiring in `main.go` (line 300) keeps its `libhttp.NewJSONErrorHandler(triggerHandler)` shape — the alias is the contract.
- The factory gains a new `NewSinglePRTriggerHandler(sender)` constructor as the canonical post-CQRS form. The legacy 9-arg `CreateSinglePRTriggerHandler` is RETAINED as a deprecated 503-stub adapter so `main.go` stays compilable mid-rollout. Prompt 4 swaps the call site to `NewSinglePRTriggerHandler` and deletes the adapter in the same commit.
- Error wrapping: `github.com/bborbe/errors` only. Never `fmt.Errorf`. Always pass `ctx` to error constructors. Never `context.Background()` in `pkg/`.
- Ginkgo v2 + Gomega + counterfeiter. External test package (`handler_test`). Coverage on the rewritten handler ≥ 80% per `docs/definition-of-done.md`.
- The existing `mocks.SinglePRTriggerHandler` mock is preserved. If the mock has no callers after the rewrite, it stays as dead code (counterfeiter will keep regenerating it on the next `make generate`); the next prompt's `make generate` removes the annotation if you want to clean it up (out of scope for this prompt).
- `main.go` is NOT edited in this prompt — the legacy adapter keeps the existing call site compiling. Prompt 4 swaps the call site and deletes the adapter.
- Do NOT commit — dark-factory handles git. Branch: `dark-factory/cqrs-trigger-github-pr`.
- Build verification: `cd /workspace/watcher/github-pr && make test` must exit 0 (full suite, not subset).
</constraints>

<verification>

Verify the handler file's import block is minimal (no GitHub/filter/trust/metrics):
```
grep -E '^\t"github\.com/bborbe/maintainer/watcher/github-pr/pkg(/|/(filter|trust))"' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
```
Expected: empty output. Only the `pkg/command` import is allowed (for `TriggerPRReviewCommandSender`).

Verify the handler struct has only one field:
```
grep -A 5 'type singlePRTriggerHandler struct' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
```
Expected: exactly one field: `sender command.TriggerPRReviewCommandSender`.

Verify the handler's `ServeHTTP` does not call `ghClient.GetPRDetails`:
```
grep -n 'GetPRDetails\|taskCreationFilter\|trustDecision' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
```
Expected: empty output. None of these symbols appear in the handler.

Verify the 202 response shape:
```
grep -A 4 'writeAccepted' /workspace/watcher/github-pr/pkg/handler/trigger_handler.go
```
Expected: `WriteHeader(http.StatusAccepted)` and a body map with `status: "accepted"` and `url: rawURL`.

Verify BOTH the new constructor AND the legacy adapter exist:
```
grep -n 'func NewSinglePRTriggerHandler\|func CreateSinglePRTriggerHandler' /workspace/watcher/github-pr/pkg/factory/single_pr.go
```
Expected: two matches. `NewSinglePRTriggerHandler(sender command.TriggerPRReviewCommandSender)` AND `CreateSinglePRTriggerHandler(...)` (legacy 9-arg adapter, deprecated, deleted in prompt 4).

Verify main.go is untouched:
```
git diff --stat /workspace/watcher/github-pr/main.go
```
Expected: empty (no changes).

Verify the panicking-GitHub-client test exists:
```
grep -n 'panicking GitHub client' /workspace/watcher/github-pr/pkg/handler/trigger_handler_test.go
```
Expected: one match (the `Context` heading).

Run the full module test suite (main.go compiles thanks to the legacy adapter):
```
cd /workspace/watcher/github-pr && make test
```
Expected: exit code 0; the table-driven 400 cases pass; the happy-path 202 case passes; the Kafka-send-failure 502 case passes; the panicking-GitHub-client case passes; the new `NewSinglePRTriggerHandler` nil-sender panic case passes; the legacy `CreateSinglePRTriggerHandler` 503-stub case passes; the command tests from prompts 1 and 2 are unaffected.
</verification>
