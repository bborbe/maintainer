---
spec: ["044-github-release-watcher-implementation"]
status: draft
created: "2026-05-27T20:38:37Z"
---

<summary>
- Filter logic is already implemented in the skeleton — this prompt adds the missing Ginkgo unit tests
- Three new test files cover `EmptyUnreleasedFilter`, `AutoReleaseFilter`, `SHAUnchangedFilter` with the spec-named acceptance criteria
- Existing `repo_allowlist_filter.go` is carried verbatim from `watcher/github-pr` and needs only a smoke test (the underlying `lib/repoallowlist` already has its own test suite)
- Tests use the in-package `Release` filter type (no import cycle on `pkg.Release`)
- `make precommit` must pass cleanly when finished
</summary>

<objective>
Add Ginkgo v2 + Gomega unit tests for the four filters in `watcher/github-release/pkg/filter/`. The filter implementations themselves already exist; this prompt's deliverable is purely the test suite covering the named spec acceptance criteria. No production code changes.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md` — Ginkgo v2 + Gomega, table tests via `DescribeTable`, external `_test` packages
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-filter-pattern.md` — the chain semantics this package implements

Read these files end-to-end:
- `watcher/github-release/pkg/filter/filter.go` — confirms the local `Release` struct fields (`RepoKey`, `HeadSHA`, `UnreleasedBullets`, `AutoRelease`) and the `TaskCreationFilter` interface
- `watcher/github-release/pkg/filter/empty_unreleased_filter.go` — predicate body already complete
- `watcher/github-release/pkg/filter/auto_release_filter.go` — predicate body already complete
- `watcher/github-release/pkg/filter/sha_unchanged_filter.go` — predicate body + local `CursorReader` interface already complete
- `watcher/github-release/pkg/filter/repo_allowlist_filter.go` — wraps `lib/repoallowlist.IsAllowed`; the parser + allow-all-on-empty behavior are already coded
- `watcher/github-release/pkg/filter/suite_test.go` — Ginkgo runner already exists; reuse it

Reference test layouts (read for style):
- `/workspace/watcher/github-pr/pkg/filter/` — sibling filter package tests (different predicates but same Ginkgo `DescribeTable` shape)

Counterfeiter / mock note: the filter package has a `//counterfeiter:generate -o ../mocks/task_creation_filter.go --fake-name TaskCreationFilter . TaskCreationFilter` directive in `filter.go` line 17. The destination `pkg/mocks/` directory does NOT yet exist — the existing root-level `mocks/` directory is a placeholder from the github-pr clone (per spec AC: "All counterfeiter `-o` directives in the skeleton resolve to `pkg/mocks/*.go`"). `make generate` will create `pkg/mocks/task_creation_filter.go` as part of the precommit run.
</context>

<requirements>

**Execute steps in order. Run `cd watcher/github-release && make test` after step 4. Run `make precommit` only at the final step.**

1. **Create `watcher/github-release/pkg/filter/empty_unreleased_filter_test.go`** as package `filter_test`:

   ```go
   package filter_test

   import (
       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
   )
   ```

   Inside `Describe("filter.EmptyUnreleasedFilter", ...)` add these `It` blocks (use the exact `It` text where called out — these are spec acceptance criteria):

   - `It("EmptyUnreleasedFilter skips when UnreleasedBullets is 0")` — instantiate via `filter.NewEmptyUnreleasedFilter()`, call `f.Skip(filter.Release{UnreleasedBullets: 0})`, assert `true`.
   - `It("EmptyUnreleasedFilter does not skip when UnreleasedBullets is 1")` — assert `f.Skip(filter.Release{UnreleasedBullets: 1})` is `false`.
   - `It("EmptyUnreleasedFilter does not skip when UnreleasedBullets is large")` — assert `f.Skip(filter.Release{UnreleasedBullets: 42})` is `false`.

