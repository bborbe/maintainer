---
status: completed
spec: [040-github-build-watcher-task-suffix]
summary: Created filename_internal_test.go with DescribeTable tests for computeBuildTitle taskSuffix parameter covering empty/dev/prod suffix cases and truncation preserving suffix
container: maintainer-exec-150-spec-040-task-suffix-tests
dark-factory-version: v0.169.0
created: "2026-05-24T19:00:00Z"
queued: "2026-05-24T19:05:04Z"
started: "2026-05-24T19:09:11Z"
completed: "2026-05-24T19:12:13Z"
branch: dark-factory/github-build-watcher-task-suffix
---

<summary>
- Create new file `watcher/github-build/pkg/filename_internal_test.go` mirroring `watcher/github-pr/pkg/filename_internal_test.go`
- Add Ginkgo `DescribeTable` tests for `computeBuildTitle` with the new `taskSuffix` parameter
- Test cases: empty suffix (legacy shape, no trailing ` - ` artifact), non-empty suffix `"dev"` and `"prod"` (ends with ` - dev` / ` - prod`), truncation case (title length cap preserves suffix)
</summary>

<objective>
Add unit tests for the new `taskSuffix` parameter of `computeBuildTitle`. Tests must cover the filename boundary with the new parameter: empty suffix, non-empty suffix, and truncation-preserving-suffix.

Expected signature after prompt 1 lands: `computeBuildTitle(provider, owner, repo, episodeSHA string, maxTitle int, taskSuffix string) string` — call sites in tests must pass the suffix as the 6th argument.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Read these files before making changes:
- `watcher/github-build/pkg/filename.go` — the `computeBuildTitle` function (updated by prompt 1 with `taskSuffix` parameter as the 6th arg)
- `watcher/github-build/pkg/watcher_internal_test.go` — existing Ginkgo test patterns for `computeBuildTitle` (existing cases live here without a suffix; you will update each call site to pass `""` as the new 6th arg) and `slugifySegment`
- `watcher/github-pr/pkg/filename_internal_test.go` — full reference for the new file layout AND for the suffix test pattern. Locate the suffix-related `DescribeTable` or `Describe`/`It` block by content (search for `taskSuffix` / `TaskSuffix`); do not rely on line numbers as they may drift.

**File layout asymmetry to be aware of:** github-pr already has `filename_internal_test.go` as a dedicated file. github-build currently keeps `computeBuildTitle` tests inside `watcher_internal_test.go` (no dedicated `filename_internal_test.go` file exists yet). This prompt creates the dedicated file to mirror the github-pr layout — you are establishing the new convention, not just adding cases.
</context>

<requirements>
1. Create a new file `watcher/github-build/pkg/filename_internal_test.go` mirroring the layout of `watcher/github-pr/pkg/filename_internal_test.go` (same package declaration `package pkg`, same Ginkgo v2 / Gomega imports and style). The new file is the canonical home for `computeBuildTitle` tests going forward.

2. In the new file, add a `DescribeTable` block for `computeBuildTitle` covering these cases (all in one table):
   - `taskSuffix=""` produces the legacy filename — string does NOT end with ` - ` and contains no trailing suffix segment
   - `taskSuffix="dev"` produces a string ending with ` - dev`
   - `taskSuffix="prod"` produces a string ending with ` - prod`
   - With `taskSuffix="dev"` and a contrived input that would exceed `maxTitle`, the suffix ` - dev` is preserved at the end and the base title is truncated to make room (mirror the PR watcher's equivalent case)

3. Existing `computeBuildTitle` test cases in `watcher/github-build/pkg/watcher_internal_test.go` need to compile against the new 6-arg signature. Update them by passing `""` as the new 6th argument to each call. Do not duplicate them in the new file; the empty-suffix case in the new file is sufficient for the new behavior.

4. Run `cd watcher/github-build && go test ./pkg/... -v -run "computeBuildTitle"` and confirm all `computeBuildTitle` tests pass (old and new).

5. Run `cd watcher/github-build && go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out` and confirm `computeBuildTitle` shows 100% coverage (or the closest practical number; package-wide ≥ 80%).
</requirements>

<constraints>
- Follow the existing Ginkgo v2 / Gomega pattern in `watcher/github-build/pkg/watcher_internal_test.go`
- Test file must be named `filename_internal_test.go` (not `filename_test.go`) to match existing convention
- Do NOT commit — dark-factory handles git.
- Existing tests must continue to pass.
</constraints>

<verification>
- `cd watcher/github-build && go test ./pkg/... -v -run "computeBuildTitle"` shows tests for empty and non-empty suffix passing
- Test coverage for `pkg/` is >= 80%
</verification>