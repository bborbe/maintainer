---
status: completed
spec: [066-cqrs-trigger-github-pr]
summary: Added TriggerPRReviewCommandExecutor (pkg/command) with full exit-path table tests, handler/executor byte-identical parity test, crash-recovery retry test, and Prometheus metric ownership tests; refactored to satisfy funlen ≤ 80; all 24 pkg/command + 107 pkg tests pass; coverage 91.5% (pkg/command) and 96.5% (pkg) — prompt 4 owns make precommit per spec.
container: maintainer-cqrs-trigger-exec-249-spec-066-trigger-pr-review-executor
dark-factory-version: v0.175.0
created: "2026-06-08T21:11:59Z"
queued: "2026-06-08T21:49:51Z"
started: "2026-06-08T21:56:31Z"
completed: "2026-06-08T22:25:55Z"
branch: dark-factory/cqrs-trigger-github-pr
---

<summary>
- Adds a new `pkg/command/trigger_pr_review_executor.go` that consumes `TriggerPRReviewCommand` messages from the in-pod Kafka topic and runs the work the old HTTP handler did: GitHub fetch → filter → trust → downstream `CreateTaskCommand` publish.
- The executor uses the project's standard `cdb.CommandObjectExecutorTxFunc(...)` shape; `SendResultEnabled` is `false` (spec Non-goal: fire-and-forget, no result topic).
- Exit paths follow `go-cqrs.md` rules strictly: invalid URL / filter-rejected / untrusted-author → `cdb.ErrCommandObjectSkipped` (non-retryable, deliberate). GitHub 5xx / Kafka send error / trust infrastructure error → wrapped error (transient, framework emits Failure on the result topic, Kafka redelivers). Success → `nil, nil, nil`.
- The `github_pr_published` Prometheus counter is now incremented from the executor (not the HTTP handler) with the same label set as today: `create`, `skipped`, `kafka_error`, `trust_error`. The executor is the sole owner.
- Table-driven tests cover all 7 exit-path branches. A dedicated crash-recovery test simulates a pod kill mid-execution: publish one `TriggerPRReviewCommand`, cancel the consumer goroutine before the offset is committed, restart the consumer, observe exactly one downstream `CreateTaskCommand` (proving at-least-once-via-idempotent-downstream).

This is prompt 2 of 4 for spec 066. It depends on prompt 1 (the command + sender types). Prompt 3 (HTTP handler shrink) and prompt 4 (consumer wiring) depend on this prompt.
</summary>

<objective>
Build the executor that performs the actual single-PR trigger work on the consumer side. After this prompt, a `TriggerPRReviewCommand` message on the request topic is processed end-to-end: GitHub is queried, filters + trust are evaluated, and a `CreateTaskCommand` is published downstream if the PR passes. The HTTP handler (prompt 3) is reduced to publish+202, and the consumer (prompt 4) is wired as the third `run.Func`.

The critical correctness invariants:
1. The downstream `CreateTaskCommand` payload MUST be byte-identical to today's `singlePRTriggerHandler.ServeHTTP` output (spec § Constraints). This is verified by an explicit "executor vs handler" fixture parity test.
2. The executor must NOT use the `return nil, nil, nil` "idempotent skip" anti-pattern from `agent/task/controller/pkg/command/task_create_task_executor.go:61` (spec § Desired Behavior 6). Filter-skip and trust-reject MUST be `cdb.ErrCommandObjectSkipped`.
3. The metric ownership flips: `github_pr_published` increments live in the executor, not the HTTP handler (spec § Desired Behavior 7).
</objective>

<context>
Read `/workspace/CLAUDE.md` and `/workspace/watcher/github-pr/CLAUDE.md` (if present) for project conventions.

Read these source files in full BEFORE editing:

