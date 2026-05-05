---
status: committing
spec: [015-github-build-watcher-mvp]
summary: Implemented core state machine (cursor, filter, metrics, watcher) for watcher/github-build with all state transitions, Kafka failure idempotency, rate-limit early exit, and ≥80% test coverage
container: maintainer-087-spec-015-watcher
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T21:00:00Z"
queued: "2026-05-05T21:18:21Z"
started: "2026-05-05T21:30:15Z"
---

<summary>
- `pkg/cursor.go` persists per-repo build state (`last_known_state`, `current_episode_sha`, `default_branch`) to `/data/cursor.json` using atomic write (`.tmp` + rename); missing file → fresh green start; corrupt file → refuse to start
- `pkg/filter/` adds a `RepoFilter` interface with `RepoFilters` OR-composite and `RepoAllowlistFilter` (skip repos not in the allowlist)
- `pkg/metrics.go` registers six Prometheus metrics: `github_build_watcher_poll_cycles_total`, `..._repos_checked_total`, `..._state_transitions_total{transition}`, `..._tasks_published_total`, `..._poll_errors_total{reason}`, `..._current_red_repos` (gauge)
- `pkg/watcher.go` implements the `Watcher` interface with `Poll(ctx)` executing the full per-repo state machine: green→red publishes task; red→red (same/different SHA) skips; red→green clears state; undefined (zero runs) skips
- All failure modes from the spec are handled: rate limit increments error metric and skips remaining repos; API 5xx/404 skips repo and continues; Kafka failure does NOT update cursor; cursor write failure logs and continues
- State machine tests cover all 6 transitions from the spec's worked example table, including the multi-commit layering scenario
- Counterfeiter mocks generated for `Watcher`, `Metrics`, `RepoFilter` in `pkg/mocks/`
- `make precommit` passes in `watcher/github-build/`
</summary>

