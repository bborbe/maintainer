---
status: approved
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:00:21Z"
---

<summary>
- Add context cancellation check inside `listOwnerReposPaginated` pagination loop
- Prevents the loop from continuing for up to 50 API calls when context is already cancelled
- Ensures graceful shutdown is not delayed by stale pagination iterations
</summary>

<objective>
Add a non-blocking `select { case <-ctx.Done(): return ctx.Err(); default: }` at the top of the `for` loop body in `listOwnerReposPaginated` so that context cancellation stops pagination immediately instead of continuing to the next page.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/githubclient.go` lines 311-330 (`listOwnerReposPaginated` function)
</context>

<requirements>
1. In `listOwnerReposPaginated` at `pkg/githubclient.go` line ~318, add a context cancellation check at the start of the `for` loop body:
   ```go
   for {
       select {
       case <-ctx.Done():
           return ctx.Err()
       default:
       }
       repos, resp, err := c.fetchRepoPage(ctx, owner, isOrg, page)
       // ... rest of existing code unchanged
   ```

2. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

3. Run `cd watcher/github-build && go test ./...` to confirm existing tests pass.
</requirements>

<constraints>
- Only change `watcher/github-build/pkg/githubclient.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
