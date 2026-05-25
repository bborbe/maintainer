---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T22:38:58Z"
---

<summary>
- `BuildCreateCommand` (pkg/watcher.go:293) — exported function used by trigger handler, 0% test coverage, branches on trustResult.Success() with two different frontmatter/body shapes
- `CreateKafkaSender` (pkg/factory/factory.go:95) — factory with no tests, returns cleanup closure
- `CreateSinglePRHandler` (pkg/factory/single_pr.go:20) — factory with 0% coverage, nil validation branches untested
</summary>

<objective>
Add tests for the three critical zero-coverage exported functions: BuildCreateCommand, CreateKafkaSender (or its replacement), and CreateSinglePRHandler (or its replacement).
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, Counterfeiter mocks, coverage ≥80%.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — factory has zero business logic.

Files to read before making changes:
- `watcher/github-pr/pkg/watcher.go` — lines 293-423; understand BuildCreateCommand, buildFrontmatter, buildHumanReviewFrontmatter, buildTaskBody, buildUntrustedBody
- `watcher/github-pr/pkg/factory/factory.go` — full file; understand current factory functions
- `watcher/github-pr/pkg/factory/single_pr.go` — full file; understand CreateSinglePRHandler
- `watcher/github-pr/pkg/handler/trigger_handler_test.go` — understand existing test patterns
</context>

<requirements>

**Execute steps in order. Run `make test` after each step. Run `make precommit` only at the final step.**

1. **Add `BuildCreateCommand` tests in `pkg/watcher_test.go`:**

   Append a new `Describe("BuildCreateCommand", ...)` block to `pkg/watcher_test.go`:

   ```go
   Describe("BuildCreateCommand", func() {
       makePRDetails := func(login string) pkg.PullRequestDetails {
           return pkg.PullRequestDetails{
               Title:    "feat: add new feature",
               AuthorLogin: login,
               HTMLURL:  "https://github.com/owner/repo/pull/1",
               Body:     "This PR adds a feature",
               UpdatedAt: libtime.DateTime(time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)),
               HeadSHA:  "abc123",
           }
       }

       It("trusted author — sets phase=planning, status=in_progress, assignee=pr-reviewer-agent", func() {
           pr := makePRDetails("trusted-user")
           trustResult := trust.NewResult(true, "author allowlist")
           taskIDStr := "00000000-0000-0000-0000-000000000001"

           cmd := pkg.BuildCreateCommand(pr, trustResult, "dev", 80, 60, "pr-reviewer", taskIDStr)

           Expect(cmd.Title).NotTo(BeEmpty())
           Expect(cmd.Frontmatter["phase"]).To(Equal("planning"))
           Expect(cmd.Frontmatter["status"]).To(Equal("in_progress"))
           Expect(cmd.Frontmatter["assignee"]).To(Equal("pr-reviewer-agent"))
           Expect(cmd.Frontmatter["provider"]).To(Equal("github"))
           Expect(cmd.Frontmatter["repo"]).To(Equal("repo"))
           Expect(cmd.Frontmatter["pr_number"]).To(Equal(float64(1)))
           Expect(cmd.Body).To(ContainSubstring("feat: add new feature"))
       })

       It("untrusted author — sets phase=human-review, status=todo, assignee=untrusted-agent", func() {
           pr := makePRDetails("unknown-user")
           trustResult := trust.NewResult(false, "author not in allowlist")
           taskIDStr := "00000000-0000-0000-0000-000000000002"

           cmd := pkg.BuildCreateCommand(pr, trustResult, "dev", 80, 60, "pr-reviewer", taskIDStr)

           Expect(cmd.Title).NotTo(BeEmpty())
           Expect(cmd.Frontmatter["phase"]).To(Equal("human-review"))
           Expect(cmd.Frontmatter["status"]).To(Equal("todo"))
           Expect(cmd.Frontmatter["assignee"]).To(Equal("untrusted-agent"))
           Expect(cmd.Body).To(ContainSubstring("Untrusted author"))
           Expect(cmd.Body).To(ContainSubstring("unknown-user"))
           Expect(cmd.Body).To(ContainSubstring("author not in allowlist"))
       })

       It("untrusted author with empty login — body contains (unknown)", func() {
           pr := makePRDetails("")
           trustResult := trust.NewResult(false, "no author")
           taskIDStr := "00000000-0000-0000-0000-000000000003"

           cmd := pkg.BuildCreateCommand(pr, trustResult, "dev", 80, 60, "pr-reviewer", taskIDStr)

           Expect(cmd.Body).To(ContainSubstring("(unknown)"))
       })

       It("title sanitizes special characters", func() {
           pr := makePRDetails("trusted-user")
           pr.Title = `fix: handle /api?id=1 in :backend`
           trustResult := trust.NewResult(true, "author allowlist")
           taskIDStr := "00000000-0000-0000-0000-000000000004"

           cmd := pkg.BuildCreateCommand(pr, trustResult, "dev", 80, 60, "pr-reviewer", taskIDStr)

           // Title must not contain slashes or colons that could break filename
           Expect(cmd.Title).NotTo(ContainSubstring("/"))
           Expect(cmd.Title).NotTo(ContainSubstring(":"))
       })

       It("respects maxTitleLen truncation", func() {
           pr := makePRDetails("trusted-user")
           pr.Title = strings.Repeat("a", 200)
           trustResult := trust.NewResult(true, "author allowlist")
           taskIDStr := "00000000-0000-0000-0000-000000000005"

           cmd := pkg.BuildCreateCommand(pr, trustResult, "dev", 80, 30, "pr-reviewer", taskIDStr) // maxTitleLen=30

           Expect(len(cmd.Title)).To(BeNumerically("<=", 30+len("-github-repo-1")))
       })
   })
   ```

   Add `"strings"` to imports if not already present.

