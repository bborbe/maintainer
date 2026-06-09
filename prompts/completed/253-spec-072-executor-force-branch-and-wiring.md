---
status: completed
spec: ["072"]
summary: Plumbed libtime.CurrentDateTimeGetter through executor + factory + main.go; executor now branches on cmd.Force to call DeriveTaskIDForce (salted) vs DeriveTaskID (canonical); added 5 Ginkgo tests covering force-true/non-force/byte-identity/two-triggers-distinct/metric-label-invariance; make precommit exits 0 with 93.2% coverage on pkg/command
container: maintainer-trigger-force-exec-253-spec-067-executor-force-branch-and-wiring
dark-factory-version: v0.175.0
created: "2026-06-09T15:50:00Z"
queued: "2026-06-09T16:02:46Z"
started: "2026-06-09T16:22:09Z"
completed: "2026-06-09T16:37:59Z"
branch: dark-factory/force-trigger-on-github-pr-watcher
---

<summary>
- Threads `libtime.CurrentDateTimeGetter` through the executor so the `force=true` branch can derive a salted task identifier without a `time.Now()` call in business logic.
- The executor now branches on `cmd.Force`: when `true`, it computes a nanosecond-resolved nonce from the injected clock and calls `DeriveTaskIDForce`; when `false`, it still calls `DeriveTaskID` — byte-identical to today's output.
- The factory gains the new dependency parameter and passes it through to the executor constructor; `main.go` constructs `libtime.NewCurrentDateTime()` once and injects it.
- Five new Ginkgo unit tests pin the new behavior: force uses salted ID, non-force uses canonical ID, non-force payload is byte-identical, two force calls with advanced clock produce distinct IDs, and the `IncPRPublished("create")` metric is still emitted exactly once.
- The poll path's `DeriveTaskID` call at `pkg/watcher.go:279` is unchanged (whitespace-only diff at most).
</summary>

<objective>
Plumb a `libtime.CurrentDateTimeGetter` into the single-PR trigger executor, branch the executor on `cmd.Force` to call `DeriveTaskIDForce` (from prompt 1) with a nanosecond-resolved nonce when `Force=true` and to call `DeriveTaskID` unchanged when `Force=false`, and update `factory.CreateCommandConsumer` + `main.go` wiring to inject the clock. The non-force path remains byte-identical to today's `CreateTaskCommand` payload; the `Force=true` path produces a `TaskIdentifier` that is always different from the canonical `(owner, repo, number, sha)`-derived one.
</objective>

<context>
Read the project conventions and the relevant docs:
- `/workspace/CLAUDE.md` (project-wide rules)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-time-injection.md` (mandatory for this prompt — defines `CurrentDateTimeGetter` injection and the `NewCurrentDateTime()` once-in-main.go pattern)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-cqrs.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`

Read these source files fully before editing (chunked reads if any exceed 2000 lines):
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor.go` — you are extending the constructor signature and `publishCreateCommand`; the rest of the file is unchanged.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor_export_test.go` — re-exports `runTriggerPRReview` as `RunTriggerPRReview`; you must update this file in lockstep or the external tests fail to compile.
- `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor_test.go` — existing Ginkgo test layout (`exit-path mapping` table-driven, `executor vs handler payload parity` block, `executor crash recovery` block). All these tests construct `RunTriggerPRReview` directly — every one needs the new `currentDateTime` parameter passed.
- `/workspace/watcher/github-pr/pkg/factory/factory.go` — extends `CreateCommandConsumer` and the `NewTriggerPRReviewCommandExecutor` call site.
- `/workspace/watcher/github-pr/pkg/factory/integration_test.go` — drives the executor directly and constructs `command.NewTriggerPRReviewCommandExecutor`; needs the new parameter.
- `/workspace/watcher/github-pr/pkg/factory/command_consumer_test.go` — calls `factory.CreateCommandConsumer`; needs the new parameter.
- `/workspace/watcher/github-pr/main.go` — wires `libtime.NewCurrentDateTime()` and passes it into `CreateCommandConsumer`.
- `/workspace/watcher/github-pr/pkg/taskid.go` — confirms the new `DeriveTaskIDForce` helper from prompt 1 is present and has the expected signature `func(owner, repo string, number int, sha, nonce string) uuid.UUID`.
- `/workspace/specs/in-progress/067-force-trigger-on-github-pr-watcher.md` — Goals, Constraints, Failure Modes, Acceptance Criteria sections.

