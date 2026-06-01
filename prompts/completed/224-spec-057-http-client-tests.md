---
status: completed
spec: [057-github-releaser-ai-review-phase]
summary: 'Added missing test cases to client_test.go: transport error, 404 for ResolveTagCommit/FetchChangelog, bearer token sanitization, and header verification for all methods'
container: maintainer-releaser-ai-review-exec-224-spec-056-http-client-tests
dark-factory-version: v0.173.0
created: "2026-05-31T20:35:00Z"
queued: "2026-05-31T20:54:57Z"
started: "2026-05-31T21:26:07Z"
completed: "2026-05-31T21:29:46Z"
branch: dark-factory/github-releaser-ai-review-phase
---

<summary>
- 10+ test cases covering TagExists, ResolveTagCommit (annotated+lightweight), FetchChangelog (happy+errors)
- HTTP transport failures, 404 on tag, non-base64 encoding, annotated tag indirection
- Bearer token never appears in error messages
- Tests use httptest.Server with real http.Client hitting the test server
- Counterfeiter mock for Client interface used in step tests
</summary>

<objective>
Write unit tests for the HTTP GitHub client (`pkg/githubreview/client.go`) using `httptest` to simulate the GitHub API. Test all three methods: TagExists, ResolveTagCommit, and FetchChangelog. Also test error paths (transport failures, 404s, non-2xx responses, non-base64 encoding). Include a test that verifies the bearer token never appears in error messages.
</objective>

