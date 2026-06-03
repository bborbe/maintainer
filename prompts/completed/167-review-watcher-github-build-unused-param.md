---
status: completed
summary: Removed unused context.Context parameter from ParseRepoAllowlist and updated all callers
container: maintainer-exec-167-review-watcher-github-build-unused-param
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:00:21Z"
started: "2026-05-25T21:19:00Z"
completed: "2026-05-25T21:20:41Z"
---

<summary>
- Remove unused `context.Context` parameter from `ParseRepoAllowlist` in `pkg/filter/repo_allowlist_filter.go`
- The ctx parameter is named `_` and never used, violating the project's context usage convention
- Update the single caller in `main.go` and `cmd/run-once/main.go` if the signature changes
</summary>

<objective>
Remove the unused `_ context.Context` parameter from `ParseRepoAllowlist` in `pkg/filter/repo_allowlist_filter.go`. The function does not use the context parameter and names it `_` indicating it was reserved for future use but never needed.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/filter/repo_allowlist_filter.go` lines 1-50 (`ParseRepoAllowlist` function)
- `watcher/github-build/pkg/filter/repo_allowlist_filter_test.go` (test callers — must update test invocations too)
- `watcher/github-build/main.go` lines 113-116 (caller)
- `watcher/github-build/cmd/run-once/main.go` lines 50-56 (caller)

CONFLICT NOTE: `review-watcher-github-build-error-wrap.md` adds error wrapping around `filter.ParseRepoAllowlist(ctx, ...)` call sites. If both prompts run, the unused-param fix must run FIRST, then error-wrap must be updated to match the new signature `filter.ParseRepoAllowlist(a.RepoAllowlist)`. Suggest approving unused-param first, then re-auditing error-wrap.
</context>

<requirements>
1. In `pkg/filter/repo_allowlist_filter.go`, change the function signature from:
   ```go
   func ParseRepoAllowlist(_ context.Context, raw string) ([]string, error) {
   ```
   To:
   ```go
   func ParseRepoAllowlist(raw string) ([]string, error) {
   ```

2. Update the call site in `main.go` line ~113 from:
   ```go
   repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
   ```
   To:
   ```go
   repoAllowlist, err := filter.ParseRepoAllowlist(a.RepoAllowlist)
   ```

3. Update the call site in `cmd/run-once/main.go` line ~50 from:
   ```go
   repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
   ```
   To:
   ```go
   repoAllowlist, err := filter.ParseRepoAllowlist(a.RepoAllowlist)
   ```

4. If `filter` import is no longer needed in `main.go` or `cmd/run-once/main.go` (because `ParseRepoAllowlist` was the only function used), remove the import. Check with `go build`.

5. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

6. Run `cd watcher/github-build && go test ./...` to confirm tests pass.
</requirements>

<constraints>
- Only change files in `watcher/github-build/` (pkg/filter/repo_allowlist_filter.go, main.go, cmd/run-once/main.go)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
