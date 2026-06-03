---
status: completed
summary: Wrapped errors following ParseRepoAllowlist calls in main.go and cmd/run-once/main.go using errors.Wrap for defensive error attribution
container: maintainer-exec-186-review-watcher-github-build-error-wrap
dark-factory-version: v0.173.0
created: "2026-05-24T12:00:00Z"
queued: "2026-05-26T06:00:56Z"
started: "2026-05-26T06:02:28Z"
completed: "2026-05-26T06:03:35Z"
---

<summary>
- Add defensive error wrapping for bare `return err` in `main.go` and `cmd/run-once/main.go`
- The `ParseRepoAllowlist` function currently never returns errors, but wrapping defensively prevents future bugs
- Ensures all error paths are properly attributed with context
</summary>

<objective>
Add `errors.Wrap` around the `return err` statements following `filter.ParseRepoAllowlist` calls in `main.go` and `cmd/run-once/main.go`. While `ParseRepoAllowlist` currently returns only `nil`, wrapping defensively ensures that if it gains error paths in the future, the errors are properly attributed.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/main.go` lines 81-84 (`ParseRepoAllowlist` call + bare `return err`)
- `watcher/github-build/cmd/run-once/main.go` lines 79-82 (same pattern)
- `watcher/github-build/pkg/filter/repo_allowlist_filter.go` line 18 — `ParseRepoAllowlist(raw string) ([]string, error)` signature is **no longer ctx-taking** (changed by an earlier prompt). Both call sites pass only `a.RepoAllowlist`, not `ctx`.
</context>

<requirements>
1. In `watcher/github-build/main.go` (~line 83), change:
   ```go
   if err != nil {
       return err
   }
   ```
   To:
   ```go
   if err != nil {
       return errors.Wrap(ctx, err, "parse repo allowlist")
   }
   ```

2. In `watcher/github-build/cmd/run-once/main.go` (~line 81), make the same change.

   Note: both call sites already have `ctx` in scope (it's the first parameter of the enclosing `Run` method), so the `errors.Wrap(ctx, ...)` form works without further changes.

3. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

4. Run `cd watcher/github-build && go test ./...` to confirm tests pass.
</requirements>

<constraints>
- Only change `watcher/github-build/main.go` and `watcher/github-build/cmd/run-once/main.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
