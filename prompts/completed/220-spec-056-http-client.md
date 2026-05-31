---
status: completed
spec: [056-github-releaser-ai-review-phase]
summary: Implemented githubreview HTTP client with TagExists, ResolveTagCommit, and FetchChangelog methods; generated ReviewClient mock; 28 tests passing
container: maintainer-releaser-ai-review-exec-220-spec-056-http-client
dark-factory-version: v0.173.0
created: "2026-05-31T20:35:00Z"
queued: "2026-05-31T20:54:57Z"
started: "2026-05-31T20:58:13Z"
completed: "2026-05-31T21:02:56Z"
branch: dark-factory/github-releaser-ai-review-phase
---

<summary>
- HTTP client calling three GitHub REST API endpoints (tag ref, tag object, contents)
- Bearer token auth, 15-second timeout, proper error wrapping
- Returns `pkg.ErrTagNotFound` sentinel on 404 from TagExists (consumed by ai_review step via `errors.Is`)
- No token in logs, url.PathEscape for path segments (no query params)
- Returns commit SHA for annotated tags, handles lightweight vs annotated tag indirection
- Package-private `newHTTPClientWithBase(token, apiBase string)` + `export_test.go` test seam for httptest.Server injection (mirrors `githubchangelog`)
</summary>

<objective>
Implement `github.com/bborbe/maintainer/agent/github-releaser/pkg/githubreview.NewHTTPClient(ghToken string) aiReviewClient` — the concrete HTTP client that implements the `aiReviewClient` interface defined in the ai_review step. This client performs the three GitHub REST API calls: tag reference lookup, tag object resolution (following annotated tag indirection), and CHANGELOG fetch from the default branch.
</objective>

<context>
Read `agent/github-releaser/pkg/steps_ai_review.go` to see the `aiReviewClient` interface definition.
Read `agent/github-releaser/pkg/githubchangelog/fetcher.go` for the existing HTTP fetch pattern used by the planning step — use it as the template for error wrapping style, client setup, and token injection.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`.
Read `agent/github-releaser/pkg/plan_output.go` and `agent/github-releaser/pkg/result_output.go` for context.

Key verified API shapes:
- `github.com/bborbe/errors`: `Wrapf(ctx, err, format, args...)`, `Errorf(ctx, format, args...)`
- GitHub Contents API response shape: `{"encoding": "base64", "content": "<base64>"}` (from `githubchangelog/fetcher.go`)
- GitHub Refs API response shape: `{"ref": "...", "object": {"sha": "...", "type": "..."}}`
- GitHub Tag API response for annotated tag: `{"sha": "...", "object": {"sha": "...", "type": "tag"}}`
- GitHub Tag API response for lightweight tag: `{"sha": "...", "object": {"sha": "...", "type": "commit"}}`
- Bearer token: `Authorization: Bearer <token>` header
- 15-second `http.Client.Timeout` (matches `githubchangelog/fetcher.go`)
</context>

<requirements>
1. Create `agent/github-releaser/pkg/githubreview/client.go` in package `githubreview`.

2. Package structure:
   ```go
   package githubreview

   import (
       "context"
       "encoding/base64"
       "encoding/json"
       "fmt"
       "io"
       "net/http"
       "net/url"
       "strings"
       "time"

       "github.com/bborbe/errors"
       "github.com/golang/glog"
   )
   ```

3. `//counterfeiter:generate -o ../../mocks/review_client.go --fake-name ReviewClient . Client` — **the `-o` path and `--fake-name` MUST exactly match this** so sibling prompt 5 can `import "github.com/bborbe/maintainer/agent/github-releaser/mocks"` and instantiate `&mocks.ReviewClient{}`.

4. Define `Client` interface — the method set MUST exactly match `pkg.AIReviewClient` defined in prompt 1 so the factory can pass `*githubreview.httpClient` as a `pkg.AIReviewClient` via structural typing:
   ```go
   type Client interface {
       TagExists(ctx context.Context, owner, repo, tag string) (tagSHA string, _ error)
       ResolveTagCommit(ctx context.Context, owner, repo, tagSHA string) (commitSHA string, _ error)
       FetchChangelog(ctx context.Context, owner, repo string) ([]byte, error)
   }
   ```

5. Constructor shape (mirrors `githubchangelog/fetcher.go` lines 47-62):
   ```go
   // NewHTTPClient returns the production client against api.github.com.
   func NewHTTPClient(token string) Client {
       return newHTTPClientWithBase(token, "https://api.github.com")
   }

   // newHTTPClientWithBase is the test seam — package-private so tests via
   // export_test.go can point the client at an httptest.Server.
   // Mirrors githubchangelog.newHTTPFetcherWithBase pattern.
   func newHTTPClientWithBase(token, apiBase string) *httpClient {
       return &httpClient{
           client:  &http.Client{Timeout: 15 * time.Second},
           token:   token,
           apiBase: apiBase,
       }
   }
   ```

   Also create `agent/github-releaser/pkg/githubreview/export_test.go` (in `package githubreview`) that aliases the test seam for external `githubreview_test` consumers:
   ```go
   package githubreview

   // NewHTTPClientForTest exposes newHTTPClientWithBase for external tests.
   // Mirrors githubchangelog/export_test.go.
   var NewHTTPClientForTest = newHTTPClientWithBase
   ```

