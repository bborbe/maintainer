---
status: completed
summary: Hardened Bitbucket HTTP client with 30s timeout, HTTPS upgrade for non-loopback hosts, and migrated all error construction to github.com/bborbe/errors with proper context propagation
container: code-reviewer-053-review-agent-pr-reviewer-security-and-error-wrapping-bitbucket-client
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T12:00:00Z"
queued: "2026-04-28T15:19:27Z"
started: "2026-04-28T16:21:53Z"
completed: "2026-04-28T16:24:56Z"
---

<summary>
- The Bitbucket HTTP client has no timeout, allowing a slow or adversarial server to hold connections open indefinitely and exhaust goroutines
- The URL builder accepts the `http://` scheme, which would send credentials over cleartext HTTP
- All error constructors in the Bitbucket client use stdlib `fmt.Errorf` instead of the project's context-aware error wrapping
- Three methods silently drop error context by using bare `return err` after status-check helpers
- Fixing all three concerns in one pass avoids repeated edits to the same file
</summary>

<objective>
Harden the Bitbucket HTTP client against slow-server DoS and cleartext credential transmission, and migrate all error construction to `github.com/bborbe/errors` with proper context propagation. After this fix the client will time out after 30 s, will never transmit credentials over HTTP, and all errors will carry stack traces.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/bitbucket/client.go` — `NewClient` (~line 35, NOT `NewBitbucketClient`), `httpClient` struct (~line 42), `GetPRBranches` (~line 71), `PostComment` (~line 120), `Approve` (~line 160), `NeedsWork` (~line 192), `buildURL` (~line 241), `checkResponseStatus` (~line 250), `checkApproveResponseStatus` (~line 268)
- `agent/pr-reviewer/pkg/bitbucket/client_test.go` — existing test coverage and test server pattern
- `agent/pr-reviewer/pkg/steps/gh_token.go` — reference implementation: `http.Client{Timeout: 10 * time.Second}` (~line 61)
</context>

<requirements>
1. **Add `Timeout: 30 * time.Second` to the `http.Client`** in `NewClient` (~line 35). Replace `httpClient: &http.Client{}` with `httpClient: &http.Client{Timeout: 30 * time.Second}`. Same pattern as `pkg/steps/gh_token.go:61`.

2. **Harden `buildURL`** (~lines 241–246) — current code passes through any `http://` or `https://` prefix. Change so that only loopback/test hosts are allowed to remain `http://`; everything else is upgraded to HTTPS. The tests use `httptest.NewServer` which produces `http://127.0.0.1:PORT/...`.

   New `buildURL`:
   ```go
   func (c *httpClient) buildURL(host, path string) string {
       if strings.HasPrefix(host, "https://") {
           return host + path
       }
       if strings.HasPrefix(host, "http://") {
           // Allow http only for loopback (test servers); upgrade everything else.
           u := strings.TrimPrefix(host, "http://")
           if strings.HasPrefix(u, "127.0.0.1") || strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "[::1]") {
               return host + path
           }
           return "https://" + u + path
       }
       return "https://" + host + path
   }
   ```
   This preserves existing test behavior (httptest uses 127.0.0.1) while enforcing HTTPS in production.

3. **Replace all `fmt.Errorf` calls with `errors.Wrapf` / `errors.Errorf`** from `github.com/bborbe/errors`. The 4 public methods (`GetPRBranches`, `PostComment`, `Approve`, `NeedsWork`) already have `ctx context.Context`. The two helpers `checkResponseStatus` and `checkApproveResponseStatus` do NOT — add `ctx context.Context` as their first parameter and update their callers to pass `ctx`:
   - `fmt.Errorf("...: %w", err)` → `errors.Wrapf(ctx, err, "...")`
   - `fmt.Errorf("...")` (no wrapped error) → `errors.Errorf(ctx, "...")`
   - Method call sites: lines 81, 88, 98, 103, 107, 110, 135, 140, 148, 173, 180, 214, 219, 227.
   - Helper call sites (in `checkResponseStatus`/`checkApproveResponseStatus`, ~lines 255–281): same conversion.

4. **Fix bare `return err`** in three places (~lines 153, 185, 232) where callers of `checkResponseStatus` / `checkApproveResponseStatus` propagate the error without context. Replace with:
   ```go
   return errors.Wrap(ctx, err, "check response status")
   ```

5. **Update tests** in `client_test.go`: the test server uses `http://` URLs via `httptest.NewServer`. After step 2, ensure the test correctly exercises the upgraded path. Add a new test case for `buildURL` with a plain `http://` host to assert it becomes `https://`.

6. **Run `cd agent/pr-reviewer && make test`** — must pass before proceeding to precommit.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` (always pass `ctx` as first arg) — never `fmt.Errorf`, never bare `return err`
- Code changes MUST add or update tests for changed paths — paths covered before the fix must remain covered, new paths must be tested
- Do NOT add `//nolint` directives
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
