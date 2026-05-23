---
status: committing
spec: [034-pr-reviewer-always-post-review]
summary: 'Implemented always-post-review invariant: planning phase now POSTs an LGTM COMMENT review when concerns are empty, eliminating silent-skip path; vault task gains ## Verdict section naming review id and event'
container: maintainer-exec-130-pr-reviewer-always-post-review-core
dark-factory-version: v0.164.0
created: "2026-05-23T00:00:00Z"
queued: "2026-05-23T11:30:19Z"
started: "2026-05-23T11:30:20Z"
---

<summary>
- Planning phase now branches on `concerns: []` vs non-empty: empty concerns POST an LGTM `COMMENT` review via `PrPoster` then advance to `phase: done`; non-empty proceeds to `in_progress` unchanged
- A new `planningStep` type replaces the generic `claudelib.NewAgentStep` for the planning phase; it wraps the Claude runner, writes `## Plan`, then inspects the parsed concerns array to decide routing
- `PrPoster` interface gains `PostLGTM(ctx, prInfo, headSHA, workDir, botLogin) PostResult` — concrete `prPoster` implements it as `event=COMMENT` with body `Reviewed by &lt;BotLogin&gt; — no concerns flagged.`
- `prPoster` is now injected into the planning step (alongside `ghTokenCheckStep` and the runner) so the empty-concerns path can POST without needing a full checkout
- Vault task file gains `## Verdict` section after the LGTM POST — names review id and `COMMENT` event
- On POST failure (network/GitHub 5xx/4xx), task escalates to `human_review` with error wrapped via `github.com/bborbe/errors`
- All new errors use `github.com/bborbe/errors`; no `fmt.Errorf` / stdlib `errors.New` in modified files
- BSD-style license header on every new `.go` file
- Mock for `PrPoster` regenerated to include `PostLGTM`
- CHANGELOG entry under `## Unreleased`
</summary>

<objective>
Implement the always-post-review invariant for the PR reviewer agent: every successful planning run produces at least one visible artifact on the GitHub PR. Empty concerns path: POST an LGTM `COMMENT` review then advance to `done`. Non-empty path: unchanged (advance to `in_progress`). Add `## Verdict` section to vault after planning.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface/constructor/struct pattern.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks, coverage ≥80%.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `bborbe/errors`, never `fmt.Errorf`.
Read `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` for which test types to write.

**Files to read fully before making any changes:**
- `agent/pr-reviewer/pkg/poster_types.go` — full file; add `PostLGTM` to `PrPoster` interface
- `agent/pr-reviewer/pkg/githubposter/poster.go` — full file; implement `PostLGTM` on `*prPoster`, understand `postReview` and `verifyAfterPost` as the posting primitive
- `agent/pr-reviewer/pkg/factory/factory.go` — full file; understand `CreateAgent` and how `planningStep` is constructed; `prPoster` and `botLogin` are already available in this function
- `agent/pr-reviewer/pkg/steps_checkout_execution.go` — full file; understand `postAndRoute` pattern, `appendDiagnosticsSection`, `buildDiagnosticBlock`, and `ExtractPRURL` — reuse these patterns for the planning step's LGTM path
- `agent/pr-reviewer/pkg/steps_review.go` — full file; understand `reviewStep` structure as a model for the new `planningStep`
- `agent/pr-reviewer/pkg/prompts/planning_output-format.md` — full file; understand the `concerns` JSON field shape (`[]` means empty)
- `agent/pr-reviewer/pkg/githubposter/types.go` — full file; confirm `DefaultBotLogin` constant value
- `agent/pr-reviewer/mocks/pr-poster.go` — full file; structure of the counterfeiter mock for `PrPoster`; this file is regenerated after step 2

**Key design decisions:**

1. `PostLGTM` is added to the `PrPoster` interface (not just the concrete type) so the mock can be injected in tests. The method takes `botLogin string` as a parameter — the concrete `prPoster` stores `botLogin` at construction time, but the interface signature carries it explicitly to avoid a type assertion in the caller.

2. `planningStep` is a new step type in `pkg` (not `pkg/factory`). It is constructed by the factory with access to `prPoster` and `botLogin`. It wraps a `claudelib.ClaudeRunner` for the actual LLM call.

3. The `## Verdict` section is written by `planningStep` after the LGTM POST succeeds. The section body names the review id and `COMMENT` event. For the non-empty-concerns path, `## Verdict` is written later by `reviewStep` (existing behavior) — this spec does not change that.

4. The planning step does NOT need a worktree — the LGTM POST path uses `prInfo` and `headSHA` from the vault frontmatter and workDir="" (no `.pr-reviewer.yaml` lookup needed for LGTM). This avoids a full clone just to post an LGTM comment.

**Dependency check — run before making any changes:**

```bash
# Confirm DefaultBotLogin is defined in githubposter/types.go:
grep -n "DefaultBotLogin" agent/pr-reviewer/pkg/githubposter/types.go
# Expected: const DefaultBotLogin = "ben-s-pull-request-reviewer[bot]"

# Confirm PrPoster.Post signature hasn't drifted:
grep -n "Post(ctx context.Context, req PostRequest) PostResult" agent/pr-reviewer/pkg/poster_types.go
# Expected: one match
```
</context>

<requirements>
**Execute steps in order. Run `make test` after step 5. Run `make precommit` only at the final step.**