<objective>
Implement the core state machine, cursor persistence, filter chain, and Prometheus metrics for the build watcher. This is the heart of the service — the `Poll()` method that converts GitHub Actions API state into `CreateTaskCommand` Kafka messages, idempotently, with correct episode-SHA tracking.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-prometheus-metrics-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-context-cancellation-in-loops.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-filter-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/`
Read `go-concurrency-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/`

Files to read before making any changes:
- `watcher/github-pr/pkg/cursor.go` — full file; mirror atomic-write pattern and file path `/data/cursor.json`
- `watcher/github-pr/pkg/watcher.go` — full file; mirror Poll() lifecycle, filter chain application, metrics emission
- `watcher/github-pr/pkg/metrics.go` — full file; mirror Prometheus interface pattern, init() pre-population
- `watcher/github-pr/pkg/filter/filter.go` — full file; mirror TaskCreationFilter interface + composite
- `watcher/github-pr/pkg/filter/repo_allowlist_filter.go` — full file; mirror allowlist filter pattern
- `watcher/github-build/pkg/githubclient.go` — understand WorkflowRun fields available
- `watcher/github-build/pkg/taskid.go` — understand DeriveTaskID returns `uuid.UUID`
- `watcher/github-build/pkg/publisher.go` — `CommandPublisher.PublishCreate(ctx, cmd agentlib.CreateTaskCommand) error`. The watcher constructs the `agentlib.CreateTaskCommand` (TaskIdentifier + Frontmatter + Body) and passes it in. The publisher is domain-agnostic.

**State machine rules (lock these before writing any code):**

| prev state | curr state | episode SHA change | Action |
|---|---|---|---|
| `""` (cold) | `green` | n/a | nothing |
| `""` (cold) | `red` | n/a | publish, set `red` + `episodeSHA` |
| `""` (cold) | undefined | n/a | skip |
| `green` | `green` | n/a | nothing |
| `green` | `red` | n/a | publish, set `red` + `episodeSHA` |
| `green` | undefined | n/a | skip |
| `red` | `red` | same | skip (idempotent) |
| `red` | `red` | different | skip (keep existing episode SHA — episode locked on first red) |
| `red` | `green` | n/a | clear `episodeSHA`, set `green` — NO publish |
| `red` | undefined | n/a | skip |
| any | API error | n/a | skip repo, increment `poll_errors_total`, do NOT update cursor |

**Red/green derivation rules:**
1. Fetch `GET /repos/{owner}/{repo}/actions/runs?branch={default}&per_page=20&status=completed`
2. Group runs by `WorkflowID`; keep only the latest run per workflow (by `CreatedAt` desc or `RunNumber` desc)
3. Filter: only consider runs with `Conclusion` in `{"failure", "success"}` — skip `cancelled`, `timed_out`, `action_required`, `skipped`, `neutral`, `stale`, and empty
4. `red` = any deduplicated run has `Conclusion == "failure"`
5. `green` = all deduplicated runs have `Conclusion == "success"` (zero runs after filtering → undefined/skip)
6. Episode SHA = the `HeadSHA` of the *earliest* (smallest `CreatedAt`) run among the failing runs

**Kafka failure invariant:** if `PublishCreate` returns an error, do NOT save the cursor for that repo. The next poll cycle will retry the publish (re-derive state, re-publish). The controller dedups by task_id so a re-publish is safe.
</context>

<requirements>
**Execute steps in order. Run `make precommit` only at the final step.**

1. **Create `watcher/github-build/pkg/cursor.go`**:

   Read `watcher/github-pr/pkg/cursor.go` fully. The build watcher cursor differs in its per-repo state shape:

   ```go
   // RepoState holds the persisted build state for one repository.
   type RepoState struct {
       LastKnownState    string `json:"last_known_state"`    // "green" | "red" | ""
       CurrentEpisodeSHA string `json:"current_episode_sha"` // empty when green
       DefaultBranch     string `json:"default_branch"`      // cached; fetched via API if empty
   }

   // Cursor is the full persisted state for the build watcher.
   type Cursor struct {
       Repos map[string]*RepoState `json:"repos"` // key: "owner/repo"
   }
   ```

   Functions:
   - `LoadCursor(ctx context.Context) (*Cursor, error)`:
     - If `/data/cursor.json` does not exist (`os.ErrNotExist`): return empty cursor (all repos start as green — cold start)
     - If file exists but cannot be read or unmarshalled: return error (caller refuses to start)
   - `SaveCursor(ctx context.Context, c *Cursor) error`:
     - Atomic write: write to `/data/cursor.json.tmp`, then `os.Rename` to `/data/cursor.json`
     - On error: log at `glog.Warningf` + return error (caller logs and continues — in-memory state preserved)
   - `GetOrCreateRepoState(c *Cursor, key string) *RepoState`:
     - Returns existing state or inserts a new zero-value `RepoState` (zero value = `LastKnownState: ""` which is treated as `green`)

2. **Create `watcher/github-build/pkg/cursor_test.go`** with Ginkgo v2 + Gomega tests. Cover:
   - `LoadCursor` on non-existent file returns empty cursor (not an error)
   - `LoadCursor` on corrupt JSON returns an error
   - `SaveCursor` + `LoadCursor` round-trips the state correctly
   - `GetOrCreateRepoState` inserts a zero-value entry for new repos
   - File path is `/data/cursor.json` (test with a temp dir override — inject the path as a parameter or use `t.TempDir()`)

   **Note:** Pass the cursor file path as a parameter to `LoadCursor`/`SaveCursor` (e.g. `cursorPath string`) so tests can use a temp directory. The production caller passes `/data/cursor.json`.

3. **Create `watcher/github-build/pkg/filter/` package**:

   Create `watcher/github-build/pkg/filter/filter.go`:
   ```go
   // RepoFilter decides whether to skip a repo in a poll cycle.
   // Skip returns true if the repo should be excluded.
   //
   //counterfeiter:generate -o mocks/repo_filter.go --fake-name RepoFilter . RepoFilter
   type RepoFilter interface {
       Skip(repoKey string) bool // repoKey = "owner/repo"
   }

   // RepoFilters is an OR-composite: skip if ANY filter votes to skip.
   type RepoFilters []RepoFilter

   func (filters RepoFilters) Skip(repoKey string) bool {
       for _, f := range filters {
           if f.Skip(repoKey) {
               return true
           }
       }
       return false
   }
   ```

   Create `watcher/github-build/pkg/filter/repo_allowlist_filter.go`:
   - `RepoAllowlistFilter` — if allowlist is non-empty, skip any repo whose key is NOT in the list
   - Empty allowlist: never skips (allow-all, but the build watcher's REPO_ALLOWLIST is required so this path only applies if the list is empty-after-parse, which startup validation blocks)

   Create `watcher/github-build/pkg/filter/suite_test.go` and `filter_test.go` covering:
   - Empty allowlist → never skips
   - Non-empty allowlist → skips repos not on list, passes repos on list
   - OR-composite skips if any filter votes skip

4. **Create `watcher/github-build/pkg/metrics.go`**:

   Read `watcher/github-pr/pkg/metrics.go` fully. Mirror the interface + Prometheus registration pattern. The build watcher's metrics:

   ```go
   // Metrics defines Prometheus counters and gauges for the build watcher.
   //
   //counterfeiter:generate -o mocks/metrics.go --fake-name Metrics . Metrics
   type Metrics interface {
       IncPollCycle(result string)                   // result: "success" | "error"
       IncReposChecked()
       IncStateTransition(transition string)         // transition: "green_to_red" | "red_to_green"
       IncTaskPublished()
       IncPollError(reason string)                   // reason: "rate_limited" | "github_error" | "kafka_error"
       SetCurrentRedRepos(count float64)
   }
   ```

   Register these Prometheus metrics in `init()` (or via `NewMetrics()` constructor matching the PR watcher pattern):
   - `github_build_watcher_poll_cycles_total{result}` — counter
   - `github_build_watcher_repos_checked_total` — counter
   - `github_build_watcher_state_transitions_total{transition}` — counter
   - `github_build_watcher_tasks_published_total` — counter
   - `github_build_watcher_poll_errors_total{reason}` — counter
   - `github_build_watcher_current_red_repos` — gauge

   Pre-initialize all label combinations to 0 in init (avoids missing-series alerts). Mirror the PR watcher's init pattern exactly.

5. **Create `watcher/github-build/pkg/watcher.go`**:

   Read `watcher/github-pr/pkg/watcher.go` fully. Mirror the overall `Watcher` interface + `NewWatcher()` constructor + `Poll()` structure.

   ```go
   // Watcher polls GitHub Actions for build status changes.
   //
   //counterfeiter:generate -o mocks/watcher.go --fake-name Watcher . Watcher
   type Watcher interface {
       Poll(ctx context.Context) error
   }
   ```

   `Poll(ctx)` lifecycle — execute for each repo key in the allowlist:

   ```
   for _, repoKey := range allowlist:
       // non-blocking context check (per go-context-cancellation-in-loops.md):
       select {
       case <-ctx.Done():
           return ctx.Err()
       default:
       }

       owner, repo = splitRepoKey(repoKey)
       repoState = cursor.GetOrCreateRepoState(repoKey)

       // 1. Ensure default branch is known
       if repoState.DefaultBranch == "" {
           branch, err = githubClient.GetDefaultBranch(ctx, owner, repo)
           if err == 404 → log warning, increment poll_errors_total{reason="github_error"}, skip
           if err → same
           repoState.DefaultBranch = branch
       }

       // 2. Fetch workflow runs
       runs, err = githubClient.GetWorkflowRuns(ctx, owner, repo, repoState.DefaultBranch)
       if err == ErrRateLimited → increment poll_errors_total{reason="rate_limited"}, break outer loop (stop polling remaining repos)
       if err → log, increment poll_errors_total{reason="github_error"}, continue (skip this repo)
       metrics.IncReposChecked()

       // 3. Derive current state
       currState, episodeSHA = deriveState(runs)
       // currState: "green" | "red" | "undefined"
       if currState == "undefined" → continue (skip this repo, no cursor update)

       // 4. Apply state machine
       prevState = repoState.LastKnownState // "" treated as "green"
       switch:
       case (prevState == "" || prevState == "green") && currState == "red":
           taskID = DeriveTaskID(owner, repo, episodeSHA)  // uuid.UUID
           cmd = buildCreateTaskCommand(taskID, owner, repo, episodeSHA, failingRuns)
           err = publisher.PublishCreate(ctx, cmd)
           if err → log, increment poll_errors_total{reason="kafka_error"}, continue (skip cursor update for this repo)
           metrics.IncTaskPublished()
           metrics.IncStateTransition("green_to_red")
           repoState.LastKnownState = "red"
           repoState.CurrentEpisodeSHA = episodeSHA

       case prevState == "red" && currState == "red":
           // same or different SHA → always skip (episode locked on first red)
           // do nothing, no cursor update

       case prevState == "red" && currState == "green":
           metrics.IncStateTransition("red_to_green")
           repoState.LastKnownState = "green"
           repoState.CurrentEpisodeSHA = ""

       case (prevState == "" || prevState == "green") && currState == "green":
           // nothing

   // 5. Count current red repos and emit gauge
   redCount = count repos where repoState.LastKnownState == "red"
   metrics.SetCurrentRedRepos(float64(redCount))

   // 6. Save cursor (best-effort; log on failure)
   if err = saveCursor(ctx, cursor); err != nil {
       glog.Warningf("cursor save failed: %v", err)
   }

   metrics.IncPollCycle("success")
   return nil
   ```

   **`deriveState(runs []WorkflowRun) (state string, episodeSHA string)`**:
   - Group by `WorkflowID`, keep latest run per workflow (by `CreatedAt` desc)
   - Filter: only `Conclusion == "failure"` or `Conclusion == "success"` (skip others)
   - After filtering: if zero runs → return "undefined", ""
   - If any run has `Conclusion == "failure"` → state = "red"
     - Episode SHA = `HeadSHA` of the failing run with the *smallest* `CreatedAt` (earliest failure)
   - Else → state = "green", episodeSHA = ""

   **`splitRepoKey(key string) (owner, repo string)`**: splits `"owner/repo"` on `/`.

   **`buildCreateTaskCommand(taskID uuid.UUID, owner, repo, episodeSHA string, failingRuns []WorkflowRun) agentlib.CreateTaskCommand`** constructs the command:
   - `TaskIdentifier`: `agentlib.TaskIdentifier(taskID.String())`
   - `Frontmatter`: map containing at minimum `assignee: "build-fixer-agent"`, `repo: "<owner>/<repo>"`, `episode_sha: "<sha>"`, `status: "todo"`
   - `Body`: markdown — title `# Build Failure: <owner>/<repo>`, the episode SHA, and a list of failing workflows with `- [<name>](<html-url>)` lines
   - Verify the exact `agentlib.CreateTaskCommand` field names by re-reading agent/lib before coding (already grep-verified in prompt 2)

   **Constructor signature:**
   ```go
   func NewWatcher(
       githubClient GitHubClient,
       publisher    CommandPublisher,
       metrics      Metrics,
       filter       RepoFilter,
       allowlist    []string, // repo keys: "owner/repo"
       cursorPath   string,   // "/data/cursor.json"
   ) Watcher
   ```

