---
status: completed
summary: Added tests for runPollLoop error handling path and countWildcards utility function
container: maintainer-exec-178-review-watcher-github-build-tests
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:25:46Z"
started: "2026-05-25T22:14:36Z"
completed: "2026-05-25T22:18:56Z"
---

<summary>
- Add missing tests for the `runPollLoop` error handling path
- The current test suite does not verify that poll errors are logged and the loop continues
- Also add a test for the `countWildcards` utility function
</summary>

<objective>
Add tests for `runPollLoop` error path and `countWildcards` in `main_test.go`. The error path (when `poll()` returns an error) is the only untested branch in `runPollLoop` — verify it logs the error and continues the loop rather than returning/exiting.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/main_test.go` (existing tests if any)
- `watcher/github-build/main.go` lines 180-202 (`runPollLoop` function)
- `watcher/github-build/main.go` lines 43-52 (`countWildcards` function)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
</context>

<requirements>
1. Create or extend `main_test.go` in `watcher/github-build/`. This file should be in `package main_test` (external test package).

2. For `runPollLoop` error path test:
   - Use a `run.Func` that returns a non-nil error
   - Capture log output via stderr redirect or use a logger interface seam if available — direct log-capture is OPTIONAL (no `glog.InMemoryLogger` exists in this codebase)
   - Behavioral assertion sufficient: verify the error path executes (loop continues, no panic, no early return)
   - Verify the loop continues (ticker fires again after a short wait)
   - Use a small poll interval (e.g., 10ms) and `Eventually` to avoid sleeps

3. For `countWildcards` tests (table-driven):
   - Test empty slice → 0
   - Test no wildcards → 0
   - Test one wildcard (`github.com/bborbe/*`) → 1
   - Test multiple wildcards → count
   - Test malformed entry (4 segments) → 0
   - Test wildcard not at position 3 → 0

4. Follow existing Ginkgo/Gomega v2 patterns used in the project. Use `gomega.Eventually` instead of `time.Sleep` for timing-sensitive assertions.

5. Run `cd watcher/github-build && go test ./...` to confirm tests pass.
</requirements>

<constraints>
- Only change files in `watcher/github-build/`
- Do NOT commit — dark-factory handles git
- Tests must pass
- Use Ginkgo/Gomega v2 conventions
- Use Counterfeiter mocks from `mocks/` dir — no manual mocks
</constraints>

<verification>
cd watcher/github-build && go test ./... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out | grep -E "main|runPollLoop|countWildcards"
</verification>
