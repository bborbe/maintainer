---
status: completed
summary: Injected libtime.CurrentDateTimeGetter into planningStep, checkoutExecutionStep, prPoster, and reviewVerifier; replaced all time.Now() calls with injected time source
container: maintainer-exec-171-review-agent-pr-reviewer-2-time-injection
dark-factory-version: v0.171.1-3-gd94f1fa
created: "2026-05-24T00:00:00Z"
queued: "2026-05-25T21:25:46Z"
started: "2026-05-25T21:25:47Z"
completed: "2026-05-25T21:43:18Z"
---

<summary>
- `steps_planning.go` calls `time.Now()` directly in production step logic
- `steps_checkout_execution.go` passes `time.Now()` as a parameter to `postAndRoute`
- `githubposter/poster.go` calls `time.Now()` in `PostLGTM` and `Post` methods
- `githubposter/verifier.go` calls `time.Now()` in `VerifyReview` method
- All should use injected `libtime.CurrentDateTimeGetter` for testability and consistency with project time-injection guidelines
</summary>

<objective>
Inject `libtime.CurrentDateTimeGetter` into step structs and githubposter components so that `time.Now()` is not called directly in production business logic.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `go-time-injection.md` in `~/.claude/plugins/marketplaces/coding/docs/` — libtime injection pattern, `DateTime` type, `SetNow()` in tests.
Read `go-testing-guide.md` in `~/.claude/plugins/marketplaces/coding/docs/` — Ginkgo/Gomega, coverage ≥80%.

Files to read before making changes:
- `agent/pr-reviewer/pkg/steps_planning.go` — full file; understand `planningStep` struct (~line 30) and `postLGTMAndDone` (~line 148)
- `agent/pr-reviewer/pkg/steps_checkout_execution.go` — full file; understand `checkoutExecutionStep` struct (~line 30) and `postAndRoute` (~line 268)
- `agent/pr-reviewer/pkg/githubposter/poster.go` — full file; understand `prPoster` struct (~line 22), `PostLGTM` (~line 61), `Post` (~line 78)
- `agent/pr-reviewer/pkg/githubposter/verifier.go` — full file; understand `VerifyReview` (~line 56)
- `agent/pr-reviewer/pkg/factory/factory.go` — full file; understand existing `Create*` signatures to know how to add `CurrentDateTimeGetter` param
- `agent/pr-reviewer/pkg/steps_planning_test.go` — understand how planning step is tested
- `agent/pr-reviewer/pkg/steps_checkout_execution_test.go` — understand how checkout execution step is tested
</context>

<requirements>
**Execute steps in order. Run `make test` after each step. Run `make precommit` only at the final step.**

1. **Add `currentDateTime libtime.CurrentDateTimeGetter` field to `planningStep` in `agent/pr-reviewer/pkg/steps_planning.go`**

   Add to the struct:
   ```go
   type planningStep struct {
       prPoster           prpkg.PrPoster
       claudeRunner      claudelib.ClaudeRunner
       prURL             prurl.PRURL
       ghToken           string
       botLogin          string
       repoAllowlist     *allowlist.RepoAllowlist
       currentDateTime   libtime.CurrentDateTimeGetter // injected time source
   }
   ```

   Update `newPlanningStep` constructor to accept `libtime.CurrentDateTimeGetter` and assign it.

   In `postLGTMAndDone` (~line 148), replace `time.Now()` with `s.currentDateTime.Now()`:
   ```go
   // Before:
   jobRunTime := time.Now()

   // After:
   jobRunTime := s.currentDateTime.Now()
   ```

   Add `"github.com/bborbe/time/libtime"` to the import block.

2. **Add `currentDateTime libtime.CurrentDateTimeGetter` field to `checkoutExecutionStep` in `agent/pr-reviewer/pkg/steps_checkout_execution.go`**

   Add to the struct and update `newCheckoutExecutionStep` constructor.

   In `postAndRoute` (~line 265), replace the direct `time.Now()` call with `s.currentDateTime.Now()`.

   Also in `resolvePRInfo` (~line 337) if it receives `jobRunTime` as a parameter and passes it to `buildDiagnosticBlock` (~line 404), ensure the `time.Time` value flows correctly — the field on the struct should be `time.Time` (for display formatting) but populated via the getter.

   Note: `buildDiagnosticBlock` uses `jobRunTime.UTC().Format(time.RFC3339)` for formatting — this is correct stdlib formatting of the converted value.

3. **Update factory functions that construct `planningStep` and `checkoutExecutionStep`**

   In `factory.go`, update `CreatePlanningStep` and `CreateCheckoutExecutionStep` (or whatever they are named) to accept and pass `libtime.CurrentDateTimeGetter` to the constructors.

4. **Update `prPoster` and `reviewVerifier` in `githubposter/poster.go` and `verifier.go`**

   Add `currentDateTime libtime.CurrentDateTimeGetter` to `prPoster` struct:
   ```go
   type prPoster struct {
       httpClient         *http.Client
       token             string
       botLogin          string
       currentDateTime   libtime.CurrentDateTimeGetter
   }
   ```

   Update `NewPrPoster` constructor to accept `libtime.CurrentDateTimeGetter`:
   ```go
   func NewPrPoster(httpClient *http.Client, token, botLogin string, currentDateTime libtime.CurrentDateTimeGetter) *prPoster {
       return &prPoster{
           httpClient:       httpClient,
           token:           token,
           botLogin:        botLogin,
           currentDateTime: currentDateTime,
       }
   }
   ```

   In `PostLGTM` (~line 61) and `Post` (~line 78), replace `time.Now()` with `p.currentDateTime.Now()`.

   Do the same for `VerifyReview` in `verifier.go`.

5. **Update `CreatePrPoster` and `CreateReviewVerifier` in `factory.go`**

   Add `libtime.CurrentDateTimeGetter` parameter and pass it to the constructors.

6. **Update all call sites** that construct these types via the factory.

7. **Run `make test`** to verify compilation and tests pass:

   ```bash
   cd agent/pr-reviewer && make test
   ```

   For any test that calls step methods with a nil time getter or default value, inject `libtime.NewCurrentDateTime()` in the test `BeforeEach`.

8. **Run `make precommit`** for final validation:

   ```bash
   cd agent/pr-reviewer && make precommit
   ```
</requirements>

<constraints>
- Only change files in `agent/pr-reviewer/pkg/` and `agent/pr-reviewer/pkg/factory/`
- Do NOT commit — dark-factory handles git
- All `time.Now()` calls in production business logic must be replaced with injected `currentDateTime.Now()`
- `main.go` entry point may still use `libtime.NewCurrentDateTime()` — that is correct per guidelines
- Existing tests must still pass
- Use `errors.Wrap`/`errors.Errorf` from `github.com/bborbe/errors` — never `fmt.Errorf`
- Coverage ≥80% for changed packages
</constraints>

<verification>
cd agent/pr-reviewer && make precommit
</verification>
