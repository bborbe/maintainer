---
status: completed
summary: Moved ParseBotAllowlist to pkg/filter.go, fixed syncProducer.Close() error logging, removed unused pollInterval parameter from CreateWatcher, and added ParseBotAllowlist tests in filter_test.go.
container: code-reviewer-055-review-watcher-github-fix-factory-business-logic
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-28T00:00:00Z"
queued: "2026-04-28T15:24:46Z"
started: "2026-04-28T18:00:55Z"
completed: "2026-04-28T18:05:32Z"
---

<summary>
- ParseBotAllowlist contains a for loop and conditionals — business logic that must not live in a factory file
- The factory guide requires zero logic in factory functions
- Moving it to pkg/filter.go puts it alongside the related ShouldSkipPR and IsBotAuthor functions
- The syncProducer.Close() error is silently discarded with "_ = err" — should be logged as a warning
- The factory package has no tests at all despite containing testable logic
- A factory_suite_test.go and factory_test.go are needed to cover ParseBotAllowlist after it moves
- The pollInterval parameter in CreateWatcher is accepted but never used inside the factory — remove it
</summary>

<objective>
Move `ParseBotAllowlist` from `pkg/factory/factory.go` to `pkg/filter.go`, log the `syncProducer.Close()` error instead of discarding it, remove the unused `pollInterval` parameter from `CreateWatcher`, and add a test suite with coverage for `ParseBotAllowlist` in its new location.
</objective>

<context>
Read `CLAUDE.md` for project conventions.

Files to read before making changes (read ALL first):
- `watcher/github/pkg/factory/factory.go` (full): `ParseBotAllowlist` (~lines 73-86), `syncProducer.Close()` cleanup (~lines 35-38), `pollInterval` parameter in `CreateWatcher`
- `watcher/github/pkg/filter.go` (full): `ShouldSkipPR` and `IsBotAuthor` — `ParseBotAllowlist` will be added here
- `watcher/github/pkg/filter_test.go` (full): existing tests for filter functions — pattern to follow for new tests
- `watcher/github/main.go`: `factory.ParseBotAllowlist` call site (~line 53), and `runPollLoop`/`pollInterval` usage
</context>

<requirements>
1. **Move `ParseBotAllowlist` from `pkg/factory/factory.go` to `watcher/github/pkg/filter.go`**:
   - Append the function to `filter.go` (after `ShouldSkipPR`)
   - Remove `ParseBotAllowlist` and its `strings` import from `factory.go`
   - Remove the `strings` import from `factory.go` if it is only used by `ParseBotAllowlist`

2. **Update `watcher/github/main.go`** (~line 53):
   - Change `factory.ParseBotAllowlist(a.BotAllowlist)` → `pkg.ParseBotAllowlist(a.BotAllowlist)`
   - Verify `pkg` is already imported; if not, add the import

3. **Fix `syncProducer.Close()` error handling in `pkg/factory/factory.go`** (~lines 36-39):
   - Replace `_ = err` with `glog.Warningf("close kafka sync producer: %v", err)`
   - Add `"github.com/golang/glog"` to imports if not already present

4. **Remove unused `pollInterval time.Duration` parameter from `CreateWatcher`** in factory.go:
   - Remove the parameter from the function signature
   - In `main.go`, the local `pollInterval` variable is still used by `runPollLoop` — keep it. Only remove `pollInterval` from the argument list passed to `factory.CreateWatcher`.
   - Remove the `"time"` import from `factory.go` if it is only used for this parameter

5. **Add tests for `ParseBotAllowlist` in `watcher/github/pkg/filter_test.go`** (its new location). The factory package becomes a no-logic wiring layer with no tests — do NOT create `pkg/factory/factory_suite_test.go` or `factory_test.go`.

   Append a `Describe("ParseBotAllowlist")` block to `pkg/filter_test.go` covering:
   - Empty string input → returns nil
   - Single entry → returns `[]string{"entry"}`
   - Multiple comma-separated entries → returns slice of all
   - Entries with leading/trailing whitespace → trimmed
   - Entries that are only whitespace after trimming → filtered out
   - Input with trailing comma → trailing empty entry filtered

7. Run `cd watcher/github && make test` — must pass.

8. Run `cd watcher/github && make precommit` — must exit 0.
</requirements>

<constraints>
- Only change files in `watcher/github/`
- Do NOT commit — dark-factory handles git
- Existing tests must still pass
- `factory.go` must contain zero loops, conditionals, or business logic after the change
- `ParseBotAllowlist` is moved to `pkg/filter.go` — it is NOT deleted
- Use `errors.Wrapf(ctx, err, "...")` from `github.com/bborbe/errors` — never `fmt.Errorf`
</constraints>

<verification>
cd watcher/github && grep -n "ParseBotAllowlist\|for.*parts\|strings.Split" pkg/factory/factory.go
# Expected: no matches (function moved out)

cd watcher/github && grep -n "func ParseBotAllowlist" pkg/filter.go
# Expected: one match

cd watcher/github && grep -n "pkg\.ParseBotAllowlist" main.go
# Expected: one match

cd watcher/github && grep -n "_ = err" pkg/factory/factory.go
# Expected: no matches (replaced with glog.Warningf)

cd watcher/github && grep -n "pollInterval" pkg/factory/factory.go
# Expected: no matches (parameter removed)

cd watcher/github && make precommit
</verification>
