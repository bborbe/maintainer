---
status: completed
summary: Updated five Ginkgo suite files in agent/pr-reviewer to use four-argument RunSpecs with GinkgoConfiguration() and 60-second timeout, matching the established pattern in main_test.go.
container: code-reviewer-041-review-agent-pr-reviewer-1-ginkgo-suite-setup
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T12:00:00Z"
queued: "2026-04-28T14:32:41Z"
started: "2026-04-28T14:34:20Z"
completed: "2026-04-28T14:37:38Z"
---

<summary>
- Five test suite files use the two-argument `RunSpecs(t, "name")` form, which silently ignores Ginkgo configuration flags including timeout
- Without passing `GinkgoConfiguration()` to `RunSpecs`, tests can hang indefinitely when a subprocess or network call stalls
- The project already has the correct four-argument pattern in `main_test.go` and `cmd/run-task/main_test.go` — this prompt brings the five package suite files into alignment
- Each file needs `GinkgoConfiguration()`, a 60-second suite timeout, and the four-argument `RunSpecs` call
- All five files already have the `//go:generate` directive for counterfeiter; do not duplicate it
</summary>

<objective>
Update five Ginkgo suite files to use `GinkgoConfiguration()` with a 60-second timeout and the four-argument `RunSpecs` form, matching the pattern already established in `agent/pr-reviewer/main_test.go` and `agent/pr-reviewer/cmd/run-task/main_test.go`. After this fix all test suites respect Ginkgo's timeout and reporter configuration flags.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/main_test.go` — the reference/correct pattern for suite setup (4-arg RunSpecs, GinkgoConfiguration, suiteConfig.Timeout)
- `agent/pr-reviewer/pkg/config/config_suite_test.go` — needs update
- `agent/pr-reviewer/pkg/verdict/verdict_suite_test.go` — needs update
- `agent/pr-reviewer/pkg/prurl/prurl_suite_test.go` — needs update
- `agent/pr-reviewer/pkg/github/github_suite_test.go` — needs update
- `agent/pr-reviewer/pkg/review/review_suite_test.go` — needs update
</context>

<requirements>
For each of the five suite files listed in the context, apply this transformation:

**Before (current 2-arg form):**
```go
func TestSuite(t *testing.T) {
    time.Local = time.UTC
    format.TruncatedDiff = false
    RegisterFailHandler(Fail)
    RunSpecs(t, "<Suite Name>")
}
```

**After (correct 4-arg form):**
```go
//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate
func TestSuite(t *testing.T) {
    time.Local = time.UTC
    format.TruncatedDiff = false
    RegisterFailHandler(Fail)
    suiteConfig, reporterConfig := GinkgoConfiguration()
    suiteConfig.Timeout = 60 * time.Second
    RunSpecs(t, "<Suite Name>", suiteConfig, reporterConfig)
}
```

All five files already have the `//go:generate` directive — do NOT add a duplicate.

1. **`pkg/config/config_suite_test.go`** — Apply the transformation. Keep the existing suite name string unchanged.

2. **`pkg/verdict/verdict_suite_test.go`** — Apply the transformation. Keep the existing suite name string unchanged.

3. **`pkg/prurl/prurl_suite_test.go`** — Apply the transformation. Keep the existing suite name string unchanged.

4. **`pkg/github/github_suite_test.go`** — Apply the transformation. Keep the existing suite name string unchanged.

5. **`pkg/review/review_suite_test.go`** — Apply the transformation. Keep the existing suite name string unchanged.

6. **Verify imports** in each file: `"time"` must be imported (for `60 * time.Second`). The `GinkgoConfiguration` function is from `github.com/onsi/ginkgo/v2` — it is already imported via the dot import (`. "github.com/onsi/ginkgo/v2"`). No new import needed for `GinkgoConfiguration` itself.

7. **Run `cd agent/pr-reviewer && make test`** — must pass.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass — this is a non-functional change
- Do NOT alter the suite name strings or any other existing content
- The `//go:generate` directive must be placed on the line immediately above `func TestSuite`
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
