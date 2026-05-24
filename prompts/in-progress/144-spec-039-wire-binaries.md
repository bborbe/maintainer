---
status: approved
spec: [039-expand-watcher-github-build-org-wildcard]
created: "2026-05-24T10:00:01Z"
queued: "2026-05-24T10:05:07Z"
---

<summary>
- Wire the new `pkg/wildcard` package into both watcher entry points: `watcher/github-build/main.go` (long-lived daemon) and `watcher/github-build/cmd/run-once/main.go` (one-shot smoke-test binary).
- Long-lived daemon: build a `ResolvedAllowlist`, run `RunRefreshLoop` as a third concurrent task alongside the poll loop and the HTTP server. Initial refresh is best-effort; the watcher starts even if it fails (literal entries still poll).
- Run-once binary: when the allowlist contains a wildcard, perform a single synchronous expansion at startup. No goroutine — the binary exits after one poll cycle.
- Change the `Watcher` constructor (`pkg.NewWatcher`) to accept a snapshot provider instead of a `[]string` so the poll loop reads the current resolved set at the start of each cycle without holding a mid-cycle reference.
- Change `factory.CreateWatcher` to accept the snapshot provider; both entry points pass the new `ResolvedAllowlist`.
- Pure-literal allowlists (no `*` anywhere) cause zero API calls and zero new goroutines — `HasWildcard` short-circuits both setup paths.
- Add one CHANGELOG entry under `## Unreleased` referencing spec 039.
</summary>

<objective>
After this prompt, deploying the watcher with `REPO_ALLOWLIST=github.com/bborbe/*` produces a non-empty resolved set at startup, refreshes it hourly, and surfaces V(2) `wildcard_expanded` log lines. The pure-literal startup path is byte-identical to today — zero new API calls, zero new goroutines. Both binaries compile and `make precommit` exits 0.
</objective>

<context>
Read CLAUDE.md for project conventions.

**Read first:**
- `specs/in-progress/039-expand-watcher-github-build-org-wildcard.md` — full spec, especially Desired Behavior 7-8 and the Constraints block.
- The sibling prompt `spec-039-wildcard-package.md` (already executed) — defines `wildcard.NewExpander`, `wildcard.NewResolvedAllowlist`, `wildcard.HasWildcard`, `wildcard.ResolvedAllowlist.Snapshot`, `wildcard.ResolvedAllowlist.RunRefreshLoop`, `wildcard.ResolvedAllowlist.Refresh`.
- `watcher/github-build/main.go` — daemon entry point. Lines 67 (`Run` method start), 77-91 (allowlist parse + validate), 106 (`factory.CreateWatcher` call), 128-131 (`run.CancelOnFirstFinish` orchestrating the poll loop and HTTP server).
- `watcher/github-build/cmd/run-once/main.go` — one-shot binary. Lines 49 (`Run` method start), 50-63 (allowlist parse + validate), 78-89 (`factory.CreateWatcher` call), 95 (`w.Poll(ctx)`).
- `watcher/github-build/pkg/factory/factory.go` lines 49-84 — current `CreateWatcher` signature passes `allowlist []string` to both `filter.NewRepoAllowlistFilter` and `pkg.NewWatcher`.
- `watcher/github-build/pkg/watcher.go` lines 32-73 — `NewWatcher` constructor + `buildWatcher` struct. Line 81 (`for _, repoKey := range w.allowlist`) is the poll loop iteration source.
- `watcher/github-build/pkg/filter/repo_allowlist_filter.go` — `NewRepoAllowlistFilter(allowlist []string)` constructs the literal/wildcard predicate. This MUST continue to receive the ORIGINAL input allowlist (including the wildcard pattern) — `repoallowlist.IsAllowed` already matches wildcard entries against concrete `host/owner/repo` strings, so passing the input directly is correct AND a wildcard input matches the wildcard predicate even for fresh repos that appear later via refresh. Do NOT pass the expanded allowlist into the filter.
- `CHANGELOG.md` lines 1-6 — the file currently has `## v0.26.5` at the very top; an `## Unreleased` section must be inserted ABOVE `## v0.26.5`.

**`run.CancelOnFirstFinish` signature:** accepts a variadic list of `run.Func` (which is `func(ctx context.Context) error`). The wildcard refresh goroutine is wrapped as a `run.Func` and appended to the existing two-task list.

