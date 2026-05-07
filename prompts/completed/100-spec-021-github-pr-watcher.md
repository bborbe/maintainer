---
status: completed
spec: [021-migrate-watchers-to-task-create-command-sender]
summary: Migrated github-pr watcher create-task and update-frontmatter publish paths to task.CreateCommandSender and task.UpdateFrontmatterCommandSender from agent/lib/command/task, removing WatcherCreateTaskCommand wrapper and hand-rolled kafkaPublisher; slug result now populates Title field; agent/lib bumped to v0.58.0.
container: maintainer-100-spec-021-github-pr-watcher
dark-factory-version: v0.151.2-4-g3dc5753
created: "2026-05-07T20:30:00Z"
queued: "2026-05-07T20:17:53Z"
started: "2026-05-07T20:17:54Z"
completed: "2026-05-07T20:30:16Z"
branch: dark-factory/migrate-watchers-to-task-create-command-sender
---

<summary>
- The `WatcherCreateTaskCommand` wrapper type and the hand-rolled `CommandPublisher` / `kafkaPublisher` are removed from the github-pr watcher
- The watcher now receives an injected `task.CreateCommandSender` for create-task and a `task.UpdateFrontmatterCommandSender` for force-push — both come from `github.com/bborbe/agent/lib/command/task`
- The slug helpers (`computePRFilenameHint`, `slugifyTitle`) are kept byte-identical; their result is now set as `task.CreateCommand.Title` instead of `WatcherCreateTaskCommand.FilenameHint`
- The wire format changes: the `filename_hint` JSON key is gone; `"title"` carries the human-readable filename stem
- `github.com/bborbe/agent/lib` is bumped from `v0.57.0` to `v0.58.0` in `watcher/github-pr/go.mod`
- The factory constructs both senders from the existing `cdb.CommandObjectSender` and injects them into the watcher
- Existing tests are updated to use counterfeiter mocks from `agent/lib/command/task/mocks/` instead of the local `mocks.CommandPublisher`
- A wire-format unit test locks `"title"` into the JSON output and asserts `"filename_hint"` is absent
- `make precommit` passes clean in `watcher/github-pr/`
</summary>

<objective>
Migrate the github-pr watcher's create-task and update-frontmatter publish paths to use the typed `task.CreateCommandSender` and `task.UpdateFrontmatterCommandSender` from `agent/lib/command/task`, replacing the hand-rolled `WatcherCreateTaskCommand` wrapper and `kafkaPublisher`. The slug that was `FilenameHint` becomes `Title` on `task.CreateCommand`. No slug logic changes.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, counterfeiter mocks, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — factory has zero business logic, `Create*` prefix.
Read `go-composition.md` in `~/.claude/plugins/marketplaces/coding/docs/` — inject interfaces, never call package functions directly.

**Files to read fully before making any changes:**
- `watcher/github-pr/go.mod` — understand current `agent/lib` version to bump
- `watcher/github-pr/pkg/publisher.go` — full file; understand `WatcherCreateTaskCommand`, `CommandPublisher`, `kafkaPublisher`, `marshalEvent`, `buildCommandObject` — all of these are being removed
- `watcher/github-pr/pkg/watcher.go` — full file; understand `publishCreate` (builds `WatcherCreateTaskCommand` twice for trusted/untrusted paths, calls `w.publisher.PublishCreate`) and `publishForcePush` (builds `agentlib.UpdateFrontmatterCommand`, calls `w.publisher.PublishUpdateFrontmatter`)
- `watcher/github-pr/pkg/watcher_test.go` — full file; understand all `It` blocks that call `pub.PublishCreateArgsForCall(0)` or `pub.PublishUpdateFrontmatterArgsForCall(0)` — these will change to use the new mock types
- `watcher/github-pr/pkg/publisher_test.go` — full file; note all tests reference `pkg.WatcherCreateTaskCommand` and `pkg.NewCommandPublisher` — this entire file is deleted
- `watcher/github-pr/pkg/filename_internal_test.go` — full file; the last `Describe("WatcherCreateTaskCommand JSON marshalling")` block must be replaced
- `watcher/github-pr/pkg/factory/factory.go` — full file; understand `CreateKafkaPublisher` and `CreateWatcher` — both will change significantly
- `watcher/github-pr/pkg/mocks/command_publisher.go` — understand counterfeiter shape; this file is deleted

