---
status: draft
spec: [036-watcher-pr-rename-trigger-add-single-pr-trigger]
created: "2026-05-23T19:45:01Z"
branch: dark-factory/watcher-pr-rename-trigger-add-single-pr-trigger
---

<summary>
- New `pkg/handler/single_trigger_handler.go` implements `http.Handler` for `POST /trigger?url=<pr_url>`
- Handler parses the `url` query param via `pkg.ParsePRURL`, returns HTTP 400 if missing
- On valid URL, fetches PR details via GitHubClient, runs the `TaskCreationFilter` chain
- HTTP 422 on filter rejection (filter name + reason in JSON body)
- HTTP 200 on success: JSON `{"task_id":"<uuid>","kafka_offset":<int>,"repo":"owner/repo","pr_number":<int>,"head_sha":"<sha>"}`
- HTTP 502 on Kafka publish failure
- Filter rejection includes filter name in `filter` field of error JSON
</summary>

<objective>
Implement the single-PR trigger HTTP handler that parses a PR URL, runs the filter chain, fetches PR details, and publishes a CreateTaskCommand with `force_bypass_dedup=true`.
</objective>

<context>
Read CLAUDE.md for project conventions.

**Read fully before implementing:**
- `watcher/github-pr/pkg/filter/filter.go` — TaskCreationFilter interface (`Skip(PR) bool`), `TaskCreationFilters` slice composite
- `watcher/github-pr/pkg/filter/age_filter.go` and `watcher/github-pr/pkg/filter/draft_filter.go` — to understand how to build a `filter.PR` struct from fetched PR details
- `watcher/github-pr/pkg/githubclient.go` — `GitHubClient` interface (`SearchPRs`, `GetPRDetails`)
- `watcher/github-pr/pkg/watcher.go` — `processPRs` logic for how PRs are built into `task.CreateCommand` (trusted path), and `buildFrontmatter`, `buildTaskBody`, `computePRTitle`
- `watcher/github-pr/pkg/filename.go` — `computePRFilenameHint` (used as `Title`)
- `watcher/github-pr/pkg/taskid.go` — `DeriveTaskID`
- `agent/pr-reviewer/pkg/prurl.go` — `ParsePRURL` (imported as `pkg.ParsePRURL` in agent/pr-reviewer, but the lib/prurl package in this repo — verify the correct import path)
- `watcher/github-pr/pkg/factory/factory.go` — understand how `createSender task.CreateCommandSender` is created

**Important discovery:** `ParsePRURL` lives at `github.com/bborbe/maintainer/agent/pr-reviewer/pkg/prurl.go` (in the `pkg` package). The watcher is a separate module. You need to either:
a) Copy or move the `prurl` type/function to a shared location (e.g. `lib/prurl/`)
b) Import from `agent/pr-reviewer/pkg` (same repo, different module — check if this is clean)
c) Implement a lightweight GitHub-only parser inline in the handler

The cleanest approach for the watcher is to implement a minimal GitHub-only PR URL parser in the handler package (mirror the pattern from `agent/pr-reviewer/pkg/prurl.go` but scoped to GitHub). Do NOT import the agent/pr-reviewer module — it has different dependencies that may conflict.

**Handler pattern:** Follow the existing pattern in the codebase. `watcher/github-pr/main.go` uses `libhttp.NewBackgroundRunHandler` for background tasks. For a synchronous request handler, use a plain `http.HandlerFunc` or `http.Handler` registered via `router.Path("/trigger").Handler(...)`.

**JSON response pattern:** Use `json.NewEncoder(w).Encode(...)` for responses. Error responses: `{"error": "...", "filter": "...", "pr_url": "..."}`.
</context>

<requirements>