**Coding plugin docs (in-container paths):**
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-concurrency-patterns.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
</context>

<requirements>

1. **Introduce a snapshot-provider interface in `watcher/github-build/pkg/watcher.go`.**

   At package scope (above `NewWatcher`), declare:

   ```go
   // AllowlistSnapshot returns the current set of concrete "host/owner/repo"
   // entries the poll loop should iterate. Implementations MUST be safe to
   // call from a goroutine concurrent with a refresh writer.
   type AllowlistSnapshot interface {
       Snapshot() []string
   }
   ```

   This interface is satisfied by `*wildcard.ResolvedAllowlist.Snapshot()` (signature `Snapshot() []string`) — verified in the sibling prompt.

   Change the `NewWatcher` signature: REPLACE the `allowlist []string` parameter with `allowlist AllowlistSnapshot`. Inside the function body, store it as a struct field of type `AllowlistSnapshot` (rename the field from `allowlist []string` to `allowlist AllowlistSnapshot`).

   In `Poll`, change line 81:

   ```go
   // before:
   for _, repoKey := range w.allowlist {

   // after:
   snapshot := w.allowlist.Snapshot()
   for _, repoKey := range snapshot {
   ```

   The `snapshot` variable is captured ONCE at the top of `Poll` (after `LoadCursor`, before the loop) so a refresh that completes mid-cycle does not interrupt the current cycle (spec Desired Behavior 7).

2. **Provide a tiny static snapshot implementation for the pure-literal path.**

   In `watcher/github-build/pkg/watcher.go` (or a new file `pkg/static_snapshot.go` — either is fine; keep it in `package pkg`):

   ```go
   // StaticSnapshot is an AllowlistSnapshot backed by an immutable slice.
   // Used by the pure-literal binary path so no wildcard machinery runs.
   type StaticSnapshot struct {
       entries []string
   }

   // NewStaticSnapshot returns a snapshot holding a defensive copy of entries.
   func NewStaticSnapshot(entries []string) *StaticSnapshot {
       return &StaticSnapshot{entries: append([]string(nil), entries...)}
   }

   // Snapshot returns the held entry slice. Callers MUST NOT mutate it.
   func (s *StaticSnapshot) Snapshot() []string { return s.entries }
   ```

3. **Update existing `pkg.NewWatcher` callers in tests.**

   Search the package for `NewWatcher(` and update each call to pass a snapshot. Easiest local change: wrap the existing `[]string` in `NewStaticSnapshot(...)`. Files likely to need updating include:

   - `watcher/github-build/pkg/watcher_test.go`
   - `watcher/github-build/pkg/watcher_internal_test.go`

   Verify with `grep -rn "NewWatcher(" watcher/github-build/` and update each call. The buildWatcher struct field `allowlist []string` becomes `allowlist AllowlistSnapshot`; any test that reaches inside the struct (unlikely — most use the Watcher interface) must adapt.

