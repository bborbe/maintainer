---
status: completed
summary: Added BACKFILL_DURATION env var (default 720h/30 days) that backdates the initial cursor on cold start by extracting parseBackfillDuration helper in main.go, backdating startTime before factory construction, and adding table-driven tests for all specified cases.
container: code-reviewer-077-backfill-cursor-on-cold-start
dark-factory-version: dev
created: "2026-05-02T13:40:00Z"
queued: "2026-05-02T11:34:54Z"
started: "2026-05-02T11:46:27Z"
completed: "2026-05-02T11:49:57Z"
---

<summary>
- The watcher's cursor (which controls the `updated:>=<since>` filter in the GitHub Search query) currently initializes to the watcher's startup time on cold start
- Practical effect on low-volume orgs: the very first poll after a fresh deploy returns ZERO PRs (no PR was updated AFTER the pod just started); operator has to wait for organic PR activity before the watcher does anything
- Fix: introduce a `BACKFILL_DURATION` env var (Go duration string). On cold start (no `cursor.json` yet), set the initial cursor to `now() - BACKFILL_DURATION` instead of `now()`. Once a cursor file exists, the env var is ignored — subsequent polls advance from the persisted cursor as today
- Default `BACKFILL_DURATION=720h` (30 days) — picks up recent PRs on first deploy without flooding (the goal's risk-mitigation rationale: avoid pulling years of historical PRs)
- Empty or zero duration preserves today's behavior (cursor = startup time)
- Negative durations are rejected at startup with a clear error (fail-fast convention)
- This is orthogonal to the filter chains: backfill controls WHICH PRs the search returns; filters then process the returned set
</summary>

<objective>
Make first-deploy useful by letting the watcher backfill recent PR history. Add a `BACKFILL_DURATION` env var (default 30 days) that sets the initial cursor to `now() - duration` on cold start.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-error-wrapping-guide.md` from coding plugin (`~/.claude/plugins/marketplaces/coding/docs/`).
Read `go-testing-guide.md` from coding plugin.
Read `go-time-injection.md` from coding plugin — `time.Now()` must NOT be called directly; inject `libtime.CurrentDateTimeGetter`.

Files to read before changing anything:
- `watcher/github/pkg/cursor.go` (or wherever the cursor lives — find it via `grep -rn "cursor" watcher/github/pkg/*.go`)
- `watcher/github/pkg/cursor_test.go` (if present)
- `watcher/github/main.go` — env var wiring (mirror `BotAllowlist`, `TrustedAuthors` patterns)
- `watcher/github/pkg/factory/factory.go` — `CreateWatcher` signature; cursor path is likely already passed
- `watcher/github/pkg/watcher.go` — initial-cursor-load logic in `Poll`/`NewWatcher` startup

Key facts to verify before editing (read the actual code):
- Where is the cursor file path computed? (likely `/data/cursor.json` per the StatefulSet manifest)
- Where does the watcher decide "cold start" vs "warm start"? (check for `os.Stat(cursorPath)` or equivalent)
- Is `libtime.CurrentDateTimeGetter` already injected? If yes, reuse. If not, accept it as a constructor parameter (do NOT call `time.Now()` directly)
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Locate the cursor initialization** by searching `watcher/github/pkg/`:

   ```bash
   grep -rn "cursor.json\|loadCursor\|Cursor{" watcher/github/pkg/*.go
   ```

   Read the relevant file(s) to understand the cold-start path. The cold-start signal is typically `os.IsNotExist(err)` after attempting to read the cursor file.

2. **Add `BACKFILL_DURATION` to `main.go`'s `application` struct** (after the `MaxPRAge` field if sibling prompt 2 has run, or after `TrustedAuthors` otherwise):

   ```go
   // BackfillDuration controls the initial cursor on cold start. On first
   // deploy (no cursor.json exists yet), the cursor is set to now() - this
   // value, which causes the first GitHub Search query to use
   // updated:>=<now-backfill> instead of updated:>=<now>. Once cursor.json
   // exists, this value is ignored.
   //
   // Empty or zero preserves today's behavior (cursor = startup time).
   // Negative values are rejected at startup.
   BackfillDuration string `required:"false" arg:"backfill-duration" env:"BACKFILL_DURATION" usage:"On cold start, backdate the initial cursor by this duration (Go duration; empty disables)" default:"720h"`
   ```

3. **Parse `BackfillDuration` in main.go's `Run` method**.

   Critical type notes:
   - `libtime.ParseDuration(ctx, raw)` returns `(*libtime.Duration, error)` — pointer, not value
   - `libtime.ParseDurationDefault` silently swallows parse errors, masking operator typos. Use explicit `ParseDuration` + manual empty-string handling for fail-fast behavior
   - Empty string causes `ParseDuration` to error (goes through `parse.ParseString`)

   ```go
   var backfillDuration libtime.Duration
   if a.BackfillDuration != "" {
       parsed, err := libtime.ParseDuration(ctx, a.BackfillDuration)
       if err != nil {
           return errors.Wrapf(ctx, err, "parse BACKFILL_DURATION")
       }
       if parsed != nil {
           backfillDuration = *parsed
       }
   }
   if backfillDuration < 0 {
       return errors.Errorf(ctx, "BACKFILL_DURATION must not be negative, got %s", backfillDuration)
   }
   ```

   `libtime.ParseDuration` accepts the extended syntax (`30d`, `1w2d3h`) in addition to stdlib durations, so operators can write `30d` instead of `720h`. The struct's `default:"720h"` populates `a.BackfillDuration` before this runs, so the empty branch only fires if the operator explicitly sets `BACKFILL_DURATION=""` (disables backfill, watcher uses startup-time cursor as before).

4. **Backdate `startTime` in `main.go`** before passing it to the factory. This is the simplest correct fix — `startTime` is already plumbed through `factory.CreateWatcher` → `NewWatcher` → `LoadCursor` and used as the cold-start cursor value. No factory or LoadCursor signature changes needed.

   In `main.go`'s `Run` method, replace:
   ```go
   startTime := libtime.NewCurrentDateTime().Now()
   ```

   With:
   ```go
   startTime := libtime.NewCurrentDateTime().Now()
   if backfillDuration > 0 {
       startTime = startTime.Add(-backfillDuration)
       glog.V(2).Infof("cursor cold-start backfilled by %s; initial=%s", backfillDuration, startTime)
   }
   ```

   Notes:
   - `libtime.DateTime.Add(libtime.Duration)` returns a new `libtime.DateTime`
   - The log line fires only when backfill is active, so default-config deploys stay quiet
   - `LoadCursor` already uses `startTime` as the initial cursor value when no cursor file exists; warm starts ignore `startTime` and load from file — so the env var is automatically only effective on cold start with no extra branching

5. (No factory or LoadCursor changes needed — see step 4 rationale.)

6. **Add tests**. Two scopes:

   - **`main.go` parsing logic** (small table test or similar): `""` → `0`; `"720h"` → `720h`; `"30d"` → equivalent of 720h; `"-1h"` → returned error contains "negative"; garbage → wrapped parse error.
   - **Existing `LoadCursor` tests** in `watcher/github/pkg/cursor_test.go` already cover cold/warm start behavior using the passed-in `startTime`; verify they still pass without modification (the change is upstream of LoadCursor).

7. **The struct's `default:"720h"` declares the default** — operators do not need to set `BACKFILL_DURATION` in `dev.env`/`prod.env` for the default to take effect. Skip env-file edits unless you want a non-default value.

8. **Update `CHANGELOG.md`** under `## Unreleased`:

   ```markdown
   - feat(watcher): add `BACKFILL_DURATION` env var (default 30 days) that backdates the initial cursor on cold start. First deploy now picks up PRs updated within the configured window instead of returning zero PRs until organic activity arrives. Once `cursor.json` exists, the env var is ignored.
   ```

9. **Run `make precommit`** in `watcher/github/`:

   ```bash
   cd watcher/github && make precommit
   ```

</requirements>

<constraints>
- Only edit files under `watcher/github/` and `CHANGELOG.md` (env files unchanged — struct default is sufficient)
- Do NOT commit — dark-factory handles git
- Do NOT change the cursor file format or location (still `/data/cursor.json` per the StatefulSet manifest)
- Do NOT touch the filter chains or trust framework
- Do NOT call `time.Now()` directly — use `libtime.CurrentDateTimeGetter` per `go-time-injection.md`
- Empty or zero `BACKFILL_DURATION` MUST preserve today's behavior (cursor = startup time)
- Negative `BACKFILL_DURATION` MUST cause startup to fail with a clear error (fail-fast)
- Once `cursor.json` exists, `BACKFILL_DURATION` is ignored — no surprise cursor rewinds on subsequent restarts
- Use `github.com/bborbe/errors` (`errors.Wrapf`, `errors.Errorf`); never `fmt.Errorf`
- `make precommit` runs from `watcher/github/`
- Existing tests must keep passing
</constraints>

<verification>
cd watcher/github && make precommit

# Confirm env var wiring:
grep -E "BACKFILL_DURATION|BackfillDuration" watcher/github/main.go dev.env prod.env

# Confirm parser + validation present:
grep -A 3 "BackfillDuration" watcher/github/main.go | grep -E "ParseDuration|negative"

# Confirm cold-start logic respects backfill (the change is in main.go, not pkg/):
grep -nE "backfillDuration|BackfillDuration|Add\\(-.*backfill" watcher/github/main.go
</verification>
