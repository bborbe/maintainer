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
1. **Create `watcher/github-build/pkg/auth/auth_test.go`** — tests for `auth.Resolve` (introduced in sibling prompt 137 at `watcher/github-build/pkg/auth/auth.go`). Use `package auth_test` (external) — `auth.Resolve` and `auth.Config` are exported. Follow Ginkgo v2 / Gomega per project conventions; model file: `watcher/github-build/pkg/githubclient_test.go`.

   Test cases for `auth.Resolve`:

   a. **PAT fallback mode — captured Authorization header (spec AC line 102)** — `AppID=0`, `InstallationID=0`, `GHToken="my-token"`:
      ```go
      httpClient, err := auth.Resolve(ctx, auth.Config{
          GHToken:   "my-token",
          LogPrefix: "test",
      })
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

   b. **Conflict mode (both set) — full warning literal (spec AC line 100)** — `AppID=123456`, `InstallationID=789012`, `PEMKey=<dummy>`, `GHToken="some-token"`:
      - Capture glog stderr output during the call:
        ```go
        origStderr := os.Stderr
        r, w, _ := os.Pipe()
        os.Stderr = w
        flag.Set("logtostderr", "true")
        flag.Set("stderrthreshold", "WARNING")
        defer func() { os.Stderr = origStderr }()

        _, _ = auth.Resolve(ctx, auth.Config{
            AppID: 123456, InstallationID: 789012,
            PEMKey:    "not-a-real-pem",
            GHToken:   "some-token",
            LogPrefix: "test",
        }) // err expected (bad PEM); we only care about stderr

        _ = w.Close()
        out, _ := io.ReadAll(r)
        ```
      - Assert: captured `out` contains the EXACT literal `both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored`. This satisfies spec AC line 100 — the literal MUST be asserted, not skipped.
      - The App branch fails at PEM parse (no real RSA key); the warning is emitted BEFORE the parse, so the captured stderr contains it regardless of the eventual error.

   c. **Refusal mode (neither set)** — call `auth.Resolve(ctx, auth.Config{LogPrefix: "test"})`:
      - Assert: returned `httpClient` is nil.
      - Assert: returned error is non-nil.
      - Assert: error message contains both literals `APP_ID` AND `GH_TOKEN` (spec AC line 101).

   d. **Missing PEMKeyFile file** — call `auth.Resolve(ctx, auth.Config{AppID: 123456, InstallationID: 789012, PEMKeyFile: "/nonexistent/path", LogPrefix: "test"})`:
      - Assert: returned error is non-nil and contains the path or "no such file".

   **Out of scope for unit tests**: the App-mode happy-path (`NewClient` + successful IAT mint) requires either a real RSA key + httptest.Server with `BaseURL` redirection on `lib/githubapp.Config`, OR a mock — both higher-friction than the value they provide. Spec line 131 explicitly defers this to Rung 3 cluster verification ("Live cluster verification is the right granularity"). Do NOT add a happy-path App-mode unit test in this prompt.

2. **Verify the existing `pkg/githubclient_test.go` still passes after sibling prompt 137's signature change**:

   The existing `buildClient` helper in `pkg/githubclient_test.go` already calls `gogithub.NewClient(server.Client())` and then uses `pkg.NewForTest(ghc)` — that test pattern is unchanged by the `NewGitHubClient(token string)` → `NewGitHubClient(httpClient *http.Client)` refactor in sibling 137, because `NewForTest` accepts `*gogithub.Client` (verified: `pkg/githubclient_export_test.go:11-13`).

   **Do NOT add a new test that calls `pkg.NewGitHubClient(server.Client())` directly** — the resulting client's `BaseURL` defaults to `https://api.github.com` (public GitHub), so any outbound call would hit real GitHub, not the test server. The existing `NewForTest`-based pattern is the correct shape.

   Verification: `grep -nE "NewGitHubClient|NewForTest" watcher/github-build/pkg/githubclient_test.go` returns at least one `NewForTest` match; `make test` exits 0.

3. **Check test coverage for the new `pkg/auth/` package**:
   ```bash
   cd watcher/github-build && go test -coverprofile=/tmp/cover.out -mod=mod ./pkg/auth/... && go tool cover -func=/tmp/cover.out
   ```
   `auth.Resolve` MUST hit ≥80% — the four test cases (a, b, c, d) cover App-success-then-PEM-parse-fail, conflict-warning, PAT-fallback, refusal, and PEM-file-missing branches.

4. **Run `cd watcher/github-build && make test`** — all tests must pass.

5. **Run `cd watcher/github-build && make precommit`** — must exit 0.
</requirements>

<constraints>
- Tests must use Ginkgo v2 / Gomega (same framework as existing tests)
- New tests live in `watcher/github-build/pkg/auth/auth_test.go` with `package auth_test` (external). `auth.Resolve` and `auth.Config` are exported, so external-package tests are the project default.
- `context.Background()` is acceptable in unit tests for ctx parameter values; only forbidden in production code paths.
- Do NOT commit — dark-factory handles git
- No counterfeiter mocks are introduced by this prompt; the existing `pkg.Watcher` interface is unchanged.
</constraints>

<verification>
```bash
# auth_test.go exists and covers all four cases
test -f watcher/github-build/pkg/auth/auth_test.go
grep -nE "PAT fallback|Conflict mode|Refusal|PEMKeyFile" watcher/github-build/pkg/auth/auth_test.go
# Expected: ≥4 matches (one per case label)

# Conflict literal asserted verbatim
grep -nF "both App credentials and GH_TOKEN are set — App wins; GH_TOKEN ignored" watcher/github-build/pkg/auth/auth_test.go
# Expected: ≥1 match

# Coverage of pkg/auth/
cd watcher/github-build && go test -coverprofile=/tmp/cover.out -mod=mod ./pkg/auth/... && go tool cover -func=/tmp/cover.out | grep auth.Resolve
# Expected: coverage ≥80% on auth.Resolve

# Full precommit (service dir)
cd watcher/github-build && make precommit
```
</verification>
