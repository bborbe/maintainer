---
status: completed
spec: [013-repo-allowlist-stage-isolation]
summary: Added REPO_ALLOWLIST env var to watcher/github with RepoAllowlistFilter leaf, RepoKey field on filter.PR, host-qualified env file updates, and full Ginkgo/Gomega test coverage.
container: code-reviewer-079-spec-013-watcher-allowlist
dark-factory-version: v0.147.2-1-g30ba42f
created: "2026-05-03T16:30:00Z"
queued: "2026-05-03T16:58:25Z"
started: "2026-05-03T16:58:56Z"
completed: "2026-05-03T17:04:20Z"
branch: dark-factory/repo-allowlist-stage-isolation
---

<summary>
- The GitHub PR watcher gains an optional `REPO_ALLOWLIST` env var and matching CLI flag that restricts which repos the watcher will publish tasks for
- Empty allowlist (default `""`) preserves today's behavior exactly — every PR matched by `REPO_SCOPE` produces a task as before
- Non-empty allowlist limits task creation to PRs whose `github.com/owner/repo` key appears in the list; all others are silently skipped (incrementing the existing skip metric)
- A malformed allowlist entry (missing slash, wrong shape) causes a startup failure with a clear operator-facing log naming the offending entry
- Whitespace-only and empty entries (e.g. trailing comma) are silently stripped during parse; remaining valid entries are used
- The configured allowlist size is logged at startup (count only, not contents)
- `dev.env` and `prod.env` are updated from the current non-host-qualified `bborbe/code-reviewer` to the host-qualified `github.com/bborbe/maintainer`
- `filter.PR` gains a `RepoKey` field (`"github.com/owner/repo"`) that is populated by the watcher before calling `taskCreationFilter.Skip()`; existing filters are unaffected since the new field defaults to empty string
- A new `RepoAllowlistFilter` leaf joins the existing `TaskCreationFilter` chain in `main.go`; it is the last filter so all other existing filters run first
- Full Ginkgo/Gomega test coverage for the new filter leaf and parse function
</summary>

<objective>
Add `REPO_ALLOWLIST` filtering to the GitHub PR watcher (`watcher/github`) so operators can restrict task creation to a configured set of host-qualified repos (`host/owner/repo`). This is the watcher layer of spec-013's defense-in-depth stage-isolation design. An empty allowlist is allow-all and preserves today's behavior.
</objective>

<context>
Read CLAUDE.md for project conventions.
Read `go-filter-pattern.md` from coding plugin (`~/.claude/plugins/marketplaces/coding/docs/`).
Read `go-error-wrapping-guide.md` from coding plugin.
Read `go-testing-guide.md` from coding plugin.

Files to read before making any changes:

- `watcher/github/main.go` — full file; understand `application` struct, `Run()` method, existing env var wiring patterns (see `BotAllowlist`, `MaxPRAge`, `BackfillDuration`), and how `taskCreationFilter` is assembled
- `watcher/github/pkg/filter/filter.go` — `PR` struct and `TaskCreationFilter` interface; you will add `RepoKey string` to `PR`
- `watcher/github/pkg/filter/age_filter.go` — leaf shape to mirror for `RepoAllowlistFilter`
- `watcher/github/pkg/filter/wip_title_filter.go` — leaf shape to mirror
- `watcher/github/pkg/filter/filter_test.go` — existing tests; must continue to pass unchanged
- `watcher/github/pkg/watcher.go` — look for the `taskCreationFilter.Skip(filter.PR{...})` call; you will add `RepoKey: "github.com/" + pr.Owner + "/" + pr.Repo` to the `filter.PR{}` literal
- `watcher/github/pkg/githubclient.go` — confirm `PullRequest.Owner` and `PullRequest.Repo` are strings (they are already populated by `SearchPRs`)
- `watcher/github/pkg/watcher_test.go` — read to see if any test asserts the exact `filter.PR` argument passed to the mock's `Skip()` call (if so, update those assertions to include `RepoKey`)
- `dev.env` and `prod.env` — current values of `REPO_ALLOWLIST`; you will update them to the host-qualified form

