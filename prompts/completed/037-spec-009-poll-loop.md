---
status: completed
spec: [009-github-pr-watcher]
summary: Implemented cursor persistence, CommandPublisher, full poll cycle with force-push detection and rate-limit backoff, and wired main.go tick loop for github-pr-watcher
container: code-reviewer-037-spec-009-poll-loop
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-27T20:45:00Z"
queued: "2026-04-27T21:02:13Z"
started: "2026-04-27T21:16:36Z"
completed: "2026-04-27T21:32:38Z"
---

<summary>
- A `Cursor` type persists watcher state to `/data/cursor.json` (PVC-backed): `last_updated_at` timestamp and a `head_shas` map of `task_identifier → head SHA`.
- On missing or corrupt cursor file the watcher falls back to service-start-time (cold start), logging the reason.
- A `CommandPublisher` interface (Counterfeiter fake) abstracts Kafka publishing of `CreateTaskCommand` and `UpdateFrontmatterCommand` from `bborbe/agent/lib`.
- The full poll cycle is implemented: paginate GitHub results → apply filters → derive task identifier → check cursor → publish `CreateTaskCommand` for new PRs or `UpdateFrontmatterCommand` for force-pushed PRs → advance in-memory cursor per-PR → persist cursor after the poll cycle if all Kafka publishes succeeded.
- Rate-limit proactive backoff: if `SearchResult.RateRemaining` drops below 10, skip remaining pages, sleep until `RateResetAt`, and return with cursor unchanged.
- A poll cycle that fails entirely (GitHub error, Kafka publish failure) logs the error, leaves the cursor unchanged, and returns nil so the outer tick loop retries next interval without crashing the process.
- `main.go` is updated to wire `GitHubClient`, `Cursor`, `CommandPublisher`, and `Watcher` together via a `Factory`.
- `make test` and `make precommit` pass for `watcher/github/`.
</summary>

<objective>
Implement cursor persistence, Kafka command publishing, and the full poll cycle, then wire everything into `main.go`. After this prompt the service is functionally complete: it polls GitHub, filters PRs, derives stable identifiers, publishes `CreateTaskCommand` / `UpdateFrontmatterCommand` to Kafka, and persists cursor state across restarts.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read the following guides from `~/.claude/plugins/marketplaces/coding/docs/`:
- `go-patterns.md` — Interface → Constructor → Struct, counterfeiter annotations
- `go-testing-guide.md` — Ginkgo v2/Gomega, coverage ≥80%, external test packages
- `go-error-wrapping-guide.md` — `bborbe/errors`, never `fmt.Errorf`
- `go-context-cancellation-in-loops.md` — non-blocking select in loops
- `go-concurrency-patterns.md` — `run.CancelOnFirstErrorWait`, caller-owned channels
- `go-factory-pattern.md` — `Create*` prefix, zero logic in factories

**Preconditions:** Prompts `1-spec-009-scaffold` and `2-spec-009-github-client` must have been applied. Verify:
```bash
ls watcher/github/pkg/githubclient/client.go watcher/github/pkg/filter/filter.go watcher/github/pkg/taskid/taskid.go
```
If any are missing, stop and report `status: failed`.

Read these files fully before implementing:
- `watcher/github/main.go` — current stub poll loop to replace
- `watcher/github/pkg/config/config.go` — Config struct fields
- `watcher/github/pkg/githubclient/client.go` — `GitHubClient` interface, `SearchResult`, `PullRequest` types
- `watcher/github/pkg/filter/filter.go` — `ShouldSkip` function
- `watcher/github/pkg/taskid/taskid.go` — `Derive` function

**Symbol verification (mandatory before writing code):**

1. **`CreateTaskCommand`** — run:
   ```bash
   grep -rn "type CreateTaskCommand struct\|CreateTaskCommand{" \
     $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/... 2>/dev/null | head -20
   ```
   Read the full struct definition:
   ```bash
   grep -A 20 "type CreateTaskCommand struct" \
     $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/... 2>/dev/null
   ```
   Note exact field names — especially `TaskIdentifier`, `Body`, `Frontmatter` or whatever the actual field names are.

2. **`UpdateFrontmatterCommand`** — run:
   ```bash
   grep -A 20 "type UpdateFrontmatterCommand struct" \
     $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/... 2>/dev/null
   ```
   The spec says this command has a `BodySection` field (appends body section) and partial frontmatter reset. Confirm exact field names.

