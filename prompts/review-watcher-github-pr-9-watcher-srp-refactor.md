---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `watcher` struct has 12 injected fields (god object) — exceeds 8-field threshold and indicates too many responsibilities
- `publishCreate` (56 lines) mixes 4 concerns: trust evaluation, command construction, Kafka send, and logging/metrics
- `processPRs` (83 lines) mixes 3 concerns: filtering with cursor-side-effects, per-PR API fetch with caching, and cursor map pruning
- `application.Run` (119 lines across 3 methods) mixes config validation, time parsing, filter construction, factory wiring, and goroutine orchestration
</summary>

<objective>
Split the `watcher` struct into focused components: extract `TaskPublisher` (trust + Kafka), extract `PRFetcher` (GitHub I/O + filtering), and keep `Watcher` as thin orchestrator. Break up `publishCreate` into isolated methods.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `go-srp-checker.md` (if exists) or review findings from srp-checker agent.

Files to read before making changes:
- `watcher/github-pr/pkg/watcher.go` — full file; understand all 5 methods and 12 struct fields
- `watcher/github-pr/pkg/watcher_test.go` — full file; understand test patterns and dependencies

This is a large refactor — the goal is to improve cohesion by grouping fields by responsibility. Read the file carefully before starting.
</context>

<requirements>

**Execute steps in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Define focused interfaces in `watcher.go`:**

   Add after the existing `Watcher` interface:
   ```go
   // PRSourcer fetches PRs from GitHub.
   type PRSourcer interface {
       FetchAllPRs(ctx context.Context, scope string, since time.Time) ([]PullRequest, error)
       GetPRDetails(ctx context.Context, owner, repo string, number int) (PullRequestDetails, error)
   }

   // TaskPublisher publishes create-task commands to Kafka.
   type TaskPublisher interface {
       PublishCreate(ctx context.Context, pr PullRequestDetails, trustResult trust.Result, taskIDStr string, stage string, maxSlugLen, maxTitleLen int, taskSuffix string) bool
   }
   ```

2. **Extract `taskPublisher` struct:**

   Create a new struct that groups the Kafka + trust fields:
   ```go
   type taskPublisher struct {
       ghClient           GitHubClient
       createSender       task.CreateCommandSender
       trustDecision      trust.Trust
       stage              string
       maxSlugLen         int
       maxTitleLen        int
       taskSuffix         string
       metrics            Metrics
   }
   ```

   Extract `publishCreate` logic into this struct as `PublishCreate` method.

3. **Extract `PRFetcher` struct:**

   Create a new struct that groups the GitHub I/O fields:
   ```go
   type prFetcher struct {
       ghClient           GitHubClient
       scope              string
       taskCreationFilter filter.TaskCreationFilter
   }
   ```

   Extract `fetchAllPRs` and `GetPRDetails` (if not already on GitHubClient) into this struct.

4. **Simplify the `watcher` struct:**

   After extraction, the `watcher` struct should hold:
   ```go
   type watcher struct {
       prFetcher     PRSourcer
       publisher     TaskPublisher
       cursorPath    string
       startTime     libtime.DateTime
       metrics       Metrics
   }
   ```

   Update `NewWatcher` to accept the new interfaces:
   ```go
   func NewWatcher(
       prFetcher PRSourcer,
       publisher TaskPublisher,
       cursorPath string,
       startTime libtime.DateTime,
       metrics Metrics,
   ) Watcher {
       return &watcher{
           prFetcher:  prFetcher,
           publisher:  publisher,
           cursorPath: cursorPath,
           startTime:  startTime,
           metrics:   metrics,
       }
   }
   ```

   Update `Poll` to delegate to the extracted components.

5. **Update factory and tests:**

   Update `CreateWatcher` in `factory.go` to compose the new interfaces and pass the extracted structs.

   Update `watcher_test.go` to use the new interface types with mocks.

6. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix compilation errors iteratively.

7. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only change `watcher/github-pr/pkg/watcher.go`, `watcher/github-pr/pkg/factory/factory.go`, and `watcher/github-pr/pkg/watcher_test.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass — update mocks to match new interface types
- Interfaces must be small (1-2 methods)
- Coverage ≥80% for changed packages
- Do NOT add new public types without good reason — keep changes minimal
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm watcher struct has fewer fields:
grep -A 15 "type watcher struct" watcher/github-pr/pkg/watcher.go

# Confirm PRSourcer interface:
grep -A 5 "type PRSourcer interface" watcher/github-pr/pkg/watcher.go

# Confirm TaskPublisher interface:
grep -A 5 "type TaskPublisher interface" watcher/github-pr/pkg/watcher.go
</verification>
