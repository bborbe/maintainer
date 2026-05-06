---
spec: ["020"]
status: draft
created: "2026-05-06T21:00:00Z"
---

<summary>
- Task body's `## Failing Workflows` bullet list is replaced by a Markdown table with columns: Workflow / Job / Failed Step / Run
- Failed-step names come from a single `GET /repos/{owner}/{repo}/actions/runs/{id}/jobs` call per failing run — exactly one call per run per publish (no re-fetching)
- `GetJobsForRun` is added to the `GitHubClient` interface and the concrete `*githubClient` implementation
- `buildCreateTaskCommand` gains a `ctx context.Context` first parameter so it can issue the jobs API call per run
- `applyStateMachine` is updated to pass `ctx` to `buildCreateTaskCommand`
- Jobs API failures (rate-limit, 5xx, timeout) cause that run's row to show `?` for Job and Failed Step — the publish always proceeds with whatever data was successfully fetched
- Exactly one jobs API call is issued per failing run: if a run has multiple failed jobs, the first one is used
- Mock for `GitHubClient` is regenerated to include `GetJobsForRunStub`
- Tests verify table format, API call count (one per run), and degraded path when jobs API fails
</summary>

<objective>
Replace the `## Failing Workflows` bullet list with a structured Markdown table showing workflow name, failed job name, failed step name, and a run link. Failed job/step names require one `GET jobs` call per failing run. `buildCreateTaskCommand` receives `ctx` so it can issue these calls internally. All failures are handled gracefully: the table always renders, with `?` for columns that couldn't be fetched.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.

**Dependency check — run before making any changes:**

```bash
# Confirm prompt 1 is complete (RunID field present):
grep -n "RunID\s*int64" watcher/github-build/pkg/githubclient.go
```
If `RunID` is not present in `WorkflowRun`, STOP and report `status: failed` with reason "spec-020 prompt 1 not yet executed — RunID field missing from WorkflowRun".

**Files to read fully before making any changes:**
- `watcher/github-build/pkg/githubclient.go` — full file; understand existing `GitHubClient` interface and `*githubClient` error-detection patterns (`stderrors.As` for rate limits); confirm `WorkflowRun.RunID` exists; understand `ErrRateLimited` sentinel
- `watcher/github-build/pkg/watcher.go` — full file; understand `buildCreateTaskCommand` body (the `lines` slice construction and the `## Failing Workflows` bullet loop); understand `applyStateMachine` call site
- `watcher/github-build/pkg/watcher_test.go` — full file; note how `GetWorkflowRunsReturns` is used; understand `makeWatcher` helper; understand existing body assertions
- `watcher/github-build/pkg/mocks/github_client.go` — understand the counterfeiter mock structure; this file will be regenerated after step 2

**Verify go-github v62 jobs API before writing any code:**

```bash
# Confirm ListWorkflowJobs method:
grep -n "func.*Actions.*ListWorkflowJobs" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_jobs.go 2>/dev/null | head -5

# Confirm ListWorkflowJobsOptions struct:
grep -n "type ListWorkflowJobsOptions\|Filter " \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_jobs.go 2>/dev/null | head -10

# Confirm WorkflowJobs return type and Jobs field:
grep -n "type WorkflowJobs\|Jobs \[" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_jobs.go 2>/dev/null | head -10

# Confirm WorkflowJob struct (ID, Name, Conclusion, Steps fields):
grep -n "type WorkflowJob struct\|ID \*int64\|Name \*string\|Conclusion \*string\|Steps " \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/actions_workflow_jobs.go 2>/dev/null | head -20

# Confirm TaskStep struct (step within a job):
grep -rn "type TaskStep struct\|type.*Step struct" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/*.go 2>/dev/null | head -10

# Confirm step Conclusion and Name accessors:
grep -n "func.*TaskStep.*GetConclusion\|func.*TaskStep.*GetName" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v62@*/github/*.go 2>/dev/null | head -10
```

Adapt implementation to the actual struct/method names found. Document any deviations in `## Improvements`.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 5. Run `make precommit` only at the final step.**