3. **Kafka publisher interface** — run:
   ```bash
   grep -rn "type.*Publisher interface\|Publish\|PublishCommand" \
     $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/... 2>/dev/null | head -20
   ```
   Understand how commands are published to Kafka in `bborbe/agent/lib`. Find the interface or function that accepts a command and publishes it to a Kafka topic. Note the exact interface method signature.

4. **`github.com/google/uuid` string form** — `uuid.UUID.String()` returns a lowercase hyphenated string. Confirm `taskid.Derive(...)` returns `uuid.UUID` and use `.String()` to convert to string for struct fields that expect `string`.
</context>

<requirements>
1. **Create `watcher/github/pkg/cursor/cursor.go`**:

   ```go
   package cursor

   import (
       "context"
       "encoding/json"
       "os"
       "time"

       "github.com/bborbe/errors"
       "github.com/golang/glog"
   )

   const DefaultCursorPath = "/data/cursor.json"

   // State holds the watcher's persisted cursor.
   type State struct {
       LastUpdatedAt time.Time         `json:"last_updated_at"`
       HeadSHAs      map[string]string `json:"head_shas"` // task_identifier → head SHA
   }

   // Load reads cursor state from path.
   // Returns (cold-start state with now=startTime, nil) if file is missing.
   // Returns (cold-start state with now=startTime, nil) if file is corrupt — logs warning.
   func Load(ctx context.Context, path string, startTime time.Time) (State, error)

   // Save persists cursor state to path atomically (write to temp file, rename).
   func Save(ctx context.Context, path string, state State) error
   ```

   Implementation of `Load`:
   - `os.ReadFile(path)` — if `os.IsNotExist(err)`: log `glog.V(2).Infof("cursor file not found, using cold-start time=%s", startTime.Format(time.RFC3339))`, return `State{LastUpdatedAt: startTime, HeadSHAs: make(map[string]string)}, nil`.
   - Other read error: return `errors.Wrapf(ctx, err, "read cursor file path=%s", path)`.
   - `json.Unmarshal` — if error: log `glog.Warningf("cursor file corrupt, using cold-start path=%s err=%v", path, err)`, return `State{LastUpdatedAt: startTime, HeadSHAs: make(map[string]string)}, nil`.
   - On success: if `state.HeadSHAs == nil`, set to `make(map[string]string)`.
   - Return loaded state.

   Implementation of `Save`:
   - Marshal state to JSON.
   - Write to `path + ".tmp"` with `os.WriteFile(path+".tmp", data, 0600)`.
   - Rename: `os.Rename(path+".tmp", path)`.
   - On any error, return `errors.Wrapf(ctx, err, "save cursor path=%s", path)`.

2. **Create `watcher/github/pkg/cursor/cursor_test.go`** (external package `cursor_test`, Ginkgo/Gomega):

   Use `os.MkdirTemp` to create a temporary directory for test cursor files.

   Test cases:
   - `Load` on missing file → returns cold-start state with provided `startTime`, no error
   - `Load` on corrupt JSON → returns cold-start state with provided `startTime`, no error (logs warning but does not error)
   - `Load` on valid JSON → returns correct `LastUpdatedAt` and `HeadSHAs`
   - `Load` with nil `HeadSHAs` in file → returns `HeadSHAs` as empty (non-nil) map
   - `Save` then `Load` round-trip → same state
   - `Save` to unwritable directory → returns non-nil error
   - `Load` then `Save` preserves existing `HeadSHAs` entries

