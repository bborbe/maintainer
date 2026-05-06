---
status: committing
spec: [020-richer-build-task-context]
summary: Extended WorkflowRun with 6 new fields (RunID, DisplayTitle, HeadBranch, Event, StartedAt, UpdatedAt) populated from existing workflow-run API response, and emitted a structured header block in build-failure task bodies with all fields conditionally emitted; added formatDuration helper with full DescribeTable coverage and 4 header-assertion tests in watcher_test.go.
container: maintainer-097-spec-020-header-context
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T21:00:00Z"
queued: "2026-05-06T20:54:21Z"
started: "2026-05-06T20:58:24Z"
branch: dark-factory/richer-build-task-context
---

<summary>
- Build-failure task bodies gain a structured header section answering "what commit broke it" without any new API calls
- Six new fields added to `WorkflowRun`: `RunID`, `DisplayTitle`, `HeadBranch`, `Event`, `StartedAt`, `UpdatedAt` — all sourced from the existing `ListRepositoryWorkflowRuns` response (zero extra round-trips)
- `RunID` is required by the next prompt (jobs API call uses the run instance ID, not the workflow definition ID)
- Header fields are emitted only when non-zero/non-empty — missing fields are silently omitted, preserving today's body format when new struct fields are zero-valued
- Duration is computed as `UpdatedAt − StartedAt` and formatted as `Xm Ys`; if either timestamp is zero, Duration is omitted
- Context comes from `failingRuns[0]` — the earliest failing run, which already establishes the episode SHA
- `formatDuration` is a private helper tested in `watcher_internal_test.go`
- Existing tests (frontmatter, task identifier, cursor state) pass unchanged — new tests verify header presence in body
- CHANGELOG entry under `## Unreleased`
</summary>

<objective>
Extend `WorkflowRun` with six new fields populated from the already-fetched workflow-run API response, and emit a structured header block in the task body that answers "what commit broke it". No new API calls are added — all new data comes from fields already present in the `ListRepositoryWorkflowRuns` response.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — struct patterns, private helpers.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, DescribeTable, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.

**Dependency check — run before making any changes:**

```bash
grep -n "WatcherCreateTaskCommand" watcher/github-build/pkg/watcher.go
```
If the output does NOT contain `WatcherCreateTaskCommand` (i.e., `buildCreateTaskCommand` still returns `agentlib.CreateTaskCommand`), STOP and report `status: failed` with reason "spec-018 not yet merged — buildCreateTaskCommand must return WatcherCreateTaskCommand before spec-020 can proceed".

**Files to read fully before making any changes:**
- `watcher/github-build/pkg/githubclient.go` — full file; understand `WorkflowRun` struct (current fields: `WorkflowID`, `Name`, `HeadSHA`, `Conclusion`, `HTMLURL`, `CreatedAt`); understand how runs are populated in `GetWorkflowRuns`
- `watcher/github-build/pkg/watcher.go` — full file; understand `buildCreateTaskCommand` method body (lines that build `lines` slice and the `# Build Failure` header)
- `watcher/github-build/pkg/watcher_test.go` — full file; note existing `WorkflowRun` struct literals in `GetWorkflowRunsReturns` calls — new fields default to zero so existing tests compile unchanged
- `watcher/github-build/pkg/watcher_internal_test.go` — full file; follow the `DescribeTable` pattern for the new `formatDuration` tests

**Verify go-github v62 field accessors before writing any code:**

```bash
# Confirm WorkflowRun accessors exist:
grep -n "func.*WorkflowRun.*Get\(ID\|DisplayTitle\|HeadBranch\|Event\|RunStartedAt\|UpdatedAt\)" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_runs.go 2>/dev/null | head -20
```

The expected accessors on `*gogithub.WorkflowRun`:
- `GetID() int64` → run instance ID (used by jobs API; distinct from `GetWorkflowID()` which is the definition ID)
- `GetDisplayTitle() string` → commit display title shown in GitHub UI
- `GetHeadBranch() string` → branch name that triggered the run
- `GetEvent() string` → triggering event (`push`, `pull_request`, `schedule`, etc.)
- `GetRunStartedAt() *Timestamp` → when execution started (not when queued/created)
- `GetUpdatedAt() *Timestamp` → when last modified; for completed runs this is the completion time

