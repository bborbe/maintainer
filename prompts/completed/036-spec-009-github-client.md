---
status: completed
spec: [009-github-pr-watcher]
summary: 'Implemented GitHub API layer: GitHubClient interface with SearchPRs/GetHeadSHA backed by go-github v62, filter.ShouldSkip/IsBotAuthor, taskid.Derive using UUID v5 SHA1, all with httptest.Server-based tests and counterfeiter mock generation'
container: code-reviewer-036-spec-009-github-client
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-27T20:45:00Z"
queued: "2026-04-27T21:02:13Z"
started: "2026-04-27T21:07:23Z"
completed: "2026-04-27T21:16:35Z"
---

<summary>
- A `GitHubClient` interface abstracts GitHub Search API calls so tests never hit the real API
- The implementation wraps `go-github` + `oauth2`, issues a single Search query (`is:pr is:open archived:false user:<scope> updated:>=<cursor>`) covering all in-scope repos at once
- Each call returns one page of results plus pagination metadata (`HasNextPage`, `NextPage`) and rate-limit metadata (`RateRemaining`, `RateResetAt`) — the orchestration that loops pagination + applies proactive rate-limit back-off lives in the poll loop (prompt 3), NOT here
- A second client method fetches a single PR's head SHA via `client.PullRequests.Get` — the Search API does not return head SHA, so a follow-up GET is required for accurate force-push detection
- A `Filter` function applies two rules: skip drafts; skip PRs whose author login is in the bot allowlist
- A `DeriveTaskIdentifier` function produces a deterministic v5 UUID from `<owner>/<repo>#<number>` using a fixed package-level namespace UUID
- All components are unit-tested with httptest.Server (single page, pagination via Link header, rate-limit headers, 401, ctx cancellation) plus table-driven tests for `Filter` and `DeriveTaskIdentifier`
</summary>

<objective>
Implement the GitHub API layer: a `GitHubClient` interface + concrete implementation using `go-github` and `oauth2`, a search result paginator, proactive rate-limit backoff, a PR filter, and deterministic task identifier derivation. No Kafka, no cursor, no poll orchestration — those come in prompt 3. The goal is a fully-tested GitHub boundary with mocked test doubles.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read the following guides from `~/.claude/plugins/marketplaces/coding/docs/`:
- `go-patterns.md` — Interface → Constructor → Struct, counterfeiter annotations
- `go-testing-guide.md` — Ginkgo v2/Gomega, coverage ≥80%, external test packages
- `go-error-wrapping-guide.md` — `bborbe/errors`, never `fmt.Errorf`
- `go-context-cancellation-in-loops.md` — non-blocking select in loops

**Precondition:** Prompt `1-spec-009-scaffold` must have been applied. Verify:
```bash
ls watcher/github/go.mod watcher/github/pkg/config/config.go
```
If missing, stop and report `status: failed` with "scaffold not applied — run 1-spec-009-scaffold first".

Read these files fully before implementing:
- `watcher/github/go.mod` — confirm go-github and uuid versions already pinned
- `agent/pr-reviewer/pkg/config/config.go` — pattern for typed config
- `agent/pr-reviewer/go.mod` — model for dependency style

**Symbol verification (mandatory before writing code):**

1. **go-github Search API** — run:
   ```bash
   grep -rn "func.*SearchIssues\|func.*Issues\|SearchOptions\|IssuesSearchResult" \
     $(go env GOPATH)/pkg/mod/github.com/google/go-github@*/github/search.go 2>/dev/null | head -20
   ```
   Note the exact method signature on `*SearchService`. The search method returns `(*IssuesSearchResult, *Response, error)` where `Response.NextPage` controls pagination and `Response.Rate` contains the rate-limit info.

2. **go-github Rate struct** — run:
   ```bash
   grep -n "type Rate struct\|Remaining\|Reset\|RateLimitError" \
     $(go env GOPATH)/pkg/mod/github.com/google/go-github@*/github/*.go 2>/dev/null | head -20
   ```
   Confirm `Rate.Remaining int` and `Rate.Reset github.Timestamp` (wraps `time.Time`).

