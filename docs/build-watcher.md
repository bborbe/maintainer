# Build Watcher

The `watcher/github-build` service polls the GitHub Actions API for CI failures
on default branches and emits vault tasks for automated remediation.

## Episode-SHA Semantics and State Machine

The watcher tracks a per-repo state: `green` or `red`. When a repo transitions
from green to red, an *episode* begins. The episode is anchored to the SHA of
the **earliest failing commit** in the current red set — the `episode_sha`.

This design ensures:
- The same broken commit always produces the same task ID (`UUID5(namespace, "owner/repo#build-SHA")`)
- Re-polls while the build is still broken do not generate duplicate tasks
- Layered failures (a second bad commit on top of an unfixed first) stay within the same episode

### State Machine Table

| prev state | curr state | action |
|---|---|---|
| `green` (or cold start) | `red` | publish `CreateTaskCommand`, set episode SHA |
| `red` | `red` (any SHA) | skip — episode locked on first red SHA |
| `red` | `green` | clear episode SHA, set green; no closure published (see follow-up spec) |
| `green` | `green` | nothing |
| any | undefined (zero runs) | skip |

### Worked Example

| Time | Event | State | Episode SHA | Action |
|---|---|---|---|---|
| t0 | repo is green | `green` | — | none |
| t1 | commit A breaks build | `red` | `A` | publish task `UUID5(repo#build-A)` |
| t2 | commit B layered, both A+B red | `red` | `A` (unchanged) | no publish |
| t3 | PR fixes both A+B → green | `green` | — | clear state, no closure |
| t4 | commit C breaks build | `red` | `C` | publish task `UUID5(repo#build-C)` — distinct from t1 |

Note: t1 and t4 produce **different** task IDs because the episode SHAs differ.
The controller deduplicates by `task_id`, so re-deploying the watcher on a red
repo publishes the same task ID (safe re-play).

## Why Per-Repo Granularity (Not Per-Workflow)

The watcher creates **one task per repo**, not one per failing workflow. Rationale:

- A repo's build is either "green enough to merge" or "broken" — the fix agent
  targets the repo, not individual workflows.
- Multiple failing workflows on the same commit are usually caused by the same
  root issue (a breaking API change, a missing dep update). A single fix PR
  addresses all of them.
- Per-workflow granularity would require the fix agent to coordinate across
  multiple tasks for the same repo — unnecessary complexity for v1.

Per-workflow granularity is a future refinement once the fix agent matures.

## Red/Green Derivation Rules

Given the latest completed workflow runs for a repo's default branch:

1. Group runs by `workflow_id`; keep only the **most recent run** per workflow
   (by `created_at` descending).
2. Filter: only count runs with `conclusion` in `{"failure", "success"}`.
   Skip `cancelled`, `timed_out`, `action_required`, `skipped`, `neutral`,
   `stale`, and runs still in progress (empty conclusion).
3. **Red**: any surviving run has `conclusion = failure`.
4. **Green**: all surviving runs have `conclusion = success`.
5. **Undefined**: zero surviving runs → repo skipped (not red, not green).

The episode SHA is the `head_sha` of the **earliest** (smallest `created_at`)
failing run in the current red set — anchoring the episode to the first commit
that broke anything.

## Cold-Start Flood Behavior

On first deploy (or after a cursor is lost), the watcher has no persisted state.
It treats every repo as `green`. On the first poll cycle, repos that are currently
red trigger a `green → red` transition and publish tasks.

If N repos are currently red on first deploy, N tasks are published in one cycle.
This **initial burst is expected and acceptable** because:
- Task IDs are deterministic (UUID5) — re-deploying the watcher republishes the
  same task IDs, which the controller deduplicates.
- The alternative (assume `red` on cold start and skip the first cycle) would
  lose all currently-red signal until a `red → green → red` transition, defeating
  the purpose of the detector.

Operators should not be surprised by an initial burst of tasks on first deploy.

## Known Deviations from Spec 015

**Per-watcher REPO_ALLOWLIST scoping (deferred).** Spec 015 calls for separate
per-watcher repo allowlists ("no shared ConfigMap"). v1 ships with both watchers
sharing the existing `REPO_ALLOWLIST` env var injected from `dev.env`/`prod.env`.
Splitting per-watcher requires a new env naming convention (`BUILD_REPO_ALLOWLIST`,
`PR_REPO_ALLOWLIST`) and corresponding code wiring; tracked as a follow-up.

## Per-Repo Configuration (`.maintenance.yaml`)

Each repo can provide a `.maintenance.yaml` at its root to override the watcher's
fleet-level defaults for its own tasks. The file is fetched fresh on every
`green → red` transition; no caching.

### Schema

```yaml
watcher:
  github-build:
    assignee: <string>   # overrides WATCHER_GITHUB_BUILD_TASK_ASSIGNEE env var
    status: <string>     # overrides WATCHER_GITHUB_BUILD_TASK_STATUS env var
    phase: <string>      # overrides WATCHER_GITHUB_BUILD_TASK_PHASE env var; empty = omit field
# Future: watcher.github-pr (PR watcher reads its own subtree)
# Future: agent.build-fixer.* (fixer agent reads its own subtree)
```

All keys are optional at every level. Each maintainer service reads **only** its own
subtree; the build watcher ignores `watcher.github-pr.*` and all `agent.*` keys.

### Override Precedence

```
.maintenance.yaml watcher.github-build.<key>  (per-repo, highest priority)
    > WATCHER_GITHUB_BUILD_TASK_ASSIGNEE / WATCHER_GITHUB_BUILD_TASK_STATUS / WATCHER_GITHUB_BUILD_TASK_PHASE env vars  (fleet-level)
        > hard-coded fallback (build-fixer-agent / todo / <empty>)
```

Precedence is **per-key**: a missing key in the file does not suppress the env-var
default for other keys. Empty string values (`assignee: ""`) are treated identically
to an absent key — the env-var default applies.

### Failure Modes

| Trigger | Behavior |
|---|---|
| File absent (HTTP 404) | Silent fall-through to env defaults — the common case |
| Malformed YAML | WARN log with parse error; publish with env defaults |
| Valid YAML, `watcher.github-build` subtree absent | Silent fall-through; subtree isolation by design |
| Valid YAML, unknown key inside `watcher.github-build` | INFO log "ignored unknown key"; known keys applied |
| `assignee: ""` (explicit empty) | Same as absent — env default applies |
| GitHub API 5xx fetching file | WARN log; publish with env defaults |
| File > 1 MiB | Reject as malformed; WARN log; env defaults applied |

Errors fetching `.maintenance.yaml` **never** prevent the task from being published.
The build-status signal is more important than the routing config.

### Example

To route `bborbe/myrepo`'s build failures to a Go-specific fixer agent:

```yaml
# .maintenance.yaml (at repo root of bborbe/myrepo)
watcher:
  github-build:
    assignee: go-deps-fixer-agent
    status: todo
```

The next `green → red` transition on `bborbe/myrepo` publishes a task with
`assignee: go-deps-fixer-agent` instead of the fleet default.
