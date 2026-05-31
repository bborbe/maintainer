---
status: approved
spec: [056-github-releaser-ai-review-phase]
created: "2026-05-31T20:35:00Z"
queued: "2026-05-31T20:54:57Z"
branch: dark-factory/github-releaser-ai-review-phase
---

<summary>
- 10+ test cases covering all failure modes from spec
- Tests use counterfeiter mock on aiReviewClient seam (HTTP boundary)
- Happy path: all three checks pass, approved: true, status: done
- Tag-missing (404): approved: false, tag_exists: false, status: failed
- Annotated tag SHA mismatch: approved: false, tag_at_expected_sha: false, status: failed
- Lightweight tag SHA mismatch: same as annotated
- Changelog header still ## Unreleased: approved: false, changelog_header_rewritten: false, status: failed
- Short-circuit on Result.outcome != "released": zero HTTP calls, approved: true, status: done
- Malformed ## Result JSON: wrapped error, no ## Review written
- Token not in error strings
- No ## Failure section written on any path (controller's job)
- Tests use pkg_test package (external), counterfeiter mocks
</summary>

<objective>
Write comprehensive unit tests for the ai_review step in `agent/github-releaser/pkg/steps_ai_review_test.go` (external `pkg_test` package). Cover all acceptance criteria from the spec including happy path, all failure modes, short-circuit logic, and the token-in-logs guard. Tests use a Counterfeiter-generated mock on the `aiReviewClient` seam so HTTP calls are stubbed.
</objective>

<context>
Read `agent/github-releaser/pkg/steps_ai_review.go` to understand the step's logic and field names.
Read `agent/github-releaser/pkg/steps_planning_test.go` for the test structure (external `pkg_test` package, Ginkgo v2, counterfeiter mocks).
Read `agent/github-releaser/pkg/result_output.go` for ResultOutput field names (Outcome, CommitSHA, Tag).
Read `agent/github-releaser/mocks/fetcher.go` to understand the counterfeiter mock structure.
Read `agent/github-releaser/mocks/claude-runner.go` for the mock pattern.
Read `agent/github-releaser/pkg/export_test.go` for how unexported helpers are exposed for testing.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` for testing conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` for counterfeiter usage.
Read `agent/github-releaser/pkg/factory/factory_test.go` to understand the existing factory test patterns.

The counterfeiter mock for `aiReviewClient` will be generated in `agent/github-releaser/mocks/review_client.go` by prompt 2's `go generate ./...`. Use `//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate` and `//counterfeiter:generate . aiReviewClient` in the source package.
</context>

<requirements>
1. Create `agent/github-releaser/pkg/steps_ai_review_test.go` in package `pkg_test`.

2. Import: `agentlib`, `domain`, `gomega`, `ginkgo`, `mocks`, `pkg` (aliased).

3. Use `var _ = Describe("AIReviewStep", func() { ... })` as the top-level test block.

4. All tests use a fake `aiReviewClient` (counterfeiter mock from `mocks/review_client.go`). The mock path is `github.com/bborbe/maintainer/agent/github-releaser/mocks`.

