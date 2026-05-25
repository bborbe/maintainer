---
status: approved
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:25:46Z"
---

<summary>
- Add concurrency limiter to the `/trigger` endpoint to prevent unbounded concurrent poll executions
- A malicious or misconfigured caller can hammer `/trigger` to exhaust GitHub API rate limits and Kafka capacity
- Use an atomic counter with a cooldown window to refuse triggers that arrive while a poll is running
</summary>

<objective>
Add a concurrency limiter to the `/trigger` endpoint in `main.go`. Use an `atomic.Int64` counter with a cooldown window: if a trigger arrives while a previous trigger's poll is still running, refuse it with a `503 Service Unavailable` response instead of spawning another concurrent poll.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/main.go` lines 180-202 (`runPollLoop`) and 204-218 (`runHTTPServer`)
</context>

<requirements>
1. Add an `atomic.Int64` field to the `application` struct:
   ```go
   type application struct {
       // ... existing fields ...
       triggerRunning atomic.Int64
   }
   ```

2. Create a helper function to atomically check-and-set the trigger:
   ```go
   func (a *application) tryAcquireTrigger() bool {
       return a.triggerRunning.CompareAndSwap(0, 1)
   }
   ```

3. Modify `runHTTPServer` to pass a wrapped `poll` function that uses the limiter. The `/trigger` handler is already at line 214:
   ```go
   router.Path("/trigger").Handler(libhttp.NewBackgroundRunHandler(context.Background(), func(ctx context.Context) error {
       if !a.tryAcquireTrigger() {
           return errors.New("trigger already running, wait for current poll to complete")
       }
       defer a.triggerRunning.Store(0)
       return poll(ctx)
   }))
   ```

4. Keep `NewBackgroundRunHandler` using `context.Background()` (from the separate trigger-context fix).

5. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

6. Run `cd watcher/github-build && go test ./...` to confirm tests pass.
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