<context>
Read `agent/github-releaser/pkg/githubreview/client.go` for the HTTP client implementation.
Read `agent/github-releaser/pkg/githubchangelog/fetcher_test.go` for the existing HTTP fetcher test pattern (httptest.Server, json responses).
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`.

Key test server patterns from the existing codebase:
- `httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {...}))`
- JSON responses via `w.Header().Set("Content-Type", "application/json")` and `json.NewEncoder(w).Encode(...)`
- Auth header verification: `r.Header.Get("Authorization")` == `"Bearer test-token"`
</context>

<requirements>
1. Create `agent/github-releaser/pkg/githubreview/client_test.go` in package `githubreview_test` (external test package).

2. Use `httptest.NewServer` to simulate GitHub API responses. The test server URL is passed to a constructor variant.

3. Use the `NewHTTPClientForTest` variable already exposed by prompt 2's `export_test.go` (mirrors `githubchangelog.NewHTTPFetcherForTest`). Build clients against the test server as:
   ```go
   client := githubreview.NewHTTPClientForTest("test-token", ts.URL)
   ```
   Do NOT add or modify any production-code constructor in this prompt — that constructor (`newHTTPClientWithBase`) and the `export_test.go` alias are owned by prompt 2. This prompt writes ONLY the test file (`client_test.go`); making production-code edits here would conflict with prompt 2 and risk losing prompt 2's defensive validation / receiver-name choices.

4. Test cases:

   **4a. TagExists: 200 returns tag SHA**:
   - Server responds to `GET /repos/owner/repo/git/ref/tags/v1.0.0` with JSON: `{"ref": "refs/tags/v1.0.0", "object": {"sha": "abc123", "type": "commit"}}`
   - Client returns `"abc123", nil`
   - Verify Authorization header is `"Bearer test-token"`

   **4b. TagExists: 404 returns `pkg.ErrTagNotFound` sentinel**:
   - Server responds with status 404, body `{"message": "Not Found"}`
   - Client returns `("", err)` where `errors.Is(err, pkg.ErrTagNotFound) == true` (the typed sentinel from prompt 1's `pkg` package — the ai_review step uses this to distinguish 404 → verdict from 5xx → retry)
   - Import: `pkg "github.com/bborbe/maintainer/agent/github-releaser/pkg"`

   **4c. TagExists: transport error**:
   - Server does not start (or close before request)
   - Client returns wrapped error with message containing `"TagExists: http"`

   **4d. TagExists: 500 returns error**:
   - Server responds with status 500
   - Client returns error with message containing `"TagExists: status 500"`

   **4e. ResolveTagCommit: lightweight tag (type=commit) returns SHA directly**:
   - Server responds to `GET /repos/owner/repo/git/tags/abc123` with JSON: `{"sha": "abc123", "object": {"sha": "abc123", "type": "commit"}}`
   - Client returns `"abc123", nil`

   **4f. ResolveTagCommit: annotated tag (type=tag) follows indirection**:
   - Server responds to `GET /repos/owner/repo/git/tags/tag-sha-annotated` with JSON: `{"sha": "tag-sha-annotated", "object": {"sha": "actual-commit-sha", "type": "tag"}}`
   - Client returns `"actual-commit-sha", nil` (follows the nested object sha)

   **4g. ResolveTagCommit: 404**:
   - Server responds with 404
   - Client returns error with message containing `"ResolveTagCommit: status 404"`

   **4h. FetchChangelog: 200 with base64 content**:
   - Use `expectedContent := []byte("# Changelog\n\n## v1.0.0\n\n- feat\n")` then `encoded := base64.StdEncoding.EncodeToString(expectedContent)` at test setup (do NOT hardcode a base64 literal — historically those drift from their plaintext annotation; computing it at test time guarantees the round-trip is faithful).
   - Server responds to `GET /repos/owner/repo/contents/CHANGELOG.md` with JSON: `{"encoding": "base64", "content": "<encoded>"}`.
   - Client returns bytes equal to `expectedContent` (decoded, not the raw base64 string).

   **4i. FetchChangelog: non-base64 encoding returns error**:
   - Server responds with `{"encoding": "utf-8", "content": "plain text"}`
   - Client returns error with message containing `"unsupported encoding"`, `"utf-8"`

   **4j. FetchChangelog: 404**:
   - Server responds with 404
   - Client returns error with message containing `"FetchChangelog: status 404"`

   **4k. Bearer token never in error messages**:
   - Use a long fake token: `"ghp_verylongtokenthatweshouldnotsee1234567890abcdef"`
   - Server responds with 500
   - Client returns error
   - Assert: `strings.Contains(err.Error(), token) == false`
   - Assert: `strings.Contains(err.Error(), "ghp_") == false`
   - The error message may contain the method name and status code, but never the token value

   **4l. Authorization header is set correctly**:
   - Verify that for each method (TagExists, ResolveTagCommit, FetchChangelog), the test server receives `Authorization: Bearer <token>`
   - Use a token that is obviously fake (e.g. `"test-token-xyz"`) and assert the server handler receives exactly that value

   **4m. Accept and X-GitHub-Api-Version headers**:
   - Verify the client sets `Accept: application/vnd.github+json` and `X-GitHub-Api-Version: 2022-11-28` for all requests

5. Test structure: `var _ = Describe("HTTPClient", func() { ... })` at top level, with `Describe` for each method. Use `var ts *httptest.Server` in a BeforeEach that starts the server, and an AfterEach that closes it.

6. For the test server handler, match URL paths with `r.URL.Path` (do not use `r.RequestURI` which includes query strings). Use `r.URL.Query().Get("ref")` if testing query params (though FetchChangelog has no query params per spec requirement).

7. External test package (`githubreview_test`) — do NOT use `package githubreview` for tests.

8. **Do NOT add any `//counterfeiter:generate` directive or production-code change in this prompt** — prompt 2 already adds the directive (`//counterfeiter:generate -o ../../mocks/review_client.go --fake-name ReviewClient . Client`) in `client.go`. Duplicating it here would create conflicting directives (different flags) and rewrite the mock with wrong output path or name. This prompt creates ONLY `client_test.go` (in `package githubreview_test`).
</requirements>

<constraints>
- Tests must be in an external `pkg_test` package (or `githubreview_test` for the sub-package).
- Counterfeiter mocks from `mocks/` directory.
- Ginkgo v2 + Gomega conventions.
- Use `httptest.Server` for the HTTP layer — do not mock `http.Client` directly.
- Bearer token must never appear in error messages (test this explicitly).
- Existing tests under `pkg/...` must continue to pass.
- Do NOT commit — dark-factory handles git.
</constraints>

<verification>
Run `cd agent/github-releaser && go generate ./...` — must produce the mock.
Run `cd agent/github-releaser && make test` — all tests must pass.
</verification>