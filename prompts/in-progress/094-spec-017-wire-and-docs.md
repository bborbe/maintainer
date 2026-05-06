---
status: committing
spec: [017-per-repo-maintenance-yaml]
summary: 'Wired maintenance loader into build watcher: NewWatcher takes 10th maintenanceLoader param, applyStateMachine loads per-repo overrides on green→red transitions only, factory creates loader from existing ghClient, tests updated with new override scenarios, .maintenance.yaml and docs added.'
container: maintainer-094-spec-017-wire-and-docs
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-06T18:10:00Z"
queued: "2026-05-06T18:40:38Z"
started: "2026-05-06T18:46:36Z"
---

<summary>
- The build watcher now fetches `.maintenance.yaml` from each repo's default branch on every `green → red` transition and applies per-repo overrides to `assignee`, `status`, and `phase`
- Non-empty values in the file's `watcher.github-build` subtree override the watcher's CLI/env defaults (`BUILD_ASSIGNEE`, `BUILD_TASK_STATUS`, `BUILD_TASK_PHASE`); empty or absent values fall through silently to the watcher defaults
- Repos without `.maintenance.yaml` (404) produce no log and behave exactly as before — full backwards compatibility
- Factory wires the maintenance loader from the same `githubClient` instance already used for workflow runs — one client, no extra connection
- Watcher tests inject a fake loader; new test cases assert all override scenarios and confirm the loader is NOT called on red→red transitions
- `bborbe/maintainer` repo gets a `.maintenance.yaml` matching the watcher defaults — no behavior change but the loader is exercised on every dev-cluster publish
- `docs/build-watcher.md` gains a "Per-Repo Configuration" section documenting the schema, precedence, and failure modes
- CHANGELOG entry and clean `make precommit` from `watcher/github-build/`
</summary>

<objective>
Wire the maintenance loader (from prompt 1) into the build watcher state machine. On every `green → red` transition, the watcher fetches `.maintenance.yaml` from the failing repo and merges its `watcher.github-build` overrides with the watcher-level CLI/env defaults before building the `CreateTaskCommand`. Repos without the file are unaffected. The factory constructs the loader internally from the existing GitHub client — no new factory parameters, no extra connections.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — factory functions must have zero business logic; constructing a concrete type from a config value IS valid factory work.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks, coverage ≥80%.

**Dependencies this prompt assumes are already in place:**

1. **Spec 016 (`configurable-task-frontmatter`) is merged.** `watcher/github-build/pkg/watcher.go` now:
   - `NewWatcher` accepts 9 parameters ending with `assignee, taskStatus, taskPhase string`
   - `buildWatcher` struct has fields `assignee`, `taskStatus`, `taskPhase string`
   - `buildCreateTaskCommand` is a METHOD `func (w *buildWatcher) buildCreateTaskCommand(taskID, owner, repo, episodeSHA, failingRuns)` that reads `w.assignee`, `w.taskStatus`, `w.taskPhase`
   - `factory.CreateWatcher` has 9 parameters ending with `assignee, taskStatus, taskPhase string`

2. **Spec 017 prompt 1 is executed.** `watcher/github-build/` now has:
   - `pkg/maintenance/loader.go` — `Loader` interface, `FileContentFetcher` interface, `GithubBuildConfig` struct (`Assignee`, `Status`, `Phase string`), `NewLoader(fetcher FileContentFetcher) Loader`
   - `pkg/mocks/maintenance_loader.go` — `*mocks.MaintenanceLoader` counterfeiter fake with `LoadOverridesReturns`, `LoadOverridesCallCount`, `LoadOverridesArgsForCall`
   - `pkg/githubclient.go` — `GitHubClient` interface now includes `GetFileContent(ctx, owner, repo, path, ref string) ([]byte, error)`; the concrete `*githubClient` satisfies `maintenance.FileContentFetcher` via structural typing