5. Helper to build a standard task with `## Result(outcome=released, commit_sha=<sha>, tag=<tag>)`. CRITICAL: Go raw-string literals CANNOT contain backtick characters (no escape exists), so build the fenced JSON blocks via concatenation:
   ```go
   func taskWithResult(commitSHA, tag string) string {
       const fm = "---\n" +
           "status: in_progress\n" +
           "phase: ai_review\n" +
           "assignee: github-releaser-agent\n" +
           "task_type: github-release\n" +
           "repo: bborbe/example\n" +
           "task_identifier: gh-release-001\n" +
           "---\n\n"
       plan := "## Plan\n\n" +
           "```json\n" +
           `{"outcome":"ready","next_version":"1.0.0","next_version_header":"## v1.0.0"}` + "\n" +
           "```\n\n"
       result := "## Result\n\n" +
           "```json\n" +
           fmt.Sprintf(`{"outcome":"released","path":"direct-push","commit_sha":%q,"tag":%q}`, commitSHA, tag) + "\n" +
           "```\n"
       return fm + plan + result
   }
   ```
   Apply the same pattern for any other helper that needs to emit a fenced code block (e.g. the `outcome:failed` variant for test 7g and the malformed-JSON variant for test 7h).

6. Helper to run the step:
   ```go
   runStep := func(taskMD string, fakeClient *mocks.ReviewClient) (*agentlib.Result, *agentlib.Markdown) {
       step := pkg.NewAIReviewStep(fakeClient, "test-token")
       md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
       Expect(err).NotTo(HaveOccurred())
       result, err := step.Run(context.Background(), md)
       Expect(err).NotTo(HaveOccurred())
       return result, md
   }
   ```

7. Test cases. Use AAA / `BeforeEach` / `JustBeforeEach` per spec constraint: `BeforeEach` constructs `fakeClient = &mocks.ReviewClient{}` and `token = "test-token"`; `JustBeforeEach` runs `step = pkg.NewAIReviewStep(fakeClient, token)`. One `It` per scenario below.

   **7a. Happy path**: All three mock methods return success.
   - `fakeClient.TagExistsReturns("abc123", nil)`
   - `fakeClient.ResolveTagCommitReturns("abc123", nil)` (same SHA — matches)
   - `fakeClient.FetchChangelogReturns([]byte("## v1.0.0\n\n- feat\n\n## Unreleased\n\n- old"), nil)`
   - Assert: `result.Status == agentlib.AgentStatusDone`
   - Assert: `result.NextPhase == "done"` (the literal `"done"` is the terminal-completed signal per `agent/lib v0.63.x agent_agent.go:91`)
   - Extract `## Review` section: `approved: true`, `checks.tag_exists: true`, `checks.tag_at_expected_sha: true`, `checks.changelog_header_rewritten: true`
   - Notes contains "passed"

   **7b. Tag missing (404)** — must return the `pkg.ErrTagNotFound` sentinel from prompt 2's client:
   - `fakeClient.TagExistsReturns("", pkg.ErrTagNotFound)` (the typed sentinel; the step uses `errors.Is(err, pkg.ErrTagNotFound)` to distinguish 404 → verdict from 5xx → retry)
   - `fakeClient.ResolveTagCommitCallCount() == 0`
   - `fakeClient.FetchChangelogCallCount() == 0`
   - Assert: `result.Status == agentlib.AgentStatusFailed`
   - Assert: `result.NextPhase == ""` (no transition — definitely NOT `"human_review"`; per platform doctrine the controller does the unassign on `Status: failed`)
   - Extract `## Review`: `approved: false`, `checks.tag_exists: false`. **`checks.tag_at_expected_sha` and `checks.changelog_header_rewritten` are vacuously `true`** (step initializes the struct with all-true defaults and only sets a field to `false` when its specific check fails — this is documented in prompt 1 Req 8d).
   - Notes contains "not found"

   **7b-bis. Tag check returns transient 5xx error** (NOT the sentinel) — must wrap, NOT verdict:
   - `fakeClient.TagExistsReturns("", errors.New("TagExists: status 500: Server Error"))` (any non-sentinel error)
   - Assert: `result == nil` AND `err != nil` (step returned wrapped error so controller retries via Kafka redelivery — NOT a verdict)
   - Assert: error chain contains the wrap message `"ai_review: TagExists"`
   - Assert: no `## Review` section was written
   - This is the critical contract distinction: 404 (sentinel) → verdict; everything else → retry.

   **7c. Annotated tag SHA mismatch**:
   - TagExists returns `"tag-sha-annotated"` (the tag ref SHA)
   - ResolveTagCommit returns `"different-commit-sha"` (annotated tag points elsewhere)
   - FetchChangelogCallCount == 0 (short-circuits after SHA mismatch)
   - Assert: `result.Status == agentlib.AgentStatusFailed`
   - Assert: `## Review.checks.tag_exists: true`, `tag_at_expected_sha: false`, `changelog_header_rewritten: true`

   **7d. Lightweight tag SHA mismatch** (type == "commit" directly):
   - TagExists returns `"tag-sha-lightweight"`
   - ResolveTagCommit returns `"different-commit-sha"` (lightweight tag points to different commit than Result says)
   - Assert: `result.Status == agentlib.AgentStatusFailed`
   - Assert: `## Review.checks.tag_at_expected_sha: false`

   **7e. CHANGELOG still has ## Unreleased as top heading**:
   - TagExists returns `"abc123", nil`
   - ResolveTagCommit returns `"abc123", nil`
   - FetchChangelog returns `[]byte("## Unreleased\n\n- new\n\n## v0.9.0\n\n- old"), nil`
   - Assert: `result.Status == agentlib.AgentStatusFailed`
   - Assert: `## Review.checks.tag_exists: true`, `tag_at_expected_sha: true`, `changelog_header_rewritten: false`

   **7f. CHANGELOG top heading is a version (pass case)**:
   - FetchChangelog returns `[]byte("## v1.0.0\n\n- feat\n\n## Unreleased\n\n- old")` — top heading is version, not ## Unreleased
   - Assert: `result.Status == agentlib.AgentStatusDone`
   - Assert: `## Review.approved: true`

   **7g. Short-circuit: Result.outcome != "released"**:
   - Task has `## Result(outcome=failed, error_category=unknown, error="clone failed")`
   - `fakeClient.TagExistsCallCount() == 0`
   - `fakeClient.ResolveTagCommitCallCount() == 0`
   - `fakeClient.FetchChangelogCallCount() == 0`
   - Assert: `result.Status == agentlib.AgentStatusDone` AND `result.NextPhase == "done"` (terminal-completed; the execution-step failure was already escalated upstream — nothing for ai_review to verify)
   - Extract `## Review`: `approved: true`, all three `checks` keys `true` (vacuously), notes contains "nothing to verify"
   - Verify: no HTTP calls were made at all

   **7h. Malformed ## Result JSON**:
   - Task has `## Result` with `{"outcome": "released", "invalid-json`
   - Assert: `err != nil` (step returned a wrapped error)
   - Assert: `result == nil` (no result on error path)
   - Assert: no `## Review` section was added to the markdown
   - The wrapped error must contain the wrapping message (e.g. "extract ## Result section")

   **7i. Missing ## Result section**:
   - Task has no `## Result` section at all
   - Assert: `err != nil`
   - Assert: no `## Review` section

   **7j. Missing frontmatter `repo`**:
   - Task has `## Result(outcome=released)` but no `repo` in frontmatter
   - Assert: `err != nil`
   - Assert: wrapped error contains "read frontmatter repo"

   **7k. Bearer token never in error strings**:
   - `fakeClient.TagExistsReturns("", errors.New("TagExists: status 500: Server Error"))` — error does not contain token
   - Run step, get `result.Status == agentlib.AgentStatusFailed`
   - Assert: `strings.Contains(result.Message, "test-token") == false`
   - Also assert: `strings.Contains(err.Error(), "test-token") == false`

   **7l. Step does NOT write ## Failure section** (controller's job):
   - Use a failure case (tag missing)
   - After step.Run, parse the markdown and assert: `strings.Contains(fullMarkdown, "## Failure") == false`
   - The step writes `## Review` on failure, but not `## Failure`

   **7m. Name() and ShouldRun()**:
   - `step.Name() == "github-release-ai-review"`
   - `step.ShouldRun()` returns `true, nil` (always runs, idempotent overwrite)

