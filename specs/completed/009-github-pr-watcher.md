---
status: completed
approved: "2026-04-27T20:42:49Z"
generating: "2026-04-27T20:42:50Z"
prompted: "2026-04-27T20:50:34Z"
verifying: "2026-04-27T21:33:38Z"
completed: "2026-05-03T18:28:35Z"
branch: dark-factory/github-pr-watcher
---

## Summary

- A new long-running service polls GitHub on a configurable interval and emits a Kafka `CreateTaskCommand` per new pull request in scope, so the existing PR-reviewer agent gets triggered automatically without human task creation
- Default repo scope is `bborbe/*` (configurable via env); default poll interval is 5 minutes (configurable)
- Filters out drafts and bot-authored PRs by default; an allowlist of bot user names is env-configurable
- Re-triggers a review when an open PR receives new commits by emitting an `UpdateFrontmatterCommand` that resets phase + appends an outdated marker to the task body
- The controller (single writer to the vault) owns task creation and mutation — the watcher never touches vault git directly

## Problem

The PR-reviewer agent at `agent/pr-reviewer/` works mechanically (v0.14.2 deployed to dev) but only runs when a human manually creates a vault task pointing at a PR. For the agent to deliver daily value across all `bborbe/*` repos as scoped in [GitHub PR Reviewer](../../../Obsidian/Personal/23%20Goals/GitHub%20PR%20Reviewer.md), trigger has to be automatic. Without auto-trigger, the agent's existence is invisible to anyone who doesn't already know it exists.

## Goal

After completion, opening (or pushing to) a pull request in any in-scope `bborbe/*` repo causes the watcher to detect it within one poll interval, publish a `CreateTaskCommand` to Kafka, and the controller writes the resulting task into the OpenClaw vault. The existing pr-reviewer agent then picks the task up and reviews. No human action required between PR open and review running. The watcher itself never touches vault git or filesystem.

## Non-goals

- Webhook-based triggering — polling is the MVP; webhook is a possible future upgrade
- Bitbucket support — separate effort, separate spec, lives in [PR Reviewer Ideas](../../../Obsidian/Personal/23%20Goals/PR%20Reviewer%20Ideas.md)
- Posting verdicts back to the PR — the agent's job (separate spec, [GitHub PR Reviewer] Task A4)
- Reviewing the same PR twice on identical commits — explicit dedup
- Multi-tenant operation — one watcher pod per environment, single-pod by design
- Reviewing closed/merged PRs — only `state:open` PRs trigger
- Per-PR filter rules (labels, branches, assignees) — defer to v2 once we observe spam patterns

## Desired Behavior