Files to read fully before making any changes:
- `watcher/github-build/pkg/watcher.go` — full file; understand current `buildWatcher` struct (lines 53–63), `NewWatcher` (line 29), `applyStateMachine` (line 141), and `buildCreateTaskCommand` method (line 238). Confirm `applyStateMachine` receives `repoState *RepoState` and that `repoState.DefaultBranch` is populated by `pollRepo` before the call (line 109–117)
- `watcher/github-build/pkg/watcher_test.go` — full file; understand `makeWatcher` helper and existing test structure; will add `maintenanceLoader` param and new test cases
- `watcher/github-build/pkg/factory/factory.go` — full file; `CreateWatcher` creates `ghClient := pkg.NewGitHubClient(ghToken)` internally; this prompt extends that to also create `maintenanceLoader := maintenance.NewLoader(ghClient)` immediately after
- `watcher/github-build/pkg/maintenance/loader.go` — full file; confirm `GithubBuildConfig` struct fields and `Loader.LoadOverrides(ctx, owner, repo, defaultBranch string) GithubBuildConfig` signature
- `watcher/github-build/pkg/mocks/maintenance_loader.go` — verify fake method names (`LoadOverridesReturns`, `LoadOverridesCallCount`, `LoadOverridesArgsForCall`)
- `docs/build-watcher.md` — full file; understand existing sections to know where to append the new "Per-Repo Configuration" section
</context>

<requirements>
**Execute steps in order. Run `make test` after step 7. Run `make precommit` only at the final step.**

