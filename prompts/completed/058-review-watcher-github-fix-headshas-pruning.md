---
status: completed
summary: Rebuilt HeadSHAs from current open-PR batch in processPRs to prune closed/merged PRs, preserving SHAs for filtered and fetch-error PRs, with a new test verifying stale entries are removed after a poll cycle.
container: code-reviewer-058-review-watcher-github-fix-headshas-pruning
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:24:46Z"
started: "2026-04-28T18:10:13Z"
completed: "2026-04-28T18:12:42Z"
---

<summary>
- The HeadSHAs map in cursor state grows unboundedly — one entry per PR ever seen
- Closed or merged PRs are never removed because the search query only returns open PRs
- Over months of operation the cursor.json file and in-memory map grow without limit
- The fix is to rebuild HeadSHAs at the end of each poll cycle from only the current batch
- PRs no longer in the open set are silently pruned from the cursor
- This is safe because re-opening a PR with the same number gets the same task ID and publishes a new CreateTaskCommand
- Tests must verify that closed PRs are removed from the cursor after a poll cycle
</summary>

<objective>
Prevent unbounded growth of `cursor.HeadSHAs` by rebuilding it at the end of each `Poll` cycle to contain only the task IDs seen in the current open-PR batch. Stale entries for closed/merged PRs are discarded naturally on each cycle.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/watcher.go` (~lines 58-80, 124-155): `Poll`, `processPRs` — where HeadSHAs is read and written
- `watcher/github/pkg/cursor.go` (~lines 20-24): `Cursor` struct with `HeadSHAs map[string]string`
- `watcher/github/pkg/watcher_test.go` (full): existing tests that verify HeadSHAs content after Poll
</context>

<requirements>
1. **In `watcher/github/pkg/watcher.go`**, the `cursorState Cursor` is currently passed by value to `processPRs` and `handlePR`. Replacing the `HeadSHAs` field on a value copy will NOT propagate to the caller. **Change `processPRs` to accept `*Cursor`** so the field replacement is visible to `Poll`:

   ```go
   func (w *watcher) processPRs(ctx context.Context, cursorState *Cursor, allPRs []PullRequest) time.Time { ... }
   ```

   Update the call site in `Poll` (~line 67) to pass `&cursorState`. `handlePR`/`publishCreate`/`publishForcePush` may keep their value-receiver `cursorState Cursor` semantics if they only mutate map keys (existing behavior), OR also be switched to `*Cursor` for consistency — preferred for clarity.

2. **Build the new `HeadSHAs` map inside `processPRs`** and replace at the end:

   At the start of `processPRs` (after `since := cursorState.LastUpdatedAt`):
   ```go
   newHeadSHAs := make(map[string]string, len(allPRs))
   ```

   Restructure the per-PR loop so `taskIDStr` is computed BEFORE the `ShouldSkipPR` skip-continue. This is required because skipped-but-already-known PRs must preserve their SHA in `newHeadSHAs`:

   ```go
   for _, pr := range allPRs {
       taskIDStr := DeriveTaskID(pr).String() // (or whatever the existing helper is)
       if w.filter.ShouldSkipPR(pr) {
           if known, ok := cursorState.HeadSHAs[taskIDStr]; ok {
               newHeadSHAs[taskIDStr] = known
           }
           continue
       }
       headSHA, err := w.fetchHeadSHA(ctx, pr)
       if err != nil {
           // Preserve prior SHA on transient fetch error to avoid spurious force-push
           // detection / re-create on next cycle.
           if known, ok := cursorState.HeadSHAs[taskIDStr]; ok {
               newHeadSHAs[taskIDStr] = known
           }
           continue
       }
       if w.handlePR(ctx, cursorState, pr, taskIDStr, headSHA) {
           newHeadSHAs[taskIDStr] = headSHA
       }
       // update maxUpdatedAt as before
   }
   ```

   At the end of `processPRs`, before returning `maxUpdatedAt`:
   ```go
   cursorState.HeadSHAs = newHeadSHAs
   ```

2. **Update `watcher/github/pkg/watcher_test.go`** to add a test:
   - First poll: PRs A and B both processed, both in cursor HeadSHAs
   - Second poll: only PR A returned (PR B was closed/merged)
   - Assert: after second poll, cursor HeadSHAs contains only PR A's task ID, not PR B's

3. Run `cd watcher/github && make test` — must pass.

4. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- The pruning must happen via `newHeadSHAs` rebuild — do NOT mutate `cursorState.HeadSHAs` by deleting keys from the existing map while iterating
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Filtered PRs (via `ShouldSkipPR`) that are already known should have their SHA preserved in `newHeadSHAs` to avoid re-publishing them as "new" on the next cycle
</constraints>

<verification>
cd watcher/github && grep -n "newHeadSHAs" pkg/watcher.go
# Expected: declaration, population, and assignment back to cursorState

cd watcher/github && make precommit
</verification>
