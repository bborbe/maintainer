---
status: completed
summary: Added Ginkgo/Gomega tests for reviewStep.Name, ShouldRun, and Run in pkg/steps with a generated ClaudeRunnerMock counterfeiter mock, achieving 93.3% statement coverage.
container: code-reviewer-063-review-agent-pr-reviewer-review-step-test-coverage
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T12:00:00Z"
queued: "2026-04-28T15:36:26Z"
started: "2026-04-28T18:29:55Z"
completed: "2026-04-28T18:33:44Z"
---

<summary>
- The custom agent step that runs the AI review has zero test coverage for its three exported methods
- The `ShouldRun` method can be tested today without any mock — it only reads the markdown content
- The `Run` method requires an injectable Claude runner, which must be verified in the `bborbe/agent/lib` module or generated before tests can be written
- The `Name` method is trivially testable (pure string return)
- Covering these methods closes the most significant test gap in the service: the step that runs the AI review is currently completely unobservable in tests
</summary>

<objective>
Add Ginkgo/Gomega tests for `reviewStep.Name()`, `reviewStep.ShouldRun()`, and `reviewStep.Run()` in `pkg/steps/`. The `ShouldRun` and `Name` tests require no mocks. The `Run` tests require a Counterfeiter mock for `claudelib.ClaudeRunner` — generate it if it does not already exist. After this fix the core step of the review pipeline has ≥80% statement coverage.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Verified facts:
- `claudelib.ClaudeRunner` interface is `github.com/bborbe/agent/lib/claude.ClaudeRunner` with single method `Run(ctx context.Context, prompt string) (*ClaudeResult, error)`.
- `reviewStep.ShouldRun(_ context.Context, md *agentlib.Markdown) (bool, error)` — takes `*agentlib.Markdown`, NOT a `string`.
- `reviewStep.Run` returns `*agentlib.Result` with fields `Status` (e.g. `agentlib.AgentStatusFailed`, `agentlib.AgentStatusDone`) and `NextPhase` (string: `"done"` or `"human_review"`).
- Mocks live in `agent/pr-reviewer/mocks/` (existing dir with `bitbucket-client.go`, `config-loader.go`, `github-client.go`, `mocks.go`, `reviewer.go`, `worktree-manager.go` — no `ClaudeRunner` yet).
- The exported helper `ExtractVerdictForTest` exists in `export_test.go`; verdict-parsing tests are NOT in scope for this prompt.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/steps/review.go` — `reviewStep` struct, `NewReviewStep`, `Name`, `ShouldRun`, `Run`
- `agent/pr-reviewer/pkg/steps/review_test.go` — existing test coverage (do not duplicate verdict-parsing tests)
- `agent/pr-reviewer/pkg/steps/export_test.go` — exported symbols for white-box testing
- `agent/pr-reviewer/pkg/steps/steps_suite_test.go` — suite setup (counterfeiter generate directive)
</context>

<requirements>
1. **Generate `ClaudeRunner` mock** — no mock exists yet in `agent/pr-reviewer/mocks/`. Add a `//counterfeiter:generate` directive to a new file `agent/pr-reviewer/pkg/steps/mocks.go` (or in `export_test.go`). The interface lives at `github.com/bborbe/agent/lib/claude.ClaudeRunner`:
   ```go
   //counterfeiter:generate -o ../../mocks/claude-runner.go --fake-name ClaudeRunner github.com/bborbe/agent/lib/claude.ClaudeRunner
   ```
   Use a fake-name that does NOT collide with the real interface — e.g. `--fake-name ClaudeRunnerMock` if `ClaudeRunner` collides. Then:
   ```bash
   cd agent/pr-reviewer && go generate ./pkg/steps/...
   ```

3. **Add test cases to `pkg/steps/review_test.go`** (external package `steps_test`, Ginkgo/Gomega):

   **`Name()` test:**
   ```go
   Describe("Name", func() {
       It("returns the step name", func() {
           step := steps.NewReviewStep(/* inject deps */)
           Expect(step.Name()).To(Equal("pr-ai-review"))
       })
   })
   ```

   **`ShouldRun()` tests — `ShouldRun` takes `*agentlib.Markdown`, not a string:**
   ```go
   Describe("ShouldRun", func() {
       DescribeTable("decides based on existing ## Verdict section",
           func(content string, expected bool) {
               md, err := agentlib.NewMarkdown(content) // grep-verify constructor name
               Expect(err).NotTo(HaveOccurred())
               step := steps.NewReviewStep(/* deps */)
               result, err := step.ShouldRun(ctx, md)
               Expect(err).NotTo(HaveOccurred())
               Expect(result).To(Equal(expected))
           },
           Entry("no verdict section", "# PR Review\n\nsome text", true),
           Entry("verdict section present", "# PR Review\n\n## Verdict\n\npass", false),
           Entry("empty content", "", true),
       )
   })
   ```
   Grep-verify the actual `agentlib.Markdown` constructor before writing — it may be `agentlib.ParseMarkdown(ctx, content)` or `agentlib.NewMarkdown(content)`.

   **`Run()` tests — require `ClaudeRunner` mock:**
   - **Claude runner returns error** → `Run` should return `agentlib.AgentStatusFailed` result, NOT propagate error
   - **Claude runner returns unparseable output** → `Run` returns `AgentStatusDone` with `NextPhase = "human_review"`
   - **Claude runner returns `verdict: pass`** → `Run` returns `AgentStatusDone` with `NextPhase = "done"` (or appropriate terminal phase)
   - **Claude runner returns `verdict: fail`** → `Run` returns `AgentStatusDone` with `NextPhase = "human_review"`
   - Use the Counterfeiter mock for `ClaudeRunner` — call `.RunReturns(...)` or `.RunStub = ...` per the counterfeiter API

4. **Verified phase constants** (no need to re-grep): `agentlib.AgentStatusFailed`, `agentlib.AgentStatusDone`, and `NextPhase` is a string-typed field on `agentlib.Result`. Values used in `review.go`: `"done"`, `"human_review"`.

5. **Check coverage after adding tests**:
   ```bash
   cd agent/pr-reviewer && go test -coverprofile=/tmp/cover.out ./pkg/steps/... \
     && go tool cover -func=/tmp/cover.out | grep steps
   ```
   Coverage for `pkg/steps` should reach ≥80%.

6. **Run `cd agent/pr-reviewer && make test`** — must pass.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use Ginkgo v2 / Gomega conventions — `DescribeTable`, `Entry`, `BeforeEach`, `context.Background()` in test setup
- Counterfeiter mocks generated via `go generate`, not hand-written
- Grep-verify all `agentlib.*` symbols before writing struct literals or calling methods
- If `claudelib.ClaudeRunner` interface does not exist (grep finds nothing), report `status: failed` with a note about the missing interface — do NOT invent the interface
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