1. **Add `WorkflowJobInfo` type and `GetJobsForRun` to `GitHubClient` in `watcher/github-build/pkg/githubclient.go`**

   After the `WorkflowRun` struct, add:

   ```go
   // WorkflowJobInfo holds the failed job and step names for one failing workflow run.
   // If no failed job is found in the response, JobName and FailedStepName are empty strings.
   type WorkflowJobInfo struct {
       JobID          int64
       JobName        string
       FailedStepName string // first failed step's name; empty when not determinable
   }
   ```

   Add `GetJobsForRun` to the `GitHubClient` interface (after `GetFileContent`):

   ```go
   // GetJobsForRun returns info about the first failed job in a run.
   // Returns an empty slice (not an error) when the run has no failed jobs.
   // Returns (nil, ErrRateLimited) when rate-limited.
   // Returns (nil, err) for other API errors.
   GetJobsForRun(ctx context.Context, owner, repo string, runID int64) ([]WorkflowJobInfo, error)
   ```

2. **Implement `GetJobsForRun` on `*githubClient` in `watcher/github-build/pkg/githubclient.go`**

   Add after the `GetFileContent` method:

   ```go
   func (c *githubClient) GetJobsForRun(
       ctx context.Context,
       owner, repo string,
       runID int64,
   ) ([]WorkflowJobInfo, error) {
       opts := &gogithub.ListWorkflowJobsOptions{Filter: "latest"}
       result, _, err := c.client.Actions.ListWorkflowJobs(ctx, owner, repo, runID, opts)
       if err != nil {
           var rl *gogithub.RateLimitError
           var arl *gogithub.AbuseRateLimitError
           if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
               return nil, ErrRateLimited
           }
           return nil, errors.Wrapf(ctx, err, "list jobs for run %d owner=%s repo=%s", runID, owner, repo)
       }
       var infos []WorkflowJobInfo
       for _, job := range result.Jobs {
           if job.GetConclusion() != "failure" {
               continue
           }
           var failedStep string
           for _, step := range job.Steps {
               if step.GetConclusion() == "failure" {
                   failedStep = step.GetName()
                   break
               }
           }
           infos = append(infos, WorkflowJobInfo{
               JobID:          job.GetID(),
               JobName:        job.GetName(),
               FailedStepName: failedStep,
           })
       }
       return infos, nil
   }
   ```

   Note: `job.Steps` may be a `[]*gogithub.TaskStep` or a named type — adapt to what the grep found in the context step.

3. **Regenerate `GitHubClient` mock** (interface gained `GetJobsForRun`):

   ```bash
   cd watcher/github-build && go generate ./pkg/...
   ```

   If no `//go:generate` directive, run counterfeiter directly:

   ```bash
   cd watcher/github-build && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o pkg/mocks/github_client.go \
       --fake-name GitHubClient \
       ./pkg/. GitHubClient
   ```

   Confirm `pkg/mocks/github_client.go` includes `GetJobsForRunStub`, `GetJobsForRunCallCount`, `GetJobsForRunArgsForCall`, `GetJobsForRunReturns`.

