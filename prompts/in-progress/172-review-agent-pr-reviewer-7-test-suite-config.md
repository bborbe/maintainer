---
status: approved
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T21:25:46Z"
---

<summary>
- `pkg/factory/factory_suite_test.go` is missing the `//go:generate` directive — `go generate ./...` will not regenerate mocks for the factory package
- Multiple test suites (`factory_suite_test.go`, `bitbucket/client_suite_test.go`, `git/git_suite_test.go`, `githubauth/githubauth_suite_test.go`) call `RunSpecs` without `GinkgoConfiguration()` and per-test timeout
- `githubauth/githubauth_suite_test.go` and `githubposter/githubposter_suite_test.go` use non-standard `Test*` function names instead of `TestSuite`
</summary>

<objective>
Fix test suite infrastructure: add missing `//go:generate` directive, add GinkgoConfiguration/timeout to all affected suites, and rename non-standard test functions to `TestSuite`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo suite setup, `//go:generate` convention.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/factory/factory_suite_test.go`
- `agent/pr-reviewer/pkg/bitbucket/client_suite_test.go`
- `agent/pr-reviewer/pkg/git/git_suite_test.go`
- `agent/pr-reviewer/pkg/githubauth/githubauth_suite_test.go`
- `agent/pr-reviewer/pkg/githubposter/githubposter_suite_test.go`
- `agent/pr-reviewer/pkg/githubposter/poster_test.go` — to see the correct GinkgoConfiguration pattern
</context>

<requirements>
**Execute steps in order. Run `make test` after each change. Run `make precommit` only at the final step.**

1. **Fix `pkg/factory/factory_suite_test.go`**

   a. Add `//go:generate` directive above `TestSuite`:
   ```go
   //go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate
   func TestSuite(t *testing.T) {
   ```

   b. Add GinkgoConfiguration and timeout:
   ```go
   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false

       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Factory Suite", suiteConfig, reporterConfig)
   }
   ```

2. **Fix `pkg/bitbucket/client_suite_test.go`**

   Add GinkgoConfiguration and timeout:
   ```go
   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false

       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Bitbucket Client Suite", suiteConfig, reporterConfig)
   }
   ```

3. **Fix `pkg/git/git_suite_test.go`**

   Add GinkgoConfiguration and timeout (use 120s since git operations can be slow):
   ```go
   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false

       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 120 * time.Second
       RunSpecs(t, "Git Suite", suiteConfig, reporterConfig)
   }
   ```

4. **Fix `pkg/githubauth/githubauth_suite_test.go`**

   a. Rename `func TestGitHubAuth` to `func TestSuite`:
   ```go
   func TestSuite(t *testing.T) {
   ```

   b. Add GinkgoConfiguration and timeout:
   ```go
   suiteConfig, reporterConfig := GinkgoConfiguration()
   suiteConfig.Timeout = 60 * time.Second
   RunSpecs(t, "Githubauth Suite", suiteConfig, reporterConfig)
   ```

5. **Fix `pkg/githubposter/githubposter_suite_test.go`**

   a. Rename `func TestGitHubPoster` to `func TestSuite`:
   ```go
   func TestSuite(t *testing.T) {
   ```

   b. The GinkgoConfiguration is already present in this file — just fix the function name.

6. **Run `make test`** to verify:

   ```bash
   cd agent/pr-reviewer && make test
   ```

7. **Run `go generate`** to verify the `//go:generate` directive works:

   ```bash
   cd agent/pr-reviewer && go generate ./pkg/factory/...
   ```

8. **Run `make precommit`** for final validation:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only change the 5 test suite files listed above
- Do NOT commit — dark-factory handles git
- All test suites must use `func TestSuite` (not `func TestGitHubAuth`, `func TestGitHubPoster`, etc.)
- All test suites must call `GinkgoConfiguration()` and set `suiteConfig.Timeout`
- Existing tests must still pass
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
