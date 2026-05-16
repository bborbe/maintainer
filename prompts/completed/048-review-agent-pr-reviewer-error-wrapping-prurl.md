---
status: completed
summary: Added ctx context.Context to prurl.Parse and internal helpers, replaced all fmt.Errorf calls with errors.Errorf(ctx, ...) from github.com/bborbe/errors, and updated all callers.
container: code-reviewer-048-review-agent-pr-reviewer-error-wrapping-prurl
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T12:00:00Z"
queued: "2026-04-28T14:52:29Z"
started: "2026-04-28T15:04:28Z"
completed: "2026-04-28T15:06:40Z"
---

<summary>
- The PR URL parser constructs all errors with `fmt.Errorf`, producing errors without stack traces or context metadata
- The parser function currently has no `context.Context` parameter, so the project-standard `errors.Errorf(ctx, …)` cannot be used without adding one
- The internal sub-parsers (`parseGitHub`, `parseBitbucket`) also need the context parameter threaded through
- All callers of `prurl.Parse` must be updated to pass `ctx` after the signature change
- Migrating to context-aware errors aligns the parser with every other package in the service
</summary>

<objective>
Add `ctx context.Context` to `prurl.Parse` and its internal helpers, then replace all `fmt.Errorf` calls with `errors.Errorf(ctx, "...")`. Update all callers of `prurl.Parse` to pass their existing context. After this fix the URL parser returns context-tagged errors that carry stack traces.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/prurl/prurl.go` — `Parse` function (~line 35), `parseGitHub` and `parseBitbucket` helpers, all `fmt.Errorf` calls at ~lines 43, 48, 52, 65, 76, 85, 89, 94, 120, 124, 130, 136, 142, 147
- `agent/pr-reviewer/pkg/prurl/prurl_test.go` — test call sites (must be updated to pass `context.Background()`)
- `agent/pr-reviewer/cmd/cli/main.go` — caller of `prurl.Parse` (search for `prurl.Parse` usages, pass `ctx` from surrounding function)
</context>

<requirements>
1. **Update `prurl.Parse` signature** to accept `ctx context.Context` as first parameter (return type is unchanged — `*PRInfo`, not `PRInfo`):
   ```go
   func Parse(ctx context.Context, rawURL string) (*PRInfo, error)
   ```

2. **Thread `ctx` through internal helpers** `parseGitHub` and `parseBitbucket` by adding `ctx context.Context` as first parameter to each and updating their call sites in `Parse`.

3. **Replace all `fmt.Errorf` calls** in `prurl.go` with `errors.Errorf(ctx, "...")` (approximately 12–14 call sites at lines ~43, 48, 52, 65, 76, 85, 89, 94, 120, 124, 130, 136, 142, 147).

4. **Update `prurl_test.go`**: every call to `prurl.Parse(...)` must become `prurl.Parse(context.Background(), ...)`. Add `"context"` import. Tests may use `context.Background()` — this is correct for test code.

5. **Update the single caller in `cmd/cli/main.go`** (~line 68): find the call to `prurl.Parse(...)` and add `ctx` as first parameter from the surrounding `run` function (which has `ctx context.Context` via `signal.NotifyContext`). Verified there is only one caller in the repo.

   Imports: ensure `"github.com/bborbe/errors"` is imported in `prurl.go` (do not alias-collide with stdlib `errors`).

6. **Run `cd agent/pr-reviewer && make test`** — must pass.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Errorf(ctx, "...")` from `github.com/bborbe/errors` (always pass `ctx` as first arg) — never `fmt.Errorf`
- Test code may use `context.Background()` when calling `prurl.Parse` — that is the correct pattern in tests
- Code changes MUST add or update tests for changed paths — paths covered before the fix must remain covered
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
