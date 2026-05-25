---
status: committing
summary: Added integration tests for cmd/run-once/main.go covering error paths and success path using injectable WatcherFactory
container: maintainer-exec-177-review-watcher-github-build-run-once-tests
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:25:46Z"
started: "2026-05-25T22:05:49Z"
completed: "2026-05-25T22:14:12Z"
---

<summary>
- Add integration tests for `cmd/run-once/main.go` entry point
- The entire `cmd/run-once` package has 0% test coverage
- Test env var validation, error paths, and the one-shot poll path
</summary>

<objective>
Add tests for `cmd/run-once/main.go`. The `Run` function reads env vars, constructs a watcher, and calls `Poll`. Test the error paths (missing env vars, invalid config) and the successful path with a mock watcher.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/cmd/run-once/main.go` (full file)
- `watcher/github-build/pkg/factory/factory.go` (`CreateWatcher` signature)
- `watcher/github-build/pkg/watcher.go` (`Watcher` interface)
- `watcher/github-build/pkg/mocks/watcher.go` (Counterfeiter mock)
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
</context>

<requirements>
1. Create `cmd/run-once/main_test.go` in `package main_test` (external test package).

2. Test cases to cover:
   a) `Run` returns error when `KAFKA_BROKERS` is missing
   b) `Run` returns error when `REPO_ALLOWLIST` is empty/missing
   c) `Run` returns error when `Poll` on the watcher fails
   d) `Run` succeeds when all required env vars are set and `Poll` succeeds

3. Use the `Watcher` mock from `pkg/mocks/watcher.go` to control the `Poll` return value.

4. Use `os.Setenv`/`os.Unsetenv` to control env vars in each test case. Restore original values in `AfterEach` or `DeferCleanup`.

5. For the success path, set env vars: `KAFKA_BROKERS`, `STAGE`, `REPO_ALLOWLIST`, `TASK_ASSIGNEE`, `TASK_STATUS`.

6. Follow Ginkgo/Gomega v2 patterns. Use `gomega.Expect` for assertions.

7. Run `cd watcher/github-build && go test ./cmd/run-once/... -v` to confirm tests pass.
</requirements>

<constraints>
- Only change files in `watcher/github-build/cmd/run-once/`
- Do NOT commit — dark-factory handles git
- Tests must pass
- Use Ginkgo/Gomega v2 conventions
- Use Counterfeiter mocks from `mocks/` dir — no manual mocks
</constraints>

<verification>
cd watcher/github-build && go test ./cmd/run-once/... -coverprofile=/tmp/cover.out && go tool cover -func=/tmp/cover.out
</verification>
