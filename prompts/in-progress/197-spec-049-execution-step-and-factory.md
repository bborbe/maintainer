---
status: approved
spec: [049-github-releaser-execution-phase-direct-push]
created: "2026-05-29T00:00:00Z"
queued: "2026-05-28T22:17:49Z"
---

<summary>
- Wires the execution phase end-to-end: creates `pkg/steps_execution.go` (the `ExecutionStep` implementing `agentlib.Step`) and adds the execution phase to the factory next to planning.
- The step reads the `## Plan` JSON written by planning, clones the target repo into an ephemeral workdir, rewrites the `## Unreleased` heading via the prompt-2 helper, commits + annotated-tags + pushes via the prompt-1 `GitOps`, then writes a typed `## Result` JSON (`released` or `failed`).
- Closed-enum error categorization: every failure path produces an `error_category` from the 8-value enum defined in prompt 1; substring classifier maps git stderr to the right bucket. The PR-fallback spec consumes `protected_branch_rejected`.
- Workdir lifetime is managed by the step: created under `os.TempDir()` per `task_identifier`, removed via `defer` on every exit path. Cleanup failure is logged via `glog.Warningf` but does not block return.
- Factory wires both planning AND execution phases together so the agent advances `planning → execution → done` on the happy path. Typed phase constants only — `domain.TaskPhaseExecution`, `domain.TaskPhaseAIReview`.
- Two integration tests with a counterfeiter-mocked `GitOps` cover the happy path (clone + rewrite + commit + tag + push + Result Done) and the protected-branch failure path (Result Failed with `error_category: protected_branch_rejected`).
</summary>

<objective>
Ship the execution step + factory wiring so the github-releaser agent advances a task from `phase: planning` (with a `## Plan` JSON `outcome: ready`) through `phase: execution` to `## Result` JSON `outcome: released` + `Status: Done` + `NextPhase: ai_review`. Failure paths produce `## Result` JSON `outcome: failed` + `error_category` from the closed enum + `Status: Failed`.

End state: `cd agent/github-releaser && make precommit` exits 0; coverage ≥ 75% on the new step file; the factory wires `agentlib.NewPhase(domain.TaskPhaseExecution, executionStep)` next to the existing planning phase; two integration tests (happy path + protected-branch failure) pass against the mocked `GitOps`; root `CHANGELOG.md` Unreleased section gains a `feat:` bullet mentioning `execution phase`.
</objective>

<context>
Read before writing code (repo-relative paths; container mounts repo root at `/workspace`):

- `CLAUDE.md` at repo root.
- `specs/in-progress/049-github-releaser-execution-phase-direct-push.md` — re-read Goal, Desired Behavior 3-8, Constraints, Failure Modes table (9 rows), Acceptance Criteria. This prompt covers behaviors 3-8 and ACs covering `pkg/steps_execution.go` + factory wiring + integration tests + the root CHANGELOG bullet.
- `agent/github-releaser/pkg/git/git.go` — produced by prompt 1; exports `GitOps` interface (4 methods Clone/Commit/Tag/Push), `NewOSExecGitOps()` zero-arg constructor, `DefaultBotIdentity()` accessor, and 8 `ErrorCategory` constants (plus the empty-string sentinel returned by `ClassifyError(nil)`). Mock at `pkg/git/mocks/git_ops.go` (type `GitOps` — counterfeiter v6 names: `CloneStub`/`CloneReturns`/`CloneCallCount`/`CloneArgsForCall`).
- `agent/github-releaser/pkg/git/error_classifier.go` — produced by prompt 1; exports `ClassifyError(err) ErrorCategory` and the 8-value `ErrorCategory` typed string enum (`ErrorCategoryAuth`, `ErrorCategoryRepoNotFound`, `ErrorCategoryChangelogMissing`, `ErrorCategoryUnreleasedNotFound`, `ErrorCategoryTagCollision`, `ErrorCategoryProtectedBranchRejected`, `ErrorCategoryPushNonFastForward`, `ErrorCategoryUnknown`).
- `agent/github-releaser/pkg/changelog/changelog.go` — extended by prompt 2; exports `RewriteUnreleasedHeader(content []byte, newHeader string) ([]byte, error)`.
- `agent/github-releaser/pkg/plan_output.go` — exports `PlanOutput` (read by the execution step). Fields used: `Outcome`, `NextVersion`, `NextVersionHeader`. `PlanOutcomeReady` constant equals `"ready"`.
- `agent/github-releaser/pkg/steps_planning.go` — read for `agentlib.Step` implementation shape (`Name`, `ShouldRun`, `Run`). The execution step mirrors this layout exactly: constructor takes deps, methods are pointer-receiver, returns `*agentlib.Result`.
- `agent/github-releaser/pkg/factory/factory.go` — read full file. The execution phase is added INSIDE `CreateAgent` next to the existing planning phase: change `agentlib.NewAgent(agentlib.NewPhase(domain.TaskPhasePlanning, planningStep))` to `agentlib.NewAgent(agentlib.NewPhase(domain.TaskPhasePlanning, planningStep), agentlib.NewPhase(domain.TaskPhaseExecution, executionStep))`. The factory also gains a new dependency: a `GitOps` instance, constructed once via a new `CreateGitOps()` factory function.
- `agent/github-releaser/pkg/steps_planning_test.go` lines 1-66 — integration-test pattern with counterfeiter mocks + `agentlib.ParseMarkdown` + `agentlib.ExtractSection[T]`. Mirror this exactly for the execution-step tests.
- `agent/github-releaser/main.go` — already exposes `GHToken` as a struct field + passes it to the factory. NO changes required to main.go in this prompt (the factory wiring change keeps the existing `CreateAgentProvider` signature stable; `GHToken` plumbs through unchanged).
- `agent/github-releaser/CHANGELOG.md` — repo root `CHANGELOG.md` (NOT the per-agent one). Current top of `## Unreleased` has spec 048 + spec 047 bullets; add ONE new bullet at the TOP.

