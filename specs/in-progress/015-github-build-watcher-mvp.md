---
status: generating
tags:
    - dark-factory
    - spec
approved: "2026-05-05T20:40:29Z"
generating: "2026-05-05T20:46:35Z"
branch: dark-factory/github-build-watcher-mvp
---

## Summary

- New service `watcher/github-build/` mirrors the existing `watcher/github-pr/` shape: poll loop + Kafka publisher + per-repo cursor + filter chain + `/healthz` + `/trigger`.
- Polls failed CI workflow runs on the **default branch** of every repo in `REPO_ALLOWLIST` (host-qualified, same syntax as PR watcher).
- Per-repo state: `last_known_state` (`green` | `red`) + `current_episode_sha` (the first failed commit since the last green). Episode SHA is the natural identity for the failure.
- On `green → red` transition: publish `CreateTaskCommand` with `task_id = UUID5(namespace, "<owner>/<repo>#build-<episode_sha>")`, `assignee: build-fixer-agent`, body containing repo, run URLs, failing workflow names, head SHA.
- Re-poll while red: same episode SHA → same task ID → idempotent (controller dedups). No new task spam.
- On `red → green`: state cleared. **Task closure is out of scope here** — covered by a follow-up spec (`github-build-watcher-close-on-green`).
- Granularity: one task per repo (not per workflow). Multiple workflows red simultaneously list all of them in the body.
- Failure type filter: only `conclusion = failure`. Skip `cancelled`, `timed_out`, `action_required`, `skipped`, `neutral`, `stale`.

## Problem

The "Auto-Fix Failed bborbe GitHub Builds" goal needs a detector half. Today the manual `/github-check-builds` runs on demand, scans 200+ repos, lists failures, then a human triages and dispatches the fix runbook. The detector half — converting "default branch is red" into a vault task an agent can pick up — does not exist. Without it the auto-fix pipeline cannot start: there's no signal source to feed the task-controller. This spec builds the signal source.

## Goal

After this work, a CI failure on the default branch of any allowlisted bborbe repo materializes as a vault task within one poll interval (default 5m), idempotent under repeated polls (same red episode = same task ID, no duplicates). When the build is fixed, no new task is created on subsequent polls (state correctly tracks the recovery). When a previously-fixed build goes red again with a different commit, a new distinct task is created (different episode SHA → different UUID). The watcher emits Prometheus metrics for poll-cycle counts, repos-checked, transitions detected, and tasks published, sufficient to alert on a stalled poll loop.

## Non-goals