1. The service runs as a persistent process, polling GitHub on a configurable interval (default: 5 minutes)
2. Each poll, the watcher issues a single GitHub Search API query covering ALL in-scope repos at once (e.g. `is:pr is:open archived:false user:bborbe`), using the configured cursor as a `updated:>=<cursor>` qualifier; results are paginated and consumed fully before advancing the cursor
3. On cold start (no cursor), the cursor is set to service-start-time — no historical PRs are picked up
4. For each candidate PR, the watcher applies filters: skip drafts; skip PRs authored by users in the configured bot allowlist (default: `dependabot[bot]`, `renovate[bot]`)
5. Dedup is handled by deriving a stable `task_identifier` from the PR's GitHub global ID (e.g. SHA-256 of `<owner>/<repo>#<number>`, formatted as UUID); the controller's `CreateTaskCommand` handler treats existing-identifier as a no-op so dedup is enforced server-side, not by the watcher querying vault state
6. If the PR has no existing local cursor entry: publish `CreateTaskCommand{taskIdentifier, body, frontmatter}` to Kafka. The controller writes the task file, commits, and pushes
7. If the PR head SHA changed since the last poll for the same task (tracked in the watcher's local cursor state): publish `UpdateFrontmatterCommand` that (a) resets `phase: planning`, `status: in_progress`, `trigger_count: 0`, and (b) appends a body section `## Outdated by force-push <old-sha>` via the command's `BodySection` field
8. After a successful poll, the cursor (last-seen-updated-at + last-seen-head-sha-per-task) is persisted to a PVC-backed file so restarts resume from the last seen state
9. All filter, scope, interval, and allowlist values come from env vars — no recompile to change behavior

## Constraints

- Lives at `watcher/github/` in `bborbe/code-reviewer` — new Go module with its own `go.mod`, sibling to (not under) `agent/pr-reviewer/` to reflect the producer-vs-consumer distinction. Future per-VCS watchers go in `watcher/<vcs>/` (e.g. `watcher/bitbucket/`)
- Authenticates to GitHub via a `GH_TOKEN` env (separate teamvault entry from the agent — watcher only needs `repo` read scope)
- Depends on `bborbe/agent/lib` for the Kafka command types (`CreateTaskCommand`, `UpdateFrontmatterCommand`) and CQRS publisher infrastructure
- Vault writes are NOT performed by the watcher — the controller (`bborbe/agent/task/controller`) is the sole writer to the vault. The watcher publishes commands; the controller applies them
- The pr-watcher MVP requires `CreateTaskCommand` from `bborbe/agent/lib >= v0.55.0` (shipped) and the corresponding controller executor (shipped via `bborbe/agent` v0.54.2). Pin the lib version explicitly in `go.mod`
- `task_identifier` is derived deterministically as a v5 UUID with a fixed namespace (e.g. `uuid.NewSHA1(prWatcherNamespace, []byte("<owner>/<repo>#<number>"))`) — same input always yields same identifier, RFC 4122 compliant, enabling controller-side dedup. The fixed namespace UUID lives as a package constant in the watcher
- Vault task schema written by the controller must match what the existing pr-reviewer agent already consumes — same frontmatter fields (`assignee: pr-reviewer-agent`, `phase: planning`, `status: in_progress`, `stage: <env>`, `task_identifier: <derived>`, `title: <PR title>`) plus a body containing the PR URL
- One pod per environment (single-replica K8s Deployment); no concurrent-watcher locking required
- Cursor stored at `/data/cursor.json` (PVC-backed) — schema: `{last_updated_at: timestamp, head_shas: {task_identifier: sha}}`. Survives pod restarts; corrupt or missing cursor falls back to service-start-time
- All GitHub API calls use `github.com/google/go-github` (latest stable major) + `golang.org/x/oauth2` for token authentication. The Search API endpoint (`client.Search.Issues`) covers all in-scope repos in a single query — avoid per-repo iteration
- The watcher MUST handle pagination via the response's `NextPage` field until all results are consumed
- The watcher MUST inspect rate-limit response headers and back off explicitly when remaining quota drops below a safe threshold — don't rely on retry-on-429 alone
- The container image needs neither `gh` CLI nor git binaries (much lighter than pr-reviewer's image)
- Errors wrapped via `github.com/bborbe/errors` (per `docs/dod.md`); structured logging via `glog`
- Tests use Ginkgo/Gomega; mock the GitHub API client boundary via `httptest.Server` (canned responses for Search, including pagination + rate-limit-header scenarios); use Counterfeiter for the Kafka producer boundary (interface name is a prompt-level decision)
- Configurable env vars: `POLL_INTERVAL` (default `5m`), `REPO_SCOPE` (default `bborbe`), `BOT_ALLOWLIST` (comma-separated, default `dependabot[bot],renovate[bot]`), `KAFKA_BROKERS` (no default — must be set), `STAGE` (no default — must be explicitly set per environment)
- Startup validation: fail-fast (process exits non-zero before any poll) if `KAFKA_BROKERS`, `STAGE`, or `GH_TOKEN` are unset. Defaults exist only for genuinely-optional behavior knobs (interval, scope, allowlist) — never for environment-routing fields where a silent default could publish commands to the wrong cluster
- Stale-task body marker MUST follow the exact format `## Outdated by force-push <old-sha>` so future tooling can grep for it
- A poll cycle that fails entirely (GitHub unreachable, auth fails, Kafka publish fails) must NOT crash the process — log error, retain cursor, retry next interval

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Empty result from GitHub query (no PRs) | Cursor advances to `now`, no tasks created | n/a |
| GitHub API client returns error on Search call | Logged error, cursor unchanged, retry next poll | Automatic |
| `GH_TOKEN` unauthorized (HTTP 401 from GitHub API) | Logged error with rotation guidance, cursor unchanged, retry next poll | Operator rotates teamvault entry |
| GitHub rate-limit hit (HTTP 429 / 403 with rate-limit headers, reactive) | Logged warning, cursor unchanged, retry next poll | Automatic next interval |
| Rate-limit headers show remaining quota below safe threshold (proactive) | Skip remainder of poll cycle, cursor unchanged, sleep until reset window, log warning with reset time | Automatic |
| Pagination fails partway through result set | Discard partial results from this poll, cursor unchanged, retry next poll | Automatic |
| Kafka publish fails (broker unreachable, timeout) | Logged error, cursor not advanced past failed PR, retry next poll | Automatic; persistent failure surfaces in logs |
| Controller rejects command (validation, malformed payload) | Logged error from publish ack, cursor not advanced, retry next poll | Operator investigates |
| Cursor file corrupt / unparseable | Treat as cold start (service-start-time), log warning | Automatic |
| Cursor file missing | Treat as cold start, log info | Automatic (first boot) |
| PR with no existing task and `CreateTaskCommand` publish fails | Cursor not advanced past this PR, retry next poll | Retry next interval |
| Same `task_identifier` already exists in vault | Controller-side no-op (idempotent CreateTask); watcher logs debug | n/a |
| Bot author matches allowlist | Skip PR, log debug with author + reason | n/a |
| Draft PR | Skip PR, log debug with reason | n/a |
| Force-push detected (existing task, new head SHA) | Update existing task body with outdated marker + reset frontmatter for re-review | Automatic |
| Filesystem write to PVC fails (disk full, permissions) | Log error, do not persist cursor, retry next poll | Operator investigates PVC |
| Context cancellation (pod shutdown signal) | Finish current poll cycle, persist cursor, exit cleanly | n/a |

## Security / Abuse Cases

- `GH_TOKEN` only needs `repo` read scope — minimum privilege; verify scope at startup (similar to pr-reviewer's preflight pattern)
- `go-github` call arguments (repo names, query strings, scopes) flow via typed struct fields and the library's URL-encoding — no shell interpolation, no exec subprocess; injection surface is closed by construction
- The watcher emits structured Kafka commands (typed Go structs) — no path traversal possible from PR metadata since the watcher never constructs file paths; the controller chooses the path from the validated `task_identifier`
- Task body content sanitizes PR title / description for any markdown injection that could break the task file's frontmatter parsing (the controller may also sanitize defensively when materializing the file)
- The PR-reviewer agent eventually consumes these task bodies and feeds them to Claude — if a PR title contains prompt-injection content, the agent inherits that risk. Document this constraint; do not attempt content-level sanitization in the watcher (premature and ineffective)

## Acceptance Criteria

- [ ] Service exists at `watcher/github/` with its own `go.mod`, `Dockerfile`, `Makefile`, and `k8s/` manifests
- [ ] `main.go` reads env vars, starts the polling loop, handles SIGTERM gracefully
- [ ] Filters (drafts, bot allowlist) and deterministic `task_identifier` derivation implemented and unit-tested
- [ ] Re-trigger on new head SHA emits an `UpdateFrontmatterCommand` with the `## Outdated by force-push <old-sha>` body section + frontmatter reset
- [ ] GitHub API client tested via `httptest.Server` with pagination + rate-limit scenarios; Counterfeiter fake at the Kafka-producer boundary (interface name is a prompt decision)
- [ ] Unit tests cover all rows of the Failure Modes table
- [ ] `make precommit` passes for `watcher/github/`
- [ ] Released as `code-reviewer v0.16.0`
- [ ] `go.mod` pins `github.com/bborbe/agent/lib >= v0.55.0` (the version exporting `CreateTaskCommand`)
- [ ] **E2E scenario** (HTTP integration seam to GitHub + Kafka publish seam to controller — required per spec-writing.md): deploy v0.16.0 to dev; create a test PR in `bborbe/code-reviewer`; observe within 5 minutes that (a) the watcher publishes a `CreateTaskCommand` (visible in watcher logs), (b) the controller materializes a vault task with `assignee: pr-reviewer-agent`, (c) the existing pr-reviewer agent picks it up and runs the 3-phase pipeline, (d) verdict is posted to the task body, (e) closing the test PR does not create new tasks, (f) opening a draft PR is skipped, (g) opening a PR authored by a bot in the allowlist is skipped, (h) force-pushing to an existing PR appends the outdated marker and re-triggers — confirms polling, filtering, controller-side dedup, Kafka publish, controller-side vault write, and downstream-trigger seams all work end-to-end

## Verification

```sh
cd watcher/github && make precommit
```

Manual verification in dev (after deploy):

```sh
# Watcher running
kubectlquant -n dev get pods -l app=github-pr-watcher

# Logs show poll cycles + Kafka publishes
kubectlquant -n dev logs deploy/github-pr-watcher --tail=50

# Cursor persisted
kubectlquant -n dev exec deploy/github-pr-watcher -- cat /data/cursor.json
```

Then trigger a real PR (open + push + draft + bot-authored) in `bborbe/code-reviewer` and observe the vault for task creation per the E2E acceptance criterion.

## Do-Nothing Option

Stay manual: continue creating vault tasks by hand per PR. This is what blocks [GitHub PR Reviewer](../../../Obsidian/Personal/23%20Goals/GitHub%20PR%20Reviewer.md) from delivering its value — the agent works mechanically but daily-life invisible. The MVP cannot complete without auto-trigger; this spec is on the critical path.

A weaker alternative — having the watcher write directly to the vault git repo (clone + commit + push from the pod) — duplicates task-creation logic that already lives in the controller, requires SSH key management for pods, introduces a write-conflict race with the human's `obsidian-git` flow, and forces every future watcher (jira, calendar, market-event) to repeat the same git plumbing. The Kafka-command path centralizes vault writes in the controller and makes new producers trivial.

## Dependency

This spec depends on a sibling spec in the `bborbe/agent` repo that adds `CreateTaskCommand` (and an idempotent controller executor that no-ops on duplicate `task_identifier`). The dependent agent-side spec must ship first; pr-watcher cannot deploy without that lib version available.