Agent-lib API contract (already in go.mod, v0.63.11):
- `agentlib.Step` interface — 3 methods (`Name() string`, `ShouldRun(ctx, *Markdown) (bool, error)`, `Run(ctx, *Markdown) (*Result, error)`).
- `agentlib.Result{Status: AgentStatus, NextPhase: string, Message: string, ContinueToNext: bool}`. Status values: `AgentStatusDone`, `AgentStatusFailed`, `AgentStatusNeedsInput`. The execution step does NOT use `NeedsInput` — planning's escalation contract does not apply here. Failures are `AgentStatusFailed` (controller retries per its own cap).
- `agentlib.MarshalSectionTyped[T](ctx, heading, value) (Section, error)` — writes the `## Result` JSON section.
- `agentlib.ExtractSection[T](ctx, *Markdown, heading) (*T, error)` — reads `## Plan` and (in tests) reads back the just-written `## Result`.
- `md.ReplaceSection(section)` — idempotent in-place section update.
- `md.Frontmatter.String(key) (string, bool)` — read frontmatter strings (task_identifier, clone_url).
- `domain.TaskPhaseExecution`, `domain.TaskPhaseAIReview` from `github.com/bborbe/vault-cli/pkg/domain` — typed `TaskPhase` constants. NEVER use the string literal `"execution"` or `"ai_review"` in production code.

