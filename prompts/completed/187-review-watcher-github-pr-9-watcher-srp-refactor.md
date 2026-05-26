---
status: completed
summary: Refactored watcher struct from 12 to 7 fields by extracting TaskConfig value type and TaskPublisher interface/struct; all existing tests pass, precommit clean
container: maintainer-exec-187-review-watcher-github-pr-9-watcher-srp-refactor
dark-factory-version: v0.173.0
created: "2026-05-24T00:00:00Z"
queued: "2026-05-26T15:34:05Z"
started: "2026-05-26T15:34:07Z"
completed: "2026-05-26T15:37:59Z"
---

<summary>
- `watcher` struct (`watcher.go:59`) has 12 injected fields — god-object code smell
- 4 of those fields are pure task-config (stage, maxSlugLen, maxTitleLen, taskSuffix) — bundle into `TaskConfig`
- 3 fields belong to publish concern (createSender, trustDecision, metrics) — extract `taskPublisher` struct + `PublishCreate` method
- `publishCreate` (lines 248-303, 56 lines) becomes `taskPublisher.PublishCreate` — clearer ownership
- Result: `watcher` struct shrinks from 12 → 7 fields (keeps `metrics` for the two metric calls it still owns: `IncPollCycle` on Poll start/end + `IncPRPublished("skipped")` on filter-skip); `NewWatcher` signature from 12 → 7 args
</summary>

<objective>
Reduce `watcher` struct field count and clarify ownership by (1) bundling task-publish config into a `TaskConfig` value type and (2) extracting trust+publish logic into a `taskPublisher` struct exposed via a small `TaskPublisher` interface. Do NOT introduce a separate PR-fetch interface — `GitHubClient` already exposes `SearchPRs` and `GetPRDetails`, and `BuildCreateCommand` is already factored out as a free function.
</objective>

<context>
Read CLAUDE.md for project conventions.
Files to read in full before making changes:
- `watcher/github-pr/pkg/watcher.go` (437 lines) — confirm `Watcher` iface (line 24), `NewWatcher` (line 29), `watcher` struct (line 59), `Poll` (line 74), `fetchAllPRs` (line 107), `processPRs` (line 157), `publishCreate` (line 248), `BuildCreateCommand` already at line 307, `fetchPRDetails` (line 357)
- `watcher/github-pr/pkg/factory/factory.go` — `CreateWatcher` signature (line 57) takes the same 12 args as `NewWatcher`
- `watcher/github-pr/pkg/watcher_test.go` (1212 lines) — `newTestWatcher` helper (line 27) calls `pkg.NewWatcher(...)` with positional 12 args
- `watcher/github-pr/pkg/githubclient.go` — `GitHubClient` interface already has `SearchPRs` and `GetPRDetails` (line 75-89). Do NOT create a redundant PRSourcer interface.

Current `watcher` struct fields (12):
```
ghClient, createSender, cursorPath, startTime, scope, taskCreationFilter,
stage, metrics, trustDecision, maxSlugLen, maxTitleLen, taskSuffix
```

This refactor's *only* concern: SRP cleanup. Public behavior must not change. All existing tests in `watcher_test.go` must still pass — only `newTestWatcher` (line 27) needs updating, individual `It(...)` blocks should be unchanged.
</context>

<requirements>

**Execute steps in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Add `TaskConfig` value type in `watcher.go`** (just below the `Watcher` interface, around line 27):
   ```go
   // TaskConfig groups the per-task publishing configuration.
   type TaskConfig struct {
       Stage       string
       MaxSlugLen  int
       MaxTitleLen int
       TaskSuffix  string
   }
   ```
   No methods, no validation — pure value type.

