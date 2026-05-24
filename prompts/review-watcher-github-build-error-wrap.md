---
status: draft
created: "2026-05-24T12:00:00Z"
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
- `watcher/github-build/main.go` lines 113-116
- `watcher/github-build/cmd/run-once/main.go` lines 50-56
</context>

<requirements>
1. In `main.go` line ~115, change:
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

2. In `cmd/run-once/main.go` line ~54, make the same change.

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