3. **Define the `CommandPublisher` interface**:

   Create `watcher/github/pkg/publisher/publisher.go`:

   The agent ecosystem publishes commands via `cdb.CommandObjectSender` from `github.com/bborbe/cqrs`. There is NO `PublishCreate` / `PublishUpdateFrontmatter` API in `bborbe/agent/lib` — those are watcher-side facade method names. The lib provides typed command structs (`lib.CreateTaskCommand`, `lib.UpdateFrontmatterCommand`); the watcher marshals them into a `cdb.CommandObject` and calls `Send(ctx, CommandObject)`.

   **Verify the exact API** before writing the facade:
   ```bash
   grep -rn "type CommandObjectSender\|func.*CommandObjectSender.*Send" $(go env GOPATH)/pkg/mod/github.com/bborbe/cqrs@*/cdb/ 2>/dev/null | head
   grep -rn "NewCommandObjectSender" $(go env GOPATH)/pkg/mod/github.com/bborbe/cqrs@*/ 2>/dev/null | head
   grep -rn "CreateTaskCommand\|CreateTaskCommandOperation" $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.55.0/ | head
   ```

   Define a thin facade interface (watcher-side) so the poll loop tests don't need a real Kafka producer:

   ```go
   package publisher

   import (
       "context"

       agentlib "github.com/bborbe/agent/lib"
   )

   //counterfeiter:generate -o mocks/command_publisher.go --fake-name CommandPublisher . CommandPublisher
   type CommandPublisher interface {
       // PublishCreate marshals a lib.CreateTaskCommand into a cdb.CommandObject
       // and sends it via the underlying cdb.CommandObjectSender.
       PublishCreate(ctx context.Context, cmd agentlib.CreateTaskCommand) error

       // PublishUpdateFrontmatter marshals a lib.UpdateFrontmatterCommand the same way.
       PublishUpdateFrontmatter(ctx context.Context, cmd agentlib.UpdateFrontmatterCommand) error
   }
   ```

   The constructor wraps a `cdb.CommandObjectSender`:
   ```go
   // NewKafkaPublisher constructs a CommandPublisher backed by a real
   // cdb.CommandObjectSender wired to a Kafka SyncProducer.
   func NewKafkaPublisher(ctx context.Context, brokers libkafka.Brokers, branch base.Branch) (CommandPublisher, func(), error)
   ```
   Look at `agent/pr-reviewer/pkg/factory/factory.go` `CreateSyncProducer` + `CreateKafkaResultDeliverer` for the exact wiring pattern. The cleanup function closes the sync producer on shutdown.

   Internally each `PublishCreate` / `PublishUpdateFrontmatter`:
   1. Build a `cdb.Command{Operation: agentlib.CreateTaskCommandOperation, Initiator: "github-pr-watcher", Data: <serialized cmd>}` (look at `agent/pr-reviewer/pkg/factory/factory.go` and `lib/delivery/result-deliverer.go` for how existing publishers build the CommandObject).
   2. Wrap into `cdb.CommandObject` with the appropriate event metadata.
   3. Call `sender.Send(ctx, commandObject)`.
   4. Wrap any error via `errors.Wrapf(ctx, err, "publish <op>")`.

4. **Generate Counterfeiter mock for `CommandPublisher`**:
   ```bash
   cd watcher/github && go generate ./pkg/publisher/...
   ```
   Creates `watcher/github/pkg/publisher/mocks/command_publisher.go`.

