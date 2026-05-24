---
status: draft
created: "2026-05-24T12:00:00Z"
---

<summary>
- Add context cancellation checks inside `pollRepo` at logical blocking boundaries
- Prevents SIGTERM from being deferred until after the current repo finishes all API calls
- Context is now checked before GetDefaultBranch, GetWorkflowRuns, and each fetchJobInfoForRun call
</summary>

<objective>
Add non-blocking `select { case <-ctx.Done(): return false; default: }` checks inside `pollRepo` at logical blocking boundaries — before `GetDefaultBranch`, before `GetWorkflowRuns`, and before each `fetchJobInfoForRun` call in the failing-runs loop. This ensures context cancellation is detected during long-running operations, not just at repo-loop boundaries.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes:
- `watcher/github-build/pkg/watcher.go` lines 136-220 (`pollRepo` function)
- `watcher/github-build/pkg/watcher.go` lines 380-400 (`fetchJobInfoForRun` function)
</context>

<requirements>
1. In `pollRepo` at `pkg/watcher.go` ~line 137, add context cancellation checks at the following points:

   a) Before `GetDefaultBranch` call (~line 142):
   ```go
   if repoState.DefaultBranch == "" {
       select {
       case <-ctx.Done():
           return false
       default:
       }
       branch, err := w.githubClient.GetDefaultBranch(ctx, owner, repo)
   ```

   b) Before `GetWorkflowRuns` call (~line 155):
   ```go
   select {
   case <-ctx.Done():
       return false
   default:
   }
   runs, err := w.githubClient.GetWorkflowRuns(ctx, owner, repo, runState)
   ```

   c) Before each `fetchJobInfoForRun` call in the failing-runs loop (~line 196):
   Inside the `for _, run := range failingRuns` loop, add a context check before the `fetchJobInfoForRun` call.

2. Do NOT add checks inside `fetchJobInfoForRun` itself — the caller should manage cancellation. The `fetchJobInfoForRun` already returns early with "?" placeholders on error, which is the intended degradation mode.

3. Run `cd watcher/github-build && go build ./...` to confirm the build succeeds.

4. Run `cd watcher/github-build && go test ./...` to confirm existing tests pass.
</requirements>

<constraints>
- Only change `watcher/github-build/pkg/watcher.go`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf` or bare `return err`
</constraints>

<verification>
cd watcher/github-build && go build ./... && go test ./...
</verification>
