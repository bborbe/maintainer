---
status: approved
created: "2026-05-24T12:00:00Z"
queued: "2026-05-25T21:00:21Z"
---

<summary>
- Add panic recovery wrapper around the refresh goroutine closure passed to CancelOnFirstFinish
- If RunRefreshLoop itself panics (not caught by safeRefresh), the goroutine reaper aborts all sibling tasks
- The closure now recovers panics and logs them, keeping the refresh task as a non-fatal degraded component
</summary>

<objective>
Wrap the `resolvedSet.RunRefreshLoop(ctx)` closure in `main.go:buildAllowlistSnapshot` with a panic-recover guard. While `safeRefresh` inside `RunRefreshLoop` catches panics from `Refresh`, any panic outside that guard (e.g. in ticker creation) would kill the entire `CancelOnFirstFinish` task set.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/main.go` lines 58-75 (`buildAllowlistSnapshot` function)
- `watcher/github-build/pkg/wildcard/resolved.go` lines 155-185 (`RunRefreshLoop` and `safeRefresh`)
</context>

<requirements>
1. In `main.go:buildAllowlistSnapshot` at line ~69, wrap the closure with panic recovery:
   ```go
   return resolvedSet, func(ctx context.Context) error {
       defer func() {
           if rec := recover(); rec != nil {
               glog.Errorf("wildcard refresh loop panic recovered: %v", rec)
           }
       }()
       return resolvedSet.RunRefreshLoop(ctx)
   }
   ```

2. The `safeRefresh` panic-recover in `resolved.go` is still needed — it catches panics from individual `Refresh` calls. The new defer in `main.go` catches panics from `RunRefreshLoop` itself (e.g. if the ticker creation panics).

3. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

4. Run `cd watcher/github-build && go test ./...` to confirm existing tests pass.
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