- Closing tasks on `red → green` transition (separate spec: `github-build-watcher-close-on-green`)
- Fixing the build (separate spec: `build-fixer-agent`)
- Per-workflow task granularity (this spec uses per-repo only; granularity refinement is future work)
- Watching non-default branches, scheduled workflows, or `pull_request`-triggered runs (default branch only — that's where merges land and Dependabot fails)
- Bitbucket / GitLab parity
- Webhook-driven detection (poll only — matches `watcher/github-pr` pattern)
- Build-failure classification (the watcher publishes raw signal; classification is the agent's job in spec 3)
- Runbook dispatch
- Notification fan-out (Obsidian task IS the notification surface)

## Desired Behavior

1. Watcher starts up, validates `REPO_ALLOWLIST` (non-empty, host-qualified, all entries match `^[a-zA-Z0-9._/-]+$`), refuses to start if invalid.
2. Each poll cycle: for each repo in allowlist, fetch the latest workflow runs on the default branch via `GET /repos/{owner}/{repo}/actions/runs?branch=<default>&per_page=20&status=completed`. Default branch is fetched once per repo at startup and cached (refreshed on poll-cycle restart).
3. From the returned runs, the watcher derives the repo's current state: `red` if any of the latest *distinct workflows* (one entry per `workflow_id`, latest run wins) has `conclusion = failure`; `green` if all distinct workflows are `success`. Repos with zero completed runs are skipped (not `red`, not `green` — undefined).
4. On `green → red`: pick the **earliest red run's `head_sha`** in the current red set as `episode_sha`. Compute task ID. Publish `CreateTaskCommand` with `task_id`, `assignee: build-fixer-agent`, body listing failing workflows + run URLs + episode SHA. Update cursor: `last_known_state = red`, `current_episode_sha = <sha>`.
5. On `red → red` with **same** `episode_sha`: do nothing (idempotent — controller would dedup anyway, but skip the publish to save bandwidth).
6. On `red → red` with **different** `episode_sha` (new failure on top of unfixed old failure): treat as same episode (do not publish). The episode SHA is set once per red period and only resets on green. Rationale: a single fix is expected to clear both the original and the layered failures; if not, a follow-up watcher poll after partial fix would see the layered failure as a fresh red.

   **Worked example** (locks the rule against intuition that "different SHA = different task"):

   | Time | Event | `last_known_state` | `current_episode_sha` | Watcher action |
   |---|---|---|---|---|
   | t0 | repo green | `green` | `nil` | none |
   | t1 | commit A breaks build | `red` | `A` | publish task UUID5(`<repo>#build-A`) |
   | t2 | commit B layered, also red | `red` | `A` (unchanged) | no publish (same episode) |
   | t3 | commits A+B both fixed in one PR → green | `green` | `nil` | clear state (no closure published per spec scope) |
   | t4 | commit C breaks build | `red` | `C` | publish task UUID5(`<repo>#build-C`) — distinct from t1 |
7. On `red → green`: clear `current_episode_sha`, set `last_known_state = green`. **No task closure published.**
8. Cursor is persisted to disk (BoltDB or JSON, matching `watcher/github-pr` pattern). Cold start with no cursor: assume `last_known_state = green` for all repos so the first poll cycle creates tasks for currently-red repos. `BACKFILL_DURATION` is not relevant here (the signal is "is currently red", not "did it fail in window X").
9. Filter chain (`pkg/filter/`):
   - `RepoAllowlistFilter` — only check allowlisted repos
   - `ConclusionFilter` — only `conclusion = failure` counts as red; other terminal states (`cancelled`, `timed_out`, etc.) treated as `green` for state purposes
   - **No `BotAuthorFilter`** — runs on default branch are commits, not PRs; no author concept to filter on. Deviation from PR watcher's chain (which has the slot); the build watcher's chain is shorter on purpose.
10. `/trigger` HTTP endpoint forces an immediate poll cycle (matches `watcher/github-pr`).
11. Prometheus metrics: `github_build_watcher_poll_cycles_total`, `..._repos_checked_total`, `..._state_transitions_total{transition="green_to_red|red_to_green"}`, `..._tasks_published_total`, `..._poll_errors_total`, `..._current_red_repos` (gauge).

## Constraints

- **Mirror `watcher/github-pr` shape verbatim.** Same package layout (`pkg/`, `pkg/factory/`, `pkg/filter/`, `pkg/mocks/`), same `pkg/cursor` storage pattern, same `main.go` env-var schema, same `run.CancelOnFirstFinish` of poll loop + HTTP server, same Sentry/glog wiring. Diff against `watcher/github-pr/main.go` first; do not reinvent.
- **GitHub API client reuse.** Either share `pkg/github` between watchers (extract to `pkg/` at repo root once second consumer exists — exactly the lib-extraction trigger CLAUDE.md describes) or duplicate inline. Recommendation: duplicate for now; extract in a follow-up once the build watcher's needs are clear (PR client surface and Actions client surface diverge enough that premature extraction would add friction).
- **`assignee: build-fixer-agent`** is a NEW assignee value. The task-controller does not need to know about it for spec 1 — tasks materialize in vault regardless of whether a Config CRD exists for the assignee. Tasks without a matching Config simply stay in `todo` until spec 3 ships.
- **Episode SHA, not run ID.** Run IDs are unique per execution (re-run = new ID), but the user-facing failure is "this commit broke it." Using the SHA naturally collapses re-runs of the same broken commit into one task.
- **Default branch is per-repo, not global.** Most bborbe repos use `master`, but some use `main`. Fetch via `GET /repos/{owner}/{repo}` `default_branch` field. Cache per cursor entry (re-fetched on cold start).
- **REPO_ALLOWLIST format.** Same as `watcher/github-pr`: comma-separated `host/owner/repo`. Empty allowlist refuses startup (defense — without a list, the watcher would scan public GitHub indiscriminately or do nothing useful). Each watcher process owns its own `REPO_ALLOWLIST` env (no shared ConfigMap, no `BUILD_WATCHER_*` prefix) — same env name across watchers, separate Pod env namespaces.
- **Cold-start flood is acceptable.** First deploy with N currently-red repos publishes N tasks at once. Controller dedups on re-deploys (UUID5 deterministic). The alternative (assume `red` and skip first cycle) would lose all currently-red signal until a red→green→red transition, defeating the purpose. Operators should not be surprised by an initial burst on first deploy.
- **One-shot CLI entry point.** New `cmd/run-once/` for local smoke testing — no poll loop, no HTTP server, single cycle then exit. Deliberate addition over PR watcher's `/trigger`-only model because build-watcher tests need deterministic single-cycle runs against fixture repos. `/trigger` HTTP endpoint also exists in the long-running binary (parity with PR watcher).
- **Trusted authors not relevant** — runs on default branch are triggered by commits already merged. Trust-gating happens at PR-merge time, upstream of this watcher.
- **Image / k8s naming**: `maintainer-watcher-github-build` per the rename convention. Manifest layout mirrors `watcher/github-pr/k8s/` (StatefulSet for cursor PVC + ConfigMap + Service + ServiceMonitor).
- **Existing knowledge to reference**:
  - `watcher/github-pr/main.go` — env schema + startup flow
  - `watcher/github-pr/pkg/watcher.go` — poll cycle structure
  - `watcher/github-pr/pkg/cursor.go` — state persistence pattern
  - `watcher/github-pr/pkg/filter/` — filter chain pattern
  - `watcher/github-pr/pkg/taskid.go` — UUID5 derivation pattern
- **Domain knowledge to capture**: `docs/build-watcher.md` (required by AC) — covers episode-SHA semantics + state machine + worked example, per-repo granularity rationale, red/green derivation rules, cold-start flood behavior. Specs die after implementation; the doc is the durable home for these decisions so future maintainers don't re-derive them.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| GitHub API rate limit (5000/hr) | Poll cycle marks remaining repos skipped, increments `poll_errors_total{reason="rate_limit"}`, returns; next cycle retries | Auto-recover next interval; alert if >3 consecutive cycles rate-limited |
| GitHub API 5xx for one repo | Skip that repo, log warning, continue cycle for remaining repos | Self-heal next cycle |
| GitHub API 404 for repo (renamed/deleted) | Skip + log; do not crash the cycle | Operator removes repo from allowlist or restores it |
| Cursor disk write failure | Log + continue (in-memory state preserved for this process); next cycle retries write | Operator inspects PVC; eventual replay on restart |
| Cursor disk read failure on startup | Refuse to start (cannot guarantee idempotency without state) | Operator inspects PVC; cursor regenerates on next cold start |
| Repo has zero workflow runs ever | Treat as undefined (neither red nor green); skip | None — repo with no CI is not relevant |
| Default branch not found (rare, e.g. empty repo) | Skip + log; do not crash | None |
| Kafka publish failure | Log + record metric; do NOT update cursor (so next cycle retries publish) | Auto-retry next cycle |
| `episode_sha` already published this process lifetime but cursor lost it (PVC corruption) | Republish; controller dedups by `task_id` (UUID5 is deterministic) | Controller's idempotency layer absorbs |
| Two concurrent watcher pods (operator error) | Both publish same task IDs; controller dedups | Operator scales back to 1 pod (StatefulSet replicas: 1) |
| Workflow `conclusion` is `null` (still in progress) | Skip that run for state derivation; rely on completed runs only | None — incomplete runs revisited next cycle |
| First deploy with N currently-red repos publishes N tasks in one cycle | Expected — UUID5 deterministic, controller dedups subsequent re-deploys | None — see "Cold-start flood is acceptable" in Constraints |

## Security / Abuse Cases

- **GitHub PAT scope.** Read-only `repo` scope sufficient (Actions runs are part of repo metadata). The watcher never writes to GitHub. Same teamvault key shape as `watcher/github-pr` — recommend a separate PAT for build watcher to limit blast radius if compromised, but acceptable to share initially.
- **`REPO_ALLOWLIST` is the trust boundary.** Without it, a misconfigured watcher could scan unrelated repos or burn rate limit on unintended targets. Empty allowlist refuses startup (matches `watcher/github-pr`).
- **No new outbound network surface beyond api.github.com.** Same as PR watcher.
- **Task body is GitHub-API-derived strings (workflow names, URLs, SHAs).** Treated as opaque text in the task body; no shell interpolation. Vault task is markdown-rendered; no code execution path.
- **No write capability.** This service publishes Kafka commands and writes its own cursor PVC. It cannot trigger workflows, dismiss runs, or modify the repo. The agent (spec 3) will need write capability; this spec does not.

## Acceptance Criteria

- [ ] New module `watcher/github-build/` with own `go.mod`, mirroring `watcher/github-pr/` layout. Builds clean: `cd watcher/github-build && make precommit` passes.
- [ ] Poll loop fetches workflow runs on default branch for each repo in `REPO_ALLOWLIST`, derives current state from latest runs per workflow, publishes `CreateTaskCommand` on `green → red` transition with `task_id = UUID5(namespace, "<owner>/<repo>#build-<episode_sha>")` and `assignee: build-fixer-agent`.
- [ ] Idempotency verified: re-running poll cycle without state change produces zero new tasks (same task ID; controller dedups). Tested in `watcher_test.go` with a fixed cursor + repeated `Poll()` calls.
- [ ] State machine verified: `green → red` publishes; `red → red` (same episode SHA) skips publish; `red → red` (different SHA) skips publish (locked to first SHA); `red → green` clears state without publishing closure (closure is out of scope per Non-goals).
- [ ] Cursor persisted to disk; survives pod restart; cold start with no cursor assumes `green` for all repos so first cycle creates tasks for currently-red repos.
- [ ] `/trigger` HTTP endpoint forces immediate poll cycle (parity with `watcher/github-pr`).
- [ ] `/healthz`, `/readiness`, `/metrics` endpoints serve.
- [ ] Prometheus metrics emitted: `..._poll_cycles_total`, `..._repos_checked_total`, `..._state_transitions_total{transition}`, `..._tasks_published_total`, `..._poll_errors_total{reason}`, `..._current_red_repos`.
- [ ] k8s manifests in `watcher/github-build/k8s/`: StatefulSet (replicas: 1, PVC for cursor), ConfigMap (REPO_ALLOWLIST, POLL_INTERVAL), Secret (GH_TOKEN, SENTRY_DSN), Service, ServiceMonitor. Naming convention: `maintainer-watcher-github-build`.
- [ ] `dev.env` and `prod.env` extended with `REPO_ALLOWLIST=github.com/bborbe/go-skeleton` (dev) and the production repo set (prod) under the build-watcher's k8s manifest scope (per-watcher Pod env, not a shared ConfigMap).
- [ ] **Local smoke test** via `cmd/run-once/main.go`: target `bborbe/go-skeleton` (the existing dev test-bed repo, currently red on its default branch — same target as PR watcher's dev allowlist). Observe one task published to Kafka with the expected UUID5 and body.
- [ ] **Scenario coverage** (integration seam): `scenarios/NNN-build-watcher-end-to-end.md` exercises a real failed build on `bborbe/go-skeleton` (or seeded test repo) and asserts: (a) task materializes in vault within 1 poll interval, (b) re-poll doesn't duplicate, (c) when build is fixed (manual push), no new tasks on subsequent polls, (d) when build goes red again with a NEW commit, a new distinct task is created. Prompt-level / unit tests cannot fake the GitHub Actions API; the scenario is the only layer that runs the real path.
- [ ] `make precommit` passes in `watcher/github-build/`.
- [ ] CHANGELOG entry under `## Unreleased`.
- [ ] After dev deploy: `kubectlquant -n dev get pods -l app=maintainer-watcher-github-build` shows 1/1 ready; `kubectlquant -n dev logs <pod> | grep "poll cycle"` shows poll cycles every `POLL_INTERVAL`; current red repos surface as vault tasks within one cycle.
- [ ] `docs/build-watcher.md` exists and covers (a) episode-SHA semantics + state machine (with the worked example from Desired Behavior #6), (b) why per-repo not per-workflow granularity, (c) red/green derivation rules from latest-per-workflow runs, (d) cold-start flood behavior and why it's acceptable. The doc is the durable home for institutional memory after this spec is implemented and archived.

## Verification

```
cd watcher/github-build && make precommit
```

Local one-shot run against a known red repo (after seeding `dev.env`):

```
cd watcher/github-build && go run ./cmd/run-once -- repo bborbe/go-skeleton
```

Inspect: task appears in Kafka topic `tasks` with the expected UUID5, assignee `build-fixer-agent`, body listing failing workflows.

After deploy to dev:

```
kubectlquant -n dev logs <maintainer-watcher-github-build-pod> | grep "state_transition"
# Expect: green_to_red transitions for currently-red repos within one poll cycle.
```

## Do-Nothing Option

Stay with manual `/github-check-builds`. Failures pile up across 200+ repos until I run the slash command — typically once a week, sometimes longer when busy. The "Auto-Fix Failed bborbe GitHub Builds" goal cannot start: with no detector, there's nothing to dispatch. Dependabot PRs continue blocking on red CI; cascading failures from shared deps stay un-noticed. The full pipeline (detector + classifier + fixer) cannot be validated end-to-end without this signal source.

A weaker alternative — webhook-driven detection (GitHub Actions `workflow_run` event POSTed to a webhook endpoint) — saves the poll loop and gives instant signal. Cost: requires a public ingress endpoint for the webhook, webhook signature verification, GitHub App registration, and per-repo webhook configuration. The polling approach mirrors the existing PR watcher exactly (zero new infrastructure), is sufficient for the goal's "weekly chore" cadence, and the latency budget (10 min) is comfortably met by a 5-minute poll. Webhooks are a future optimization, not a v1 requirement.