4. **Update `watcher/github-build/pkg/factory/factory.go`** — `CreateWatcher` signature.

   Replace the `allowlist []string` parameter with TWO things: the original input slice (still needed by `filter.NewRepoAllowlistFilter`) AND the snapshot provider for `pkg.NewWatcher`:

   ```go
   func CreateWatcher(
       ctx context.Context,
       httpClient *http.Client,
       brokers libkafka.Brokers,
       stage string,
       inputAllowlist []string,        // raw input — passed to the filter
       resolved pkg.AllowlistSnapshot, // resolved snapshot — passed to NewWatcher
       cursorPath string,
       assignee string,
       taskStatus string,
       taskPhase string,
       maxTitleLen int,
   ) (pkg.Watcher, func(), error) {
       branch := base.Branch(stage)
       createSender, cleanup, err := CreateKafkaCreateSender(ctx, brokers, branch)
       if err != nil {
           return nil, nil, errors.Wrap(ctx, err, "create kafka create sender")
       }
       ghClient := pkg.NewGitHubClient(httpClient)
       maintenanceLoader := maintenance.NewLoader(ghClient)
       repoFilter := filter.RepoFilters{filter.NewRepoAllowlistFilter(inputAllowlist)}
       w := pkg.NewWatcher(
           ghClient,
           createSender,
           pkg.NewMetrics(),
           repoFilter,
           resolved,         // <-- snapshot provider, NOT inputAllowlist
           cursorPath,
           assignee,
           taskStatus,
           taskPhase,
           maintenanceLoader,
           maxTitleLen,
       )
       return w, cleanup, nil
   }
   ```

   The factory does NOT construct the `ResolvedAllowlist` itself — that is the binary's responsibility (because the daemon wires it into the run loop and the run-once binary uses a synchronous one-shot path). The factory just consumes whatever snapshot the caller provides.

   Also expose the `pkg.GitHubClient` from the factory so the binary can construct the expander with the SAME client the watcher uses. Add a helper:

   ```go
   // NewGitHubClient is a thin pass-through so binaries can build a
   // wildcard.Expander with the same client the watcher will use.
   func NewGitHubClient(httpClient *http.Client) pkg.GitHubClient {
       return pkg.NewGitHubClient(httpClient)
   }
   ```

   Alternatively (cleaner) the binary calls `pkg.NewGitHubClient(httpClient)` directly. Either approach is acceptable; pick whichever yields fewer factory import dependencies in the binaries. The verification step checks only that ONE client is shared between expander and watcher (no second `gogithub.NewClient` call per binary).

5. **Wire the daemon entry point `watcher/github-build/main.go`.**

   Inside `Run` (after the `httpClient` is resolved, before `factory.CreateWatcher`):

   ```go
   ghClient := pkg.NewGitHubClient(httpClient)

   var resolved pkg.AllowlistSnapshot
   var refreshTask run.Func
   if wildcard.HasWildcard(repoAllowlist) {
       expander := wildcard.NewExpander(ghClient)
       resolvedSet := wildcard.NewResolvedAllowlist(expander, repoAllowlist)
       resolved = resolvedSet
       refreshTask = func(ctx context.Context) error {
           return resolvedSet.RunRefreshLoop(ctx)
       }
       glog.V(2).Infof(
           "wildcard_refresh_enabled entries=%d (interval=%s)",
           countWildcards(repoAllowlist), wildcard.RefreshInterval(),
       )
   } else {
       resolved = pkg.NewStaticSnapshot(repoAllowlist)
       glog.V(2).Infof("wildcard_refresh_disabled allowlist=pure-literal")
   }
   ```

   The `ghClient` constructed here MUST be passed into `factory.CreateWatcher`. **Required signature** (no agent choice — pick this shape):

   ```go
   func CreateWatcher(
       ctx context.Context,
       ghClient pkg.GitHubClient,
       brokers libkafka.Brokers,
       stage string,
       inputAllowlist []string,
       resolved pkg.AllowlistSnapshot,
       cursorPath string,
       assignee string,
       taskStatus string,
       taskPhase string,
       maxTitleLen int,
   ) (pkg.Watcher, func(), error)
   ```

   The `httpClient *http.Client` parameter is REMOVED from `CreateWatcher` — the binary constructs the client once via `auth.Resolve` and passes the resulting `pkg.GitHubClient` (via `pkg.NewGitHubClient(httpClient)`) into the factory. Both binaries (`main.go` AND `cmd/run-once/main.go`) update their factory calls accordingly. Verification grep `grep -A12 "func CreateWatcher" watcher/github-build/pkg/factory/factory.go` MUST show `ghClient pkg.GitHubClient` as the second parameter and MUST NOT show `httpClient *http.Client`.

   Define a tiny helper in `main.go` for the count log:

   ```go
   func countWildcards(entries []string) int {
       n := 0
       for _, e := range entries {
           parts := strings.Split(strings.TrimSpace(e), "/")
           if len(parts) == 3 && parts[2] == "*" {
               n++
           }
       }
       return n
   }
   ```

   Add `"strings"` to the imports.

   Append the refresh task to the `run.CancelOnFirstFinish` list ONLY when it is non-nil:

   ```go
   tasks := []run.Func{
       a.runPollLoop(pollOnce, pollInterval),
       a.runHTTPServer(pollOnce),
   }
   if refreshTask != nil {
       tasks = append(tasks, refreshTask)
   }
   return run.CancelOnFirstFinish(ctx, tasks...)
   ```

   Imports to add: `"github.com/bborbe/maintainer/watcher/github-build/pkg/wildcard"`. The `pkg` import is already present.

