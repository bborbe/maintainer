---
status: draft
created: "2026-05-24T00:00:00Z"
---

<summary>
- `factory.go` creates `http.DefaultClient` (no timeout) for `PrPoster` and `ReviewVerifier`
- `bitbucket/client.go` and `steps_gh_token.go` correctly use `&http.Client{Timeout: ...}`
- A stalled GitHub API connection can hang goroutines indefinitely with no timeout
- Fix: use a scoped `http.Client` with a 15-second timeout for GitHub poster/verifier
</summary>

<objective>
Add a 15-second timeout to the `http.Client` used by `PrPoster` and `ReviewVerifier` in `agent/pr-reviewer/pkg/factory/factory.go`, matching the pattern already used by `bitbucket/client.go` and `steps_gh_token.go`.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-security-linting.md` in `~/.claude/plugins/marketplaces/coding/docs/` — `#nosec` usage, `http.DefaultClient` vs custom client.

Files to read before making changes:
- `agent/pr-reviewer/pkg/factory/factory.go` — full file; understand `CreatePrPoster` (~line 138) and `CreateReviewVerifier` (~line 144)
- `agent/pr-reviewer/pkg/bitbucket/client.go` — understand the existing `&http.Client{Timeout: 30 * time.Second}` pattern
- `agent/pr-reviewer/pkg/steps_gh_token.go` — understand the existing `&http.Client{Timeout: 10 * time.Second}` pattern
</context>

<requirements>
**Execute steps in order. Run `make test` after the fix. Run `make precommit` only at the final step.**

1. **Add scoped HTTP client with timeout in `agent/pr-reviewer/pkg/factory/factory.go`**

   Change `CreatePrPoster` and `CreateReviewVerifier` to create a scoped client instead of using `http.DefaultClient`:

   ```go
   func CreatePrPoster(token, botLogin string, currentDateTime libtime.CurrentDateTimeGetter) prpkg.PrPoster {
       return githubposter.NewPrPoster(
           &http.Client{Timeout: 15 * time.Second},
           token,
           botLogin,
           currentDateTime,
       )
   }

   func CreateReviewVerifier(token, botLogin string) prpkg.ReviewVerifier {
       return githubposter.NewReviewVerifier(
           &http.Client{Timeout: 15 * time.Second},
           token,
           botLogin,
       )
   }
   ```

   Note: If `NewPrPoster` now requires `currentDateTime` (from the time injection prompt), pass it. If not yet applied, add the `libtime.CurrentDateTimeGetter` parameter in this prompt as well.

   Add `"time"` to the import block if not already present.

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
- Only change `agent/pr-reviewer/pkg/factory/factory.go`
- Do NOT commit — dark-factory handles git
- Use `&http.Client{Timeout: 15 * time.Second}` — consistent with project security hardening
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
