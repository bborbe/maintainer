---
status: completed
spec: [012-watcher-author-trust-filter]
summary: 'Wired trust framework into github-pr-watcher: added TRUSTED_AUTHORS config and startup validation in main.go, composed trust.And{NewAuthorAllowlist} in factory, added trustDecision parameter to NewWatcher and watcher struct, implemented trust-aware publishCreate and publishForcePush with human_review routing for untrusted authors, and added 5 new test cases covering trusted/untrusted/force-push/error/missing-author scenarios.'
container: code-reviewer-073-spec-012-wire-trust
dark-factory-version: dev
created: "2026-05-02T10:00:00Z"
queued: "2026-05-02T10:14:28Z"
started: "2026-05-02T10:19:02Z"
completed: "2026-05-02T10:23:36Z"
branch: dark-factory/watcher-author-trust-filter
---

<summary>
- A new `TRUSTED_AUTHORS` env var and matching `-trusted-authors` CLI flag (comma-separated GitHub logins) supplies the allowlist to the watcher
- The watcher refuses to start when no trusted authors are configured, logging a clear operator-facing reason
- The watcher logs the configured trusted-author count (not the names) at startup
- The factory parses the raw trusted-authors string, constructs a `trust.And{trust.NewAuthorAllowlist(...)}` decision, and injects it into the watcher
- On the new-PR publish path, trusted PRs produce the same frontmatter as before (`phase: planning, status: in_progress`); untrusted PRs produce `phase: human_review, status: todo` with the documented untrusted-author body
- On the force-push re-review path, trust is re-evaluated; untrusted PRs keep `phase: human_review, status: todo` instead of being flipped back to auto-processing
- PRs with no author (`AuthorLogin == ""`) are treated as untrusted with the body showing "(unknown)" as the author
- Trust-leaf errors are logged and cause the PR to be skipped for the poll cycle; the cursor is not advanced past the failed PR
- Existing watcher integration tests are updated to supply a trust decision via the updated `newTestWatcher` helper; new test cases cover trusted-author create, untrusted-author create, untrusted force-push, trust-error skip, and missing-author defensive handling
- `CHANGELOG.md` gets an `## Unreleased` entry summarizing the trust gate
</summary>

<objective>
Wire the trust framework (from prompt 1) into the github-pr-watcher binary. Update `main.go` for config and startup validation, `factory/factory.go` to compose and inject the trust decision, and `pkg/watcher.go` to consult the trust decision on the create and force-push publish paths. Update all affected tests.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-factory-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `Create*` prefix, zero logic in factory functions, factory accepts deps as params.
Read `go-patterns.md` in `~/.claude/plugins/marketplaces/coding/docs/` — interface + constructor + private struct.
Read `go-error-wrapping-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `errors.Wrapf`, `errors.Errorf`, never `fmt.Errorf`.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo v2/Gomega, external test packages, counterfeiter.
Read `go-boolean-combinator-pattern.md` in `~/.claude/plugins/marketplaces/coding/docs/` — combinator structure for context on what is being integrated.
Read `changelog-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — changelog entry format.

Files to read before making any changes (read ALL before writing any code):
- `watcher/github/main.go` — application struct, `Run()` method, how `BotAllowlist` is parsed and used
- `watcher/github/pkg/filter.go` — `ParseBotAllowlist` and `ParseTrustedAuthors` (added in prompt 1)
- `watcher/github/pkg/factory/factory.go` — `CreateWatcher` signature and body
- `watcher/github/pkg/watcher.go` — `watcher` struct, `NewWatcher`, `publishCreate`, `publishForcePush`, `buildFrontmatter`, `buildTaskBody`
- `watcher/github/pkg/watcher_test.go` — `newTestWatcher` helper and all test cases
- `watcher/github/pkg/githubclient.go` — `PullRequest.AuthorLogin` field
- `watcher/github/pkg/trust/trust.go` — `Trust` interface signature (from prompt 1)
- `CHANGELOG.md` — check for existing `## Unreleased` section before writing

