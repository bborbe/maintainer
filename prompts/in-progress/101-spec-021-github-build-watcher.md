---
status: approved
spec: [021-migrate-watchers-to-task-create-command-sender]
created: "2026-05-07T20:30:00Z"
queued: "2026-05-07T20:17:53Z"
branch: dark-factory/migrate-watchers-to-task-create-command-sender
---

<summary>
- The `WatcherCreateTaskCommand` wrapper type and the hand-rolled `CommandPublisher` / `kafkaPublisher` are removed from the github-build watcher
- The watcher now receives an injected `task.CreateCommandSender` from `github.com/bborbe/agent/lib/command/task`
- The slug helpers (`computeFilenameHint`, `slugifySegment`) are kept byte-identical; their result now becomes `task.CreateCommand.Title` instead of `WatcherCreateTaskCommand.FilenameHint`
- The wire format changes: the `filename_hint` JSON key is gone; `"title"` carries the human-readable filename stem
- `github.com/bborbe/agent/lib` is bumped from `v0.57.0` to `v0.58.0` in `watcher/github-build/go.mod`
- The factory constructs a `task.CreateCommandSender` from the existing `cdb.CommandObjectSender` and injects it into the watcher
- Existing tests are updated to use counterfeiter mocks from `agent/lib/command/task/mocks/` instead of the local `mocks.CommandPublisher`
- A wire-format unit test locks `"title"` into the JSON output and asserts `"filename_hint"` is absent
- `make precommit` passes clean in `watcher/github-build/`
</summary>

<objective>
Migrate the github-build watcher's create-task publish path to use the typed `task.CreateCommandSender` from `agent/lib/command/task`, replacing the hand-rolled `WatcherCreateTaskCommand` wrapper and `kafkaPublisher`. The slug that was `FilenameHint` becomes `Title` on `task.CreateCommand`. No slug logic changes.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, counterfeiter mocks, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — factory has zero business logic, `Create*` prefix.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/` — inject interfaces, never call package functions directly.

**Files to read fully before making any changes:**
- `watcher/github-build/go.mod` — understand current `agent/lib` version to bump
- `watcher/github-build/pkg/publisher.go` — full file; understand `WatcherCreateTaskCommand`, `CommandPublisher`, `kafkaPublisher`, `marshalEvent`, `buildCommandObject` — all removed
- `watcher/github-build/pkg/watcher.go` — full file; understand `buildCreateTaskCommand` (currently returns `WatcherCreateTaskCommand`) and `applyStateMachine` (calls `w.publisher.PublishCreate(ctx, cmd)`) — both change
- `watcher/github-build/pkg/watcher_test.go` — full file; understand all `It` blocks that call `publisher.PublishCreateArgsForCall(0)` and assert `cmd.FilenameHint` — these change
- `watcher/github-build/pkg/publisher_test.go` — full file; all tests reference `pkg.WatcherCreateTaskCommand` and `pkg.NewCommandPublisher` — entire file deleted
- `watcher/github-build/pkg/watcher_internal_test.go` — full file; the `WatcherCreateTaskCommand JSON marshalling` describe block must be replaced with a wire-format test for `task.CreateCommand`
- `watcher/github-build/pkg/factory/factory.go` — full file; understand `CreateKafkaPublisher` and `CreateWatcher` — both change
- `watcher/github-build/pkg/mocks/command_publisher.go` — understand counterfeiter shape; deleted

**Step 0 — Bump dep and fetch module source:**

```bash
cd watcher/github-build && go get github.com/bborbe/agent/lib@v0.58.0 && go mod tidy
```

If `go get` fails (module not yet reachable), STOP and report `status: failed` with reason "agent/lib v0.58.0 not reachable from go proxy".

**Step 0b — Grep-verify ALL symbols before writing any code:**

