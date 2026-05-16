---
status: completed
summary: Introduced composable TaskCreationFilter chain in watcher/github/pkg/filter/ with DraftFilter, BotAuthorFilter, and slice composite, replacing ShouldSkipPR; wired through factory and main with no behavior change; added docs/watcher-decision-chains.md and CHANGELOG entry.
container: code-reviewer-075-task-creation-filter-chain
dark-factory-version: dev
created: "2026-05-02T13:30:00Z"
queued: "2026-05-02T11:34:54Z"
started: "2026-05-02T11:34:56Z"
completed: "2026-05-02T11:42:20Z"
---

<summary>
- The watcher's existing PR filtering is currently a single `ShouldSkipPR` function combining draft and bot-author checks; this is fine for two filters but does not generalize as more filter dimensions are added
- Introduce a composable `TaskCreationFilter` interface so future filter additions (WIP title, age, archived repo, etc.) follow a consistent pattern instead of growing the single `ShouldSkipPR` signature
- Split semantically distinct concerns: this chain answers "should we create a vault task at all?" — separate from the existing trust gate which answers "should we auto-process or route to human review?"
- Filters are composed via a slice type implementing the same interface; the chain returns true ("skip this PR") if ANY filter votes skip (the watcher's existing semantics)
- Existing draft and bot-author checks are extracted into `DraftFilter` and `BotAuthorFilter` leaves with no behavior change
- A new `docs/watcher-decision-chains.md` documents the two-chain architecture (TaskCreationFilter + TrustGate) so future filter additions land in the right chain
- Pure refactor + interface introduction: existing tests must pass without modification; new tests cover the interface, leaves, and slice-typed composite
</summary>

<objective>
Refactor the watcher's draft/bot filtering into a composable `TaskCreationFilter` interface following the Filter pattern (`go-filter-pattern.md`). Split into two leaves (`DraftFilter`, `BotAuthorFilter`) and a slice-typed composite. Wire through factory + watcher with no behavior change. Document the two-chain architecture in a new project doc.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-filter-pattern.md` from coding plugin (`~/.claude/plugins/marketplaces/coding/docs/`) — the canonical filter pattern (predicate-style, interface + leaves + composite, idiomatic in this repo).
Read `go-functional-composition-pattern.md` from coding plugin — generic Interface + Func + List trio.
Read `go-error-wrapping-guide.md` from coding plugin — `errors.Wrapf`, never `fmt.Errorf`.
Read `go-testing-guide.md` from coding plugin — Ginkgo v2 + Gomega + Counterfeiter conventions.
Read `go-factory-pattern.md` from coding plugin — factory composition (zero logic, only wiring).

Files to read before changing anything:
- `watcher/github/pkg/filter.go` — current `ShouldSkipPR`, `IsBotAuthor`, `ParseBotAllowlist`, `ParseTrustedAuthors`
- `watcher/github/pkg/filter_test.go` — existing tests for ShouldSkipPR + IsBotAuthor
- `watcher/github/pkg/watcher.go` — `ShouldSkipPR` call site at line ~135 inside `Poll`/`processPRs`
- `watcher/github/pkg/factory/factory.go` — `CreateWatcher` signature; takes `botAllowlist []string`
- `watcher/github/main.go` — `ParseBotAllowlist` call site, `factory.CreateWatcher` call site
- `watcher/github/pkg/githubclient.go` — `PullRequest` struct shape

Key facts (verified):
- `ShouldSkipPR(pr, botAllowlist)` is called once at `watcher.go:135` inside the per-PR loop
- Existing tests in `filter_test.go` cover both filters
- The trust framework lives in a separate package (`watcher/github/pkg/trust/`) — this refactor does NOT touch it
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Create `watcher/github/pkg/filter/filter.go`** (new package `filter`):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package filter implements the TaskCreationFilter chain — predicates
   // that decide whether a vault task should be created for a PR at all.
   //
   // This is one of two decision chains in the watcher:
   //
   //   1. TaskCreationFilter (this package): "should we create a task?"
   //   2. trust.Trust (sibling package):     "given we create a task,
   //                                          should it auto-process or
   //                                          route to human_review?"
   //
   // See docs/watcher-decision-chains.md for the full split rationale.
   package filter

   import "github.com/bborbe/maintainer/watcher/github-pr/pkg"

   //counterfeiter:generate -o ../mocks/task_creation_filter.go --fake-name TaskCreationFilter . TaskCreationFilter

   // TaskCreationFilter decides whether a single PR should be skipped
   // (no vault task created). Implementations return true to skip.
   type TaskCreationFilter interface {
       // Skip returns true if the PR should be excluded from task creation.
       Skip(pr pkg.PullRequest) bool
   }

   // TaskCreationFilterFunc adapts a function to the TaskCreationFilter
   // interface (function-as-implementation, useful for inline filters).
   type TaskCreationFilterFunc func(pr pkg.PullRequest) bool

   // Skip implements TaskCreationFilter for the function adapter.
   func (f TaskCreationFilterFunc) Skip(pr pkg.PullRequest) bool {
       return f(pr)
   }

   // TaskCreationFilters is a slice composite: skip if ANY member votes skip.
   // An empty slice never skips (no filters configured = process every PR).
   type TaskCreationFilters []TaskCreationFilter

   // Skip returns true if any contained filter votes skip. Iteration is
   // short-circuit on first hit (filters are pure predicates with no
   // side effects, so there is no audit-trail concern).
   func (fs TaskCreationFilters) Skip(pr pkg.PullRequest) bool {
       for _, f := range fs {
           if f.Skip(pr) {
               return true
           }
       }
       return false
   }
   ```

2. **Create `watcher/github/pkg/filter/draft_filter.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter

   import "github.com/bborbe/maintainer/watcher/github-pr/pkg"

   // NewDraftFilter returns a filter that skips draft PRs (the author
   // is signaling "not ready for review"). GitHub's draft state is the
   // primary signal; literal "WIP" prefixes in the title are handled
   // by a separate WIP filter (see wip_title_filter.go when added).
   func NewDraftFilter() TaskCreationFilter {
       return &draftFilter{}
   }

   type draftFilter struct{}

   func (f *draftFilter) Skip(pr pkg.PullRequest) bool {
       return pr.IsDraft
   }
   ```

3. **Create `watcher/github/pkg/filter/bot_author_filter.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter

   import "github.com/bborbe/maintainer/watcher/github-pr/pkg"

   // NewBotAuthorFilter returns a filter that skips PRs whose author
   // matches any entry in the configured allowlist (exact match).
   // An empty allowlist never skips.
   func NewBotAuthorFilter(allowlist []string) TaskCreationFilter {
       return &botAuthorFilter{allowlist: allowlist}
   }

   type botAuthorFilter struct {
       allowlist []string
   }

   func (f *botAuthorFilter) Skip(pr pkg.PullRequest) bool {
       for _, entry := range f.allowlist {
           if pr.AuthorLogin == entry {
               return true
           }
       }
       return false
   }
   ```

4. **Create `watcher/github/pkg/filter/suite_test.go`** (test bootstrap):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter_test

   import (
       "testing"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
   )

   func TestFilter(t *testing.T) {
       RegisterFailHandler(Fail)
       RunSpecs(t, "watcher/github/pkg/filter Suite")
   }
   ```

5. **Create `watcher/github/pkg/filter/filter_test.go`**:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter_test

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg"
       "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
   )

   var _ = Describe("DraftFilter", func() {
       It("skips draft PRs", func() {
           f := filter.NewDraftFilter()
           Expect(f.Skip(pkg.PullRequest{IsDraft: true})).To(BeTrue())
       })
       It("does not skip non-draft PRs", func() {
           f := filter.NewDraftFilter()
           Expect(f.Skip(pkg.PullRequest{IsDraft: false})).To(BeFalse())
       })
   })

   var _ = Describe("BotAuthorFilter", func() {
       It("skips PR whose author matches an allowlist entry", func() {
           f := filter.NewBotAuthorFilter([]string{"dependabot[bot]", "renovate[bot]"})
           Expect(f.Skip(pkg.PullRequest{AuthorLogin: "dependabot[bot]"})).To(BeTrue())
           Expect(f.Skip(pkg.PullRequest{AuthorLogin: "renovate[bot]"})).To(BeTrue())
       })
       It("does not skip PR whose author is not in the allowlist", func() {
           f := filter.NewBotAuthorFilter([]string{"dependabot[bot]"})
           Expect(f.Skip(pkg.PullRequest{AuthorLogin: "alice"})).To(BeFalse())
       })
       It("never skips when allowlist is empty", func() {
           f := filter.NewBotAuthorFilter(nil)
           Expect(f.Skip(pkg.PullRequest{AuthorLogin: "alice"})).To(BeFalse())
           Expect(f.Skip(pkg.PullRequest{AuthorLogin: "dependabot[bot]"})).To(BeFalse())
       })
   })

   var _ = Describe("TaskCreationFilters composite", func() {
       It("returns false when slice is empty (vacuous — no filters configured)", func() {
           var fs filter.TaskCreationFilters
           Expect(fs.Skip(pkg.PullRequest{})).To(BeFalse())
       })
       It("returns true if any member votes skip", func() {
           fs := filter.TaskCreationFilters{
               filter.NewDraftFilter(),
               filter.NewBotAuthorFilter([]string{"alice"}),
           }
           Expect(fs.Skip(pkg.PullRequest{IsDraft: true})).To(BeTrue())
           Expect(fs.Skip(pkg.PullRequest{AuthorLogin: "alice"})).To(BeTrue())
       })
       It("returns false when no member votes skip", func() {
           fs := filter.TaskCreationFilters{
               filter.NewDraftFilter(),
               filter.NewBotAuthorFilter([]string{"alice"}),
           }
           Expect(fs.Skip(pkg.PullRequest{IsDraft: false, AuthorLogin: "bob"})).To(BeFalse())
       })
       It("supports the function adapter", func() {
           fs := filter.TaskCreationFilters{
               filter.TaskCreationFilterFunc(func(pr pkg.PullRequest) bool {
                   return pr.AuthorLogin == "evil"
               }),
           }
           Expect(fs.Skip(pkg.PullRequest{AuthorLogin: "evil"})).To(BeTrue())
           Expect(fs.Skip(pkg.PullRequest{AuthorLogin: "alice"})).To(BeFalse())
       })
   })
   ```

6. **Wire counterfeiter generation** for the new `TaskCreationFilter` interface. The `//counterfeiter:generate` directive on the interface (step 1) is read by the package's `go:generate` line. Add a `doc.go` with the package-level `go:generate` directive (mirror the pattern from `watcher/github/pkg/trust/`):

   Create `watcher/github/pkg/filter/doc.go`:
   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package filter implements the TaskCreationFilter chain.
   package filter

   //go:generate go run github.com/maxbrunsfeld/counterfeiter/v6@v6.12.2 -generate
   ```

   Then run:
   ```bash
   cd watcher/github && go generate ./pkg/filter/...
   ```

   This produces `watcher/github/pkg/mocks/task_creation_filter.go` for use by sibling tests in later prompts. If `pkg/trust/` uses a different counterfeiter pattern, mirror it instead.

7. **Update `watcher/github/pkg/watcher.go`** — replace direct `ShouldSkipPR` use:

   - Add field to the `watcher` struct: `taskCreationFilter filter.TaskCreationFilter`
   - Add parameter to `NewWatcher`: `taskCreationFilter filter.TaskCreationFilter`
   - Replace the existing `ShouldSkipPR(pr, w.botAllowlist)` call with `w.taskCreationFilter.Skip(pr)`
   - Remove the `botAllowlist` field and parameter from `watcher` / `NewWatcher` (now lives inside the filter)
   - Add the import `"github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"`

8. **Update `watcher/github/pkg/factory/factory.go`** — `CreateWatcher` signature:

   - Replace the `botAllowlist []string` parameter with `taskCreationFilter filter.TaskCreationFilter`
   - Caller (main.go) builds the composite; factory just wires it through
   - Add the import for the new filter package

9. **Update `watcher/github/main.go`**:

   ```go
   // After parsing BotAllowlist (already happens), build the composite:
   botAllowlist := pkg.ParseBotAllowlist(a.BotAllowlist)
   taskCreationFilter := filter.TaskCreationFilters{
       filter.NewDraftFilter(),
       filter.NewBotAuthorFilter(botAllowlist),
   }
   // Pass taskCreationFilter to factory.CreateWatcher in place of botAllowlist.
   ```

   Add the import for the new filter package.

10. **Delete `ShouldSkipPR` and `IsBotAuthor` from `watcher/github/pkg/filter.go`** — both are now in the new package and superseded by the interface implementations. Keep `ParseBotAllowlist` and `ParseTrustedAuthors` (still used by main.go for parsing env vars).

11. **Update `watcher/github/pkg/filter_test.go`** — remove the tests for `ShouldSkipPR` and `IsBotAuthor` (covered by the new `filter` package tests). Keep tests for `ParseBotAllowlist` and `ParseTrustedAuthors`.

12. **Update existing watcher tests** at `watcher/github/pkg/watcher_test.go` — `NewWatcher` callers must pass a `filter.TaskCreationFilter` instead of `botAllowlist`. For tests that previously relied on bot-allowlist behavior, construct the equivalent composite explicitly. For tests that don't care, pass `filter.TaskCreationFilters{}` (empty composite — never skips).

13. **Create `docs/watcher-decision-chains.md`** with this exact content:

    ```markdown
    # Watcher Decision Chains

    The `watcher/github` service makes two distinct decisions per PR. They live in
    separate, composable chains so future filter additions land in the right place.

    ## Chain 1: TaskCreationFilter

    **Question:** "Should we create a vault task for this PR at all?"

    **Behavior on skip:** PR is silently ignored — no vault task, no audit trail.
    Use this chain for PRs that are pure noise: drafts, bots, abandoned work,
    work-in-progress that the author has explicitly marked as not ready.

    **Pattern:** Filter (predicate, slice composite, "skip if any votes skip").
    See `go-filter-pattern.md` and `go-boolean-combinator-pattern.md`.

    **Package:** `watcher/github/pkg/filter/`

    **Current leaves:**

    | Leaf | Skips when |
    |------|-----------|
    | `DraftFilter` | `pr.IsDraft == true` |
    | `BotAuthorFilter` | `pr.AuthorLogin` matches the configured bot allowlist |

    **Future leaves (planned):**

    - `WIPTitleFilter` — title starts with `WIP:` or `WIP ` (work-in-progress signal)
    - `AgeFilter` — PR last updated more than `MAX_PR_AGE` ago (abandoned)
    - `ArchivedRepoFilter` — repo archived (no reviewer cares)

    ## Chain 2: TrustGate

    **Question:** "Given we create a task, should it auto-process or route to
    human_review?"

    **Behavior on untrusted:** Task is created with `phase: human_review` and
    `status: todo`, body explains why. Reviewer can promote to `phase: in_progress`
    if they decide to proceed.

    **Pattern:** Boolean combinator (`And` / `Or` / `Not` over `Trust` interface).
    See `go-boolean-combinator-pattern.md`.

    **Package:** `watcher/github/pkg/trust/`

    **Current leaves:**

    | Leaf | Trusted when |
    |------|-------------|
    | `AuthorAllowlist` | `pr.AuthorLogin` matches the configured trusted-authors list (exact byte match) |

    **Future leaves (planned):**

    - `IsCollaborator` — author is a repo collaborator (queries GitHub API)
    - `RepoAllowlist` — PR target repo is in the configured allowlist
    - `RequiredLabel` — PR has a specific opt-in label (`ok-to-review`)

    ## Decision: which chain does my new filter go in?

    Ask: "Do I want this PR to be visible to a human reviewer at all?"

    - **No, it's pure noise** → TaskCreationFilter (skip)
    - **Yes, but I want a human to confirm before auto-processing** → TrustGate

    Two examples that have come up:

    | Filter | Chain | Why |
    |--------|-------|-----|
    | "WIP" in title | TaskCreationFilter | Author explicitly says "not ready" — same intent as draft |
    | "PR > 90 days old" | TaskCreationFilter | Almost certainly abandoned; reviewer attention not warranted |
    | "Author not in collaborator list" | TrustGate | Visible task with reviewer override; trust is the human's call |
    | "Repo not in allowlist" | TrustGate (or TaskCreationFilter, depending on noise tolerance) | If the operator wants to be aware of out-of-scope PRs, gate them; if they're pure spam, skip them |

    The two chains are independent. A PR that survives TaskCreationFilter then
    runs through TrustGate; both must agree before the task is auto-processed.
    ```

14. **Update `CHANGELOG.md`** at the repo root — add an entry under `## Unreleased` (or create `## Unreleased` if absent, directly above the most recent `## vX.Y.Z` heading):

    ```markdown
    - refactor(watcher): introduce composable `TaskCreationFilter` chain in `watcher/github/pkg/filter/` (interface + `DraftFilter`/`BotAuthorFilter` leaves + slice composite). Replaces the single `ShouldSkipPR` function. No behavior change. Adds `docs/watcher-decision-chains.md` documenting the split between TaskCreationFilter and TrustGate.
    ```