Spec-mandated untrusted-author body template (write EXACTLY this, substituting {author} and {reasons}):
```
## Untrusted author

This PR is by GitHub user **{author}** which did not pass the trust check:

- {reasons}

To auto-process this PR, edit the frontmatter above:
- `phase: in_progress`
- `status: in_progress`

To dismiss, set `status: aborted`.
```
</context>

<requirements>
**Execute steps in this order. Run `make test` after step 6; run `make precommit` only in step 8.**

1. **Run `make generate` to produce the Trust mock** (from the counterfeiter directive added in prompt 1):
   ```bash
   cd watcher/github && make generate
   ```
   Confirm `watcher/github/pkg/mocks/trust.go` exists after generation.

2. **Update `watcher/github/pkg/watcher.go`**:

   a. Add import for the trust package:
   ```go
   "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   ```

   b. Add `trustDecision trust.Trust` field to the `watcher` struct:
   ```go
   type watcher struct {
       ghClient       GitHubClient
       publisher      CommandPublisher
       cursorPath     string
       startTime      libtime.DateTime
       scope          string
       botAllowlist   []string
       stage          string
       metrics        Metrics
       trustDecision  trust.Trust
   }
   ```

   c. Add `trustDecision trust.Trust` parameter to `NewWatcher` (add as the last parameter after `metrics`):
   ```go
   func NewWatcher(
       ghClient GitHubClient,
       pub CommandPublisher,
       cursorPath string,
       startTime libtime.DateTime,
       scope string,
       botAllowlist []string,
       stage string,
       metrics Metrics,
       trustDecision trust.Trust,
   ) Watcher {
       return &watcher{
           ghClient:      ghClient,
           publisher:     pub,
           cursorPath:    cursorPath,
           startTime:     startTime,
           scope:         scope,
           botAllowlist:  botAllowlist,
           stage:         stage,
           metrics:       metrics,
           trustDecision: trustDecision,
       }
   }
   ```

   d. Add `buildHumanReviewFrontmatter` helper (alongside existing `buildFrontmatter`):
   ```go
   func buildHumanReviewFrontmatter(
       pr PullRequest,
       taskIDStr, stage string,
   ) agentlib.TaskFrontmatter {
       return agentlib.TaskFrontmatter{
           "assignee":        "pr-reviewer-agent",
           "phase":           "human_review",
           "status":          "todo",
           "stage":           stage,
           "task_identifier": taskIDStr,
           "title":           pr.Title,
       }
   }
   ```

   e. Add `buildUntrustedBody` helper. Use EXACTLY the template from the spec (no deviation):
   ```go
   func buildUntrustedBody(author, reasons string) string {
       return fmt.Sprintf(
           "## Untrusted author\n\nThis PR is by GitHub user **%s** which did not pass the trust check:\n\n- %s\n\nTo auto-process this PR, edit the frontmatter above:\n- `phase: in_progress`\n- `status: in_progress`\n\nTo dismiss, set `status: aborted`.\n",
           author,
           reasons,
       )
   }
   ```

   f. Replace the `publishCreate` method with the trust-aware version:
   ```go
   func (w *watcher) publishCreate(
       ctx context.Context,
       cursorState *Cursor,
       pr PullRequest,
       taskIDStr, headSHA string,
   ) bool {
       author := pr.AuthorLogin

       trustResult, err := w.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: author})
       if err != nil {
           glog.Errorf("trust check failed pr=%s err=%v", pr.HTMLURL, err)
           w.metrics.IncPRPublished("error")
           return false
       }

       var cmd agentlib.CreateTaskCommand
       if trustResult.Success() {
           cmd = agentlib.CreateTaskCommand{
               TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
               Frontmatter:    buildFrontmatter(pr, taskIDStr, w.stage),
               Body:           buildTaskBody(pr),
           }
       } else {
           if author == "" {
               author = "(unknown)"
           }
           glog.V(2).Infof("untrusted author=%q trust=%s pr=%s", author, trustResult.Description(), pr.HTMLURL)
           cmd = agentlib.CreateTaskCommand{
               TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
               Frontmatter:    buildHumanReviewFrontmatter(pr, taskIDStr, w.stage),
               Body:           buildUntrustedBody(author, trustResult.Description()),
           }
       }

       if err := w.publisher.PublishCreate(ctx, cmd); err != nil {
           glog.Errorf("publish create-task failed pr=%s err=%v", pr.HTMLURL, err)
           w.metrics.IncPRPublished("error")
           return false
       }
       cursorState.HeadSHAs[taskIDStr] = headSHA
       glog.V(2).Infof("published CreateTaskCommand pr=%s/%s#%d taskID=%s trusted=%t",
           pr.Owner, pr.Repo, pr.Number, taskIDStr, trustResult.Success())
       w.metrics.IncPRPublished("create")
       return true
   }
   ```

   g. Replace the `publishForcePush` method with the trust-aware version:
   ```go
   func (w *watcher) publishForcePush(
       ctx context.Context,
       cursorState *Cursor,
       pr PullRequest,
       taskIDStr, oldSHA, newSHA string,
   ) bool {
       author := pr.AuthorLogin

       trustResult, err := w.trustDecision.IsTrusted(ctx, trust.PR{AuthorLogin: author})
       if err != nil {
           glog.Errorf("trust check failed pr=%s err=%v", pr.HTMLURL, err)
           w.metrics.IncPRPublished("error")
           return false
       }

       heading := fmt.Sprintf("## Outdated by force-push %s", oldSHA)

       var updates agentlib.TaskFrontmatter
       var bodySection *agentlib.BodySection

       if trustResult.Success() {
           updates = agentlib.TaskFrontmatter{
               "phase":         "planning",
               "status":        "in_progress",
               "trigger_count": 0,
           }
           bodySection = &agentlib.BodySection{Heading: heading, Section: heading + "\n"}
       } else {
           if author == "" {
               author = "(unknown)"
           }
           glog.V(2).Infof("untrusted force-push author=%q trust=%s pr=%s", author, trustResult.Description(), pr.HTMLURL)
           updates = agentlib.TaskFrontmatter{
               "phase":         "human_review",
               "status":        "todo",
               "trigger_count": 0,
           }
           section := heading + "\n" + buildUntrustedBody(author, trustResult.Description())
           bodySection = &agentlib.BodySection{Heading: heading, Section: section}
       }

       cmd := agentlib.UpdateFrontmatterCommand{
           TaskIdentifier: agentlib.TaskIdentifier(taskIDStr),
           Updates:        updates,
           Body:           bodySection,
       }
       if err := w.publisher.PublishUpdateFrontmatter(ctx, cmd); err != nil {
           glog.Errorf("publish update-frontmatter failed pr=%s err=%v", pr.HTMLURL, err)
           w.metrics.IncPRPublished("error")
           return false
       }
       cursorState.HeadSHAs[taskIDStr] = newSHA
       glog.V(2).Infof("published UpdateFrontmatterCommand pr=%s/%s#%d taskID=%s trusted=%t",
           pr.Owner, pr.Repo, pr.Number, taskIDStr, trustResult.Success())
       w.metrics.IncPRPublished("update_frontmatter")
       return true
   }
   ```

