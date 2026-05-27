---
status: approved
spec: [044-github-release-watcher-implementation]
created: "2026-05-27T20:38:37Z"
queued: "2026-05-27T20:57:47Z"
---

<summary>
- `Watcher.Poll` ties everything together: load cursor → ListRepos → per-repo fetch+filter+publish → save cursor → emit cycle metric
- 5xx from GitHub during ListRepos or per-repo fetch aborts the cycle WITHOUT cursor save; emits `IncPollCycle("github_error")` or `IncPollCycle("rate_limited")` accordingly
- Per-repo transient errors prune the repo from this cycle without aborting the whole pass (next cycle re-fetches; controller dedup absorbs)
- `main.go` `resolveAuth` is filled in (App-auth wins; PAT fallback; partial-config rejected)
- Ginkgo `Poll publishes one task per non-skipped repo and saves cursor` test drives the cycle through a counterfeiter-mocked `GitHubClient`
- Final prompt — `make precommit` clean across all of `watcher/github-release/`
</summary>

<objective>
Implement `Watcher.Poll` per the spec § Desired Behavior #7 cycle contract, fill in `application.resolveAuth` in `main.go` mirroring `watcher/github-pr`, and add the Ginkgo cycle test that closes the spec's named acceptance criteria. After this prompt, all spec ACs must pass.
</objective>

<context>
Read CLAUDE.md for project conventions.

Read these guides:
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-context-cancellation-in-loops.md` — non-blocking ctx check on each iteration
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-glog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-prometheus-metrics-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`

Read these files end-to-end:
- `watcher/github-release/pkg/watcher.go` — `Watcher` interface + `NewWatcher` + `watcher` struct + `Poll` stub + `cursorReader` adapter
- `watcher/github-release/main.go` — current `Run` body (already complete except `resolveAuth` TODO at line 137)
- `watcher/github-release/pkg/cursor.go` — `LoadCursor` + `SaveCursor` (from prompt 2)
- `watcher/github-release/pkg/githubclient.go` — `GitHubClient` interface + `ErrRateLimited` sentinel (from prompt 4)
- `watcher/github-release/pkg/taskpublisher.go` — `TaskPublisher.PublishCreate` (from prompt 5)
- `watcher/github-release/pkg/release.go` — `Release` struct
- `watcher/github-release/pkg/changelog.go` — `ParseChangelog` returning `ChangelogSummary{UnreleasedBullets, UnreleasedIsFirst, LatestVersion}`
- `watcher/github-release/pkg/filter/filter.go` — local `Release` struct for filter input + `TaskCreationFilters` slice composite
- `watcher/github-release/pkg/metrics.go` — `IncPollCycle("success"|"rate_limited"|"github_error")`, `IncReposScanned(n)`, `IncFilterSkipped(reason)`

Reference implementations:
- `/workspace/watcher/github-pr/main.go` — `resolveAuth` (line 144-191). The github-release variant has the SAME shape minus PR-specific knobs.
- `/workspace/watcher/github-pr/pkg/watcher.go` `Poll` + `fetchAllPRs` + `processPRs` — for cycle-abort idiom (return early with metric on github_error) and structured glog lines.
- `/workspace/watcher/github-build/pkg/watcher.go` — per-repo loop with select-on-ctx (the github-release Poll iterates per-repo similarly).

Counterfeiter / mock note: `watcher.go` has a `//counterfeiter:generate -o mocks/watcher.go --fake-name Watcher . Watcher` directive at line 15. The destination is `pkg/mocks/watcher.go` after `make generate`. The `GitHubClient`, `TaskPublisher`, `Metrics`, and `TaskCreationFilter` mocks are also in `pkg/mocks/` from earlier prompts — use them directly in the Poll test rather than hand-rolled fakes.

Default-branch link target: the existing skeleton + Phase 1 prototype use `blob/master/CHANGELOG.md` regardless of actual default branch. Repos using `main` will have a 404-on-click body link but the agent itself clones at `ref` and reads the raw file directly. The spec carries this verbatim — do not fix as a side-effect of this prompt.
</context>

<requirements>

**Execute steps in order. Run `cd watcher/github-release && make test` after step 4. Run `make precommit` only at the final step.**