Key facts (verified against the codebase):
- `filter.PR` is defined in `watcher/github/pkg/filter/filter.go` with four fields: `AuthorLogin`, `IsDraft`, `Title`, `UpdatedAt`
- `watcher.go` constructs `filter.PR{AuthorLogin: pr.AuthorLogin, IsDraft: pr.IsDraft, Title: pr.Title, UpdatedAt: pr.UpdatedAt}` at ~line 136
- `pkg.PullRequest.Owner` and `pkg.PullRequest.Repo` are populated by `SearchPRs` from the repo URL
- The GitHub watcher always talks to `github.com`; hardcoding `"github.com"` as the host in `RepoKey` is correct and intentional for this module
- `taskCreationFilter` is a `filter.TaskCreationFilters` slice assembled in `main.go` at ~line 118
- Both `dev.env` and `prod.env` currently have `REPO_ALLOWLIST=bborbe/code-reviewer` (non-host-qualified); both must be updated to `REPO_ALLOWLIST=github.com/bborbe/maintainer`
- This prompt touches ONLY `watcher/github/`. The agent side (`agent/pr-reviewer/`) is handled by the sibling prompt 2.
</context>

<requirements>

**Execute steps in this order. Run `make precommit` only in the final step.**

1. **Add `RepoKey string` to `filter.PR`** in `watcher/github/pkg/filter/filter.go`.

   Current struct (after `UpdatedAt libtime.DateTime`):
   ```go
   type PR struct {
       AuthorLogin string
       IsDraft     bool
       Title       string
       UpdatedAt   libtime.DateTime
   }
   ```

   Replace with (add `RepoKey` as the last field; no comment needed since the field name is self-describing):
   ```go
   type PR struct {
       AuthorLogin string
       IsDraft     bool
       Title       string
       UpdatedAt   libtime.DateTime
       RepoKey     string
   }
   ```

2. **Update the `taskCreationFilter.Skip(filter.PR{...})` call** in `watcher/github/pkg/watcher.go` to populate `RepoKey`.

   Find the block that constructs `filter.PR{...}` (at ~line 136). Add `RepoKey: "github.com/" + pr.Owner + "/" + pr.Repo` to it:

   ```go
   if w.taskCreationFilter.Skip(
       filter.PR{
           AuthorLogin: pr.AuthorLogin,
           IsDraft:     pr.IsDraft,
           Title:       pr.Title,
           UpdatedAt:   pr.UpdatedAt,
           RepoKey:     "github.com/" + pr.Owner + "/" + pr.Repo,
       },
   ) {
   ```

   Check `watcher/github/pkg/watcher_test.go` for any test that asserts the exact `filter.PR` value passed to the mock's `Skip()`. If found, update those literals to also include `RepoKey`. (Most tests inject the mock without asserting the argument, so this step may be a no-op.)