3. **Update `watcher/github/pkg/factory/factory.go`**:

   a. Add import for the trust package:
   ```go
   "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   ```

   b. Add `trustedAuthors []string` as the last parameter to `CreateWatcher`:
   ```go
   func CreateWatcher(
       ctx context.Context,
       ghToken string,
       brokers libkafka.Brokers,
       stage string,
       repoScope string,
       botAllowlist []string,
       startTime libtime.DateTime,
       trustedAuthors []string,
   ) (pkg.Watcher, func(), error) {
   ```

   c. In the `CreateWatcher` body, compose the trust decision and pass it to `NewWatcher`:
   ```go
   trustDecision := trust.And{trust.NewAuthorAllowlist(trustedAuthors)}

   ghClient := pkg.NewGitHubClient(ghToken)
   w := pkg.NewWatcher(
       ghClient,
       pub,
       pkg.DefaultCursorPath,
       startTime,
       repoScope,
       botAllowlist,
       stage,
       pkg.NewMetrics(),
       trustDecision,
   )
   return w, cleanup, nil
   ```

4. **Update `watcher/github/main.go`**:

   a. Add `TrustedAuthors` field to the `application` struct, after `BotAllowlist`:
   ```go
   TrustedAuthors string `required:"false" arg:"trusted-authors" env:"TRUSTED_AUTHORS" usage:"Comma-separated trusted GitHub author logins (required; empty list refuses startup)"`
   ```

   b. In `Run()`, after the `botAllowlist` parsing line and before the `factory.CreateWatcher` call, add:
   ```go
   trustedAuthors := pkg.ParseTrustedAuthors(a.TrustedAuthors)
   if len(trustedAuthors) == 0 {
       return errors.Errorf(ctx, "no trusted authors configured: set TRUSTED_AUTHORS to a comma-separated list of GitHub logins")
   }
   glog.V(2).Infof("trusted-authors count=%d", len(trustedAuthors))
   ```

   c. Update the `factory.CreateWatcher` call to pass `trustedAuthors`:
   ```go
   w, cleanup, err := factory.CreateWatcher(
       ctx,
       a.GHToken,
       a.KafkaBrokers,
       a.Stage,
       a.RepoScope,
       botAllowlist,
       startTime,
       trustedAuthors,
   )
   ```