2. **Create `watcher/github-release/pkg/filter/auto_release_filter_test.go`** as package `filter_test`:

   `Describe("filter.AutoReleaseFilter", ...)` with:

   - `It("AutoReleaseFilter skips when AutoRelease is true")` — `filter.NewAutoReleaseFilter().Skip(filter.Release{AutoRelease: true})` → `true`.
   - `It("AutoReleaseFilter does not skip when AutoRelease is false")` — `filter.NewAutoReleaseFilter().Skip(filter.Release{AutoRelease: false})` → `false`.
   - `It("AutoReleaseFilter does not skip the zero-value Release")` — `filter.NewAutoReleaseFilter().Skip(filter.Release{})` → `false` (zero-value `AutoRelease` is `false`).

3. **Create `watcher/github-release/pkg/filter/sha_unchanged_filter_test.go`** as package `filter_test`. Define a minimal in-test `CursorReader` fake — the production `pkg.cursorReader` adapter lives in `pkg/watcher.go` and depends on `*pkg.Cursor`, which would create an import cycle (filter cannot import pkg). The in-test fake is the right boundary:

   ```go
   type fakeCursorReader struct {
       data map[string]string
   }

   func (f *fakeCursorReader) LastSeenSHA(repoKey string) string {
       return f.data[repoKey]
   }
   ```

   `Describe("filter.SHAUnchangedFilter", ...)` with:

   - `It("SHAUnchangedFilter skips when LastSeenSHA equals HeadSHA")` — populate fake with `{"github.com/bborbe/docker-utils": "d630ef3"}`, build filter via `filter.NewSHAUnchangedFilter(&fakeCursorReader{data: data})`, call `Skip(filter.Release{RepoKey: "github.com/bborbe/docker-utils", HeadSHA: "d630ef3"})`, assert `true`.
   - `It("SHAUnchangedFilter emits when LastSeenSHA differs from HeadSHA")` — same fake, call `Skip(filter.Release{RepoKey: "github.com/bborbe/docker-utils", HeadSHA: "different-sha"})`, assert `false`.
   - `It("SHAUnchangedFilter emits when repo is unseen by the cursor")` — empty fake `data`, call `Skip(filter.Release{RepoKey: "github.com/bborbe/new-repo", HeadSHA: "abc123"})`, assert `false` (the cursor returns `""` for unknown keys; `"" != "abc123"` so NOT skip).
   - `It("SHAUnchangedFilter handles empty HeadSHA against unseen repo")` — empty fake, `Skip(filter.Release{RepoKey: "x", HeadSHA: ""})` — note `"" == ""` so this filter WOULD skip. Add this as a documentation test: assert `true`. This is intentional — an empty HeadSHA never reaches the filter chain in production (the watcher fail-closes upstream), so this behavior is acceptable. Comment on the `It` body: `// degenerate case — production path never passes empty HeadSHA through; documented for posterity`.

4. **Create `watcher/github-release/pkg/filter/repo_allowlist_filter_test.go`** as package `filter_test`. The underlying `lib/repoallowlist` is already tested in `lib/repoallowlist/repoallowlist_test.go` — this is a smoke test for the wrapper, not a re-test of the matching logic.

   `Describe("filter.RepoAllowlistFilter", ...)` with:

   - `It("RepoAllowlistFilter allows everything when allowlist is empty")` — `filter.NewRepoAllowlistFilter(nil).Skip(filter.Release{RepoKey: "github.com/anyone/anything"})` → `false`. Also test `filter.NewRepoAllowlistFilter([]string{}).Skip(...)` → `false`.
   - `It("RepoAllowlistFilter skips repo outside the allowlist")` — `filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/docker-utils"}).Skip(filter.Release{RepoKey: "github.com/bborbe/other-repo"})` → `true`.
   - `It("RepoAllowlistFilter does not skip repo present in the allowlist")` — `filter.NewRepoAllowlistFilter([]string{"github.com/bborbe/docker-utils"}).Skip(filter.Release{RepoKey: "github.com/bborbe/docker-utils"})` → `false`.

   Also add a `Describe("filter.ParseRepoAllowlist", ...)` block (sibling test for the helper):
   - `It("ParseRepoAllowlist returns nil on empty input")` — `entries, err := filter.ParseRepoAllowlist(ctx, ""); Expect(err).NotTo(HaveOccurred()); Expect(entries).To(BeNil())`.
   - `It("ParseRepoAllowlist trims whitespace and skips empty entries")` — input `"github.com/bborbe/a, github.com/bborbe/b , , github.com/bborbe/c"`, expected `[]string{"github.com/bborbe/a", "github.com/bborbe/b", "github.com/bborbe/c"}` (length 3, no blank entries).

   Use `ctx := context.Background()` inside the test (test-only — safe).