```bash
AGENTLIB=$(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.58.0

# Confirm command/task package directory exists
ls "$AGENTLIB/command/task/"

# Confirm task.CreateCommand struct and its Title field
grep -A 10 "type CreateCommand struct" "$AGENTLIB/command/task/"*.go 2>/dev/null

# Confirm task.CreateCommandSender interface and its method(s)
grep -A 5 "type CreateCommandSender interface" "$AGENTLIB/command/task/"*.go 2>/dev/null

# Confirm task.NewCreateCommandSender constructor
grep -n "func NewCreateCommandSender" "$AGENTLIB/command/task/"*.go 2>/dev/null

# Confirm mocks directory and shipped mock files
ls "$AGENTLIB/command/task/mocks/" 2>/dev/null

# Confirm counterfeiter mock for CreateCommandSender
grep -n "func.*CreateCommandSender\b\|type.*CreateCommandSender\b" \
  "$AGENTLIB/command/task/mocks/"*.go 2>/dev/null | head -10

# Confirm method stub names in mocks (needed for test assertions)
grep -n "func.*Stub\|func.*CallCount\|func.*ArgsForCall\|func.*Returns" \
  "$AGENTLIB/command/task/mocks/"*.go 2>/dev/null | head -30

# Confirm task.CreateCommand JSON tags (title vs filename_hint)
grep -n '"title"\|"filename_hint"' "$AGENTLIB/command/task/"*.go 2>/dev/null

# Confirm task.CreateCommand has TaskIdentifier and Frontmatter fields (name + type)
grep -n "TaskIdentifier\|Frontmatter\|Body " "$AGENTLIB/command/task/"*.go 2>/dev/null | head -20
```

Document any deviation from expectations in `## Improvements`. Adapt all subsequent steps to the actual types/method names found.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 6. Run `make precommit` only at the final step.**

1. **Bump `github.com/bborbe/agent/lib` in `watcher/github-build/go.mod`** (already done in Step 0 above via `go get`). Confirm the `require` line reads `v0.58.0` or higher.

2. **Delete `watcher/github-build/pkg/mocks/command_publisher.go`**

   ```bash
   rm watcher/github-build/pkg/mocks/command_publisher.go
   ```