5. **Update `watcher/github/pkg/watcher_test.go`**:

   a. Import the trust package:
   ```go
   "github.com/bborbe/maintainer/watcher/github-pr/pkg/trust"
   "github.com/bborbe/maintainer/watcher/github-pr/pkg/mocks"
   ```
   (the trust mock `mocks.Trust` is generated in step 1)

   b. Update `newTestWatcher` to accept a `trust.Trust` parameter:
   ```go
   func newTestWatcher(
       ghClient pkg.GitHubClient,
       pub *mocks.CommandPublisher,
       cursorPath string,
       startTime libtime.DateTime,
       fakeMetrics *mocks.Metrics,
       trustDecision trust.Trust,
   ) pkg.Watcher {
       return pkg.NewWatcher(
           ghClient,
           pub,
           cursorPath,
           startTime,
           "bborbe",
           []string{"dependabot[bot]"},
           "dev",
           fakeMetrics,
           trustDecision,
       )
   }
   ```

   c. Update ALL existing `newTestWatcher` call sites in the file to pass `trust.NewAuthorAllowlist([]string{"alice"})` as the last argument. Search for every `newTestWatcher(ghClient, pub, cursorPath, startTime, fakeMetrics)` call and update it:
   ```go
   w := newTestWatcher(ghClient, pub, cursorPath, startTime, fakeMetrics, trust.NewAuthorAllowlist([]string{"alice"}))
   ```
   Do this for ALL call sites (there are multiple; update all of them).

   d. Add new test cases in the appropriate `Describe` blocks. Add a new `Describe("Trust decisions", ...)` block:
   ```go
   Describe("Trust decisions", func() {
       var (
           pr pkg.PullRequest
       )

       BeforeEach(func() {
           pr = pkg.PullRequest{
               Number:      10,
               Owner:       "bborbe",
               Repo:        "repo",
               Title:       "some PR",
               HTMLURL:     "https://github.com/bborbe/repo/pull/10",
               AuthorLogin: "alice",
               UpdatedAt:   libtime.DateTime(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)),
           }
           ghClient.SearchPRsReturns(pkg.SearchResult{
               PullRequests:  []pkg.PullRequest{pr},
               HasNextPage:   false,
               RateRemaining: 100,
           }, nil)
           ghClient.GetHeadSHAReturns("sha1", nil)
       })

       Describe("Trusted-author new PR", func() {
           It("publishes CreateTaskCommand with planning/in_progress frontmatter", func() {
               pub.PublishCreateReturns(nil)
               w := newTestWatcher(ghClient, pub, cursorPath, startTime, fakeMetrics,
                   trust.NewAuthorAllowlist([]string{"alice"}))
               Expect(w.Poll(ctx)).NotTo(HaveOccurred())
               Expect(pub.PublishCreateCallCount()).To(Equal(1))
               _, cmd := pub.PublishCreateArgsForCall(0)
               Expect(cmd.Frontmatter["phase"]).To(Equal("planning"))
               Expect(cmd.Frontmatter["status"]).To(Equal("in_progress"))
           })
       })

       Describe("Untrusted-author new PR", func() {
           It("publishes CreateTaskCommand with human_review/todo frontmatter and untrusted body", func() {
               pub.PublishCreateReturns(nil)
               w := newTestWatcher(ghClient, pub, cursorPath, startTime, fakeMetrics,
                   trust.NewAuthorAllowlist([]string{"bob"})) // alice is NOT trusted
               Expect(w.Poll(ctx)).NotTo(HaveOccurred())
               Expect(pub.PublishCreateCallCount()).To(Equal(1))
               _, cmd := pub.PublishCreateArgsForCall(0)
               Expect(cmd.Frontmatter["phase"]).To(Equal("human_review"))
               Expect(cmd.Frontmatter["status"]).To(Equal("todo"))
               Expect(cmd.Body).To(ContainSubstring("alice"))
               Expect(cmd.Body).To(ContainSubstring("Untrusted author"))
               Expect(cmd.Body).To(ContainSubstring("phase: in_progress"))
           })
       })

       Describe("Untrusted-author force-push", func() {
           It("re-evaluates trust and preserves human_review/todo state", func() {
               // First poll: register initial SHA (untrusted)
               pub.PublishCreateReturns(nil)
               w := newTestWatcher(ghClient, pub, cursorPath, startTime, fakeMetrics,
                   trust.NewAuthorAllowlist([]string{"bob"}))
               Expect(w.Poll(ctx)).NotTo(HaveOccurred())
               Expect(pub.PublishCreateCallCount()).To(Equal(1))

               // Second poll: force-push (untrusted author)
               pub2 := new(mocks.CommandPublisher)
               ghClient.GetHeadSHAReturns("sha2", nil)
               pub2.PublishUpdateFrontmatterReturns(nil)
               w2 := newTestWatcher(ghClient, pub2, cursorPath, startTime, fakeMetrics,
                   trust.NewAuthorAllowlist([]string{"bob"}))
               Expect(w2.Poll(ctx)).NotTo(HaveOccurred())
               Expect(pub2.PublishUpdateFrontmatterCallCount()).To(Equal(1))
               _, cmd := pub2.PublishUpdateFrontmatterArgsForCall(0)
               Expect(cmd.Updates["phase"]).To(Equal("human_review"))
               Expect(cmd.Updates["status"]).To(Equal("todo"))
           })
       })

       Describe("Trust check returns an error", func() {
           It("skips the PR, no publish, Poll returns nil, cursor NOT advanced past failed PR", func() {
               fakeErr := errors.New("github rate limit")
               fakeTrust := new(mocks.Trust)
               fakeTrust.IsTrustedReturns(nil, fakeErr)
               w := newTestWatcher(ghClient, pub, cursorPath, startTime, fakeMetrics, fakeTrust)
               Expect(w.Poll(ctx)).NotTo(HaveOccurred())
               Expect(pub.PublishCreateCallCount()).To(Equal(0))
               // Spec failure-mode contract: cursor must NOT advance past a PR whose
               // trust check errored — next poll cycle re-attempts the same PR.
               Expect(cursorState.HeadSHAs).NotTo(HaveKey(taskIDStr))
           })
       })

       Describe("PR with missing AuthorLogin (defensive)", func() {
           It("treats as untrusted and publishes human_review task", func() {
               pr.AuthorLogin = ""
               ghClient.SearchPRsReturns(pkg.SearchResult{
                   PullRequests:  []pkg.PullRequest{pr},
                   HasNextPage:   false,
                   RateRemaining: 100,
               }, nil)
               pub.PublishCreateReturns(nil)
               w := newTestWatcher(ghClient, pub, cursorPath, startTime, fakeMetrics,
                   trust.NewAuthorAllowlist([]string{"alice"}))
               Expect(w.Poll(ctx)).NotTo(HaveOccurred())
               Expect(pub.PublishCreateCallCount()).To(Equal(1))
               _, cmd := pub.PublishCreateArgsForCall(0)
               Expect(cmd.Frontmatter["phase"]).To(Equal("human_review"))
               Expect(cmd.Body).To(ContainSubstring("unknown"))
           })
       })
   })
   ```

   Note: you need `"errors"` in imports for `errors.New("...")` in the trust-error test — use the standard library `errors` (not `github.com/bborbe/errors`) for constructing the test error. Check the existing imports in `watcher_test.go`; it already imports `"errors"`.

