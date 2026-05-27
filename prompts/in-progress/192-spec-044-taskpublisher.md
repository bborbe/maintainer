---
status: approved
spec: [044-github-release-watcher-implementation]
created: "2026-05-27T20:38:37Z"
queued: "2026-05-27T20:57:47Z"
---

<summary>
- `BuildCreateCommand` produces the frozen Phase 1 frontmatter shape: `task_type: github-release`, `assignee: github-releaser-agent`, `phase: planning`, `status: in_progress`, plus repo/clone_url/ref/current_version fields
- Body is an operator-readable header only — no `## Unreleased` bullets embedded
- `PublishCreate` sends the command via `task.CreateCommandSender`, increments `IncPublished("create"|"error")` accordingly, logs structured outcome via `glog.V(2)`
- Ginkgo tests cover the named acceptance criterion `BuildCreateCommand produces frontmatter task_type github-release for bborbe/docker-utils d630ef3` against the Phase 1 vault evidence
- Mock for `TaskPublisher` regenerated to satisfy the existing counterfeiter directive
</summary>

<objective>
Replace the TODO stubs in `watcher/github-release/pkg/taskpublisher.go` with working `BuildCreateCommand` and `PublishCreate` implementations. The frontmatter contract is FROZEN per the Phase 1 vault evidence — every key MUST match the prototype output for the same `(owner, repo, head_sha)`. Add Ginkgo tests anchored on the `bborbe/docker-utils d630ef3` table row.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-prometheus-metrics-guide.md`

Read these files end-to-end before writing code:
- `watcher/github-release/pkg/taskpublisher.go` — current stub; preserve the interface, `TaskConfig`, `NewTaskPublisher`, `BuildCreateCommand` signature, and the godoc comment block describing the frozen contract
- `watcher/github-release/pkg/release.go` — `Release` struct + `ShortSHA()` helper
- `watcher/github-release/pkg/repo.go` — `Repo.Key()` shape (host-qualified `github.com/owner/name`)
- `watcher/github-release/pkg/filename.go` — `ComputeTaskTitle` (already complete)
- `watcher/github-release/pkg/taskid.go` — `DeriveTaskID` (implemented in prompt 1)
- `watcher/github-release/pkg/metrics.go` — confirm `IncPublished(status)` accepts `"create"`, `"skipped"`, `"error"` labels (pre-initialized in `init`)

Phase 1 evidence — frontmatter the watcher MUST emit for `bborbe/docker-utils` @ ref `d630ef3526cfc57fbdccd9ba53c5c3a02945e407` (vault file `Release bborbe-docker-utils d630ef3.md`, watcher-emit-time snapshot — the vault file's current `phase: done, status: completed` are post-execution agent transitions, NOT the watcher-emit values):

```yaml
task_type: github-release
assignee: github-releaser-agent
phase: planning
status: in_progress
stage: dev
task_identifier: b28fa8db-c5eb-e4ac-9dc5-103ec1038900
title: Release bborbe/docker-utils at d630ef3
repo: bborbe/docker-utils
clone_url: git@github.com:bborbe/docker-utils.git
ref: d630ef3526cfc57fbdccd9ba53c5c3a02945e407
current_version: v1.7.7
```

EVERY key in `buildFrontmatter`'s output map MUST equal these literal values for inputs `(owner=bborbe, repo=docker-utils, head_sha=d630ef3526cfc57fbdccd9ba53c5c3a02945e407, current_version=v1.7.7, stage=dev)`. The `task_identifier` `b28fa8db-c5eb-e4ac-9dc5-103ec1038900` is what `DeriveTaskID(owner, repo, head_sha)` MUST produce given the frozen namespace UUID `4f9e2c1a-7b30-4d8f-9a2e-1c5b8d4f3a90` — this is the canonical regression input shared with prompt 1.

**Title format resolved (OPEN QUESTION closed):** vault wins per spec § Constraints "FROZEN per Phase 1 evidence". The skeleton's `ComputeTaskTitle` has been updated to return `Release <owner>/<repo> at <sha[:7]>` (slash + " at " literal). Spec § Desired Behavior #6 updated to match. Implement per the updated skeleton — do NOT reintroduce the dash/no-"at" form.

Reference implementation — READ for the `agentlib.TaskFrontmatter{...}` pattern and `task.CreateCommand` shape:
- `/workspace/watcher/github-pr/pkg/watcher.go` — see `BuildCreateCommand` (line 312), `buildFrontmatter` (line 398), `buildTaskBody` (line 386). Confirm `agentlib.TaskFrontmatter` is a `map[string]string`-shaped type from `github.com/bborbe/agent/lib`; values are string literals (`"in_progress"`, `"planning"`, etc.). Confirm `TaskIdentifier` is a typed string from the same package, constructed via `agentlib.TaskIdentifier(taskIDStr)`. Confirm `task.CreateCommand` has fields `Title string`, `TaskIdentifier agentlib.TaskIdentifier`, `Frontmatter agentlib.TaskFrontmatter`, `Body string`.
- `/workspace/watcher/github-pr/pkg/watcher.go` `PublishCreate` body — mirror the glog + metrics shape (`glog.V(2).Infof("published CreateTaskCommand ...")` on success, `glog.Errorf("publish create-task failed ...")` on send error, `IncPublished("create")` / `IncPublished("error")`).

Counterfeiter / mock note: `pkg/taskpublisher.go` line 15 already declares `//counterfeiter:generate -o mocks/task_publisher.go --fake-name TaskPublisher . TaskPublisher`. This emits to `pkg/mocks/task_publisher.go` after `make generate`.

