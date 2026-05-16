---
status: completed
summary: Added WIPTitleFilter and AgeFilter to the TaskCreationFilter chain, extended filter.PR with Title/UpdatedAt fields, wired MAX_PR_AGE env var into main.go with fail-fast negative validation, and added full test coverage for both new filter leaves.
container: code-reviewer-076-wip-and-age-filters
dark-factory-version: dev
created: "2026-05-02T13:35:00Z"
queued: "2026-05-02T11:34:54Z"
started: "2026-05-02T11:42:22Z"
completed: "2026-05-02T11:46:25Z"
---

<summary>
- Add two new leaves to the `TaskCreationFilter` chain (established by sibling prompt 1): a filter for work-in-progress PRs (title prefix) and a filter for stale PRs (last-updated age)
- Both address noise: WIP PRs are author-signaled "not ready"; old PRs are almost certainly abandoned and not worth reviewer attention
- WIP detection uses a fixed regex anchored at title start (`^WIP[:\s]`) — matches `WIP:` prefix and `WIP ` (space-separated) but NOT `working in progress` mid-title
- Age threshold defaults to 90 days, operator-configurable; an empty value disables the filter (preserves prior behavior)
- The age filter compares against the watcher's `startTime` (captured once at process start, already plumbed through factory→watcher→cursor), avoiding any new `CurrentDateTimeGetter` injection. Sub-day clock skew is irrelevant at the 90-day granularity this filter operates on.
- All time values use `libtime` types per `go-time-injection.md` (`libtime.DateTime`, `libtime.Duration`); the `MAX_PR_AGE` env var is parsed via `libtime.ParseDuration` which supports the extended `"30d"` / `"1w2d"` syntax in addition to stdlib durations
- New filters added to the default chain in `main.go`; pre-existing fields on `pkg.PullRequest` (`Title`, `UpdatedAt`) are already populated by `SearchPRs` — no struct changes
</summary>

**Sibling-prompt dependency:** This prompt assumes prompt `1-task-creation-filter-chain.md` has been applied. It uses the `TaskCreationFilter` interface and `TaskCreationFilters` composite from `watcher/github/pkg/filter/`.