2. **Add `CreateSinglePRHandler` nil-check tests in `pkg/factory/single_pr_test.go`** (create if not exists):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package factory_test

   import (
       "context"
       "net/http"
       "testing"
       "time"

       "github.com/bborbe/agent/lib/command/task/mocks"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/factory"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
       "github.com/bborbe/trust/mocks"
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   var _ = Describe("CreateSinglePRTriggerHandler", func() {
       ctx := context.Background()
       validHTTPClient := &http.Client{Timeout: 30 * time.Second}
       validSender := new(taskmocks.TaskCreateCommandSender)
       validFilter := filter.TaskCreationFilters{}
       validTrust := new(trustmocks.Trust)

       It("returns non-nil handler when all params are non-nil", func() {
           handler := factory.CreateSinglePRTriggerHandler(
               validHTTPClient,
               validSender,
               validFilter,
               validTrust,
               "dev",
               80, 60, "pr-reviewer",
               pkg.NewMetrics(),
           )
           Expect(handler).NotTo(BeNil())
       })

       It("panics when httpClient is nil", func() {
           Expect(func() {
               factory.CreateSinglePRTriggerHandler(
                   nil,
                   validSender,
                   validFilter,
                   validTrust,
                   "dev",
                   80, 60, "pr-reviewer",
                   pkg.NewMetrics(),
               )
           }).To(PanicWith("ghClient is required"))
       })

       It("panics when createSender is nil", func() {
           Expect(func() {
               factory.CreateSinglePRTriggerHandler(
                   validHTTPClient,
                   nil,
                   validFilter,
                   validTrust,
                   "dev",
                   80, 60, "pr-reviewer",
                   pkg.NewMetrics(),
               )
           }).To(PanicWith("createSender is required"))
       })

       It("panics when taskCreationFilter is nil", func() {
           Expect(func() {
               factory.CreateSinglePRTriggerHandler(
                   validHTTPClient,
                   validSender,
                   nil,
                   validTrust,
                   "dev",
                   80, 60, "pr-reviewer",
                   pkg.NewMetrics(),
               )
           }).To(PanicWith("taskCreationFilter is required"))
       })

       It("panics when trustDecision is nil", func() {
           Expect(func() {
               factory.CreateSinglePRTriggerHandler(
                   validHTTPClient,
                   validSender,
                   validFilter,
                   nil,
                   "dev",
                   80, 60, "pr-reviewer",
                   pkg.NewMetrics(),
               )
           }).To(PanicWith("trustDecision is required"))
       })
   })

   func TestFactory(t *testing.T) {
       time.Local = time.UTC
       RegisterFailHandler(Fail)
       RunSpecs(t, "Factory Suite")
   }
   ```

   Note: After the factory refactoring (prompt 2), the signatures will change and nil checks may move to `NewSinglePRTriggerHandler`. Adapt accordingly.

3. **Run `make test`:**
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix any compilation errors.

4. **Run `make precommit`:**
   ```bash
   cd watcher/github-pr && make precommit
   ```

   Verify coverage:
   ```bash
   cd watcher/github-pr && go test -coverprofile=/tmp/cover.out -mod=vendor ./pkg/... && go tool cover -func=/tmp/cover.out | grep -E "BuildCreateCommand|CreateSinglePR"
   ```
</requirements>

<constraints>
- Only add test files, do not modify production code
- Do NOT commit — dark-factory handles git
- Tests must use Ginkgo/Gomega conventions
- Use Counterfeiter mocks from `mocks/` directory
- External test packages (`package <name>_test`)
- Coverage ≥80% for the tested functions
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm BuildCreateCommand tests exist:
grep -c "BuildCreateCommand" watcher/github-pr/pkg/watcher_test.go

# Confirm CreateSinglePRTriggerHandler tests:
grep -c "CreateSinglePRTriggerHandler" watcher/github-pr/pkg/factory/single_pr_test.go 2>/dev/null || echo "0"
</verification>
