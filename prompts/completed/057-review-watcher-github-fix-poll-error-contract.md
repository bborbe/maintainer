---
status: completed
summary: Fixed Poll to propagate LoadCursor error via errors.Wrapf instead of swallowing it, and changed processPRs/handlePR/publishCreate/publishForcePush to accept *Cursor so HeadSHAs mutation is explicit in function signatures; added permission-denial test for LoadCursor error path.
container: code-reviewer-057-review-watcher-github-fix-poll-error-contract
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:24:46Z"
started: "2026-04-28T18:07:50Z"
completed: "2026-04-28T18:10:12Z"
---

<summary>
- The Watcher interface declares Poll returns an error, but the implementation always returns nil
- This makes the error check in main.go's poll loop dead code (glog.Errorf is never reached)
- LoadCursor returning a real error should propagate rather than being silently swallowed
- cursorState is passed by value to processPRs but mutated through the embedded HeadSHAs map
- This implicit aliasing makes mutation invisible in function signatures — pass *Cursor instead
- Both issues reduce code trustworthiness and make testing harder
- Changes require updating all callsites and tests that construct or pass Cursor
</summary>

<objective>
Fix `Poll` to surface the `LoadCursor` error instead of swallowing it (align implementation with interface contract), and change `cursorState` to be passed as `*Cursor` everywhere it is mutated so the mutation is explicit in the function signature.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/watcher.go` (full): `Poll`, `processPRs`, `handlePR`, `publishCreate`, `publishForcePush` — anchor by function name, not line
- `watcher/github/pkg/watcher_test.go` (full): all tests that construct `Cursor` or call `Poll`
- `watcher/github/main.go`: the `runPollLoop` function with the `if err := w.Poll(ctx)` check
</context>

<requirements>
1. **Fix `Poll` error propagation in `watcher/github/pkg/watcher.go`** (~line 58):

   Change the `LoadCursor` error handling from:
   ```go
   cursorState, err := LoadCursor(ctx, w.cursorPath, w.startTime)
   if err != nil {
       glog.Errorf("failed to load cursor err=%v", err)
       return nil  // swallowed
   }
   ```
   To:
   ```go
   cursorState, err := LoadCursor(ctx, w.cursorPath, w.startTime)
   if err != nil {
       return errors.Wrapf(ctx, err, "load cursor")
   }
   ```
   The outer `runPollLoop` in `main.go` already logs the error when `Poll` returns one, so this will surface properly. Read the actual log statement in `runPollLoop` before referencing it.

   Keep the `fetchAllPRs` error handling as `return nil` (transient — log and retry). Keep `SaveCursor` error as log-only.

2. **Fix `cursorState` aliasing — pass `*Cursor` in `watcher/github/pkg/watcher.go`**:

   Change the following function signatures to accept `*Cursor` instead of `Cursor`:
   - `processPRs(ctx context.Context, cursorState *Cursor, allPRs []PullRequest) time.Time`
   - `handlePR(ctx context.Context, cursorState *Cursor, pr PullRequest, taskIDStr, headSHA string) bool`
   - `publishCreate(ctx context.Context, cursorState *Cursor, pr PullRequest, taskIDStr, headSHA string) bool`
   - `publishForcePush(ctx context.Context, cursorState *Cursor, pr PullRequest, taskIDStr, oldSHA, newSHA string) bool`

   Update the call sites in `Poll` to pass `&cursorState`:
   ```go
   maxUpdatedAt := w.processPRs(ctx, &cursorState, allPRs)
   ```

3. **Update `watcher/github/pkg/watcher_test.go`** to add a test case for the `LoadCursor` error propagation:
   - Test: when the cursor path points to a file that exists but has mode 000 (unreadable), `Poll` returns a non-nil error.
   - Setup: `os.WriteFile(cursorPath, []byte("{}"), 0600)` then `os.Chmod(cursorPath, 0000)`, defer restore to 0600.
   - Assert: `Expect(err).To(HaveOccurred())`.
   - Note: this test requires running as a non-root user to observe the permission denial. Add `if os.Getuid() == 0 { Skip("running as root, skipping permission test") }`.

4. Run `cd watcher/github && make test` — must pass.

5. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `fetchAllPRs` errors must remain non-propagating (transient network failures should not crash the poll loop)
- `SaveCursor` errors must remain log-only (cursor loss → double-publish on restart, which is tolerable)
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`, never bare `return err`
</constraints>

<verification>
cd watcher/github && grep -n "return nil" pkg/watcher.go | grep -i "cursor\|load"
# Expected: no matches (LoadCursor error now propagates)

cd watcher/github && grep -n "cursorState \*Cursor" pkg/watcher.go
# Expected: multiple matches (pointer receiver in processPRs, handlePR, publishCreate, publishForcePush)

cd watcher/github && make precommit
</verification>