3. **Create `watcher/github/pkg/filter/repo_allowlist_filter.go`** with the parse function and filter leaf:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter

   import (
       "context"
       "regexp"
       "strings"

       "github.com/bborbe/errors"
   )

   // repoAllowlistEntryPattern validates a single host-qualified repo entry.
   // Required shape: host/owner/repo (three slash-delimited segments, no trailing .git).
   var repoAllowlistEntryPattern = regexp.MustCompile(
       `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`,
   )

   // ParseRepoAllowlist parses a comma-separated allowlist string into a slice
   // of validated host-qualified repo keys ("host/owner/repo").
   //
   // Empty string and unset env var both return (nil, nil) — allow-all.
   // Whitespace-only entries and entries produced by trailing commas are silently
   // dropped. Any entry that does not match the required shape causes an error.
   func ParseRepoAllowlist(ctx context.Context, raw string) ([]string, error) {
       if raw == "" {
           return nil, nil
       }
       var result []string
       for _, entry := range strings.Split(raw, ",") {
           entry = strings.TrimSpace(entry)
           if entry == "" {
               continue
           }
           if !repoAllowlistEntryPattern.MatchString(entry) {
               return nil, errors.Errorf(
                   ctx,
                   "repo allowlist entry %q does not match required format host/owner/repo (pattern: ^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$)",
                   entry,
               )
           }
           result = append(result, entry)
       }
       return result, nil
   }

   // NewRepoAllowlistFilter returns a TaskCreationFilter that skips PRs whose
   // RepoKey is not in the allowlist. An empty allowlist never skips (allow-all).
   func NewRepoAllowlistFilter(allowlist []string) TaskCreationFilter {
       return &repoAllowlistFilter{allowlist: allowlist}
   }

   type repoAllowlistFilter struct {
       allowlist []string
   }

   func (f *repoAllowlistFilter) Skip(pr PR) bool {
       if len(f.allowlist) == 0 {
           return false
       }
       for _, entry := range f.allowlist {
           if pr.RepoKey == entry {
               return false
           }
       }
       return true
   }
   ```

4. **Create `watcher/github/pkg/filter/repo_allowlist_filter_test.go`** using Ginkgo v2 + Gomega:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package filter_test

   import (
       "context"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-pr/pkg/filter"
   )

   var _ = Describe("ParseRepoAllowlist", func() {
       var ctx context.Context
       BeforeEach(func() { ctx = context.Background() })

       It("returns nil for empty string (allow-all)", func() {
           result, err := filter.ParseRepoAllowlist(ctx, "")
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(BeNil())
       })

       It("parses a single valid entry", func() {
           result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/maintainer")
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(Equal([]string{"github.com/bborbe/maintainer"}))
       })

       It("parses multiple valid entries", func() {
           result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/foo,github.com/bborbe/bar")
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(ConsistOf("github.com/bborbe/foo", "github.com/bborbe/bar"))
       })

       It("strips whitespace around entries", func() {
           result, err := filter.ParseRepoAllowlist(ctx, " github.com/bborbe/foo , github.com/bborbe/bar ")
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(ConsistOf("github.com/bborbe/foo", "github.com/bborbe/bar"))
       })

       It("silently drops empty entries from trailing comma", func() {
           result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/foo,")
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(Equal([]string{"github.com/bborbe/foo"}))
       })

       It("silently drops whitespace-only entries", func() {
           result, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/foo, ,github.com/bborbe/bar")
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(ConsistOf("github.com/bborbe/foo", "github.com/bborbe/bar"))
       })

       It("returns error for entry with only two segments (no host)", func() {
           _, err := filter.ParseRepoAllowlist(ctx, "bborbe/code-reviewer")
           Expect(err).To(HaveOccurred())
           Expect(err.Error()).To(ContainSubstring("bborbe/code-reviewer"))
       })

       It("returns error for entry with only one segment", func() {
           _, err := filter.ParseRepoAllowlist(ctx, "code-reviewer")
           Expect(err).To(HaveOccurred())
       })

       It("returns error for entry with four segments", func() {
           _, err := filter.ParseRepoAllowlist(ctx, "github.com/bborbe/foo/extra")
           Expect(err).To(HaveOccurred())
       })

       It("returns error for empty-string-after-trim entry that is otherwise malformed", func() {
           // A single comma produces only empty entries — all dropped, no error.
           result, err := filter.ParseRepoAllowlist(ctx, ",")
           Expect(err).NotTo(HaveOccurred())
           Expect(result).To(BeNil())
       })
   })

   var _ = Describe("RepoAllowlistFilter", func() {
       It("never skips when allowlist is empty", func() {
           f := filter.NewRepoAllowlistFilter(nil)
           Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/foo"})).To(BeFalse())
           Expect(f.Skip(filter.PR{RepoKey: ""})).To(BeFalse())
       })

       It("does not skip a PR whose RepoKey is on the allowlist", func() {
           f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/maintainer"})
           Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/maintainer"})).To(BeFalse())
       })

       It("skips a PR whose RepoKey is NOT on the allowlist", func() {
           f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/maintainer"})
           Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/other-repo"})).To(BeTrue())
       })

       It("skips a PR with an empty RepoKey when the allowlist is non-empty", func() {
           f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/maintainer"})
           Expect(f.Skip(filter.PR{RepoKey: ""})).To(BeTrue())
       })

       It("matches exactly — prefix match is not a match", func() {
           f := filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/code"})
           Expect(f.Skip(filter.PR{RepoKey: "github.com/bborbe/maintainer"})).To(BeTrue())
       })
   })
   ```

5. **Add `REPO_ALLOWLIST` to the `application` struct** in `watcher/github/main.go`.

   After the `BackfillDuration` field, add:
   ```go
   RepoAllowlist string `required:"false" arg:"repo-allowlist" env:"REPO_ALLOWLIST" usage:"Comma-separated host-qualified repo allowlist (host/owner/repo format); empty means allow-all"`
   ```

6. **Parse `REPO_ALLOWLIST` in the `Run()` method** in `watcher/github/main.go`.

   After the `parseBackfillDuration` block (and before the `taskCreationFilter` assembly block), add:

   ```go
   repoAllowlist, err := filter.ParseRepoAllowlist(ctx, a.RepoAllowlist)
   if err != nil {
       return err
   }
   if len(repoAllowlist) == 0 {
       glog.V(2).Infof("repo-allowlist count=0 (allow-all)")
   } else {
       glog.V(2).Infof("repo-allowlist count=%d", len(repoAllowlist))
   }
   ```

