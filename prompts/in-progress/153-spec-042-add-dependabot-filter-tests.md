---
status: approved
spec: [042-github-build-watcher-filter-dependabot-graph-update]
created: "2026-05-24T21:30:00Z"
queued: "2026-05-24T20:37:11Z"
branch: dark-factory/github-build-watcher-filter-dependabot-graph-update
---

<summary>
- Add unit tests for the Dependabot graph-update workflow filter covering: pure Dependabot (zero tasks), mixed real CI + Dependabot (one task, correct workflow name), Dependabot Updates variant, case sensitivity guard (lowercase does not match), and nil/empty name guard (safe, treated as non-matching)
- Tests use the existing `watcher_test.go` infrastructure pattern with `GitHubClient` mock and `TaskCreateCommandSender` mock
- Tests follow Ginkgo v2 + Gomega style already established in the codebase
</summary>

<objective>
Add unit tests in `watcher/github-build/pkg/watcher_test.go` to verify the Dependabot graph-update filter works end-to-end through the watcher's Poll cycle, confirming that Dependabot internal workflows never produce a CreateTaskCommand.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read the `coding` plugin's `go-testing-guide.md` (Ginkgo v2 / Gomega patterns; the existing `watcher_test.go` is the in-repo reference for the style).

Files to read fully before making changes:
- `watcher/github-build/pkg/watcher.go` — confirm `DependabotGraphUpdatePrefixes` and `isDependabotGraphUpdateWorkflow` exist from prompt 1
- `watcher/github-build/pkg/watcher_test.go` — understand the `makeWatcher` helper, `ghClient` mock setup, and `createSender` mock assertions

Key facts from the codebase:
- `WorkflowRun.Name` is a plain `string` (not a pointer) — empty string is `""`, not nil
- The `ghClient.GetWorkflowRunsReturns` mock accepts `[]pkg.WorkflowRun`
- Tests use `Describe("Poll", func() { ... })` with `Context` blocks for test cases
- `makeWatcher` helper returns `pkg.Watcher` with a `StaticSnapshot` of the allowlist
</context>

<requirements>

**Execute steps in order. Run `make test` after step 2. Run `make precommit` only at the final step.**

1. **Confirm prompt 1 is deployed** — read `watcher/github-build/pkg/watcher.go` and verify:
   - `DependabotGraphUpdatePrefixes` var exists (around line ~27)
   - `isDependabotGraphUpdateWorkflow` function exists (around line ~31)
   
   If either is missing, STOP and report `status: failed` with `"Dependabot filter not yet deployed (prompt 1)"`.