1. **Add `PostLGTM` to `PrPoster` interface in `agent/pr-reviewer/pkg/poster_types.go`**

   Add after the `Post` method declaration:

   ```go
   // PostLGTM posts an LGTM COMMENT review when planning finds no concerns.
   // event is always "COMMENT"; body is "Reviewed by <botLogin> — no concerns flagged."
   // workDir is optional (empty string is fine — no .pr-reviewer.yaml lookup needed for LGTM).
   // On success, returns a PostResult with Outcome="success" and PostedEvent="COMMENT".
   // On failure, returns a PostResult with Outcome="failed" and ErrorClass/ErrorMessage set.
   PostLGTM(ctx context.Context, pr PRInfo, headSHA, workDir, botLogin string) PostResult
   ```

2. **Implement `PostLGTM` on `*prPoster` in `agent/pr-reviewer/pkg/githubposter/poster.go`**

   Add after the `Post` method:

   ```go
   // PostLGTM posts a COMMENT review with body "Reviewed by <botLogin> — no concerns flagged."
   // workDir is ignored (no .pr-reviewer.yaml lookup needed for LGTM).
   // Verdict is not applicable — always COMMENT event.
   func (p *prPoster) PostLGTM(
       ctx context.Context,
       pr PRInfo,
       headSHA, workDir, botLogin string,
   ) PostResult {
       start := time.Now()
       const event = "COMMENT"
       body := fmt.Sprintf("Reviewed by %s — no concerns flagged.", botLogin)

       reviewID, result, proceed := p.postReview(ctx, pr, headSHA, event, body)
       if !proceed {
           result.ElapsedMs = time.Since(start).Milliseconds()
           return result
       }
       result = p.verifyAfterPost(ctx, pr, headSHA, event, nil)
       result.ReviewID = reviewID
       result.PostedEvent = event
       result.ElapsedMs = time.Since(start).Milliseconds()
       return result
   }
   ```

   Add `"time"` to the import block in `poster.go` if not already present.

3. **Regenerate `PrPoster` mock** to include the new `PostLGTM` method:

   ```bash
   cd agent/pr-reviewer && go generate ./pkg/...
   ```

   If the directive doesn't trigger counterfeiter, run it directly:

   ```bash
   cd agent/pr-reviewer && \
     go run github.com/maxbrunsfeld/counterfeiter/v6 \
       -o mocks/pr-poster.go \
       --fake-name PrPoster \
       ./pkg/. PrPoster
   ```

   Confirm `mocks/pr-poster.go` now includes `PostLGTMStub`, `PostLGTMCallCount`, `PostLGTMArgsForCall`, `PostLGTMReturns`.