7. **Add `filter.NewRepoAllowlistFilter(repoAllowlist)` to the `taskCreationFilter` slice** in `watcher/github/main.go`.

   Current `taskCreationFilter` assembly (~line 118):
   ```go
   taskCreationFilter := filter.TaskCreationFilters{
       filter.NewDraftFilter(),
       filter.NewBotAuthorFilter(botAllowlist),
       filter.NewWIPTitleFilter(),
       filter.NewAgeFilter(maxAge, startTime),
   }
   ```

   Replace with:
   ```go
   taskCreationFilter := filter.TaskCreationFilters{
       filter.NewDraftFilter(),
       filter.NewBotAuthorFilter(botAllowlist),
       filter.NewWIPTitleFilter(),
       filter.NewAgeFilter(maxAge, startTime),
       filter.NewRepoAllowlistFilter(repoAllowlist),
   }
   ```

   The repo allowlist filter runs last — all other noise filters (draft, bot, WIP, age) run first so that the allowlist log shows the repo is being filtered, not that the PR was draft.

8. **Update `dev.env`** — change the `REPO_ALLOWLIST` line from:
   ```
   export REPO_ALLOWLIST=bborbe/code-reviewer
   ```
   to:
   ```
   export REPO_ALLOWLIST=github.com/bborbe/maintainer
   ```

9. **Update `prod.env`** — same change:
   ```
   export REPO_ALLOWLIST=github.com/bborbe/maintainer
   ```

10. **Update `CHANGELOG.md`** — add to the `## Unreleased` section (or create it above the most recent `## vX.Y.Z`):

    ```markdown
    - feat(watcher): add `REPO_ALLOWLIST` env var (comma-separated `host/owner/repo` entries) that restricts task creation to configured repos. Empty allowlist is allow-all (preserves today's behavior). Malformed entries cause startup failure with a clear log. Adds `RepoAllowlistFilter` leaf to the `TaskCreationFilter` chain. Updated `dev.env` and `prod.env` to host-qualified form (`github.com/bborbe/maintainer`).
    ```

11. **Run `make precommit`** in `watcher/github/`:

    ```bash
    cd watcher/github && make precommit
    ```

</requirements>

<constraints>
- Only edit files under `watcher/github/`, `dev.env`, `prod.env`, and `CHANGELOG.md`
- Do NOT commit — dark-factory handles git
- Do NOT introduce a shared library between watcher and agent modules — the allowlist parse logic is a watcher-local copy (agent gets its own copy in sibling prompt 2)
- Do NOT change `REPO_SCOPE` semantics, the Kafka command schema, or the vault task structure
- Do NOT change any existing filter leaves (`DraftFilter`, `BotAuthorFilter`, `WIPTitleFilter`, `AgeFilter`) — additive only
- The `TaskCreationFilter` interface and `TaskCreationFilters` composite in `filter.go` remain unchanged except for the `RepoKey` field addition to `PR`
- Empty allowlist MUST behave identically to today (allow-all): `NewRepoAllowlistFilter(nil)` and `NewRepoAllowlistFilter([]string{})` both return `false` from `Skip` for any PR
- The regex for allowlist entry validation is exactly `^[a-zA-Z0-9.-]+/[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$` — three slash-delimited segments, no trailing `.git`
- Whitespace entries and empty entries (from trailing comma) are silently dropped — this is the specified behavior, not an error
- Use `github.com/bborbe/errors` (`errors.Wrapf`, `errors.Errorf`); never `fmt.Errorf`
- Existing tests must pass without modification (the new `RepoKey` field has a zero-value `""` which causes `RepoAllowlistFilter` with an empty allowlist to never skip)
- `make precommit` runs from `watcher/github/`, never at repo root
</constraints>

<verification>
cd watcher/github && make precommit

# Confirm new filter file exists:
ls watcher/github/pkg/filter/repo_allowlist_filter.go watcher/github/pkg/filter/repo_allowlist_filter_test.go

# Confirm RepoKey field added to filter.PR:
grep -n "RepoKey" watcher/github/pkg/filter/filter.go

# Confirm RepoKey populated in watcher.go:
grep -n "RepoKey" watcher/github/pkg/watcher.go

# Confirm wired in main.go:
grep -n "REPO_ALLOWLIST\|RepoAllowlist\|ParseRepoAllowlist\|NewRepoAllowlistFilter" watcher/github/main.go

# Confirm env files updated to host-qualified form:
grep "REPO_ALLOWLIST" dev.env prod.env
# Expected: both show github.com/bborbe/maintainer

# Confirm CHANGELOG updated:
grep -n "REPO_ALLOWLIST\|repo-allowlist\|RepoAllowlist" CHANGELOG.md
</verification>