1. **Create `watcher/github-pr/pkg/handler/single_trigger_handler.go`**

   This file implements the single-PR trigger handler.

   The handler receives a `url` query param, validates it, fetches the PR from GitHub, runs the filter chain, and publishes a `CreateTaskCommand` with `force_bypass_dedup=true`.

   **Constructor signature:**
   ```go
   func NewSingleTriggerHandler(
       ghClient GitHubClient,
       createSender task.CreateCommandSender,
       taskCreationFilter filter.TaskCreationFilter,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
   ) http.Handler
   ```

   **Handler behavior:**

   a. **Parse URL** — read `r.URL.Query().Get("url")`. If empty, return HTTP 400 with `{"error": "url query parameter required"}`.

   b. **Parse GitHub PR URL** — implement a minimal GitHub-only parser inline in the handler (do NOT import agent/pr-reviewer module). Support format: `https://github.com/{owner}/{repo}/pull/{number}`. Return HTTP 400 with `{"error": "unsupported URL format: <url>"}` on parse failure.

   c. **Fetch PR details** — call `ghClient.GetPRDetails(ctx, owner, repo, number)`. On error, return HTTP 502 with `{"error": "fetch PR details failed: <err>"}`. (Note: the existing `GitHubClient.GetPRDetails` does NOT accept a PRInfo object — it takes `owner, repo string, number int` directly.)

   d. **Build filter.PR** — construct a `filter.PR` from the PR details:
      ```go
      pr := filter.PR{
          AuthorLogin: /* from GitHub user login — not in PRDetails, use a placeholder or fetch via SearchPRs */,
          IsDraft:     /* from PR details — not in PRDetails, use false as fallback */,
          Title:       /* from PR details — not in PRDetails, use number as fallback */,
          UpdatedAt:   libtime.NewCurrentDateTime().Now(), // Use current time; PR is fresh if triggered manually
          RepoKey:     "github.com/" + owner + "/" + repo,
      }
      ```
      Note: `PRDetails` (from `GetPRDetails`) only contains `HeadSHA`, `CloneURL`, `BaseRef`. It does NOT contain `AuthorLogin`, `IsDraft`, `Title`, `UpdatedAt`. The simplest path for the single-trigger handler is to fetch these from the search API OR use the `GetPRDetails` response plus a separate search call. Evaluate the trade-off:
      - Option A: call `SearchPRs` first to get basic PR info, then `GetPRDetails` for the SHA
      - Option B: fetch details then use `ghClient` to get the author/title/etc from the PR itself
      - Option C: only filter on `RepoKey` and `UpdatedAt` (current time), accept that draft/bot/age filtering is approximate for manually-triggered PRs

      Pick one approach and document it. The spec says "runs the existing TaskCreationFilter chain". The most correct approach is Option A: call `SearchPRs` with a narrow query to get the PR's basic fields, then `GetPRDetails` for the SHA. Implement Option A.

   e. **Run filter chain** — call `taskCreationFilter.Skip(pr)`. If true, determine which filter rejected it. Return HTTP 422 with `{"error": "PR filtered", "filter": "<filter name>", "pr_url": "<url>"}`.

      Note: `TaskCreationFilters.Skip` returns `true` for skip but does NOT indicate WHICH filter rejected. To get the filter name, iterate manually:
      ```go
      var rejectedBy string
      for _, f := range taskCreationFilter.(TaskCreationFilters) {
          if f.Skip(pr) {
              rejectedBy = filterName(f) // implement filterName() to return a human-readable name
              break
          }
      }
      ```
      If `taskCreationFilter` is not a `TaskCreationFilters` slice, fall back to generic `"filter"`.

   f. **Build CreateTaskCommand** — construct the command with the same logic as `watcher.go:publishCreate` (trusted path):

      ```go
      taskID := DeriveTaskID(owner, repo, number, details.HeadSHA)
      taskIDStr := taskID.String()
      cmd := task.CreateCommand{
          Title:          computePRFilenameHint("github", owner, repo, number, title), // title from SearchPRs
          TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
          Frontmatter:    buildFrontmatterFromDetails(owner, repo, number, taskIDStr, stage, details),
          Body:           buildTaskBodyFromPR(owner, repo, number, title, htmlURL),
      }
      ```

      **Key question for implementer:** The existing `buildFrontmatter` in `watcher.go` takes a `PullRequest` struct (which has all fields). The `PRDetails` only has `HeadSHA`, `CloneURL`, `BaseRef`. Implement a `buildFrontmatterFromDetails` variant in the handler that works with `owner, repo, number, taskIDStr, stage, PRDetails`. Mirror the existing logic.

   g. **Publish to Kafka** — call `createSender.SendCommand(ctx, cmd)`. On error, return HTTP 502 with `{"error": "kafka publish failed: <err>"}`.

   h. **Success response** — HTTP 200 with JSON:
      ```json
      {
        "task_id": "<uuid>",
        "kafka_offset": <int-or-null>,
        "repo": "<owner/repo>",
        "pr_number": <int>,
        "head_sha": "<sha>"
      }
      ```
      Note: `SendCommand` may not return a Kafka offset. If it returns an offset, include it. If not, return `0` or null.