- `/workspace/watcher/github-pr/pkg/handler/trigger_handler.go` — the file being effectively "forked". The body of `singlePRTriggerHandler.ServeHTTP` (lines 66-140) is the source of truth for the executor's downstream publish. The `parseAndValidateURL` (lines 142-167), `buildFilterPR` (lines 186-197), `buildPullRequest` (lines 199-213), and `writeSuccess` (lines 169-184) helpers are NOT all used — the executor does not write HTTP responses, and validation is now in `TriggerPRReviewCommand.Validate` (prompt 1). The only helpers the executor needs are `buildFilterPR` and `buildPullRequest` (private to `handler` package — the executor must redefine them locally; do NOT export from `handler`).
- `/workspace/watcher/github-pr/pkg/watcher.go` — `BuildCreateCommand` and its private helpers (`buildFrontmatter`, `buildHumanReviewFrontmatter`, `buildTaskBody`, `buildUntrustedBody`). The executor calls **only** `pkg.BuildCreateCommand(pr, details, taskIDStr, stage, maxSlugLen, maxTitleLen, taskSuffix, trustResult)` — that single entry point encapsulates all title/frontmatter/body construction. Do not call the private helpers directly; do not duplicate their logic. Anchor by symbol name (`grep -n 'func BuildCreateCommand' /workspace/watcher/github-pr/pkg/watcher.go`).
- `/workspace/watcher/github-pr/pkg/metrics.go` — the `Metrics` interface and the `github_pr_published` counter definition. Note: the metric is named `github_pr_watcher_prs_total` internally but exposed via `Metrics.IncPRPublished(command string)`. The label set today is `create`, `update_frontmatter`, `skipped`, `error`, `trust_error`, `kafka_error` (initialized to 0 in `init()` for `prometheus.MustRegister`). The executor uses only the four labels listed in the spec § AC 12: `create`, `skipped`, `kafka_error`, `trust_error`. The `update_frontmatter` and `error` labels stay registered (zero-init) for the poll-loop compatibility — do NOT remove them.
- `/workspace/watcher/github-pr/pkg/githubclient.go` — the `GitHubClient` interface the executor depends on. The executor uses only `GetPRDetails` (line 89). `SearchPRs` is poll-loop only.
- `/workspace/watcher/github-pr/pkg/filter/filter.go` — the `TaskCreationFilter` interface + `TaskCreationFilters` composite. Same as the handler.
- `/workspace/watcher/github-pr/pkg/trust/trust.go` — the `Trust` interface + `PR{AuthorLogin}` input struct. Same as the handler.
- `/workspace/watcher/github-pr/pkg/taskid.go` — `pkg.DeriveTaskID(owner, repo, number, sha)` (line 20). The executor uses this exactly as the handler does.
- `/workspace/prompts/1-spec-066-trigger-pr-review-command.md` (the prompt 1 output) — the `pkg/command` package with `TriggerPRReviewCommand`, `TriggerPRReviewCommandSender`, `TriggerPRReviewCommandOperation`, and the counterfeiter mock. Verify the package exists before editing: `ls /workspace/watcher/github-pr/pkg/command/`. If empty, STOP and report `status: failed` with message "prompt 1 of spec 066 has not shipped".

Reference the canonical executor pattern in the agent project:

- `/home/node/go/pkg/mod/github.com/bborbe/agent@v0.65.0/prompts/completed/083-spec-017-controller-create-task-executor.md` — load-bearing reference for the executor shape, the `cdb.CommandObjectExecutorTxFunc` constructor call, the `commandObject.Command.Data.MarshalInto(ctx, &cmd)` unmarshal pattern, the `cdb.ErrCommandObjectSkipped` return path, the `errors.Wrapf` wrapping convention, and the table-driven test layout.
- `/home/node/go/pkg/mod/github.com/bborbe/cqrs@v0.5.3/cdb/cdb_command-object-executor-tx-func.go` — the `cdb.CommandObjectExecutorTxFunc` factory and the `HandleCommandFunc` signature: `func(ctx, tx, commandObject) (*base.EventID, base.Event, error)`. The executor's closure MUST match this signature exactly. `tx` is unused for the trigger executor (no kv writes) but the parameter is mandatory.
- `/home/node/go/pkg/mod/github.com/bborbe/cqrs@v0.5.3/cdb/cdb_command-object-executor-result-sender.go` — the `cdb.ErrCommandObjectSkipped` sentinel. The spec says: "wrap with `ErrCommandObjectSkipped`, not the bare sentinel" — use `errors.Wrapf(ctx, cdb.ErrCommandObjectSkipped, "...")` to attach context. The framework's `NewCommandObjectExecutorTxResultSender` (in the Tx variant) recognizes the wrapped error via `errors.Is` (see `cdb_command-object-executor-tx-result-sender.go` line 37: `if resultErr != nil && errors.Is(resultErr, CommandObjectSkippedError) {`).

