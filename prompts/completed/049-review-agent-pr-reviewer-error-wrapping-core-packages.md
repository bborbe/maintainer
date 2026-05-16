---
status: completed
summary: Migrated all fmt.Errorf calls in pkg/review, pkg/github/client.go, pkg/config/config.go, and pkg/steps/review.go to errors.Wrapf/Errorf(ctx,...), replaced log.Printf with glog.Warningf, and threaded ctx through validateConfig, FindRepo, extractVerdict, and lastJSONBlock.
container: code-reviewer-049-review-agent-pr-reviewer-error-wrapping-core-packages
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T12:00:00Z"
queued: "2026-04-28T14:52:29Z"
started: "2026-04-28T15:41:48Z"
completed: "2026-04-28T15:47:06Z"
---

<summary>
- Four packages each use stdlib `fmt.Errorf` for error construction while the project standard is `github.com/bborbe/errors`, which attaches stack traces and context metadata
- An internal helper function constructs errors without a `context.Context` parameter, requiring that parameter to be threaded through from its caller
- One file uses the stdlib `log` package for a warning message while the rest of the codebase uses `glog`
- The error at a key failure site discards the original `exec` error, replacing it with only the stderr text and losing the exit-code information
- All affected files already have `ctx context.Context` available at their call sites — no new parameters are needed except for the internal helper
</summary>

<objective>
Migrate all `fmt.Errorf` calls in `pkg/review`, `pkg/github/client.go`, `pkg/config/config.go`, and `pkg/steps/review.go` to `errors.Wrapf(ctx, err, "...")` / `errors.Errorf(ctx, "...")`, replace the `log.Printf` call with `glog.Warningf`, and thread `ctx` through the private `extractVerdict` helper so it can produce context-tagged errors. After this fix errors in these packages carry full stack traces.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `agent/pr-reviewer/pkg/review/review.go` — `Review` method, `fmt.Errorf` calls ~lines 44, 50, 56, 59, 87 (note: line 87 discards the `exec` run error)
- `agent/pr-reviewer/pkg/review/review_test.go` — existing test coverage
- `agent/pr-reviewer/pkg/github/client.go` — `GetPRBranches`, `PostComment` methods, `fmt.Errorf` calls ~lines 77, 82, 117, 134, 162
- `agent/pr-reviewer/pkg/github/client_test.go` — existing tests
- `agent/pr-reviewer/pkg/config/config.go` — `validateConfig` (~line 180), `FindRepo` (~line 205), `fmt.Errorf` at ~lines 183, 199, 217; `log.Printf` warning ~line 149; imports `"log"` ~line 10
- `agent/pr-reviewer/pkg/config/config_test.go` — existing tests
- `agent/pr-reviewer/pkg/steps/review.go` — `extractVerdict` (~line 125), `lastJSONBlock` (~line 130), `fmt.Errorf` at ~lines 131, 134; `Run` method which calls `extractVerdict` ~line 81
- `agent/pr-reviewer/pkg/steps/review_test.go` and `export_test.go` — existing test infrastructure
</context>

<requirements>
1. **`pkg/review/review.go`** — Replace 5 `fmt.Errorf` calls with `errors.Wrapf(ctx, err, "...")` (the `Review` method already accepts `ctx context.Context`):
   - Lines ~44, 50, 56, 59: wrap OS / file errors.
   - Line ~87: **keep the original `err` from `cmd.Run()`** — replace `fmt.Errorf("claude review failed: %s", strings.TrimSpace(stderr.String()))` with `errors.Wrapf(ctx, err, "claude review failed: %s", strings.TrimSpace(stderr.String()))`. This preserves the exit-code information in the error chain.

2. **`pkg/github/client.go`** — Replace all `fmt.Errorf` calls (~lines 77, 82, 117, 134, 162) with `errors.Wrapf(ctx, err, "...")` or `errors.Errorf(ctx, "...")`. All `Client` methods receive `ctx context.Context`.

3. **`pkg/config/config.go`** — three sites have `fmt.Errorf`, only the first is in `validateConfig`:
   a. Replace `log.Printf(...)` (~line 149) with `glog.Warningf(...)`. Remove the `"log"` import; `"github.com/golang/glog"` should already be imported elsewhere.
   b. Line 183 (`validateConfig`): replace `fmt.Errorf(...)` with `errors.Errorf(ctx, "...")`. Thread `ctx context.Context` as the first parameter of `validateConfig`; its caller `fileLoader.Load` already has `ctx`.
   c. Line 199 (`FindRepoPath`) and line 217 (`FindRepo`): both methods currently have signature `func (c *Config) FindRepoPath(repoURL string) (string, error)` / `FindRepo(repoURL string) (*RepoInfo, error)`. Add `ctx context.Context` as the first parameter, replace the `fmt.Errorf` with `errors.Errorf(ctx, "...")`. Update all callers:
      - `agent/pr-reviewer/cmd/cli/main.go:83` (`cfg.FindRepo(prInfo.RepoURL)` → pass `ctx`)
      - `agent/pr-reviewer/pkg/config/config_test.go:509` (`cfg.FindRepoPath(repoURL)` → pass `ctx`)
      - `agent/pr-reviewer/pkg/config/config_test.go:612` (`cfg.FindRepo(repoURL)` → pass `ctx`)
   These are the only three callers in the repo; verify with `grep -rn "FindRepoPath\\|FindRepo(" agent/pr-reviewer --include="*.go"`.

4. **`pkg/steps/review.go`** — Thread `ctx` into the private helpers (preserve existing return types):
   a. Current signatures: `extractVerdict(raw string) (verdictPayload, error)` and `lastJSONBlock(s string) (string, bool)`. Change to:
      - `extractVerdict(ctx context.Context, raw string) (verdictPayload, error)`
      - `lastJSONBlock(ctx context.Context, s string) (string, bool)` — return type stays `(string, bool)`; do NOT change to error.
   b. Update the call site in `Run` (~line 81): pass `ctx` to `extractVerdict`.
   c. Inside `extractVerdict`, replace any `fmt.Errorf` with `errors.Errorf(ctx, "...")` / `errors.Wrapf(ctx, err, "...")`.
   d. Update `export_test.go` and any direct test calls to `extractVerdict`/`lastJSONBlock` to pass `context.Background()`.

5. **Run `cd agent/pr-reviewer && make test`** — must pass after changes.
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` (always pass `ctx` as first arg) — never `fmt.Errorf`, never bare `return err`
- Test code may use `context.Background()` when calling helpers that now require ctx — this is the correct pattern in tests
- Code changes MUST add or update tests for changed paths — paths covered before the fix must remain covered, new paths must be tested
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