6. `TagExists(ctx, owner, repo, tag string) (string, error)` — receiver name is `c` (for `httpClient`); do NOT copy `f` from `githubchangelog.httpFetcher`:
   a. Validate non-empty: if `owner == ""` || `repo == ""` || `tag == ""`, return `errors.Errorf(ctx, "TagExists: owner/repo/tag must be non-empty")`. (Matches `githubchangelog/fetcher.go:83-92` defensive pattern.)
   b. Build URL: `fmt.Sprintf("%s/repos/%s/%s/git/ref/tags/%s", c.apiBase, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag))`
   c. `req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)`; wrap error as `"TagExists: build request"`. (CRITICAL: use `NewRequestWithContext` so ctx cancellation propagates — bare `http.NewRequest` loses cancellation.)
   d. Set headers: `Authorization: Bearer <token>`, `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`.
   e. `resp, err := c.client.Do(req)` — on transport error: return wrapped error with message `"TagExists: http"`.
   f. `defer resp.Body.Close()`; read body via `io.ReadAll` (truncate to 1KB for error messages, same as `fetcher.go:117-122`).
   g. On non-2xx: **if `resp.StatusCode == 404`, return `"", ErrTagNotFound` (the sentinel from prompt 1's `pkg` package — import as `pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"`)**. For other non-2xx, return `errors.Errorf(ctx, "TagExists: status %d: %s", resp.StatusCode, truncatedBody)`.
   h. Decode JSON into `refResponse{Ref string, Object refObject{SHA string, Type string}}`; wrap decode error as `"TagExists: decode json"`.
   i. Return `refResp.Object.SHA, nil`.

7. `ResolveTagCommit(ctx, owner, repo, tagSHA string) (string, error)`:
   a. Validate non-empty (same shape as 6a) — wrap as `"ResolveTagCommit: owner/repo/tagSHA must be non-empty"`.
   b. Build URL: `fmt.Sprintf("%s/repos/%s/%s/git/tags/%s", c.apiBase, url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tagSHA))`.
   c. `http.NewRequestWithContext` + same auth/accept headers as TagExists.
   d. `c.client.Do(req)`; defer close; read+truncate body.
   e. On non-2xx: return wrapped error `"ResolveTagCommit: status %d: %s"`.
   f. Decode JSON: `tagResponse{SHA string, Object tagObject{SHA string, Type string}}`; wrap decode error as `"ResolveTagCommit: decode json"`.
   g. If `Object.Type == "tag"`: return `Object.SHA` (annotated tag → wrapped commit SHA).
   h. If `Object.Type == "commit"`: return `Object.SHA` (lightweight tag SHA is already a commit).
   i. Unknown type: return `errors.Errorf(ctx, "ResolveTagCommit: unknown tag object type %q for tag %s", Object.Type, tagSHA)`.

8. `FetchChangelog(ctx, owner, repo string) ([]byte, error)`:
   a. Validate non-empty (same shape as 6a).
   b. Build URL: `fmt.Sprintf("%s/repos/%s/%s/contents/CHANGELOG.md", c.apiBase, url.PathEscape(owner), url.PathEscape(repo))` — NO `?ref=` parameter.
   c. `http.NewRequestWithContext` + same auth/accept headers.
   d. `c.client.Do(req)`; defer close; read+truncate body.
   e. On non-2xx: return wrapped error `"FetchChangelog: status %d: %s"`.
   f. Decode JSON: `contentResponse{Encoding string, Content string}`; wrap decode error as `"FetchChangelog: decode json"`.
   g. Validate `Encoding == "base64"`. If not: return `errors.Errorf(ctx, "FetchChangelog: unsupported encoding %q (want base64)", Encoding)`.
   h. Strip whitespace: `strings.NewReplacer("\n", "", "\r", "").Replace(cr.Content)`.
   i. `base64.StdEncoding.DecodeString(cleaned)`. Wrap error as `"FetchChangelog: base64 decode"`.
   j. Return decoded bytes, nil.

9. Logging: use `glog.V(2).Infof` for outbound requests (method + URL pattern + status + bytes). Never log the bearer token. Token appears only in the `Authorization` header, never in any log line.

10. Error messages must identify the method name and the specific failure: `"TagExists: http: ..."`, `"TagExists: tag %q not found (404)"`, `"ResolveTagCommit: status %d"`, `"FetchChangelog: status %d"`.

11. Internal types (not exported):
    ```go
    type refResponse struct {
        Ref   string    `json:"ref"`
        Object refObject `json:"object"`
    }
    type refObject struct {
        SHA  string `json:"sha"`
        Type string `json:"type"`
    }
    type tagResponse struct {
        SHA   string    `json:"sha"`
        Object tagObject `json:"object"`
    }
    type tagObject struct {
        SHA  string `json:"sha"`
        Type string `json:"type"`
    }
    type contentResponse struct {
        Encoding string `json:"encoding"`
        Content  string `json:"content"`
    }
    type httpClient struct {
        client  *http.Client
        token   string
        apiBase string
    }
    ```

12. The `FetchChangelog` method must NOT hardcode `main` — do not pass a `?ref=` query parameter. The GitHub API defaults to the repo's actual default branch.

13. Run `go generate ./...` to generate the counterfeiter mock at `mocks/review_client.go`.
</requirements>

<constraints>
- Do NOT add configurable timeouts, base URLs, or token sources — the values are hardcoded to match the existing planning step's HTTP fetcher pattern.
- Bearer token must never appear in any log line at any verbosity.
- Use `github.com/bborbe/errors` for all error wrapping. Never bare `return err`, never `fmt.Errorf`.
- The counterfeiter directive `//counterfeiter:generate -o ../../mocks/review_client.go --fake-name ReviewClient . Client` goes in `client.go`.
- Run `go generate ./...` in `agent/github-releaser/` to produce the mock.
</constraints>

<verification>
Run `cd agent/github-releaser && go generate ./...` — must produce `mocks/review_client.go`.
Run `cd agent/github-releaser && make test` — must pass.
</verification>