If any accessor does not exist in the grep output, adapt the field name to what actually exists and document the deviation in `## Improvements`.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 4. Run `make precommit` only at the final step.**

1. **Extend `WorkflowRun` struct in `watcher/github-build/pkg/githubclient.go`**

   Add six new fields after `CreatedAt`:

   ```go
   type WorkflowRun struct {
       WorkflowID   int64
       RunID        int64     // run instance ID — used by jobs API (GET /actions/runs/{id}/jobs)
       Name         string
       HeadSHA      string
       Conclusion   string
       HTMLURL      string
       CreatedAt    time.Time
       DisplayTitle string    // display_title: commit message shown in GitHub UI
       HeadBranch   string    // head_branch: branch that triggered the run
       Event        string    // event: push / pull_request / schedule / workflow_dispatch / etc.
       StartedAt    time.Time // run_started_at: when execution began (not queuing time)
       UpdatedAt    time.Time // updated_at: last status change — completion time for done runs
   }
   ```

2. **Populate new fields in `GetWorkflowRuns` in `watcher/github-build/pkg/githubclient.go`**

   In the `for _, run := range result.WorkflowRuns` loop, after the existing `createdAt` extraction block, add nil-guarded extractions for the new fields, then populate them in the `WorkflowRun` struct:

   ```go
   var startedAt time.Time
   if run.RunStartedAt != nil {
       startedAt = run.RunStartedAt.Time
   }
   var updatedAt time.Time
   if run.UpdatedAt != nil {
       updatedAt = run.UpdatedAt.Time
   }

   runs = append(runs, WorkflowRun{
       WorkflowID:   run.GetWorkflowID(),
       RunID:        run.GetID(),
       Name:         run.GetName(),
       HeadSHA:      run.GetHeadSHA(),
       Conclusion:   run.GetConclusion(),
       HTMLURL:      run.GetHTMLURL(),
       CreatedAt:    createdAt,
       DisplayTitle: run.GetDisplayTitle(),
       HeadBranch:   run.GetHeadBranch(),
       Event:        run.GetEvent(),
       StartedAt:    startedAt,
       UpdatedAt:    updatedAt,
   })
   ```

3. **Add `formatDuration` helper to `watcher/github-build/pkg/watcher.go`**

   Add this private helper after the `coalesceString` function at the bottom of the file:

   ```go
   // formatDuration formats d as a human-readable string for the task body header.
   // Returns "" when d ≤ 0 so callers can omit the Duration line for zero timestamps.
   func formatDuration(d time.Duration) string {
       if d <= 0 {
           return ""
       }
       d = d.Round(time.Second)
       h := int(d.Hours())
       m := int(d.Minutes()) % 60
       s := int(d.Seconds()) % 60
       if h > 0 {
           return fmt.Sprintf("%dh %dm %ds", h, m, s)
       }
       if m > 0 {
           return fmt.Sprintf("%dm %ds", m, s)
       }
       return fmt.Sprintf("%ds", s)
   }
   ```