4. **Add `ctx context.Context` as the first parameter to `buildCreateTaskCommand` in `watcher/github-build/pkg/watcher.go`**

   Change the signature from:
   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
       assignee, taskStatus, taskPhase string,
   ) WatcherCreateTaskCommand {
   ```
   To:
   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
       ctx context.Context,
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
       assignee, taskStatus, taskPhase string,
   ) WatcherCreateTaskCommand {
   ```

   Update the call site in `applyStateMachine`. The current call:
   ```go
   cmd := w.buildCreateTaskCommand(
       taskID,
       owner,
       repo,
       episodeSHA,
       failingRuns,
       effectiveAssignee,
       effectiveStatus,
       effectivePhase,
   )
   ```
   Becomes:
   ```go
   cmd := w.buildCreateTaskCommand(
       ctx,
       taskID,
       owner,
       repo,
       episodeSHA,
       failingRuns,
       effectiveAssignee,
       effectiveStatus,
       effectivePhase,
   )
   ```

   If `watcher/github-build/pkg/watcher_internal_test.go` directly calls `buildCreateTaskCommand`, update that call to pass a `ctx` as the first argument.

5. **Replace the bullet list with a Markdown table in `buildCreateTaskCommand` in `watcher/github-build/pkg/watcher.go`**

   The current body-building code ends with a bullet-list loop:
   ```go
   for _, run := range failingRuns {
       lines = append(lines, fmt.Sprintf("- [%s](%s)", run.Name, run.HTMLURL))
   }
   ```

   Replace with the following table-building logic. The table is built AFTER the header lines and before the `body` string join:

   ```go
   // Table header
   lines = append(lines,
       "| Workflow | Job | Failed Step | Run |",
       "|---|---|---|---|",
   )

   // One row per failing run; one GetJobsForRun call per run.
   for _, run := range failingRuns {
       jobName := "?"
       stepName := "?"
       if run.RunID != 0 {
           jobs, err := w.githubClient.GetJobsForRun(ctx, owner, repo, run.RunID)
           if err != nil {
               glog.Warningf("jobs API failed run=%d repo=%s/%s err=%v — using ? placeholders", run.RunID, owner, repo, err)
           } else if len(jobs) > 0 {
               jobName = jobs[0].JobName
               if jobs[0].FailedStepName != "" {
                   stepName = jobs[0].FailedStepName
               }
           }
       }
       lines = append(lines, fmt.Sprintf("| %s | %s | %s | [Run](%s) |",
           run.Name, jobName, stepName, run.HTMLURL))
   }
   ```

   The `"## Failing Workflows"` and `""` lines are still appended earlier (from step 4 in prompt 1 — they appear right before the table). The structure is:
   ```
   ## Failing Workflows

   | Workflow | Job | Failed Step | Run |
   |---|---|---|---|
   | CI | build | Run tests | [Run](url) |
   ```

   Remove the old `fmt.Sprintf("- [%s](%s)", run.Name, run.HTMLURL)` bullet line — it is no longer used.

6. **Run `make test`** to verify compilation and test passage:

   ```bash
   cd watcher/github-build && make test
   ```

   Existing tests may now see a table (with `?` for job/step since `GetJobsForRun` mock returns nil by default) instead of a bullet list in `cmd.Body`. Any body assertions that used `ContainSubstring("- [CI]")` must be updated to use `ContainSubstring("| CI |")` or `ContainSubstring("## Failing Workflows")`. Check and fix any such breakage before proceeding.

7. **Add jobs-table tests in `watcher/github-build/pkg/watcher_test.go`**

   Append a new `Describe("failing workflows table", ...)` block:

   ```go
   Describe("failing workflows table", func() {
       var runID int64 = 42

       singleFailingRunWithID := func(sha string) []pkg.WorkflowRun {
           return []pkg.WorkflowRun{
               {
                   WorkflowID: 1,
                   RunID:      runID,
                   Name:       "CI",
                   HeadSHA:    sha,
                   Conclusion: "failure",
                   HTMLURL:    "https://github.com/owner/repo/actions/runs/42",
                   CreatedAt:  time.Now(),
               },
           }
       }

       BeforeEach(func() {
           ghClient.GetDefaultBranchReturns("main", nil)
       })

       It("emits a table with job name and step name when jobs API succeeds", func() {
           ghClient.GetWorkflowRunsReturns(singleFailingRunWithID("sha-jobs"), nil)
           ghClient.GetJobsForRunReturns([]pkg.WorkflowJobInfo{
               {JobID: 99, JobName: "build", FailedStepName: "Run tests"},
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Body).To(ContainSubstring("| Workflow | Job | Failed Step | Run |"))
           Expect(cmd.Body).To(ContainSubstring("| CI | build | Run tests | [Run](https://github.com/owner/repo/actions/runs/42) |"))
       })

       It("shows ? for job and step when jobs API returns an error", func() {
           ghClient.GetWorkflowRunsReturns(singleFailingRunWithID("sha-err"), nil)
           ghClient.GetJobsForRunReturns(nil, errors.New("http 503 service unavailable"))

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           // Publish still succeeds despite jobs API failure
           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Body).To(ContainSubstring("| CI | ? | ? |"))
       })

       It("shows ? for step when jobs API returns a job with no failed step", func() {
           ghClient.GetWorkflowRunsReturns(singleFailingRunWithID("sha-nostep"), nil)
           ghClient.GetJobsForRunReturns([]pkg.WorkflowJobInfo{
               {JobID: 99, JobName: "build", FailedStepName: ""},
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Body).To(ContainSubstring("| CI | build | ? |"))
       })

       It("calls GetJobsForRun exactly once per failing run", func() {
           early := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
           late := early.Add(time.Minute)

           ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
               {WorkflowID: 1, RunID: 10, Name: "CI", HeadSHA: "sha-a",
                   Conclusion: "failure", HTMLURL: "https://x/1", CreatedAt: early},
               {WorkflowID: 2, RunID: 20, Name: "Lint", HeadSHA: "sha-a",
                   Conclusion: "failure", HTMLURL: "https://x/2", CreatedAt: late},
           }, nil)
           ghClient.GetJobsForRunReturns([]pkg.WorkflowJobInfo{
               {JobID: 1, JobName: "check", FailedStepName: "golangci-lint"},
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())

           // Exactly 2 calls — one per failing run (not one per job or step)
           Expect(ghClient.GetJobsForRunCallCount()).To(Equal(2))
           _, _, _, runID1 := ghClient.GetJobsForRunArgsForCall(0)
           _, _, _, runID2 := ghClient.GetJobsForRunArgsForCall(1)
           Expect([]int64{runID1, runID2}).To(ConsistOf(int64(10), int64(20)))
       })

       It("table still renders on second poll (red→red) without additional GetJobsForRun calls", func() {
           ghClient.GetWorkflowRunsReturns(singleFailingRunWithID("sha-locked"), nil)
           ghClient.GetJobsForRunReturns([]pkg.WorkflowJobInfo{
               {JobID: 99, JobName: "build", FailedStepName: "Run tests"},
           }, nil)

           w := makeWatcher([]string{"owner/repo"})
           Expect(w.Poll(ctx)).To(Succeed())
           firstCallCount := ghClient.GetJobsForRunCallCount()
           Expect(firstCallCount).To(Equal(1))

           // Second poll: red→red — no publish, no GetJobsForRun call
           Expect(w.Poll(ctx)).To(Succeed())
           Expect(ghClient.GetJobsForRunCallCount()).To(Equal(firstCallCount))
           Expect(publisher.PublishCreateCallCount()).To(Equal(1)) // still only 1 publish
       })
   })
   ```

   Note: the `errors.New` call in the test needs `stderrors "errors"` import or `os.ErrNotExist` — use whatever is already imported. Or use `fmt.Errorf` in test code only (test code is exempt from the "no fmt.Errorf" rule since `go-error-wrapping-guide.md` bans it for production code, not tests). Alternatively, import the already-present `os` package and use `os.ErrNotExist`.

8. **Add a CHANGELOG entry** to root `CHANGELOG.md` under `## Unreleased`:

   ```
   - feat(watcher/github-build): replace failing-workflows bullet list with a Markdown table — columns: Workflow / Job / Failed Step / Run; failed-step names fetched via one jobs API call per failing run; degraded gracefully (shows ?) when the jobs API is unavailable
   ```

9. **Run `make precommit`** from `watcher/github-build/`:

   ```bash
   cd watcher/github-build && make precommit
   ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/pkg/` and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- **Dependency on spec-020 prompt 1:** If `WorkflowRun.RunID` is missing, STOP and report `status: failed`
- `GetJobsForRun` MUST be called at most ONCE per failing run per `buildCreateTaskCommand` invocation — NOT once per job, NOT once per step
- When `run.RunID == 0` (zero-valued), skip the jobs API call and use `?` — this protects against tests that don't set `RunID`
- When `GetJobsForRun` returns an error (including `ErrRateLimited`): WARN log + use `?` for job/step columns + continue publishing — the publish MUST NOT be blocked
- When `GetJobsForRun` returns no failed jobs (empty slice or no job with `Conclusion=="failure"`): use `?` for both columns
- When `GetJobsForRun` succeeds but the first failed job has an empty `FailedStepName`: use `?` for the step column only
- The table header row `| Workflow | Job | Failed Step | Run |` and separator `|---|---|---|---|` MUST always appear even when all rows show `?`
- `buildCreateTaskCommand` MUST accept `ctx context.Context` as its first parameter — it is now responsible for issuing API calls
- `applyStateMachine` MUST pass its `ctx` parameter to `buildCreateTaskCommand` — never `context.Background()`
- The `jobs` API is NEVER called for `red→red` or other non-publish transitions — `buildCreateTaskCommand` is only called in the publish branch
- Mock MUST be regenerated via counterfeiter after adding `GetJobsForRun` to the interface
- Error wrapping: `github.com/bborbe/errors` for production code; `stderrors "errors"` or `os.ErrNotExist` acceptable in test code for simple stub errors
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Coverage ≥80% for changed packages
- All existing tests must still pass — update any body assertion that used the old bullet-list format `"- [CI]"` to use the new table format `"| CI |"` or `ContainSubstring("## Failing Workflows")`
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm WorkflowJobInfo type defined:
grep -n "type WorkflowJobInfo struct\|JobID\|JobName\|FailedStepName" watcher/github-build/pkg/githubclient.go
# Expected: struct with 3 fields

# Confirm GetJobsForRun in interface:
grep -n "GetJobsForRun" watcher/github-build/pkg/githubclient.go
# Expected: interface declaration + concrete method

# Confirm GetJobsForRun returns ErrRateLimited for rate limit:
grep -A 5 "RateLimitError" watcher/github-build/pkg/githubclient.go | grep -A 3 "GetJobsForRun" || \
  grep -n "ErrRateLimited" watcher/github-build/pkg/githubclient.go
# Expected: ErrRateLimited returned on rate limit errors

# Confirm mock regenerated with GetJobsForRun:
grep -n "GetJobsForRun" watcher/github-build/pkg/mocks/github_client.go
# Expected: GetJobsForRunStub, GetJobsForRunCallCount, GetJobsForRunArgsForCall, GetJobsForRunReturns

# Confirm buildCreateTaskCommand takes ctx as first param:
grep -A 9 "func.*buildWatcher.*buildCreateTaskCommand" watcher/github-build/pkg/watcher.go
# Expected: ctx context.Context as first parameter

# Confirm applyStateMachine passes ctx (not context.Background()):
grep -B 2 -A 10 "buildCreateTaskCommand" watcher/github-build/pkg/watcher.go | grep "ctx,"
# Expected: ctx passed as first arg

# Confirm table header always emitted:
grep -n "Workflow.*Job.*Failed Step.*Run" watcher/github-build/pkg/watcher.go
# Expected: table header string in buildCreateTaskCommand

# Confirm bullet list removed:
grep -n '"\- \[' watcher/github-build/pkg/watcher.go
# Expected: zero matches — old bullet format gone

# Confirm ? placeholders for degraded path:
grep -n '"?"' watcher/github-build/pkg/watcher.go
# Expected: jobName and stepName initialized to "?"

# Confirm GetJobsForRun skipped when RunID == 0:
grep -n "RunID.*!= 0\|RunID == 0" watcher/github-build/pkg/watcher.go
# Expected: guard against zero RunID

# Confirm warn log on jobs API error:
grep -n "Warningf.*jobs\|Warningf.*GetJobsForRun" watcher/github-build/pkg/watcher.go
# Expected: glog.Warningf call in the error path

# Confirm call-count test:
grep -n "GetJobsForRunCallCount\|exactly.*once" watcher/github-build/pkg/watcher_test.go
# Expected: assertion on call count

# Confirm CHANGELOG entry:
grep -n "bullet.*table\|Workflow.*Job.*Failed\|jobs.*API.*table" CHANGELOG.md
# Expected: one match under ## Unreleased
</verification>