4. **Create `agent/pr-reviewer/pkg/steps_planning.go`** — the new `planningStep` type:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg

   import (
       "context"
       "encoding/json"
       "fmt"
       "strings"

       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"
       "github.com/bborbe/errors"
       "github.com/golang/glog"
   )

   // planningOutput is the parsed shape of the ## Plan JSON block.
   type planningOutput struct {
       Concerns []struct{} `json:"concerns"`
   }

   // planningStep runs Claude to produce the ## Plan section, then branches:
   // - concerns empty → POST LGTM via PrPoster → write ## Verdict → done
   // - concerns non-empty → advance to in_progress
   type planningStep struct {
       runner       claudelib.ClaudeRunner
       instructions claudelib.Instructions
       prPoster     PrPoster // nil = skip posting (cmd/run-task mode)
       botLogin     string
   }

   // NewPlanningStep constructs the planning-phase step.
   // prPoster may be nil (local CLI mode).
   func NewPlanningStep(
       runner claudelib.ClaudeRunner,
       instructions claudelib.Instructions,
       prPoster PrPoster,
       botLogin string,
   ) agentlib.Step {
       return &planningStep{
           runner:       runner,
           instructions: instructions,
           prPoster:     prPoster,
           botLogin:     botLogin,
       }
   }

   // Name implements agentlib.Step.
   func (s *planningStep) Name() string { return "pr-plan" }

   // ShouldRun returns false if ## Plan already exists (idempotent).
   func (s *planningStep) ShouldRun(_ context.Context, md *agentlib.Markdown) (bool, error) {
       _, exists := md.FindSection("## Plan")
       return !exists, nil
   }

   // Run calls Claude with the planning prompt, writes ## Plan, parses concerns,
   // and routes: empty → LGTM POST → done; non-empty → in_progress.
   func (s *planningStep) Run(ctx context.Context, md *agentlib.Markdown) (*agentlib.Result, error) {
       taskContent, err := md.Marshal(ctx)
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "planning marshal task")
       }

       prompt := claudelib.BuildPrompt(s.instructions.String(), nil, taskContent)
       runResult, runErr := s.runner.Run(ctx, prompt)
       if runErr != nil {
           return &agentlib.Result{
               Status:  agentlib.AgentStatusFailed,
               Message: fmt.Sprintf("planning claude run failed: %v", runErr),
           }, nil
       }

       // Write ## Plan to vault first (vault-first, same invariant as ## Review).
       md.ReplaceSection(agentlib.Section{
           Heading: "## Plan",
           Body:    runResult.Result,
       })

       // Parse concerns from ## Plan body.
       concerns, parseErr := parsePlanningConcerns(ctx, runResult.Result)
       if parseErr != nil {
           // Malformed JSON in ## Plan is a planning failure — escalate.
           return &agentlib.Result{
               Status:    agentlib.AgentStatusDone,
               NextPhase: "human_review",
               Message:   fmt.Sprintf("planning: failed to parse ## Plan JSON: %v", parseErr),
           }, nil
       }

       if len(concerns) == 0 {
           // Empty concerns — LGTM path.
           return s.postLGTMAndDone(ctx, md)
       }

       // Non-empty concerns — advance to in_progress.
       return &agentlib.Result{
           Status:    agentlib.AgentStatusDone,
           NextPhase: "in_progress",
       }, nil
   }

   // postLGTMAndDone posts an LGTM COMMENT review and writes ## Verdict.
   func (s *planningStep) postLGTMAndDone(
       ctx context.Context,
       md *agentlib.Markdown,
   ) (*agentlib.Result, error) {
       prURLStr := ExtractPRURL(md)
       if prURLStr == "" {
           return &agentlib.Result{
               Status:    agentlib.AgentStatusDone,
               NextPhase: "human_review",
               Message:   "planning: no GitHub PR URL found — cannot post LGTM",
           }, nil
       }

       prInfo, parseErr := ParsePRURL(ctx, prURLStr)
       if parseErr != nil {
           return &agentlib.Result{
               Status:    agentlib.AgentStatusDone,
               NextPhase: "human_review",
               Message:   fmt.Sprintf("planning: failed to parse PR URL %q: %v", prURLStr, parseErr),
           }, nil
       }

       if prInfo.Platform != PlatformGitHub {
           // Non-GitHub — skip posting, advance to done.
           return &agentlib.Result{
               Status:    agentlib.AgentStatusDone,
               NextPhase: "done",
           }, nil
       }

       ref, _ := md.Frontmatter.String("ref")
       jobRunTime := time.Now()

       // Post the LGTM review.
       if s.prPoster != nil {
           result := s.prPoster.PostLGTM(ctx, *prInfo, ref, "", s.botLogin)

           // Always append diagnostics (one entry per Job run, append-only).
           appendPlanningDiagnostics(md, buildPlanningDiagnosticBlock(jobRunTime, md.Frontmatter.TriggerCount(), result))

           if result.Outcome != "success" && result.Class != ErrorClassNotAFailure {
               return &agentlib.Result{
                   Status:    agentlib.AgentStatusDone,
                   NextPhase: "human_review",
                   Message:   fmt.Sprintf("planning: LGTM POST failed: %s", result.ErrorMessage),
               }, nil
           }

           // Write ## Verdict section naming review id and COMMENT event.
           writePlanningVerdict(md, result.ReviewID, "COMMENT")
           return &agentlib.Result{
               Status:    agentlib.AgentStatusDone,
               NextPhase: "done",
           }, nil
       }

       // nil poster — skip posting (cmd/run-task backward-compat), advance to done.
       return &agentlib.Result{
           Status:    agentlib.AgentStatusDone,
           NextPhase: "done",
       }, nil
   }

   // parsePlanningConcerns extracts the concerns array from the ## Plan JSON body.
   // The JSON may be wrapped in ```json ... ``` fences. Returns an error if the
   // JSON cannot be parsed or the concerns field is absent.
   func parsePlanningConcerns(ctx context.Context, body string) ([]struct{}, error) {
       trimmed := strings.TrimSpace(body)
       // Strip ```json fences.
       trimmed = strings.TrimPrefix(trimmed, "```json")
       trimmed = strings.TrimPrefix(trimmed, "```")
       trimmed = strings.TrimSuffix(trimmed, "```")
       trimmed = strings.TrimSpace(trimmed)

       var p planningOutput
       if err := json.Unmarshal([]byte(trimmed), &p); err != nil {
           return nil, errors.Wrapf(ctx, err, "parse ## Plan JSON")
       }
       return p.Concerns, nil
   }

   // writePlanningVerdict writes the ## Verdict section after an LGTM POST.
   func writePlanningVerdict(md *agentlib.Markdown, reviewID int64, postedEvent string) {
       body := fmt.Sprintf("review_id: %d\nevent: %s\n", reviewID, postedEvent)
       md.ReplaceSection(agentlib.Section{Heading: "## Verdict", Body: body})
   }

   ```

   **Reuse, don't duplicate, diagnostics helpers.** The `appendDiagnosticsSection(md, block)` + `buildDiagnosticBlock(jobRunTime, triggerCount, result)` helpers already exist in `agent/pr-reviewer/pkg/steps_checkout_execution.go:392-429` — same package, byte-identical to what we need. Call them directly from `postLGTMAndDone`:

   ```go
   appendDiagnosticsSection(md, buildDiagnosticBlock(jobRunTime, md.Frontmatter.TriggerCount(), result))
   ```

   Do NOT introduce `appendPlanningDiagnostics` or `buildPlanningDiagnosticBlock` — they would be byte-identical duplicates that invite drift when the diagnostic schema changes.

   Note: `time` is used via `time.Now()` — add `"time"` to the import block.

   Note: `ExtractPRURL` is **exported** in `agent/pr-reviewer/pkg/steps_checkout_execution.go:211` (same package as the new file). Call it directly (`ExtractPRURL(md)`); do NOT re-implement or duplicate the `githubPRURLPattern` regex.

   Note: `ParsePRURL` is exported in `pkg` — call it directly. `PRInfo` and `Platform` are also exported. `ErrorClass` constants are in `pkg`.

5. **Update `CreateAgent` in `agent/pr-reviewer/pkg/factory/factory.go`** to use `planningStep` instead of `claudelib.NewAgentStep`:

   Replace the `planningStep` construction block in `CreateAgent`:

   ```go
   planningStep := prpkg.NewPlanningStep(
       CreateClaudeRunner(claudeConfigDir, agentDir, model, env, planningTools),
       prompts.BuildPlanningInstructions(),
       poster,   // prpkg.PrPoster — the concrete poster (spec 033 wired)
       botLogin, // string — resolved from BOT_GITHUB_LOGIN env
   )
   ```

   The `planningStep` no longer needs `tokenCheck` wrapping — the `prPoster` handles auth internally (via the App auth bearer token). Remove the `planningStep = claudelib.NewAgentStep(...)` line entirely and replace with the above.

   Also remove the `planningTools` variable if it is only used for the planning step (check if `executionTools` uses it — yes, `executionTools` is separate and used by `executionStep`). The `planningTools` variable should remain as it is still used to configure the runner.

   Also: remove the `planningStep` line from the `agentlib.NewAgent` constructor call — `NewAgent` takes variadic `*agentlib.Phase` arguments, and `planningStep` is already constructed as `agentlib.Step` via `prpkg.NewPlanningStep`. The `agentlib.NewPhase("planning", tokenCheck, planningStep)` still needs `tokenCheck` as the auth guard. The updated `CreateAgent` should look like:

   ```go
   planningPhase := agentlib.NewPhase("planning", tokenCheck, prpkg.NewPlanningStep(
       CreateClaudeRunner(claudeConfigDir, agentDir, model, env, planningTools),
       prompts.BuildPlanningInstructions(),
       poster,
       botLogin,
   ))
   executionPhase := agentlib.NewPhase("in_progress", tokenCheck, prpkg.NewCheckoutExecutionStep(
       repoManager,
       claudeConfigDir,
       agentDir,
       model,
       env,
       executionTools,
       reviewMode,
       repoAllowlist,
       poster,
   ))
   reviewPhase := agentlib.NewPhase("ai_review", tokenCheck, prpkg.NewReviewStep(
       CreateClaudeRunner(claudeConfigDir, agentDir, model, env, reviewTools),
       prompts.BuildReviewInstructions(),
       verifier,
       ghToken,
       botLogin,
   ))
   return agentlib.NewAgent(planningPhase, executionPhase, reviewPhase)
   ```

   Note: `poster` here is the `prpkg.PrPoster` interface value returned by `CreatePrPoster`. Pass it directly to `NewPlanningStep` and to `NewCheckoutExecutionStep`.

6. **Run `make test`** to verify compilation and existing tests pass:

   ```bash
   cd agent/pr-reviewer && make test
   ```

   Fix any compile errors. The main likely issues: import cycles (if `steps_planning.go` imports `githubposter` directly — use only `pkg` types), missing `time` import, `ExtractPRURL` visibility.

7. **Create `agent/pr-reviewer/pkg/steps_planning_test.go`** — Ginkgo tests for `planningStep`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package pkg_test

   import (
       "context"
       "encoding/json"
       "net/http"
       "net/http/httptest"
       "time"

       agentlib "github.com/bborbe/agent/lib"
       claudelib "github.com/bborbe/agent/lib/claude"
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/agent/pr-reviewer/mocks"
       pkg "github.com/bborbe/maintainer/agent/pr-reviewer/pkg"
   )

   var _ = Describe("planningStep", func() {
       var (
           ctx       context.Context
           runner    *mocks.ClaudeRunnerMock
           prPoster  *mocks.PrPosterMock
           step      agentlib.Step
           botLogin  string
       )

       BeforeEach(func() {
           ctx = context.Background()
           runner = &mocks.ClaudeRunnerMock{}
           prPoster = &mocks.PrPosterMock{}
           botLogin = "ben-s-pull-request-reviewer-dev[bot]"
           step = pkg.NewPlanningStep(
               runner,
               claudelib.Instructions{},
               prPoster,
               botLogin,
           )
       })

       Describe("Name", func() {
           It("returns pr-plan", func() {
               Expect(step.Name()).To(Equal("pr-plan"))
           })
       })

       Describe("ShouldRun", func() {
           DescribeTable("decides based on existing ## Plan section",
               func(content string, expected bool) {
                   md, err := agentlib.ParseMarkdown(ctx, content)
                   Expect(err).NotTo(HaveOccurred())
                   result, err := step.ShouldRun(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result).To(Equal(expected))
               },
               Entry("no plan section", "# PR Review\n\nsome text", true),
               Entry("plan section present", "# PR Review\n\n## Plan\n\n{}", false),
               Entry("empty content", "", true),
           )
       })

       Describe("Run — empty concerns path (LGTM)", func() {
           var md *agentlib.Markdown

           BeforeEach(func() {
               var err error
               md, err = agentlib.ParseMarkdown(ctx, `---
   ref: abc123
   task_identifier: 00000000-0000-0000-0000-000000000001
   ---
   # PR Review

   https://github.com/bborbe/maintainer/pull/14
   `)
               Expect(err).NotTo(HaveOccurred())
           })

           Context("when ## Plan has concerns: [] and POST succeeds", func() {
               BeforeEach(func() {
                   planBody, _ := json.Marshal(map[string]interface{}{
                       "pr_url":      "https://github.com/bborbe/maintainer/pull/14",
                       "pr_title":    "test PR",
                       "base_branch": "main",
                       "head_branch": "feat/test",
                       "files_changed": []string{"README.md"},
                       "scope":       "docs",
                       "focus_areas": []string{"docs"},
                       "concerns":    []interface{}{},
                   })
                   runner.RunReturns(&claudelib.ClaudeResult{
                       Result: "```json\n" + string(planBody) + "\n```",
                   }, nil)
                   prPoster.PostLGTMReturns(pkg.PostResult{
                       Outcome:     "success",
                       ReviewID:    12345,
                       PostedEvent: "COMMENT",
                   })
               })

               It("calls PrPoster.PostLGTM with correct arguments", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(prPoster.PostLGTMCallCount()).To(Equal(1))
                   _, prArg, headSHAArg, workDirArg, botLoginArg := prPoster.PostLGTMArgsForCall(0)
                   Expect(prArg.Owner).To(Equal("bborbe"))
                   Expect(prArg.Repo).To(Equal("maintainer"))
                   Expect(prArg.Number).To(Equal(14))
                   Expect(headSHAArg).To(Equal("abc123"))
                   Expect(workDirArg).To(Equal(""))
                   Expect(botLoginArg).To(Equal(botLogin))
               })

               It("returns status done with NextPhase done", func() {
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
                   Expect(result.NextPhase).To(Equal("done"))
               })

               It("writes ## Plan section with the LLM output", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   planSection, exists := md.FindSection("## Plan")
                   Expect(exists).To(BeTrue())
                   Expect(planSection.Body).To(ContainSubstring("concerns"))
               })

               It("writes ## Verdict section naming review id and COMMENT", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   verdictSection, exists := md.FindSection("## Verdict")
                   Expect(exists).To(BeTrue())
                   Expect(verdictSection.Body).To(ContainSubstring("review_id: 12345"))
                   Expect(verdictSection.Body).To(ContainSubstring("event: COMMENT"))
               })

               It("appends a success diagnostics one-liner", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   diagSection, exists := md.FindSection("## Diagnostics")
                   Expect(exists).To(BeTrue())
                   Expect(diagSection.Body).To(ContainSubstring("outcome: success"))
                   Expect(diagSection.Body).To(ContainSubstring("review_id: 12345"))
               })
           })

           Context("when ## Plan has concerns: [] and POST returns failure", func() {
               BeforeEach(func() {
                   planBody, _ := json.Marshal(map[string]interface{}{
                       "pr_url":      "https://github.com/bborbe/maintainer/pull/14",
                       "pr_title":    "test PR",
                       "base_branch": "main",
                       "head_branch": "feat/test",
                       "files_changed": []string{"README.md"},
                       "scope":       "docs",
                       "focus_areas": []string{"docs"},
                       "concerns":    []interface{}{},
                   })
                   runner.RunReturns(&claudelib.ClaudeResult{
                       Result: "```json\n" + string(planBody) + "\n```",
                   }, nil)
                   prPoster.PostLGTMReturns(pkg.PostResult{
                       Outcome:      "failed",
                       FailureStep:  "POST /pulls/N/reviews",
                       Class:        pkg.ErrorClassTransient,
                       ErrorMessage: "network timeout",
                       HTTPStatus:   500,
                   })
               })

               It("returns status done with NextPhase human_review", func() {
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
                   Expect(result.NextPhase).To(Equal("human_review"))
                   Expect(result.Message).To(ContainSubstring("LGTM POST failed"))
               })

               It("appends a failure diagnostic block", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   diagSection, exists := md.FindSection("## Diagnostics")
                   Expect(exists).To(BeTrue())
                   Expect(diagSection.Body).To(ContainSubstring("outcome: failed"))
                   Expect(diagSection.Body).To(ContainSubstring("network timeout"))
               })

               It("does NOT write ## Verdict section", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   _, exists := md.FindSection("## Verdict")
                   Expect(exists).To(BeFalse())
               })
           })

           Context("when prPoster is nil (cmd/run-task mode)", func() {
               BeforeEach(func() {
                   step = pkg.NewPlanningStep(runner, claudelib.Instructions{}, nil, botLogin)
                   planBody, _ := json.Marshal(map[string]interface{}{
                       "pr_url":      "https://github.com/bborbe/maintainer/pull/14",
                       "pr_title":    "test PR",
                       "base_branch": "main",
                       "head_branch": "feat/test",
                       "files_changed": []string{"README.md"},
                       "scope":       "docs",
                       "focus_areas": []string{"docs"},
                       "concerns":    []interface{}{},
                   })
                   runner.RunReturns(&claudelib.ClaudeResult{
                       Result: "```json\n" + string(planBody) + "\n```",
                   }, nil)
               })

               It("returns done without calling PostLGTM", func() {
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
                   Expect(result.NextPhase).To(Equal("done"))
               })
           })
       })

       Describe("Run — non-empty concerns path (execution)", func() {
           var md *agentlib.Markdown

           BeforeEach(func() {
               var err error
               md, err = agentlib.ParseMarkdown(ctx, `---
   ref: abc123
   task_identifier: 00000000-0000-0000-0000-000000000001
   ---
   # PR Review

   https://github.com/bborbe/maintainer/pull/14
   `)
               Expect(err).NotTo(HaveOccurred())
           })

           Context("when ## Plan has non-empty concerns", func() {
               BeforeEach(func() {
                   planBody, _ := json.Marshal(map[string]interface{}{
                       "pr_url":      "https://github.com/bborbe/maintainer/pull/14",
                       "pr_title":    "test PR",
                       "base_branch": "main",
                       "head_branch": "feat/test",
                       "files_changed": []string{"pkg/auth/handler.go"},
                       "scope":       "feature",
                       "focus_areas": []string{"security"},
                       "concerns": []map[string]string{
                           {"area": "security", "file": "pkg/auth/handler.go", "note": "missing rate limit"},
                       },
                   })
                   runner.RunReturns(&claudelib.ClaudeResult{
                       Result: "```json\n" + string(planBody) + "\n```",
                   }, nil)
               })

               It("returns status done with NextPhase in_progress", func() {
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
                   Expect(result.NextPhase).To(Equal("in_progress"))
               })

               It("does NOT call PostLGTM", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(prPoster.PostLGTMCallCount()).To(Equal(0))
               })

               It("does NOT write ## Verdict section", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   _, exists := md.FindSection("## Verdict")
                   Expect(exists).To(BeFalse())
               })

               It("does NOT append diagnostics", func() {
                   _, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   _, exists := md.FindSection("## Diagnostics")
                   Expect(exists).To(BeFalse())
               })
           })
       })

       Describe("Run — error cases", func() {
           Context("when ## Plan JSON is malformed", func() {
               BeforeEach(func() {
                   runner.RunReturns(&claudelib.ClaudeResult{
                       Result: "not valid json at all",
                   }, nil)
               })

               It("routes to human_review", func() {
                   md, err := agentlib.ParseMarkdown(ctx, "# PR Review\n\nhttps://github.com/bborbe/maintainer/pull/14\n")
                   Expect(err).NotTo(HaveOccurred())
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.Status).To(Equal(agentlib.AgentStatusDone))
                   Expect(result.NextPhase).To(Equal("human_review"))
                   Expect(result.Message).To(ContainSubstring("parse ## Plan JSON"))
               })
           })

           Context("when Claude runner returns an error", func() {
               BeforeEach(func() {
                   runner.RunReturns(nil, context.DeadlineExceeded)
               })

               It("returns AgentStatusFailed", func() {
                   md, err := agentlib.ParseMarkdown(ctx, "# PR Review\n\nhttps://github.com/bborbe/maintainer/pull/14\n")
                   Expect(err).NotTo(HaveOccurred())
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.Status).To(Equal(agentlib.AgentStatusFailed))
               })
           })

           Context("when PR URL is absent from task", func() {
               BeforeEach(func() {
                   planBody, _ := json.Marshal(map[string]interface{}{
                       "pr_url":      "https://github.com/bborbe/maintainer/pull/14",
                       "pr_title":    "test PR",
                       "base_branch": "main",
                       "head_branch": "feat/test",
                       "files_changed": []string{"README.md"},
                       "scope":       "docs",
                       "focus_areas": []string{"docs"},
                       "concerns":    []interface{}{},
                   })
                   runner.RunReturns(&claudelib.ClaudeResult{
                       Result: "```json\n" + string(planBody) + "\n```",
                   }, nil)
               })

               It("returns human_review when PR URL missing", func() {
                   md, err := agentlib.ParseMarkdown(ctx, "# PR Review\n")
                   Expect(err).NotTo(HaveOccurred())
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.NextPhase).To(Equal("human_review"))
                   Expect(result.Message).To(ContainSubstring("no GitHub PR URL"))
               })
           })

           Context("when non-GitHub platform", func() {
               BeforeEach(func() {
                   planBody, _ := json.Marshal(map[string]interface{}{
                       "pr_url":      "https://bitbucket.org/bborbe/maintainer/pull/14",
                       "pr_title":    "test PR",
                       "base_branch": "main",
                       "head_branch": "feat/test",
                       "files_changed": []string{"README.md"},
                       "scope":       "docs",
                       "focus_areas": []string{"docs"},
                       "concerns":    []interface{}{},
                   })
                   runner.RunReturns(&claudelib.ClaudeResult{
                       Result: "```json\n" + string(planBody) + "\n```",
                   }, nil)
               })

               It("skips posting and returns done", func() {
                   md, err := agentlib.ParseMarkdown(ctx, "# PR Review\n\nhttps://bitbucket.org/bborbe/maintainer/pull/14\n")
                   Expect(err).NotTo(HaveOccurred())
                   result, err := step.Run(ctx, md)
                   Expect(err).NotTo(HaveOccurred())
                   Expect(result.NextPhase).To(Equal("done"))
                   Expect(prPoster.PostLGTMCallCount()).To(Equal(0))
               })
           })
       })

       Describe("parsePlanningConcerns", func() {
           DescribeTable("extracts concerns array from various JSON wrapping",
               func(body, want string) {
                   concerns, err := pkg.ParsePlanningConcernsForTest(body)
                   if want == "error" {
                       Expect(err).To(HaveOccurred())
                       return
                   }
                   Expect(err).NotTo(HaveOccurred())
                   if want == "empty" {
                       Expect(concerns).To(BeEmpty())
                   } else {
                       Expect(concerns).NotTo(BeEmpty())
                   }
               },
               Entry("bare JSON array", `{"concerns":[]}`, "empty"),
               Entry("json fence", "```json\n{\"concerns\":[]}\n```", "empty"),
               Entry("non-empty concerns", "```json\n{\"concerns\":[{\"area\":\"security\"}]}\n```", "non-empty"),
               Entry("malformed JSON", "not json at all", "error"),
           )
       })
   })
   ```

   Note: `ParsePlanningConcernsForTest` must be exposed via `export_test.go` (add it alongside the existing export helpers).

7b. **Add `httptest`-server integration test for `*prPoster.PostLGTM`** in `agent/pr-reviewer/pkg/githubposter/poster_test.go`. The Step-7 tests above mock the `PrPoster` interface — they do NOT exercise the concrete `*prPoster.PostLGTM` body construction. **Spec AC #3 requires** asserting the actual POST request body equals `Reviewed by <BotLogin> — no concerns flagged.` against a real `httptest.Server`. Add this `Describe` block alongside the existing poster tests:

   ```go
   Describe("*prPoster.PostLGTM (integration boundary)", func() {
       var (
           server *httptest.Server
           client *http.Client
       )

       AfterEach(func() {
           if server != nil {
               server.Close()
           }
       })

       It("posts a COMMENT review with the canonical LGTM body and reports success", func(ctx context.Context) {
           const botLogin = "ben-s-pull-request-reviewer-dev[bot]"
           const headSHA = "abc123def456abc123def456abc123def456abc1"

           var capturedBody string
           server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
               if r.URL.Path == "/user" {
                   _, _ = w.Write([]byte(`{"login":"` + botLogin + `"}`))
                   return
               }
               if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/reviews") {
                   body, _ := io.ReadAll(r.Body)
                   capturedBody = string(body)
                   _, _ = w.Write([]byte(`{"id":99999}`))
                   return
               }
               if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/reviews") {
                   _, _ = w.Write([]byte(`[{"id":99999,"user":{"login":"` + botLogin + `"},"commit_id":"` + headSHA + `","state":"COMMENTED"}]`))
                   return
               }
               http.NotFound(w, r)
           }))

           client = server.Client()
           // Construct a poster targeting the test server. The poster currently hardcodes api.github.com; use the same pkg-internal helper the existing tests use to swap the base URL (see existing poster_test.go for the pattern — likely a package-level var or a test seam).
           poster := NewPrPosterWithBaseURL(client, "test-iat", botLogin, server.URL)

           result := poster.PostLGTM(ctx, prpkg.PRInfo{Owner: "bborbe", Repo: "go-skeleton", Number: 99}, headSHA, "", botLogin)

           Expect(result.Outcome).To(Equal("success"))
           Expect(result.PostedEvent).To(Equal("COMMENT"))
           Expect(result.ReviewID).To(Equal(int64(99999)))
           // The load-bearing assertion: the actual POST body matches the LGTM template, NOT a typo.
           Expect(capturedBody).To(MatchRegexp(`"body":\s*"Reviewed by ` + regexp.QuoteMeta(botLogin) + ` — no concerns flagged\."`))
           Expect(capturedBody).To(MatchRegexp(`"event":\s*"COMMENT"`))
           Expect(capturedBody).To(MatchRegexp(`"commit_id":\s*"` + headSHA + `"`))
       })
   })
   ```

   If the existing `poster_test.go` already swaps `api.github.com` for `httptest.Server.URL` via a different mechanism (package-level `baseURL` var, constructor option, or `http.Client.Transport` rewrite), use that mechanism — read the existing test setup first and match it. Do NOT add a new public `NewPrPosterWithBaseURL` constructor purely for tests if a less invasive seam already exists.

8. **Add to `export_test.go`** in `agent/pr-reviewer/pkg/`:

   ```go
   // ParsePlanningConcernsForTest exposes parsePlanningConcerns for unit testing.
   func ParsePlanningConcernsForTest(body string) ([]struct{}, error) {
       return parsePlanningConcerns(context.Background(), body)
   }
   ```

9. **Run `make test`** again to confirm all tests pass:

   ```bash
   cd agent/pr-reviewer && make test
   ```

10. **Add CHANGELOG entry** in root `CHANGELOG.md` under `## Unreleased`:

    ```
    - feat(agent/pr-reviewer): planning phase now posts an LGTM COMMENT review when concerns are empty, eliminating the silent-skip path; every PR that reaches planning produces at least one visible artifact; vault task gains `## Verdict` section naming the posted review id and event
    ```

11. **Run `make precommit`**:

    ```bash
    cd agent/pr-reviewer && make precommit
    ```
</requirements>

<constraints>
- Only edit files under `agent/pr-reviewer/pkg/` (mocks regenerated via `go generate`) AND the repo-root `CHANGELOG.md`. Do NOT create `agent/pr-reviewer/CHANGELOG.md` — there is no per-service changelog in this repo.
- If the repo-root `CHANGELOG.md` does not have a `## Unreleased` heading yet, **create it** above the most recent released version. The sibling docs prompt (`034-pr-reviewer-always-post-review-docs.md`) depends on this heading existing.
- Do NOT commit — dark-factory handles git
- `PrPoster.PostLGTM` MUST be added to the `PrPoster` interface in `poster_types.go` (not just the concrete type) so the mock supports it for testing
- The `planningStep` MUST accept `PrPoster` as the interface type (not `*githubposter.prPoster` concrete) so mocks can be injected
- `planningStep.Run` MUST write `## Plan` to the vault BEFORE making any branching decision — vault-first invariant (same as `## Review` in execution)
- `planningStep.Run` MUST write `## Verdict` only after the LGTM POST succeeds — not before, not on failure
- On POST failure (any `Outcome != "success"` where `Class != ErrorClassNotAFailure`), MUST route to `human_review` — same as execution step's failure routing
- `nil` prPoster MUST be handled gracefully (backward-compatible with `cmd/run-task`) — skip POST and return `done`
- Non-GitHub PR URLs MUST be handled gracefully — skip POST and return `done` (no `human_review` escalation)
- `parsePlanningConcerns` MUST strip ```json``` fences before parsing JSON
- All new errors wrapped via `github.com/bborbe/errors`; no `fmt.Errorf` / `errors.New` in modified/new files
- BSD-style license header on every new `.go` file
- Mock regenerated via `go generate ./pkg/...` after adding `PostLGTM` to the interface
- Coverage ≥80% for changed packages
- `make precommit` runs from `agent/pr-reviewer/`, never at repo root
- Do NOT change `ai_review` phase behavior — `reviewStep` remains unchanged; `## Verdict` for the non-empty-concerns path is written by `reviewStep` (existing behavior)
- Do NOT change the planning prompt (`prompts/planning.go` / `prompts/planning_workflow.md` / `prompts/planning_output-format.md`) — the JSON output shape is unchanged
- Do NOT add `autoApprove` handling to the LGTM path — LGTM always uses `event=COMMENT`, never `APPROVE`
</constraints>

