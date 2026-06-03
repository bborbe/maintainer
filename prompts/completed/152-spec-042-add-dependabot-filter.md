---
status: completed
spec: [042-github-build-watcher-filter-dependabot-graph-update]
summary: 'Added Dependabot graph-update workflow filtering to watcher/github-build: skip workflows named Graph Update: or Dependabot Updates from red/green state machine'
container: maintainer-exec-152-spec-042-add-dependabot-filter
dark-factory-version: v0.169.0
created: "2026-05-24T21:30:00Z"
queued: "2026-05-24T20:37:11Z"
started: "2026-05-24T20:37:13Z"
completed: "2026-05-24T20:38:39Z"
branch: dark-factory/github-build-watcher-filter-dependabot-graph-update
---

<summary>
- Add two hardcoded string constants for Dependabot internal workflow prefixes: `"Graph Update:"` and `"Dependabot Updates"`
- Add `isDependabotGraphUpdateWorkflow` predicate function used inside `deriveState` to filter these runs before the failing runs list is built
- Filter is applied inside the `considered` loop (after `latestByWorkflow` deduplication, before `failingRuns` accumulation), ensuring Dependabot runs do not affect red/green state machine or task emission
- Empty/nil `run.Name` is treated as non-matching (no crash, no skip)
- Constants are package-level vars in `pkg/watcher.go`, not configurable
</summary>

<objective>
Add Dependabot internal workflow filtering to `watcher/github-build` so that workflow runs named `Graph Update:` or `Dependabot Updates` (prefix match, case-sensitive) are silently excluded from the failing-runs list used by the state machine.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read fully before making changes:
- `watcher/github-build/pkg/watcher.go` — understand `deriveState` function structure where the filter must be inserted
- `watcher/github-build/pkg/githubclient.go` — confirm `WorkflowRun.Name` field exists and is a plain `string` (not pointer)
- `watcher/github-build/pkg/watcher_test.go` — understand the Ginkgo test patterns and mock infrastructure
</context>

<requirements>

**Execute steps in order. Run `make test` after step 2. Run `make precommit` only at the final step.**

1. **Read `watcher/github-build/pkg/watcher.go`** — focus on the `deriveState` function (line numbers ~228-269 at spec time, may drift; anchor by the literal `latestByWorkflow := make(map[int64]WorkflowRun)` initialization). Understand:
   - `latestByWorkflow` map construction (de-duplicates by WorkflowID)
   - `considered` slice construction (filters to `failure` or `success` conclusions)
   - `failingRuns` accumulation (only `failure` conclusions)
   - `deriveState` returns `(state, episodeSHA, failingRuns)`
   - The filter goes inside the `for _, run := range latestByWorkflow` loop, immediately inside the existing `if run.Conclusion == "failure" || run.Conclusion == "success"` block, so Dependabot runs are skipped before they are appended to `considered`

   **`strings` package is already imported** at `watcher.go:12` — no new import needed.

2. **Add constants and filter function to `watcher/github-build/pkg/watcher.go`**

   Add these as package-level vars near the top of the file (after imports, before `deriveState`):

   ```go
   // DependabotGraphUpdatePrefixes are workflow-name prefixes used by Dependabot for
   // internal graph-maintenance jobs. These are NOT real CI failures — their HTTP 503s
   // are Dependabot's own service being temporarily flaky. The real CI workflows on
   // the same commits succeed. These runs must not trigger OpenClaw build-failure tasks.
   var DependabotGraphUpdatePrefixes = []string{
       "Graph Update:",
       "Dependabot Updates",
   }
   ```

   Add this predicate function (place it before `deriveState`):

   ```go
   // isDependabotGraphUpdateWorkflow returns true when run.Name starts with any
   // prefix in DependabotGraphUpdatePrefixes. Comparison is case-sensitive.
   // An empty or zero Name is NOT considered a Dependabot workflow — returns false.
   func isDependabotGraphUpdateWorkflow(run WorkflowRun) bool {
       if run.Name == "" {
           return false
       }
       for _, prefix := range DependabotGraphUpdatePrefixes {
           if strings.HasPrefix(run.Name, prefix) {
               return true
           }
       }
       return false
   }
   ```

3. **Modify `deriveState` to skip Dependabot graph-update workflows**

   In the `for _, run := range latestByWorkflow` loop, **inside** the existing `if run.Conclusion == "failure" || run.Conclusion == "success"` block, add the Dependabot-filter check **before** the existing `considered = append(considered, run)` call. After this change the loop body looks like:

   ```go
   // Filter: only "failure" or "success" conclusions
   var considered []WorkflowRun
   for _, run := range latestByWorkflow {
       if run.Conclusion == "failure" || run.Conclusion == "success" {
           // Skip Dependabot internal graph-maintenance workflows.
           // They are not real CI and must not affect the red/green state machine.
           if isDependabotGraphUpdateWorkflow(run) {
               glog.V(4).Infof("skipping workflow run id=%d name=%q (Dependabot graph-update)", run.RunID, run.Name)
               continue
           }
           considered = append(considered, run)
       }
   }
   ```

   Concretely: add 4 new lines (the comment + the `if` block with `continue`) inside the existing outer `if`, immediately before the existing `considered = append(considered, run)`.

4. **Verify the change compiles** by running `make test`:

   ```bash
   cd watcher/github-build && make test
   ```

   All existing tests must still pass. If they do not, diagnose and fix before proceeding.

5. **Run `make precommit`**:

   ```bash
   cd watcher/github-build && make precommit
   ```

</requirements>

<constraints>
- Only edit files under `watcher/github-build/`
- Do NOT commit — dark-factory handles git
- Prefix match is case-sensitive — `strings.HasPrefix` is used directly (no `strings.EqualFold`)
- Empty/nil `run.Name` must NOT cause a crash and must NOT cause a skip — the filter returns `false` for empty string
- The filter is applied AFTER `latestByWorkflow` deduplication (so a Dependabot run older than the latest real CI run on the same WorkflowID does not poison `latestByWorkflow`) and BEFORE the `failingRuns` accumulation
- Constants are package-level vars in `pkg/watcher.go` — no env var, no config struct field
- `make precommit` runs from `watcher/github-build/`, never at repo root
- Existing tests must keep passing
</constraints>

<verification>
cd watcher/github-build && make precommit

# Confirm constants exist:
grep -n "DependabotGraphUpdatePrefixes" watcher/github-build/pkg/watcher.go

# Confirm filter function exists:
grep -n "isDependabotGraphUpdateWorkflow" watcher/github-build/pkg/watcher.go

# Confirm filter is called inside deriveState (not just defined as a stray helper):
awk '/^func deriveState/,/^}/' watcher/github-build/pkg/watcher.go | grep "isDependabotGraphUpdateWorkflow"
# Expected: ≥1 line

# Confirm strings.HasPrefix is used:
grep "strings.HasPrefix" watcher/github-build/pkg/watcher.go
</verification>