5. **Create `watcher/github/pkg/watcher/watcher.go`** — the poll cycle:

   ```go
   package watcher

   // Watcher polls GitHub and publishes commands.
   //counterfeiter:generate -o mocks/watcher.go --fake-name Watcher . Watcher
   type Watcher interface {
       Poll(ctx context.Context) error
   }

   // NewWatcher returns a Watcher.
   func NewWatcher(
       ghClient githubclient.GitHubClient,
       publisher publisher.CommandPublisher,
       cursorPath string,
       startTime time.Time,
       scope string,
       botAllowlist []string,
       stage string,
       rateSafeThreshold int, // minimum remaining rate-limit quota before aborting poll cycle (default: 10)
   ) Watcher
   ```

   Implement `Poll(ctx context.Context) error` on the private struct:

   ```
   a. Load cursor state: cursor.Load(ctx, cursorPath, startTime)
      - On error: log and return nil (non-crash: retry next interval).

   b. Paginate GitHub search results:
      since := cursorState.LastUpdatedAt
      page := 1
      var allPRs []githubclient.PullRequest
      for {
          result, err := ghClient.SearchPRs(ctx, scope, since, page)
          if err != nil:
              log error, return nil (discard partial results, cursor unchanged, retry next poll)
          
          // Proactive rate-limit check
          if result.RateRemaining < rateSafeThreshold:
              glog.Warningf("github rate limit low remaining=%d reset=%s — aborting poll cycle", 
                  result.RateRemaining, result.RateResetAt.Format(time.RFC3339))
              // Sleep until rate reset (blocking, but context-aware)
              select {
              case <-ctx.Done():
                  return nil
              case <-time.After(time.Until(result.RateResetAt) + 5*time.Second):
              }
              return nil  // cursor unchanged; next poll will retry from same since point

          allPRs = append(allPRs, result.PullRequests...)

          if !result.HasNextPage:
              break
          page = result.NextPage

          // Context cancellation check between pages
          select {
          case <-ctx.Done():
              return nil
          default:
          }
      }

   c. Track the most-recent updatedAt seen during this poll (for advancing the cursor at the end):
      var maxUpdatedAt = since

   d. For each PR in allPRs:
      - If filter.ShouldSkip(pr, botAllowlist): log debug, continue.
      - taskID := taskid.Derive(pr.Owner, pr.Repo, pr.Number)
      - taskIDStr := taskID.String()
      
      - knownSHA, exists := cursorState.HeadSHAs[taskIDStr]
      
      - For PRs that need head-SHA tracking (new PR or known PR — every PR in the result set), call ghClient.GetHeadSHA(ctx, pr.Owner, pr.Repo, pr.Number) since Search API doesn't return it. Cache the result in a local map keyed on taskIDStr to avoid duplicate fetches.
        On err from GetHeadSHA: log error, continue (skip this PR; do not advance maxUpdatedAt past it).

      - If !exists (new PR):
          body := buildTaskBody(pr)
          frontmatter := buildFrontmatter(pr, taskIDStr, stage)
          cmd := agentlib.CreateTaskCommand{
              TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
              Frontmatter:    frontmatter,  // type is agentlib.TaskFrontmatter (map[string]interface{})
              Body:           body,         // string field per spec 017
          }
          if err := publisher.PublishCreate(ctx, cmd); err != nil:
              log error with pr URL, continue (cursor not advanced for this PR — retry next poll picks it up again)
          cursorState.HeadSHAs[taskIDStr] = headSHA  // from GetHeadSHA
          // Only advance maxUpdatedAt for THIS PR after successful publish:
          if pr.UpdatedAt > maxUpdatedAt: maxUpdatedAt = pr.UpdatedAt
          log debug "published CreateTaskCommand pr=%s/%s#%d taskID=%s"

      - If exists && knownSHA != headSHA (force-push):
          // UpdateFrontmatterCommand.Body is *lib.BodySection{Heading, Section}, NOT a string.
          // The lib doc says: Heading must include the markdown prefix; Section must include
          // the heading as its first line and a trailing newline.
          heading := fmt.Sprintf("## Outdated by force-push %s", knownSHA)
          section := heading + "\n"
          bodySection := &agentlib.BodySection{Heading: heading, Section: section}
          updates := agentlib.TaskFrontmatter{
              "phase":         "planning",
              "status":        "in_progress",
              "trigger_count": 0,
          }
          cmd := agentlib.UpdateFrontmatterCommand{
              TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
              Updates:        updates,
              Body:           bodySection,
          }
          if err := publisher.PublishUpdateFrontmatter(ctx, cmd); err != nil:
              log error with pr URL, continue
          cursorState.HeadSHAs[taskIDStr] = headSHA
          if pr.UpdatedAt > maxUpdatedAt: maxUpdatedAt = pr.UpdatedAt
          log debug "published UpdateFrontmatterCommand pr=%s/%s#%d taskID=%s"

      - If exists && knownSHA == headSHA:
          log debug "no change, skipping"
          if pr.UpdatedAt > maxUpdatedAt: maxUpdatedAt = pr.UpdatedAt   // safe to advance — nothing failed

   e. Advance cursor: cursorState.LastUpdatedAt = maxUpdatedAt (only if > since, else leave unchanged). NOTE: maxUpdatedAt advances ONLY for PRs that either succeeded or were intentionally skipped — failed publishes leave their PR's UpdatedAt OUT of the maxUpdatedAt calculation, so the next poll re-emits.

   f. Persist cursor: cursor.Save(ctx, cursorPath, cursorState).
      On error: log glog.Errorf("failed to save cursor err=%v", err) — do NOT return error (the poll succeeded; cursor loss just means potential re-publish on next boot, which the controller handles idempotently).
   ```

   **Helper functions** (unexported, in same file):
   - `buildTaskBody(pr githubclient.PullRequest) string` — returns markdown string:
     ```
     # PR Review: <title>

     <HTMLURL>
     ```
   - `buildFrontmatter(pr githubclient.PullRequest, taskIDStr, stage string) agentlib.TaskFrontmatter` — returns:
     ```go
     agentlib.TaskFrontmatter{
         "assignee":        "pr-reviewer-agent",
         "phase":           "planning",
         "status":          "in_progress",
         "stage":           stage,
         "task_identifier": taskIDStr,
         "title":           pr.Title,
     }
     ```
     (Note: `agentlib.TaskFrontmatter` is `map[string]interface{}`, not `map[string]string`.)