8. Use `gomega.Eventually` or plain `Expect` with call count assertions. Counterfeiter mocks record call counts via `CallCount()` methods.

9. For extracting `## Review` in tests: use `agentlib.ExtractSection[pkg.ReviewOutput](context.Background(), md, "## Review")`.

10. Run `go generate ./...` first to produce the `mocks/review_client.go` mock.

11. External test package (`pkg_test`) — do NOT use `package pkg` for tests.

12. Add `//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate` at the top of the source file (`steps_ai_review.go`) to keep the mock regenerable alongside the other mocks.
</requirements>

<constraints>
- Tests must be in `pkg_test` package (external).
- Counterfeiter mocks from `mocks/` directory.
- Ginkgo v2 + Gomega conventions.
- Test the HTTP boundary (mock on `aiReviewClient` seam, not on `http.Client` internals).
- Test all acceptance criteria from the spec.
- Existing tests under `pkg/...` must continue to pass.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
1. `cd agent/github-releaser && go generate ./...` — must produce `mocks/review_client.go`.
2. `cd agent/github-releaser && make precommit` — must pass (project's `validationCommand` per `.dark-factory.yaml` — covers tests + lint + coverage gate as configured).
3. `cd agent/github-releaser && go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep steps_ai_review.go` — every function in `steps_ai_review.go` must show ≥80% coverage in the per-function output (the grep scopes the threshold to just this new file; do NOT assert overall package coverage, which can drift for unrelated reasons).
</verification>