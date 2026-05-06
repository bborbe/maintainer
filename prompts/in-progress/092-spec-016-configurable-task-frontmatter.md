---
status: committing
spec: [016-configurable-task-frontmatter]
summary: Added BUILD_ASSIGNEE, BUILD_TASK_STATUS, BUILD_TASK_PHASE env vars to github-build watcher; converted buildCreateTaskCommand to a method on buildWatcher; updated factory, both mains, run-once Makefile, README, and CHANGELOG; all tests pass at 85.8% coverage and make precommit exits 0.
container: maintainer-092-spec-016-configurable-task-frontmatter
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T18:00:00Z"
queued: "2026-05-06T17:52:12Z"
started: "2026-05-06T17:52:58Z"
branch: dark-factory/configurable-task-frontmatter
---

<summary>
- Operators can override the task `assignee`, `status`, and `phase` via env vars at deploy time without code changes or rebuilds
- Three new env vars added: `BUILD_ASSIGNEE` (default: `build-fixer-agent`), `BUILD_TASK_STATUS` (default: `todo`), `BUILD_TASK_PHASE` (default: empty — field omitted)
- Empty `BUILD_TASK_PHASE` produces NO `phase:` key in published task frontmatter (not `phase: ""`), preserving today's behavior
- Explicit empty `BUILD_ASSIGNEE` or `BUILD_TASK_STATUS` is rejected at startup with a validation error from the service framework
- Both the long-running watcher binary (`main.go`) and `cmd/run-once` honor all three overrides identically
- `cmd/run-once/Makefile` `run-once` target exposes all three as make variables overridable on the command line
- README env vars table includes the three new entries with their defaults
- Default behavior (no env vars set) is unchanged: tasks carry `assignee: build-fixer-agent`, `status: todo`, and no `phase:` key
- Existing tests updated to compile with the new `NewWatcher` signature; new test cases assert all three frontmatter scenarios
</summary>

<objective>
Add `BUILD_ASSIGNEE`, `BUILD_TASK_STATUS`, and `BUILD_TASK_PHASE` CLI args / env vars to the github-build watcher so operators can reconfigure published task frontmatter at deploy time without a code change. The values flow from `main.go` through `factory.CreateWatcher` into `pkg.NewWatcher`, are stored on `buildWatcher`, and `buildCreateTaskCommand` reads them from the struct receiver (no package-level state).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — factory functions must have zero business logic.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega patterns, Counterfeiter mocks, coverage ≥80%.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — constructor pattern, error wrapping.

Files to read fully before making any changes:

- `watcher/github-build/pkg/watcher.go` — full file. `NewWatcher` (line 29) and `buildWatcher` struct (line 47) need 3 new fields. `buildCreateTaskCommand` (line 229) is a standalone function with hard-coded `"build-fixer-agent"` and `"todo"` — convert to a method on `buildWatcher`. Call site is `applyStateMachine` at line 145.
- `watcher/github-build/pkg/watcher_test.go` — `makeWatcher` helper at lines 38–47 calls `pkg.NewWatcher` with the current 6-param signature. Update to 9 params. The existing test at line 71 asserts `cmd.Frontmatter["assignee"]` equals `"build-fixer-agent"` — must still pass.
- `watcher/github-build/pkg/factory/factory.go` — `CreateWatcher` (line 47) calls `pkg.NewWatcher` with 6 args. Add 3 new params, pass through. Keep factory zero-logic.
- `watcher/github-build/main.go` — `application` struct (line 36). Add 3 new fields after `RepoAllowlist`. Update `factory.CreateWatcher` call in `Run` (line 66) to pass 9 args.
- `watcher/github-build/cmd/run-once/main.go` — mirror the same 3 fields. Update the `factory.CreateWatcher` call (line 50) to pass 9 args.
- `watcher/github-build/cmd/run-once/Makefile` — add 3 make variables and pass them to `go run`.
- `watcher/github-build/README.md` — Environment Variables table (line 30). Append 3 new rows.
- `CHANGELOG.md` — root changelog, append under `## Unreleased`.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 6 for fast feedback. Run `make precommit` only at the final step.**