1. **Implement `Watcher.Poll` in `watcher/github-release/pkg/watcher.go`** — full cycle per spec § Desired Behavior #7. Keep the existing `Watcher` interface, `NewWatcher`, `watcher` struct, `cursorReader` adapter and `NewCursorReader` helper. The `taskCreationFilter` field already holds the cycle-invariant chain assembled in `main.go` (`RepoAllowlistFilter`, `EmptyUnreleasedFilter`, `AutoReleaseFilter`); `SHAUnchangedFilter` is composed in per cycle below.

   Required structure (extract helpers — keep `Poll` short and readable):

   ```go
   func (w *watcher) Poll(ctx context.Context) error {
       cursorState, err := LoadCursor(ctx, w.cursorPath)
       if err != nil {
           return errors.Wrapf(ctx, err, "load cursor path=%s", w.cursorPath)
       }

       repos, err := w.ghClient.ListRepos(ctx, w.owner)
       if err != nil {
           if stderrors.Is(err, ErrRateLimited) {
               w.metrics.IncPollCycle("rate_limited")
               glog.Warningf("poll cycle aborted: rate limited during ListRepos owner=%s", w.owner)
               return nil
           }
           w.metrics.IncPollCycle("github_error")
           glog.Errorf("poll cycle aborted: ListRepos owner=%s err=%v", w.owner, err)
           return nil
       }
       w.metrics.IncReposScanned(len(repos))

       // Compose cycle-specific SHAUnchangedFilter into the chain.
       cycleFilter := filter.TaskCreationFilters{
           w.taskCreationFilter,
           filter.NewSHAUnchangedFilter(NewCursorReader(cursorState)),
       }

       abortReason := w.processRepos(ctx, cursorState, repos, cycleFilter)
       if abortReason != "" {
           w.metrics.IncPollCycle(abortReason)
           // Do NOT save cursor on abort — next cycle resumes from same state.
           return nil
       }

       if err := SaveCursor(ctx, w.cursorPath, cursorState); err != nil {
           // Per spec failure-modes: cursor save error post-publish is best-effort.
           // Tasks were already published; controller dedup absorbs re-emit next cycle.
           glog.Warningf("save cursor failed path=%s err=%v", w.cursorPath, err)
       }
       w.metrics.IncPollCycle("success")
       return nil
   }
   ```

   Helper `processRepos` — per-repo gather + filter + publish:

   ```go
   // processRepos iterates repos sequentially (spec § Non-goals: per-repo parallelism is agent territory).
   // Returns "" on success, "github_error" or "rate_limited" if the cycle should abort and skip cursor save.
   //
   // Per-repo error policy (spec failure-modes):
   //   - Cycle-aborting (return early): rate_limited at any layer; 5xx during ListRepos (handled in Poll above).
   //   - Per-repo prune (continue loop): GetMasterSHA / GetChangelogContent / GetAutoReleaseConfig transient
   //     non-rate-limit error — log via glog.V(2).Infof so operator can grep "repo dropped from cycle".
   func (w *watcher) processRepos(
       ctx context.Context,
       cursorState *Cursor,
       repos []Repo,
       cycleFilter filter.TaskCreationFilter,
   ) string {
       for _, repo := range repos {
           select {
           case <-ctx.Done():
               glog.V(2).Infof("poll cancelled during processRepos at repo=%s", repo.Key())
               return ""
           default:
           }

           release, abortReason, dropped := w.gatherRelease(ctx, repo)
           if abortReason != "" {
               return abortReason
           }
           if dropped {
               continue
           }

           filterInput := filter.Release{
               RepoKey:           repo.Key(),
               HeadSHA:           release.HeadSHA,
               UnreleasedBullets: release.UnreleasedBullets,
               AutoRelease:       release.AutoRelease,
           }
           if cycleFilter.Skip(filterInput) {
               // Specific reason metrics: probe each cycle-aware predicate. Cheap (≤4 calls per skipped release).
               w.recordSkipReason(filterInput, cursorState)
               continue
           }

           if w.publisher.PublishCreate(ctx, release) {
               if cursorState.Repos == nil {
                   cursorState.Repos = make(map[string]*RepoState)
               }
               cursorState.Repos[repo.Key()] = &RepoState{LastSeenMasterSHA: release.HeadSHA}
           }
       }
       return ""
   }

   // gatherRelease fetches HeadSHA, ChangelogContent, AutoReleaseConfig for one repo.
   // Returns (release, "", false) on success.
   // Returns ({}, "rate_limited"|"github_error", false) when the whole cycle should abort.
   // Returns ({}, "", true) when this repo should be silently pruned from the cycle.
   func (w *watcher) gatherRelease(ctx context.Context, repo Repo) (Release, string, bool) {
       headSHA, err := w.ghClient.GetMasterSHA(ctx, repo)
       if err != nil {
           if stderrors.Is(err, ErrRateLimited) {
               return Release{}, "rate_limited", false
           }
           glog.V(2).Infof("repo dropped from cycle: owner=%s repo=%s err=%v", repo.Owner, repo.Name, err)
           return Release{}, "", true
       }
       content, err := w.ghClient.GetChangelogContent(ctx, repo)
       if err != nil {
           if stderrors.Is(err, ErrRateLimited) {
               return Release{}, "rate_limited", false
           }
           glog.V(2).Infof("repo dropped from cycle: owner=%s repo=%s err=%v", repo.Owner, repo.Name, err)
           return Release{}, "", true
       }
       autoRelease, err := w.ghClient.GetAutoReleaseConfig(ctx, repo)
       if err != nil {
           if stderrors.Is(err, ErrRateLimited) {
               return Release{}, "rate_limited", false
           }
           glog.V(2).Infof("repo dropped from cycle: owner=%s repo=%s err=%v", repo.Owner, repo.Name, err)
           return Release{}, "", true
       }
       summary := ParseChangelog(content)
       currentVersion := summary.LatestVersion
       if currentVersion == "" {
           currentVersion = "v0.0.0"
       }
       return Release{
           Repo:              repo,
           HeadSHA:           headSHA,
           CurrentVersion:    currentVersion,
           UnreleasedBullets: summary.UnreleasedBullets,
           AutoRelease:       autoRelease,
       }, "", false
   }

   // recordSkipReason maps the specific predicate that triggered the skip to
   // its metric label. Order MUST match main.go's static-filter ordering plus
   // the cycle-composed SHAUnchangedFilter.
   func (w *watcher) recordSkipReason(in filter.Release, cursorState *Cursor) {
       switch {
       case !isAllowed(in.RepoKey, w):
           w.metrics.IncFilterSkipped("scope")
       case in.UnreleasedBullets == 0:
           w.metrics.IncFilterSkipped("empty_unreleased")
       case in.AutoRelease:
           w.metrics.IncFilterSkipped("auto_release")
       case NewCursorReader(cursorState).LastSeenSHA(in.RepoKey) == in.HeadSHA:
           w.metrics.IncFilterSkipped("sha_unchanged")
       default:
           // Composite voted skip but no single predicate matched our probes — should not happen.
           glog.Warningf("unattributed skip repoKey=%s headSHA=%s", in.RepoKey, in.HeadSHA)
       }
   }
   ```

   The `isAllowed(repoKey, w)` helper hits a snag: the watcher struct does NOT carry the allowlist (it carries the assembled `taskCreationFilter`). To probe scope skip-reason cleanly without re-parsing the env, either:

   a. (preferred — minimal blast radius) Add an `allowlist []string` field to the `watcher` struct and to `NewWatcher`. Update `factory.CreateWatcher` to receive and pass it. Update `main.go` to pass `allowlist` to `factory.CreateWatcher` (the parsed slice already exists at line 62 of `main.go`). Then `isAllowed` is `repoallowlist.IsAllowed(w.allowlist, repoKey)` from `github.com/bborbe/maintainer/lib/repoallowlist`.

   b. (acceptable fallback) Skip the `scope` label and emit it only as a fallback log — but this fails spec AC § Constraint "Pre-init Prometheus counter label combinations to .Add(0)" requires the `scope` label to be FIRED at least once across the test surface. Option (a) is cleaner.

   Go with (a). Mirror github-pr's pattern: the watcher struct field for the allowlist is purely for metric attribution; the actual gate stays in `taskCreationFilter`. Add to `watcher` struct:
   ```go
   allowlist []string
   ```
   Add to `NewWatcher` signature as last positional arg `allowlist []string`. Update `factory.CreateWatcher` signature to accept `allowlist []string` and forward. Update `main.go` `factory.CreateWatcher(...)` call (line 101-103) to pass `allowlist`.