3. **google/uuid v5 derivation** — run:
   ```bash
   grep -n "func NewSHA1\|func NewV5\|NameSpace" \
     $(go env GOPATH)/pkg/mod/github.com/google/uuid@*/uuid.go 2>/dev/null
   ```
   If `NewSHA1(namespace UUID, data []byte) UUID` exists, use it.
   If only `NewV5(namespace UUID, name string) UUID` exists, use `NewV5` with `name = "<owner>/<repo>#<number>"`.
   Use whichever form the library exports — do not invent the other.

4. **oauth2 transport** — run:
   ```bash
   grep -n "StaticTokenSource\|NewClient\|Token " \
     $(go env GOPATH)/pkg/mod/golang.org/x/oauth2@*/oauth2.go 2>/dev/null | head -10
   ```
   Confirm `oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})` pattern.
</context>

<requirements>
1. **Create `watcher/github/pkg/githubclient/client.go`**:

   Define the interface:
   ```go
   // PullRequest holds the fields the watcher needs from a GitHub PR.
   type PullRequest struct {
       GlobalID   int64   // maps to Issue.GetID() from go-github
       Number     int
       Owner      string
       Repo       string
       Title      string
       HTMLURL    string
       HeadSHA    string
       AuthorLogin string
       IsDraft    bool
       UpdatedAt  time.Time
   }

   // SearchResult is the result of a single paginated search call.
   type SearchResult struct {
       PullRequests []PullRequest
       HasNextPage  bool
       NextPage     int
       RateRemaining int
       RateResetAt   time.Time
   }

   //counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient
   type GitHubClient interface {
       // SearchPRs issues a GitHub Search query for open PRs updated since cursor.
       // page=1 for the first call; use SearchResult.NextPage for subsequent calls.
       // PullRequest.HeadSHA in the result is empty — call GetHeadSHA to fetch it.
       SearchPRs(ctx context.Context, scope string, since time.Time, page int) (SearchResult, error)

       // GetHeadSHA fetches the head commit SHA for a single PR. The Search
       // API does NOT return head SHA, so the poll loop must call this for
       // any PR it needs head-SHA tracking for (force-push detection).
       GetHeadSHA(ctx context.Context, owner, repo string, number int) (string, error)
   }
   ```

   The constructor:
   ```go
   // NewGitHubClient returns a GitHubClient backed by the real GitHub API.
   func NewGitHubClient(token string) GitHubClient
   ```

   Implementation:
   - Create an `oauth2.StaticTokenSource` with `token`, wrap in `oauth2.NewClient`.
   - Pass the oauth2 http.Client to `github.NewClient(httpClient)`.
   - Build search query: `fmt.Sprintf("is:pr is:open archived:false user:%s updated:>=%s", scope, since.UTC().Format(time.RFC3339))`.
   - Call `client.Search.Issues(ctx, query, &github.SearchOptions{ListOptions: github.ListOptions{Page: page, PerPage: 100}})`.
   - Map `IssuesSearchResult.Issues` to `[]PullRequest`. For each issue:
     - `Owner`: extract from `issue.GetRepositoryURL()` — the URL is `https://api.github.com/repos/{owner}/{repo}`, so parse the last two path segments.
     - `Repo`: same parse.
     - `GlobalID`: `issue.GetID()`
     - `Number`: `issue.GetNumber()`
     - `Title`: `issue.GetTitle()`
     - `HTMLURL`: `issue.GetHTMLURL()`
     - `HeadSHA`: leave empty `""` — Search API does NOT return head SHA. **Do NOT use `MergeCommitSHA` — that's the merge result, not the head, and would silently break force-push detection.** The poll loop calls `GetHeadSHA` separately for any PR it needs to track.
     - `AuthorLogin`: `issue.GetUser().GetLogin()`
     - `IsDraft`: `issue.GetDraft()` — grep verify `GetDraft()` exists on `Issue`.
     - `UpdatedAt`: `issue.GetUpdatedAt().Time`
   - Set `HasNextPage`: `resp.NextPage > 0`.
   - Set `NextPage`: `resp.NextPage`.
   - Set `RateRemaining`: `resp.Rate.Remaining`.
   - Set `RateResetAt`: `resp.Rate.Reset.Time`.
   - On error, return `errors.Wrapf(ctx, err, "search github prs scope=%s", scope)`.

   Then implement `GetHeadSHA`:
   ```go
   func (c *githubClient) GetHeadSHA(ctx context.Context, owner, repo string, number int) (string, error) {
       pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
       if err != nil {
           return "", errors.Wrapf(ctx, err, "get pull request %s/%s#%d", owner, repo, number)
       }
       return pr.GetHead().GetSHA(), nil
   }
   ```
   Add a corresponding httptest test (`GET /repos/<o>/<r>/pulls/<n>` returning a PR JSON with `head.sha`).