<objective>
Ship two new TaskCreationFilter leaves so the watcher skips work-in-progress and abandoned PRs without creating vault tasks. Wire them into the default chain via the existing factory.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-filter-pattern.md` from coding plugin (`~/.claude/plugins/marketplaces/coding/docs/`).
Read `go-error-wrapping-guide.md` from coding plugin.
Read `go-testing-guide.md` from coding plugin.

Files to read before changing anything:
- `watcher/github/pkg/filter/filter.go` — `TaskCreationFilter` interface + `TaskCreationFilters` composite (from sibling prompt 1)
- `watcher/github/pkg/filter/draft_filter.go` and `bot_author_filter.go` — leaf shape to mirror
- `watcher/github/pkg/githubclient.go` — `PullRequest` struct (already has `Title string` and `UpdatedAt libtime.DateTime`; do NOT add or change these)
- `watcher/github/pkg/factory/factory.go` — `CreateWatcher` already accepts `startTime libtime.DateTime`; the new chain composition lives in `main.go` (factory stays a pass-through)
- `watcher/github/main.go` — env var parsing pattern (see `BotAllowlist`, `TrustedAuthors`); `startTime` is already captured via `libtime.NewCurrentDateTime().Now()`

Reference docs (project):
- `docs/watcher-decision-chains.md` — confirms WIP and Age both belong in TaskCreationFilter (chain 1)

Key facts (verified against the codebase):
- `pkg.PullRequest.Title` is `string`; `pkg.PullRequest.UpdatedAt` is `libtime.DateTime` (NOT stdlib `time.Time`)
- `SearchPRs` already populates both fields from the GitHub Search response — no struct or mapping changes needed
- `startTime libtime.DateTime` is already captured once in `main.go` at startup and threaded through `factory.CreateWatcher` → `NewWatcher` → `LoadCursor`. Reusing it for the age filter avoids any new `CurrentDateTimeGetter` injection
- `libtime.ParseDuration(ctx, "30d")` supports extended duration syntax (days, weeks); use it instead of stdlib `time.ParseDuration` for `MAX_PR_AGE`
- `libtime.DateTime.Before(...)` and `libtime.DateTime.Add(...)` work with `HasTime` / `HasDuration` interfaces — stdlib `time.Duration` satisfies `HasDuration`, but prefer `libtime.Duration` for type consistency
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Confirm pre-conditions** by reading `watcher/github/pkg/githubclient.go`:
   - `Title string` — present at line ~24 ✓
   - `UpdatedAt libtime.DateTime` — present at line ~29 ✓
   - Both are populated by `SearchPRs` already
   - **Do NOT modify the struct or mapping** — they're correct as-is

2. **Create `watcher/github/pkg/filter/wip_title_filter.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter

   import (
       "regexp"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg"
   )

   // wipTitleRegexp matches titles whose first three characters are "WIP"
   // followed by a colon or whitespace separator. Anchored at title start;
   // intentionally NOT case-insensitive (the convention is uppercase).
   //
   // Matches: "WIP: foo", "WIP foo"
   // Does NOT match: "wip: foo" (lowercase), "Working in progress" (mid-title),
   //                 "WIPER: foo" (no separator after WIP)
   var wipTitleRegexp = regexp.MustCompile(`^WIP[:\s]`)

   // NewWIPTitleFilter returns a filter that skips PRs whose title starts
   // with "WIP:" or "WIP ". The author has explicitly signaled the PR is
   // not ready for review.
   func NewWIPTitleFilter() TaskCreationFilter {
       return &wipTitleFilter{}
   }

   type wipTitleFilter struct{}

   func (f *wipTitleFilter) Skip(pr pkg.PullRequest) bool {
       return wipTitleRegexp.MatchString(pr.Title)
   }
   ```

3. **Create `watcher/github/pkg/filter/age_filter.go`** — uses the watcher's existing `startTime libtime.DateTime` (captured once at startup, threaded through factory). No `CurrentDateTimeGetter` injection needed.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter

   import (
       libtime "github.com/bborbe/time"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg"
   )

   // NewAgeFilter returns a filter that skips PRs whose UpdatedAt is older
   // than (referenceTime - maxAge). A zero maxAge disables the filter.
   //
   // referenceTime is the watcher's startTime — captured once at process
   // start and threaded through factory.CreateWatcher. This avoids any
   // CurrentDateTimeGetter injection: a watcher pod's lifetime is bounded
   // by its restart cadence (hours), and the age filter's granularity is
   // days, so sub-day clock drift is irrelevant. If a watcher's pod runs
   // for longer than the maxAge window, the operator should restart it
   // (or the kubelet will via probes) — caching startTime is correct.
   func NewAgeFilter(maxAge libtime.Duration, referenceTime libtime.DateTime) TaskCreationFilter {
       return &ageFilter{
           maxAge:        maxAge,
           referenceTime: referenceTime,
       }
   }

   type ageFilter struct {
       maxAge        libtime.Duration
       referenceTime libtime.DateTime
   }

   func (f *ageFilter) Skip(pr pkg.PullRequest) bool {
       if f.maxAge <= 0 {
           return false
       }
       cutoff := f.referenceTime.Add(-f.maxAge)
       return pr.UpdatedAt.Before(cutoff)
   }
   ```

   The constructor parameter type is `libtime.Duration` (NOT stdlib `time.Duration`) for consistency with the rest of the codebase. `libtime.Duration` is an alias of stdlib `time.Duration` in current bborbe/time, so basic arithmetic works the same; if the alias changes, this file will surface the type error at compile time.

4. **Add `MAX_PR_AGE` to `main.go`'s `application` struct** (after the `TrustedAuthors` field):

   ```go
   // MaxPRAge is the maximum age of a PR (by updated_at) before it is skipped
   // entirely. Empty or zero disables the filter. Format: Go duration string.
   MaxPRAge string `required:"false" arg:"max-pr-age" env:"MAX_PR_AGE" usage:"Skip PRs older than this (Go duration; empty disables)" default:"2160h"`
   ```