2. **Implement `resolveAuth` in `watcher/github-release/main.go`** (replace the TODO at line 137). Copy the body from `watcher/github-pr/main.go` `resolveAuth` (line 144-191) VERBATIM, adapting only the `getEnvInt` calls. The reference reads env via `os.Getenv` directly — to avoid pulling in `getEnvInt` from github-pr, inline `getEnvInt` at the bottom of `main.go`:

   ```go
   func getEnvInt(name string) int64 {
       v, err := strconv.ParseInt(os.Getenv(name), 10, 64)
       if err != nil {
           return 0
       }
       return v
   }
   ```

   Add `"strconv"` to the imports. The function body uses `os.Getenv("APP_ID")`, `os.Getenv("INSTALLATION_ID")`, `os.Getenv("PEM_KEY")`, `os.Getenv("GH_TOKEN")` exactly like the github-pr variant. The partial-config rejection and App-wins precedence rules are identical.

   Replace the existing stub:
   ```go
   func (a *application) resolveAuth(ctx context.Context) (*http.Client, error) {
       return nil, errors.New(ctx, "main: resolveAuth not implemented")
   }
   ```

3. **Update `factory.CreateWatcher` signature in `watcher/github-release/pkg/factory/factory.go`** to accept and forward `allowlist []string`. The current signature ends with `metrics pkg.Metrics`; add `allowlist []string` AFTER `metrics`. Pass it through to `pkg.NewWatcher(...)`. Then update `main.go` `factory.CreateWatcher(...)` call site to pass `allowlist` (the already-parsed local at line 62 of `main.go`).

