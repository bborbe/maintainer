---
status: approved
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:25:46Z"
---

<summary>
- Add tests for `GetJobsForRun` in `pkg/githubclient_test.go`
- This critical GitHub API method has 0% test coverage
- Test successful response with failed jobs, no failed jobs, HTTP error, and rate limit scenarios
</summary>

<objective>
Add tests for `GetJobsForRun` in `pkg/githubclient_test.go`. This method identifies failing jobs from a workflow run and is central to the watcher's core function. The current test suite has no coverage for it.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/githubclient.go` lines 205-246 (`GetJobsForRun` method)
- `watcher/github-build/pkg/githubclient_test.go` (existing test patterns)
- `watcher/github-build/pkg/mocks/github_client.go` (Counterfeiter mock)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
</context>

<requirements>
1. In `pkg/githubclient_test.go`, add a test table or separate Ginkgo `It` blocks for `GetJobsForRun`:

   a) **Successful response with failed jobs**:
   - Mock `GetWorkflowJobs` to return a response with 2 jobs, each having failed steps
   - Verify the returned list has 2 entries with correct `JobName` and `StepName`
   - Verify the `FailedStep` field contains the name of the failed step

   b) **Successful response with no failed jobs**:
   - Mock `GetWorkflowJobs` to return jobs with only successful steps
   - Verify returned list is empty

   c) **HTTP error from `GetWorkflowJobs`**:
   - Mock `GetWorkflowJobs` to return a non-nil error
   - Verify the error propagates (via `gomega.Expect`). The method does NOT wrap errors from `GetWorkflowJobs`, so match the exact error.

   d) **Rate limited response**:
   - Mock `GetWorkflowJobs` to return `ErrRateLimited`
   - Verify `ErrRateLimited` propagates

2. Use the existing `githubClientTest` struct pattern from `githubclient_test.go`. Create a `testServer` (using `httptest`) that mocks the GitHub API responses.

3. Follow Ginkgo/Gomega v2 patterns used in the file.

4. Run `cd watcher/github-build && go test ./pkg/... -run TestGetJobsForRun -v` to confirm tests pass.

5. Verify coverage increases: `cd watcher/github-build && go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep GetJobsForRun`
</requirements>

<constraints>
- Only change `watcher/github-build/pkg/githubclient_test.go`
- Do NOT commit — dark-factory handles git
- Tests must pass
- Use Ginkgo/Gomega v2 conventions
- Use Counterfeiter mocks from `mocks/` dir — no manual mocks
</constraints>

<verification>
cd watcher/github-build && go test ./pkg/... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out | grep -i getjobs
</verification>