6. **Wire the one-shot entry point `watcher/github-build/cmd/run-once/main.go`.**

   Same shape as the daemon EXCEPT no refresh goroutine — the binary exits after one `Poll`. After `httpClient` is resolved, before `factory.CreateWatcher`:

   ```go
   ghClient := pkg.NewGitHubClient(httpClient)

   var resolved pkg.AllowlistSnapshot
   if wildcard.HasWildcard(repoAllowlist) {
       expander := wildcard.NewExpander(ghClient)
       resolvedSet := wildcard.NewResolvedAllowlist(expander, repoAllowlist)
       if err := resolvedSet.Refresh(ctx); err != nil {
           glog.Warningf("initial wildcard refresh failed: %v", err)
           // Continue: literal entries still poll; wildcard contributes empty set.
       }
       resolved = resolvedSet
   } else {
       resolved = pkg.NewStaticSnapshot(repoAllowlist)
   }
   ```

   The `pkg` import must be added (currently absent in `run-once/main.go`). Verify with `grep "\"github.com/bborbe/maintainer/watcher/github-build/pkg\"" watcher/github-build/cmd/run-once/main.go`. Also add `"github.com/bborbe/maintainer/watcher/github-build/pkg/wildcard"` and `"github.com/golang/glog"` if not already imported.

   Pass `resolved` and `ghClient` (or `httpClient`, depending on the chosen factory shape) into `factory.CreateWatcher`.

7. **Update both factory call sites** to match the chosen `factory.CreateWatcher` signature from step 4/5. The two callers are:

   - `watcher/github-build/main.go` line 106 (daemon)
   - `watcher/github-build/cmd/run-once/main.go` line 78 (run-once)

   Verify with: `grep -rn "factory.CreateWatcher" watcher/github-build/` — exactly TWO matches expected.

8. **Verify the pure-literal cold-start path stays byte-identical.**

   The `else` branch in steps 5 and 6 must:
   - Construct ONLY `pkg.NewStaticSnapshot(repoAllowlist)` (a single allocation + slice copy).
   - NOT call any GitHub API method.
   - NOT start any goroutine.

   Add a regression assertion via a quick code review: in the daemon path, when `HasWildcard(repoAllowlist) == false`, no `wildcard.NewExpander` or `wildcard.NewResolvedAllowlist` call appears in the resulting AST.

9. **Verify all existing tests still pass.**

   ```bash
   cd watcher/github-build && go test ./...
   ```

   Likely affected test files (because they call `NewWatcher` or `factory.CreateWatcher`):
   - `watcher/github-build/pkg/watcher_test.go`
   - `watcher/github-build/pkg/watcher_internal_test.go`
   - any factory-level test (none currently exists per `ls watcher/github-build/pkg/factory/` showing only `factory.go` — confirm with `ls`).

   Fix each by:
   - Wrapping an existing `[]string` literal in `pkg.NewStaticSnapshot(...)` where the test passes an allowlist to `NewWatcher`.
   - Updating the order/types of arguments to `factory.CreateWatcher` calls in any test.