4. **Add the Ginkgo cycle test in `watcher/github-release/pkg/watcher_test.go`** as package `pkg_test`. Use the counterfeiter mocks generated in earlier prompts (`pkg/mocks/github_client.go`, `pkg/mocks/task_publisher.go`, `pkg/mocks/metrics.go`).

   Imports:
   ```go
   import (
       "context"
       stderrors "errors"
       "os"
       "path/filepath"

       . "github.com/onsi/ginkgo/v2"
       . "github.com/onsi/gomega"

       "github.com/bborbe/maintainer/watcher/github-release/pkg"
       "github.com/bborbe/maintainer/watcher/github-release/pkg/filter"
       "github.com/bborbe/maintainer/watcher/github-release/pkg/mocks"
   )
   ```
   The `stderrors "errors"` alias matches the convention in `/workspace/watcher/github-build/pkg/watcher_test.go:9`. Use `stderrors.Is` / `stderrors.New` in tests for stdlib errors; reserve unprefixed `errors` for `github.com/bborbe/errors` (production paths only — never in `_test.go`).

   `Describe("pkg.Watcher.Poll", ...)` with these `It` blocks:

   a. **`It("Poll publishes one task per non-skipped repo and saves cursor")`** — spec acceptance criterion verbatim. Setup:
   - `ghClient := &mocks.GitHubClient{}`
   - `ghClient.ListReposReturns([]pkg.Repo{{Owner:"bborbe", Name:"docker-utils", DefaultBranch:"master"}, {Owner:"bborbe", Name:"empty-repo", DefaultBranch:"main"}}, nil)`
   - Wire `GetMasterSHA` via `ghClient.GetMasterSHAStub = func(_ context.Context, r pkg.Repo) (string, error) { ... }` returning `"d630ef3..."` for `docker-utils`, `"abc123..."` for `empty-repo`.
   - Wire `GetChangelogContent` to return `[]byte("## Unreleased\n\n- entry\n\n## v1.7.7\n")` for `docker-utils`, and `[]byte("## Unreleased\n\n## v0.0.1\n")` (empty Unreleased) for `empty-repo`.
   - Wire `GetAutoReleaseConfig` to return `(false, nil)` for both.
   - `publisher := &mocks.TaskPublisher{}` with `publisher.PublishCreateReturns(true)`.
   - `metricsMock := &mocks.Metrics{}`.
   - `tmpDir, _ := os.MkdirTemp("", "watcher-poll-*")`; `cursorPath := filepath.Join(tmpDir, "cursor.json")`.
   - Static chain: `staticFilters := filter.TaskCreationFilters{filter.NewRepoAllowlistFilter(nil), filter.NewEmptyUnreleasedFilter(), filter.NewAutoReleaseFilter()}`.
   - `w := pkg.NewWatcher(ghClient, publisher, metricsMock, cursorPath, "bborbe", staticFilters, nil /*allowlist*/)`.
   - Call `Expect(w.Poll(context.Background())).To(Succeed())`.

   Assertions:
   - `publisher.PublishCreateCallCount() == 1` — only `docker-utils` makes it through (empty-repo skipped by EmptyUnreleasedFilter).
   - Capture the published release: `_, release := publisher.PublishCreateArgsForCall(0)`. Assert `release.Repo.Name == "docker-utils"`, `release.HeadSHA == "d630ef3..."`, `release.CurrentVersion == "v1.7.7"`, `release.UnreleasedBullets == 1`.
   - Cursor file was written: `_, err := os.Stat(cursorPath); Expect(err).NotTo(HaveOccurred())`. Load it back: `loaded, _ := pkg.LoadCursor(ctx, cursorPath)`. Assert `loaded.Repos["github.com/bborbe/docker-utils"].LastSeenMasterSHA == "d630ef3..."` AND `loaded.Repos["github.com/bborbe/empty-repo"]` is absent (skipped repos do not get cursor entries).
   - `metricsMock.IncPollCycleCallCount() == 1` with arg `"success"`.
   - `metricsMock.IncFilterSkippedCallCount() == 1` with arg `"empty_unreleased"`.
   - `metricsMock.IncReposScannedCallCount() == 1` with arg `2`.

   Cleanup `tmpDir` in `AfterEach`.

   b. **`It("Poll aborts cycle and skips cursor save on ListRepos rate-limit")`** — same setup but `ghClient.ListReposReturns(nil, pkg.ErrRateLimited)`. Assertions:
   - `Poll` returns nil (not an error — the metric label IS the signal).
   - `metricsMock.IncPollCycle` called once with arg `"rate_limited"`.
   - `publisher.PublishCreateCallCount() == 0`.
   - Cursor file was NOT written: `_, err := os.Stat(cursorPath); Expect(os.IsNotExist(err)).To(BeTrue())`.

   c. **`It("Poll aborts cycle and skips cursor save on ListRepos github_error")`** — `ghClient.ListReposReturns(nil, errpkg.New("500 internal error"))`. Assertions: metric label is `"github_error"`; no publish; no cursor write.

   d. **`It("Poll prunes individual repos with transient GetMasterSHA errors and continues")`** — two repos in `ListRepos`. `GetMasterSHA` for repo A returns `errpkg.New("transient network error")`; for repo B returns a valid SHA + non-empty Unreleased. Assertions:
   - `Poll` completes successfully with `IncPollCycle("success")`.
   - `publisher.PublishCreateCallCount() == 1` (only repo B).
   - Cursor saved with only repo B's entry.

   e. **`It("Poll aborts mid-cycle on per-repo rate-limit during GetChangelogContent")`** — two repos; for repo A, `GetMasterSHA` succeeds but `GetChangelogContent` returns `pkg.ErrRateLimited`. Assertion: metric label is `"rate_limited"`; publish count 0 for both; no cursor write.

   f. **`It("Poll updates cursor only for repos that successfully publish")`** — one repo. `publisher.PublishCreateReturns(false)` (Kafka send failure simulated by publisher returning false). Assertions: `Poll` succeeds; cursor file exists but `loaded.Repos["github.com/bborbe/docker-utils"]` is absent (the failed publish did NOT advance the cursor — re-emit next cycle, controller dedup absorbs).