2. **Create `watcher/github-pr/pkg/handler/single_trigger_handler_test.go`**

   Table-driven unit tests covering all failure modes plus happy path. Use counterfeiter mocks.

   Test cases:
   - Missing `url` param → HTTP 400
   - Invalid URL (not github.com PR URL) → HTTP 400
   - Valid GitHub PR URL but fetch details fails → HTTP 502
   - Valid URL but PR is filtered (e.g. draft) → HTTP 422 with filter name
   - Valid URL, PR passes filter, Kafka publish succeeds → HTTP 200 with task_id, repo, pr_number, head_sha

   Mock dependencies:
   - `*mocks.GitHubClient` — mock `GetPRDetails` and `SearchPRs`
   - `*taskmocks.CreateCommandSender` — mock `SendCommand`

3. **Add handler package to factory or wire in main.go**

   The handler needs `GitHubClient` and `task.CreateCommandSender`. These are created in `factory.CreateWatcher`. You can either:
   a) Create a new factory function for the handler in `pkg/factory/handler_factory.go`
   b) Construct the handler in `main.go`'s `runHTTPServer` using the same senders

   The simpler approach (less code): construct the handler in `main.go`'s `runHTTPServer` after creating the watcher. Pass `w` (the Watcher) which contains the GitHubClient, plus the createSender.

   Actually, looking at `factory.CreateWatcher`, it creates both the `createSender` and `ghClient` internally. They are not exported. The cleanest solution is to create a new `CreateSingleTriggerHandler` factory function in `pkg/factory/handler_factory.go`.

   ```go
   func CreateSingleTriggerHandler(
       ctx context.Context,
       ghToken string,
       brokers libkafka.Brokers,
       branch base.Branch,
       taskCreationFilter filter.TaskCreationFilter,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
   ) (http.Handler, func(), error)
   ```

   This mirrors `CreateKafkaSender` + creates the handler. But it re-creates the Kafka sender for the handler...

   Actually, the existing `CreateWatcher` in factory already creates a `createSender`. We should reuse it. The factory should return the `createSender` as well so `runHTTPServer` can pass it to the handler.

   **Decision:** The spec says to make the minimal diff. The cleanest minimal approach is to modify `CreateWatcher` in `pkg/factory/factory.go` to also return the `createSender`. Then `runHTTPServer` in `main.go` gets the sender and passes it to the handler.

   Implement step 4 in `1-spec-036-register-endpoints.md`, then update here to reference the returned sender.

4. **Regenerate mocks if needed:**
   ```bash
   cd watcher/github-pr && make generate
   ```

5. **Run tests:**
   ```bash
   cd watcher/github-pr && make test
   ```

</requirements>

<constraints>
- Create files: `pkg/handler/single_trigger_handler.go`, `pkg/handler/single_trigger_handler_test.go`
- BSD license header on every file
- Do NOT commit — dark-factory handles git
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`
- Logging: `github.com/golang/glog` (V(2)=heartbeat, V(3)=per-item)
- PR URL parser is GitHub-only inline implementation (do NOT import agent/pr-reviewer module)
- The filter chain is the existing `TaskCreationFilters` — reuse, do not reimplement
- `ParsePRURL` logic from `agent/pr-reviewer/pkg/prurl.go` is NOT available in the watcher module — implement GitHub-only parsing inline
- If `GitHubClient.GetPRDetails` does not return `AuthorLogin`, `IsDraft`, `Title`, `UpdatedAt`, fetch them via `SearchPRs` first
- HTTP responses: JSON encoded with `json.NewEncoder`, Content-Type: application/json
- HTTP 400: missing or unparseable URL
- HTTP 422: filter rejection (include filter name)
- HTTP 502: Kafka publish failure
- HTTP 200: success with task_id, repo, pr_number, head_sha
- Coverage ≥80% for `pkg/handler/` package
</constraints>

<verification>
cd watcher/github-pr && make generate
cd watcher/github-pr && make test
# Expected: all tests pass, coverage ≥80% on pkg/handler/

# Confirm new handler file exists:
ls watcher/github-pr/pkg/handler/single_trigger_handler.go
# Expected: file exists

# Confirm tests cover all failure modes:
grep -n "HTTP 400\|HTTP 422\|HTTP 502\|HTTP 200" watcher/github-pr/pkg/handler/single_trigger_handler_test.go
# Expected: all four status codes covered
</verification>