<verification>
cd agent/pr-reviewer && make precommit

# Confirm PostLGTM added to PrPoster interface:
grep -n "PostLGTM" agent/pr-reviewer/pkg/poster_types.go
# Expected: interface method declaration

# Confirm PostLGTM implemented on *prPoster:
grep -n "func.*prPoster.*PostLGTM" agent/pr-reviewer/pkg/githubposter/poster.go
# Expected: one match

# Confirm planningStep type exists:
grep -n "type planningStep struct\|func NewPlanningStep" agent/pr-reviewer/pkg/steps_planning.go
# Expected: type declaration + constructor

# Confirm factory uses NewPlanningStep:
grep -n "NewPlanningStep" agent/pr-reviewer/pkg/factory/factory.go
# Expected: at least one match in CreateAgent

# Confirm factory no longer uses claudelib.NewAgentStep for planning:
grep -n "claudelib.NewAgentStep.*pr-plan\|Name.*pr-plan" agent/pr-reviewer/pkg/factory/factory.go
# Expected: zero matches (replaced by NewPlanningStep)

# Confirm mock includes PostLGTM:
grep -n "PostLGTM" agent/pr-reviewer/mocks/pr-poster.go
# Expected: PostLGTMStub, PostLGTMCallCount, PostLGTMArgsForCall, PostLGTMReturns

