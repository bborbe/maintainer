---
status: completed
spec: [044-github-release-watcher-implementation]
summary: Implemented four GitHub API methods (ListRepos, GetMasterSHA, GetChangelogContent, GetAutoReleaseConfig) using google/go-github/v84 with full httptest coverage
container: maintainer-github-release-exec-191-spec-044-githubclient
dark-factory-version: v0.173.0
created: "2026-05-27T20:38:37Z"
queued: "2026-05-27T20:57:47Z"
started: "2026-05-27T21:42:26Z"
completed: "2026-05-27T21:51:03Z"
---

<summary>
- Four GitHub API methods get real implementations: `ListRepos`, `GetMasterSHA`, `GetChangelogContent`, `GetAutoReleaseConfig`
- Uses `google/go-github/v84` (already in `go.mod` indirect — needs promotion to direct via `go get` during this prompt)
- 404 returns `(nil, nil)` for both content fetchers (no-CHANGELOG and no-`.dark-factory/config.yml` are normal, not errors)
- Rate-limit (`*RateLimitError` or `*AbuseRateLimitError`) surfaces as `ErrRateLimited` so the Watcher can label `IncPollCycle("rate_limited")` correctly
- Tests use `httptest.NewServer` — never touch the real GitHub API
- `make generate` creates `pkg/mocks/github_client.go` from the existing counterfeiter directive
</summary>

<objective>
Replace the four TODO stub methods in `watcher/github-release/pkg/githubclient.go` with real `google/go-github/v84` calls backed by the supplied `*http.Client`. Add httptest-driven unit tests covering happy paths, 404 sentinels, rate-limit detection, and the `autoRelease: true` YAML parse path.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md` — for the paginated `ListRepos` loop
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`

Read these files end-to-end:
- `watcher/github-release/pkg/githubclient.go` — interface contract you MUST preserve
- `watcher/github-release/pkg/repo.go` — `Repo{Owner, Name, DefaultBranch}.Key()` shape
- `watcher/github-release/go.mod` — confirm `github.com/google/go-github/v84 v84.0.0` is present (currently indirect — promote to direct via `go get` in step 0)

Canonical reference implementation — READ IN FULL and mirror error-handling exactly:
- `/workspace/watcher/github-build/pkg/githubclient.go` — same `lib/githubapp` auth shape, identical rate-limit detection idiom (`stderrors.As(err, &rl)` / `stderrors.As(err, &arl)`), identical `GetContents` 404-as-nil-nil idiom, identical `ListByUser` / `ListByOrg` pagination loop with `wrapRateLimitErr` helper and `filterRepoNames`.
- `/workspace/watcher/github-build/pkg/githubclient_test.go` — Ginkgo httptest pattern with `client.BaseURL` override.

The github-release variant differs from github-build in only three ways:
1. `ListRepos` returns `[]Repo` (struct with `Owner`, `Name`, `DefaultBranch`) — NOT `[]string`. Each `Repo` carries `DefaultBranch` cached from the list response, removing the need for the separate `GetDefaultBranch` round-trip on subsequent calls.
2. `GetMasterSHA` is named after the default-branch HEAD — internally it calls `client.Repositories.GetBranch(ctx, owner, repo, repo.DefaultBranch, 0)` and returns `branch.GetCommit().GetSHA()`. The 0 is the `maxRedirects` parameter; this signature is verified below.
3. `GetAutoReleaseConfig` fetches `.dark-factory/config.yml` from the default-branch HEAD, parses YAML with `gopkg.in/yaml.v3` (already in `go.sum`), and returns the `autoRelease` boolean. Missing file = `(false, nil)`. Missing key in the YAML = `(false, nil)`.