6. **Compile and run tests** (iteratively — fix any compile errors before proceeding):
   ```bash
   cd watcher/github && go build ./...
   cd watcher/github && make test
   ```

7. **Update `CHANGELOG.md`**:

   Check whether `## Unreleased` already exists. If it does, append to it. If not, add it before the first `## v` section. Add this entry:
   ```markdown
   ## Unreleased

   - feat: add trusted-authors trust gate to github-pr-watcher; untrusted PR authors are routed to human_review instead of auto-processing; watcher refuses to start without a non-empty TRUSTED_AUTHORS list
   ```

8. **Final validation**:
   ```bash
   cd watcher/github && make precommit
   ```
</requirements>

<constraints>
- Only modify files in `watcher/github/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Do NOT add GitHub API calls — `AuthorLogin` is already available from `PullRequest.AuthorLogin`
- Do NOT change the `CreateTaskCommand` or `UpdateFrontmatterCommand` struct schema — only the values written into `Frontmatter` and `Body` change
- Do NOT modify `BotAllowlist` behavior — bot filtering remains an earlier, independent filter
- The `buildUntrustedBody` function must use the EXACT body template from the spec (no deviation from the heading, wording, or formatting)
- Trust evaluation must run on BOTH the `publishCreate` AND `publishForcePush` paths
- Trust-check errors must cause the PR to be skipped (return false) and the cursor must NOT advance for that PR
- `make generate` must run BEFORE `make test` in this prompt (step 1) to produce the Trust mock
- All error wrapping uses `github.com/bborbe/errors` — never `fmt.Errorf`
- `NewWatcher`'s `trustDecision` parameter is the last parameter (after `metrics`) — add it at the end to minimize diff on existing callers
- `CreateWatcher`'s `trustedAuthors []string` parameter is the last parameter (after `startTime`)
- The factory composes `trust.And{trust.NewAuthorAllowlist(trustedAuthors)}` — do not pass naked `[]string` to the watcher
- `make precommit` runs from `watcher/github/`, never from repo root
</constraints>

<verification>
cd watcher/github && make generate

cd watcher/github && go build ./...

cd watcher/github && make test

cd watcher/github && make precommit

# Confirm TrustedAuthors field is present in application struct
grep -n "TrustedAuthors\|TRUSTED_AUTHORS" watcher/github/main.go

# Confirm startup validation is present
grep -n "no trusted authors" watcher/github/main.go

# Confirm trust is injected by factory
grep -n "trustDecision\|NewAuthorAllowlist" watcher/github/pkg/factory/factory.go

# Confirm publishCreate and publishForcePush both call IsTrusted
grep -n "IsTrusted" watcher/github/pkg/watcher.go

# Confirm human_review frontmatter helper exists
grep -n "buildHumanReviewFrontmatter\|buildUntrustedBody" watcher/github/pkg/watcher.go

# Confirm CHANGELOG entry
grep -n "trusted-authors\|trust gate" CHANGELOG.md
</verification>
