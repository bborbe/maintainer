---
status: draft
spec: [056-ai-review-actionable]
created: "2026-06-01T00:00:00Z"
branch: dark-factory/ai-review-actionable
---

<summary>
- Adds `ParkedBecause` struct to `watcher/github-pr/pkg/watcher.go` for prior-ai-review-fail context
- Adds `PublishCreateWithParkedBecause` method to `TaskPublisher` interface and implementation
- Adds `buildParkedBecauseBody` and `buildParkedBecauseCommand` helper functions
- Adds `BuildCreateCommandWithParkedBecause` as the exported factory function
- Extends existing `buildUntrustedBody` and `buildTaskBody` tests to assert no `Parked Because` leakage
- Adds new tests for `Parked Because` body content, hallucination ordering, empty hallucinations, and frontmatter
</summary>

<objective>
Implement the `## Parked Because` section in the watcher: when the watcher spawns a `human_review`-phased task because the most recent prior task on the same PR had `ai_review verdict: fail`, the spawned task body contains a `## Parked Because` section listing prior task ID, prior head SHA, prior verdict, and the prior hallucinations. Untrusted-author and trusted-author paths are unchanged.
</objective>

<context>
Read these files before making changes:
- `/workspace/watcher/github-pr/pkg/watcher.go` — `BuildCreateCommand`, `buildUntrustedBody`, `buildTaskBody`, `buildHumanReviewFrontmatter`, `TaskPublisher` interface
- `/workspace/watcher/github-pr/pkg/watcher_test.go` — existing test patterns for `BuildCreateCommand` and `pkg.Watcher`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 / Gomega patterns
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` — Create* / New* conventions

This prompt depends on prompt 1 (`1-spec-056-agent-dismiss-and-comment.md`) for the `Hallucination` struct definition in `agent/pr-reviewer/pkg/poster_types.go`.
</context>

<requirements>
1. **Add `ParkedBecause` data struct to `watcher/github-pr/pkg/watcher.go`.**

   Add this at the top of the file (after imports, before the `Watcher` interface):

   ```go
   // ParkedBecause holds the context for a task parked due to a prior ai_review
   // failure. It is passed to BuildCreateCommand when the watcher decides to
   // spawn a human_review task because the most recent prior task on the same PR
   // had `ai_review verdict: fail`.
   type ParkedBecause struct {
       PriorTaskID    string
       PriorHeadSHA   string
       PriorVerdict   string // always "fail" in practice
       Hallucinations []struct {
           File  string
           Line  int
           Issue string
       }
   }
   ```

2. **Extend `TaskPublisher` interface** in `watcher/github-pr/pkg/watcher.go`.

   Add a new method to the `TaskPublisher` interface:

   ```go
   // PublishCreateWithParkedBecause is like PublishCreate but attaches
   // prior-ai-review-fail context for the ## Parked Because section.
   // Call this when the watcher spawns a human_review task due to a prior
   // ai_review verdict: fail on the same PR.
   PublishCreateWithParkedBecause(ctx context.Context, pr PullRequest, taskIDStr string, details PRDetails, parkedBecause ParkedBecause) bool
   ```

   Add the implementation to the `taskPublisher` struct:

   ```go
   // PublishCreateWithParkedBecause implements TaskPublisher.
   func (p *taskPublisher) PublishCreateWithParkedBecause(
       ctx context.Context,
       pr PullRequest,
       taskIDStr string,
       details PRDetails,
       parkedBecause ParkedBecause,
   ) bool {
       author := pr.AuthorLogin
       if author == "" {
           author = "(unknown)"
       }
       cmd := buildParkedBecauseCommand(
           pr,
           details,
           taskIDStr,
           p.cfg.Stage,
           p.cfg.MaxSlugLen,
           p.cfg.MaxTitleLen,
           p.cfg.TaskSuffix,
           parkedBecause,
       )
       if err := p.createSender.SendCommand(ctx, cmd); err != nil {
           glog.Errorf("publish create-task (parked-because) failed pr=%s err=%v", pr.HTMLURL, err)
           p.metrics.IncPRPublished("error")
           return false
       }
       glog.V(2).Infof(
           "published CreateTaskCommand (parked-because) pr=%s/%s#%d sha=%s taskID=%s priorVerdict=fail hallucinations=%d",
           pr.Owner, pr.Repo, pr.Number, details.HeadSHA, taskIDStr, len(parkedBecause.Hallucinations),
       )
       p.metrics.IncPRPublished("create")
       return true
   }
   ```

3. **Add `buildParkedBecauseCommand` function** to `watcher/github-pr/pkg/watcher.go`:

   ```go
   // buildParkedBecauseCommand builds a CreateTaskCommand for a task parked
   // at human_review due to a prior ai_review verdict: fail.
   func buildParkedBecauseCommand(
       pr PullRequest,
       details PRDetails,
       taskIDStr string,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       parkedBecause ParkedBecause,
   ) task.CreateCommand {
       return task.CreateCommand{
           Title: computePRTitle(
               "github",
               pr.Owner,
               pr.Repo,
               pr.Number,
               details.HeadSHA,
               pr.Title,
               maxSlugLen,
               maxTitleLen,
               taskSuffix,
           ),
           TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
           Frontmatter:    buildHumanReviewFrontmatter(pr, taskIDStr, stage, details),
           Body:           buildParkedBecauseBody(parkedBecause),
       }
   }
   ```

4. **Add `buildParkedBecauseBody` function**:

   ```go
   // buildParkedBecauseBody produces the body for a task parked at human_review
   // due to a prior ai_review verdict: fail. It is the ONLY top-level section —
   // there is no parent "# PR Review" wrapper.
   func buildParkedBecauseBody(pb ParkedBecause) string {
       var sb strings.Builder
       sb.WriteString("## Parked Because\n\n")
       sb.WriteString(fmt.Sprintf("- **Prior task ID:** `%s`\n", pb.PriorTaskID))
       sb.WriteString(fmt.Sprintf("- **Prior head SHA:** `%s`\n", pb.PriorHeadSHA))
       sb.WriteString(fmt.Sprintf("- **Prior verdict:** `%s`\n", pb.PriorVerdict))
       sb.WriteString("\n**Hallucinations from prior ai_review:**\n\n")
       if len(pb.Hallucinations) == 0 {
           sb.WriteString("_(none listed)_\n")
       } else {
           for _, h := range pb.Hallucinations {
               sb.WriteString(fmt.Sprintf("- %s:%d — %s\n", h.File, h.Line, h.Issue))
           }
       }
       return sb.String()
   }
   ```

   Add `"strings"` to the imports in `watcher.go`.

5. **Add `BuildCreateCommandWithParkedBecause` as the exported factory**:

   ```go
   // BuildCreateCommandWithParkedBecause builds a CreateTaskCommand for a task
   // parked at human_review due to a prior ai_review verdict: fail.
   func BuildCreateCommandWithParkedBecause(
       pr PullRequest,
       details PRDetails,
       taskIDStr string,
       stage string,
       maxSlugLen int,
       maxTitleLen int,
       taskSuffix string,
       parkedBecause ParkedBecause,
   ) task.CreateCommand {
       return buildParkedBecauseCommand(
           pr,
           details,
           taskIDStr,
           stage,
           maxSlugLen,
           maxTitleLen,
           taskSuffix,
           parkedBecause,
       )
   }
   ```

6. **Run mock generation**: In `watcher/github-pr/`, run `go generate ./...` to regenerate mocks including the new `PublishCreateWithParkedBecause` method on `TaskPublisher`.

7. **Add tests to `watcher/github-pr/pkg/watcher_test.go`** — add a new `Describe("BuildCreateCommandWithParkedBecause")` block:

   (a) **Parked Because body contains all required fields:**
   ```go
   It("includes prior task ID, prior SHA, prior verdict, and hallucinations", func() {
       pb := pkg.ParkedBecause{
           PriorTaskID:    "00000000-0000-0000-0000-000000000001",
           PriorHeadSHA:   "abc123def",
           PriorVerdict:   "fail",
           Hallucinations: []struct {
               File  string
               Line  int
               Issue string
           }{
               {File: "pkg/foo.go", Line: 99, Issue: "line 99 not in diff"},
               {File: "pkg/bar.go", Line: 42, Issue: "referenced but not changed"},
           },
       }
       cmd := pkg.BuildCreateCommandWithParkedBecause(pr, makeDetails(), taskIDStr, "dev", 80, 60, "", pb)
       Expect(cmd.Body).To(ContainSubstring("## Parked Because"))
       Expect(cmd.Body).To(ContainSubstring("00000000-0000-0000-0000-000000000001"))
       Expect(cmd.Body).To(ContainSubstring("abc123def"))
       Expect(cmd.Body).To(ContainSubstring("fail"))
       Expect(cmd.Body).To(ContainSubstring("pkg/foo.go"))
       Expect(cmd.Body).To(ContainSubstring("99"))
       Expect(cmd.Body).To(ContainSubstring("line 99 not in diff"))
       Expect(cmd.Body).To(ContainSubstring("pkg/bar.go"))
       Expect(cmd.Body).To(ContainSubstring("42"))
   })
   ```

   (b) **Hallucinations appear in the same order as the input array:**
   ```go
   It("maintains hallucination order (first appears before second in body)", func() {
       pb := pkg.ParkedBecause{
           PriorTaskID:    "00000000-0000-0000-0000-000000000002",
           PriorHeadSHA:    "sha-abc",
           PriorVerdict:    "fail",
           Hallucinations: []struct {
               File  string
               Line  int
               Issue string
           }{
               {File: "a.go", Line: 1, Issue: "first"},
               {File: "b.go", Line: 2, Issue: "second"},
           },
       }
       cmd := pkg.BuildCreateCommandWithParkedBecause(pr, makeDetails(), taskIDStr, "dev", 80, 60, "", pb)
       idxA := strings.Index(cmd.Body, "a.go")
       idxB := strings.Index(cmd.Body, "b.go")
       Expect(idxA).To(BeNumerically("<", idxB))
   })
   ```

   (c) **Empty hallucinations shows placeholder:**
   ```go
   It("shows placeholder when hallucinations is empty", func() {
       pb := pkg.ParkedBecause{
           PriorTaskID:    "00000000-0000-0000-0000-000000000003",
           PriorHeadSHA:   "sha-def",
           PriorVerdict:   "fail",
           Hallucinations: nil,
       }
       cmd := pkg.BuildCreateCommandWithParkedBecause(pr, makeDetails(), taskIDStr, "dev", 80, 60, "", pb)
       Expect(cmd.Body).To(ContainSubstring("_(none listed)_"))
   })
   ```

   (d) **Frontmatter is human_review with empty assignee:**
   ```go
   It("sets frontmatter to human_review, status=todo, assignee=empty", func() {
       pb := pkg.ParkedBecause{
           PriorTaskID:  "00000000-0000-0000-0000-000000000004",
           PriorHeadSHA: "sha-ghi",
           PriorVerdict: "fail",
           Hallucinations: []struct {
               File  string
               Line  int
               Issue string
           }{},
       }
       cmd := pkg.BuildCreateCommandWithParkedBecause(pr, makeDetails(), taskIDStr, "dev", 80, 60, "", pb)
       Expect(cmd.Frontmatter["phase"]).To(Equal("human_review"))
       Expect(cmd.Frontmatter["status"]).To(Equal("todo"))
       Expect(cmd.Frontmatter["assignee"]).To(Equal(""))
   })
   ```

8. **Extend existing `buildUntrustedBody` test** — add an assertion that `buildUntrustedBody` output does NOT contain "Parked Because":

   ```go
   It("body does NOT contain 'Parked Because'", func() {
       body := pkg.BuildCreateCommand(pr, makeDetails(), taskIDStr, "dev", 80, 60, "",
           trust.NewResult(false, "author not in allowlist"),
       ).Body
       Expect(body).NotTo(ContainSubstring("Parked Because"))
   })
   ```

   Add this to the existing `Describe("BuildCreateCommand")` block.

9. **Extend existing `buildTaskBody` test** — add an assertion that trusted-author body does NOT contain "Parked Because":

   ```go
   It("body does NOT contain 'Parked Because'", func() {
       body := pkg.BuildCreateCommand(pr, makeDetails(), taskIDStr, "dev", 80, 60, "",
           trust.NewResult(true, "author allowlist"),
       ).Body
       Expect(body).NotTo(ContainSubstring("Parked Because"))
   })
   ```

10. **Run `cd /workspace/watchers/github-pr && go generate ./... && make test`** — all tests must pass.
</requirements>

<constraints>
- Do NOT modify `buildTaskBody` or `buildUntrustedBody` — they are frozen by the spec Non-goals.
- `BuildCreateCommandWithParkedBecause` is a NEW function, separate from the existing `BuildCreateCommand`. The existing function remains unchanged.
- Factory contains zero business logic — `buildParkedBecauseCommand` is private.
- The `ParkedBecause` struct mirrors the fields the spec requires: prior task ID, prior head SHA, prior verdict, hallucinations. It uses a local struct (not the agent's `Hallucination` type) to avoid import coupling.
- Mock generation required after interface change.
</constraints>

<verification>
```bash
cd /workspace/watchers/github-pr && go generate ./... && make test
```

Expected: all tests pass, exit code 0. Specifically check:
- `go test ./pkg/...` — new `Parked Because` tests pass
- `go generate` produces no errors (mocks regenerated)
- Existing `buildUntrustedBody` test still passes (no regression)
</verification>