2. **Create `watcher/github/pkg/githubclient/client_test.go`** using `httptest.Server`:

   Set up a test server that handles `GET /search/issues` and returns canned JSON. Use Ginkgo/Gomega.

   Test cases (each with its own `httptest.NewServer`):

   a. **Single page, two PRs, no next page**:
      Response: `{"total_count":2,"items":[...two PR items...]}`, no `Link` header.
      `X-RateLimit-Remaining: 4999`, `X-RateLimit-Reset: <future unix>`.
      Assert: `len(result.PullRequests) == 2`, `HasNextPage == false`, `RateRemaining == 4999`.

   b. **Pagination**: first page returns 1 PR + `Link: <.../search/issues?page=2>; rel="next"`.
      Second call with `page=2` returns 1 PR with no next link.
      Assert both calls happen, first result has `HasNextPage == true`, `NextPage == 2`.

   c. **Rate limit check fields**: response with `X-RateLimit-Remaining: 5` and `X-RateLimit-Reset: <unix ts>`.
      Assert `result.RateRemaining == 5`.

   d. **GitHub API error (HTTP 401)**: server returns 401 JSON. Assert `SearchPRs` returns non-nil error.

   e. **Context cancellation**: cancel ctx before calling `SearchPRs`. Assert non-nil error.

   **Important:** Replace `github.NewClient` default base URL with the test server URL. After constructing the go-github client, set:
   ```go
   client.BaseURL, _ = url.Parse(testServer.URL + "/")
   ```
   Grep for how `BaseURL` is set in go-github: `grep -n "BaseURL" $(go env GOPATH)/pkg/mod/github.com/google/go-github@*/github/github.go | head -10`.

3. **Create `watcher/github/pkg/filter/filter.go`**:

   ```go
   package filter

   import "github.com/bborbe/maintainer/watcher/github-pr/pkg/githubclient"

   // IsBotAuthor returns true if the PR author is in the allowlist.
   func IsBotAuthor(pr githubclient.PullRequest, allowlist []string) bool

   // ShouldSkip returns true if the PR should be filtered out (draft or bot-authored).
   func ShouldSkip(pr githubclient.PullRequest, botAllowlist []string) bool
   ```

   Implementation:
   - `IsBotAuthor`: iterate allowlist, return true if `pr.AuthorLogin == entry` (exact match).
   - `ShouldSkip`: return `pr.IsDraft || IsBotAuthor(pr, botAllowlist)`.

4. **Create `watcher/github/pkg/filter/filter_test.go`** (Ginkgo/Gomega, external package `filter_test`):

   Table-driven tests using `DescribeTable`:
   - Draft PR, empty allowlist → skipped
   - Non-draft PR, empty allowlist → not skipped
   - Non-draft PR, author in allowlist → skipped
   - Non-draft PR, author NOT in allowlist → not skipped
   - Draft PR, author in allowlist → skipped (both conditions true)
   - Case sensitivity: allowlist entry `"Dependabot[bot]"` does NOT match `"dependabot[bot]"` (exact match)

5. **Create `watcher/github/pkg/taskid/taskid.go`**:

   ```go
   package taskid

   import (
       "fmt"
       "github.com/google/uuid"
   )

   // prWatcherNamespace is the fixed v5 UUID namespace for all watcher-derived task identifiers.
   // This value is a constant — changing it invalidates all existing task identifiers.
   var prWatcherNamespace = uuid.MustParse("7d4b3e5f-8a21-4c9d-b036-2e5f7a8c1d0e")

   // Derive returns a deterministic task identifier for a PR.
   // Input: "<owner>/<repo>#<number>", e.g. "bborbe/code-reviewer#42".
   // Output: v5 UUID derived via SHA-1 with the fixed prWatcherNamespace.
   func Derive(owner, repo string, number int) uuid.UUID {
       key := fmt.Sprintf("%s/%s#%d", owner, repo, number)
       // Use NewSHA1 or NewV5 — whichever the uuid library exports (grep-verified in step above)
       return uuid.NewSHA1(prWatcherNamespace, []byte(key))
   }
   ```

   **Adapt to actual uuid API based on grep result from context step.** If the library uses `NewV5(namespace, name string)` instead, use: `return uuid.NewV5(prWatcherNamespace, key)`.