1. **Update `buildCreateTaskCommand` to take explicit frontmatter parameters** in `watcher/github-build/pkg/watcher.go`.

   The method currently reads `w.assignee`, `w.taskStatus`, `w.taskPhase` from the receiver. After this change, the effective (already-merged) values are passed in explicitly so the merge logic lives in `applyStateMachine`, not here.

   Change the signature from:
   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
   ) agentlib.CreateTaskCommand {
   ```
   To:
   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
       assignee, taskStatus, taskPhase string,
   ) agentlib.CreateTaskCommand {
   ```

   Inside the method body, replace `w.assignee` → `assignee`, `w.taskStatus` → `taskStatus`, `w.taskPhase` → `taskPhase`.

2. **Add `coalesceString` helper** to `watcher/github-build/pkg/watcher.go` (private, after the other helpers at the bottom of the file):

   ```go
   // coalesceString returns the first non-empty string. Used to merge a
   // per-repo file override (a) with the watcher-level default (b).
   func coalesceString(a, b string) string {
       if a != "" {
           return a
       }
       return b
   }
   ```

3. **Add `maintenanceLoader` field to `buildWatcher` struct** in `watcher/github-build/pkg/watcher.go`:

   a. Add import:
   ```go
   "github.com/bborbe/maintainer/watcher/github-build/pkg/maintenance"
   ```

   b. Add `maintenanceLoader maintenance.Loader` as the last field in `buildWatcher`:
   ```go
   type buildWatcher struct {
       githubClient      GitHubClient
       publisher         CommandPublisher
       metrics           Metrics
       repoFilter        filter.RepoFilter
       allowlist         []string
       cursorPath        string
       assignee          string
       taskStatus        string
       taskPhase         string
       maintenanceLoader maintenance.Loader
   }
   ```

   c. Add `maintenanceLoader maintenance.Loader` as the 10th parameter to `NewWatcher` (after `taskPhase`):
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
       maintenanceLoader maintenance.Loader,
   ) Watcher {
   ```

   d. Store it in the constructor return:
   ```go
   return &buildWatcher{
       githubClient:      githubClient,
       publisher:         publisher,
       metrics:           metrics,
       repoFilter:        repoFilter,
       allowlist:         allowlist,
       cursorPath:        cursorPath,
       assignee:          assignee,
       taskStatus:        taskStatus,
       taskPhase:         taskPhase,
       maintenanceLoader: maintenanceLoader,
   }
   ```

4. **Update `applyStateMachine` in `watcher/github-build/pkg/watcher.go`** to load per-repo overrides on `green → red` transitions and pass effective values to `buildCreateTaskCommand`.

   In the `case (prevState == "" || prevState == "green") && currState == "red":` branch, replace:
   ```go
   taskID := DeriveTaskID(owner, repo, episodeSHA)
   cmd := w.buildCreateTaskCommand(taskID, owner, repo, episodeSHA, failingRuns)
   ```

   With:
   ```go
   overrides := w.maintenanceLoader.LoadOverrides(ctx, owner, repo, repoState.DefaultBranch)
   effectiveAssignee := coalesceString(overrides.Assignee, w.assignee)
   effectiveStatus := coalesceString(overrides.Status, w.taskStatus)
   effectivePhase := coalesceString(overrides.Phase, w.taskPhase)
   taskID := DeriveTaskID(owner, repo, episodeSHA)
   cmd := w.buildCreateTaskCommand(taskID, owner, repo, episodeSHA, failingRuns, effectiveAssignee, effectiveStatus, effectivePhase)
   ```

   `repoState.DefaultBranch` is guaranteed non-empty here — `pollRepo` populates it (via `GetDefaultBranch`) before calling `applyStateMachine` if the cached value is empty (see lines 109–117 of `pollRepo`).

5. **Update `watcher/github-build/pkg/factory/factory.go`** to create the maintenance loader and wire it into `NewWatcher`.

   The factory signature stays at 9 parameters (unchanged from spec 016). The factory creates ONE `ghClient` from `ghToken` and reuses it for both workflow-run fetching and maintenance config loading — no extra connections.

   a. Add import:
   ```go
   "github.com/bborbe/maintainer/watcher/github-build/pkg/maintenance"
   ```

   b. `CreateWatcher` signature is unchanged (9 params — same as spec 016):
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
   ```

   c. After `ghClient := pkg.NewGitHubClient(ghToken)`, add:
   ```go
   maintenanceLoader := maintenance.NewLoader(ghClient)
   ```

   d. Pass `maintenanceLoader` as the 10th argument to `pkg.NewWatcher`:
   ```go
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
       maintenanceLoader,
   )
   ```

   The factory keeps zero business logic — `maintenance.NewLoader(ghClient)` is pure construction, no conditionals.

6. **`watcher/github-build/main.go` and `watcher/github-build/cmd/run-once/main.go` require NO changes** to their `factory.CreateWatcher` call sites — the factory signature is unchanged at 9 params. Only verify both files compile.

   If `watcher/github-build/main_test.go` or `watcher/github-build/cmd/run-once/main_test.go` directly invoke `pkg.NewWatcher` with 9 params, update them to pass 10 (add a `new(mocks.MaintenanceLoader)` as the last arg). Run `go build ./...` to surface any compile breaks:
   ```bash
   cd watcher/github-build && go build ./...
   ```

7. **Update `watcher/github-build/pkg/watcher_test.go`** — two changes:

   a. **Update `makeWatcher` helper** to pass a `*mocks.MaintenanceLoader` (returning empty config by default — no per-repo overrides). The existing tests don't rely on per-repo overrides, so the empty default preserves all existing behavior:

   ```go
   makeWatcher := func(allowlist []string) pkg.Watcher {
       ml := new(mocks.MaintenanceLoader)
       ml.LoadOverridesReturns(maintenance.GithubBuildConfig{})
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
           ml,
       )
   }
   ```

   Add imports for:
   - `"github.com/bborbe/maintainer/watcher/github-build/pkg/maintenance"`
   - `"github.com/bborbe/maintainer/watcher/github-build/pkg/mocks"`

   (They may already be present from the GitHubClient mock usage.)

   b. **Add a new `Describe("per-repo maintenance overrides", ...)` block** at the end of the outer `Describe("Watcher", ...)`:

   ```go
   Describe("per-repo maintenance overrides", func() {
       var maintenanceLoader *mocks.MaintenanceLoader

       makeWatcherWithLoader := func(allowlist []string, loader maintenance.Loader) pkg.Watcher {
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
               loader,
           )
       }

       BeforeEach(func() {
           maintenanceLoader = new(mocks.MaintenanceLoader)
           maintenanceLoader.LoadOverridesReturns(maintenance.GithubBuildConfig{})
       })

       singleFailingRunMaint := func(sha string) []pkg.WorkflowRun {
           return []pkg.WorkflowRun{
               {
                   WorkflowID: 999,
                   Name:       "CI",
                   HeadSHA:    sha,
                   Conclusion: "failure",
                   HTMLURL:    "https://github.com/owner/repo/actions/runs/1",
                   CreatedAt:  time.Now(),
               },
           }
       }

       It("uses watcher defaults when maintenance file returns empty config", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleFailingRunMaint("sha-default"), nil)
           maintenanceLoader.LoadOverridesReturns(maintenance.GithubBuildConfig{})

           w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Frontmatter["assignee"]).To(Equal("build-fixer-agent"))
           Expect(cmd.Frontmatter["status"]).To(Equal("todo"))
           Expect(cmd.Frontmatter).NotTo(HaveKey("phase"))
       })

       It("overrides all three fields when the maintenance file provides them", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleFailingRunMaint("sha-override"), nil)
           maintenanceLoader.LoadOverridesReturns(maintenance.GithubBuildConfig{
               Assignee: "go-deps-fixer-agent",
               Status:   "backlog",
               Phase:    "planning",
           })

           w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Frontmatter["assignee"]).To(Equal("go-deps-fixer-agent"))
           Expect(cmd.Frontmatter["status"]).To(Equal("backlog"))
           Expect(cmd.Frontmatter["phase"]).To(Equal("planning"))
       })

       It("overrides only assignee; watcher defaults apply for status and phase", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleFailingRunMaint("sha-partial"), nil)
           maintenanceLoader.LoadOverridesReturns(maintenance.GithubBuildConfig{
               Assignee: "other-agent",
               // Status and Phase empty — fall through to watcher defaults
           })

           w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Frontmatter["assignee"]).To(Equal("other-agent"))
           Expect(cmd.Frontmatter["status"]).To(Equal("todo"))    // watcher default
           Expect(cmd.Frontmatter).NotTo(HaveKey("phase"))        // watcher default (empty = omit)
       })

       It("empty assignee in file falls through to watcher default", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleFailingRunMaint("sha-empty"), nil)
           maintenanceLoader.LoadOverridesReturns(maintenance.GithubBuildConfig{
               Assignee: "", // explicitly empty = treat as absent
           })

           w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
           Expect(w.Poll(ctx)).To(Succeed())

           Expect(publisher.PublishCreateCallCount()).To(Equal(1))
           _, cmd := publisher.PublishCreateArgsForCall(0)
           Expect(cmd.Frontmatter["assignee"]).To(Equal("build-fixer-agent"))
       })

       It("loader is NOT called on red→red (no wasted API call)", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           // First poll: green→red → publish, loader called once
           ghClient.GetWorkflowRunsReturns(singleFailingRunMaint("sha-red"), nil)
           w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
           Expect(w.Poll(ctx)).To(Succeed())
           callsAfterFirst := maintenanceLoader.LoadOverridesCallCount()
           Expect(callsAfterFirst).To(Equal(1))

           // Second poll: red→red → no publish, loader must NOT be called again
           Expect(w.Poll(ctx)).To(Succeed())
           Expect(maintenanceLoader.LoadOverridesCallCount()).To(Equal(callsAfterFirst))
       })

       It("loader is NOT called on green→green (steady state, no publish)", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           ghClient.GetWorkflowRunsReturns(singleSuccessRunMaint("sha-green"), nil)
           w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
           Expect(w.Poll(ctx)).To(Succeed())
           Expect(w.Poll(ctx)).To(Succeed())
           Expect(maintenanceLoader.LoadOverridesCallCount()).To(Equal(0))
           Expect(publisher.PublishCreateCallCount()).To(Equal(0))
       })

       It("loader is NOT called on red→green (clear-state path; no publish)", func() {
           ghClient.GetDefaultBranchReturns("main", nil)
           // First poll: green→red → publish, loader called once
           ghClient.GetWorkflowRunsReturns(singleFailingRunMaint("sha-red"), nil)
           w := makeWatcherWithLoader([]string{"owner/repo"}, maintenanceLoader)
           Expect(w.Poll(ctx)).To(Succeed())
           callsAfterRed := maintenanceLoader.LoadOverridesCallCount()
           Expect(callsAfterRed).To(Equal(1))

           // Second poll: red→green → cursor cleared, no publish, no loader call
           ghClient.GetWorkflowRunsReturns(singleSuccessRunMaint("sha-green"), nil)
           Expect(w.Poll(ctx)).To(Succeed())
           Expect(maintenanceLoader.LoadOverridesCallCount()).To(Equal(callsAfterRed))
           Expect(publisher.PublishCreateCallCount()).To(Equal(1)) // still 1 from first poll
       })
   })
   ```

   The three "not called" tests together pin the constraint: the maintenance loader is invoked ONLY on the publish branch (`green→red` / cold→red), never on red→red, red→green, or green→green.

8. **Run `make test`** to verify everything compiles and all tests pass:

   ```bash
   cd watcher/github-build && make test
   ```

   Fix any compile errors or test failures before proceeding.

9. **Create `.maintenance.yaml`** at the repository root (`.maintenance.yaml` — repo-relative, container working dir is the repo root):

   ```yaml
   watcher:
     github-build:
       assignee: build-fixer-agent
       status: todo
   ```

   This sets the bborbe/maintainer repo's watcher.github-build config to match the current defaults. No behavior change, but the loader path is exercised on every dev-cluster publish. Do NOT set `phase:` — empty default = field omitted.

10. **Update `docs/build-watcher.md`** — append a new section at the end of the file:

    ````markdown
    ## Per-Repo Configuration (`.maintenance.yaml`)

    Each repo can provide a `.maintenance.yaml` at its root to override the watcher's
    fleet-level defaults for its own tasks. The file is fetched fresh on every
    `green → red` transition; no caching.

    ### Schema

    ```yaml
    watcher:
      github-build:
        assignee: <string>   # overrides BUILD_ASSIGNEE env var
        status: <string>     # overrides BUILD_TASK_STATUS env var
        phase: <string>      # overrides BUILD_TASK_PHASE env var; empty = omit field
    # Future: watcher.github-pr (PR watcher reads its own subtree)
    # Future: agent.build-fixer.* (fixer agent reads its own subtree)
    ```

    All keys are optional at every level. Each maintainer service reads **only** its own
    subtree; the build watcher ignores `watcher.github-pr.*` and all `agent.*` keys.

    ### Override Precedence

    ```
    .maintenance.yaml watcher.github-build.<key>  (per-repo, highest priority)
        > BUILD_ASSIGNEE / BUILD_TASK_STATUS / BUILD_TASK_PHASE env vars  (fleet-level)
            > hard-coded fallback (build-fixer-agent / todo / <empty>)
    ```

    Precedence is **per-key**: a missing key in the file does not suppress the env-var
    default for other keys. Empty string values (`assignee: ""`) are treated identically
    to an absent key — the env-var default applies.

    ### Failure Modes

    | Trigger | Behavior |
    |---|---|
    | File absent (HTTP 404) | Silent fall-through to env defaults — the common case |
    | Malformed YAML | WARN log with parse error; publish with env defaults |
    | Valid YAML, `watcher.github-build` subtree absent | Silent fall-through; subtree isolation by design |
    | Valid YAML, unknown key inside `watcher.github-build` | INFO log "ignored unknown key"; known keys applied |
    | `assignee: ""` (explicit empty) | Same as absent — env default applies |
    | GitHub API 5xx fetching file | WARN log; publish with env defaults |
    | File > 1 MiB | Reject as malformed; WARN log; env defaults applied |

    Errors fetching `.maintenance.yaml` **never** prevent the task from being published.
    The build-status signal is more important than the routing config.

    ### Example

    To route `bborbe/myrepo`'s build failures to a Go-specific fixer agent:

    ```yaml
    # .maintenance.yaml (at repo root of bborbe/myrepo)
    watcher:
      github-build:
        assignee: go-deps-fixer-agent
        status: todo
    ```

    The next `green → red` transition on `bborbe/myrepo` publishes a task with
    `assignee: go-deps-fixer-agent` instead of the fleet default.
    ````

11. **Add CHANGELOG entry** to the root `CHANGELOG.md` under `## Unreleased`:

    ```
    - feat(watcher/github-build): per-repo .maintenance.yaml overrides — build watcher reads watcher.github-build.{assignee,status,phase} from the repo's root on each green→red transition; missing file, malformed YAML, and API errors fall through silently to watcher defaults (BUILD_ASSIGNEE / BUILD_TASK_STATUS / BUILD_TASK_PHASE)
    ```

12. **Run `make precommit`** in `watcher/github-build/`:

    ```bash
    cd watcher/github-build && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/`, `docs/build-watcher.md`, `CHANGELOG.md`, and create `.maintenance.yaml` at repo root
- Do NOT commit — dark-factory handles git
- **Dependency on spec 016:** If `NewWatcher` does NOT yet have `assignee, taskStatus, taskPhase` params, STOP and report `status: failed` with reason "spec 016 not yet merged"
- **Dependency on prompt 1:** If `pkg/maintenance/loader.go` does NOT exist, STOP and report `status: failed` with reason "spec 017 prompt 1 not yet executed"
- `factory.CreateWatcher` MUST keep its 9-parameter signature (unchanged from spec 016) — the factory creates the maintenance loader internally from the github client it already builds; NO new factory parameters
- `buildCreateTaskCommand` MUST take explicit `assignee, taskStatus, taskPhase string` parameters — it MUST NOT read `w.assignee`, `w.taskStatus`, `w.taskPhase` directly after this change; the effective values (post-merge) are passed in from `applyStateMachine`
- `LoadOverrides` is called ONLY in the `green → red` case of `applyStateMachine` — never for `red → red`, `red → green`, or `green → green` transitions (no wasted API calls)
- `coalesceString(a, b string) string` MUST return the first non-empty string (`a` if non-empty, else `b`) — this correctly implements "empty override = fall through to default"
- The factory creates ONE `ghClient := pkg.NewGitHubClient(ghToken)` and passes it to BOTH `pkg.NewWatcher` (for workflow runs) AND `maintenance.NewLoader(ghClient)` (for file contents) — single client, single token
- `main.go` and `cmd/run-once/main.go` call sites for `factory.CreateWatcher` are unchanged (still 9 params)
- `.maintenance.yaml` at repo root MUST set `assignee: build-fixer-agent` and `status: todo` (matching watcher defaults); MUST NOT set `phase:` (default is empty = omit)
- `docs/build-watcher.md` schema block MUST use `github-build:` (with hyphen) — the YAML key is `github-build` matching the service directory name
- Error wrapping: `github.com/bborbe/errors`; never `fmt.Errorf`
- All existing tests (cursor, filter, watcher, publisher, githubclient) must still pass
- Coverage ≥80% for changed packages; `make precommit` enforces the gate
- `make precommit` runs from `watcher/github-build/`, never at repo root
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm buildCreateTaskCommand takes explicit frontmatter params:
grep -A 10 "func.*buildWatcher.*buildCreateTaskCommand" watcher/github-build/pkg/watcher.go
# Expected: method signature includes assignee, taskStatus, taskPhase string params

# Confirm w.assignee / w.taskStatus / w.taskPhase not read inside buildCreateTaskCommand body:
grep -n "w\.assignee\|w\.taskStatus\|w\.taskPhase" watcher/github-build/pkg/watcher.go
# Expected: only in NewWatcher constructor assignment block (not in buildCreateTaskCommand)

# Confirm maintenanceLoader field and 10th NewWatcher param:
grep -n "maintenanceLoader" watcher/github-build/pkg/watcher.go
# Expected: field declaration + assignment in NewWatcher + call in applyStateMachine

grep -A 13 "func NewWatcher" watcher/github-build/pkg/watcher.go
# Expected: 10 parameters, last one is maintenanceLoader maintenance.Loader

# Confirm coalesceString helper exists:
grep -n "func coalesceString" watcher/github-build/pkg/watcher.go
# Expected: one match

# Confirm LoadOverrides called only in green→red case:
grep -n "LoadOverrides" watcher/github-build/pkg/watcher.go
# Expected: exactly 1 call site

# Confirm factory signature is unchanged at 9 params (no maintenanceLoader param):
grep -A 11 "func CreateWatcher" watcher/github-build/pkg/factory/factory.go
# Expected: still 9 parameters ending with taskPhase string

# Confirm factory creates loader from ghClient (single client):
grep -n "maintenance.NewLoader\|NewGitHubClient" watcher/github-build/pkg/factory/factory.go
# Expected: both present; NewLoader called with ghClient

# Confirm factory has zero business logic (no if/for except err propagation):
grep -n -E "^\s*(if|for|switch)\b" watcher/github-build/pkg/factory/factory.go | grep -v "err != nil"
# Expected: zero matches

# Confirm main.go factory call unchanged (still 9 params — no new args):
grep -A 10 "factory.CreateWatcher" watcher/github-build/main.go
# Expected: 9 args (same as before spec 017 prompt 2)

# Confirm cmd/run-once unchanged:
grep -A 10 "factory.CreateWatcher" watcher/github-build/cmd/run-once/main.go
# Expected: 9 args

# Confirm watcher tests updated:
grep -n "MaintenanceLoader\|LoadOverridesReturns\|maintenance.GithubBuildConfig" watcher/github-build/pkg/watcher_test.go
# Expected: at least 3 matches

# Confirm loader NOT called on red→red:
grep -n "loader is NOT called\|LoadOverridesCallCount" watcher/github-build/pkg/watcher_test.go
# Expected: test that asserts LoadOverridesCallCount stays at 1 after second poll

# Confirm .maintenance.yaml at repo root:
cat .maintenance.yaml
# Expected: watcher: github-build: assignee: build-fixer-agent, status: todo

# Confirm docs updated with new section:
grep -n "Per-Repo Configuration\|\.maintenance\.yaml\|Override Precedence\|Failure Modes" docs/build-watcher.md
# Expected: all four headings present

# Confirm schema uses "github-build" (with hyphen):
grep -n "github-build:" docs/build-watcher.md
# Expected: at least 2 matches (schema block + example block)

# Confirm CHANGELOG entry:
grep -n "maintenance.yaml\|per-repo" CHANGELOG.md
# Expected: one match under ## Unreleased
</verification>