5. **In `main.go`'s `Run` method**, parse the duration and build the new default chain.

   Critical type notes:
   - `libtime.ParseDuration(ctx, raw)` returns `(*libtime.Duration, error)` — pointer, not value
   - `libtime.ParseDurationDefault` silently swallows parse errors (returns the default), which masks operator typos. We want fail-fast — so use `ParseDuration` + manual empty-string handling
   - Empty string causes `ParseDuration` to error (it goes through `parse.ParseString`)

   ```go
   var maxAge libtime.Duration
   if a.MaxPRAge != "" {
       parsed, err := libtime.ParseDuration(ctx, a.MaxPRAge)
       if err != nil {
           return errors.Wrapf(ctx, err, "parse MAX_PR_AGE")
       }
       if parsed != nil {
           maxAge = *parsed
       }
   }
   if maxAge < 0 {
       return errors.Errorf(ctx, "MAX_PR_AGE must not be negative, got %s", maxAge)
   }
   ```

   - Empty `MAX_PR_AGE` → `maxAge = 0` → filter disabled (verified: `if f.maxAge <= 0 { return false }` in `age_filter.go`).
   - The struct's `default:"2160h"` (90 days) populates `a.MaxPRAge` before this runs, so the empty branch only fires if the operator explicitly sets `MAX_PR_AGE=""`.
   - Garbage input → fail-fast via wrapped error (operator sees the parse failure at startup, not silent disable).

   The watcher's `startTime` is already captured earlier in `Run` (`startTime := libtime.NewCurrentDateTime().Now()` at ~line 70). Reuse it.

   Compose the chain with all four leaves (existing two + two new):

   ```go
   taskCreationFilter := filter.TaskCreationFilters{
       filter.NewDraftFilter(),
       filter.NewBotAuthorFilter(botAllowlist),
       filter.NewWIPTitleFilter(),
       filter.NewAgeFilter(maxAge, startTime),
   }
   ```

6. **Add tests** at `watcher/github/pkg/filter/wip_title_filter_test.go` and `age_filter_test.go`. Use Ginkgo + Gomega. Cover:

   - **WIPTitleFilter**: skips `"WIP: foo"`, `"WIP fix the thing"`; does NOT skip `"wip: foo"`, `"Wip: foo"`, `"Working in progress"`, `"feature: WIP cleanup"`, empty title.
   - **AgeFilter**: with `maxAge = 0` never skips (regardless of `UpdatedAt`); with `maxAge = 90 days` and a fixed `referenceTime`, a PR updated 100 days ago is skipped, 89 days ago is not, exactly 90 days ago is at the boundary (use `Before` semantics — strictly older). No mock needed: pass concrete `libtime.DateTime` values to the constructor.

   Construct test fixtures via `libtime.ParseDateTime(ctx, "2026-05-02T12:00:00Z")` (returns `*libtime.DateTime`) and `libtime.ParseDuration(ctx, "90d")` (returns `*libtime.Duration`). Both return pointers — dereference once to get values for the constructor: `*ref`, `*dur`. Or use `libtime.ParseDurationDefault(ctx, "90d", 0)` which returns a value directly.

7. **Update `CHANGELOG.md`** under `## Unreleased`:

   ```markdown
   - feat(watcher): add `WIPTitleFilter` (skip PRs with `WIP:` / `WIP ` title prefix) and `AgeFilter` (skip PRs older than `MAX_PR_AGE`, default 90 days). Both extend the `TaskCreationFilter` chain. Configurable via `MAX_PR_AGE` env var (libtime extended duration; empty disables age filter, negative rejected at startup).
   ```

8. **Run `make precommit`** in `watcher/github/`:

   ```bash
   cd watcher/github && make precommit
   ```

</requirements>

<constraints>
- Only edit files under `watcher/github/` and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Do NOT touch the trust framework (`watcher/github/pkg/trust/`)
- Do NOT change `TaskCreationFilter` interface or `TaskCreationFilters` composite — they are frozen by sibling prompt 1
- WIP regex is `^WIP[:\s]` — case-sensitive (uppercase WIP only); the convention in this codebase is uppercase WIP, lowercase mid-title is treated as content not a marker
- `MAX_PR_AGE` default is `2160h` (90 days, expressible as `90d` via `libtime.ParseDuration`). Empty value disables the filter for backwards compatibility. Negative values are rejected at startup with a clear error (fail-fast)
- `time.Now()` must NOT be called directly inside `ageFilter` — uses the watcher's pre-captured `startTime libtime.DateTime` per `go-time-injection.md`. No new injection needed (sub-day clock skew is irrelevant at 90-day granularity)
- Use `github.com/bborbe/errors` (`errors.Wrapf`, `errors.Errorf`); never `fmt.Errorf`
- `make precommit` runs from `watcher/github/`
- Existing tests must keep passing
</constraints>

<verification>
cd watcher/github && make precommit

# Confirm new filters exist:
grep -E "NewWIPTitleFilter|NewAgeFilter" watcher/github/pkg/filter/*.go

# Confirm wired into main.go:
grep -E "MAX_PR_AGE|MaxPRAge|NewWIPTitleFilter|NewAgeFilter" watcher/github/main.go

# Confirm Title and UpdatedAt fields populated on PullRequest:
grep -E "Title|UpdatedAt" watcher/github/pkg/githubclient.go | head -5
</verification>
