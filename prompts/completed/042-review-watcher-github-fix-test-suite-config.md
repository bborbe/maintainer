---
status: completed
summary: Fixed pkg/suite_test.go to use GinkgoConfiguration() with 60s timeout and replaced all 13 time.Now() calls in watcher_test.go and githubclient_test.go with a fixed date var.
container: code-reviewer-042-review-watcher-github-fix-test-suite-config
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T14:32:41Z"
started: "2026-04-28T14:37:39Z"
completed: "2026-04-28T14:40:23Z"
---

<summary>
- pkg/suite_test.go calls RunSpecs without passing suiteConfig or reporterConfig
- This means the pkg test suite has no timeout cap and ignores reporter settings
- The main_test.go in the same repo correctly uses GinkgoConfiguration() + suiteConfig.Timeout = 60s
- Thirteen test fixtures use time.Now() making tests non-deterministic
- Fixed dates should be used instead of time.Now() in all PR and test fixtures
- Affected files: pkg/suite_test.go, pkg/watcher_test.go, pkg/githubclient_test.go
</summary>

<objective>
Fix `pkg/suite_test.go` to use `GinkgoConfiguration()` with a 60-second timeout and pass `suiteConfig`/`reporterConfig` to `RunSpecs`, matching the pattern in `main_test.go`. Replace all `time.Now()` calls in test fixtures with fixed `time.Date(...)` values for determinism.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/suite_test.go` (full): current `RunSpecs` call without config
- `watcher/github/main_test.go` (full): the correct `GinkgoConfiguration()` + `suiteConfig.Timeout` pattern to replicate
- `watcher/github/pkg/watcher_test.go`: 4 `time.Now()` usages at lines 198, 221, 343, 368 (all `UpdatedAt: time.Now()`)
- `watcher/github/pkg/githubclient_test.go`: 9 `time.Now()` usages at lines 47, 85, 117, 162, 171, 183, 195, 217, 237
</context>

<requirements>
1. **Fix `watcher/github/pkg/suite_test.go`** to match `main_test.go`'s suite pattern:

   Change:
   ```go
   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       RunSpecs(t, "Pkg Suite")
   }
   ```
   To:
   ```go
   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Pkg Suite", suiteConfig, reporterConfig)
   }
   ```

2. **Fix `watcher/github/pkg/watcher_test.go`** — replace all 4 `time.Now()` calls (lines 198, 221, 343, 368) in PR fixtures with a fixed date.

   Define a package-level var near the top of the test file:
   ```go
   var fixedNow = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
   ```

   Replace all 4 occurrences:
   - `UpdatedAt: time.Now()` → `UpdatedAt: fixedNow`

3. **Fix `watcher/github/pkg/githubclient_test.go`** — replace all 9 `time.Now()` calls (lines 47, 85, 117, 162, 171, 183, 195, 217, 237) with fixed values.

   Define a package-level var:
   ```go
   var fixedNow = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
   ```

   Substitute `time.Now()` with `fixedNow` verbatim — chained `.Add(...)` expressions remain unchanged. Examples:
   - `time.Now().Add(time.Hour).Unix()` → `fixedNow.Add(time.Hour).Unix()`
   - `time.Now().Add(-24*time.Hour)` → `fixedNow.Add(-24*time.Hour)`
   - `time.Now().Add(30 * time.Minute).Truncate(time.Second)` → `fixedNow.Add(30 * time.Minute).Truncate(time.Second)`

4. Run `cd watcher/github && make test` — must pass.

5. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass with the same assertions — only the time values change
- Do NOT change test logic, only substitute `time.Now()` with fixed values
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd watcher/github && grep -n "time\.Now()" pkg/suite_test.go pkg/watcher_test.go pkg/githubclient_test.go
# Expected: no matches

cd watcher/github && grep -n "GinkgoConfiguration\|suiteConfig" pkg/suite_test.go
# Expected: two matches (call + usage)

cd watcher/github && make precommit
</verification>
