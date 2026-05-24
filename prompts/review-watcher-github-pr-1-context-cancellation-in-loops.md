---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `processPRs` loop iterates all PRs per poll without checking context cancellation — GitHub API calls and Kafka sends ignore shutdown
- `fetchAllPRs` checks `ctx.Done()` only after each API call, not before — a slow/hanging `SearchPRs` blocks shutdown until HTTP timeout
- `Poll` itself has no per-cycle ctx check, so heavy poll cycles (50+ PRs) cannot be aborted when context is cancelled
</summary>

<objective>
Add non-blocking `select { case <-ctx.Done(): ... }` context checks at the top of the `processPRs` per-PR loop and at the start of each pagination iteration in `fetchAllPRs`. Add a ctx check inside `Poll` before calling `processPRs`.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-context-cancellation-in-loops.md` in `~/.claude/plugins/marketplaces/coding/docs/` — when to apply, error wrapping with ctx.

Files to read before making changes:
- `watcher/github-pr/pkg/watcher.go` — lines 74-232 (Poll, fetchAllPRs, processPRs); read fully
</context>

<requirements>
1. In `watcher/github-pr/pkg/watcher.go`, add a non-blocking context check at the **top** of the `for _, pr := range allPRs` loop in `processPRs` (~line 160). Insert immediately after `pr := pr`:
   ```go
   select {
   case <-ctx.Done():
       glog.V(2).Infof("poll cancelled during processPRs at pr %d", pr.Number)
       return nil
   default:
   }
   ```
   This ensures that if the watcher is shutting down mid-iteration (e.g., 200 PRs in scope), the loop exits promptly.

2. In `fetchAllPRs`, add a context check **before** each `w.ghClient.SearchPRs` call (~line 113), not just after:
   ```go
   select {
   case <-ctx.Done():
       glog.V(2).Infof("fetchAllPRs cancelled before page search")
       return nil, nil
   default:
   }
   ```
   Move the existing post-call ctx check to be a pre-call check instead. The key fix is that a slow/hanging `SearchPRs` call must not block shutdown.

3. In `Poll` (~line 74), add a context check before calling `processPRs`:
   ```go
   select {
   case <-ctx.Done():
       return nil
   default:
   }
   ```
   This allows the outer poll loop in `main.go:runPollLoop` to cancel the entire poll cycle promptly.

4. Run `make test`:
   ```bash
   cd watcher/github-pr && make test
   ```
   Fix any compilation errors.

5. Run `make precommit`:
   ```bash
   cd watcher/github-pr && make precommit
   ```
</requirements>

<constraints>
- Only change `watcher/github-pr/pkg/watcher.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Wrapf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
- `glog.V(2)` for cancellation logs (developer-level, not operator-level)
- Do NOT add blocking `time.Sleep` — use non-blocking `select { case <-ctx.Done(): ... default: }`
</constraints>

<verification>
cd watcher/github-pr && make precommit

# Confirm ctx check in processPRs loop:
grep -n "case <-ctx.Done" watcher/github-pr/pkg/watcher.go

# Confirm ctx check before SearchPRs:
grep -B 2 "SearchPRs" watcher/github-pr/pkg/watcher.go | grep "ctx.Done"

# Confirm tests still pass:
grep -c "PASS" /dev/null || true
</verification>
