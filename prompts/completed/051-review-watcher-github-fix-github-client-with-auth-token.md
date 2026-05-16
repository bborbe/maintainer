---
status: completed
summary: Replaced oauth2.StaticTokenSource + context.Background() with gogithub.NewClient(nil).WithAuthToken(token) in watcher/github/pkg/githubclient.go, removing the golang.org/x/oauth2 import.
container: code-reviewer-051-review-watcher-github-fix-github-client-with-auth-token
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:19:27Z"
started: "2026-04-28T15:50:05Z"
completed: "2026-04-28T15:52:37Z"
---

<summary>
- The GitHub client constructor uses a legacy oauth2 token-source pattern that embeds a hardcoded context.Background()
- This violates the project rule that context.Background() must not appear in pkg/ business logic
- The go-github v62 library ships a built-in WithAuthToken method that replaces the oauth2 boilerplate entirely
- Switching removes the golang.org/x/oauth2 import from githubclient.go
- The oauth2 transport cannot be cancelled by callers because it is bound to a non-cancellable context
- The factory call site must be updated to drop the no-longer-needed context threading
- Tests using the fake GitHubClient are unaffected (they use the counterfeiter mock)
- make precommit must pass after the change
</summary>

<objective>
Replace the `oauth2.StaticTokenSource` + `oauth2.NewClient(context.Background(), ts)` pattern in `NewGitHubClient` with `gogithub.NewClient(nil).WithAuthToken(token)`. This eliminates the `context.Background()` violation in `pkg/` and removes the now-unnecessary `golang.org/x/oauth2` import from that file. The factory call site requires a trivial update to drop the token parameter forwarding.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/githubclient.go` (~lines 56-63): `NewGitHubClient` constructor using `oauth2.StaticTokenSource` + `oauth2.NewClient(context.Background(), ts)`
- `watcher/github/pkg/factory/factory.go` (~line 59): `pkg.NewGitHubClient(ghToken)` call site (verified, signature unchanged)
- `watcher/github/pkg/githubclient_test.go`: verify no tests directly exercise the constructor (they use the mock)
- `watcher/github/pkg/githubclient_export_test.go`: check if any internals are exported for test
</context>

<requirements>
1. In `watcher/github/pkg/githubclient.go`, rewrite `NewGitHubClient` (~line 57):

   Replace:
   ```go
   func NewGitHubClient(token string) GitHubClient {
       ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
       httpClient := oauth2.NewClient(context.Background(), ts)
       return &githubClient{
           client: gogithub.NewClient(httpClient),
       }
   }
   ```
   With:
   ```go
   func NewGitHubClient(token string) GitHubClient {
       return &githubClient{
           client: gogithub.NewClient(nil).WithAuthToken(token),
       }
   }
   ```

2. Remove the now-unused imports from `watcher/github/pkg/githubclient.go`:
   - Remove `"context"` (no longer needed in this file — all methods already receive `ctx` as a parameter)
   - Remove `"golang.org/x/oauth2"` (fully replaced by `WithAuthToken`)

3. Verify `watcher/github/pkg/factory/factory.go` still compiles unchanged — `pkg.NewGitHubClient(ghToken)` on ~line 61 takes only the token string and does not change.

4. Run `cd watcher/github && make test` — must pass.

5. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Do NOT change the `GitHubClient` interface signature — only the constructor body changes
- Do NOT add a `context.Context` parameter to `NewGitHubClient` — `WithAuthToken` makes that unnecessary
</constraints>

<verification>
cd watcher/github && grep -n "context.Background\|oauth2" pkg/githubclient.go
# Expected: no matches (both removed)

cd watcher/github && grep -n "WithAuthToken" pkg/githubclient.go
# Expected: one match

cd watcher/github && make precommit
</verification>