Coding plugin docs (in-container paths):

- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md` — load-bearing. Read the entire file. The "Skipping Invalid Commands" section and the two RULE blocks (`go-cqrs/auto-tx-wrapper-no-manual-wrap`, `go-cqrs/skipped-not-nil-for-non-retryable`) are the rules the executor's error mapping MUST obey.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `errors.Wrapf` over `fmt.Errorf`; pass `ctx` to every constructor.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter; external test package.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-prometheus-metrics-guide.md` — the `Metrics` interface is already in place; the executor just calls `IncPRPublished(label)`. No new metric registration is needed (the labels are pre-registered in `init()`).
</context>

<requirements>

1. **Create `pkg/command/trigger_pr_review_executor.go`** in the same `pkg/command` package as prompt 1 (do NOT create a new package — the executor is a sibling of the sender, both logically part of "the trigger command's CQRS plumbing"). Add the executor to the same external-test-package Ginkgo suite that prompt 1 created.

   The executor file shape (anchored by symbol name, not line number):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package command

   import (
       "context"

       task "github.com/bborbe/agent/lib/command/task"
       "github.com/bborbe/cqrs/base"
       cdb "github.com/bborbe/cqrs/cdb"
       "github.com/bborbe/errors"
       libkv "github.com/bborbe/kv"
       "github.com/golang/glog"

       "github.com/bborbe/maintainer/lib/prurl"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   )

   // NewTriggerPRReviewCommandExecutor creates a cdb.CommandObjectExecutorTx
   // that consumes TriggerPRReviewCommand messages and drives the single-PR
   // review pipeline: GitHub fetch → filter → trust → downstream publish.
   //
   // Exit-path mapping (per spec 066 § Desired Behavior 5):
   //   - invalid URL (Validate fails)            → cdb.ErrCommandObjectSkipped
   //   - filter-rejected PR (filter.Skip==true)  → cdb.ErrCommandObjectSkipped
   //   - untrusted author (trust.IsTrusted==false) → cdb.ErrCommandObjectSkipped
   //   - GitHub 5xx / network error              → wrapped error (transient, retried)
   //   - trust infrastructure error              → wrapped error (transient, retried)
   //   - downstream CreateTaskCommand send error → wrapped error (transient, retried)
   //   - success                                  → nil, nil, nil
   //
   // SendResultEnabled is false (spec Non-goal: fire-and-forget).
   // The github_pr_published metric is incremented from this executor
   // (not the HTTP handler) with labels: create, skipped, kafka_error, trust_error.
   func NewTriggerPRReviewCommandExecutor(
       ghClient pkg.GitHubClient,
       createSender task.CreateCommandSender,
       taskCreationFilter filter.TaskCreationFilter,
       trustDecision trust.Trust,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       metrics pkg.Metrics,
   ) cdb.CommandObjectExecutorTx {
       return cdb.CommandObjectExecutorTxFunc(
           TriggerPRReviewCommandOperation,
           false, // SendResultEnabled = false
           func(ctx context.Context, tx libkv.Tx, commandObject cdb.CommandObject) (*base.EventID, base.Event, error) {
               return runTriggerPRReview(
                   ctx, tx, commandObject,
                   ghClient, createSender, taskCreationFilter, trustDecision,
                   stage, maxSlugLen, maxTitleLen, taskSuffix, metrics,
               )
           },
       )
   }

   // runTriggerPRReview is the work-loop for a single TriggerPRReviewCommand.
   // Splitting it out from the constructor (a) keeps the constructor's
   // closure short and (b) makes the function directly testable from
   // the package's external _test.go (the constructor returns an interface,
   // not a closure).
   func runTriggerPRReview(
       ctx context.Context,
       _ libkv.Tx,
       commandObject cdb.CommandObject,
       ghClient pkg.GitHubClient,
       createSender task.CreateCommandSender,
       taskCreationFilter filter.TaskCreationFilter,
       trustDecision trust.Trust,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       metrics pkg.Metrics,
   ) (*base.EventID, base.Event, error) {
       var cmd TriggerPRReviewCommand
       if err := commandObject.Command.Data.MarshalInto(ctx, &cmd); err != nil {
           return nil, nil, errors.Wrapf(
               ctx,
               cdb.ErrCommandObjectSkipped,
               "malformed TriggerPRReviewCommand: %v",
               err,
           )
       }
       if err := cmd.Validate(ctx); err != nil {
           return nil, nil, errors.Wrapf(
               ctx,
               cdb.ErrCommandObjectSkipped,
               "validate TriggerPRReviewCommand: %v",
               err,
           )
       }

       prInfo, err := prurl.ParsePRURL(ctx, cmd.URL)
       if err != nil {
           return nil, nil, errors.Wrapf(
               ctx,
               cdb.ErrCommandObjectSkipped,
               "parse url %q: %v",
               cmd.URL,
               err,
           )
       }

       details, err := ghClient.GetPRDetails(ctx, prInfo.Owner, prInfo.Repo, prInfo.Number)
       if err != nil {
           // Transient: GitHub 5xx / network error. Framework emits Failure
           // on the result topic, Kafka redelivers.
           return nil, nil, errors.Wrapf(ctx, err, "get PR details for %s", cmd.URL)
       }

       filterPR := filter.PR{
           AuthorLogin: details.AuthorLogin,
           IsDraft:     details.IsDraft,
           Title:       details.Title,
           UpdatedAt:   details.UpdatedAt,
           RepoKey:     "github.com/" + prInfo.Owner + "/" + prInfo.Repo,
       }
       if taskCreationFilter.Skip(filterPR) {
           metrics.IncPRPublished("skipped")
           glog.V(2).Infof(
               "trigger executor: filtered pr=%s/%s#%d",
               prInfo.Owner, prInfo.Repo, prInfo.Number,
           )
           return nil, nil, errors.Wrapf(
               ctx,
               cdb.ErrCommandObjectSkipped,
               "filter rejected pr=%s/%s#%d",
               prInfo.Owner, prInfo.Repo, prInfo.Number,
           )
       }

       trustResult, err := trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: details.AuthorLogin})
       if err != nil {
           // Transient: trust infrastructure error (e.g. allowlist lookup).
           // Framework emits Failure, Kafka redelivers.
           metrics.IncPRPublished("trust_error")
           glog.Errorf("trigger executor: trust check failed pr=%s err=%v", cmd.URL, err)
           return nil, nil, errors.Wrapf(ctx, err, "check trust for %s", cmd.URL)
       }
       if !trustResult.Success() {
           // Deliberate: author not on allowlist. Non-retryable.
           metrics.IncPRPublished("skipped")
           glog.V(2).Infof(
               "trigger executor: untrusted author pr=%s author=%s reason=%s",
               cmd.URL, details.AuthorLogin, trustResult.Description(),
           )
           return nil, nil, errors.Wrapf(
               ctx,
               cdb.ErrCommandObjectSkipped,
               "untrusted author %s for pr %s",
               details.AuthorLogin, cmd.URL,
           )
       }

       pr := pkg.PullRequest{
           Number:      prInfo.Number,
           Owner:       prInfo.Owner,
           Repo:        prInfo.Repo,
           Title:       details.Title,
           AuthorLogin: details.AuthorLogin,
           HTMLURL:     cmd.URL,
           IsDraft:     details.IsDraft,
       }
       taskIDStr := pkg.DeriveTaskID(prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA).String()

       createCmd := pkg.BuildCreateCommand(
           pr,
           details,
           taskIDStr,
           stage,
           maxSlugLen,
           maxTitleLen,
           taskSuffix,
           trustResult,
       )

       if err := createSender.SendCommand(ctx, createCmd); err != nil {
           // Transient: downstream Kafka send error. Framework emits Failure,
           // Kafka redelivers. Downstream is idempotent via derived task_id.
           metrics.IncPRPublished("kafka_error")
           glog.Errorf("trigger executor: send create-task failed pr=%s err=%v", cmd.URL, err)
           return nil, nil, errors.Wrapf(ctx, err, "send create task command for %s", cmd.URL)
       }

       metrics.IncPRPublished("create")
       glog.V(2).Infof(
           "trigger executor: published task_id=%s pr=%s/%s#%d sha=%s",
           taskIDStr, prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA,
       )
       return nil, nil, nil
   }
   ```

   The `task "github.com/bborbe/agent/lib/command/task"` import is already in the example block above. The `task.CreateCommandSender` parameter type lives in agent/lib. The import alias is `task` (matching the existing usage in `pkg/factory/factory.go` line 12 and the handler tests).

2. **Validate-import audit.** The `runTriggerPRReview` function does NOT call `cmd.Validate` after `MarshalInto` succeeds — `MarshalInto` decodes the wire bytes back into the struct but does not run the `Validate` method. The two-step is mandatory because the framework's auto-tx-wrapper does NOT call `Validate` on the command (it only validates the `CommandObject` itself, not the typed payload). Re-read `cdb_command-object.go` lines 22-40 to confirm: `CommandObject.Validate` only validates `SchemaID` and `Command` (the wrapper), not the `Command.Data` payload. Calling `cmd.Validate(ctx)` after the unmarshal is the executor's responsibility.

   However, given the `Validate` call was already made on the publisher side (in `NewTriggerPRReviewCommandSender.SendCommand`), a duplicate call here is wasteful but **not incorrect** — it is the safety net for a buggy client that bypasses the HTTP handler and publishes a `TriggerPRReviewCommand` directly. The double-check is intentional. Document the rationale with a single comment in the code (do NOT inline multiple deliberation comments — one comment is enough).

3. **Create `pkg/command/trigger_pr_review_executor_test.go`** (external test package `command_test`). Required test cases — mirror the layout of the agent's spec-017 test file at `/home/node/go/pkg/mod/github.com/bborbe/agent@v0.65.0/prompts/completed/083-spec-017-controller-create-task-executor.md` test (b) through (g):

   a. **Table-driven exit-path test.** Build a `DescribeTable("runTriggerPRReview", ...)` that asserts the executor's three return shapes. Use a hand-rolled fake `cdb.CommandObjectSender` (or the cdb counterfeiter mock at `github.com/bborbe/cqrs@v0.5.3/mocks/cdb-command-sender.go` if available) to observe the downstream publish count. The table entries:

   ```go
   DescribeTable("exit-path mapping",
       func(
           name string,
           setup func(
               ghClient *mocks.GitHubClient,
               createSender *taskmocks.TaskCreateCommandSender,
               taskCreationFilter *mocks.TaskCreationFilter,
               trustDecision *mocks.Trust,
           ),
           cmd TriggerPRReviewCommand,
           expectSkipped bool,         // errors.Is(err, cdb.ErrCommandObjectSkipped)
           expectWrappedErr bool,      // err != nil && !expectSkipped
           expectDownstreamSent int,   // createSender.SendCommandCallCount()
           expectMetricLabel string,   // "" means no metric increment
       ) {
           ghClient := new(mocks.GitHubClient)
           createSender := new(taskmocks.TaskCreateCommandSender)
           taskCreationFilter := new(mocks.TaskCreationFilter)
           trustDecision := new(mocks.Trust)

           taskCreationFilter.SkipReturns(false)
           trustDecision.IsTrustedReturns(trust.NewResult(true, "trusted"), nil)
           ghClient.GetPRDetailsReturns(pkg.PRDetails{
               HeadSHA:     "abc123",
               CloneURL:    "https://github.com/bborbe/repo.git",
               BaseRef:     "main",
               AuthorLogin: "bborbe",
               Title:       "Feature: add support",
               IsDraft:     false,
           }, nil)

           setup(ghClient, createSender, taskCreationFilter, trustDecision)

           // mustParseEvent is a test-only helper defined once in the suite_test.go file:
           //   func mustParseEvent(cmd command.TriggerPRReviewCommand) base.Event {
           //       evt, err := base.ParseEvent(context.Background(), cmd)
           //       Expect(err).NotTo(HaveOccurred())
           //       return evt
           //   }
           commandObject := cdb.CommandObject{
               Command: base.Command{
                   Operation: TriggerPRReviewCommandOperation,
                   Data:      mustParseEvent(cmd),
               },
               SchemaID: lib.GithubPRReviewV1SchemaID,
           }

           _, _, err := command.RunTriggerPRReview(  // see note below
               context.Background(),
               nil,
               commandObject,
               ghClient, createSender, taskCreationFilter, trustDecision,
               "dev", 80, 200, "",
               pkg.NewMetrics(),
           )

           if expectSkipped {
               Expect(err).To(HaveOccurred())
               Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeTrue(),
                   "%s: expected ErrCommandObjectSkipped, got %v", name, err)
           } else if expectWrappedErr {
               Expect(err).To(HaveOccurred())
               Expect(errors.Is(err, cdb.ErrCommandObjectSkipped)).To(BeFalse(),
                   "%s: expected wrapped error, got ErrCommandObjectSkipped", name)
           } else {
               Expect(err).NotTo(HaveOccurred(), "%s: unexpected error: %v", name, err)
           }
           Expect(createSender.SendCommandCallCount()).To(Equal(expectDownstreamSent),
               "%s: downstream send count mismatch", name)
       },
       Entry("valid pr → create + downstream sent", validSetup, validCmd, false, false, 1, "create"),
       Entry("invalid url (non-github) → skipped", validSetup, nonGithubCmd, true, false, 0, ""),
       Entry("malformed payload → skipped", malformedSetup, validCmd, true, false, 0, ""),
       Entry("filter rejects → skipped", filterRejectSetup, validCmd, true, false, 0, "skipped"),
       Entry("untrusted author → skipped", untrustedSetup, validCmd, true, false, 0, "skipped"),
       Entry("github 5xx → wrapped err", githubFailSetup, validCmd, false, true, 0, ""),
       Entry("trust infra err → wrapped err + trust_error metric", trustErrSetup, validCmd, false, true, 0, "trust_error"),
       Entry("kafka send err → wrapped err + kafka_error metric", kafkaErrSetup, validCmd, false, true, 0, "kafka_error"),
   )
   ```

   **Critical detail for testability**: `runTriggerPRReview` is package-private (lowercase `r`). The test file is in `package command_test` (external). To call a private function from an external test package, the executor file MUST add an export_test.go file with a re-export:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package command

   import (
       "context"

       cdb "github.com/bborbe/cqrs/cdb"
       libkv "github.com/bborbe/kv"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   )

   // RunTriggerPRReview re-exports the private runTriggerPRReview for
   // the external test package. The _test.go suffix keeps this file
   // out of production builds.
   var RunTriggerPRReview = runTriggerPRReview
   ```

   The signature of `RunTriggerPRReview` must match `runTriggerPRReview` exactly. The file is named `trigger_pr_review_executor_export_test.go` to follow Go convention (`*_test.go` files are excluded from the production build).

   b. **Byte-identical payload parity test.** Build a `Describe("executor vs handler payload parity (spec 066 AC: byte-identical downstream)")` block. Wire the SAME dependencies (same `ghClient.GetPRDetailsReturns`, same `trustDecision.IsTrustedReturns`, same `createSender` capture) into BOTH the OLD `handler.NewSinglePRTriggerHandler` (via the existing handler's `ServeHTTP` with a `httptest.NewRecorder`) and the NEW `command.NewTriggerPRReviewCommandExecutor` (via the inner closure). Assert that the `task.CreateCommand` captured by `createSender.SendCommandStub` is **deep-equal** in both invocations. This is the load-bearing AC for spec § Constraints ("downstream `CreateTaskCommand` payload MUST be byte-identical").

   c. **Crash-recovery test (spec § AC 16).** Build a `Describe("executor crash recovery (spec 066 AC 16)")` block. The test:
   - Publishes a single `TriggerPRReviewCommand` to a fake `cdb.CommandObjectSender` whose `SendCommand` returns nil (success on first attempt, but the executor is killed before any side effect).
   - Runs the executor's closure once with a context that is cancelled **immediately after** the executor calls `ghClient.GetPRDetails` (use a `GetPRDetailsStub` that calls `cancel()` before returning).
   - Asserts the closure returns `ctx.Err()` (a wrapped context error, transient — Kafka would redeliver).
   - Runs the closure a SECOND time on a fresh context with the same `commandObject`.
   - Asserts the second invocation completes successfully: `createSender.SendCommandCallCount() == 1` and the metric `IncPRPublished("create")` was called.

   The point of the test: prove that on retry (which Kafka would do via redelivery), the same downstream `CreateTaskCommand` is published exactly once. Use a fresh `createSender` fake for each invocation so the call count is unambiguous. The fake `GitHubClient` `GetPRDetailsStub` returns the same `PRDetails` both times (deterministic).

4. **Update `pkg/metrics.go` ONLY if needed.** The label set is already pre-registered in `init()` at lines 35-40. The executor's four labels (`create`, `skipped`, `kafka_error`, `trust_error`) are already in the pre-registration list. **Do not modify the file.** If the pre-registration list were missing a label, add it — but verify the current state first: `grep -A 6 'prPublishedTotal.WithLabelValues' /workspace/watcher/github-pr/pkg/metrics.go`.

5. **Add a Prometheus counter unit test for the executor's label ownership.** Build a `Describe("github_pr_published metric (spec 066 AC 12)")` block. Use `testutil.ToFloat64` from `github.com/prometheus/client_golang/prometheus/testutil` (verify the import path with `go doc github.com/prometheus/client_golang/prometheus/testutil ToFloat64` or grep `$(go env GOPATH)/pkg/mod/github.com/prometheus/client_golang@*/prometheus/testutil/testutil.go`). Capture the metric delta across an executor invocation, assert the expected label increments. The test must assert:

   - After a valid invocation: `create` counter incremented by 1.
   - After a filter-reject invocation: `skipped` counter incremented by 1, `create` unchanged.
   - After a trust-reject invocation: `skipped` counter incremented by 1, `create` unchanged.
   - After a trust-infra-error invocation: `trust_error` counter incremented by 1.
   - After a downstream-Kafka-send-error invocation: `kafka_error` counter incremented by 1.

   **Important**: the `Metrics` interface is a counterfeiter-friendly seam, but the production `prometheusMetrics` writes to the global `prPublishedTotal` counter. To assert the metric value in tests, use the global registry's `testutil.ToFloat64`:

   ```go
   import "github.com/prometheus/client_golang/prometheus/testutil"
   // ...
   Expect(testutil.ToFloat64(prometheusMetricsForLabel("create"))).To(BeNumerically(">", 0))
   ```

   The exact accessor for the named counter is `testutil.ToFloat64(prPublishedTotal.WithLabelValues("create"))` — but `prPublishedTotal` is package-private. The test must be in the same package as `metrics.go` (i.e., a new test file under `pkg/`, NOT `pkg/command/`). Add this test to a new file: `pkg/metrics_executor_test.go` (external test package `pkg_test`). This is the only test in this prompt that lives outside `pkg/command/`.

6. **Run `make test` in the changed module.** From the github-pr watcher dir:

   ```
   cd /workspace/watcher/github-pr && make test
   ```

   Expected: exit code 0; the new executor tests pass; all pre-existing tests pass unchanged (the existing handler tests still use the synchronous handler — that is fine, prompt 3 shrinks the handler and updates those tests in the same prompt).

7. **Do NOT run `make precommit` in this prompt.** Prompt 4 (wiring the third `run.Func`) owns the final precommit gate. This prompt only needs `make test`.

8. **YAGNI guard.** Do NOT add a `Force` handling branch — the executor reads `cmd.URL` and ignores `cmd.Force`. Do NOT add a new Prometheus metric — the four labels are pre-registered. Do NOT add a "trigger-executor-publish" metric — the `github_pr_published` counter is the only observability seam (spec § Non-goal: "Do NOT add per-request opt-out flags"). Do NOT add a `Validate` call inside `NewTriggerPRReviewCommandExecutor`'s closure wrapper — `runTriggerPRReview` calls it itself. Do NOT add a `WaitGroup` or `sync.Mutex` — the executor is single-message-at-a-time (the framework's `MessageHandlerTx` calls it serially per command).
</requirements>