Coding-plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` — `bborbe/errors` patterns.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega + counterfeiter mocks.
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md` — counterfeiter mock method names (`*Returns`, `*CallCount`, `*ArgsForCall`).
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — Create* convention.
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` — Unreleased bullet format.
</context>

<requirements>

**Run order: do steps in sequence. Run `cd agent/github-releaser && go build ./...` after step 3 to catch type errors early. Run `cd agent/github-releaser && go test ./pkg/...` after step 5. Run `cd agent/github-releaser && make precommit` only as the final verification step.**

1. **Create `agent/github-releaser/pkg/result_output.go`** — typed `ResultOutput` struct for the `## Result` JSON section (NEW file, mirrors `pkg/plan_output.go`):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import "github.com/bborbe/maintainer/agent/github-releaser/pkg/git"

   // ResultOutput is the typed contract for the `## Result` JSON section the
   // execution step writes for every release task. Round-trips with
   // agentlib.MarshalSectionTyped + agentlib.ExtractSection[ResultOutput].
   //
   // Two shapes are valid:
   //   - Outcome="released" — direct-push succeeded; CommitSHA + Tag populated; ErrorCategory empty
   //   - Outcome="failed"   — any failure; ErrorCategory + Error populated; CommitSHA + Tag empty
   //
   // Future fields require a spec amendment.
   type ResultOutput struct {
       Outcome       string            `json:"outcome"`
       Path          string            `json:"path"`
       CommitSHA     string            `json:"commit_sha,omitempty"`
       Tag           string            `json:"tag,omitempty"`
       ErrorCategory git.ErrorCategory `json:"error_category,omitempty"`
       Error         string            `json:"error,omitempty"`
   }

   // Outcome values for ResultOutput.Outcome.
   const (
       ResultOutcomeReleased = "released"
       ResultOutcomeFailed   = "failed"
   )

   // Path values for ResultOutput.Path. Only one value today; the PR-fallback
   // spec will add a second (`"pr-merge"`).
   const ResultPathDirectPush = "direct-push"
   ```

2. **Create `agent/github-releaser/pkg/steps_execution.go`** — the execution step. Mirrors `steps_planning.go` structure (constructor takes deps, pointer-receiver methods, no global state):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
       "context"
       "os"
       "path/filepath"
       "strings"

       agentlib "github.com/bborbe/agent/lib"
       "github.com/bborbe/errors"
       domain "github.com/bborbe/vault-cli/pkg/domain"
       "github.com/golang/glog"

       "github.com/bborbe/maintainer/agent/github-releaser/pkg/changelog"
       "github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
   )

   // changelogFileName is the only file the execution step rewrites in the
   // cloned target repo. Spec 049 § Non-goals explicitly defers mono-repo
   // support (multiple CHANGELOGs in one repo).
   const changelogFileName = "CHANGELOG.md"

   // workdirPrefix is the os.TempDir-rooted prefix used for ephemeral clone
   // workdirs. Full path: <tempdir>/<workdirPrefix><task_identifier>/.
   // The directory is removed on every Run exit path via defer.
   const workdirPrefix = "github-releaser-"

   // executionStep implements agentlib.Step. Dependencies are constructor-injected;
   // no global state. Both ops (clone/commit/tag/push) and cloneURLBuilder are
   // mockable seams — the integration tests in steps_execution_test.go use a
   // counterfeiter GitOps mock and a stub URL builder.
   type executionStep struct {
       ops     git.GitOps
       ghToken string
   }

   // NewExecutionStep wires the execution step with its GitOps seam and the
   // GitHub token (used for HTTPS auth URL transformation). Empty ghToken
   // means clone goes out anonymously — fine for tests; production always
   // supplies a token.
   func NewExecutionStep(ops git.GitOps, ghToken string) agentlib.Step {
       return &executionStep{ops: ops, ghToken: ghToken}
   }

   // Name implements agentlib.Step.
   func (s *executionStep) Name() string { return "github-release-execute" }

   // ShouldRun returns true. The step is idempotent at the framework level:
   // a re-trigger overwrites ## Result. The controller's per-task lock
   // prevents concurrent invocations on the same task_identifier.
   func (s *executionStep) ShouldRun(_ context.Context, _ *agentlib.Markdown) (bool, error) {
       return true, nil
   }

   // Run executes the direct-push release pipeline. Sequence:
   //  1. Read & validate ## Plan(outcome=ready) + frontmatter
   //  2. Create ephemeral workdir under os.TempDir()
   //  3. Clone target repo via GitOps
   //  4. Read + rewrite CHANGELOG.md (## Unreleased → next header)
   //  5. Commit + annotated-tag + push
   //  6. Write ## Result(outcome=released) and return Done/NextPhase=ai_review
   //
   // Failures at any step produce ## Result(outcome=failed) + error_category
   // and return Status=Failed (controller retry per its cap).
   func (s *executionStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
       plan, err := agentlib.ExtractSection[PlanOutput](ctx, md, "## Plan")
       if err != nil || plan == nil {
           return s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Wrapf(ctx, err, "execution invoked but planning did not complete"))
       }
       if plan.Outcome != PlanOutcomeReady || plan.NextVersion == "" || plan.NextVersionHeader == "" {
           return s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Errorf(ctx, "execution invoked with non-ready plan: outcome=%s next_version=%q next_version_header=%q",
                   plan.Outcome, plan.NextVersion, plan.NextVersionHeader))
       }

       cloneURL, _ := md.Frontmatter.String("clone_url")
       ref, _ := md.Frontmatter.String("ref")
       taskID, _ := md.Frontmatter.String("task_identifier")
       if cloneURL == "" || ref == "" || taskID == "" {
           return s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Errorf(ctx, "missing frontmatter: clone_url=%q ref=%q task_identifier=%q",
                   cloneURL, ref, taskID))
       }

       // Workdir is per-task-identifier so concurrent runs on different tasks
       // don't collide. Re-running the same task removes any prior workdir
       // before clone (idempotent under replay).
       workdir := filepath.Join(os.TempDir(), workdirPrefix+taskID)
       if err := os.RemoveAll(workdir); err != nil {
           return s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Wrapf(ctx, err, "remove stale workdir: %s", workdir))
       }
       defer func() {
           if err := os.RemoveAll(workdir); err != nil {
               glog.Warningf("workdir cleanup failed: path=%s err=%v", workdir, err)
           }
       }()

       authedURL := s.injectToken(cloneURL)
       if err := s.ops.Clone(ctx, authedURL, ref, workdir); err != nil {
           return s.fail(ctx, md, git.ClassifyError(err), err)
       }

       changelogPath := filepath.Join(workdir, changelogFileName)
       content, err := os.ReadFile(changelogPath) // #nosec G304 -- workdir is os.TempDir-rooted; filename is the constant changelogFileName
       if err != nil {
           if os.IsNotExist(err) {
               return s.fail(ctx, md, git.ErrorCategoryChangelogMissing,
                   errors.Wrapf(ctx, err, "read %s", changelogPath))
           }
           return s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Wrapf(ctx, err, "read %s", changelogPath))
       }

       rewritten, err := changelog.RewriteUnreleasedHeader(content, plan.NextVersionHeader)
       if err != nil {
           return s.fail(ctx, md, git.ErrorCategoryUnreleasedNotFound,
               errors.Wrap(ctx, err, "rewrite ## Unreleased"))
       }
       if err := os.WriteFile(changelogPath, rewritten, 0o644); err != nil { // #nosec G306 -- standard CHANGELOG file perms (matches Phase 1 slash command)
           return s.fail(ctx, md, git.ErrorCategoryUnknown,
               errors.Wrapf(ctx, err, "write %s", changelogPath))
       }

       // plan.NextVersionHeader is "## vX.Y.Z" — strip the "## " prefix for the
       // tag name and commit subject.
       tagName := strings.TrimPrefix(plan.NextVersionHeader, "## ")
       commitMsg := "release " + tagName
       tagMsg := "release " + tagName

       sha, err := s.ops.Commit(ctx, workdir, commitMsg, changelogFileName)
       if err != nil {
           return s.fail(ctx, md, git.ClassifyError(err), err)
       }
       if err := s.ops.Tag(ctx, workdir, tagName, tagMsg); err != nil {
           return s.fail(ctx, md, git.ClassifyError(err), err)
       }
       // Push HEAD (the new commit) and the new tag in one push so a partial
       // failure cannot leave the remote with one but not the other.
       if err := s.ops.Push(ctx, workdir, "HEAD", "refs/tags/"+tagName); err != nil {
           return s.fail(ctx, md, git.ClassifyError(err), err)
       }

       output := ResultOutput{
           Outcome:   ResultOutcomeReleased,
           Path:      ResultPathDirectPush,
           CommitSHA: sha,
           Tag:       tagName,
       }
       section, err := agentlib.MarshalSectionTyped(ctx, "## Result", output)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "marshal ## Result section")
       }
       md.ReplaceSection(section)

       return &agentlib.Result{
           Status:    agentlib.AgentStatusDone,
           NextPhase: string(domain.TaskPhaseAIReview),
       }, nil
   }

   // injectToken transforms an HTTPS GitHub URL into a token-authenticated form.
   // https://github.com/owner/repo.git → https://x-access-token:<token>@github.com/owner/repo.git
   // Empty token returns the input unchanged (anonymous; fine for tests).
   func (s *executionStep) injectToken(cloneURL string) string {
       if s.ghToken == "" {
           return cloneURL
       }
       const prefix = "https://"
       if !strings.HasPrefix(cloneURL, prefix) {
           return cloneURL
       }
       return prefix + "x-access-token:" + s.ghToken + "@" + strings.TrimPrefix(cloneURL, prefix)
   }

   // fail writes a ## Result(outcome=failed) section with the supplied
   // error_category + error string, and returns Status=Failed for controller
   // retry. The workdir cleanup defer in Run still runs after this returns.
   func (s *executionStep) fail(
       ctx context.Context,
       md *agentlib.Markdown,
       category git.ErrorCategory,
       cause error,
   ) (*agentlib.Result, error) {
       msg := ""
       if cause != nil {
           msg = cause.Error()
       }
       output := ResultOutput{
           Outcome:       ResultOutcomeFailed,
           Path:          ResultPathDirectPush,
           ErrorCategory: category,
           Error:         msg,
       }
       section, err := agentlib.MarshalSectionTyped(ctx, "## Result", output)
       if err != nil {
           // Failing to marshal the failure is a real error — surface it so
           // the framework records the panic-equivalent rather than swallowing.
           return nil, errors.Wrapf(ctx, err, "marshal ## Result section (failed)")
       }
       md.ReplaceSection(section)

       glog.V(2).Infof("execution failed: category=%s err=%v", category, cause)
       return &agentlib.Result{
           Status:  agentlib.AgentStatusFailed,
           Message: msg,
       }, nil
   }
   ```

   Notes:
   - `NewExecutionStep` constructor anchored at column 1 — AC grep `grep -c '^func NewExecutionStep(' pkg/steps_execution.go` returns 1.
   - The literal substring `"workdir cleanup failed"` appears in the `glog.Warningf` line — AC grep target.
   - All errors via `bborbe/errors`. NO `fmt.Errorf`.
   - `injectToken` is private — its only entry point is the production code path. No public surface, no separate test (the happy-path integration test exercises it via mocked Clone, which receives the transformed URL).