15. **Run `make precommit`** in `watcher/github/`:

    ```bash
    cd watcher/github && make precommit
    ```

    Must pass with exit code 0.

</requirements>

<constraints>
- Only edit files under `watcher/github/`, `docs/watcher-decision-chains.md`, and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Do NOT change the watcher's external behavior — same PRs must be skipped before and after this refactor (verifiable by the existing watcher_test.go bot/draft cases)
- Do NOT touch the trust framework (`watcher/github/pkg/trust/`) — separate concern
- Do NOT change `pkg.PullRequest` shape, `pkg.SearchResult`, or `pkg.SearchPRs`
- Do NOT add the future leaves (`WIPTitleFilter`, `AgeFilter`, etc.) — they ship in sibling prompt 2
- Use `github.com/bborbe/errors` for any error wrapping (`errors.Wrapf`, `errors.Errorf`); never `fmt.Errorf`
- Use `github.com/bborbe/collection` if a set-of-strings helper is helpful; not strictly required for this prompt
- Constructor functions return interface, not concrete struct (per `go-factory-pattern.md`); concrete types stay private (lowercase)
- New tests follow Ginkgo v2 + Gomega + Counterfeiter conventions per `go-testing-guide.md`
- Empty `TaskCreationFilters{}` slice MUST return `false` from `Skip` (vacuous — no filters configured = process every PR). This is the documented behavior from `go-boolean-combinator-pattern.md` for filter-style chains where "skip" is the rejection vote (matches the `Or`/Filter intuition: empty Or = false).
- `make precommit` runs from `watcher/github/`, never from repo root
</constraints>

<verification>
cd watcher/github && make precommit

# Confirm new package exists and is referenced:
ls watcher/github/pkg/filter/
grep -n "TaskCreationFilter" watcher/github/pkg/watcher.go watcher/github/main.go watcher/github/pkg/factory/factory.go

# Confirm the deleted functions are gone:
grep -E "func ShouldSkipPR|func IsBotAuthor" watcher/github/pkg/filter.go
# Expected: no matches

# Confirm doc exists:
ls docs/watcher-decision-chains.md
head -5 docs/watcher-decision-chains.md

# Confirm CHANGELOG updated:
grep "TaskCreationFilter" CHANGELOG.md
</verification>
