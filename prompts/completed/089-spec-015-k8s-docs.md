---
status: completed
spec: [015-github-build-watcher-mvp]
summary: Created watcher/github-build/k8s/ manifests (StatefulSet, Secret, Service, Makefile), docs/build-watcher.md with episode-SHA semantics and state machine, and scenarios/016-build-watcher-end-to-end.md covering detect/idempotency/recover/new-episode sub-scenarios; CHANGELOG.md updated with Unreleased section.
container: maintainer-089-spec-015-k8s-docs
dark-factory-version: v0.148.4-3-gc45254a
created: "2026-05-05T21:00:00Z"
queued: "2026-05-05T21:18:21Z"
started: "2026-05-05T21:46:42Z"
completed: "2026-05-05T21:48:50Z"
---

<summary>
- New `watcher/github-build/k8s/` mirrors the PR watcher's k8s layout: a StatefulSet with an inline volumeClaimTemplate for cursor persistence, a Secret for the GitHub token, a Service for HTTP, and a Makefile — naming convention `maintainer-watcher-github-build`
- The StatefulSet pulls per-pod env directly via go-template (`{{"VAR" | env}}`) from `dev.env` / `prod.env` — same pattern as the PR watcher, no separate ConfigMap
- `dev.env` and `prod.env` are NOT modified for `REPO_ALLOWLIST` in v1: both watchers continue to share the existing `REPO_ALLOWLIST` env (the spec's "per-watcher scoping" goal is deferred to a follow-up — documented as a known deviation)
- `docs/build-watcher.md` documents episode-SHA semantics + state machine with the worked example, per-repo vs per-workflow granularity rationale, red/green derivation rules, and cold-start flood behavior — institutional memory that survives spec archival
- End-to-end scenario covers detect, idempotency, recover, and distinct new episode on a different SHA
- No Go code changes — no `make precommit` needed
</summary>

<objective>
Provide the operational layer for the build watcher: k8s manifests, environment variable wiring, durable documentation of episode-SHA semantics (the spec's institutional memory), and the integration scenario. These are the artifacts that survive after the spec is archived.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read before making any changes:
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` — full file; canonical StatefulSet pattern (node affinity, security context, probes, resources, inline `volumeClaimTemplates` for cursor persistence)
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml` — full file; Secret structure
- `watcher/github-pr/k8s/maintainer-watcher-github-pr-svc.yaml` — full file; Service structure
- `watcher/github-pr/k8s/Makefile` — full file; targets for k8s deploy
- `dev.env` and `prod.env` — flat `export VAR=value` lines; `REPO_ALLOWLIST` is shared globally (NOT per-watcher)
- `scenarios/006-watcher-author-trust-filter.md` — full file; canonical scenario structure to mirror
- `docs/architecture.md` — understand the pipeline so `docs/build-watcher.md` correctly references the flow

Key facts verified from the existing PR watcher k8s files:
- File naming convention: `maintainer-watcher-github-{pr,build}-{sts,secret,svc}.yaml` (resource name as filename prefix, NOT `github-build-watcher-*`)
- Image template: `{{"DOCKER_REGISTRY" | env}}/maintainer-watcher-{name}:{{"BRANCH" | env}}` (go-template via `{{ "VAR" | env }}`)
- Env injection: directly from `dev.env`/`prod.env` via `{{"VAR" | env}}` template substitutions in the StatefulSet — there is NO ConfigMap and NO ServiceMonitor; Prometheus scrape uses pod annotations (`prometheus.io/scrape: "true"`, `prometheus.io/port: "9090"`)
- Cursor persistence: inline `volumeClaimTemplates` (mountPath `/data`, name `datadir`, storage `100Mi`, `local-path` storageClass, RWO) — NOT a separate PVC manifest
- No separate ConfigMap manifest in PR watcher's k8s/
- No separate ServiceMonitor manifest in PR watcher's k8s/
- StatefulSet: replicas 1 (single-writer for the cursor)
- Resources: requests 20m CPU / 20Mi RAM; limits 200m / 100Mi
- Service port: 9090 (metrics + `/healthz` + `/readiness` + `/trigger`)
- Build watcher MUST NOT have `REPO_SCOPE`, `TRUSTED_AUTHORS`, `BOT_ALLOWLIST` env vars

**Known deviation (v1):** the spec's Constraint "no shared ConfigMap; separate Pod env namespaces" is NOT honored in v1. Both watchers share the existing `REPO_ALLOWLIST` env injected from `dev.env`/`prod.env`. Splitting per-watcher requires a new env naming convention (e.g. `BUILD_REPO_ALLOWLIST`, `PR_REPO_ALLOWLIST`) and is deferred to a follow-up spec. This prompt documents the deviation in `docs/build-watcher.md` instead of introducing the convention.
</context>

<requirements>
**Execute all steps. No `make precommit` needed (no Go changes).**

1. **Determine the next available scenario number:**
   ```bash
   ls scenarios/ | grep -E '^[0-9]+-' | sort | tail -5
   ```
   Use the next integer after the highest existing scenario file number as `NNN`. Confirm uniqueness — there are existing duplicates at 006, do not reuse any number.

2. **Create `watcher/github-build/k8s/` directory with these four files** — mirror PR watcher's k8s/ verbatim, substituting `pr` → `build`:

   **`watcher/github-build/k8s/Makefile`** — copy `watcher/github-pr/k8s/Makefile` verbatim, replacing `pr` with `build` in service-name references.

   **`watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml`** — StatefulSet. Copy `watcher/github-pr/k8s/maintainer-watcher-github-pr-sts.yaml` verbatim, then change:
   - All `maintainer-watcher-github-pr` strings → `maintainer-watcher-github-build`
   - Image: `{{"DOCKER_REGISTRY" | env}}/maintainer-watcher-github-build:{{"BRANCH" | env}}` (go-template, not `${DOCKER_REGISTRY}/...`)
   - Remove env vars not used by the build watcher: `TRUSTED_AUTHORS`. Keep: `LISTEN`, `GH_TOKEN` (from Secret), `KAFKA_BROKERS`, `STAGE`, `SENTRY_PROXY`, `REPO_ALLOWLIST`
   - Keep `volumeClaimTemplates` block (inline PVC at `/data`, name `datadir`, 100Mi, local-path) verbatim
   - Keep node affinity, security context, probes, resources verbatim

   **`watcher/github-build/k8s/maintainer-watcher-github-build-secret.yaml`** — Secret. Copy `watcher/github-pr/k8s/maintainer-watcher-github-pr-secret.yaml` verbatim, change name to `maintainer-watcher-github-build`. Keys: at minimum `GH_TOKEN` (mirror whatever the PR Secret uses; do not invent new keys).

   **`watcher/github-build/k8s/maintainer-watcher-github-build-svc.yaml`** — Service. Copy `watcher/github-pr/k8s/maintainer-watcher-github-pr-svc.yaml` verbatim, change name to `maintainer-watcher-github-build`.

   Do NOT create a separate ConfigMap, ServiceMonitor, PVC, PriorityClass, or ResourceQuota manifest — none exist in PR watcher k8s/.

3. **Do NOT modify `dev.env` or `prod.env`** in v1. The build watcher reads the existing `REPO_ALLOWLIST` shared with the PR watcher. Per-watcher scoping is a known deviation deferred to a follow-up spec (documented in step 5 below). If the build watcher needs a different `POLL_INTERVAL` than the default `5m`, default behavior is acceptable for v1 — do not introduce a new env var.

4. (intentionally empty — env file split deferred)

5. **Create `docs/build-watcher.md`**:

   This document is the durable institutional memory for the episode-SHA design decisions. It must cover all four required topics:

   ```markdown
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
   ```

6. **Create `scenarios/NNN-build-watcher-end-to-end.md`** (use the number from step 1):

   Model the structure on `scenarios/006-watcher-author-trust-filter.md` (frontmatter, Prerequisites, Sub-scenarios with Action + Expected + Cleanup, markdown checklist `- [ ]`).

   ```markdown
   ---
   status: draft
   spec: 015-github-build-watcher-mvp
   ---

   # Scenario NNN: build watcher end-to-end — detect, publish, recover, new episode

   Validates the four core behaviors of spec-015: the build watcher detects a red
   build, publishes a task within one poll interval; re-polls do not duplicate;
   after the build is fixed no new tasks are created; when the build goes red again
   with a *different commit*, a new distinct task is created.

   This is the required integration-seam test for spec-015. The GitHub Actions API
   and Kafka task materialization path cannot be exercised by unit tests alone.

   Target repo: `bborbe/go-skeleton` (the existing dev test-bed repo; already red
   on its default branch per the spec's AC).

   ## Prerequisites
   - [ ] Dev cluster is running and healthy
   - [ ] Build watcher deployed to dev namespace with spec-015 changes:
         ```bash
         kubectl get pods -n dev -l app=maintainer-watcher-github-build
         # Expected: 1/1 Running
         ```
   - [ ] `REPO_ALLOWLIST` (in `dev.env`) includes `github.com/bborbe/go-skeleton` (shared with PR watcher in v1)
   - [ ] Vault CLI available: `vault kv list secret/code-reviewer/tasks/` returns results
   - [ ] Access to `bborbe/go-skeleton` to trigger CI runs (push to default branch or re-run a workflow)

   ## Sub-scenario A: red build → vault task materializes within one poll interval

   ### Action
   - [ ] Confirm `bborbe/go-skeleton` default branch currently has a failing workflow:
         ```bash
         gh run list --repo bborbe/go-skeleton --branch master --status failure --limit 5
         # Expected: at least one failed run listed
         ```
   - [ ] Note the failing run's head SHA: `<episode-sha>`
   - [ ] Compute the expected task ID:
         ```bash
         cd watcher/github-build && go run ./cmd/run-once
         # Or compute manually: UUID5(namespace="8e3f5a2c-...", "bborbe/go-skeleton#build-<episode-sha>")
         ```
   - [ ] Wait one poll interval (≤ 5 min) or trigger manually:
         ```bash
         curl -s -X POST http://<build-watcher-service>:9090/trigger
         ```

   ### Expected
   - [ ] A vault task with `assignee: build-fixer-agent` appears:
         ```bash
         vault kv list secret/code-reviewer/tasks/ | grep -i build
         ```
   - [ ] The vault task `task_id` matches the expected UUID5
   - [ ] The task body contains the failing workflow name(s), run URL(s), and episode SHA:
         ```bash
         vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
           | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['data'].get('body',''))"
         # Expected: markdown body with ## Failing Workflows section and episode SHA
         ```
   - [ ] Build watcher pod log shows a `green_to_red` transition for `bborbe/go-skeleton`:
         ```bash
         kubectl logs -n dev <build-watcher-pod> | grep "state_transition\|green_to_red\|go-skeleton"
         ```

   ## Sub-scenario B: re-poll does not duplicate (idempotency)

   ### Action
   - [ ] Trigger a second poll cycle without changing the build state:
         ```bash
         curl -s -X POST http://<build-watcher-service>:9090/trigger
         ```
   - [ ] Wait for the poll to complete (check pod logs for "poll cycle complete")

   ### Expected
   - [ ] No new vault task was created (same task_id — controller would dedup anyway,
         but the watcher should skip the publish entirely):
         ```bash
         vault kv list secret/code-reviewer/tasks/ | grep -c build
         # Expected: same count as after sub-scenario A (no new entry)
         ```
   - [ ] Pod logs show no `green_to_red` transition in the second poll cycle:
         ```bash
         kubectl logs -n dev <build-watcher-pod> | grep "green_to_red" | wc -l
         # Expected: still 1 (from sub-scenario A only)
         ```

   ## Sub-scenario C: build fixed → no new tasks on subsequent polls

   ### Action
   - [ ] Fix the `bborbe/go-skeleton` build: push a commit that makes all workflows pass,
         or manually re-run a failed workflow after fixing the root cause
   - [ ] Wait for the GitHub Actions run to complete with `conclusion: success`
   - [ ] Trigger a poll cycle:
         ```bash
         curl -s -X POST http://<build-watcher-service>:9090/trigger
         ```

   ### Expected
   - [ ] Pod logs show a `red_to_green` transition:
         ```bash
         kubectl logs -n dev <build-watcher-pod> | grep "red_to_green\|go-skeleton"
         ```
   - [ ] No new vault task was published (no `CreateTaskCommand` for this poll):
         ```bash
         vault kv list secret/code-reviewer/tasks/ | grep -c build
         # Expected: same count — no new build task
         ```
   - [ ] Subsequent poll cycles also produce no new tasks (state is now green):
         ```bash
         curl -s -X POST http://<build-watcher-service>:9090/trigger
         vault kv list secret/code-reviewer/tasks/ | grep -c build
         # Expected: count unchanged
         ```

   ## Sub-scenario D: new red episode on different SHA → distinct task ID

   ### Action
   - [ ] Push a NEW breaking commit to `bborbe/go-skeleton` default branch (different from
         the commit in sub-scenario A), causing a fresh CI failure
   - [ ] Note the new failing run's head SHA: `<new-episode-sha>` (must differ from `<episode-sha>`)
   - [ ] Trigger a poll cycle:
         ```bash
         curl -s -X POST http://<build-watcher-service>:9090/trigger
         ```

   ### Expected
   - [ ] A NEW vault task appears with a DIFFERENT task_id than sub-scenario A:
         ```bash
         vault kv list secret/code-reviewer/tasks/ | grep build
         # Expected: 2 entries total (one from sub-scenario A, one new)
         ```
   - [ ] The new task body references `<new-episode-sha>`, not `<episode-sha>`
   - [ ] Pod logs show a new `green_to_red` transition for `bborbe/go-skeleton`

   ## Cleanup
   - [ ] Close or cancel any open vault tasks created during this scenario
   - [ ] Restore `bborbe/go-skeleton` to a green state if broken during sub-scenario D

   ## Notes
   Last run: (not yet run — scenario created for spec-015)
   ```

7. **Verify all created files are non-empty and well-formed:**
   ```bash
   ls -la watcher/github-build/k8s/
   ls -la docs/build-watcher.md
   # Show the NNN value used:
   ls scenarios/ | sort | grep build-watcher

   # Manifest schema sanity check (no apply, just dry-run validation):
   kubectl apply --dry-run=client -f watcher/github-build/k8s/ 2>&1 | head
   ```
</requirements>

<constraints>
- Only create/edit files in `watcher/github-build/k8s/`, `docs/build-watcher.md`, and `scenarios/`
- Do NOT edit `dev.env`, `prod.env`, any Go source files, or `CHANGELOG.md` in this prompt
- Do NOT commit — dark-factory handles git
- No `make precommit` needed (no Go changes)
- k8s resource names MUST use convention `maintainer-watcher-github-build` (NOT `github-build-watcher`)
- k8s manifest filenames MUST be `maintainer-watcher-github-build-{sts,secret,svc}.yaml` (resource-name-prefixed; mirrors PR watcher convention)
- Image template MUST be `{{"DOCKER_REGISTRY" | env}}/maintainer-watcher-github-build:{{"BRANCH" | env}}` (go-template substitution; NOT `${DOCKER_REGISTRY}/...`)
- All env injection MUST be `{{"VAR" | env}}` go-template; do NOT introduce a ConfigMap or `valueFrom: configMapKeyRef`
- Do NOT create a ConfigMap, ServiceMonitor, PriorityClass, ResourceQuota, or separate PVC manifest — none exist in PR watcher k8s/
- StatefulSet MUST have `replicas: 1` — two concurrent pods would cause duplicate task publishes
- Cursor persistence MUST be an inline `volumeClaimTemplates` entry (mountPath `/data`, name `datadir`, storage `100Mi`, local-path) — NOT a separate PVC manifest
- The docs MUST include the worked example table from spec Desired Behavior #6 (the t0–t4 sequence)
- Scenario file MUST use the actual next available unused number; check for collisions
- Do NOT alter existing scenario files
- Sub-scenario D MUST assert a DIFFERENT task_id than sub-scenario A (distinct episode SHA → distinct UUID)
</constraints>

<verification>
# Confirm k8s manifests created with correct filenames:
ls watcher/github-build/k8s/
# Expected: Makefile, maintainer-watcher-github-build-{sts,secret,svc}.yaml

# Confirm no spurious ConfigMap/ServiceMonitor/PVC files:
ls watcher/github-build/k8s/ | grep -E 'configmap|servicemonitor|pvc' && echo FAIL || echo OK

# Confirm StatefulSet name and replicas:
grep -n "name:\|replicas:" watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml | head -10
# Expected: maintainer-watcher-github-build, replicas: 1

# Confirm image template uses go-template syntax + correct image name:
grep -n "image:" watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml
# Expected: '{{"DOCKER_REGISTRY" | env}}/maintainer-watcher-github-build:{{"BRANCH" | env}}'

# Confirm volumeClaimTemplates inline (NOT a separate PVC):
grep -n "volumeClaimTemplates\|/data" watcher/github-build/k8s/maintainer-watcher-github-build-sts.yaml

# Confirm dev.env / prod.env unchanged:
git diff --stat dev.env prod.env
# Expected: no changes

# Confirm docs file exists and covers all required topics + deviation:
grep -n "Episode-SHA\|Per-Repo\|Red/Green\|Cold-Start\|Known Deviations" docs/build-watcher.md

# Confirm worked example table in docs:
grep -n "t0\|t1\|t2\|t3\|t4" docs/build-watcher.md

# Confirm scenario file created with correct spec reference:
grep "spec:" scenarios/*build-watcher*.md
# Expected: spec: 015-github-build-watcher-mvp

# Confirm scenario has all 4 sub-scenarios:
grep "Sub-scenario" scenarios/*build-watcher*.md
# Expected: A, B, C, D
</verification>