3. **Create `agent/github-releaser/pkg/steps_execution_test.go`** — external test package (`package pkg_test`). Two Ginkgo cases minimum (happy path + protected-branch failure). Mirror the imports + setup from `steps_planning_test.go`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg_test

   import (
       "context"
       "os"
       "path/filepath"

       agentlib "github.com/bborbe/agent/lib"
       "github.com/bborbe/errors"
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"
       "github.com/bborbe/maintainer/agent/github-releaser/pkg/git"
       gitmocks "github.com/bborbe/maintainer/agent/github-releaser/pkg/git/mocks"
   )

   var _ = Describe("ExecutionStep", func() {
       const taskMD = `---
   status: in_progress
   phase: execution
   assignee: github-releaser-agent
   task_type: github-release
   repo: bborbe/example
   clone_url: https://github.com/bborbe/example.git
   ref: master
   current_version: v1.2.7
   task_identifier: gh-release-bborbe-example-master-049a
   ---

   # release task

   ## Plan

   ` + "```json" + `
   {
     "outcome": "ready",
     "bump": "patch",
     "reasoning": "fix-only batch",
     "current_version": "v1.2.7",
     "next_version": "1.2.8",
     "next_version_header": "## v1.2.8",
     "header_prefix_style": "v",
     "bullets": ["fix: thing"]
   }
   ` + "```" + `
   `

       writeChangelog := func(workdir string) {
           Expect(os.MkdirAll(workdir, 0o755)).To(Succeed())
           content := []byte("# Changelog\n\n## Unreleased\n\n- fix: thing\n\n## v1.2.6\n\n- old\n")
           Expect(os.WriteFile(filepath.Join(workdir, "CHANGELOG.md"), content, 0o644)).To(Succeed())
       }

       Context("happy path", func() {
           It("clones, rewrites, commits, tags, pushes; writes ## Result(released); returns Done/NextPhase=ai_review", func() {
               fakeOps := &gitmocks.GitOps{}

               // Capture the workdir that the step passed to Clone so we can
               // write a CHANGELOG.md there before Commit reads it.
               fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
                   writeChangelog(workdir)
                   return nil
               }

               // Per spec AC #11(e): the bytes on disk at the moment Commit is
               // invoked MUST contain `## v1.2.8` AND NOT contain `## Unreleased`.
               // This proves RewriteUnreleasedHeader ran BEFORE Commit, not as
               // a hardcoded JSON-output-only step. Read the CHANGELOG inside
               // the stub (before the defer cleanup runs).
               fakeOps.CommitStub = func(_ context.Context, workdir, _ string, _ ...string) (string, error) {
                   content, readErr := os.ReadFile(filepath.Join(workdir, "CHANGELOG.md"))
                   Expect(readErr).NotTo(HaveOccurred())
                   Expect(string(content)).To(ContainSubstring("## v1.2.8"))
                   Expect(string(content)).NotTo(ContainSubstring("## Unreleased"))
                   return "abc1234", nil
               }
               fakeOps.TagReturns(nil)
               fakeOps.PushReturns(nil)

               step := pkg.NewExecutionStep(fakeOps, "test-token")
               md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
               Expect(err).NotTo(HaveOccurred())

               result, err := step.Run(context.Background(), md)
               Expect(err).NotTo(HaveOccurred())
               Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
               Expect(result.NextPhase).To(Equal("ai_review"))

               // All 4 GitOps methods called exactly once.
               Expect(fakeOps.CloneCallCount()).To(Equal(1))
               Expect(fakeOps.CommitCallCount()).To(Equal(1))
               Expect(fakeOps.TagCallCount()).To(Equal(1))
               Expect(fakeOps.PushCallCount()).To(Equal(1))

               // Tag name + message verbatim from plan.next_version_header[3:].
               _, _, tagName, tagMsg := fakeOps.TagArgsForCall(0)
               Expect(tagName).To(Equal("v1.2.8"))
               Expect(tagMsg).To(Equal("release v1.2.8"))

               // Commit message uses the same canonical "release v1.2.8".
               _, _, commitMsg, _ := fakeOps.CommitArgsForCall(0)
               Expect(commitMsg).To(Equal("release v1.2.8"))

               // ## Result body shape.
               got, err := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
               Expect(err).NotTo(HaveOccurred())
               Expect(got.Outcome).To(Equal("released"))
               Expect(got.Path).To(Equal("direct-push"))
               Expect(got.CommitSHA).To(Equal("abc1234"))
               Expect(got.Tag).To(Equal("v1.2.8"))
               Expect(string(got.ErrorCategory)).To(BeEmpty())

               // Clone URL had token injected.
               _, gotCloneURL, _, _ := fakeOps.CloneArgsForCall(0)
               Expect(gotCloneURL).To(Equal("https://x-access-token:test-token@github.com/bborbe/example.git"))
           })
       })

       Context("protected_branch_rejected", func() {
           It("Push fails with GH006 → Result(failed, error_category=protected_branch_rejected); Status=Failed; Tag was called", func() {
               fakeOps := &gitmocks.GitOps{}
               fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
                   writeChangelog(workdir)
                   return nil
               }
               fakeOps.CommitReturns("def5678", nil)
               fakeOps.TagReturns(nil)
               // Realistic GH006 protected-branch error from `git push`.
               fakeOps.PushReturns(errors.Errorf(context.Background(),
                   "git push: remote: error: GH006: Protected branch update failed for refs/heads/master.\nremote: error: At least 1 approving review is required."))

               step := pkg.NewExecutionStep(fakeOps, "")
               md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
               Expect(err).NotTo(HaveOccurred())

               result, err := step.Run(context.Background(), md)
               Expect(err).NotTo(HaveOccurred())
               Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))

               // Tag + Push were called (proves failure surfaces post-tag, not pre-commit).
               Expect(fakeOps.TagCallCount()).To(Equal(1))
               Expect(fakeOps.PushCallCount()).To(Equal(1))

               got, err := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
               Expect(err).NotTo(HaveOccurred())
               Expect(got.Outcome).To(Equal("failed"))
               Expect(string(got.ErrorCategory)).To(Equal("protected_branch_rejected"))
               Expect(got.CommitSHA).To(BeEmpty())
               Expect(got.Tag).To(BeEmpty())
           })
       })

       Context("workdir cleanup observability", func() {
           // The cleanup-failure path is hard to trigger from a unit test
           // (would require an unwritable parent dir). This test instead
           // asserts the log message constant is in source so the
           // observability AC grep is satisfied AND the defer block does
           // run on the happy path (proven by stat).
           It("removes the workdir after Run completes", func() {
               fakeOps := &gitmocks.GitOps{}
               capturedWorkdir := ""
               fakeOps.CloneStub = func(_ context.Context, _, _, workdir string) error {
                   capturedWorkdir = workdir
                   writeChangelog(workdir)
                   return nil
               }
               fakeOps.CommitReturns("abc1234", nil)
               step := pkg.NewExecutionStep(fakeOps, "")
               md, err := agentlib.ParseMarkdown(context.Background(), taskMD)
               Expect(err).NotTo(HaveOccurred())

               _, err = step.Run(context.Background(), md)
               Expect(err).NotTo(HaveOccurred())

               Expect(capturedWorkdir).NotTo(BeEmpty())
               _, statErr := os.Stat(capturedWorkdir)
               Expect(os.IsNotExist(statErr)).To(BeTrue(), "workdir %s should be removed after Run", capturedWorkdir)
           })
       })

       Context("plan output validation", func() {
           It("non-ready plan → Result(failed, error_category=unknown); Status=Failed; Clone NOT called", func() {
               nonReadyMD := `---
   status: in_progress
   phase: execution
   task_identifier: gh-release-x-y-master-049b
   clone_url: https://github.com/x/y.git
   ref: master
   ---

   ## Plan

   ` + "```json" + `
   {"outcome":"needs_input","reason":"upstream changelog regression"}
   ` + "```" + `
   `
               fakeOps := &gitmocks.GitOps{}
               step := pkg.NewExecutionStep(fakeOps, "")
               md, err := agentlib.ParseMarkdown(context.Background(), nonReadyMD)
               Expect(err).NotTo(HaveOccurred())

               result, err := step.Run(context.Background(), md)
               Expect(err).NotTo(HaveOccurred())
               Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
               Expect(fakeOps.CloneCallCount()).To(Equal(0))

               got, _ := agentlib.ExtractSection[pkg.ResultOutput](context.Background(), md, "## Result")
               Expect(got.Outcome).To(Equal("failed"))
               Expect(string(got.ErrorCategory)).To(Equal("unknown"))
           })
       })
   })
   ```

   Notes:
   - Mock method names follow counterfeiter conventions: `Clone` interface method → `CloneReturns`, `CloneStub`, `CloneCallCount`, `CloneArgsForCall`. If the generated mock uses different names (verify by reading `pkg/git/mocks/git_ops.go` once prompt 1 lands), adjust.
   - `CloneStub` is used (not `CloneReturns`) for the happy path because the test must write a `CHANGELOG.md` into the workdir before `os.ReadFile` runs.
   - The literal `"v1.2.8"` appears twice (commit + tag assertions) — satisfies AC grep `grep -c '"v1.2.8"'` ≥ 1.
   - The literal `"protected_branch_rejected"` appears once in the failure-path test — satisfies AC grep `grep -c 'protected_branch_rejected'` ≥ 1.

4. **Modify `agent/github-releaser/pkg/factory/factory.go`** — add execution-phase wiring. Three changes:

   a. Add a new `CreateGitOps()` function:
   ```go
   // CreateGitOps returns the production GitOps implementation, wired with
   // the Phase 1 verbatim bot identity. Pure plumbing.
   func CreateGitOps() git.GitOps {
       return git.NewOSExecGitOps()
   }
   ```

   b. Add the import for `git "github.com/bborbe/maintainer/agent/github-releaser/pkg/git"` to the import block.

   c. Modify `CreateAgent` to add the execution phase:
   ```go
   func CreateAgent(
       claudeConfigDir claudelib.ClaudeConfigDir,
       agentDir claudelib.AgentDir,
       model claudelib.ClaudeModel,
       ghToken string,
       env map[string]string,
   ) *agentlib.Agent {
       planningRunner := CreateClaudeRunner(claudeConfigDir, agentDir, model, env, planningTools)
       fetcher := githubchangelog.NewHTTPFetcher(ghToken)
       planningStep := releaserpkg.NewPlanningStep(planningRunner, fetcher)

       executionOps := CreateGitOps()
       executionStep := releaserpkg.NewExecutionStep(executionOps, ghToken)

       return agentlib.NewAgent(
           agentlib.NewPhase(domain.TaskPhasePlanning, planningStep),
           agentlib.NewPhase(domain.TaskPhaseExecution, executionStep),
       )
   }
   ```

   The signature of `CreateAgent` is UNCHANGED — no main.go edits required. `ghToken` already plumbs through.

5. **Extend `agent/github-releaser/pkg/factory/factory_test.go`** with a regression test for the execution-phase wiring. Append to the existing `Describe("CreateAgentProvider", ...)`:

   ```go
   It("CreateAgent wires both planning and execution phases", func() {
       agent := factory.CreateAgent(
           claudelib.ClaudeConfigDir("/tmp/claude"),
           claudelib.AgentDir("/tmp/agent"),
           claudelib.ClaudeModel("sonnet"),
           "",
           map[string]string{},
       )
       Expect(agent).NotTo(BeNil())
       // The agent-lib does not expose the phase list on *Agent; the
       // assertion above plus the grep-AC on factory.go (`NewPhase(domain.TaskPhaseExecution`)
       // covers the structural guarantee. This test additionally ensures
       // CreateAgent does not panic and returns a non-nil Agent — which it
       // would not, if the second phase argument were malformed.
   })
   ```

6. **Update root `CHANGELOG.md`** — add ONE new bullet at the TOP of the `## Unreleased` block. The bullet MUST contain the literal substring `execution phase` so the AC grep `awk '/^## Unreleased$/,/^## v/' CHANGELOG.md | grep -c 'execution phase'` returns ≥ 1:

   ```
   ## Unreleased

   - feat(agent/github-releaser): wire execution phase direct-push path — adds pkg/git GitOps interface + osExecGitOps shell-out impl + 8-category error classifier; extends pkg/changelog with RewriteUnreleasedHeader; adds pkg/steps_execution ExecutionStep that clones target repo, rewrites ## Unreleased → next version header, commits + annotated tags + pushes via GitOps; factory wires planning + execution phases together (spec 049)
   - fix(agent/github-releaser): planning escalation now returns ...  (existing — leave unchanged)
   ```

