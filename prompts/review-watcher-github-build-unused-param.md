---
status: draft
created: "2026-05-24T12:00:00Z"
---

<summary>
- Remove unused `context.Context` parameter from `ParseRepoAllowlist` in `pkg/filter/filter.go`
- The ctx parameter is named `_` and never used, violating the project's context usage convention
- Update the single caller in `main.go` and `cmd/run-once/main.go` if the signature changes
</summary>

<objective>
Remove the unused `_ context.Context` parameter from `ParseRepoAllowlist` in `pkg/filter/filter.go`. The function does not use the context parameter and names it `_` indicating it was reserved for future use but never needed.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/filter/filter.go` lines 1-50 (`ParseRepoAllowlist` function)
- `watcher/github-build/main.go` lines 113-116 (caller)
- `watcher/github-build/cmd/run-once/main.go` lines 50-56 (caller)
</context>

<requirements>
1. In `pkg/filter/filter.go`, change the function signature from:
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
- Only change files in `watcher/github-build/` (pkg/filter/filter.go, main.go, cmd/run-once/main.go)
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
