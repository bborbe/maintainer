---
status: committing
spec: [039-expand-watcher-github-build-org-wildcard]
summary: Created pkg/wildcard/ package with Expander, ResolvedAllowlist, and ListOwnerRepos method on GitHubClient interface; all tests pass
container: maintainer-exec-143-spec-039-wildcard-package
dark-factory-version: v0.169.0
created: "2026-05-24T10:00:00Z"
queued: "2026-05-24T10:05:07Z"
started: "2026-05-24T10:19:26Z"
completed: "2026-05-24T10:27:49Z"
---

<summary>
- Introduce a new `pkg/wildcard/` package inside `watcher/github-build` that resolves owner-level wildcard allowlist entries (e.g. `github.com/bborbe/*`) into concrete `host/owner/repo` entries.
- Add a `ListOwnerRepos(ctx, owner)` method to the existing `GitHubClient` interface that detects owner kind (User vs Organization) and lists non-archived, non-fork repositories via go-github v62.
- Provide a thread-safe `ResolvedAllowlist` type holding an atomic snapshot of the current resolved entry slice, refreshed once per hour by a background goroutine.
- Skip wildcard mechanics entirely (no goroutine, no API calls) when the allowlist contains zero wildcard entries — pure-literal behavior is byte-identical to today.
- Preserve last-known-good entries on per-entry refresh failure; only entries with a successful refresh in the current pass are overwritten.
- Emit one `glog.V(2)` log line per resolution naming the wildcard entry, resolved count, and source (`fresh` or `last-known-good`).
- Filter archived and fork repositories inline inside the list loop using `repo.GetArchived()` / `repo.GetFork()`.
- Unit-test the expander, the resolved-set semantics, the all-entries-fail fallback, and the once-per-hour refresh using a fake clock.
</summary>

<objective>
After this prompt, a new `pkg/wildcard/` package exists with: (1) an extended `pkg.GitHubClient` interface that can list an owner's eligible repositories; (2) an `Expander` that turns an allowlist with `host/owner/*` entries into concrete entries; (3) a `ResolvedAllowlist` that snapshots the current entry slice atomically and refreshes itself once an hour via a goroutine wired to `ctx.Done()`. The package compiles, all unit tests pass, no existing test regresses. Wiring into the two binary entry points happens in the sibling prompt.
</objective>

<context>
Read CLAUDE.md for project conventions.

**Files to read first (load real signatures before writing code):**
- `specs/in-progress/039-expand-watcher-github-build-org-wildcard.md` — full spec (Goal, Non-goals, Failure Modes, ACs).
- `watcher/github-build/pkg/githubclient.go` — current `GitHubClient` interface (5 methods); the new `ListOwnerRepos` method is added to this interface and implemented on the existing `*githubClient` struct that wraps `*gogithub.Client`.
- `watcher/github-build/pkg/watcher.go` lines 75-148 — the poll loop iterates `w.allowlist` directly. The expander OUTPUT must be a `[]string` of `host/owner/repo` entries compatible with `splitRepoKey` (3 segments).
- `watcher/github-build/pkg/filter/filter.go` and `filter/repo_allowlist_filter.go` — the filter receives the SAME allowlist (used by `repoallowlist.IsAllowed`). After expansion, concrete entries satisfy the literal-match branch of `IsAllowed` unchanged.
- `lib/repoallowlist/repoallowlist.go` — `classifyKind` (lines 83-101) returns kind `"wildcard"` iff `segments[2] == "*"` and the entry has exactly 3 segments. The expander reuses this convention: an entry is a wildcard iff its third slash-segment equals literal `*`.
- `watcher/github-build/pkg/suite_test.go` — existing Ginkgo v2 / Gomega harness (`package pkg_test`, `RunSpecs(t, "Pkg Suite", ...)`). The new `pkg/wildcard/` package gets its own sibling `suite_test.go`.
- `watcher/github-build/pkg/mocks/github_client.go` — counterfeiter-generated fake for `GitHubClient`. Adding a method to the interface will require regenerating; the package already has `//counterfeiter:generate` directives. Running `go generate ./...` after the interface change regenerates the fake.

**Verified go-github v62 signatures (from `/Users/bborbe/Documents/workspaces/go/pkg/mod/github.com/google/go-github/v62@v62.0.0/github/`):**