7. **Coverage check** — from `agent/github-releaser/`:

   ```bash
   go test -cover ./pkg/...
   ```

   `pkg/git/...` ≥ 75% (from prompt 1); `pkg/changelog/...` ≥ 90% (from prompt 2); `pkg/steps_execution.go` ≥ 75% via the four Ginkgo cases above (happy path + protected-branch + cleanup observability + plan validation). If steps_execution coverage drops below 75%, add a `tag_collision` case mirroring the protected-branch case but with `TagReturns(errors.Errorf(..., "git tag: fatal: tag 'v1.2.8' already exists"))`.

8. **Final verification** — from `agent/github-releaser/`:

   ```bash
   make precommit
   ```

   Must exit 0. No `fmt.Errorf` in any new file. No raw string literal `"execution"` or `"ai_review"` in `pkg/factory/factory.go` or `pkg/steps_execution.go` (main.go and cmd/run-task/main.go are exempted per spec 047 § Constraints amendment — libargument struct-tag defaults must be string literals).

</requirements>

<constraints>
- New files:
  - `agent/github-releaser/pkg/steps_execution.go`
  - `agent/github-releaser/pkg/steps_execution_test.go`
  - `agent/github-releaser/pkg/result_output.go`
- Modified files:
  - `agent/github-releaser/pkg/factory/factory.go` (add `CreateGitOps`, add execution phase to `CreateAgent`, add `git` import)
  - `agent/github-releaser/pkg/factory/factory_test.go` (append one regression test case)
  - `CHANGELOG.md` at repo root (one new Unreleased bullet at the top)