5. **Run unit tests**:
   ```bash
   cd watcher/github-release && make test
   ```
   Fix any failures. Coverage on `Poll` and helpers should be near 100% given the six tests above.

6. **Run full precommit**:
   ```bash
   cd watcher/github-release && make precommit
   ```

7. **Final spec AC sweep** (verify all named acceptance criteria from spec § Acceptance Criteria):
   ```bash
   cd watcher/github-release
   # No TODO remaining
   grep -rn "TODO" . --include='*.go'
   # Expected: 0 lines

   # Mock regen is stable
   make generate
   git diff --exit-code pkg/mocks
   # Expected: exit 0

   # No context.Background() in production
   grep -rn "context.Background()" . --include='*.go' | grep -v _test.go
   # Expected: 1 acceptable hit in main.go's `service.Main(context.Background(), ...)` — this is the standard entry point per `bborbe/service`. Document with an inline comment if not already present.

   # No fmt.Errorf in production
   grep -rn "fmt.Errorf" . --include='*.go' | grep -v _test.go
   # Expected: 0 lines

   # Prometheus pre-init still wired
   grep -A2 "publishedTotal.WithLabelValues" pkg/metrics.go
   # Expected: shows the `for _, s := range []string{"create", "skipped", "error"}` block
   ```

</requirements>