3. **Replace `watcher/github-build/pkg/publisher.go`** — remove all types and functions. Keep only the copyright header and package declaration:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg
   ```

   Symbols removed: `WatcherCreateTaskCommand`, `CommandPublisher`, `NewCommandPublisher`, `kafkaPublisher`, `marshalEvent`, `buildCommandObject`.

4. **Delete `watcher/github-build/pkg/publisher_test.go`**

   ```bash
   rm watcher/github-build/pkg/publisher_test.go
   ```

5. **Update `watcher/github-build/pkg/watcher.go`**

   a. **Update the struct fields** — replace `publisher CommandPublisher` with `createSender task.CreateCommandSender`:

   ```go
   type buildWatcher struct {
       githubClient      GitHubClient
       createSender      task.CreateCommandSender
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

   b. **Update `NewWatcher`** — replace `publisher CommandPublisher` with `createSender task.CreateCommandSender`:

   ```go
   func NewWatcher(
       githubClient GitHubClient,
       createSender task.CreateCommandSender,
       metrics Metrics,
       repoFilter filter.RepoFilter,
       allowlist []string,
       cursorPath string,
       assignee string,
       taskStatus string,
       taskPhase string,
       maintenanceLoader maintenance.Loader,
   ) Watcher {
       return &buildWatcher{
           githubClient:      githubClient,
           createSender:      createSender,
           metrics:           metrics,
           repoFilter:        repoFilter,
           allowlist:         allowlist,
           cursorPath:        cursorPath,
           assignee:          assignee,
           taskStatus:        taskStatus,
           taskPhase:         taskPhase,
           maintenanceLoader: maintenanceLoader,
       }
   }
   ```

   c. **Update `buildCreateTaskCommand`** — change return type from `WatcherCreateTaskCommand` to `task.CreateCommand`. Replace the `return WatcherCreateTaskCommand{...}` at the end with a `task.CreateCommand` literal. Move the `FilenameHint` value to the `Title` field:

   ```go
   func (w *buildWatcher) buildCreateTaskCommand(
       ctx context.Context,
       taskID uuid.UUID,
       owner, repo, episodeSHA string,
       failingRuns []WorkflowRun,
       assignee, taskStatus, taskPhase string,
       includeLogs bool,
   ) task.CreateCommand {
       // ... (body unchanged) ...
       return task.CreateCommand{
           Title:          computeFilenameHint("github", owner, repo, episodeSHA),
           TaskIdentifier: agentlib.TaskIdentifier(taskID.String()),
           Frontmatter:    fm,
           Body:           body,
       }
   }
   ```

   Verify the exact field names (`Title`, `TaskIdentifier`, `Frontmatter`, `Body`) from Step 0b before writing.

   If `task.CreateCommand` uses different field types than `agentlib.TaskIdentifier` / `agentlib.TaskFrontmatter`, adjust the construction accordingly and document in `## Improvements`.

   d. **Update `applyStateMachine`** — replace `w.publisher.PublishCreate(ctx, cmd)` with the typed sender call (use the actual method name from Step 0b):

   ```go
   if err := w.createSender.SendCommand(ctx, cmd); err != nil {
       glog.Errorf("publish create-task failed repo=%s err=%v", repoKey, err)
       w.metrics.IncPollError("kafka_error")
       return
   }
   ```

   e. **Update imports** in `watcher.go`:
   - Add `task "github.com/bborbe/agent/lib/command/task"`
   - Keep `agentlib "github.com/bborbe/agent/lib"` if `agentlib.TaskIdentifier` or `agentlib.TaskFrontmatter` are still used in `buildCreateTaskCommand` or elsewhere in the file
   - Remove `"github.com/bborbe/cqrs/base"`, `"github.com/bborbe/cqrs/cdb"` if they were only in the publisher (check — they are NOT in watcher.go currently, only in publisher.go)

6. **Update `watcher/github-build/pkg/factory/factory.go`**

   a. Replace `CreateKafkaPublisher` with `CreateKafkaCreateSender` that returns a `task.CreateCommandSender`:

   ```go
   // CreateKafkaCreateSender constructs a typed create-task command sender backed by a Kafka sync producer.
   func CreateKafkaCreateSender(
       ctx context.Context,
       brokers libkafka.Brokers,
       branch base.Branch,
   ) (task.CreateCommandSender, func(), error) {
       syncProducer, err := libkafka.NewSyncProducerWithName(
           ctx,
           brokers,
           "maintainer-watcher-github-build",
       )
       if err != nil {
           return nil, nil, errors.Wrap(ctx, err, "create sync producer")
       }
       sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
       cleanup := func() {
           if err := syncProducer.Close(); err != nil {
               glog.Warningf("close kafka sync producer: %v", err)
           }
       }
       return task.NewCreateCommandSender(sender), cleanup, nil
   }
   ```

   b. Update `CreateWatcher` to call `CreateKafkaCreateSender` and pass the sender to `pkg.NewWatcher`:

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
       createSender, cleanup, err := CreateKafkaCreateSender(ctx, brokers, branch)
       if err != nil {
           return nil, nil, errors.Wrap(ctx, err, "create kafka create sender")
       }
       ghClient := pkg.NewGitHubClient(ghToken)
       maintenanceLoader := maintenance.NewLoader(ghClient)
       repoFilter := filter.RepoFilters{filter.NewRepoAllowlistFilter(allowlist)}
       w := pkg.NewWatcher(
           ghClient,
           createSender,
           pkg.NewMetrics(),
           repoFilter,
           allowlist,
           cursorPath,
           assignee,
           taskStatus,
           taskPhase,
           maintenanceLoader,
       )
       return w, cleanup, nil
   }
   ```

   c. **Update imports** in `factory.go`:
   - Add `task "github.com/bborbe/agent/lib/command/task"`
   - Remove `agentlib "github.com/bborbe/agent/lib"` if no longer used in factory (it was not used before either)

7. **Run `make test`**:

   ```bash
   cd watcher/github-build && make test
   ```

   If `NewWatcher` callers in tests fail (wrong argument count/types), fix before proceeding.

8. **Update `watcher/github-build/pkg/watcher_test.go`**

   a. Replace `*mocks.CommandPublisher` with the typed mock from `agent/lib/command/task/mocks`. Use the exact mock struct name found in Step 0b:

   ```go
   // Old:
   publisher *mocks.CommandPublisher
   // New (adjust to exact struct name):
   createSender *taskmocks.TaskCreateCommandSender
   ```

   Add the import:
   ```go
   taskmocks "github.com/bborbe/agent/lib/command/task/mocks"
   ```

   Remove the local `"github.com/bborbe/maintainer/watcher/github-build/pkg/mocks"` import if it is no longer used for `CommandPublisher`.

   b. Update `BeforeEach`:
   ```go
   createSender = new(taskmocks.TaskCreateCommandSender)
   ```

   c. Update `makeWatcher` helper:
   ```go
   makeWatcher := func(allowlist []string) pkg.Watcher {
       ml := new(mocks.MaintenanceLoader)
       ml.LoadOverridesReturns(maintenance.GithubBuildConfig{})
       return pkg.NewWatcher(
           ghClient,
           createSender,
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

   d. In each `It` block that previously called `publisher.PublishCreateArgsForCall(0)`:
   - Replace `publisher.PublishCreateCallCount()` with `createSender.SendCommandCallCount()`
   - Replace `publisher.PublishCreateArgsForCall(0)` with `createSender.SendCommandArgsForCall(0)`
   - The `cmd` is now `task.CreateCommand`
   - Replace every `cmd.FilenameHint` assertion with `cmd.Title` — value stays the same:
     ```go
     // Old:
     Expect(cmd.FilenameHint).To(Equal("Build Failure github - owner-repo - sha-abc"))
     // New:
     Expect(cmd.Title).To(Equal("Build Failure github - owner-repo - sha-abc"))
     ```
   - Keep `cmd.TaskIdentifier`, `cmd.Frontmatter`, `cmd.Body` assertions unchanged (field names should match if `task.CreateCommand` mirrors the old shape minus `FilenameHint`)

   e. `include_logs` tests (the `Describe("include_logs opt-in", ...)` block) use `publisher.PublishCreateCallCount()` and `publisher.PublishCreateArgsForCall(0)` — update those too.

9. **Update `watcher/github-build/pkg/watcher_internal_test.go`**

   Replace the `WatcherCreateTaskCommand JSON marshalling` describe block:

   Remove:
   ```go
   // JSON marshal contract: lock the wire-format tag for the controller boundary.
   var _ = Describe("WatcherCreateTaskCommand JSON marshalling", func() { ... })
   ```

   Replace with:
   ```go
   // Wire-format contract: lock "title" in and "filename_hint" out.
   var _ = Describe("task.CreateCommand wire format", func() {
       It("emits 'title' as the top-level key (not 'filename_hint')", func() {
           cmd := task.CreateCommand{
               Title: "Build Failure github - bborbe-maintainer - 5886450",
               // populate other fields as needed based on actual struct shape from Step 0b
           }
           raw, err := json.Marshal(cmd)
           Expect(err).NotTo(HaveOccurred())
           Expect(string(raw)).To(ContainSubstring(`"title":"Build Failure github - bborbe-maintainer - 5886450"`))
           Expect(string(raw)).NotTo(ContainSubstring(`"filename_hint"`))
       })

       // Boundary contract: slug helper output MUST pass task.CreateCommand.Validate (level-1 contract test).
       // Prevents future drift between watcher's slug rules and lib's Title validator.
       DescribeTable("computeFilenameHint output passes task.CreateCommand.Validate",
           func(provider, owner, repo, sha string) {
               title := computeFilenameHint(provider, owner, repo, sha)
               cmd := task.CreateCommand{
                   TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
                   Title:          title,
                   Frontmatter:    agentlib.TaskFrontmatter{"assignee": "build-fixer-agent", "status": "todo"},
                   Body:           "build failed",
               }
               Expect(cmd.Validate(context.Background())).To(Succeed())
           },
           Entry("typical", "github", "bborbe", "maintainer", "5886450a1234"),
           Entry("hyphenated repo", "github", "my-org", "my-repo", "abc1234"),
           Entry("digits in repo", "github", "bborbe", "repo123", "deadbeef"),
       )
   })
   ```

   Add import `task "github.com/bborbe/agent/lib/command/task"` to this file. Remove the `agentlib` import from this file if `WatcherCreateTaskCommand` was the only thing that used it (check — `computeFilenameHint` tests use plain strings and don't need `agentlib`; the old JSON test used `agentlib.CreateTaskCommand`).

10. **Add CHANGELOG entry** to root `CHANGELOG.md`. The repo uses release-versioned headers (`## v0.23.28`, …) with NO existing `## Unreleased` section. Prepend a new `## Unreleased` section above the latest version header (release tooling renames it to `## vX.Y.Z` on next release):

    ```markdown
    # Changelog

    All notable changes to this project will be documented in this file.

    ## Unreleased

    - refactor(watcher/github-build): migrate create-task publish path to task.CreateCommandSender from agent/lib/command/task — removes WatcherCreateTaskCommand wrapper; slug result now populates Title field; bumps agent/lib to v0.58.0

    ## v0.23.28
    ...
    ```

    If a `## Unreleased` section already exists (created by prompt 1 of this spec), append the entry there instead of creating a new section.

11. **Run `make precommit`** from `watcher/github-build/`:

    ```bash
    cd watcher/github-build && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-build/` and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- `agent/lib` MUST be bumped to exactly `v0.58.0` (or higher) — this is the minimum that contains `github.com/bborbe/agent/lib/command/task`
- **NEVER call `task.NewCreateCommandSender` or reference any symbol from `command/task` without first grepping `$(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.58.0/command/task/` to confirm it exists** — hallucinated symbols compile-fail
- `task.CreateCommand.Title` must carry the slug result from `computeFilenameHint` — same value that was previously in `WatcherCreateTaskCommand.FilenameHint`
- `computeFilenameHint` and `slugifySegment` in `watcher/github-build/pkg/filename.go` MUST NOT change (spec says "slug helpers carry over unchanged")
- `publisher.go` must be gutted (all symbols removed); `publisher_test.go` must be deleted; `mocks/command_publisher.go` must be deleted
- Tests MUST use the shipped counterfeiter mocks from `agent/lib/command/task/mocks/` — do NOT hand-write mocks or regenerate them locally
- Wire-format test MUST assert `"title"` is present in JSON output AND `"filename_hint"` is absent
- The `include_logs` tests (Describe block added in spec-020 prompt 3) must still pass — update mock call counts and args-for-call as in step 8e
- `buildCreateTaskCommand` internal tests (if they call it directly) must update: return type changes from `WatcherCreateTaskCommand` to `task.CreateCommand`, `cmd.FilenameHint` → `cmd.Title`
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`
- Coverage ≥80% for changed packages
- All existing slug-helper tests (`computeFilenameHint`, `slugifySegment`) must still pass unchanged
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm agent/lib bumped to v0.58.0:
grep "github.com/bborbe/agent/lib" watcher/github-build/go.mod
# Expected: v0.58.0 or higher

# Confirm WatcherCreateTaskCommand is gone:
grep -rn "WatcherCreateTaskCommand\|CommandPublisher\|kafkaPublisher\|NewCommandPublisher" \
  watcher/github-build/pkg/
# Expected: zero matches

# Confirm filename_hint JSON key is gone:
grep -rn "filename_hint" watcher/github-build/pkg/
# Expected: zero matches

# Confirm task.CreateCommandSender injected in watcher:
grep -n "CreateCommandSender" watcher/github-build/pkg/watcher.go
# Expected: type in struct + NewWatcher signature

# Confirm Title field used in buildCreateTaskCommand:
grep -n "Title:" watcher/github-build/pkg/watcher.go | grep computeFilenameHint
# Expected: Title: computeFilenameHint(...)

# Confirm factory creates sender from cdb.CommandObjectSender:
grep -n "NewCreateCommandSender" watcher/github-build/pkg/factory/factory.go
# Expected: constructor called

# Confirm publisher.go has no exported symbols:
grep -n "^type\|^func\|^var\|^const" watcher/github-build/pkg/publisher.go
# Expected: zero matches

# Confirm publisher_test.go deleted:
ls watcher/github-build/pkg/publisher_test.go 2>&1
# Expected: No such file or directory

# Confirm command_publisher mock deleted:
ls watcher/github-build/pkg/mocks/command_publisher.go 2>&1
# Expected: No such file or directory

# Confirm watcher_test.go uses taskmocks:
grep -n "taskmocks\|CreateCommandSender" watcher/github-build/pkg/watcher_test.go
# Expected: new mock import and usage

# Confirm wire-format test asserts "title" in and "filename_hint" out:
grep -n '"title"\|"filename_hint"' watcher/github-build/pkg/watcher_internal_test.go
# Expected: assertions on both

# Confirm slug tests still pass (no changes to filename.go):
grep -n "func computeFilenameHint\|func slugifySegment" watcher/github-build/pkg/filename.go
# Expected: both functions still present unchanged

# Confirm CHANGELOG entry:
grep -n "github-build.*migrate\|CreateCommandSender\|task.CreateCommand" CHANGELOG.md | head -3
# Expected: one match under ## Unreleased
</verification>