- Frozen signature: `func NewExecutionStep(ops git.GitOps, ghToken string) agentlib.Step`. Anchored at column 1 (AC grep target).
- Step `Name()` returns the literal `"github-release-execute"`.
- Phase constants typed: `domain.TaskPhaseExecution` and `domain.TaskPhaseAIReview` from `github.com/bborbe/vault-cli/pkg/domain`. NEVER use the string literal `"execution"` or `"ai_review"` in `pkg/steps_execution.go` or `pkg/factory/factory.go`. Per spec § Constraints AND the spec 047 amendment for `pkg/`-scoped grep.
- `## Result` body shape: typed `ResultOutput` struct with `outcome`, `path`, `commit_sha`, `tag`, `error_category`, `error` fields. JSON via `agentlib.MarshalSectionTyped`. NO raw `strings.Index` for sections.
- Error categories from the 8-value enum exported by `pkg/git`. NO new string literals — `git.ErrorCategoryProtectedBranchRejected` etc. only.
- Errors via `github.com/bborbe/errors` (`Wrap`/`Wrapf`/`Errorf`). NO `fmt.Errorf` (banned by AC).
- Workdir under `os.TempDir()` with prefix `"github-releaser-"` + `task_identifier`. Removed on EVERY exit path via `defer os.RemoveAll`. Removal-failure logged via `glog.Warningf` matching the literal string `workdir cleanup failed`.
- HTTPS auth: token injected client-side via `https://x-access-token:<token>@github.com/<owner>/<repo>.git` URL transformation. Empty token returns input unchanged.
- Annotated tag via `GitOps.Tag` (no lightweight tags — enforced at the `pkg/git/` impl layer by prompt 1).
- Commit message + tag message: `"release vX.Y.Z"`. Phase 1 verbatim.
- Push covers commit + tag in ONE call (`refs := []string{"HEAD", "refs/tags/vX.Y.Z"}`) so partial failure cannot leave remote with one but not the other.
- Coverage targets per package: `pkg/git/` ≥ 75%, `pkg/changelog/` ≥ 90%, `pkg/steps_execution.go` ≥ 75% (sub-target measured at file granularity per the spec § Verification block).
- `## Result` JSON `outcome` values: `"released"` (success) or `"failed"` (failure). Constants `ResultOutcomeReleased`, `ResultOutcomeFailed`.
- `path` value: `"direct-push"` (only value today). Constant `ResultPathDirectPush`.
- License header (3 lines) at the top of every new `.go` file.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass: `cd agent/github-releaser && make test` is green before AND after (planning step, factory provider, etc.).
- Root `CHANGELOG.md` bullet at the TOP of the `## Unreleased` block (not appended) — preserves chronological order of recent specs.
</constraints>

