---
status: committing
summary: Replaced context.Background() with parent ctx in cleanup closure in createCloneAndFetch so cleanup is cancelled when signal fires
container: maintainer-exec-156-review-agent-pr-reviewer-3-context-background-cmd-cli
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T21:00:21Z"
started: "2026-05-25T21:00:22Z"
completed: "2026-05-25T21:01:41Z"
---

<summary>
- `cmd/cli/main.go:277` uses `context.Background()` in a deferred cleanup function
- This cleanup runs after the main signal context may already be cancelled, making it unkillable
- Fix: pass the parent `ctx` to the cleanup closure so it respects the signal context's cancellation
</summary>

<objective>
Fix the deferred cleanup function in `cmd/cli/main.go` to use a cancellable context instead of `context.Background()`, enabling graceful shutdown when the signal context fires.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-context-cancellation-in-loops.md` in `~/.claude/plugins/marketplaces/coding/docs/` — context cancellation patterns.

Files to read before making changes:
- `agent/pr-reviewer/cmd/cli/main.go` — full file; understand `createCloneAndFetch` (~line 200) and the cleanup closure (~line 277)
</context>

<requirements>
**Execute steps in order. Run `make test` after the fix. Run `make precommit` only at the final step.**

1. **Fix the cleanup context in `cmd/cli/main.go`**

   Locate the `createCloneAndFetch` function and its returned cleanup closure. The issue is:
   ```go
   cleanup := func() {
       cleanupCtx := context.Background()  // ← BUG: unkillable after signal
       if err := worktreeManager.RemoveClone(cleanupCtx, clonePath); err != nil {
           glog.Warningf("cleanup remove clone failed: %v", err)
       }
   }
   ```

   Change to pass the parent `ctx` through so the cleanup respects signal cancellation:
   ```go
   cleanup := func() {
       // Use parent ctx so cleanup is cancelled when signal fires — bounded by signal context lifetime
       if err := worktreeManager.RemoveClone(ctx, clonePath); err != nil {
           glog.Warningf("cleanup remove clone failed: %v", err)
       }
   }
   ```

   Verify that `ctx` is in scope at the closure creation point (it should be the `ctx` from `createCloneAndFetch`'s signature).

2. **Run `make test`** to verify:

   ```bash
   cd agent/pr-reviewer && make test
   ```

3. **Run `make precommit`** for final validation:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only change `agent/pr-reviewer/cmd/cli/main.go`
- Do NOT commit — dark-factory handles git
- The cleanup must use the parent `ctx` so it can be cancelled when the process receives a signal
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