# Confirm parsePlanningConcerns strips json fences:
grep -n "TrimPrefix.*\`\`\`json\|TrimPrefix.*\`\`\`" agent/pr-reviewer/pkg/steps_planning.go
# Expected: fence stripping before json.Unmarshal

# Confirm ## Verdict written only on success:
grep -n "writePlanningVerdict\|review_id:" agent/pr-reviewer/pkg/steps_planning.go
# Expected: writePlanningVerdict called only in the success path

# Confirm failure routes to human_review:
grep -n "human_review.*LGTM\|LGTM.*human_review" agent/pr-reviewer/pkg/steps_planning.go
# Expected: failure path returns NextPhase: human_review

# Confirm nil prPoster handled:
grep -n "prPoster.*nil\|nil.*prPoster" agent/pr-reviewer/pkg/steps_planning.go
# Expected: nil check before calling PostLGTM

# Confirm botLogin passed to PostLGTM:
grep -n "botLogin\|PostLGTM.*botLogin" agent/pr-reviewer/pkg/steps_planning.go
# Expected: botLogin from struct field passed to PostLGTM

# Confirm no hardcoded bot login literal in steps_planning.go:
grep -rn "ben-s-pull-request-reviewer" agent/pr-reviewer/pkg/steps_planning.go
# Expected: zero matches outside test context

# Confirm CHANGELOG entry:
grep -n "LGTM\|no concerns\|always.*post" CHANGELOG.md
# Expected: at least one match under ## Unreleased

# Confirm test file exists:
ls agent/pr-reviewer/pkg/steps_planning_test.go
# Expected: file exists

# Confirm test covers empty concerns → done:
grep -n "NextPhase.*done\|PostLGTMCallCount" agent/pr-reviewer/pkg/steps_planning_test.go
# Expected: assertions for done phase and PostLGTM call count

# Confirm test covers non-empty concerns → in_progress:
grep -n "in_progress\|PostLGTMCallCount.*0" agent/pr-reviewer/pkg/steps_planning_test.go
# Expected: non-empty concerns test with zero PostLGTM calls

# Confirm test covers POST failure → human_review:
grep -n "human_review\|PostLGTMReturns.*failed" agent/pr-reviewer/pkg/steps_planning_test.go
# Expected: failure path test
</verification>
