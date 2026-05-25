---
status: completed
summary: Use context.Background() in /trigger handler to prevent requests during graceful shutdown from being dropped
container: maintainer-exec-166-review-watcher-github-build-trigger-context
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:00:21Z"
started: "2026-05-25T21:17:33Z"
completed: "2026-05-25T21:18:56Z"
---

<summary>
- Fix `/trigger` endpoint to not capture the application-level context that gets cancelled on shutdown
- Background poll operations should use a fresh background context, not the app's lifecycle context
- Ensures trigger requests received during graceful shutdown are still processed
</summary>

<objective>
Fix the `/trigger` HTTP handler in `main.go` to use a fresh `context.Background()` instead of the application lifecycle `ctx`. The current code passes `ctx` (which is cancelled on SIGTERM) to `NewBackgroundRunHandler`, causing trigger requests during graceful shutdown to be silently dropped.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/main.go` lines 200-218 (`runHTTPServer` function)
- `watcher/github-build/main.go` lines 103-135 (application.Run start)
</context>

<requirements>
1. In `runHTTPServer` at line ~214, change the `/trigger` handler registration from:
   ```go
   router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(ctx, poll))
   ```
   To use `context.Background()`:
   ```go
   router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(context.Background(), poll))
   ```

2. Add `"context"` to the import block if not already present.

3. Verify no other handlers in `runHTTPServer` should similarly use `Background()` — the healthz, readiness, and metrics handlers are fine as-is (they don't spawn async work).

4. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.
</requirements>

<constraints>
- Only change `watcher/github-build/main.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