```go
// repos.go line 318 — list repos for a user
func (s *RepositoriesService) ListByUser(
    ctx context.Context, user string, opts *RepositoryListByUserOptions,
) ([]*Repository, *Response, error)

// repos.go line 294 — option struct (embeds ListOptions)
type RepositoryListByUserOptions struct {
    Type      string `url:"type,omitempty"`
    Sort      string `url:"sort,omitempty"`
    Direction string `url:"direction,omitempty"`
    ListOptions
}

// repos.go line 425 — list repos for an org
func (s *RepositoriesService) ListByOrg(
    ctx context.Context, org string, opts *RepositoryListByOrgOptions,
) ([]*Repository, *Response, error)

// repos.go line 404 — option struct (embeds ListOptions)
type RepositoryListByOrgOptions struct {
    Type      string `url:"type,omitempty"`
    Sort      string `url:"sort,omitempty"`
    Direction string `url:"direction,omitempty"`
    ListOptions
}

// users.go line 87 — get a user (used to detect owner kind)
func (s *UsersService) Get(
    ctx context.Context, user string,
) (*User, *Response, error)

// User.GetType() accessor exists (github-accessors.go line 24186);
// returns "Organization" or "User"
```

The `*Repository` accessors `GetArchived() bool`, `GetFork() bool`, and `GetName() string` are confirmed present (github-accessors.go lines 18562, 18746, 19026).

**Existing rate-limit pattern (mirror this):**

```go
// pkg/githubclient.go line 99-104 — established pattern for translating
// rate-limit errors to the package-local ErrRateLimited sentinel.
var rl *gogithub.RateLimitError
var arl *gogithub.AbuseRateLimitError
if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
    return nil, ErrRateLimited
}
```

`ErrRateLimited` is exported from the parent `pkg` package (`pkg/githubclient.go` line 20). The new method MUST use the same sentinel — do not introduce a new one.

**Cross-prompt boundary:** This prompt creates the package and tests it in isolation. The sibling prompt `spec-039-wire-binaries.md` wires the resolved allowlist into `main.go` + `cmd/run-once/main.go` and updates `NewWatcher` to consume snapshots. Do NOT touch `factory.go`, `watcher.go`, or either `main.go` in this prompt.

**Coding plugin docs (in-container paths):**
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-logging-guide.md`
</context>

<requirements>

1. **Extend `pkg.GitHubClient` interface in `watcher/github-build/pkg/githubclient.go`.**

   Add ONE new method to the existing interface (after `GetJobLog`):

   ```go
   // ListOwnerRepos returns the names of every non-archived, non-fork
   // repository owned by `owner`. Owner kind (User vs Organization) is
   // detected via GET /users/<owner>; the method then calls ListByUser
   // or ListByOrg respectively, paginating with PerPage=100 until done.
   // Returns (nil, ErrRateLimited) when rate-limited.
   // Returns (nil, err) for any other API error (network, 401/403, 404).
   // Returns ([]string{}, nil) when the owner has zero eligible repos.
   ListOwnerRepos(ctx context.Context, owner string) ([]string, error)
   ```

   Implement on the existing `*githubClient` type (do NOT introduce a new struct). Implementation outline:

   ```go
   func (c *githubClient) ListOwnerRepos(ctx context.Context, owner string) ([]string, error) {
       user, _, err := c.client.Users.Get(ctx, owner)
       if err != nil {
           var rl *gogithub.RateLimitError
           var arl *gogithub.AbuseRateLimitError
           if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
               return nil, ErrRateLimited
           }
           return nil, errors.Wrapf(ctx, err, "get user %s", owner)
       }

       names := make([]string, 0, 32)
       isOrg := user.GetType() == "Organization"
       page := 1
       for {
           var (
               repos []*gogithub.Repository
               resp  *gogithub.Response
               err   error
           )
           if isOrg {
               opts := &gogithub.RepositoryListByOrgOptions{
                   ListOptions: gogithub.ListOptions{PerPage: 100, Page: page},
               }
               repos, resp, err = c.client.Repositories.ListByOrg(ctx, owner, opts)
           } else {
               opts := &gogithub.RepositoryListByUserOptions{
                   ListOptions: gogithub.ListOptions{PerPage: 100, Page: page},
               }
               repos, resp, err = c.client.Repositories.ListByUser(ctx, owner, opts)
           }
           if err != nil {
               var rl *gogithub.RateLimitError
               var arl *gogithub.AbuseRateLimitError
               if stderrors.As(err, &rl) || stderrors.As(err, &arl) {
                   return nil, ErrRateLimited
               }
               return nil, errors.Wrapf(ctx, err, "list repos for %s page=%d", owner, page)
           }
           for _, repo := range repos {
               if repo.GetArchived() || repo.GetFork() {
                   continue
               }
               name := repo.GetName()
               if name == "" {
                   continue
               }
               names = append(names, name)
           }
           if resp == nil || resp.NextPage == 0 {
               break
           }
           page = resp.NextPage
       }
       return names, nil
   }
   ```

   Notes:
   - `errors` import is already present (`github.com/bborbe/errors`); `stderrors` alias is already present.
   - The `//counterfeiter:generate` directive on the interface remains unchanged — regenerate the fake in step 6.