6. **Create `watcher/github-build/pkg/watcher_test.go`** with Ginkgo v2 + Gomega tests.

   Use counterfeiter mocks for `GitHubClient`, `CommandPublisher`, `Metrics`. Use a `t.TempDir()` for cursor persistence.

   **Required test cases (from spec's state machine table):**

   - `green → red`: `PublishCreate` called once with correct task_id; cursor updated to `red` + episodeSHA
   - `red → red` (same SHA): `PublishCreate` NOT called; cursor episode SHA unchanged
   - `red → red` (different SHA from next layered commit): `PublishCreate` NOT called; episode SHA stays as the original first-red SHA
   - `red → green`: `PublishCreate` NOT called; cursor cleared to `green`, episodeSHA = ""
   - `green → green`: `PublishCreate` NOT called
   - Cold start (empty cursor) + repo currently red: `PublishCreate` called (treat as green→red)
   - Kafka failure: `PublishCreate` returns error → cursor NOT updated (idempotency: next poll retries)
   - GitHub API error for one repo: that repo skipped, polling continues for remaining repos
   - Rate-limit error: poll loop terminates early for remaining repos; `IncPollError("rate_limited")` called
   - Zero runs (undefined state): repo skipped; `PublishCreate` NOT called; cursor unchanged
   - Corrupt cursor on first `Poll`: `Poll(ctx)` returns the load error and never calls `PublishCreate` (cursor is loaded inside `Poll`, not in the constructor — mirrors PR watcher)

   **Worked example verification (from spec Desired Behavior #6):**
   ```
   t0: repo green, cursor empty → Poll → no publish
   t1: commit A breaks build → Poll → publish task UUID5("owner/repo#build-A"); cursor: state=red, SHA=A
   t2: commit B layered, still red (new SHA B) → Poll → no publish (same episode, SHA stays A)
   t3: both fixed → Poll → no publish, cursor: state=green, SHA=""
   t4: commit C breaks build → Poll → publish task UUID5("owner/repo#build-C"); distinct from t1
   ```
   Assert that task ID at t1 ≠ task ID at t4 (different episode SHAs).

7. **Generate counterfeiter mocks for new interfaces** — prefer `make generate` if PR watcher's Makefile has the target; otherwise:
   ```bash
   cd watcher/github-build/pkg && go generate ./...
   ```
   Mocks land at:
   - `watcher/github-build/pkg/mocks/watcher.go`
   - `watcher/github-build/pkg/mocks/metrics.go`
   - `watcher/github-build/pkg/filter/mocks/repo_filter.go` (mocks dir relative to filter package)

8. **Run `make precommit`** in `watcher/github-build/`:
   ```bash
   cd watcher/github-build && make precommit
   ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/`; do NOT touch CHANGELOG.md yet (prompt 4 adds the entry)
- Do NOT commit — dark-factory handles git
- **Kafka failure MUST NOT update the cursor** — this invariant must be enforced in the code and tested
- `deriveState` MUST group by WorkflowID and keep only the latest run per workflow before computing red/green — not just a raw scan
- Episode SHA MUST be the `HeadSHA` of the *earliest* (smallest `CreatedAt`) failing run — not an arbitrary one
- `red → red` with DIFFERENT SHA MUST skip publish and MUST keep the original episode SHA (not update to the new SHA)
- `LoadCursor` on `os.ErrNotExist` MUST return a fresh empty cursor (not an error) — cold start is valid
- `LoadCursor` on any other error MUST return an error; `Poll` MUST surface that error and skip publishing on this cycle (the binary stays alive — next cycle retries; this matches the PR watcher's lifecycle where the cursor is loaded inside `Poll`, not in `NewWatcher`)
- All external calls (`ghClient.GetWorkflowRuns`, `ghClient.GetDefaultBranch`, `publisher.PublishCreate`) MUST receive the `ctx` passed into `Poll`; do NOT introduce `context.Background()` anywhere
- The `agentlib.CreateTaskCommand` MUST set `Frontmatter["assignee"] = "build-fixer-agent"` — that string is the contract with the build-fixer agent
- Context cancellation check MUST appear in the per-repo loop (non-blocking select)
- Error wrapping uses `github.com/bborbe/errors`; never `fmt.Errorf`
- Use `glog.Warningf` for repo-level skip events (not fatal errors)
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Existing tests (scaffold + github client + task ID + publisher) must still pass
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm Watcher interface and Poll method:
grep -n "type Watcher interface\|Poll(" watcher/github-build/pkg/watcher.go

# Confirm state machine transitions in watcher:
grep -n "green_to_red\|red_to_green\|CurrentEpisodeSHA\|episodeSHA" watcher/github-build/pkg/watcher.go

# Confirm cursor file path and atomic write:
grep -n "cursor.json\|Rename\|\.tmp" watcher/github-build/pkg/cursor.go

# Confirm Kafka failure does NOT update cursor (episodeSHA assigned only after publish succeeds):
grep -n -A5 "PublishCreate\|kafka_error" watcher/github-build/pkg/watcher.go
# Expected: cursor update only on the non-error path

# Confirm metrics are pre-initialized in init():
grep -n "func init\|MustRegister\|With(" watcher/github-build/pkg/metrics.go

# Confirm rate-limit early exit:
grep -n "ErrRateLimited\|rate_limited\|break" watcher/github-build/pkg/watcher.go

# Confirm context cancellation in loop:
grep -n "ctx.Done\|select" watcher/github-build/pkg/watcher.go

# Confirm all mocks generated:
ls watcher/github-build/pkg/mocks/

# Confirm worked-example test exists:
grep -n "commit A\|episodeSHA\|t1\|t4" watcher/github-build/pkg/watcher_test.go

# Confirm assignee contract is wired in the watcher:
grep -n "build-fixer-agent" watcher/github-build/pkg/watcher.go
# Expected: at least one match — the watcher sets Frontmatter["assignee"]
</verification>