<constraints>
- Mirror `watcher/github-pr` and `watcher/github-build` Go patterns verbatim: `errors.Wrapf(ctx, err, ...)` for production wrapping, `glog.V(N).Infof` / `glog.Warningf` / `glog.Errorf` for logs, counterfeiter mocks in `pkg/mocks/`, Ginkgo v2 + Gomega for tests, external `_test` packages.
- Frontmatter contract is FROZEN (already enforced by prompts 1 + 5).
- `lib/repoallowlist` carried verbatim — no domain logic change.
- Cursor file format is JSON via `encoding/json`; atomic write via temp-file + rename (already in `cursor.go`). Stays compatible with existing `/data/cursor.json` PVC mount path.
- Mocks regenerate via existing `make generate` target — no new tooling.
- No `context.Background()` in production paths EXCEPT the single `service.Main(context.Background(), ...)` entry point in `main.go` (Go services convention).
- Pre-init Prometheus counter label combinations to `.Add(0)` per `coding-guidelines/go-prometheus-metrics-guide.md` — already done in `pkg/metrics.go` `init()`; do not duplicate.
- No new capabilities beyond the Phase 1 prototype scope (Stage 2 anti-pattern guard). Specifically: do NOT add per-repo parallelism, do NOT add a "retry on rate-limit within the cycle" loop, do NOT add an HTTP `/trigger` handler. The spec explicitly defers these.
- 5xx during ListRepos or per-repo fetch aborts the cycle WITHOUT cursor save — per failure-mode table row.
- Per-repo `GetMasterSHA` or `GetChangelogContent` transient errors prune that repo via `glog.V(2).Infof("repo dropped from cycle: ...")` log line (operator greps this). `IncFilterSkipped` is NOT emitted — this is a fetch failure, not a filter skip.
- Per-repo `GetChangelogContent` 404 (no CHANGELOG.md) is the documented normal path → `ChangelogSummary{UnreleasedBullets:0}` → EmptyUnreleasedFilter skips → metric `empty_unreleased`. Verified by Poll test (b) above using `nil` content.
- Cursor `SaveCursor` failure post-publish: log warn, do NOT return error from `Poll` — tasks already published; controller dedup absorbs.
- Kafka `SendCommand` failure surfaces as `publisher.PublishCreate(...)` returning false → cursor NOT updated for that repo → re-publish next cycle.
- Do NOT commit — dark-factory handles git.
- Touch only: `pkg/watcher.go`, `pkg/watcher_test.go`, `pkg/factory/factory.go`, `main.go`, plus auto-generated `pkg/mocks/*.go` via `make generate`. If touching `pkg/metrics.go` becomes necessary (it should not), STOP and surface — the metric labels are spec-frozen.
</constraints>

<verification>
```bash
cd watcher/github-release

# Spec AC sweep — must all pass

# 1. make precommit clean
make precommit

# 2. No TODO remaining anywhere
grep -rn "TODO" . --include='*.go'
# Expected: 0 lines

# 3. Mock regen deterministic
make generate && git diff --exit-code pkg/mocks

# 4. All named acceptance-criterion It blocks present verbatim
grep -rF "Poll publishes one task per non-skipped repo and saves cursor"            pkg/watcher_test.go
grep -rF "BuildCreateCommand produces frontmatter task_type github-release for bborbe/docker-utils d630ef3" pkg/taskpublisher_test.go
grep -rF "EmptyUnreleasedFilter skips when UnreleasedBullets is 0"                  pkg/filter/empty_unreleased_filter_test.go
grep -rF "AutoReleaseFilter skips when AutoRelease is true"                         pkg/filter/auto_release_filter_test.go
grep -rF "SHAUnchangedFilter skips when LastSeenSHA equals HeadSHA"                 pkg/filter/sha_unchanged_filter_test.go
grep -rF "SHAUnchangedFilter emits when LastSeenSHA differs from HeadSHA"           pkg/filter/sha_unchanged_filter_test.go
grep -rF "DeriveTaskID is deterministic for identical inputs"                       pkg/taskid_test.go
grep -rF "ParseChangelog handles Unreleased at bottom with mixed v-prefix"          pkg/changelog_test.go
grep -rF "SaveCursor + LoadCursor round-trip preserves Repos map"                   pkg/cursor_test.go

# 5. No context.Background() outside _test.go EXCEPT main.go's service.Main entry
grep -rn "context.Background()" . --include='*.go' | grep -v _test.go
# Expected: at most 1 line in main.go (service.Main entry point)

# 6. No fmt.Errorf in production
grep -rn "fmt.Errorf" . --include='*.go' | grep -v _test.go
# Expected: 0 lines

# 7. Prometheus label pre-init still in place
grep -A2 'publishedTotal.WithLabelValues' pkg/metrics.go
```
</verification>