<constraints>
- The executor uses ONLY `cdb.ErrCommandObjectSkipped` for non-retryable paths and ONLY wrapped `errors.Wrapf(ctx, err, "...")` for transient paths. NEVER `return nil, nil, nil` as a skip signal — the spec explicitly forbids this anti-pattern (spec § Desired Behavior 6).
- The downstream `CreateTaskCommand` payload MUST be byte-identical to today's handler output. Use the EXACT same `pkg.BuildCreateCommand(pr, details, taskIDStr, stage, maxSlugLen, maxTitleLen, taskSuffix, trustResult)` call with the EXACT same arguments. No field reordering, no helper substitution.
- `SendResultEnabled` is `false` — hard-coded in the constructor's second argument to `cdb.CommandObjectExecutorTxFunc`. Do NOT add a config field.
- The `tx libkv.Tx` parameter is unused (`_ libkv.Tx`); the executor does not write to kv. Do not call any tx methods. The parameter is required by the `HandleCommandFunc` signature.
- Error wrapping: `github.com/bborbe/errors` only. Never `fmt.Errorf`. Always pass `ctx` to error constructors. Never `context.Background()` in `pkg/`.
- The `runTriggerPRReview` function is package-private. Test access via the `*_export_test.go` re-export pattern (Go convention). Do NOT make it public.
- The `pkg/metrics_executor_test.go` test is in `package pkg_test` (external to `pkg`) and uses the global Prometheus registry via `testutil.ToFloat64`. Do NOT register a new metric — use the existing `prPublishedTotal` counter.
- The `pkg.DeriveTaskID` and `pkg.BuildCreateCommand` are the canonical helpers — do not duplicate or rename them in the executor file. The executor calls them by their existing package-qualified name.
- Ginkgo v2 + Gomega + counterfeiter. External test packages (`command_test`, `pkg_test`). Coverage on the new code ≥ 80% per `docs/definition-of-done.md`.
- Do NOT modify the existing `pkg/handler/trigger_handler.go` in this prompt — prompt 3 shrinks it. The existing handler stays functional (and the byte-identical payload test depends on this).
- Do NOT modify `main.go` or the factory in this prompt — prompt 4 wires the third `run.Func`.
- **Transient double-metric-counting window (deliberate, expected).** After this prompt lands, both the existing HTTP handler (legacy path, still running) and the new executor (no incoming Kafka traffic yet) own the `github_pr_published` counter. The handler increments on every `/trigger` HTTP call as today; the executor increments zero times because no Kafka publisher exists until prompts 3 and 4 ship. The metric is single-counted, not doubled, until prompt 3 strips the handler's increments. Operators may see no metric change at all between prompt-2-land and prompt-3-land. Once all four prompts land in the same PR/merge unit, the final state is exactly-once counting from the executor.
- Do NOT commit — dark-factory handles git. Branch: `dark-factory/cqrs-trigger-github-pr`.
- Build verification: `cd /workspace/watcher/github-pr && make test` must exit 0.
</constraints>

