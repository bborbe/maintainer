---
status: approved
spec: [038-migrate-watcher-github-build-to-github-app]
created: "2026-05-23T21:30:00Z"
queued: "2026-05-23T21:24:11Z"
branch: dark-factory/migrate-watcher-github-build-to-github-app
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

Existing test files to model:
- `watcher/github-build/main_allowlist_test.go` (`package main`, internal — direct access to `application` struct; this is the pattern to follow for `resolveAuth` tests)
- `watcher/github-build/validate_test.go` (`package main`, internal)
- `watcher/github-build/pkg/githubclient_test.go` (httptest mock pattern)
</context>

<requirements>
1. **Create `watcher/github-build/main_resolveauth_test.go`** — internal-package test (`package main`, NOT `main_test`) so `application` and `resolveAuth` are directly accessible. Follow the same pattern as `main_allowlist_test.go` (which is also `package main`).

   Test cases for `resolveAuth`:

   a. **PAT fallback mode — captured Authorization header (spec AC line 102)** — `AppID=0`, `InstallationID=0`, `GHToken="my-token"`:
      ```go
      app := &application{GHToken: "my-token"}
      httpClient, err := app.resolveAuth(ctx)
      Expect(err).NotTo(HaveOccurred())
      Expect(httpClient).NotTo(BeNil())

      // Drive an outbound request through the returned client to capture the Bearer header.
      var captured string
      server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          captured = r.Header.Get("Authorization")
          w.WriteHeader(200)
      }))
      defer server.Close()

      _, err = httpClient.Get(server.URL)
      Expect(err).NotTo(HaveOccurred())
      Expect(captured).To(Equal("Bearer my-token"))
      ```
      This satisfies spec AC line 102 — the captured header equals `Bearer <GH_TOKEN>`.

   b. **Conflict mode (both set) — full warning literal** — `AppID=123456`, `InstallationID=789012`, `PEMKey=<dummy>`, `GHToken="some-token"`:
      - Note: the App branch will fail at PEM parse (no real RSA key), so this test exercises the conflict-detection path before the mint. Test approach: assert that when both are set, the code path that logs the conflict warning is taken (verify via the order of operations or by inspecting a captured glog output). If `glog` output is hard to capture in-process, accept this case as covered by Rung 3 e2e and skip the unit test; document the skip.
      - At minimum: assert that with both set AND an invalid PEM, the error returned mentions App-side failure (not "neither configured") — proves the conflict branch was taken.

   c. **Refusal mode (neither set)** — `AppID=0`, `GHToken=""`:
      - Assert: returned `httpClient` is nil.
      - Assert: returned error is non-nil.
      - Assert: error message contains both literals `APP_ID` AND `GH_TOKEN` (spec AC).

   d. **Missing PEMKeyFile file** — `AppID=123456`, `InstallationID=789012`, `PEMKeyFile="/nonexistent/path"`, `PEMKey=""`:
      - Assert: returned error is non-nil and contains the path or "no such file".

   **Out of scope for unit tests**: the App-mode happy-path (`NewClient` + successful IAT mint) requires either a real RSA key + httptest.Server with `BaseURL` redirection on `lib/githubapp.Config`, OR a mock — both higher-friction than the value they provide. Spec line 131 explicitly defers this to Rung 3 cluster verification ("Live cluster verification is the right granularity"). Do NOT add a happy-path App-mode unit test in this prompt.

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
- New `resolveAuth` tests live in `watcher/github-build/main_resolveauth_test.go` with `package main` (NOT `main_test`) — `application` and `resolveAuth` are unexported, so internal-package access is required. Model: `main_allowlist_test.go`.
- `context.Background()` is acceptable in unit tests for ctx parameter values; only forbidden in production code paths.
- Do NOT commit — dark-factory handles git
- No counterfeiter mocks are introduced by this prompt; the existing `pkg.Watcher` interface is unchanged.
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