6. **Create `watcher/github/pkg/watcher/watcher_test.go`** (external package `watcher_test`, Ginkgo/Gomega):

   Use:
   - `FakeGitHubClient` from `pkg/githubclient/mocks/`
   - `FakeCommandPublisher` from `pkg/publisher/mocks/`
   - A temp dir for `cursorPath`

   Test cases:
   - **No PRs returned** → `Poll` returns nil, cursor `LastUpdatedAt` advances to `since` (unchanged if no results), cursor saved.
   - **New PR (no existing cursor entry)** → `PublishCreate` called once with correct `taskIDStr`, `HeadSHAs[taskIDStr]` set to PR's `HeadSHA`.
   - **Existing PR, same SHA** → neither `PublishCreate` nor `PublishUpdateFrontmatter` called.
   - **Force-push (existing entry, different SHA)** → `PublishUpdateFrontmatter` called once; body section contains `"## Outdated by force-push <oldSHA>"`; `HeadSHAs[taskIDStr]` updated to new SHA.
   - **Draft PR** → skipped, no publish calls.
   - **Bot-authored PR** → skipped, no publish calls.
   - **GitHub error** → `Poll` returns nil (non-crash), cursor unchanged, no publish calls.
   - **Kafka publish fails (CreateTaskCommand)** → `Poll` returns nil, cursor NOT updated for that PR.
   - **Rate limit low (< rateSafeThreshold)** → `Poll` aborts early, no publishes for remaining pages (use short sleep or mock `time.Until` — acceptable to use a context with very short timeout or mock RateResetAt in the past to avoid real sleeping in tests).
   - **Context cancellation between pages** → `Poll` returns nil cleanly.
   - **Cursor file missing on first poll** → cold-start, correct `since` used in `SearchPRs` call.
   - **Cursor save fails** → `Poll` returns nil (non-crash, error is logged only).

   For the rate-limit test: set `RateResetAt` to `time.Now().Add(-1 * time.Second)` (already past) so `time.After(time.Until(RateResetAt) + 5s)` fires immediately (≈ 5 seconds). To avoid slow tests, pass a pre-cancelled context after the first SearchPRs call returns or use a very short `RateResetAt` in the past. Document the approach in a test comment.

7. **Create `watcher/github/pkg/factory/factory.go`** — pure composition, zero business logic:

   ```go
   package factory

   import (
       "time"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/config"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/githubclient"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/publisher"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/watcher"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/cursor"
   )

   // CreateWatcher wires all dependencies and returns a ready-to-use Watcher.
   func CreateWatcher(cfg config.Config, startTime time.Time) (watcher.Watcher, error) {
       ghClient := githubclient.NewGitHubClient(cfg.GHToken)
       pub, err := publisher.NewKafkaPublisher(cfg.KafkaBrokers, <kafka topic>)
       if err != nil {
           return nil, errors.Wrap(ctx, err, "create kafka publisher")
       }
       return watcher.NewWatcher(
           ghClient,
           pub,
           cursor.DefaultCursorPath,
           startTime,
           cfg.RepoScope,
           cfg.BotAllowlist,
           cfg.Stage,
           10, // rateSafeThreshold
       ), nil
   }
   ```

   **Kafka topic name:** Grep `bborbe/agent/lib` for a topic constant:
   ```bash
   grep -rn "TaskTopic\|task.*topic\|const.*Topic" \
     $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/... 2>/dev/null | head -10
   ```
   Use the discovered constant. If no constant exists, use `"task"` as the default and add a TODO.

   **Factory context:** `CreateWatcher` takes no `ctx` — it's pure construction. If `publisher.NewKafkaPublisher` requires a context for dial, use `context.Background()` only inside this factory function (construction-time only).