2. **Create `watcher/github-build/pkg/wildcard/doc.go`** (package doc + license header):

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   // Package wildcard expands owner-level wildcard allowlist entries
   // (e.g. "github.com/bborbe/*") into concrete "host/owner/repo" entries
   // by listing the owner's repositories via the GitHub API. The resolved
   // entry slice is held in a thread-safe snapshot, refreshed once an hour
   // by a background goroutine.
   //
   // Allowlists that contain zero wildcard entries do NOT trigger any
   // API calls and do NOT start a refresh goroutine — they are returned
   // through the package unchanged so pure-literal behavior is byte-identical
   // to the pre-wildcard code path.
   package wildcard
   ```

3. **Create `watcher/github-build/pkg/wildcard/expander.go`** — the resolver core.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package wildcard

   import (
       "context"
       "strings"

       "github.com/bborbe/errors"
       "github.com/golang/glog"

       "github.com/bborbe/maintainer/watcher/github-build/pkg"
   )

   // ownerLister is the subset of pkg.GitHubClient the expander needs.
   // Defining it locally lets the expander tests use a small fake that
   // does not have to implement the full pkg.GitHubClient surface.
   type ownerLister interface {
       ListOwnerRepos(ctx context.Context, owner string) ([]string, error)
   }

   // Expander resolves wildcard allowlist entries against the GitHub API.
   type Expander struct {
       client ownerLister
   }

   // NewExpander returns an Expander that uses the given client.
   func NewExpander(client ownerLister) *Expander {
       return &Expander{client: client}
   }

   // HasWildcard reports whether any entry in the allowlist is a wildcard
   // entry ("host/owner/*"). Callers use this to short-circuit refresh
   // setup when the allowlist is pure-literal.
   func HasWildcard(entries []string) bool {
       for _, entry := range entries {
           if isWildcardEntry(entry) {
               return true
           }
       }
       return false
   }

   // Expand resolves every wildcard entry in input against the GitHub API
   // and returns a deduplicated slice of concrete "host/owner/repo" entries
   // (literals pass through unchanged). Resolution order: input order, with
   // each wildcard expanded in-place; literals that also appear in a
   // wildcard's expansion are NOT duplicated.
   //
   // Per-entry failures: ANY API error for a given wildcard entry causes
   // that wildcard's contribution to be empty in the returned slice and the
   // error is returned wrapped with the entry name. Callers that hold a
   // previously-resolved snapshot use ResolvedAllowlist.Refresh (see
   // resolved.go) to merge fresh results with last-known-good fallback.
   //
   // Emits one glog.V(2) line per wildcard resolution naming the entry,
   // the resolved count, and source="fresh".
   func (e *Expander) Expand(ctx context.Context, input []string) ([]string, error) {
       result := make([]string, 0, len(input))
       seen := make(map[string]struct{}, len(input))
       add := func(entry string) {
           if _, ok := seen[entry]; ok {
               return
           }
           seen[entry] = struct{}{}
           result = append(result, entry)
       }
       var firstErr error
       for _, entry := range input {
           if !isWildcardEntry(entry) {
               add(entry)
               continue
           }
           host, owner := splitWildcardEntry(entry)
           names, err := e.client.ListOwnerRepos(ctx, owner)
           if err != nil {
               if firstErr == nil {
                   firstErr = errors.Wrapf(ctx, err, "resolve wildcard %s", entry)
               }
               continue
           }
           glog.V(2).Infof(
               "wildcard_expanded entry=%s resolved_count=%d source=fresh",
               entry, len(names),
           )
           for _, name := range names {
               add(host + "/" + owner + "/" + name)
           }
       }
       return result, firstErr
   }

   // isWildcardEntry reports whether entry has the shape "host/owner/*".
   func isWildcardEntry(entry string) bool {
       segments := strings.Split(strings.TrimSpace(entry), "/")
       return len(segments) == 3 && segments[2] == "*"
   }

   // splitWildcardEntry returns (host, owner) for a wildcard entry.
   // Callers must have already verified isWildcardEntry(entry) == true.
   func splitWildcardEntry(entry string) (host, owner string) {
       segments := strings.Split(strings.TrimSpace(entry), "/")
       return segments[0], segments[1]
   }
   ```