10. **Add a CHANGELOG entry.**

    The current `CHANGELOG.md` has `## v0.26.5` at line 3 (no `## Unreleased` block exists). Insert a new section ABOVE `## v0.26.5`:

    ```markdown
    ## Unreleased

    - fix(watcher/github-build): expand owner-level wildcard allowlist entries (e.g. `github.com/bborbe/*`) into concrete repos at startup and refresh hourly — closes the silent-zero-polls bug introduced by the v0.25.0 wildcard rollout (spec 039)
    ```

    Exactly one bullet, prefix `fix(watcher/github-build):`, mention spec 039.

11. **Run `make precommit`** in `watcher/github-build`:

    ```bash
    cd watcher/github-build && make precommit
    ```

    MUST exit 0.

</requirements>

<constraints>
- Refresh interval is hardcoded `time.Hour` via `wildcard.RefreshInterval()` (constant in the sibling package). Do NOT add a `POLL_INTERVAL`-style env var for refresh cadence (spec Non-goal line 33).
- DO NOT add any Prometheus metrics or new `WildcardMetrics` interface. Observability for spec 039 is V(2) log lines only.
- DO NOT add an opt-out flag (spec Non-goal line 34).
- DO NOT modify `lib/repoallowlist/repoallowlist.go` (spec Non-goal line 28).
- DO NOT remove the `repoallowlist.Validate` call in either binary — the spec says the parser/validator wildcard syntax is frozen and must still pass validation; the existing call at `main.go:82` and `cmd/run-once/main.go:55` stays as-is.
- DO NOT change `REPO_ALLOWLIST` env var name, format, or any other env var. The operator's `prod.env`/`dev.env` MUST work unchanged (spec Constraint line 49).
- DO NOT introduce any new env var.
- The pure-literal allowlist path (no `*` in any entry) MUST:
  - Make zero GitHub API calls beyond what the watcher already makes today.
  - Start zero new goroutines beyond the existing poll loop and HTTP server.
  - Allocate at most one slice copy (the `StaticSnapshot` defensive copy).
- All errors wrapped via `github.com/bborbe/errors`. No `fmt.Errorf`, no stdlib `errors.New`.
- `context.Background()` forbidden — always use the injected `ctx`.
- License header on any NEW file (none expected in this prompt — all edits are to existing files plus the in-place addition of `StaticSnapshot` which can live next to `NewWatcher` in `watcher.go` OR in a new `static_snapshot.go`; if the latter, add the header).
- `cmd/run-once/main.go` is a sibling entry point — verified to exist at `watcher/github-build/cmd/run-once/main.go` and to call `factory.CreateWatcher` at line 78. It MUST be updated alongside `main.go` in this same prompt.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass; `make precommit` must exit 0.
</constraints>

<verification>

```bash
# AllowlistSnapshot interface declared in pkg
grep -n "type AllowlistSnapshot interface" watcher/github-build/pkg/watcher.go
# Expected: exactly one match

# StaticSnapshot exists
grep -rn "func NewStaticSnapshot\|type StaticSnapshot" watcher/github-build/pkg/
# Expected: at least 2 matches (type + constructor)

# factory.CreateWatcher signature updated — should NOT have `allowlist []string` as the sole allowlist parameter
grep -A12 "func CreateWatcher" watcher/github-build/pkg/factory/factory.go | head -15
# Expected: contains both `inputAllowlist []string` AND `resolved pkg.AllowlistSnapshot`
# (or whichever names you picked; the principle is two-param separation)

# Both entry points import the wildcard package
grep -l "watcher/github-build/pkg/wildcard" watcher/github-build/main.go watcher/github-build/cmd/run-once/main.go
# Expected: both files listed

# Daemon adds the refresh task to run.CancelOnFirstFinish ONLY when HasWildcard
grep -B1 -A8 "HasWildcard(repoAllowlist)" watcher/github-build/main.go
# Expected: context shows an if/else with NewResolvedAllowlist branch + NewStaticSnapshot branch

# Run-once does a synchronous Refresh (no goroutine)
grep -n "Refresh(ctx)\|RunRefreshLoop" watcher/github-build/cmd/run-once/main.go
# Expected: at least one Refresh(ctx); NO RunRefreshLoop call

# Daemon uses RunRefreshLoop (goroutine), NOT a synchronous Refresh
grep -n "RunRefreshLoop\|resolvedSet.Refresh" watcher/github-build/main.go
# Expected: RunRefreshLoop present in main.go

# Exactly two factory.CreateWatcher callers
grep -rn "factory.CreateWatcher" watcher/github-build/
# Expected: exactly 2 matches (main.go + cmd/run-once/main.go)

# CHANGELOG entry under Unreleased
grep -B0 -A3 "^## Unreleased" CHANGELOG.md
# Expected: shows the spec-039 fix line

# No Prometheus additions for wildcard
grep -rn "wildcard" watcher/github-build/pkg/metrics.go
# Expected: zero matches

# REPO_ALLOWLIST env var unchanged
grep -n "REPO_ALLOWLIST" watcher/github-build/main.go watcher/github-build/cmd/run-once/main.go
# Expected: still present in both, no opt-out flag added nearby

# All tests pass
cd watcher/github-build && go test ./...
# Expected: PASS

# Full precommit
cd watcher/github-build && make precommit
# Expected: exit 0
```

</verification>