5. **Composite chain test** in `watcher/github-release/pkg/filter/filter_test.go` (NEW file, package `filter_test`). This tests the `TaskCreationFilters` slice composite that `main.go` wires up:

   `Describe("filter.TaskCreationFilters", ...)` with:

   - `It("TaskCreationFilters returns false when every filter votes false")` — chain `EmptyUnreleased + AutoRelease`, pass `filter.Release{UnreleasedBullets: 3, AutoRelease: false}`, assert `chain.Skip(...)` is `false`.
   - `It("TaskCreationFilters returns true on first filter that votes true")` — chain `EmptyUnreleased + AutoRelease`, pass `filter.Release{UnreleasedBullets: 0, AutoRelease: false}`, assert `true` (empty-unreleased wins).
   - `It("TaskCreationFilters returns true when later filter votes true")` — chain `EmptyUnreleased + AutoRelease`, pass `filter.Release{UnreleasedBullets: 3, AutoRelease: true}`, assert `true` (auto-release wins).
   - `It("TaskCreationFilters with empty slice never skips")` — `filter.TaskCreationFilters{}.Skip(filter.Release{})` is `false`. Spec invariant: empty chain = process every Release.

6. **Run unit tests**:
   ```bash
   cd watcher/github-release && make test
   ```

7. **Run full precommit**:
   ```bash
   cd watcher/github-release && make precommit
   ```
   This regenerates counterfeiter mocks. The `//counterfeiter:generate` directive in `pkg/filter/filter.go` (line 17) emits to `pkg/mocks/task_creation_filter.go` — if `pkg/mocks/` does not exist yet, `make generate` creates it. Verify with `ls pkg/mocks/task_creation_filter.go` after.

</requirements>

<constraints>
- Mirror `watcher/github-pr` Go patterns verbatim: Ginkgo v2 + Gomega; external `_test` packages.
- Do NOT change the filter implementation files — they are already complete and constitute the spec contract.
- `lib/repoallowlist` carried verbatim — no domain logic change in `repo_allowlist_filter.go`; test the wrapper only.
- The local `Release` struct in `filter.go` exists specifically to break the import cycle. Use `filter.Release{...}` in tests; do NOT introduce a `pkg.Release` import from the filter test file.
- No `context.Background()` in production paths — tests are exempt.
- No `fmt.Errorf` in production paths — N/A here; this prompt adds tests only.
- Do NOT commit — dark-factory handles git.
- Do NOT modify any file outside the five new test files under `watcher/github-release/pkg/filter/`.
</constraints>

<verification>
```bash
cd watcher/github-release

# Five test files exist
ls pkg/filter/empty_unreleased_filter_test.go \
   pkg/filter/auto_release_filter_test.go \
   pkg/filter/sha_unchanged_filter_test.go \
   pkg/filter/repo_allowlist_filter_test.go \
   pkg/filter/filter_test.go

# Spec-named acceptance criteria present verbatim
grep -F "EmptyUnreleasedFilter skips when UnreleasedBullets is 0" pkg/filter/empty_unreleased_filter_test.go
grep -F "AutoReleaseFilter skips when AutoRelease is true"        pkg/filter/auto_release_filter_test.go
grep -F "SHAUnchangedFilter skips when LastSeenSHA equals HeadSHA" pkg/filter/sha_unchanged_filter_test.go
grep -F "SHAUnchangedFilter emits when LastSeenSHA differs from HeadSHA" pkg/filter/sha_unchanged_filter_test.go

# Filter production files untouched (no TODO removal needed — already complete)
grep -c "TODO" pkg/filter/*.go
# Expected: 0 (skeleton file bodies already done)

# Tests pass
make test

# Mock regen lands in pkg/mocks (not root-level mocks/)
ls pkg/mocks/task_creation_filter.go

# Full precommit
make precommit
```
</verification>