**Step 0 — Bump dep and fetch module source:**

```bash
cd watcher/github-pr && go get github.com/bborbe/agent/lib@v0.58.0 && go mod tidy
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

# Confirm task.UpdateFrontmatterCommandSender interface and its method(s)
grep -A 5 "type UpdateFrontmatterCommandSender interface" "$AGENTLIB/command/task/"*.go 2>/dev/null

# Confirm task.NewUpdateFrontmatterCommandSender constructor
grep -n "func NewUpdateFrontmatterCommandSender" "$AGENTLIB/command/task/"*.go 2>/dev/null

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
```

Document any deviation from expectations in `## Improvements`. Adapt all subsequent steps to the actual types/method names found.
</context>

<requirements>
**Execute steps in order. Run `make test` after step 6. Run `make precommit` only at the final step.**

1. **Bump `github.com/bborbe/agent/lib` in `watcher/github-pr/go.mod`** (already done in Step 0 above via `go get`). Confirm the `require` line reads `v0.58.0` or higher.

2. **Delete `watcher/github-pr/pkg/mocks/command_publisher.go`**

   ```bash
   rm watcher/github-pr/pkg/mocks/command_publisher.go
   ```

3. **Replace `watcher/github-pr/pkg/publisher.go`** — remove all types and functions. Keep only the copyright header and package declaration:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg
   ```

   Symbols removed: `WatcherCreateTaskCommand`, `CommandPublisher`, `NewCommandPublisher`, `kafkaPublisher`, `marshalEvent`, `buildCommandObject`. All callers will be updated in subsequent steps.

4. **Delete `watcher/github-pr/pkg/publisher_test.go`**

   ```bash
   rm watcher/github-pr/pkg/publisher_test.go
   ```

5. **Update `watcher/github-pr/pkg/watcher.go`**

   a. **Update the struct fields** — replace `publisher CommandPublisher` with the two typed senders. Use the exact interface types verified in Step 0b:

   ```go
   type watcher struct {
       ghClient                   GitHubClient
       createSender               task.CreateCommandSender
       updateFrontmatterSender    task.UpdateFrontmatterCommandSender
       cursorPath                 string
       startTime                  libtime.DateTime
       scope                      string
       taskCreationFilter         filter.TaskCreationFilter
       stage                      string
       metrics                    Metrics
       trustDecision              trust.Trust
   }
   ```

   b. **Update `NewWatcher`** — replace `pub CommandPublisher` with both senders:

   ```go
   func NewWatcher(
       ghClient GitHubClient,
       createSender task.CreateCommandSender,
       updateFrontmatterSender task.UpdateFrontmatterCommandSender,
       cursorPath string,
       startTime libtime.DateTime,
       scope string,
       taskCreationFilter filter.TaskCreationFilter,
       stage string,
       metrics Metrics,
       trustDecision trust.Trust,
   ) Watcher {
       return &watcher{
           ghClient:                ghClient,
           createSender:            createSender,
           updateFrontmatterSender: updateFrontmatterSender,
           cursorPath:              cursorPath,
           startTime:               startTime,
           scope:                   scope,
           taskCreationFilter:      taskCreationFilter,
           stage:                   stage,
           metrics:                 metrics,
           trustDecision:           trustDecision,
       }
   }
   ```

   c. **Update `publishCreate`** — build `task.CreateCommand` and use `w.createSender`. Verify the exact field names (Title, TaskIdentifier, Frontmatter, Body) from Step 0b before writing. Use the ACTUAL types found in v0.58.0:

   ```go
   var cmd task.CreateCommand
   if trustResult.Success() {
       cmd = task.CreateCommand{
           Title:          computePRFilenameHint("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
           TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
           Frontmatter:    buildFrontmatter(pr, taskIDStr, w.stage, details),
           Body:           buildTaskBody(pr),
       }
   } else {
       if author == "" {
           author = "(unknown)"
       }
       glog.V(2).Infof("untrusted author=%q trust=%s pr=%s", author, trustResult.Description(), pr.HTMLURL)
       cmd = task.CreateCommand{
           Title:          computePRFilenameHint("github", pr.Owner, pr.Repo, pr.Number, pr.Title),
           TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
           Frontmatter:    buildHumanReviewFrontmatter(pr, taskIDStr, w.stage, details),
           Body:           buildUntrustedBody(author, trustResult.Description()),
       }
   }

   if err := w.createSender.SendCommand(ctx, cmd); err != nil {
       glog.Errorf("publish create-task failed pr=%s err=%v", pr.HTMLURL, err)
       w.metrics.IncPRPublished("error")
       return false
   }
   ```

   (Adjust `SendCommand` to the actual method name found in Step 0b.)

   Remove the `var cmd WatcherCreateTaskCommand` declaration that was at the top of `publishCreate`. Remove the `agentlib` import if it is no longer referenced directly in this function (but `agentlib.TaskIdentifier` and `agentlib.TaskFrontmatter` may still be used in `buildFrontmatter` etc. — keep the import if so).

   d. **Update `publishForcePush`** — replace `w.publisher.PublishUpdateFrontmatter(ctx, cmd)` with `w.updateFrontmatterSender.SendCommand(ctx, cmd)`. **CRITICAL: also switch the struct type** from `agentlib.UpdateFrontmatterCommand` to `task.UpdateFrontmatterCommand`, and from `*agentlib.BodySection` to `*task.BodySection`. The `task` package re-declares both types (see `agent/lib/command/task/update-frontmatter-command.go`), and `task.UpdateFrontmatterCommandSender.SendCommand` accepts `task.UpdateFrontmatterCommand` only. Field shape is identical; only the type qualifier changes.

   ```go
   if err := w.updateFrontmatterSender.SendCommand(ctx, cmd); err != nil {
       glog.Errorf("publish update-frontmatter failed pr=%s err=%v", pr.HTMLURL, err)
       w.metrics.IncPRPublished("error")
       return false
   }
   ```

   e. **Update imports** in `watcher.go`:
   - Add `task "github.com/bborbe/agent/lib/command/task"`
   - Keep `agentlib "github.com/bborbe/agent/lib"` — `agentlib.TaskIdentifier` and `agentlib.TaskFrontmatter` are still used in `buildFrontmatter` etc. The `agentlib.UpdateFrontmatterCommand` and `agentlib.BodySection` references are REMOVED in step 5d above (replaced with `task.UpdateFrontmatterCommand` / `*task.BodySection`).

6. **Update `watcher/github-pr/pkg/factory/factory.go`**

   a. Replace `CreateKafkaPublisher` with `CreateKafkaSenders` that returns both senders:

   ```go
   // CreateKafkaSenders constructs typed task command senders backed by a Kafka sync producer.
   func CreateKafkaSenders(
       ctx context.Context,
       brokers libkafka.Brokers,
       branch base.Branch,
   ) (task.CreateCommandSender, task.UpdateFrontmatterCommandSender, func(), error) {
       syncProducer, err := libkafka.NewSyncProducerWithName(
           ctx,
           brokers,
           "maintainer-watcher-github-pr",
       )
       if err != nil {
           return nil, nil, nil, errors.Wrap(ctx, err, "create sync producer")
       }
       sender := cdb.NewCommandObjectSender(syncProducer, branch, log.DefaultSamplerFactory)
       cleanup := func() {
           if err := syncProducer.Close(); err != nil {
               glog.Warningf("close kafka sync producer: %v", err)
           }
       }
       return task.NewCreateCommandSender(sender),
           task.NewUpdateFrontmatterCommandSender(sender),
           cleanup,
           nil
   }
   ```

   b. Update `CreateWatcher` to call `CreateKafkaSenders` and pass both senders to `pkg.NewWatcher`:

   ```go
   func CreateWatcher(
       ctx context.Context,
       ghToken string,
       brokers libkafka.Brokers,
       stage string,
       repoScope string,
       taskCreationFilter filter.TaskCreationFilter,
       startTime libtime.DateTime,
       trustedAuthors []string,
   ) (pkg.Watcher, func(), error) {
       branch := base.Branch(stage)
       createSender, updateFrontmatterSender, cleanup, err := CreateKafkaSenders(ctx, brokers, branch)
       if err != nil {
           return nil, nil, errors.Wrap(ctx, err, "create kafka senders")
       }

       trustDecision := trust.And{trust.NewAuthorAllowlist(trustedAuthors)}
       ghClient := pkg.NewGitHubClient(ghToken)
       w := pkg.NewWatcher(
           ghClient,
           createSender,
           updateFrontmatterSender,
           pkg.DefaultCursorPath,
           startTime,
           repoScope,
           taskCreationFilter,
           stage,
           pkg.NewMetrics(),
           trustDecision,
       )
       return w, cleanup, nil
   }
   ```

   c. **Update imports** in `factory.go`:
   - Add `task "github.com/bborbe/agent/lib/command/task"`
   - Remove `agentlib "github.com/bborbe/agent/lib"` if it is no longer used in this file (check — it was not used directly in the old factory)
   - Keep `"github.com/bborbe/cqrs/base"`, `"github.com/bborbe/cqrs/cdb"`, `libkafka`, `"github.com/bborbe/log"`

   **Grep-verify `task.NewCreateCommandSender` and `task.NewUpdateFrontmatterCommandSender` constructor signatures from Step 0b before writing the above.** If the constructors take additional parameters beyond `sender`, adjust accordingly.

7. **Run `make test`**:

   ```bash
   cd watcher/github-pr && make test
   ```

   If `NewWatcher` callers in tests fail to compile (wrong argument count/types), fix before proceeding.

8. **Update `watcher/github-pr/pkg/watcher_test.go`**

   a. Replace `*mocks.CommandPublisher` with the typed mocks from `agent/lib/command/task/mocks`. Use the exact mock struct names found in Step 0b:

   ```go
   // Old:
   pub  *mocks.CommandPublisher
   // New (adjust to exact struct names from the taskmocks package):
   createSender              *taskmocks.TaskCreateCommandSender
   updateFrontmatterSender   *taskmocks.TaskUpdateFrontmatterCommandSender
   ```

   Add the import:
   ```go
   taskmocks "github.com/bborbe/agent/lib/command/task/mocks"
   ```

   Remove the import of `"github.com/bborbe/maintainer/watcher/github-pr/pkg/mocks"` if it is no longer used.

   b. Update `BeforeEach` setup to instantiate the new mocks:
   ```go
   createSender = new(taskmocks.TaskCreateCommandSender)
   updateFrontmatterSender = new(taskmocks.TaskUpdateFrontmatterCommandSender)
   ```

   c. Update `newTestWatcher` to pass both senders:
   ```go
   func newTestWatcher(
       ghClient pkg.GitHubClient,
       createSender task.CreateCommandSender,
       updateFrontmatterSender task.UpdateFrontmatterCommandSender,
       cursorPath string,
       startTime libtime.DateTime,
       fakeMetrics *mocks.Metrics,
       trustDecision trust.Trust,
   ) pkg.Watcher {
       return pkg.NewWatcher(
           ghClient,
           createSender,
           updateFrontmatterSender,
           cursorPath,
           startTime,
           "bborbe",
           filter.TaskCreationFilters{...},
           "dev",
           fakeMetrics,
           trustDecision,
       )
   }
   ```

   d. In each `It` block that previously called `pub.PublishCreateArgsForCall(0)`:
   - Replace with `createSender.SendCommandCallCount()` and `createSender.SendCommandArgsForCall(0)` (use exact stub method names from Step 0b)
   - The `cmd` is now `task.CreateCommand`
   - Replace `cmd.FilenameHint` assertions with `cmd.Title` assertions — e.g.:
     ```go
     // Old:
     Expect(cmd.FilenameHint).To(Equal("PR Review github - bborbe-code-reviewer - 42 - feat-new-feature"))
     // New:
     Expect(cmd.Title).To(Equal("PR Review github - bborbe-code-reviewer - 42 - feat-new-feature"))
     ```
   - The `cmd.TaskIdentifier`, `cmd.Frontmatter`, `cmd.Body` assertions continue to work if `task.CreateCommand` has those fields at the top level (verify from Step 0b).

   e. In each `It` block that previously called `pub.PublishUpdateFrontmatterArgsForCall(0)`:
   - Replace with `updateFrontmatterSender.SendCommandCallCount()` and `updateFrontmatterSender.SendCommandArgsForCall(0)`

   f. For tests that assert `pub.PublishCreateCallCount() == 0` (nothing published), change to `createSender.SendCommandCallCount() == 0`.

9. **Update `watcher/github-pr/pkg/filename_internal_test.go`** — replace the `WatcherCreateTaskCommand JSON marshalling` describe block with a wire-format test for `task.CreateCommand`:

   Remove the entire:
   ```go
   var _ = Describe("WatcherCreateTaskCommand JSON marshalling", func() { ... })
   ```

   Replace with:
   ```go
   var _ = Describe("task.CreateCommand wire format", func() {
       It("emits 'title' as the top-level key (not 'filename_hint')", func() {
           cmd := task.CreateCommand{
               Title: "PR Review github - bborbe-maintainer - 2 - test-pr",
               // other fields via actual type shape found in Step 0b
           }
           raw, err := json.Marshal(cmd)
           Expect(err).NotTo(HaveOccurred())
           Expect(string(raw)).To(ContainSubstring(`"title":"PR Review github - bborbe-maintainer - 2 - test-pr"`))
           Expect(string(raw)).NotTo(ContainSubstring(`"filename_hint"`))
       })

       // Boundary contract: slug helper output MUST pass task.CreateCommand.Validate (level-1 contract test).
       // Prevents future drift between watcher's slug rules and lib's Title validator.
       DescribeTable("computePRFilenameHint output passes task.CreateCommand.Validate",
           func(provider, owner, repo string, number int, prTitle string) {
               title := computePRFilenameHint(provider, owner, repo, number, prTitle)
               cmd := task.CreateCommand{
                   TaskIdentifier: agentlib.TaskIdentifier("00000000-0000-0000-0000-000000000000"),
                   Title:          title,
                   Frontmatter:    agentlib.TaskFrontmatter{"assignee": "pr-reviewer-agent", "status": "todo"},
                   Body:           "review the PR",
               }
               Expect(cmd.Validate(context.Background())).To(Succeed())
           },
           Entry("typical PR", "github", "bborbe", "maintainer", 2, "test: delete this PR never"),
           Entry("hyphenated repo", "github", "my-org", "my-repo", 99, "bump deps"),
           Entry("special chars in title", "github", "bborbe", "trading", 110, "fix: chromium @trixie [edge]"),
           Entry("empty title (slug omits segment)", "github", "bborbe", "x", 7, ""),
           Entry("unicode-only title (slug omits segment)", "github", "bborbe", "x", 7, "🚀🎉"),
       )
   })
   ```

   Add import `task "github.com/bborbe/agent/lib/command/task"` to this file. Keep `agentlib "github.com/bborbe/agent/lib"` (used by `agentlib.TaskIdentifier` / `agentlib.TaskFrontmatter` in the new test entries).

10. **Add CHANGELOG entry** to root `CHANGELOG.md`. The repo uses release-versioned headers (`## v0.23.28`, …) with NO existing `## Unreleased` section. Prepend a new `## Unreleased` section above the latest version header (release tooling renames it to `## vX.Y.Z` on next release):

    ```markdown
    # Changelog

    All notable changes to this project will be documented in this file.

    ## Unreleased

    - refactor(watcher/github-pr): migrate create-task and update-frontmatter publish paths to task.CreateCommandSender / task.UpdateFrontmatterCommandSender from agent/lib/command/task — removes WatcherCreateTaskCommand wrapper; slug result now populates Title field; bumps agent/lib to v0.58.0

    ## v0.23.28
    ...
    ```

    If a `## Unreleased` section already exists (created by prompt 2 of this spec), append the entry there instead of creating a new section.

11. **Run `make precommit`** from `watcher/github-pr/`:

    ```bash
    cd watcher/github-pr && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `watcher/github-pr/` and root `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- `agent/lib` MUST be bumped to exactly `v0.58.0` (or higher) — this is the minimum that contains `github.com/bborbe/agent/lib/command/task`
- **NEVER call `task.NewCreateCommandSender` or reference any symbol from `command/task` without first grepping `$(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@v0.58.0/command/task/` to confirm it exists** — hallucinated symbols compile-fail
- `task.CreateCommand.Title` must carry the slug result from `computePRFilenameHint` — same value that was previously in `WatcherCreateTaskCommand.FilenameHint`
- `computePRFilenameHint` and `slugifyTitle` in `watcher/github-pr/pkg/filename.go` MUST NOT change (spec says "slug helpers carry over unchanged")
- `publishForcePush` MUST construct `task.UpdateFrontmatterCommand` (NOT `agentlib.UpdateFrontmatterCommand`) with `*task.BodySection` (NOT `*agentlib.BodySection`); `task.UpdateFrontmatterCommandSender.SendCommand` accepts the `task` types only. Field names and shapes are identical between the two struct families — only the package qualifier changes. Do NOT modify the frontmatter update field values.
- `publisher.go` must be gutted (all symbols removed); `publisher_test.go` must be deleted; `mocks/command_publisher.go` must be deleted
- Tests MUST use the shipped counterfeiter mocks from `agent/lib/command/task/mocks/` — do NOT hand-write mocks or regenerate them locally
- Wire-format test MUST assert `"title"` is present in JSON output AND `"filename_hint"` is absent
- `make precommit` runs from `watcher/github-pr/`, never at repo root
- Error wrapping: `github.com/bborbe/errors` — never `fmt.Errorf`
- Coverage ≥80% for changed packages
- All existing slug-helper tests (`slugifyTitle`, `computePRFilenameHint`) must still pass unchanged
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm agent/lib bumped to v0.58.0:
grep "github.com/bborbe/agent/lib" watcher/github-pr/go.mod
# Expected: v0.58.0 or higher

# Confirm WatcherCreateTaskCommand is gone:
grep -rn "WatcherCreateTaskCommand\|CommandPublisher\|kafkaPublisher\|NewCommandPublisher" \
  watcher/github-pr/pkg/
# Expected: zero matches (type removed from all files)

# Confirm filename_hint JSON key is gone:
grep -rn "filename_hint" watcher/github-pr/pkg/
# Expected: zero matches

# Confirm task.CreateCommandSender injected in watcher:
grep -n "CreateCommandSender\|UpdateFrontmatterCommandSender" watcher/github-pr/pkg/watcher.go
# Expected: both types in struct + NewWatcher signature

# Confirm Title field used in publishCreate:
grep -n "Title:" watcher/github-pr/pkg/watcher.go | grep computePRFilenameHint
# Expected: Title: computePRFilenameHint(...)

# Confirm factory creates senders from cdb.CommandObjectSender:
grep -n "NewCreateCommandSender\|NewUpdateFrontmatterCommandSender" watcher/github-pr/pkg/factory/factory.go
# Expected: both constructors called

# Confirm publisher.go has no exported symbols:
grep -n "^type\|^func\|^var\|^const" watcher/github-pr/pkg/publisher.go
# Expected: zero matches (file contains only package declaration)

# Confirm publisher_test.go deleted:
ls watcher/github-pr/pkg/publisher_test.go 2>&1
# Expected: No such file or directory

# Confirm command_publisher mock deleted:
ls watcher/github-pr/pkg/mocks/command_publisher.go 2>&1
# Expected: No such file or directory

# Confirm watcher_test.go uses taskmocks not old CommandPublisher mock:
grep -n "taskmocks\|CreateCommandSender\|UpdateFrontmatterCommandSender" watcher/github-pr/pkg/watcher_test.go
# Expected: new mock imports and usage

# Confirm wire-format test asserts "title" in and "filename_hint" out:
grep -n '"title"\|"filename_hint"' watcher/github-pr/pkg/filename_internal_test.go
# Expected: assertions on both

# Confirm slug tests still pass (no changes to filename.go):
grep -n "func computePRFilenameHint\|func slugifyTitle" watcher/github-pr/pkg/filename.go
# Expected: both functions still present unchanged

# Confirm CHANGELOG entry:
grep -n "github-pr.*migrate\|CreateCommandSender\|task.CreateCommand" CHANGELOG.md | head -3
# Expected: one match under ## Unreleased
</verification>
