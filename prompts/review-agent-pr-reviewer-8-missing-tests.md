---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `config.go`: `ExpandHome` and `normalizeURL` are untested — failures produce confusing "config not found" errors
- `githubposter/retry.go`: `classifyError` has untested branches (HTTP 429, network timeout, ErrorClassNotAFailure)
- `poster.go`: `eventToState` mapping and `truncateBody` have no direct unit tests
- `steps_planning.go`: `writePlanningVerdict`, `isGitHubPRURL`, `hasAnyPRURL` lack direct tests
- `steps_review.go`: `appendVerifyDiagnostic` format not tested in isolation
</summary>

<objective>
Add focused unit tests for the untested functions identified by the coverage review, following Ginkgo/Gomega patterns.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, DescribeTable, coverage ≥80%.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/config_test.go` — existing test patterns
- `agent/pr-reviewer/pkg/config.go` — `ExpandHome` (~line 168), `normalizeURL` (~line 210)
- `agent/pr-reviewer/pkg/githubposter/retry.go` — `classifyError` (~line 41)
- `agent/pr-reviewer/pkg/githubposter/poster.go` — `eventToState` (~line 440), `truncateBody` (~line 381)
- `agent/pr-reviewer/pkg/steps_planning.go` — `writePlanningVerdict` (~line 205), `isGitHubPRURL` (~line 214), `hasAnyPRURL` (~line 224)
- `agent/pr-reviewer/pkg/steps_review.go` — `appendVerifyDiagnostic` (~line 226)
- `agent/pr-reviewer/pkg/steps_planning_test.go` — existing test patterns
- `agent/pr-reviewer/pkg/steps_review_test.go` — existing test patterns
</context>

<requirements>
**Execute steps in order. Run `make test` after adding each test block. Run `make precommit` only at the final step.**

1. **Add `ExpandHome` and `normalizeURL` tests in `agent/pr-reviewer/pkg/config_test.go`**

   Append to `config_test.go`:
   ```go
   var _ = Describe("ExpandHome", func() {
       It("expands ~ to home directory", func() {
           home := os.Getenv("HOME")
           result := ExpandHome("~/some/path")
           Expect(result).To(Equal(filepath.Join(home, "some/path")))
       })

       It("returns input unchanged when no ~ prefix", func() {
           Expect(ExpandHome("/absolute/path")).To(Equal("/absolute/path"))
       })

       It("handles empty string", func() {
           Expect(ExpandHome("")).To(Equal(""))
       })
   })

   var _ = Describe("normalizeURL", func() {
       DescribeTable("normalizes URLs",
           func(input, want string) {
               Expect(normalizeURL(input)).To(Equal(want))
           },
           Entry("strips trailing slash", "https://github.com/", "https://github.com"),
           Entry("lowercases host", "https://GITHUB.COM/owner/repo", "https://github.com/owner/repo"),
           Entry("returns input unchanged", "https://github.com/owner/repo", "https://github.com/owner/repo"),
       )
   })
   ```

2. **Add `classifyError` tests in `agent/pr-reviewer/pkg/githubposter/retry_test.go`** (create if not exists)

   Create `agent/pr-reviewer/pkg/githubposter/retry_test.go`:
   ```go
   package githubposter_test

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   var _ = Describe("classifyError", func() {
       // Exported for testing via export_test.go
       DescribeTable("classifies errors",
           func(httpStatus int, err error, want githubposter.ErrorClass) {
               // Call the exported classifyError function via the package's exported interface
               // Adapt to however classifyError is accessible (may need to export it in export_test.go)
           },
           Entry("HTTP 200 → NotAFailure", 200, nil, githubposter.ErrorClassNotAFailure),
           Entry("HTTP 429 → Transient", 429, nil, githubposter.ErrorClassTransient),
           Entry("HTTP 500 → Transient", 500, nil, githubposter.ErrorClassTransient),
           Entry("HTTP 401 → NotTransient", 401, nil, githubposter.ErrorClassNotTransient),
           Entry("network timeout (status 0) → Transient", 0, &net.DNSError{Timeout: true}, githubposter.ErrorClassTransient),
           Entry("generic error with status → Unknown", 503, someError, githubposter.ErrorClassUnknown),
       )
   })
   ```

   Note: If `classifyError` is unexported, add an exported wrapper or expose it via `export_test.go` as done with `retryBaseDelay` and `retryJitterMs`.

3. **Add `eventToState` tests in `agent/pr-reviewer/pkg/githubposter/poster_test.go`**

   Append to `poster_test.go`:
   ```go
   var _ = Describe("eventToState", func() {
       It("maps APPROVE to APPROVED", func() {
           Expect(githubposter.EventToState("APPROVE")).To(Equal("APPROVED"))
       })
       It("maps REQUEST_CHANGES to CHANGES_REQUESTED", func() {
           Expect(githubposter.EventToState("REQUEST_CHANGES")).To(Equal("CHANGES_REQUESTED"))
       })
       It("maps COMMENT to COMMENTED", func() {
           Expect(githubposter.EventToState("COMMENT")).To(Equal("COMMENTED"))
       })
   })
   ```

4. **Add `truncateBody` test in `agent/pr-reviewer/pkg/githubposter/poster_test.go`**

   Append:
   ```go
   var _ = Describe("truncateBody", func() {
       It("truncates to 500 bytes", func() {
           long := strings.Repeat("x", 1000)
           result := githubposter.TruncateBody([]byte(long))
           Expect(len(result)).To(BeNumerically("<=", 500))
       })
       It("returns input unchanged when under limit", func() {
           short := "hello world"
           Expect(githubposter.TruncateBody([]byte(short))).To(Equal([]byte(short)))
       })
   })
   ```

5. **Add `writePlanningVerdict`, `isGitHubPRURL`, `hasAnyPRURL` tests in `agent/pr-reviewer/pkg/steps_planning_test.go`**

   Append to `steps_planning_test.go`:
   ```go
   var _ = Describe("isGitHubPRURL", func() {
       It("returns true for github.com URLs", func() {
           // Use the actual function if exported, or test via the full Run flow
       })
   })
   ```

   If these are unexported, test them via the existing integration test patterns rather than creating new test helpers.

6. **Run `make test`** to verify all new tests pass:

   ```bash
   cd agent/pr-reviewer && make test
   ```

7. **Run `make precommit`** for final validation:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only change/add test files in `agent/pr-reviewer/pkg/`
- Do NOT commit — dark-factory handles git
- Use Ginkgo/Gomega conventions — `Describe`, `DescribeTable`, `Entry`, `It`
- Use Counterfeiter mocks for any new interface mocks needed
- Coverage ≥80% for changed packages
- Existing tests must still pass
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