<verification>

Verify the executor file was created and exports the expected public constructor:
```
grep -n 'func NewTriggerPRReviewCommandExecutor' /workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor.go
```
Must show the constructor signature: `(ghClient pkg.GitHubClient, createSender task.CreateCommandSender, ..., metrics pkg.Metrics) cdb.CommandObjectExecutorTx`.

Verify the private helper is re-exported for the external test package:
```
grep -n 'RunTriggerPRReview' /workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor_export_test.go
```
Must show `var RunTriggerPRReview = runTriggerPRReview`.

Verify the exit-path mapping uses the right sentinels (spec § AC 6, 7, 8):
```
grep -n 'cdb.ErrCommandObjectSkipped' /workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor.go
```
Must show at least four occurrences (one per non-retryable branch: marshal fail, Validate fail, filter reject, untrusted author).

```
grep -n 'errors.Wrapf' /workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor.go
```
Must show at least three wrapped-error returns (GitHub fetch, trust infra err, downstream Kafka send).

Verify the metric is incremented from the executor (not the handler — spec § AC 12):
```
grep -n 'metrics.IncPRPublished' /workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor.go
```
Must show at least four `IncPRPublished` calls with labels `create`, `skipped` (×2), `kafka_error`, `trust_error`.

Run the new tests:
```
cd /workspace/watcher/github-pr && go test -mod=mod -v -count=1 ./pkg/command/... -run "TriggerPRReview|exit.path|runTriggerPRReview"
```
Expected: exit code 0; the table-driven exit-path test passes all 8 entries; the byte-identical parity test passes; the crash-recovery test passes.

Run the metric ownership test:
```
cd /workspace/watcher/github-pr && go test -mod=mod -v -count=1 ./pkg/... -run "github_pr_published"
```
Expected: exit code 0; the new `Describe("github_pr_published metric (spec 066 AC 12)")` block passes.

Run the full module test suite to confirm no regression in handler tests (the handler is unchanged in this prompt — its tests should still pass):
```
cd /workspace/watcher/github-pr && make test
```
Expected: exit code 0; all pre-existing tests pass unchanged; the new `pkg/command/` tests are additive.
</verification>