2. **Add `TaskPublisher` interface and `taskPublisher` struct in `watcher.go`** (after `TaskConfig`):
   ```go
   //counterfeiter:generate -o mocks/task_publisher.go --fake-name TaskPublisher . TaskPublisher

   // TaskPublisher publishes create-task commands for a given PR + details pair.
   // Returns true on successful publish, false on trust check failure or send failure.
   type TaskPublisher interface {
       PublishCreate(ctx context.Context, pr PullRequest, taskIDStr string, details PRDetails) bool
   }

   // NewTaskPublisher returns a TaskPublisher that performs trust evaluation
   // then publishes a CreateTaskCommand via the given CreateCommandSender.
   func NewTaskPublisher(
       createSender task.CreateCommandSender,
       trustDecision trust.Trust,
       metrics Metrics,
       cfg TaskConfig,
   ) TaskPublisher {
       return &taskPublisher{
           createSender:  createSender,
           trustDecision: trustDecision,
           metrics:       metrics,
           cfg:           cfg,
       }
   }

   type taskPublisher struct {
       createSender  task.CreateCommandSender
       trustDecision trust.Trust
       metrics       Metrics
       cfg           TaskConfig
   }
   ```

3. **Move existing `publishCreate` body (lines 248-303) into `taskPublisher.PublishCreate`**:
   - Rename method receiver `w *watcher` → `p *taskPublisher`
   - Replace `w.trustDecision` → `p.trustDecision`, `w.metrics` → `p.metrics`, `w.createSender` → `p.createSender`
   - Replace `w.stage` → `p.cfg.Stage`, `w.maxSlugLen` → `p.cfg.MaxSlugLen`, `w.maxTitleLen` → `p.cfg.MaxTitleLen`, `w.taskSuffix` → `p.cfg.TaskSuffix`
   - Logic is unchanged. Same 4-arg signature `(ctx, pr, taskIDStr, details) bool`. Both trusted and untrusted branches stay intact (use `BuildCreateCommand`-style inline calls already present).

4. **Update `watcher` struct (line 59) to 7 fields**:

   `metrics` stays on `watcher` because `Poll` calls `IncPollCycle` (lines 82, 101) and `processPRs` calls `IncPRPublished("skipped")` (line 185) — both before any publish happens, so they can't move into the publisher.

   ```go
   type watcher struct {
       ghClient           GitHubClient
       publisher          TaskPublisher
       metrics            Metrics
       cursorPath         string
       startTime          libtime.DateTime
       scope              string
       taskCreationFilter filter.TaskCreationFilter
   }
   ```

   Update `NewWatcher` (line 29) signature to 7 args in this order:
   ```go
   func NewWatcher(
       ghClient GitHubClient,
       publisher TaskPublisher,
       metrics Metrics,
       cursorPath string,
       startTime libtime.DateTime,
       scope string,
       taskCreationFilter filter.TaskCreationFilter,
   ) Watcher
   ```

   Update `processPRs` (line 157): replace `w.publishCreate(ctx, pr, taskIDStr, details)` with `w.publisher.PublishCreate(ctx, pr, taskIDStr, details)`.

5. **Update `factory/factory.go` `CreateWatcher` (line 57)**: still accepts the 12 args from `main.go`, but composes internally:
   ```go
   func CreateWatcher(
       httpClient *http.Client,
       createSender task.CreateCommandSender,
       cursorPath string,
       startTime libtime.DateTime,
       scope string,
       taskCreationFilter filter.TaskCreationFilter,
       stage string,
       metrics pkg.Metrics,
       trustDecision trust.Trust,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
   ) pkg.Watcher {
       ghClient := pkg.NewGitHubClient(httpClient)
       publisher := pkg.NewTaskPublisher(
           createSender,
           trustDecision,
           metrics,
           pkg.TaskConfig{
               Stage:       stage,
               MaxSlugLen:  maxSlugLen,
               MaxTitleLen: maxTitleLen,
               TaskSuffix:  taskSuffix,
           },
       )
       return pkg.NewWatcher(
           ghClient,
           publisher,
           metrics,
           cursorPath,
           startTime,
           scope,
           taskCreationFilter,
       )
   }
   ```
   `main.go` does NOT change.