<verification>

Run from repo root unless noted.

```bash
# Build + tests + coverage
cd agent/github-releaser && make precommit                              # exit 0
cd agent/github-releaser && go test -cover ./pkg/...                    # all green
cd agent/github-releaser && go test -cover ./pkg/git/...                # ≥ 75%
cd agent/github-releaser && go test -cover ./pkg/changelog/...          # ≥ 90%
cd agent/github-releaser && go test -cover -run 'ExecutionStep' ./pkg/  # ExecutionStep cases pass

# Files exist
ls agent/github-releaser/pkg/steps_execution.go                          # exists
ls agent/github-releaser/pkg/steps_execution_test.go                     # exists
ls agent/github-releaser/pkg/result_output.go                            # exists

# Frozen step constructor + name
grep -c '^func NewExecutionStep(' agent/github-releaser/pkg/steps_execution.go              # =1
grep -c '"github-release-execute"' agent/github-releaser/pkg/steps_execution.go             # ≥1

# Factory wired both phases
grep -c 'agentlib.NewPhase(domain.TaskPhaseExecution' agent/github-releaser/pkg/factory/factory.go    # =1
grep -c 'agentlib.NewPhase(domain.TaskPhasePlanning'  agent/github-releaser/pkg/factory/factory.go    # =1

# Typed-constant gates (production code only — main.go + cmd/run-task/main.go exempt)
grep -cE '"(execution|ai_review)"' agent/github-releaser/pkg/factory/factory.go             # =0
grep -cE '"(execution|ai_review)"' agent/github-releaser/pkg/steps_execution.go             # =0

# Error-wrapping convention
grep -c 'fmt.Errorf' agent/github-releaser/pkg/steps_execution.go                           # =0
grep -c 'fmt.Errorf' agent/github-releaser/pkg/factory/factory.go                           # =0

# Mocked happy path + failure path markers
grep -c 'CommitCallCount' agent/github-releaser/pkg/steps_execution_test.go                 # ≥1
grep -c 'TagCallCount'    agent/github-releaser/pkg/steps_execution_test.go                 # ≥1
grep -c '"v1.2.8"'        agent/github-releaser/pkg/steps_execution_test.go                 # ≥1
grep -c 'protected_branch_rejected' agent/github-releaser/pkg/steps_execution_test.go       # ≥1

# Workdir cleanup observability
grep -c 'workdir cleanup failed' agent/github-releaser/pkg/steps_execution.go               # ≥1

# Root CHANGELOG bullet within Unreleased section
awk '/^## Unreleased$/,/^## v/' CHANGELOG.md | grep -c 'execution phase'                    # ≥1
```

</verification>