Frozen body shape (per spec § Desired Behavior #6 — "operator-readable header only"):
```
# Release: <owner>/<name>

**Current version:** <CurrentVersion>
**HEAD:** <ShortSHA>
**Changelog:** https://github.com/<owner>/<name>/blob/master/CHANGELOG.md
**Repo:** [<owner>/<name>](https://github.com/<owner>/<name>)
```

The `blob/master/` hardcoding matches the Phase 1 prototype. If repos with non-master default branches need a different link target, that is a follow-up — the spec explicitly carries this verbatim.
</context>

<requirements>

**Execute steps in order. Run `cd watcher/github-release && make test` after step 4. Run `make precommit` only at the final step.**

1. **Replace the stub `BuildCreateCommand` in `watcher/github-release/pkg/taskpublisher.go`** with the full implementation. Keep the exported signature:

   ```go
   func BuildCreateCommand(release Release, cfg TaskConfig) task.CreateCommand
   ```

   Body:
   ```go
   taskIDStr := DeriveTaskID(release.Repo.Owner, release.Repo.Name, release.HeadSHA).String()
   return task.CreateCommand{
       Title:          ComputeTaskTitle(release),
       TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
       Frontmatter:    buildFrontmatter(release, taskIDStr, cfg),
       Body:           buildTaskBody(release),
   }
   ```

   Then add the two unexported helpers in the same file:

   ```go
   func buildFrontmatter(release Release, taskIDStr string, cfg TaskConfig) agentlib.TaskFrontmatter {
       return agentlib.TaskFrontmatter{
           "task_type":       "github-release",
           "assignee":        "github-releaser-agent",
           "phase":           "planning",
           "status":          "in_progress",
           "stage":           cfg.Stage,
           "task_identifier": taskIDStr,
           "title":           ComputeTaskTitle(release),
           "repo":            fmt.Sprintf("%s/%s", release.Repo.Owner, release.Repo.Name),
           "clone_url":       fmt.Sprintf("git@github.com:%s/%s.git", release.Repo.Owner, release.Repo.Name),
           "ref":             release.HeadSHA,
           "current_version": release.CurrentVersion,
       }
   }

   func buildTaskBody(release Release) string {
       owner := release.Repo.Owner
       name := release.Repo.Name
       return fmt.Sprintf(
           "# Release: %s/%s\n\n**Current version:** %s\n**HEAD:** %s\n**Changelog:** https://github.com/%s/%s/blob/master/CHANGELOG.md\n**Repo:** [%s/%s](https://github.com/%s/%s)\n",
           owner, name,
           release.CurrentVersion,
           release.ShortSHA(),
           owner, name,
           owner, name,
           owner, name,
       )
   }
   ```

   Imports to add: `"fmt"`. Keep the existing `agentlib`, `task`, `context`, `errors` imports. Remove the `errUnimplemented` var + `ErrUnimplemented()` func — they are no longer needed (search for any caller across `watcher/github-release/` first; if found, surface as an error and stop. There should be none.).

2. **Replace the stub `PublishCreate`** with the full send path:

   ```go
   func (p *taskPublisher) PublishCreate(ctx context.Context, release Release) bool {
       cmd := BuildCreateCommand(release, p.cfg)

       if err := p.sender.SendCommand(ctx, cmd); err != nil {
           glog.Errorf(
               "publish create-task failed repo=%s sha=%s taskID=%s err=%v",
               release.Repo.Key(),
               release.HeadSHA,
               string(cmd.TaskIdentifier),
               err,
           )
           p.metrics.IncPublished("error")
           return false
       }
       glog.V(2).Infof(
           "published CreateTaskCommand repo=%s sha=%s taskID=%s stage=%s",
           release.Repo.Key(),
           release.HeadSHA,
           string(cmd.TaskIdentifier),
           p.cfg.Stage,
       )
       p.metrics.IncPublished("create")
       return true
   }
   ```

   Add `github.com/golang/glog` to imports.

3. **Regenerate mocks**:
   ```bash
   cd watcher/github-release && make generate
   ls pkg/mocks/task_publisher.go
   ```

4. **Create `watcher/github-release/pkg/taskpublisher_test.go`** as package `pkg_test`. Imports:
   ```go
   import (
       "context"

       agentlib "github.com/bborbe/agent/lib"
       task "github.com/bborbe/agent/lib/command/task"
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-release/pkg"
       "github.com/bborbe/maintainer/watcher/github-release/pkg/mocks"
   )
   ```

   `Describe("pkg.BuildCreateCommand", ...)` with these `It` blocks:

   a. **`It("BuildCreateCommand produces frontmatter task_type github-release for bborbe/docker-utils d630ef3")`** — exact spec acceptance criterion. Build:
   ```go
   release := pkg.Release{
       Repo: pkg.Repo{Owner: "bborbe", Name: "docker-utils", DefaultBranch: "master"},
       HeadSHA: "d630ef3526cfc57fbdccd9ba53c5c3a02945e407",
       CurrentVersion: "v1.7.7",
       UnreleasedBullets: 5,
       AutoRelease: false,
   }
   cmd := pkg.BuildCreateCommand(release, pkg.TaskConfig{Stage: "dev"})
   ```
   Assert (one `Expect` per field — clearer failure output):
   - `cmd.Frontmatter["task_type"] == "github-release"`
   - `cmd.Frontmatter["assignee"] == "github-releaser-agent"`
   - `cmd.Frontmatter["phase"] == "planning"`
   - `cmd.Frontmatter["status"] == "in_progress"`
   - `cmd.Frontmatter["stage"] == "dev"`
   - `cmd.Frontmatter["repo"] == "bborbe/docker-utils"`
   - `cmd.Frontmatter["clone_url"] == "git@github.com:bborbe/docker-utils.git"`
   - `cmd.Frontmatter["ref"] == "d630ef3526cfc57fbdccd9ba53c5c3a02945e407"`
   - `cmd.Frontmatter["current_version"] == "v1.7.7"`
   - `cmd.Frontmatter["task_identifier"]` is a non-empty UUID string equal to `pkg.DeriveTaskID("bborbe","docker-utils","d630ef3526cfc57fbdccd9ba53c5c3a02945e407").String()`
   - `string(cmd.TaskIdentifier) == cmd.Frontmatter["task_identifier"]` (the two are derived from the same source — coupling check)
   - `cmd.Title == "Release bborbe-docker-utils d630ef3"` (matches the existing `ComputeTaskTitle` output)

   b. **`It("BuildCreateCommand body is operator-readable header without bullet content")`** — assert:
   - `cmd.Body` starts with `"# Release: bborbe/docker-utils\n\n"`
   - `cmd.Body` contains `"**Current version:** v1.7.7"`
   - `cmd.Body` contains `"**HEAD:** d630ef3"`
   - `cmd.Body` contains `"https://github.com/bborbe/docker-utils/blob/master/CHANGELOG.md"`
   - `cmd.Body` does NOT contain `"- "` at the start of any line (no bullets — spec § 6 contract). Verify via `strings.Contains(cmd.Body, "\n- ")` == false.

   c. **`It("BuildCreateCommand stamps the stage from TaskConfig")`** — same release, `cfg := pkg.TaskConfig{Stage: "prod"}`, assert `cmd.Frontmatter["stage"] == "prod"`.

   d. **`It("BuildCreateCommand same inputs produce identical commands")`** — call `BuildCreateCommand` twice with the same release + cfg, assert the two `cmd.Frontmatter` maps are equal (`Expect(a.Frontmatter).To(Equal(b.Frontmatter))`) and the two `cmd.TaskIdentifier` values are equal. Determinism coupled to `DeriveTaskID`.

   `Describe("pkg.TaskPublisher", ...)` with:

   e. **`It("PublishCreate returns true and calls IncPublished(\"create\") on send success")`** — wire a `fakeSender := &mocks.CreateCommandSender{}` (constructed via the counterfeiter mock the agent lib already exposes; if no such mock exists in `pkg/mocks/`, define a tiny in-test stub satisfying the `task.CreateCommandSender` interface with a captured `SendCommandStub func(context.Context, task.CreateCommand) error` returning nil). Wire a `fakeMetrics := &mocks.Metrics{}` (from `pkg/mocks/metrics.go` — generated by the existing counterfeiter directive in `pkg/metrics.go`). Construct via `publisher := pkg.NewTaskPublisher(fakeSender, fakeMetrics, pkg.TaskConfig{Stage: "dev"})`. Call `publisher.PublishCreate(ctx, release)`. Assert: result is `true`; `fakeMetrics.IncPublishedCallCount() == 1` and the captured status arg is `"create"`; `fakeSender` recorded one `SendCommand` call whose `cmd.Frontmatter["task_type"] == "github-release"`.

   f. **`It("PublishCreate returns false and calls IncPublished(\"error\") on send failure")`** — same wiring but `fakeSender` returns `errors.New("kafka send failed")` (from `stderrors "errors"` import — test-only). Assert result is `false`, metric arg is `"error"`.

   If the agent lib does NOT provide a counterfeiter mock for `task.CreateCommandSender`, define an in-test fake in `taskpublisher_test.go`:
   ```go
   type fakeCreateCommandSender struct {
       sendErr      error
       capturedCmds []task.CreateCommand
   }

   func (f *fakeCreateCommandSender) SendCommand(_ context.Context, cmd task.CreateCommand) error {
       f.capturedCmds = append(f.capturedCmds, cmd)
       return f.sendErr
   }
   ```
   This satisfies the same `task.CreateCommandSender` interface that production wiring expects.

5. **Run unit tests**:
   ```bash
   cd watcher/github-release && make test
   ```

6. **Run full precommit**:
   ```bash
   cd watcher/github-release && make precommit
   ```

</requirements>

<constraints>
- Frontmatter contract is FROZEN per [[Agent Task File Contract]] and the Phase 1 evidence inlined in `<context>`. Every key in `buildFrontmatter` MUST match the spec § Desired Behavior #6 list verbatim AND the inlined `docker-utils d630ef3` regression row. Adding or removing a key breaks contract parity.
- `task_identifier` namespace UUID stays stable across releases (enforced in prompt 1 — `taskid.go` literal).
- Mirror `watcher/github-pr` Go patterns verbatim: `errors.Wrapf(ctx, err, ...)`, `glog.V(2).Infof`, counterfeiter-generated mocks in `pkg/mocks/`, Ginkgo v2 + Gomega, external `_test` packages.
- No `context.Background()` in production paths — `PublishCreate` receives ctx from the Watcher.
- No `fmt.Errorf` in production paths — N/A here (`PublishCreate` does not construct new errors; it logs and returns bool).
- Pre-init Prometheus counter label combinations are already in place in `pkg/metrics.go` — `IncPublished("create")` / `IncPublished("error")` labels are registered at package init. Do not duplicate.
- Body is operator-readable markdown ONLY — no shell interpolation, no template engine, no `## Unreleased` bullets embedded (agent does not parse body). Verified by the assertion that `cmd.Body` contains no `"\n- "` lines.
- Do NOT commit — dark-factory handles git.
- Do NOT modify any file outside `pkg/taskpublisher.go` and `pkg/taskpublisher_test.go` (plus the auto-generated `pkg/mocks/task_publisher.go` via `make generate`).
- Resolve the open-question comment in `<context>` against the current `ComputeTaskTitle` implementation. If the auditor needs the vault-file title shape, this prompt is the right place to swap — but the spec wording (skeleton-aligned) is the default.
</constraints>

<verification>
```bash
cd watcher/github-release

# No TODOs remain
grep -c "TODO" pkg/taskpublisher.go
# Expected: 0

# Frozen frontmatter keys all present
grep -F '"task_type":       "github-release"'          pkg/taskpublisher.go
grep -F '"assignee":        "github-releaser-agent"'   pkg/taskpublisher.go
grep -F '"phase":           "planning"'                pkg/taskpublisher.go
grep -F '"status":          "in_progress"'             pkg/taskpublisher.go
grep -F '"clone_url":'                                  pkg/taskpublisher.go
grep -F '"current_version":'                            pkg/taskpublisher.go

# Spec-named acceptance criterion verbatim
grep -F "BuildCreateCommand produces frontmatter task_type github-release for bborbe/docker-utils d630ef3" pkg/taskpublisher_test.go

# Mocks regenerated
ls pkg/mocks/task_publisher.go pkg/mocks/metrics.go

# Tests pass
make test

# Full precommit
make precommit
```
</verification>