6. **Create `watcher/github/pkg/taskid/taskid_test.go`** (external package `taskid_test`):

   - Same inputs always produce the same UUID (determinism test — call `Derive` twice with identical args).
   - Different `owner/repo#number` combos produce different UUIDs.
   - `Derive("bborbe","code-reviewer",42)` produces a specific expected UUID — compute it once and hardcode as `Expected("bborbe/code-reviewer#42" → <computed value>)` so the test pins the exact output.

   To compute the expected value: run a small Go snippet in the test `init` or compute manually:
   ```go
   // In the test file, compute once and verify
   expected := uuid.NewSHA1(uuid.MustParse("7d4b3e5f-8a21-4c9d-b036-2e5f7a8c1d0e"), []byte("bborbe/code-reviewer#42"))
   Expect(taskid.Derive("bborbe","code-reviewer",42)).To(Equal(expected))
   ```
   (This pins behavior without needing an external fixture.)

7. **Generate Counterfeiter mocks** for `GitHubClient`:
   ```bash
   cd watcher/github && go generate ./pkg/githubclient/...
   ```
   This creates `watcher/github/pkg/githubclient/mocks/github_client.go`.

8. **Create suite test files** for each new package (`githubclient`, `filter`, `taskid`) following the pattern from `agent/pr-reviewer/`:
   ```go
   // pkg/githubclient/suite_test.go
   package githubclient_test

   import (
       "testing"
       "github.com/onsi/ginkgo/v2"
       "github.com/onsi/gomega"
   )

   func TestGitHubClient(t *testing.T) {
       gomega.RegisterFailHandler(ginkgo.Fail)
       ginkgo.RunSpecs(t, "GitHubClient Suite")
   }
   ```
   Create equivalent files for `filter_test` and `taskid_test` packages.

9. **Run `cd watcher/github && make test`** — must pass.

10. **Check coverage for all three packages** (≥80% each):
    ```bash
    cd watcher/github && go test -coverprofile=/tmp/cover.out ./pkg/githubclient/... ./pkg/filter/... ./pkg/taskid/... && go tool cover -func=/tmp/cover.out
    ```

11. **Run `cd watcher/github && make precommit`** — must exit 0.
</requirements>

<constraints>
- `GitHubClient` interface must be in `pkg/githubclient/` — the concrete implementation is package-private; only the interface and constructor are exported
- Tests for `GitHubClient` must use `httptest.NewServer` — never call the real GitHub API
- All symbol references in go-github must be grep-verified before use — do NOT assume field/method names from memory
- `prWatcherNamespace` UUID is a package-level `var` (not `const` — `uuid.UUID` is a type, not a primitive); its value must be a fixed literal that never changes
- `IsBotAuthor` uses exact string equality — no glob, no regex, no case folding
- Errors wrapped with `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Counterfeiter mocks generated via `go generate`, not hand-written
- Do NOT run `go mod vendor`
- Do NOT commit — dark-factory handles git
- All existing tests must still pass
- `make precommit` must exit 0
</constraints>

<verification>
```bash
# Precondition
ls watcher/github/go.mod

# Interface and types exist
grep -n "type GitHubClient interface\|type PullRequest struct\|type SearchResult struct" watcher/github/pkg/githubclient/client.go

# Filter functions
grep -n "func ShouldSkip\|func IsBotAuthor" watcher/github/pkg/filter/filter.go

# task identifier derivation
grep -n "func Derive\|prWatcherNamespace" watcher/github/pkg/taskid/taskid.go

# Counterfeiter mock generated
ls watcher/github/pkg/githubclient/mocks/github_client.go

# No real GitHub API calls in tests
grep -rn "api.github.com" watcher/github/pkg/githubclient/client_test.go
# Expected: no matches

# Coverage
cd watcher/github && go test -coverprofile=/tmp/cover.out ./pkg/githubclient/... ./pkg/filter/... ./pkg/taskid/... && go tool cover -func=/tmp/cover.out

# Full precommit
cd watcher/github && make precommit
```
</verification>