6. **Update `watcher_test.go` helper `newTestWatcher` (line 27)** to use the new API. Construct a `taskPublisher` from the supplied mocks:
   ```go
   func newTestWatcher(
       ghClient pkg.GitHubClient,
       createSender task.CreateCommandSender,
       cursorPath string,
       startTime libtime.DateTime,
       fakeMetrics *mocks.Metrics,
       trustDecision trust.Trust,
   ) pkg.Watcher {
       publisher := pkg.NewTaskPublisher(
           createSender,
           trustDecision,
           fakeMetrics,
           pkg.TaskConfig{
               Stage:       "dev",
               MaxSlugLen:  pkg.DefaultMaxSlugLen,
               MaxTitleLen: pkg.DefaultMaxTitleLen,
               TaskSuffix:  "",
           },
       )
       return pkg.NewWatcher(
           ghClient,
           publisher,
           fakeMetrics,
           cursorPath,
           startTime,
           "bborbe",
           filter.TaskCreationFilters{
               filter.NewDraftFilter(),
               filter.NewBotAuthorFilter([]string{"dependabot[bot]"}),
           },
       )
   }
   ```
   The individual `It(...)` blocks in the file should NOT need changes since they all call `newTestWatcher`.

7. **Regenerate mocks**:
   ```bash
   cd watcher/github-pr && make generate
   ```
   This creates `pkg/mocks/task_publisher.go` and refreshes `pkg/mocks/watcher.go` (already exists from `counterfeiter:generate` on the `Watcher` interface).

8. **Add one `TaskPublisher` contract test in `watcher_test.go`**: with the publisher dispatch now crossing an interface boundary, add a single `It("calls publisher.PublishCreate with derived taskID for new (PR,SHA) pairs")` block. Bypass `newTestWatcher` — construct the SUT directly via `pkg.NewWatcher(ghClient, fakePublisher, fakeMetrics, ...)` where `fakePublisher` is `new(mocks.TaskPublisher)`. Drive a single `Poll` with one new-SHA PR in the SearchPRs return, configure `GetPRDetails` to return non-empty `HeadSHA`, and assert `fakePublisher.PublishCreateCallCount() == 1` and the captured args match `(ctx, pr, taskIDStr, details)`. Place near the existing publish-related tests; do not modify other `It` blocks.

9. **Run `make test`** and fix any failures:
   ```bash
   cd watcher/github-pr && make test
   ```

10. **Run `make precommit`**:
    ```bash
    cd watcher/github-pr && make precommit
    ```

</requirements>

<constraints>
- Only change: `watcher/github-pr/pkg/watcher.go`, `watcher/github-pr/pkg/factory/factory.go`, `watcher/github-pr/pkg/watcher_test.go` (and auto-generated `watcher/github-pr/pkg/mocks/task_publisher.go` via `make generate`)
- Do NOT modify `watcher/github-pr/main.go` — its call to `factory.CreateWatcher` must keep working
- Do NOT introduce `PRSourcer` — `GitHubClient` already serves both `SearchPRs` and `GetPRDetails`
- Do NOT touch `BuildCreateCommand` (line 307) — already factored out, used by single-PR trigger handler
- Do NOT change public behavior — all existing test assertions must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- Do NOT commit — dark-factory handles git
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm watcher struct has 7 fields:
grep -A 9 "^type watcher struct" pkg/watcher.go

# Confirm TaskPublisher interface exists:
grep -A 3 "^type TaskPublisher interface" pkg/watcher.go

# Confirm taskPublisher struct exists:
grep -A 6 "^type taskPublisher struct" pkg/watcher.go

# Confirm TaskConfig exists:
grep -A 6 "^type TaskConfig struct" pkg/watcher.go

# Confirm NewWatcher is now 7-arg:
grep -A 8 "^func NewWatcher" pkg/watcher.go
</verification>