8. **Update `watcher/github/main.go`** to replace the stub loop:

   ```go
   func run(ctx context.Context) error {
       cfg, err := config.Load()
       if err != nil {
           return errors.Wrap(ctx, err, "load config")
       }
       glog.V(2).Infof("github-pr-watcher starting stage=%s scope=%s interval=%s", cfg.Stage, cfg.RepoScope, cfg.PollInterval)

       startTime := time.Now().UTC()
       w, err := factory.CreateWatcher(cfg, startTime)
       if err != nil {
           return errors.Wrap(ctx, err, "create watcher")
       }

       ticker := time.NewTicker(cfg.PollInterval)
       defer ticker.Stop()
       for {
           select {
           case <-ctx.Done():
               glog.V(2).Infof("context cancelled, exiting cleanly")
               return nil
           case <-ticker.C:
               glog.V(2).Infof("poll cycle start stage=%s", cfg.Stage)
               if err := w.Poll(ctx); err != nil {
                   glog.Errorf("poll cycle error: %v", err)
                   // non-crash: continue to next tick
               }
           }
       }
   }
   ```

9. **Run `cd watcher/github && make test`** — must pass.

10. **Check coverage** for `cursor`, `watcher`, `publisher` packages (≥80% each):
    ```bash
    cd watcher/github && go test -coverprofile=/tmp/cover.out \
      ./pkg/cursor/... ./pkg/watcher/... ./pkg/publisher/... \
      && go tool cover -func=/tmp/cover.out
    ```

11. **Run `cd watcher/github && make precommit`** — must exit 0.
</requirements>

<constraints>
- Poll cycle errors (GitHub unreachable, Kafka failure) must NOT crash the process — `Poll` returns nil and logs; only the outer `run` function can return a non-nil error (which kills the pod)
- Stale-task body marker MUST be exactly `## Outdated by force-push <oldSHA>` (no other format) — future tooling greps for this
- `CreateTaskCommand` field names must match what `bborbe/agent/lib` exports — grep-verify before writing struct literals
- Cursor save failure must NOT propagate as an error from `Poll` — log only; the controller's idempotent `task_identifier` dedup handles a double-publish gracefully
- `context.Background()` forbidden in `watcher.go` or `cursor.go` — use injected `ctx` everywhere
- `context.Background()` acceptable ONLY in `factory.go` at construction time if a library dial requires a context
- Rate-limit sleep uses `time.After(time.Until(reset) + 5s)` in a `select` with `ctx.Done()` — never a bare `time.Sleep`
- Kafka topic must be grep-verified from `bborbe/agent/lib`; do not invent
- Counterfeiter mocks generated via `go generate`, not hand-written
- Do NOT run `go mod vendor`
- Do NOT commit — dark-factory handles git
- All existing tests must still pass; `make precommit` must exit 0
</constraints>

<verification>
```bash
# Preconditions
ls watcher/github/pkg/githubclient/client.go watcher/github/pkg/filter/filter.go watcher/github/pkg/taskid/taskid.go

# Cursor package
grep -n "func Load\|func Save\|type State struct" watcher/github/pkg/cursor/cursor.go

# Watcher interface
grep -n "type Watcher interface\|func NewWatcher\|func.*Poll" watcher/github/pkg/watcher/watcher.go

# Outdated marker format (must be exact)
grep -n "Outdated by force-push" watcher/github/pkg/watcher/watcher.go
# Expected: exactly one match with format "## Outdated by force-push %s"

# Frontmatter keys match spec
grep -n "pr-reviewer-agent\|planning\|in_progress\|task_identifier" watcher/github/pkg/watcher/watcher.go

# Rate-limit select pattern (not bare time.Sleep)
grep -n "time.Sleep" watcher/github/pkg/watcher/watcher.go
# Expected: no matches (time.After in select is correct; bare Sleep is not)

# Factory wires all deps
grep -n "func CreateWatcher\|NewGitHubClient\|NewKafkaPublisher\|NewWatcher" watcher/github/pkg/factory/factory.go

# main.go uses factory (stub loop removed)
grep -n "factory.CreateWatcher\|w.Poll" watcher/github/main.go

# Mocks generated
ls watcher/github/pkg/publisher/mocks/command_publisher.go

# Tests pass
cd watcher/github && make test

# Coverage ≥80%
cd watcher/github && go test -coverprofile=/tmp/cover.out ./pkg/cursor/... ./pkg/watcher/... && go tool cover -func=/tmp/cover.out

# Full precommit
cd watcher/github && make precommit
```
</verification>