Sibling entry-point check (per GLOBAL rules): this watcher is single-binary (`main.go` only — there is no `cmd/run-once/main.go` or `cmd/cli/main.go` equivalent, confirmed by `ls /workspace/watcher/github-pr/`). Only `main.go` needs wiring updates.
</context>

<requirements>

1. **Extend the executor constructor signature in `pkg/command/trigger_pr_review_executor.go`.** Add one new parameter — `currentDateTime libtime.CurrentDateTimeGetter` — at the END of the parameter list of `NewTriggerPRReviewCommandExecutor` and `runTriggerPRReview`. Place it after `metrics pkg.Metrics` so the existing callers' positional-argument lists grow by one in a known location. Rationale: appending (not inserting in the middle) means mechanical find-and-replace and keeps the diff minimal for prompt 3's call sites (which are out of scope for this prompt but will hit the same constructors).

   Concretely, the new signature is:

   ```go
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
       currentDateTime libtime.CurrentDateTimeGetter, // <-- NEW, appended
   ) cdb.CommandObjectExecutorTx
   ```

   Add the import `libtime "github.com/bborbe/time"` (it is already imported by `pkg/watcher.go` — confirm the import path is the same and add it to this file's import block). Also import `strconv` for the `FormatInt` call.

2. **Thread `currentDateTime` through to `publishCreateCommand`.** Extend its signature the same way. Update the constructor closure in `NewTriggerPRReviewCommandExecutor` to pass `currentDateTime` to `runTriggerPRReview`, and update `runTriggerPRReview` to pass it to `publishCreateCommand`.

3. **Branch on `cmd.Force` inside `publishCreateCommand`.** This is the only behavioral change to the executor body. Replace the existing canonical-ID line:

   ```go
   taskIDStr := pkg.DeriveTaskID(
       prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA,
   ).String()
   ```

   with:

   ```go
   var taskIDStr string
   if cmd.Force {
       nonce := strconv.FormatInt(
           currentDateTime.Now().UnixNano(), 10,
       )
       taskIDStr = pkg.DeriveTaskIDForce(
           prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA, nonce,
       ).String()
   } else {
       taskIDStr = pkg.DeriveTaskID(
           prInfo.Owner, prInfo.Repo, prInfo.Number, details.HeadSHA,
       ).String()
   }
   ```

   The non-force branch is byte-identical to the existing line. The force branch derives the nonce from `currentDateTime.Now().UnixNano()` and passes it to `DeriveTaskIDForce`.

   Do NOT log the nonce in production. (The nonce leaks no security-sensitive data, but logging it adds noise to `glog` without operator benefit.) The existing `glog.V(2).Infof("trigger executor: published task_id=%s ...")` line stays unchanged.

4. **Update the executor re-export in `pkg/command/trigger_pr_review_executor_export_test.go`.** The `var _ = func(...)` compile-time guard at the bottom of the file MUST be updated to add the `currentDateTime libtime.CurrentDateTimeGetter` parameter and pass it through, otherwise the file fails to build. The var-export alias `RunTriggerPRReview = runTriggerPRReview` does not need a signature change (Go re-exports the function value, not its type).

5. **Update every existing call site of `RunTriggerPRReview` and `NewTriggerPRReviewCommandExecutor`.** The signature change breaks all callers. Update them in lockstep:

   - `/workspace/watcher/github-pr/pkg/command/trigger_pr_review_executor_test.go` — every invocation of `command.RunTriggerPRReview(...)` needs `currentDateTime` appended. Use `libtime.NewCurrentDateTime()` for the existing tests (they don't assert nonce behavior, so a real clock is fine). Do NOT modify the test logic or the table entries.
   - `/workspace/watcher/github-pr/pkg/factory/integration_test.go` — the same call site at the `executor = command.NewTriggerPRReviewCommandExecutor(...)` assignment needs the new parameter. The wired `factory.CreateCommandConsumer(...)` call below it also needs the new parameter (see step 6).
   - `/workspace/watcher/github-pr/pkg/factory/command_consumer_test.go` — the `factory.CreateCommandConsumer(...)` invocation needs the new parameter.

   For test-side `currentDateTime`, prefer `libtime.NewCurrentDateTime()` for cases that don't manipulate time and `libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime { ... })` for cases that need a frozen/advanced clock (only the new `TestExecutor_TwoForceTriggersProduceDifferentIDs` test, defined in step 8).

6. **Extend `factory.CreateCommandConsumer` in `pkg/factory/factory.go`.** Add `currentDateTime libtime.CurrentDateTimeGetter` to the parameter list (appended after `branch base.Branch`, the last param). Forward it into the `command.NewTriggerPRReviewCommandExecutor` call. Update the doc comment to mention the new parameter. The factory body must remain zero-logic — no conditionals, no loops (per the existing `CreateCommandConsumer body has no control flow` test).

7. **Wire the clock in `main.go`.** Construct `libtime.NewCurrentDateTime()` ONCE near the top of `Run` (next to the existing `startTime := libtime.NewCurrentDateTime().Now()` line — or use a single named var that you reference twice). Pass it into the `factory.CreateCommandConsumer(...)` call. Do NOT construct a second `libtime.NewCurrentDateTime()` anywhere else in this file.

   ```go
   currentDateTime := libtime.NewCurrentDateTime()
   // ...
   commandConsumer := factory.CreateCommandConsumer(
       saramaClientProvider, syncProducer, db, ghClient, createSender,
       taskCreationFilter, trustDecision, a.Stage, a.MaxSlugLen,
       a.MaxTitleLen, a.TaskSuffix, metrics, branch,
       currentDateTime, // <-- NEW, appended
   )
   ```

8. **Add five new Ginkgo tests in `pkg/command/trigger_pr_review_executor_test.go`.** Add a new top-level `Describe` block (do not modify the existing three `Describe` blocks). Use a fake clock so the tests are deterministic. The test structure mirrors the existing `BeforeEach` block in the file — clone it, but inject a `CurrentDateTimeGetterFunc` with a fixed `libtime.DateTime` value.

   Use these test names (Ginkgo `It` strings, mapped directly to spec AC names):
   - `It("TestExecutor_ForceTrueUsesSaltedID")` — feed a `TriggerPRReviewCommand{URL: validPRURL, Force: true}` to `RunTriggerPRReview`; capture the `CreateCommand` the mock `createSender` receives; assert its `TaskIdentifier` is NOT equal to `pkg.DeriveTaskID("bborbe", "repo", 42, "abc123").String()` (the canonical ID for the fixture). Use the fixture values already in the file's existing `BeforeEach` (`Owner="bborbe"`, `Repo="repo"`, `Number=42`, `HeadSHA="abc123"`).
   - `It("TestExecutor_ForceFalseUsesCanonicalID")` — same as above with `Force: false`; assert the captured `TaskIdentifier` EQUALS `pkg.DeriveTaskID("bborbe", "repo", 42, "abc123").String()`.
   - `It("TestExecutor_ForceFalseProducesIdenticalCreateCommand")` — with `Force: false`, capture the full `CreateCommand` and assert field-by-field equality on every field (`Title`, `TaskIdentifier`, `Frontmatter`, `Body`) against `pkg.BuildCreateCommand(<fixture>, "dev", 80, 200, "", trust.NewResult(true, "trusted"))` where the fixture is the same `pkg.PullRequest` / `pkg.PRDetails` pair the executor constructs internally. Document the fixture inline in a comment. The most portable form is to assert the `TaskIdentifier` equals the canonical `DeriveTaskID(...)` string AND assert `Body` is identical to `pkg.BuildCreateCommand(...).Body` (since `BuildCreateCommand` is the canonical builder).
   - `It("TestExecutor_TwoForceTriggersProduceDifferentIDs")` — invoke `RunTriggerPRReview` twice in sequence with `Force: true`. Between the two invocations, advance the fake clock by setting the `CurrentDateTimeGetterFunc` to return `t1` then `t2` where `t2.UnixNano() != t1.UnixNano()`. Capture both `CreateCommand` outputs and assert `Expect(cmd1.TaskIdentifier).NotTo(Equal(cmd2.TaskIdentifier))`. Define a `var fakeNow libtime.DateTime` in `BeforeEach` and a getter `libtime.CurrentDateTimeGetterFunc(func() libtime.DateTime { return fakeNow })`; advance `fakeNow` by assigning a different value between calls.
   - `It("TestExecutor_ForceTrueIncrementsCreateLabel")` — use a `mocks.Metrics` (counterfeiter) instead of `pkg.NewMetrics()`. Drive the executor with `Force: true`. Assert `metrics.IncPRPublishedCallCount() == 1` and `metrics.IncPRPublishedArgsForCall(0) == "create"`. Build a `map[string]bool` from all `IncPRPublishedArgsForCall` values; assert `Expect(labels).To(Equal(map[string]bool{"create": true}))` — no `"force"`, `"forced_create"`, `"forced"`, etc.

   The new tests must NOT alter the existing three `Describe` blocks. They live in a new `Describe("force-true branch (spec 067)", func() { ... })` block at the end of the file.

9. **Verify the negative AC.** Run `! grep -rnE '\btime\.Now\(\)' pkg/command/ pkg/taskid.go pkg/factory/`. It must exit 0 — no `time.Now()` introduced. The only new time access in `pkg/command/` is `currentDateTime.Now().UnixNano()` (injected clock).

10. **Verify the diff-against-master ACs.** After the prompt, `git diff master -- pkg/watcher.go` shows whitespace-only or no changes around the poll path's `DeriveTaskID` call. `git diff master -- pkg/metrics.go` shows no changes (the existing label set in `init()` at line 38 is untouched). `git diff master -- pkg/taskid.go` shows only the additive `DeriveTaskIDForce` function (prompt 1's change) and nothing else.

</requirements>

<constraints>
- The non-force path's published `CreateTaskCommand` MUST be byte-identical to today's output. Every byte of the `Force=false` branch is pinned by the spec Constraint. The `TestExecutor_ForceFalseProducesIdenticalCreateCommand` test enforces this — do not weaken it.
- The `Force` field on `TriggerPRReviewCommand` is already shipped (spec 066). Do not rename, retype, or move it.
- The metric label for a successful `Force=true` publish is the existing `create` label. No new label is introduced (spec Non-goal + AC).
- The `libtime.CurrentDateTimeGetter` from `github.com/bborbe/time` is the only allowed clock source in business logic (per `go-time-injection.md`). No direct `time.Now()` in executor or helper code.
- `SinglePRTriggerHandler`, `NewSinglePRTriggerHandler`, and the HTTP wire shape (`{"status":"accepted","url":<raw>}`) are FROZEN in this prompt — prompt 3 owns the HTTP-side change. Do NOT touch `pkg/handler/trigger_handler.go` in this prompt.
- Do NOT modify the poll path's `DeriveTaskID` call at `pkg/watcher.go:279` (spec Non-goal: `Force` is a `/trigger`-only concept).
- The poll path's `watcher.Poll` does NOT take a `CurrentDateTimeGetter` and does NOT need one — only the trigger executor does.
- Do NOT introduce a config flag, an opt-out, or a tunable threshold for the `force` mechanism. The spec's Non-goals explicitly forbid an escape hatch.
- Do NOT add a CHANGELOG bullet in this prompt — prompt 3 owns the user-facing release note.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
Run from `/workspace/watcher/github-pr/`:

```bash
go test ./pkg/command -v
go test ./pkg/factory -v
go test ./pkg/handler -v
! grep -rnE '\btime\.Now\(\)' pkg/command/ pkg/taskid.go pkg/factory/
grep -nE 'libtime\.CurrentDateTimeGetter' pkg/command/trigger_pr_review_executor.go pkg/factory/factory.go
grep -nE 'libparse\.ParseBoolDefault|FormValue\("force"\)' pkg/handler/trigger_handler.go || true
```

Expected:
- `go test ./pkg/command -v` runs the existing `NewTriggerPRReviewCommandExecutor` exit-path mapping, the `executor vs handler payload parity` block, the `executor crash recovery` block, AND the new `force-true branch` block. All PASS.
- `go test ./pkg/factory -v` passes (the `integration_test.go` and `command_consumer_test.go` updates compile and run).
- `go test ./pkg/handler -v` passes (this prompt does not touch the handler, but the suite test confirms we did not break compilation).
- `! grep` exits 0 (no `time.Now()` in business logic paths).
- `grep CurrentDateTimeGetter` returns at least one line in the executor and one in the factory.
- The handler grep is allowed to return no lines yet (prompt 3 owns that).

Then run the full precommit for this module:

```bash
cd /workspace/watcher/github-pr && make precommit
```

`make precommit` must exit 0. If lint complains about funlen on `main.go` (it was already at 82 lines), the addition of one or two more lines is fine — the `//nolint:funlen` directive is already in place and the diff stays under 90 lines.

Also run a diff check to confirm the load-bearing ACs from spec 067:

```bash
cd /workspace && git diff master -- watcher/github-pr/pkg/watcher.go watcher/github-pr/pkg/metrics.go
```

Expected: empty or whitespace-only on `watcher.go`; empty on `metrics.go` (no label added).
</verification>