Symbol verification before writing (run these greps in the prompt's working dir):
```bash
# Confirm go-github v84 API shapes
grep -n "func.*GetContents\|RepositoryContentGetOptions\|RepositoryListByUserOptions\|RepositoryListByOrgOptions" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v84@v84.0.0/github/*.go 2>/dev/null | head -20

grep -n "func.*GetBranch\b\|type Branch " \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v84@v84.0.0/github/repos_branches.go 2>/dev/null | head -10

grep -n "type RateLimitError\|type AbuseRateLimitError\|type ErrorResponse" \
  $(go env GOPATH)/pkg/mod/github.com/google/go-github/v84@v84.0.0/github/github.go 2>/dev/null | head
```

If `GetBranch` in v84 has a different signature than v62, ADAPT — do not invent. Read the function definition before calling.

YAML parsing: import `gopkg.in/yaml.v3`. Define an unexported struct `type darkFactoryConfig struct { AutoRelease bool \`yaml:"autoRelease"\` }`. The zero-value `AutoRelease == false` cleanly handles missing-key.

Counterfeiter / mock note: `pkg/githubclient.go` already has `//counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient` at line 14. The destination `mocks/` is RELATIVE to the file containing the directive, so this lands in `pkg/mocks/github_client.go` (per spec AC). Verify by `ls pkg/mocks/github_client.go` after `make generate`.
</context>

<requirements>

**Execute steps in order. Run `cd watcher/github-release && make test` after step 5. Run `make precommit` only at the final step.**

0. **Promote `go-github/v84` from indirect to direct** in `watcher/github-release/go.mod`:
   ```bash
   cd watcher/github-release && go get github.com/google/go-github/v84@v84.0.0
   go mod tidy
   ```
   Confirm the line moves out of the `// indirect` block in the `require (...)` clause. If `go get` upgrades to a newer v84 patch, accept the upgrade — patch bumps are safe within a major version.

   Also ensure `gopkg.in/yaml.v3` is a direct dependency (likely already indirect via transitive deps). Run:
   ```bash
   go get gopkg.in/yaml.v3
   ```

1. **Replace `githubClient` struct in `watcher/github-release/pkg/githubclient.go`** with the real backing client:

   ```go
   import (
       "context"
       stderrors "errors"
       "net/http"

       "github.com/bborbe/errors"
       gogithub "github.com/google/go-github/v84/github"
       "gopkg.in/yaml.v3"
   )

   // ErrRateLimited is returned when the GitHub API responds with a rate-limit
   // or abuse-rate-limit error.
   var ErrRateLimited = stderrors.New("github rate limited")

   type githubClient struct {
       client *gogithub.Client
   }

   func NewGitHubClient(httpClient *http.Client) GitHubClient {
       return &githubClient{client: gogithub.NewClient(httpClient)}
   }
   ```

   Remove the existing empty `githubClient struct{}` declaration. Keep the interface definition intact.

2. **Implement `ListRepos`** — paginated, owner-kind-aware:

   ```go
   func (c *githubClient) ListRepos(ctx context.Context, owner string) ([]Repo, error) {
       user, _, err := c.client.Users.Get(ctx, owner)
       if err != nil {
           return nil, c.wrapRateLimitErr(ctx, err, "get user %s", owner)
       }
       isOrg := user.GetType() == "Organization"
       return c.listOwnerReposPaginated(ctx, owner, isOrg)
   }
   ```

   Then `listOwnerReposPaginated` MUST:
   - Use `PerPage: 100`.
   - Loop pages; check `ctx.Done()` non-blocking on each iteration via a `select { case <-ctx.Done(): return nil, ctx.Err(); default: }` (mirrors `watcher/github-build/pkg/githubclient.go listOwnerReposPaginated`).
   - For each page, dispatch through `fetchRepoPage(ctx, owner, isOrg, page)` which returns `([]*gogithub.Repository, *gogithub.Response, error)`. Inside `fetchRepoPage` branch on `isOrg` calling `client.Repositories.ListByOrg` vs `ListByUser`.
   - Filter each `*gogithub.Repository`: skip `repo.GetArchived()` AND `repo.GetFork()` AND any whose `GetName()` is empty. Build the `Repo` struct: `Repo{Owner: owner, Name: repo.GetName(), DefaultBranch: repo.GetDefaultBranch()}`.
   - On non-rate-limit error, return `nil, errors.Wrapf(ctx, err, "list repos for %s page=%d", owner, page)`.
   - Add helper `wrapRateLimitErr(ctx, err, format, args...)` identical to `watcher/github-build/pkg/githubclient.go`:
     ```go
     func (c *githubClient) wrapRateLimitErr(ctx context.Context, err error, msg string, args ...interface{}) error {
         var rl *gogithub.RateLimitError
         var arl *gogithub.AbuseRateLimitError
         if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
             return ErrRateLimited
         }
         return errors.Wrapf(ctx, err, msg, args...)
     }
     ```

3. **Implement `GetMasterSHA`** — fetches the SHA of the repo's default-branch HEAD using the `DefaultBranch` cached on the `Repo` argument:

   ```go
   func (c *githubClient) GetMasterSHA(ctx context.Context, repo Repo) (string, error) {
       branch, _, err := c.client.Repositories.GetBranch(ctx, repo.Owner, repo.Name, repo.DefaultBranch, 0)
       if err != nil {
           return "", c.wrapRateLimitErr(ctx, err, "get branch %s/%s@%s", repo.Owner, repo.Name, repo.DefaultBranch)
       }
       return branch.GetCommit().GetSHA(), nil
   }
   ```

   If `repo.DefaultBranch` is empty (cold cache miss — should not happen in production because `ListRepos` always populates it), surface a wrapped error rather than calling GitHub with an empty branch. Add an early-return:
   ```go
   if repo.DefaultBranch == "" {
       return "", errors.Errorf(ctx, "repo %s/%s has empty DefaultBranch — cannot fetch HEAD SHA", repo.Owner, repo.Name)
   }
   ```

4. **Implement `GetChangelogContent`** — 404-as-nil-nil, rate-limit-aware:

   ```go
   func (c *githubClient) GetChangelogContent(ctx context.Context, repo Repo) ([]byte, error) {
       opts := &gogithub.RepositoryContentGetOptions{Ref: repo.DefaultBranch}
       fileContent, _, _, err := c.client.Repositories.GetContents(ctx, repo.Owner, repo.Name, "CHANGELOG.md", opts)
       if err != nil {
           var ghErr *gogithub.ErrorResponse
           if stderrors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
               return nil, nil
           }
           var rl *gogithub.RateLimitError
           var arl *gogithub.AbuseRateLimitError
           if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
               return nil, ErrRateLimited
           }
           return nil, errors.Wrapf(ctx, err, "get CHANGELOG.md %s/%s@%s", repo.Owner, repo.Name, repo.DefaultBranch)
       }
       if fileContent == nil {
           return nil, nil
       }
       if fileContent.GetSize() > 1024*1024 {
           return nil, errors.Errorf(ctx, "CHANGELOG.md %s/%s too large: %d bytes (max 1 MiB)", repo.Owner, repo.Name, fileContent.GetSize())
       }
       decoded, err := fileContent.GetContent()
       if err != nil {
           return nil, errors.Wrapf(ctx, err, "decode CHANGELOG.md %s/%s", repo.Owner, repo.Name)
       }
       return []byte(decoded), nil
   }
   ```

5. **Implement `GetAutoReleaseConfig`** — same 404 handling, YAML decode:

   ```go
   func (c *githubClient) GetAutoReleaseConfig(ctx context.Context, repo Repo) (bool, error) {
       opts := &gogithub.RepositoryContentGetOptions{Ref: repo.DefaultBranch}
       fileContent, _, _, err := c.client.Repositories.GetContents(ctx, repo.Owner, repo.Name, ".dark-factory/config.yml", opts)
       if err != nil {
           var ghErr *gogithub.ErrorResponse
           if stderrors.As(err, &ghErr) && ghErr.Response.StatusCode == http.StatusNotFound {
               return false, nil
           }
           var rl *gogithub.RateLimitError
           var arl *gogithub.AbuseRateLimitError
           if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
               return false, ErrRateLimited
           }
           return false, errors.Wrapf(ctx, err, "get .dark-factory/config.yml %s/%s@%s", repo.Owner, repo.Name, repo.DefaultBranch)
       }
       if fileContent == nil {
           return false, nil
       }
       decoded, err := fileContent.GetContent()
       if err != nil {
           return false, errors.Wrapf(ctx, err, "decode .dark-factory/config.yml %s/%s", repo.Owner, repo.Name)
       }
       var cfg darkFactoryConfig
       if err := yaml.Unmarshal([]byte(decoded), &cfg); err != nil {
           return false, errors.Wrapf(ctx, err, "parse .dark-factory/config.yml %s/%s", repo.Owner, repo.Name)
       }
       return cfg.AutoRelease, nil
   }

   type darkFactoryConfig struct {
       AutoRelease bool `yaml:"autoRelease"`
   }
   ```

6. **Create `watcher/github-release/pkg/githubclient_export_test.go`** to expose package-internal hooks for tests (mirror `watcher/github-build/pkg/githubclient_export_test.go`):

   ```go
   package pkg

   import (
       "net/url"
       gogithub "github.com/google/go-github/v84/github"
   )

   // SetBaseURL replaces the underlying go-github BaseURL — test-only hook.
   func SetBaseURL(c GitHubClient, raw string) error {
       gc, ok := c.(*githubClient)
       if !ok {
           return errors.New("SetBaseURL only works on *githubClient")
       }
       u, err := url.Parse(raw)
       if err != nil {
           return err
       }
       gc.client.BaseURL = u
       _ = gogithub.NewClient // keep import live
       return nil
   }
   ```

   (Adjust the error import — use `stderrors "errors"` or simply construct via `fmt.Errorf` — note this is `*_test.go`-tier code so `fmt.Errorf` is acceptable here. If lint complains, switch to a tiny `errors.New` sentinel.)

7. **Create `watcher/github-release/pkg/githubclient_test.go`** as package `pkg_test` using `net/http/httptest`. The test file should:

   - Import `net/http`, `net/http/httptest`, `github.com/bborbe/maintainer/watcher/github-release/pkg`, the ginkgo/gomega imports.
   - In a `Describe("pkg.GitHubClient", ...)` use `BeforeEach` to spin up `httptest.NewServer(http.HandlerFunc(...))` and tear down in `AfterEach`. Construct the client via `pkg.NewGitHubClient(server.Client())` then `pkg.SetBaseURL(client, server.URL + "/")`.

   `It` blocks:

   a. **`It("ListRepos paginates and filters archived/forks")`**: handler routes:
   - `GET /users/bborbe` → `{"login":"bborbe","type":"User"}`
   - `GET /users/bborbe/repos?page=1&per_page=100` → JSON array of 3 repos: `[{"name":"docker-utils","default_branch":"master","archived":false,"fork":false}, {"name":"old-stuff","default_branch":"master","archived":true,"fork":false}, {"name":"a-fork","default_branch":"main","archived":false,"fork":true}]`. Set `Link: <.../repos?page=2>; rel="next"` header.
   - `GET /users/bborbe/repos?page=2&per_page=100` → JSON array of 1 repo: `[{"name":"disk-status","default_branch":"main","archived":false,"fork":false}]`. No `Link` header.

   Assert: `repos` has length 2 (`docker-utils` + `disk-status`); `repos[0].DefaultBranch == "master"`; `repos[1].DefaultBranch == "main"`.

   b. **`It("ListRepos returns ErrRateLimited on 403 + X-RateLimit-Remaining: 0")`**: handler returns `403` with `X-RateLimit-Remaining: 0`, `X-RateLimit-Reset: <future-unix>`, and a JSON body shaped like `{"message":"API rate limit exceeded","documentation_url":"..."}`. Assert `errors.Is(err, pkg.ErrRateLimited)` is `true`.

   c. **`It("GetMasterSHA returns the branch HEAD commit SHA")`**: handler at `GET /repos/bborbe/docker-utils/branches/master` returns `{"name":"master","commit":{"sha":"d630ef3526cfc57fbdccd9ba53c5c3a02945e407"}}`. Call `client.GetMasterSHA(ctx, pkg.Repo{Owner: "bborbe", Name: "docker-utils", DefaultBranch: "master"})`, assert `sha == "d630ef3526cfc57fbdccd9ba53c5c3a02945e407"`.

   d. **`It("GetMasterSHA returns wrapped error when DefaultBranch is empty")`**: pass `pkg.Repo{Owner: "x", Name: "y", DefaultBranch: ""}`, assert `err != nil` and that no HTTP request was made (count requests in the handler).

   e. **`It("GetChangelogContent returns nil bytes on 404")`**: handler at `GET /repos/bborbe/x/contents/CHANGELOG.md` returns `404` with `{"message":"Not Found"}`. Assert `(content, err) == (nil, nil)`.

   f. **`It("GetChangelogContent returns decoded bytes on 200")`**: handler returns `200` with JSON content payload: `{"name":"CHANGELOG.md","path":"CHANGELOG.md","size":42,"encoding":"base64","content":"<base64-of-fixture-bytes>"}`. Use a tiny fixture like `## Unreleased\n\n- new\n`. Assert `string(content) == "## Unreleased\n\n- new\n"`.

   g. **`It("GetChangelogContent rejects files larger than 1 MiB")`**: handler returns `200` with `"size": 2000000` and minimal `"content": ""`. Assert `err != nil` and `content == nil`. (Size check happens BEFORE decoding.)

   h. **`It("GetAutoReleaseConfig returns false on 404")`**: handler returns `404` at `GET /repos/bborbe/x/contents/.dark-factory/config.yml`. Assert `(false, nil)`.

   i. **`It("GetAutoReleaseConfig parses autoRelease: true")`**: handler returns base64-encoded YAML `autoRelease: true\n`. Assert `(true, nil)`.

   j. **`It("GetAutoReleaseConfig returns false when key is missing")`**: handler returns base64-encoded YAML `someOtherKey: value\n`. Assert `(false, nil)` (zero-value).

   k. **`It("GetAutoReleaseConfig surfaces YAML parse error")`**: handler returns base64-encoded INVALID YAML — use the literal string `"{invalid"` (unbalanced brace; guarantees `yaml.Unmarshal` failure). Do NOT use `:::not yaml:::` — that parses as a valid YAML scalar string and the test would observe nil err when wrapped err is expected. Assert `err != nil` and `result == false`.

   For each test that calls `httptest.NewServer`, register only the specific routes that test exercises — fail with `http.StatusInternalServerError` and a body like `"unexpected route: " + r.URL.Path` for unmatched routes so a regression surfaces clearly.

   Use `context.Background()` inside the test (test-only — safe per the spec constraint that bans `context.Background()` only in non-test code).

8. **Regenerate mocks via the existing counterfeiter directive**:
   ```bash
   cd watcher/github-release && make generate
   ls pkg/mocks/github_client.go
   ```
   The fake satisfies the four-method `GitHubClient` interface.

9. **Run unit tests**:
   ```bash
   cd watcher/github-release && make test
   ```

10. **Run full precommit**:
    ```bash
    cd watcher/github-release && make precommit
    ```

</requirements>

<constraints>
- Mirror `watcher/github-pr` Go patterns verbatim where they exist: `errors.Wrapf(ctx, err, ...)` for production wrapping, `glog.V(N).Infof` for logs, counterfeiter-generated mocks in `pkg/mocks/`, Ginkgo v2 + Gomega for tests, external `_test` packages.
- Rate-limit detection MUST use `stderrors.As(err, &rl)` against BOTH `*gogithub.RateLimitError` AND `*gogithub.AbuseRateLimitError` (abuse limit responds with 403 + custom payload — both surface as the same `ErrRateLimited` sentinel).
- 404 on `GetChangelogContent` and `GetAutoReleaseConfig` returns `(nil, nil)` / `(false, nil)` respectively — this is the documented "normal path" per the spec failure-mode table, NOT an error.
- No `context.Background()` outside `*_test.go` — verified by spec AC.
- No `fmt.Errorf` in production paths — use `errors.Wrapf` / `errors.Errorf`. Test code may use `fmt.Errorf` sparingly but prefer the package-consistent style.
- All HTTP requests in tests MUST go through `httptest.NewServer` — never let a test touch the real `api.github.com` (the precommit lint check `grep -rn "api.github.com" pkg/githubclient_test.go` should return zero matches).
- Do NOT commit — dark-factory handles git.
- Do NOT modify any file outside `pkg/githubclient.go`, `pkg/githubclient_export_test.go`, `pkg/githubclient_test.go`, and `go.mod` / `go.sum`.
- Preserve the existing godoc above each interface method — those reference patterns help future readers.
</constraints>

<verification>
```bash
cd watcher/github-release

# No TODOs remain in githubclient.go
grep -c "TODO" pkg/githubclient.go
# Expected: 0

# Sentinel error exists
grep -n "var ErrRateLimited" pkg/githubclient.go

# go-github v84 is a direct dependency
grep -n "github.com/google/go-github/v84" go.mod | grep -v indirect
# Expected: at least one non-indirect line

# Counterfeiter mock generated at the spec-defined location
ls pkg/mocks/github_client.go

# Test file uses httptest, never the real GitHub host
grep -F "httptest.NewServer" pkg/githubclient_test.go
grep -c "api.github.com" pkg/githubclient_test.go
# Expected: 0

# Stage 2 anti-pattern guard: no new capability beyond the four interface methods
grep -E "^func \(c \*githubClient\)" pkg/githubclient.go | wc -l
# Expected: 4 main methods + helpers (wrapRateLimitErr, listOwnerReposPaginated, fetchRepoPage)

# Tests pass
make test

# Full precommit
make precommit
```
</verification>
