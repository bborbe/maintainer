---
status: completed
summary: Added three new Ginkgo test cases to watcher/github/pkg/watcher_test.go covering publishForcePush Kafka failure, fetchHeadSHA error path, and fetchHeadSHA cache hit with duplicate task ID; all target functions now at 100% coverage and make precommit exits 0.
container: code-reviewer-062-review-watcher-github-add-missing-test-coverage
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:36:26Z"
started: "2026-04-28T18:26:26Z"
completed: "2026-04-28T18:29:54Z"
---

<summary>
- Several error paths in the core polling loop have no test coverage
- publishForcePush Kafka error path is untested — cursor SHA not updated on failure
- fetchHeadSHA returning an error causes the PR to be skipped — untested
- fetchHeadSHA cache hit (two PRs with the same task ID) is untested
- LoadCursor OS read error (file exists but unreadable) propagates to Poll — untested
- All these are correctness-critical paths that affect cursor state and publish behavior
- Tests must use the existing Counterfeiter mocks (mocks.GitHubClient, mocks.CommandPublisher)
</summary>

<objective>
Add tests for the four uncovered error and logic paths in `watcher/github/pkg/watcher_test.go`: the `publishForcePush` Kafka failure, the `fetchHeadSHA` error path, the `fetchHeadSHA` cache hit, and the `LoadCursor` non-IsNotExist error. Each test verifies the correct cursor state and publish call count after the failure.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Mock types in `pkg/mocks/`: `GitHubClient` (in `github_client.go`) and `CommandPublisher` (in `command_publisher.go`) — note these are the actual exported names, NOT prefixed with `Fake`.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/watcher_test.go` (full): all existing test cases, the `BeforeEach` setup, and how `mocks.GitHubClient` and `mocks.CommandPublisher` are configured
- `watcher/github/pkg/watcher.go` (~lines 178-240): `publishCreate`, `publishForcePush`, `fetchHeadSHA` implementations
- `watcher/github/pkg/mocks/github_client.go`: available stub methods on `mocks.GitHubClient`
- `watcher/github/pkg/mocks/command_publisher.go`: available stub methods on `mocks.CommandPublisher`
- `watcher/github/pkg/cursor.go` (~lines 28-47): `LoadCursor` error branches
</context>

<requirements>
1. **Add test: `publishForcePush` Kafka publish error** in `watcher/github/pkg/watcher_test.go`:

   Setup:
   - First poll: new PR, `PublishCreate` succeeds → SHA recorded in cursor
   - Second poll: same PR, different SHA returned by `GetHeadSHA` → `PublishUpdateFrontmatter` returns an error

   Assertions:
   - `Poll` returns nil (non-crash)
   - After the second poll, cursor SHA is NOT updated to the new SHA (the old SHA is preserved)
   - A third poll still calls `PublishUpdateFrontmatter` again (retry on next cycle)

2. **Add test: `fetchHeadSHA` returns an error** in `watcher/github/pkg/watcher_test.go`:

   Setup:
   - Search returns one PR
   - `GetHeadSHA` returns an error for that PR

   Assertions:
   - `Poll` returns nil (non-crash)
   - `PublishCreate` is never called
   - Cursor `LastUpdatedAt` is not advanced past `startTime`

3. **Add test: `fetchHeadSHA` cache hit** in `watcher/github/pkg/watcher_test.go`:

   Setup:
   - Search returns two PRs with the same `Owner`, `Repo`, `Number` (same task ID)
   - `GetHeadSHA` is configured to return a valid SHA

   Assertions:
   - `GetHeadSHA` is called exactly once (cache hit on second PR)
   - `PublishCreate` is called exactly once (same task ID deduplicated)

4. **Add test: `LoadCursor` non-IsNotExist error propagates** in `watcher/github/pkg/watcher_test.go`:

   Setup:
   - Write a cursor file to `cursorPath` with content `{}`
   - `os.Chmod(cursorPath, 0000)` to make it unreadable
   - Defer `os.Chmod(cursorPath, 0600)` to restore
   - Skip test if `os.Getuid() == 0` (root bypasses permissions)

   Assertions:
   - `Poll` returns a non-nil error
   - `PublishCreate` is never called

5. Run `cd watcher/github && go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep watcher` to verify coverage improved.

6. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Only add new test cases — do NOT modify existing tests
- Use external test package `pkg_test` (consistent with the existing file)
- Use Ginkgo/Gomega patterns — no `t.Error`, no table-driven tests
- Use existing `mocks.GitHubClient` and `mocks.CommandPublisher` mocks — do NOT create manual mocks
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd watcher/github && go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep -E "publishForcePush|fetchHeadSHA|LoadCursor"
# Expected: coverage on these functions improved

cd watcher/github && make precommit
</verification>