2. **Add test cases to `watcher/github-build/pkg/watcher_test.go`**

   Add a new `Describe("Dependabot graph-update workflow filter", func() { ... })` block inside the existing `Describe("Poll", func() { ... })` block, or as a sibling `Describe` block at the same level as the other `Describe` blocks (e.g. alongside `Describe("configurable frontmatter")`).

   The test block should use the same setup pattern as other `Poll` tests:
   - `ghClient.GetDefaultBranchReturns("main", nil)` in `BeforeEach`
   - `ghClient.GetWorkflowRunsReturns(...)` to control the returned workflow runs
   - `createSender.SendCommandCallCount()` to assert task emission

   Add the following `Context` blocks:

   ```go
   Describe("Dependabot graph-update workflow filter", func() {
       var t0 time.Time

       BeforeEach(func() {
           t0 = time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
           ghClient.GetDefaultBranchReturns("main", nil)
       })

       Context("pure Dependabot case: Graph Update: go_modules", func() {
           It("emits zero CreateTaskCommands (workflow filtered)", func() {
               ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
                   {
                       WorkflowID: 1,
                       Name:       "Graph Update: go_modules",
                       HeadSHA:    "sha-dep",
                       Conclusion: "failure",
                       HTMLURL:    "https://github.com/owner/repo/actions/runs/99",
                       CreatedAt:  t0,
                   },
               }, nil)

               w := makeWatcher([]string{"owner/repo"})
               Expect(w.Poll(ctx)).To(Succeed())

               // Zero tasks — the Dependabot workflow is filtered out
               Expect(createSender.SendCommandCallCount()).To(Equal(0))
           })
       })

       Context("pure Dependabot case: Dependabot Updates", func() {
           It("emits zero CreateTaskCommands (workflow filtered)", func() {
               ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
                   {
                       WorkflowID: 2,
                       Name:       "Dependabot Updates",
                       HeadSHA:    "sha-dep2",
                       Conclusion: "failure",
                       HTMLURL:    "https://github.com/owner/repo/actions/runs/88",
                       CreatedAt:  t0,
                   },
               }, nil)

               w := makeWatcher([]string{"owner/repo"})
               Expect(w.Poll(ctx)).To(Succeed())

               Expect(createSender.SendCommandCallCount()).To(Equal(0))
           })
       })

       Context("mixed case: real CI fails alongside Graph Update: go_modules", func() {
           It("emits exactly one CreateTaskCommand and the body references the CI workflow, not the Dependabot one", func() {
               ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
                   {
                       WorkflowID: 1,
                       Name:       "Graph Update: go_modules",
                       HeadSHA:    "sha-ci",
                       Conclusion: "failure",
                       HTMLURL:    "https://github.com/owner/repo/actions/runs/99",
                       CreatedAt:  t0,
                   },
                   {
                       WorkflowID: 2,
                       Name:       "CI",
                       HeadSHA:    "sha-ci",
                       Conclusion: "failure",
                       HTMLURL:    "https://github.com/owner/repo/actions/runs/1",
                       CreatedAt:  t0.Add(time.Second),
                   },
               }, nil)

               w := makeWatcher([]string{"owner/repo"})
               Expect(w.Poll(ctx)).To(Succeed())

               Expect(createSender.SendCommandCallCount()).To(Equal(1))
               _, cmd := createSender.SendCommandArgsForCall(0)
               Expect(cmd.Frontmatter["repo"]).To(Equal("owner/repo"))

               // CRITICAL: the emitted task must derive from the CI workflow, NOT the Dependabot one.
               // Assert the task body / title references "CI" and does NOT reference "Graph Update".
               // The exact field carrying the workflow name depends on the command body shape;
               // pick whichever observable field on `cmd` carries the originating workflow run's URL or name.
               // E.g. if there is `cmd.Body` containing the task markdown, check it does not contain "Graph Update".
               taskBody := cmd.Body // adjust if the field name differs — verify by reading the publisher's existing test
               Expect(taskBody).NotTo(ContainSubstring("Graph Update"))
               Expect(taskBody).To(ContainSubstring("CI"))
           })
       })

       Context("case sensitivity guard: lowercase graph update: x", func() {
           It("emits one CreateTaskCommand (lowercase does not match filter)", func() {
               ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
                   {
                       WorkflowID: 1,
                       Name:       "graph update: x",
                       HeadSHA:    "sha-lower",
                       Conclusion: "failure",
                       HTMLURL:    "https://github.com/owner/repo/actions/runs/1",
                       CreatedAt:  t0,
                   },
               }, nil)

               w := makeWatcher([]string{"owner/repo"})
               Expect(w.Poll(ctx)).To(Succeed())

               Expect(createSender.SendCommandCallCount()).To(Equal(1))
           })
       })

       Context("nil/empty name guard: empty string workflow name", func() {
           It("emits one CreateTaskCommand (empty name is non-matching, does not crash)", func() {
               ghClient.GetWorkflowRunsReturns([]pkg.WorkflowRun{
                   {
                       WorkflowID: 1,
                       Name:       "",
                       HeadSHA:    "sha-empty",
                       Conclusion: "failure",
                       HTMLURL:    "https://github.com/owner/repo/actions/runs/1",
                       CreatedAt:  t0,
                   },
               }, nil)

               w := makeWatcher([]string{"owner/repo"})
               Expect(w.Poll(ctx)).To(Succeed())

               Expect(createSender.SendCommandCallCount()).To(Equal(1))
           })
       })
   })
   ```

3. **Run `make test`**:

   ```bash
   cd watcher/github-build && make test
   ```

   All tests must pass. If any fail, diagnose and fix before proceeding.

4. **Run `make precommit`**:

   ```bash
   cd watcher/github-build && make precommit
   ```

</requirements>

<constraints>
- Only edit `watcher/github-build/pkg/watcher_test.go`
- Do NOT commit — dark-factory handles git
- Tests must be added to the existing Ginkgo suite — do NOT create a new `_test.go` file
- Use `pkg.WorkflowRun` with the exact field names from `pkg/githubclient.go`
- `make precommit` runs from `watcher/github-build/`, never at repo root
- All existing tests must continue to pass
- The `makeWatcher` helper is reused — confirm it is in scope in the test file
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm test block exists (use grep -E for alternation):
grep -cE "Graph Update:|Dependabot Updates|empty name|lowercase|mixed case" watcher/github-build/pkg/watcher_test.go
# Expected: ≥5 lines

# Confirm zero-command assertions for pure Dependabot cases (tight match on Equal(0)):
grep -c "SendCommandCallCount()).To(Equal(0))" watcher/github-build/pkg/watcher_test.go
# Expected: ≥2 lines (Graph Update + Dependabot Updates)

# Confirm one-command assertions for case-sensitivity, empty-name, and mixed cases:
grep -c "SendCommandCallCount()).To(Equal(1))" watcher/github-build/pkg/watcher_test.go
# Expected: ≥3 lines (lowercase, empty, mixed)

# Confirm mixed-case test verifies CI not Dependabot drives the command:
grep -E 'NotTo\(ContainSubstring\("Graph Update"\)\)' watcher/github-build/pkg/watcher_test.go
# Expected: ≥1 line
</verification>