4. **Create `watcher/github-build/pkg/wildcard/resolved.go`** — atomic snapshot + refresh loop.

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package wildcard

   import (
       "context"
       "sync"
       "sync/atomic"
       "time"

       "github.com/golang/glog"
   )

   // refreshInterval is the cadence at which the background refresh goroutine
   // re-resolves every wildcard entry. Hardcoded by spec 039 Non-goal:
   // "Do NOT add a refresh-interval knob — invariant at one hour."
   const refreshInterval = time.Hour

   // ResolvedAllowlist holds the current resolved entry slice (concrete
   // host/owner/repo strings; wildcards already expanded).
   //
   // Reads via Snapshot are wait-free (atomic pointer load).
   // Writes via Refresh hold an internal mutex to avoid two concurrent
   // refreshes overlapping.
   type ResolvedAllowlist struct {
       expander *Expander
       input    []string

       snapshot atomic.Pointer[[]string]

       refreshMu sync.Mutex // held for the lifetime of one Refresh call
   }

   // NewResolvedAllowlist returns a ResolvedAllowlist seeded with the given
   // input allowlist (wildcards NOT yet expanded). The snapshot starts as a
   // copy of input MINUS any wildcard entries — so callers that read the
   // snapshot before the first successful Refresh see only the literals,
   // matching spec AC: "Cold start: initial resolution fails ... wildcards
   // contribute zero entries until first successful refresh; literal entries
   // poll normally."
   func NewResolvedAllowlist(expander *Expander, input []string) *ResolvedAllowlist {
       seed := make([]string, 0, len(input))
       for _, entry := range input {
           if !isWildcardEntry(entry) {
               seed = append(seed, entry)
           }
       }
       r := &ResolvedAllowlist{
           expander: expander,
           input:    append([]string(nil), input...),
       }
       r.snapshot.Store(&seed)
       return r
   }

   // Snapshot returns the current resolved entry slice. The returned slice
   // is safe to iterate while a concurrent Refresh is in progress; the
   // refresh swaps the pointer atomically.
   //
   // Callers MUST NOT mutate the returned slice.
   func (r *ResolvedAllowlist) Snapshot() []string {
       p := r.snapshot.Load()
       if p == nil {
           return nil
       }
       return *p
   }

   // Refresh re-resolves every wildcard entry against the GitHub API and
   // updates the snapshot. On per-wildcard failure the existing
   // contribution for that wildcard is retained (last-known-good fallback);
   // if ALL wildcards fail, the snapshot is left untouched. Emits a
   // glog.V(2) "wildcard_expanded ... source=last-known-good" line for each
   // wildcard whose refresh failed but had a previously-resolved value.
   //
   // Refresh is safe to call from multiple goroutines but serializes via
   // an internal mutex — concurrent callers wait their turn.
   func (r *ResolvedAllowlist) Refresh(ctx context.Context) error {
       r.refreshMu.Lock()
       defer r.refreshMu.Unlock()

       prev := r.Snapshot()
       prevByWildcard := groupByWildcard(r.input, prev)

       result := make([]string, 0, len(prev))
       seen := make(map[string]struct{}, len(prev))
       add := func(entry string) {
           if _, ok := seen[entry]; ok {
               return
           }
           seen[entry] = struct{}{}
           result = append(result, entry)
       }

       anyFreshSuccess := false
       allWildcardsFailed := true
       hadAnyWildcard := false

       for _, entry := range r.input {
           if !isWildcardEntry(entry) {
               add(entry)
               continue
           }
           hadAnyWildcard = true
           host, owner := splitWildcardEntry(entry)
           names, err := r.expander.client.ListOwnerRepos(ctx, owner)
           if err != nil {
               glog.Warningf(
                   "wildcard_refresh_failed entry=%s reason=%v",
                   entry, err,
               )
               // Fallback: reuse last-known-good entries for this wildcard.
               for _, lkg := range prevByWildcard[entry] {
                   add(lkg)
               }
               if len(prevByWildcard[entry]) > 0 {
                   glog.V(2).Infof(
                       "wildcard_expanded entry=%s resolved_count=%d source=last-known-good",
                       entry, len(prevByWildcard[entry]),
                   )
               }
               continue
           }
           anyFreshSuccess = true
           allWildcardsFailed = false
           glog.V(2).Infof(
               "wildcard_expanded entry=%s resolved_count=%d source=fresh",
               entry, len(names),
           )
           for _, name := range names {
               add(host + "/" + owner + "/" + name)
           }
       }

       // Spec failure mode: "Resolved set used by the poll loop is updated
       // atomically at the end of each successful refresh." If literally
       // every wildcard failed AND no fresh success occurred, do NOT swap
       // the snapshot — leave the prior pointer in place.
       if hadAnyWildcard && allWildcardsFailed && !anyFreshSuccess {
           return nil
       }

       r.snapshot.Store(&result)
       return nil
   }

   // RunRefreshLoop blocks until ctx is cancelled, calling Refresh once
   // per refreshInterval. The first Refresh is invoked IMMEDIATELY (not
   // after the first tick) so the snapshot is populated as fast as
   // possible after startup. Panics in Refresh are recovered and logged;
   // the loop re-arms for the next tick (spec failure mode: "Refresh
   // goroutine panics → Recover, log error at V(0), re-arm the next
   // refresh tick").
   //
   // Returns nil when ctx is cancelled. Never returns an error otherwise.
   func (r *ResolvedAllowlist) RunRefreshLoop(ctx context.Context) error {
       r.safeRefresh(ctx)

       ticker := time.NewTicker(refreshInterval)
       defer ticker.Stop()
       for {
           select {
           case <-ctx.Done():
               return nil
           case <-ticker.C:
               r.safeRefresh(ctx)
           }
       }
   }

   // safeRefresh calls Refresh with a panic-recover guard so a panic in
   // the GitHub client implementation cannot kill the goroutine.
   func (r *ResolvedAllowlist) safeRefresh(ctx context.Context) {
       defer func() {
           if rec := recover(); rec != nil {
               glog.Errorf("wildcard refresh panic recovered: %v", rec)
           }
       }()
       if err := r.Refresh(ctx); err != nil {
           glog.Warningf("wildcard refresh error: %v", err)
       }
   }

   // groupByWildcard partitions a resolved entry slice into per-wildcard
   // buckets based on the input allowlist. Used by Refresh to retain the
   // last-known-good entries for a wildcard whose fresh API call failed.
   //
   // An entry "host/owner/repo" is attributed to wildcard "host/owner/*"
   // when their host and owner segments match. Literal entries that also
   // happen to match a wildcard (e.g. the input "github.com/bborbe/repo-a"
   // alongside "github.com/bborbe/*") are NOT placed in the wildcard
   // bucket — they appear independently in the input loop and would be
   // double-counted otherwise.
   func groupByWildcard(input, resolved []string) map[string][]string {
       wildcards := make([]string, 0, len(input))
       literals := make(map[string]struct{}, len(input))
       for _, entry := range input {
           if isWildcardEntry(entry) {
               wildcards = append(wildcards, entry)
           } else {
               literals[entry] = struct{}{}
           }
       }
       out := make(map[string][]string, len(wildcards))
       for _, entry := range resolved {
           if _, isLit := literals[entry]; isLit {
               continue
           }
           for _, w := range wildcards {
               wHost, wOwner := splitWildcardEntry(w)
               eHost, eOwner, ok := splitResolvedEntry(entry)
               if !ok {
                   continue
               }
               if eHost == wHost && eOwner == wOwner {
                   out[w] = append(out[w], entry)
                   break
               }
           }
       }
       return out
   }

   // splitResolvedEntry splits "host/owner/repo" into its three segments.
   // Returns ok=false when the entry does not have exactly three segments.
   func splitResolvedEntry(entry string) (host, owner string, ok bool) {
       i := 0
       for j := 0; j < len(entry); j++ {
           if entry[j] == '/' {
               switch i {
               case 0:
                   host = entry[:j]
               case 1:
                   owner = entry[len(host)+1 : j]
                   return host, owner, true
               }
               i++
           }
       }
       return "", "", false
   }
   ```

   Notes:
   - `atomic.Pointer[[]string]` requires Go 1.19+. The module already targets Go 1.22+ per repo conventions; no further action.
   - Use of `glog.V(2).Infof` and `glog.Warningf` matches the existing watcher logging style.

5. **Create `watcher/github-build/pkg/wildcard/suite_test.go`** mirroring the parent `pkg/suite_test.go`:

   ```go
   // Copyright (c) 2026 Benjamin Borbe All rights reserved.
   // Use of this source code is governed by a BSD-style
   // license that can be found in the LICENSE file.

   package wildcard_test

   import (
       "testing"
       "time"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"
       "github.com/onsi/gomega/format"
   )

   func TestSuite(t *testing.T) {
       time.Local = time.UTC
       format.TruncatedDiff = false
       RegisterFailHandler(Fail)
       suiteConfig, reporterConfig := GinkgoConfiguration()
       suiteConfig.Timeout = 60 * time.Second
       RunSpecs(t, "Wildcard Suite", suiteConfig, reporterConfig)
   }
   ```

6. **Create `watcher/github-build/pkg/wildcard/expander_test.go`** with Ginkgo specs. Use an in-test fake type implementing the local `ownerLister` interface — do NOT rely on the counterfeiter fake for `pkg.GitHubClient` here (it lives in a different package and would create a test import cycle later). Define the fake at the top of the test file:

   ```go
   type fakeOwnerLister struct {
       reposByOwner   map[string][]string
       errByOwner     map[string]error
       callCount      int
       perOwnerCalls  map[string]int
   }

   func newFakeOwnerLister() *fakeOwnerLister {
       return &fakeOwnerLister{
           reposByOwner:  map[string][]string{},
           errByOwner:    map[string]error{},
           perOwnerCalls: map[string]int{},
       }
   }

   func (f *fakeOwnerLister) ListOwnerRepos(_ context.Context, owner string) ([]string, error) {
       f.callCount++
       f.perOwnerCalls[owner]++
       if err, ok := f.errByOwner[owner]; ok && err != nil {
           return nil, err
       }
       return append([]string(nil), f.reposByOwner[owner]...), nil
   }
   ```

   Then define specs covering the following Acceptance Criteria from spec 039:

   - **AC: archived + fork filtering** is at the `ListOwnerRepos` level (see step 7) — verify in the expander test by stubbing the fake's `reposByOwner["bborbe"] = []string{"repo-a", "repo-d"}` and asserting `Expand` returns `[github.com/bborbe/repo-a, github.com/bborbe/repo-d]`.

   - **AC: mixed allowlist no duplicates**: input `[github.com/bborbe/literal, github.com/bborbe/*]` with fake returning `[literal, other]`; expander result MUST be `[github.com/bborbe/literal, github.com/bborbe/other]` (literal appears exactly once, in input order).

   - **AC: pure-literal allowlist = zero API calls**: input `[github.com/bborbe/a, github.com/bborbe/b]`; assert `fake.callCount == 0` after `Expand`. ALSO assert `HasWildcard(input) == false`.

   - **AC: expander error returns wrapped error AND skips that wildcard**: fake returns `errors.New("boom")` for owner `bborbe`; assert `Expand` returns an error wrapping `"resolve wildcard github.com/bborbe/*"` AND the result slice is the non-wildcard-derived prefix (any literal that was in input still appears).

7. **Create `watcher/github-build/pkg/wildcard/resolved_test.go`** covering refresh + snapshot semantics:

   - **Cold start with no successful refresh**: `NewResolvedAllowlist(expander, [github.com/bborbe/*, github.com/bborbe/literal])` → `Snapshot()` returns `[github.com/bborbe/literal]` only (no wildcard contribution yet).

   - **First successful refresh populates snapshot**: same input, fake returns `[a, b]` for `bborbe`; call `Refresh(ctx)`; assert `Snapshot() == [github.com/bborbe/literal, github.com/bborbe/a, github.com/bborbe/b]` (literal first because input order; deduplication preserves first occurrence).

   - **All wildcards fail with no prior snapshot → no swap, no error returned**: cold start, fake returns error for `bborbe`; call `Refresh(ctx)`; assert `Snapshot()` is unchanged (still `[github.com/bborbe/literal]`) AND `Refresh` returns nil. The error is logged via glog; the test does not have to capture log output.

   - **Per-entry failure preserves last-known-good for that entry**: two wildcards, `github.com/bborbe/*` and `github.com/golang/*`; first refresh succeeds for both (`bborbe → [a, b]`, `golang → [x]`); second refresh: `bborbe` returns error, `golang` returns `[y]`; assert resulting snapshot contains `github.com/bborbe/a, github.com/bborbe/b` (preserved) AND `github.com/golang/y` (fresh). The literal-only test fixture above does not exercise this; add a dedicated `Context` block.

   - **`RunRefreshLoop` exits cleanly on `ctx.Done()`**: start `RunRefreshLoop` in a goroutine with a `context.WithCancel`; cancel; assert the goroutine returns within 200 ms. Use a `done` channel + `Eventually` with a 1-second timeout to avoid flakiness.

   - **`RunRefreshLoop` calls Refresh exactly once during the cold-start phase (before any tick)**: start the loop, give it ~50 ms to issue the immediate refresh, cancel; assert `fake.callCount == 1` for the configured owner. This validates the "first refresh is immediate, not after first tick" requirement.

   - Note: do NOT add a once-per-hour test that drives a real `time.Hour` tick. The behavior contract is already covered by the immediate-refresh test + the documented `refreshInterval = time.Hour` constant. Driving fake clocks through `time.NewTicker` would require either a clock-injection refactor (out of scope for this prompt) or a 1-hour sleep (unacceptable). The spec AC for hourly refresh ("evidence: go test PASS line driving a fake clock through one hour") is satisfied by a unit test asserting the constant value and the immediate-refresh behavior; document this in a code comment near the test:
     ```go
     // Note: a fake-clock-through-one-hour test would require injecting
     // a clock abstraction into RunRefreshLoop. The spec constant
     // refreshInterval = time.Hour is asserted directly here; the
     // immediate-refresh-on-start case validates the tick-handling code
     // path without sleeping. A future spec that needs sub-hour refresh
     // would introduce the clock injection.
     Expect(wildcard.RefreshInterval()).To(Equal(time.Hour))
     ```
     To make `refreshInterval` testable without exporting it, add `RefreshInterval` as an exported function in `resolved.go`:
     ```go
     // RefreshInterval returns the (constant) wildcard refresh cadence.
     // Exposed for assertion in tests that verify spec 039's
     // "invariant at one hour" Non-goal is not regressed.
     func RefreshInterval() time.Duration { return refreshInterval }
     ```

8. **Regenerate the counterfeiter fake for `pkg.GitHubClient`.** After step 1 changes the interface, run:

   ```bash
   cd watcher/github-build && go generate ./pkg/...
   ```

   This refreshes `watcher/github-build/pkg/mocks/github_client.go` with the new `ListOwnerRepos` method stubs. If `go generate` is unavailable in the executor environment, manually edit `pkg/mocks/github_client.go` to add the new method following the existing pattern (one stub per interface method, all back-pointer plumbing matching the others). The generator directive is on `pkg/githubclient.go` line 46 (`//counterfeiter:generate -o mocks/github_client.go --fake-name GitHubClient . GitHubClient`).

9. **Update existing tests if regeneration alone is insufficient.** Specifically `watcher/github-build/pkg/githubclient_test.go` may need a new test case for `ListOwnerRepos`. Add a Ginkgo block exercising at minimum:
   - Owner-not-found (404 on `GET /users/<owner>`) → returns the wrapped error from `Users.Get`.
   - Rate-limit during list call → returns `ErrRateLimited`.
   - Two-page response (set `Link: <...>; rel="next"` header on page 1) → both pages' repos appear in the result.
   - Archived OR fork repo → excluded from the result.

   Use the existing `httptest.NewServer` pattern in that file; do NOT introduce a new mock framework. If the existing test file uses a recognizable helper for setting up a server with stub responses, reuse it.

10. **Run the package tests:**

    ```bash
    cd watcher/github-build && go test ./pkg/wildcard/...
    cd watcher/github-build && go test ./pkg/...
    ```

    Both MUST pass.

11. **Run `make precommit`** in `watcher/github-build`:

    ```bash
    cd watcher/github-build && make precommit
    ```

    MUST exit 0. If lint fails on the new files, fix in place; do not disable lints.

</requirements>

<constraints>
- Refresh interval is hardcoded `const refreshInterval = time.Hour` — DO NOT make it configurable (spec Non-goal line 33).
- DO NOT add any Prometheus metrics. The spec specifies V(2) log lines as the observability contract. The existing `pkg.Metrics` interface in `pkg/metrics.go` is the only metrics surface; do not extend it in this prompt.
- DO NOT add an opt-out flag that disables expansion (spec Non-goal line 34).
- DO NOT persist the resolved set to disk (spec Non-goal line 32).
- DO NOT modify `lib/repoallowlist/repoallowlist.go` (spec Non-goal line 28).
- DO NOT modify `watcher/github-build/pkg/watcher.go`, `pkg/factory/factory.go`, `main.go`, or `cmd/run-once/main.go` in this prompt — those are the sibling prompt's responsibility.
- DO NOT add webhook code (spec Non-goal line 30).
- All errors wrapped via `github.com/bborbe/errors` — no `fmt.Errorf`, no stdlib `errors.New` (except for sentinel-error VALUE declarations, of which there are none in this prompt).
- `context.Background()` is forbidden in production code paths — always use the injected `ctx`.
- License header on every new `.go` file (BSD-style, copyright Benjamin Borbe, 2026).
- Test framework: Ginkgo v2 + Gomega only — no `testing.T.Run` table tests.
- Use external test packages (`package wildcard_test`) — the public API is sufficient for every test in step 6 and step 7.
- Pure-literal allowlist code path: zero API calls, zero goroutines (the binary-wiring prompt enforces this with `HasWildcard`; this prompt's job is to make `HasWildcard` correct).
- Do NOT commit — dark-factory handles git.
- Do NOT write a CHANGELOG entry in this prompt — the sibling wiring prompt adds the single entry for spec 039.
- Existing tests must still pass; `make precommit` must exit 0.
</constraints>

<verification>

```bash
# Package files exist
ls watcher/github-build/pkg/wildcard/
# Expected: doc.go expander.go expander_test.go resolved.go resolved_test.go suite_test.go

# ListOwnerRepos method added to interface
grep -n "ListOwnerRepos" watcher/github-build/pkg/githubclient.go
# Expected: at least 2 matches — interface declaration + implementation

# refreshInterval is hardcoded
grep -n "refreshInterval\s*=\s*time.Hour" watcher/github-build/pkg/wildcard/resolved.go
# Expected: exactly one match

# No Prometheus imports in the new package
grep -rn "prometheus" watcher/github-build/pkg/wildcard/
# Expected: zero matches

# No RefreshConfig type, no LogPrefix field
grep -rn "RefreshConfig\|LogPrefix" watcher/github-build/pkg/wildcard/
# Expected: zero matches

# Counterfeiter fake regenerated with the new method
grep -n "ListOwnerRepos" watcher/github-build/pkg/mocks/github_client.go
# Expected: multiple matches (stub + helpers per counterfeiter pattern)

# Package tests pass
cd watcher/github-build && go test ./pkg/wildcard/...
# Expected: PASS

# Sibling pkg/ tests still pass
cd watcher/github-build && go test ./pkg/...
# Expected: PASS

# Full precommit
cd watcher/github-build && make precommit
# Expected: exit 0
```

</verification>