1. **Update `watcher/github-build/pkg/watcher.go`:**

   a. Extend `NewWatcher` to accept three new string parameters after `cursorPath`:

   ```go
   func NewWatcher(
       githubClient GitHubClient,
       publisher CommandPublisher,
       metrics Metrics,
       repoFilter filter.RepoFilter,
       allowlist []string,
       cursorPath string,
       assignee string,
       taskStatus string,
       taskPhase string,
   ) Watcher {
   ```

   b. Add the three fields to `buildWatcher`:

   ```go
   type buildWatcher struct {
       githubClient GitHubClient
       publisher    CommandPublisher
       metrics      Metrics
       repoFilter   filter.RepoFilter
       allowlist    []string
       cursorPath   string
       assignee     string
       taskStatus   string
       taskPhase    string
   }
   ```

   c. Store them in the `NewWatcher` return statement:

   ```go
   return &buildWatcher{
       githubClient: githubClient,
       publisher:    publisher,
       metrics:      metrics,
       repoFilter:   repoFilter,
       allowlist:    allowlist,
       cursorPath:   cursorPath,
       assignee:     assignee,
       taskStatus:   taskStatus,
       taskPhase:    taskPhase,
   }
   ```

   d. Convert the standalone `buildCreateTaskCommand` function into a method on `buildWatcher`. Change the signature from:

   ```go
   func buildCreateTaskCommand(
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
   ) agentlib.CreateTaskCommand {
   ```

   to:

   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
   ) agentlib.CreateTaskCommand {
   ```

   e. Inside `buildCreateTaskCommand`, replace the hard-coded frontmatter values with the struct receiver fields. Replace the `return agentlib.CreateTaskCommand{...}` block:

   ```go
   fm := agentlib.TaskFrontmatter{
       "assignee":    w.assignee,
       "repo":        owner + "/" + repo,
       "episode_sha": episodeSHA,
       "status":      w.taskStatus,
   }
   if w.taskPhase != "" {
       fm["phase"] = w.taskPhase
   }
   return agentlib.CreateTaskCommand{
       TaskIdentifier: agentlib.TaskIdentifier(taskID.String()),
       Frontmatter:    fm,
       Body:           body,
   }
   ```

   f. Update the call site in `applyStateMachine` (the line `cmd := buildCreateTaskCommand(...)`) to use the receiver:

   ```go
   cmd := w.buildCreateTaskCommand(taskID, owner, repo, episodeSHA, failingRuns)
   ```

2. **Update `watcher/github-build/pkg/factory/factory.go`:**

   Add three new string params to `CreateWatcher` (after `cursorPath`) and pass them to `pkg.NewWatcher`. The factory MUST remain zero-logic:

   ```go
   func CreateWatcher(
       ctx context.Context,
       ghToken string,
       brokers libkafka.Brokers,
       stage string,
       allowlist []string,
       cursorPath string,
       assignee string,
       taskStatus string,
       taskPhase string,
   ) (pkg.Watcher, func(), error) {
       branch := base.Branch(stage)
       pub, cleanup, err := CreateKafkaPublisher(ctx, brokers, branch)
       if err != nil {
           return nil, nil, errors.Wrap(ctx, err, "create kafka publisher")
       }
       ghClient := pkg.NewGitHubClient(ghToken)
       repoFilter := filter.RepoFilters{filter.NewRepoAllowlistFilter(allowlist)}
       w := pkg.NewWatcher(
           ghClient,
           pub,
           pkg.NewMetrics(),
           repoFilter,
           allowlist,
           cursorPath,
           assignee,
           taskStatus,
           taskPhase,
       )
       return w, cleanup, nil
   }
   ```

3. **Update `watcher/github-build/main.go`:**

   Add three new fields to the `application` struct after the existing `RepoAllowlist` field:

   ```go
   BuildAssignee   string `required:"true"  arg:"build-assignee"    env:"BUILD_ASSIGNEE"    usage:"Frontmatter assignee for published tasks"                default:"build-fixer-agent"`
   BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"BUILD_TASK_STATUS" usage:"Frontmatter status for published tasks"                  default:"todo"`
   BuildTaskPhase  string `required:"false" arg:"build-task-phase"  env:"BUILD_TASK_PHASE"  usage:"Frontmatter phase for published tasks; empty = omit field"`
   ```

   Update the `factory.CreateWatcher` call in `Run` to pass the three new fields after `"/data/cursor.json"`:

   ```go
   w, cleanup, err := factory.CreateWatcher(
       ctx,
       a.GHToken,
       a.KafkaBrokers,
       a.Stage,
       repoAllowlist,
       "/data/cursor.json",
       a.BuildAssignee,
       a.BuildTaskStatus,
       a.BuildTaskPhase,
   )
   ```

4. **Update `watcher/github-build/cmd/run-once/main.go`:**

   Add the same three fields to the `application` struct (identical to `main.go`):

   ```go
   BuildAssignee   string `required:"true"  arg:"build-assignee"    env:"BUILD_ASSIGNEE"    usage:"Frontmatter assignee for published tasks"                default:"build-fixer-agent"`
   BuildTaskStatus string `required:"true"  arg:"build-task-status" env:"BUILD_TASK_STATUS" usage:"Frontmatter status for published tasks"                  default:"todo"`
   BuildTaskPhase  string `required:"false" arg:"build-task-phase"  env:"BUILD_TASK_PHASE"  usage:"Frontmatter phase for published tasks; empty = omit field"`
   ```

   Update the `factory.CreateWatcher` call in `Run`:

   ```go
   w, cleanup, err := factory.CreateWatcher(
       ctx,
       a.GHToken,
       a.KafkaBrokers,
       a.Stage,
       repoAllowlist,
       "/data/cursor.json",
       a.BuildAssignee,
       a.BuildTaskStatus,
       a.BuildTaskPhase,
   )
   ```

5. **Update `watcher/github-build/cmd/run-once/Makefile`:**

   Add three new make variables (with defaults matching the struct tag defaults) and pass them as flags to `go run`:

   ```makefile
   BUILD_ASSIGNEE    ?= build-fixer-agent
   BUILD_TASK_STATUS ?= todo
   BUILD_TASK_PHASE  ?=

   run-once:
   	@GH_TOKEN=$$(gh auth token) go run -mod=mod . \
   		-kafka-brokers="$(KAFKA_BROKERS)" \
   		-stage=$(STAGE) \
   		-repo-allowlist="$(REPO_ALLOWLIST)" \
   		-build-assignee="$(BUILD_ASSIGNEE)" \
   		-build-task-status="$(BUILD_TASK_STATUS)" \
   		-build-task-phase="$(BUILD_TASK_PHASE)" \
   		-v=2
   ```

   Preserve the existing `include` lines and `REPO_ALLOWLIST`/`STAGE` variable declarations above the new additions.

6. **Update `watcher/github-build/pkg/watcher_test.go`:**

   a. Update the `makeWatcher` helper to pass the three new params (pass the defaults to preserve existing test behavior):

   ```go
   makeWatcher := func(allowlist []string) pkg.Watcher {
       return pkg.NewWatcher(
           ghClient,
           publisher,
           metrics,
           filter.RepoFilters{},
           allowlist,
           cursorPath,
           "build-fixer-agent",
           "todo",
           "",
       )
   }
   ```

   b. Add a new `Describe("configurable frontmatter", ...)` block at the end of the outer `Describe("Watcher", ...)`. Do NOT modify any existing test:

   ```go
   Describe("configurable frontmatter", func() {
       makeCustomWatcher := func(allowlist []string, assignee, taskStatus, taskPhase string) pkg.Watcher {
           return pkg.NewWatcher(
               ghClient,
               publisher,
               metrics,
               filter.RepoFilters{},
               allowlist,
               cursorPath,
               assignee,
               taskStatus,
               taskPhase,
           )
       }

       singleFailingRun := func(workflowID int64, sha string) []pkg.WorkflowRun {
           return []pkg.WorkflowRun{
               {
                   WorkflowID: workflowID,
                   Name:       "CI",
                   HeadSHA:    sha,
                   Conclusion: "failure",
                   HTMLURL:    "https://github.com/owner/repo/actions/runs/99",
                   CreatedAt:  time.Now(),
               },
           }
       }

       It("uses custom assignee and status when set", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleFailingRun(10, "sha-custom"), nil)

           w := makeCustomWatcher([]string{"owner/repo"}, "other-agent", "backlog", "")
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Frontmatter["assignee"]).To(Equal("other-agent"))
           Expect(cmd.Frontmatter["status"]).To(Equal("backlog"))
           Expect(cmd.Frontmatter).NotTo(HaveKey("phase"))
       })

       It("includes phase key when BUILD_TASK_PHASE is non-empty", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleFailingRun(11, "sha-phase"), nil)

           w := makeCustomWatcher([]string{"owner/repo"}, "build-fixer-agent", "todo", "planning")
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Frontmatter["phase"]).To(Equal("planning"))
       })

       It("omits phase key when BUILD_TASK_PHASE is empty string", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleFailingRun(12, "sha-nophase"), nil)

           w := makeCustomWatcher([]string{"owner/repo"}, "build-fixer-agent", "todo", "")
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Frontmatter).NotTo(HaveKey("phase"))
       })
   })
   ```

   Note: each `It` block gets a fresh `publisher` mock because `publisher = new(mocks.CommandPublisher)` is called in the outer `BeforeEach`. Call count assertions start from 0 in each block.

7. **Run `make test`** in `watcher/github-build/`:

   ```bash
   cd watcher/github-build && make test
   ```

   Fix any compile errors or test failures before proceeding to `make precommit`.

8. **Update `watcher/github-build/README.md`:**

   Append three new rows to the Environment Variables table (insert before the closing blank line after the `SENTRY_PROXY` row, or append at the end of the table body). The new rows:

   ```markdown
   | `BUILD_ASSIGNEE`    | no | `build-fixer-agent` | Frontmatter `assignee` written to published tasks; explicit empty string rejected at startup |
   | `BUILD_TASK_STATUS` | no | `todo`              | Frontmatter `status` written to published tasks; explicit empty string rejected at startup |
   | `BUILD_TASK_PHASE`  | no | (empty — omitted)   | Frontmatter `phase` written to published tasks; if empty or unset, the key is NOT written to frontmatter |
   ```

9. **Add CHANGELOG entry** to root `CHANGELOG.md` under `## Unreleased` (create the section if it does not exist):

   ```
   - feat(watcher/github-build): add BUILD_ASSIGNEE, BUILD_TASK_STATUS, BUILD_TASK_PHASE env vars so operators can override published task frontmatter at deploy time without a code change; empty BUILD_TASK_PHASE omits the phase key entirely
   ```

10. **Verify ancillary test files compile.** The following may need updates if they reference changed symbols:
    - `watcher/github-build/main_test.go` and `watcher/github-build/cmd/run-once/main_test.go` — these are `gexec.Build` compile tests; if they construct `application{}` directly and the service framework validates `required:"true"` at struct-init time, they may need defaults set in test setup. If they only build the binary, no change needed.
    - `watcher/github-build/pkg/watcher_internal_test.go` (if it exists) — if it calls `buildCreateTaskCommand` as a free function, it must now call `(&buildWatcher{...}).buildCreateTaskCommand(...)` since the function moves to a method on the struct.
    - Run `cd watcher/github-build && go build ./...` first to surface any compile breaks before `make precommit`.

11. **Run `make precommit`** in `watcher/github-build/`:

    ```bash
    cd watcher/github-build && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/` and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- `Frontmatter["phase"]` MUST be omitted (not set to empty string) when `taskPhase == ""` — `phase: ""` in YAML is observably different from no `phase` key and breaks the spec's AC
- `BuildAssignee` and `BuildTaskStatus` MUST use `required:"true"` struct tag so an explicit empty env var override (e.g. `BUILD_ASSIGNEE=`) is rejected at startup by the service framework
- `BuildTaskPhase` MUST use `required:"false"` — empty string is valid and means "omit the field"
- `buildCreateTaskCommand` MUST be a method on `buildWatcher` reading `w.assignee`, `w.taskStatus`, `w.taskPhase` — NO package-level mutable state, NO globals
- `factory.CreateWatcher` MUST pass all 3 new values directly to `pkg.NewWatcher` with no intermediate logic — factory stays zero-logic
- `cmd/run-once/main.go` MUST have the same 3 struct fields with the same defaults and required tags as `main.go`
- `cmd/run-once/Makefile` `run-once` target MUST pass all 3 new flags to `go run` so they are overridable on the make command line
- All existing tests in `pkg/watcher_test.go` MUST still pass — the `makeWatcher` helper must be updated to supply the 3 default values to the new `NewWatcher` signature
- New test coverage: custom assignee/status, non-empty phase (key present), empty phase (key absent)
- Error wrapping: `github.com/bborbe/errors`; never `fmt.Errorf`
- `make precommit` runs from `watcher/github-build/`, never at repo root
- The default deploy (no env vars set) produces today's exact output: `assignee: build-fixer-agent`, `status: todo`, no `phase` key
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm NewWatcher signature has 3 new params:
grep -A 12 "func NewWatcher" watcher/github-build/pkg/watcher.go
# Expected: assignee, taskStatus, taskPhase params visible after cursorPath

# Confirm buildWatcher stores the 3 fields:
grep -n "assignee\|taskStatus\|taskPhase" watcher/github-build/pkg/watcher.go
# Expected: field declarations in struct + assignments in constructor + reads in buildCreateTaskCommand

# Confirm phase is conditionally written (not always):
grep -n "taskPhase\|fm\[.phase.\]" watcher/github-build/pkg/watcher.go
# Expected: `if w.taskPhase != ""` guard before setting fm["phase"]

# Confirm buildCreateTaskCommand is now a method (not a standalone func):
grep -n "func.*buildWatcher.*buildCreateTaskCommand\|func buildCreateTaskCommand" watcher/github-build/pkg/watcher.go
# Expected: only the method form `func (w *buildWatcher) buildCreateTaskCommand`

# Confirm factory passes 3 new params:
grep -A 18 "func CreateWatcher" watcher/github-build/pkg/factory/factory.go
# Expected: assignee, taskStatus, taskPhase in signature + passed to pkg.NewWatcher

# Confirm no business logic added to factory:
grep -n -E "^\s*(if|for|switch)\b" watcher/github-build/pkg/factory/factory.go | grep -v "err != nil"
# Expected: zero matches

# Confirm main.go struct fields:
grep -n "BuildAssignee\|BuildTaskStatus\|BuildTaskPhase" watcher/github-build/main.go
# Expected: 3 matches with required tags, default tags, and env names

# Confirm run-once mirrors main.go:
grep -n "BuildAssignee\|BuildTaskStatus\|BuildTaskPhase" watcher/github-build/cmd/run-once/main.go
# Expected: 3 matches (identical to main.go)

# Confirm run-once Makefile exposes the 3 vars:
grep -n "BUILD_ASSIGNEE\|BUILD_TASK_STATUS\|BUILD_TASK_PHASE" watcher/github-build/cmd/run-once/Makefile
# Expected: at least 6 matches (3 variable declarations + 3 flag pass-throughs)

# Confirm README updated:
grep -n "BUILD_ASSIGNEE\|BUILD_TASK_STATUS\|BUILD_TASK_PHASE" watcher/github-build/README.md
# Expected: 3 matches in the env vars table

# Confirm new tests present:
grep -n "other-agent\|backlog\|planning\|configurable frontmatter\|nophase" watcher/github-build/pkg/watcher_test.go
# Expected: matches for each new test case

# Confirm phase-omission tests:
grep -n "NotTo(HaveKey" watcher/github-build/pkg/watcher_test.go
# Expected: at least 2 matches (empty phase → no key in two It blocks)

# Confirm CHANGELOG entry:
grep -n "BUILD_ASSIGNEE\|build-task-phase\|configurable.*frontmatter" CHANGELOG.md
# Expected: one match under ## Unreleased
</verification>
