---
spec: [038]
status: draft
created: "2026-05-23T21:30:00Z"
---

<summary>
- Ginkgo unit tests cover the hybrid auth resolver in `main.go`: App-mode with valid config, PAT-fallback mode, conflict warning, and refusal when neither is set.
- Tests for `NewGitHubClient(httpClient *http.Client)` constructor using a test server and verifying authenticated requests.
- Ginkgo unit tests for `pkg/factory.CreateWatcher` with an `*http.Client` parameter.
- No new scenario test — Rung 3 dev cluster deploy is the end-to-end verification.
</summary>

<objective>
Add Ginkgo v2 unit tests for the auth resolver logic and the updated constructor signatures. Tests must cover all four resolver paths and the new `*http.Client`-based client constructor.
</objective>

<context>
Read before writing tests:

- `watcher/github-build/main.go` — the `application` struct and `resolveAuth` method you implemented in prompt 1
- `watcher/github-build/pkg/githubclient.go` — the updated `NewGitHubClient(httpClient *http.Client)` constructor
- `watcher/github-build/pkg/factory/factory.go` — the updated `CreateWatcher` signature
- `watcher/github-build/pkg/githubclient_test.go` — existing test patterns (httptest, gomega assertions)
- `watcher/github-build/main_test.go` — existing main package test file

The existing test pattern uses `httptest.NewServer` for GitHub API mocking. Follow the same pattern for auth resolver tests.

Read coding docs:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-patterns.md` (counterfeiter annotations)
</context>

<requirements>
1. **Add auth resolver tests to `watcher/github-build/main_test.go`**:

   Add a new `Describe("resolveAuth", ...)` block in the existing `Describe("Main", ...)` in `main_test.go`.

   Create a test helper struct for the test cases:
   ```go
   func buildApp() *application {
       return &application{
           AppID:          123456,
           InstallationID: 789012,
           PEMKeyFile:     "", // will be set per-test
           PEMKey:         "", // will be set per-test
           GHToken:        "",
       }
   }
   ```

   Test cases for `resolveAuth`:

   a. **App mode with PEM content** — set `PEMKey` with valid-looking PEM bytes (any bytes, since the lib parses and mints; in test, use a mock or httptest server to avoid real GitHub calls):
      - For unit tests, set `PEMKey` to a valid PEM header string so `githubapp.NewClient` succeeds up to the IAT mint step.
      - Use an `httptest.Server` as a mock GitHub API that returns 200 on app token exchange.
      - Assert: returned `*http.Client` is non-nil.
      - Assert: the auth mode log line `auth mode=github-app app_id=123456 installation_id=789012` appears (check via `glog` output capture or just check client is non-nil).

   b. **PAT fallback mode** — `AppID=0`, `InstallationID=0`, `GHToken="my-token"`:
      - Assert: returned `*http.Client` is non-nil.
      - Assert: log line `auth mode=pat-fallback` appears.

   c. **Conflict mode (both set)** — `AppID=123456`, `InstallationID=789012`, `PEMKey=validPEM`, `GHToken="some-token"`:
      - Assert: returned `*http.Client` is non-nil (App wins).
      - Assert: warning log line contains `both App credentials and GH_TOKEN are set — App wins`.

   d. **Refusal mode (neither set)** — `AppID=0`, `GHToken=""`:
      - Assert: returned error is non-nil.
      - Assert: error message contains `APP_ID` (or `neither App nor PAT`).

   e. **Missing PEM (App set but no PEMKey/PEMKeyFile)** — `AppID=123456`, `InstallationID=789012`, `PEMKey=""`, `PEMKeyFile=""`:
      - Assert: returned error is non-nil.

   f. **Missing PEMKeyFile file** — `AppID=123456`, `InstallationID=789012`, `PEMKeyFile="/nonexistent/path"`, `PEMKey=""`:
      - Assert: returned error is non-nil and contains the path.

2. **Add `NewGitHubClient` constructor tests to `watcher/github-build/pkg/githubclient_test.go`**:

   The existing `buildClient` helper uses `gogithub.NewClient(server.Client())`. Update the test to verify that the constructor properly uses an `*http.Client`:

   a. Add a test that constructs a `pkg.GitHubClient` with the server's client and verifies an authenticated request is made:
      ```go
      It("uses the provided http.Client for requests", func() {
          var receivedToken string
          server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
              receivedToken = r.Header.Get("Authorization")
              w.Header().Set("Content-Type", "application/json")
              fmt.Fprintf(w, `{"total_count": 0, "workflow_runs": []}`)
          }))
          defer server.Close()

          ghc := gogithub.NewClient(server.Client())
          baseURL, _ := url.Parse(server.URL + "/")
          ghc.BaseURL = baseURL

          client := pkg.NewGitHubClient(server.Client())
          _, _ = client.GetWorkflowRuns(ctx, "owner", "repo", "main")

          Expect(receivedToken).NotTo(BeEmpty())
      })
      ```

   Note: The existing tests already use `pkg.NewForTest` which wraps the github client. If `NewForTest` is updated in prompt 1 to take an `*http.Client`, verify the existing tests still work with the new signature.

3. **Regenerate mocks if needed**:
   ```bash
   cd watcher/github-build && go generate ./...
   ```
   Check if `factory.CreateWatcher` signature change requires updating any mock calls. If `pkg.Watcher` interface is unchanged (it is), mocks should not need regeneration.

4. **Check test coverage for auth-related code**:
   ```bash
   cd watcher/github-build && go test -coverprofile=/tmp/cover.out -mod=mod ./... && go tool cover -func=/tmp/cover.out
   ```
   The `resolveAuth` function and the `NewGitHubClient` constructor should both show coverage.

   The new auth resolver code should be covered. Add additional test cases if coverage is below 80% for the affected code paths.

5. **Run `cd watcher/github-build && make test`** — all tests must pass.

6. **Run `cd watcher/github-build && make precommit`** — must exit 0.
</requirements>

<constraints>
- Tests must use Ginkgo v2 / Gomega (same framework as existing tests)
- Counterfeiter-generated mocks only — never hand-write mocks
- External test packages (`_test` suffix) — same as existing tests
- `context.Background()` forbidden in test setup that calls production code paths
- `go generate` must be run after any counterfeiter annotation changes
- Do NOT commit — dark-factory handles git
- Coverage must be ≥80% for new code paths
</constraints>

<verification>
```bash
# New test cases exist
grep -n "resolveAuth\|auth mode=\|PAT fallback\|Conflict mode" watcher/github-build/main_test.go

# NewGitHubClient test
grep -n "NewGitHubClient\|provided.*http.Client" watcher/github-build/pkg/githubclient_test.go

# Tests pass
cd watcher/github-build && make test

# Coverage check
cd watcher/github-build && go test -coverprofile=/tmp/cover.out -mod=mod ./... && go tool cover -func=/tmp/cover.out

# Full precommit
cd watcher/github-build && make precommit
```
</verification>