4. **Update `buildCreateTaskCommand` in `watcher/github-build/pkg/watcher.go`**

   The method currently opens with:
   ```go
   lines := make([]string, 0, 6+len(failingRuns))
   lines = append(lines,
       fmt.Sprintf("# Build Failure: %s/%s", owner, repo),
       "",
       fmt.Sprintf("Episode SHA: `%s`", episodeSHA),
       "",
       "## Failing Workflows",
       "",
   )
   ```

   Replace this block with the new header-emitting logic. `failingRuns` is guaranteed non-empty when this method is called (it's called only from the `green→red` branch of `applyStateMachine` where `len(failingRuns) > 0`). Use `failingRuns[0]` (the earliest failing run, which also established the episode SHA) for header context:

   ```go
   firstRun := failingRuns[0]
   lines := make([]string, 0, 12+len(failingRuns))
   lines = append(lines, fmt.Sprintf("# Build Failure: %s/%s", owner, repo), "")

   // Header fields — emit only when non-empty/non-zero (graceful degradation).
   if firstRun.DisplayTitle != "" {
       lines = append(lines, fmt.Sprintf("**Commit:** %s", firstRun.DisplayTitle))
   }
   if firstRun.HeadBranch != "" {
       lines = append(lines, fmt.Sprintf("**Branch:** %s", firstRun.HeadBranch))
   }
   if firstRun.Event != "" {
       lines = append(lines, fmt.Sprintf("**Event:** %s", firstRun.Event))
   }
   if !firstRun.StartedAt.IsZero() {
       lines = append(lines, fmt.Sprintf("**Started:** %s", firstRun.StartedAt.UTC().Format(time.RFC3339)))
   }
   if !firstRun.UpdatedAt.IsZero() {
       lines = append(lines, fmt.Sprintf("**Finished:** %s", firstRun.UpdatedAt.UTC().Format(time.RFC3339)))
   }
   if !firstRun.StartedAt.IsZero() && !firstRun.UpdatedAt.IsZero() {
       if d := formatDuration(firstRun.UpdatedAt.Sub(firstRun.StartedAt)); d != "" {
           lines = append(lines, fmt.Sprintf("**Duration:** %s", d))
       }
   }

   lines = append(lines,
       "",
       fmt.Sprintf("Episode SHA: `%s`", episodeSHA),
       "",
       "## Failing Workflows",
       "",
   )
   for _, run := range failingRuns {
       lines = append(lines, fmt.Sprintf("- [%s](%s)", run.Name, run.HTMLURL))
   }
   body := strings.Join(lines, "\n") + "\n"
   ```

   Add `"time"` to the import block in `watcher.go` if not already present.

   The rest of `buildCreateTaskCommand` (frontmatter construction, `WatcherCreateTaskCommand` return) is unchanged.

5. **Run `make test` to verify compilation and existing test passage:**

   ```bash
   cd watcher/github-build && make test
   ```

   All existing tests must pass. New fields default to zero-values — existing `WorkflowRun` literals in tests compile unchanged.

6. **Add `formatDuration` unit tests in `watcher/github-build/pkg/watcher_internal_test.go`**

   Append after the existing `WatcherCreateTaskCommand JSON marshalling` describe block:

   ```go
   var _ = Describe("formatDuration", func() {
       DescribeTable("formats duration as human-readable string",
           func(input time.Duration, want string) {
               Expect(formatDuration(input)).To(Equal(want))
           },
           Entry("zero duration returns empty", 0*time.Second, ""),
           Entry("negative duration returns empty", -1*time.Second, ""),
           Entry("seconds only", 47*time.Second, "47s"),
           Entry("minutes and seconds", 2*time.Minute+47*time.Second, "2m 47s"),
           Entry("hours, minutes, seconds", 1*time.Hour+5*time.Minute+3*time.Second, "1h 5m 3s"),
           Entry("exactly one minute", 60*time.Second, "1m 0s"),
           Entry("sub-500ms rounds to zero → empty", 499*time.Millisecond, ""),
           Entry("exactly 500ms rounds to 1s", 500*time.Millisecond, "1s"),
           Entry("1499ms rounds to 1s", 1499*time.Millisecond, "1s"),
           Entry("1500ms rounds to 2s", 1500*time.Millisecond, "2s"),
       )
   })
   ```

   Note: `formatDuration` uses `d.Round(time.Second)`. Per the Go stdlib doc, `Round` rounds **half away from zero** — so 500ms → 1s, 1500ms → 2s, etc. The 499ms / 1499ms entries above lock the boundary on the under-half side; 500ms / 1500ms lock the at-half side.

7. **Add header assertion tests in `watcher/github-build/pkg/watcher_test.go`**

   Append a new `Describe("task body header context", ...)` block inside the outer `Describe("Watcher", ...)`, after the existing `Describe("per-repo maintenance overrides", ...)` block:

   ```go
   Describe("task body header context", func() {
       var t0 time.Time

       BeforeEach(func() {
           t0 = time.Date(2026, 5, 6, 14, 32, 0, 0, time.UTC)
       })

       It("includes all header fields when WorkflowRun has full context", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
               {
                   WorkflowID:   1,
                   RunID:        42,
                   Name:         "CI",
                   HeadSHA:      "sha-abc",
                   Conclusion:   "failure",
                   HTMLURL:      "https://github.com/owner/repo/actions/runs/42",
                   CreatedAt:    t0,
                   DisplayTitle: "Fix authentication bug",
                   HeadBranch:   "main",
                   Event:        "push",
                   StartedAt:    t0,
                   UpdatedAt:    t0.Add(3*time.Minute + 47*time.Second),
               },
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Body).To(ContainSubstring("**Commit:** Fix authentication bug"))
           Expect(cmd.Body).To(ContainSubstring("**Branch:** main"))
           Expect(cmd.Body).To(ContainSubstring("**Event:** push"))
           Expect(cmd.Body).To(ContainSubstring("**Started:** 2026-05-06T14:32:00Z"))
           Expect(cmd.Body).To(ContainSubstring("**Finished:** 2026-05-06T14:35:47Z"))
           Expect(cmd.Body).To(ContainSubstring("**Duration:** 3m 47s"))
       })

       It("omits all header fields when WorkflowRun has zero context (backwards compat)", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
               {
                   WorkflowID: 1,
                   HeadSHA:    "sha-abc",
                   Conclusion: "failure",
                   CreatedAt:  time.Now(),
                   // DisplayTitle, HeadBranch, Event, StartedAt, UpdatedAt all zero
               },
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Body).NotTo(ContainSubstring("**Commit:**"))
           Expect(cmd.Body).NotTo(ContainSubstring("**Branch:**"))
           Expect(cmd.Body).NotTo(ContainSubstring("**Duration:**"))
           // Episode SHA and section header still present
           Expect(cmd.Body).To(ContainSubstring("Episode SHA: `sha-abc`"))
           Expect(cmd.Body).To(ContainSubstring("## Failing Workflows"))
       })

       It("omits Duration when only StartedAt is set (UpdatedAt zero)", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
               {
                   WorkflowID: 1,
                   HeadSHA:    "sha-abc",
                   Conclusion: "failure",
                   CreatedAt:  time.Now(),
                   StartedAt:  t0,
                   // UpdatedAt zero
               },
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Body).To(ContainSubstring("**Started:** 2026-05-06T14:32:00Z"))
           Expect(cmd.Body).NotTo(ContainSubstring("**Finished:**"))
           Expect(cmd.Body).NotTo(ContainSubstring("**Duration:**"))
       })

       It("uses earliest failing run for header context when multiple runs fail", func() {
           early := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
           late := early.Add(time.Hour)

           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
               {
                   WorkflowID:   1,
                   RunID:        10,
                   Name:         "CI",
                   HeadSHA:      "sha-late",
                   Conclusion:   "failure",
                   HTMLURL:      "https://github.com/owner/repo/actions/runs/10",
                   CreatedAt:    late,
                   DisplayTitle: "Late commit",
                   HeadBranch:   "feature",
               },
               {
                   WorkflowID:   2,
                   RunID:        20,
                   Name:         "Deploy",
                   HeadSHA:      "sha-early",
                   Conclusion:   "failure",
                   HTMLURL:      "https://github.com/owner/repo/actions/runs/20",
                   CreatedAt:    early,
                   DisplayTitle: "Early commit",
                   HeadBranch:   "main",
               },
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           _, cmd := publisher.PublishCreateArgsForCall(0)
           // Header context comes from earliest run (early, sha-early)
           Expect(cmd.Body).To(ContainSubstring("**Commit:** Early commit"))
           Expect(cmd.Body).To(ContainSubstring("**Branch:** main"))
           // Episode SHA is also from earliest run
           Expect(cmd.Frontmatter["episode_sha"]).To(Equal("sha-early"))
       })
   })
   ```

8. **Add `CHANGELOG.md` entry** under `## Unreleased` at the root:

   ```
   - feat(watcher/github-build): enrich task body header — commit subject, branch, event, started/finished timestamps, and elapsed duration now appear in every build-failure task body using fields already present in the workflow-run API response (zero extra API calls); all fields are optional and omitted gracefully when not populated
   ```

9. **Run `make precommit`** from `watcher/github-build/`:

   ```bash
   cd watcher/github-build && make precommit
   ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/pkg/` and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- **Dependency on spec-018:** If `buildCreateTaskCommand` in `watcher.go` returns `agentlib.CreateTaskCommand` (not `WatcherCreateTaskCommand`), STOP and report `status: failed` with reason "spec-018 not yet merged"
- Do NOT add `GetJobsForRun` or `GetJobLog` to `GitHubClient` — those belong in prompts 2 and 3
- `WorkflowRun.RunID` MUST be populated with `run.GetID()` (the run instance ID), NOT with `run.GetWorkflowID()` (the workflow definition ID); the next prompt uses `RunID` to call the jobs API
- Header fields MUST be conditionally emitted — a zero `time.Time` for `StartedAt` or `UpdatedAt`, or an empty string for `DisplayTitle`/`HeadBranch`/`Event`, MUST produce no corresponding `**Commit:**` / `**Branch:**` / etc. line
- Duration MUST be omitted when either `StartedAt` or `UpdatedAt` is zero
- `formatDuration` MUST return `""` for non-positive durations (zero and negative) so callers can skip the line
- Body output MUST be deterministic for the same `WorkflowRun` inputs — use `UTC().Format(time.RFC3339)` for timestamps, no random elements
- All new `WorkflowRun` fields are populated from the existing `ListRepositoryWorkflowRuns` response — NO new GitHub API methods are added in this prompt
- `GitHubClient` interface (method signatures) MUST NOT change in this prompt — no mock regeneration needed
- Existing tests in `watcher_test.go` MUST still pass — their `WorkflowRun` literals omit the new fields (zero-valued), which is valid Go and preserves today's body format
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Error wrapping: `github.com/bborbe/errors`; never `fmt.Errorf` (note: `formatDuration` uses `fmt.Sprintf` for STRING formatting — this is correct; `fmt.Errorf` is banned only for ERROR wrapping)
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm RunID field added to WorkflowRun:
grep -n "RunID\s*int64" watcher/github-build/pkg/githubclient.go
# Expected: one match

# Confirm RunID populated with GetID() not GetWorkflowID():
grep -n "RunID.*GetID\|RunID.*GetWorkflowID" watcher/github-build/pkg/githubclient.go
# Expected: RunID: run.GetID()

# Confirm DisplayTitle/HeadBranch/Event/StartedAt/UpdatedAt fields exist:
grep -n "DisplayTitle\|HeadBranch\s\|Event\s\|StartedAt\|UpdatedAt" watcher/github-build/pkg/githubclient.go
# Expected: struct field declarations + population in GetWorkflowRuns

# Confirm nil-guard for RunStartedAt:
grep -A 2 "RunStartedAt" watcher/github-build/pkg/githubclient.go
# Expected: nil check before .Time access

# Confirm formatDuration helper exists:
grep -n "func formatDuration" watcher/github-build/pkg/watcher.go
# Expected: one match

# Confirm header lines conditionally emitted (no unconditional DisplayTitle output):
grep -n "DisplayTitle\|HeadBranch\|Event\|StartedAt\|UpdatedAt\|formatDuration" watcher/github-build/pkg/watcher.go
# Expected: conditional if blocks before each Sprintf call

# Confirm timestamp format is RFC3339:
grep -n "RFC3339" watcher/github-build/pkg/watcher.go
# Expected: at least one match in buildCreateTaskCommand

# Confirm CHANGELOG entry:
grep -n "header\|display_title\|branch.*event\|richer" CHANGELOG.md
# Expected: one match under ## Unreleased

# Confirm formatDuration tests in internal test file:
grep -n "formatDuration" watcher/github-build/pkg/watcher_internal_test.go
# Expected: DescribeTable with at least 5 entries

# Confirm header tests in watcher_test.go:
grep -n "DisplayTitle\|HeadBranch\|Duration\|header context" watcher/github-build/pkg/watcher_test.go
# Expected: new Describe block with assertions for header fields
</verification>
