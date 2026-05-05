# Architecture

System overview and contracts for the code-reviewer pipeline. Specs reference this doc instead of redefining shared concepts.

## High-level pipeline

```
┌────────────┐  Kafka   ┌──────────────────┐  vault  ┌─────────────────┐
│  Watchers  │ ───────▶ │  Task Controller │ ──────▶ │  Agent (K8s Job)│
│            │ Create   │  (bborbe/agent)  │  task   │  pr-reviewer    │
│ - github   │ Task     │                  │  file   │                 │
│ - bitbucket│ Command  │                  │         │                 │
│   (future) │          │                  │         │                 │
└────────────┘          └──────────────────┘         └────────┬────────┘
                                                              │
                                                              ▼
                                              /coding:pr-review (Claude plugin)
                                              ├─ go-quality
                                              ├─ go-security
                                              ├─ srp-checker
                                              └─ ...
                                                              │
                                                              ▼
                                              ┌─────────────────────────┐
                                              │  Verdict post-back      │
                                              │  gh pr review --approve │
                                              │  bb pr approve (future) │
                                              └─────────────────────────┘
```

## Components

### 1. Watcher (per provider)

- **One watcher per git server** (GitHub today, Bitbucket/GitLab future)
- Polls provider API for new/updated PRs (or branches in future)
- Filters drafts, bot authors, out-of-scope repos
- Dedupes by `pr_url + head_sha`
- Emits `CreateTaskCommand` to Kafka with the **agent contract** payload (see below)

Code: `watcher/<provider>/`

### 2. Task Controller (`bborbe/agent`)

- Consumes `CreateTaskCommand` from Kafka
- Materializes a vault task file with frontmatter carrying the agent contract
- Spawns a K8s Job per task (Pattern B)
- Routes the result back to the originating system (PR comment, ticket update, etc.)

Out of scope for this repo — see `bborbe/agent`.

### 3. Agent (`agent/pr-reviewer`)

K8s Job that consumes one task and produces one verdict. Three modes share the same core:

| Mode | Entry | Use case |
|---|---|---|
| Standalone CLI | `cmd/cli` | Ad-hoc local review |
| Local task runner | `cmd/run-task` | Dev loop |
| K8s Job | `main.go` | Autonomous, controller-spawned |

Three-phase execution: **planning → execution → ai_review**, each phase reads/writes the same workdir.

### 4. Verdict post-back

After ai_review verifies the verdict, a final step posts back to the source system:
- GitHub: `gh pr review --approve | --request-changes | --comment`
- Bitbucket Server: REST `participants/{user}/approve` (future)

Configurable per repo: `autoApprove: true` enables `approve`/`request_changes`; default is comment-only.

## Agent contract

The agent is intentionally **provider-agnostic** and **ref-agnostic**. Its inputs are:

| Field | Example | Notes |
|---|---|---|
| `clone_url` | `https://github.com/bborbe/maintainer.git` | Any git server reachable with available credentials |
| `ref` | `feature/foo`, `v1.2.3`, `abc123…` | Branch, tag, or SHA — checked out as the review target |
| `base_ref` | `master` | Used to compute the diff (`origin/<base>...HEAD`) |
| `task_id` | `bd4d883b-…` | Stable key for workdir, dedup, observability |

PR-specific glue (PR number, PR URL, author) lives **at the edges**:
- Watcher resolves a PR → `(clone_url, head_sha, base_ref, task_id)`
- Post-back step uses the original PR URL to comment on the right thing

This contract enables:
- Reviewing arbitrary branches (no PR required)
- Multiple git providers without agent code change
- Local CLI mode reuses the same workdir convention

## Workdir & repo storage

### Pattern: bare cache + ephemeral worktrees

```
/repos/<host>/<owner>/<repo>.git/   # bare clone, ONE per repo
/work/<task-id>/                    # worktree, ONE per task
```

Paths are **locked** — code uses `/repos` and `/work` from day one. The backing storage swaps over time (see Phased rollout) without code change. In step 1, both paths are unmounted directories in the container's writable layer (overlayfs, ephemeral). In step 2.5, `/repos` becomes a PVC mount and `/work` becomes an emptyDir mount.

Defaults are configurable per entry point (`WorkdirConfig` struct):
- K8s Job: `/repos`, `/work` (env override `REPOS_PATH`, `WORK_PATH`)
- Local task runner: repo-local `./.cache/repos`, `./.cache/work`
- Standalone CLI: `~/.cache/code-reviewer/repos`, `~/.cache/code-reviewer/work`

**First task on a repo:**
```sh
git clone --bare <clone_url> /repos/<host>/<owner>/<repo>.git
```

**Every task (review is read-only — no commits, pushes, or branches):**
```sh
REPO=/repos/<host>/<owner>/<repo>.git
git -C "$REPO" fetch --prune origin
git -C "$REPO" worktree add /work/<task-id> <ref>
cd /work/<task-id>
# ... execution phase reads files, runs sub-agents, computes verdict ...
git -C "$REPO" worktree remove /work/<task-id>
```

**Pod startup:**
```sh
# clean up any stale worktree admin entries from crashed pods
for r in $(find /repos -name '*.git' -type d); do
  git -C "$r" worktree prune
done
```

### Why this layout

- **Objects shared** across all worktrees of a repo — incremental cost per task ≈ MB, not GB
- **Worktree key = task-id** — no collisions even with concurrent reviews of the same branch
- **Cleanup** = remove a single `/tmp/work/<task-id>` directory; bare cache stays warm
- **Survives pod restart** because bare cache is on PVC — instant warm-up
- **Provider-agnostic path** — `<host>/<owner>/<repo>` works for any git server
- **Crash-safe** — `/tmp/work` wiped with pod, stale worktree admin entries pruned at startup

### Storage tiers

| Tier | Path | Backing today (step 1) | Backing target (step 2.5) | Persistence |
|---|---|---|---|---|
| Per-task workdir | `/work/<task-id>` | overlayfs (counts toward `ephemeral-storage`) | emptyDir | wiped on pod end |
| Per-repo bare cache | `/repos/<host>/<owner>/<repo>.git` | overlayfs (no caching benefit) | PVC | survives pod end (step 2.5+) |
| `.claude/` config | `/home/claude/.claude/` | existing PVC `agent-pr-reviewer` | unchanged | survives pod |

**Why locked paths from day one** — the agent task controller's `Config` CRD (`bborbe/agent`) currently supports only one PVC mount per agent, fixed at `/home/claude/.claude`. Adding a second PVC at `/repos` and an emptyDir at `/work` requires extending the CRD to accept multiple volume mounts (separate task: `Multi-volume support in agent Config CRD`). Until that lands, `/repos` and `/work` are unmounted directories backed by overlayfs — the cache logic runs but the cache never hits in K8s (every pod starts empty). The day the CRD ships, mount yaml is added; code does not change. CLI mode benefits from caching today because `/repos` is a real persistent directory on the developer's machine (configurable via `WorkdirConfig`).

### Eviction (future)

Bare cache grows unbounded. Mitigations (in order of preference):
1. Size cap on PVC — fail-loud when full, manual cleanup
2. LRU eviction by `mtime` of `<repo>.git/FETCH_HEAD`
3. TTL eviction (delete repos not touched in N days)

Not in scope for v1 — size generously, observe, then add eviction once growth pattern is known.

## Phased rollout

This architecture is the **target**. Migration happens in steps so each piece can be verified independently:

| Step | Workdir backing | Bare cache backing | Caching active in K8s? | Provider | Refs |
|---|---|---|---|---|---|
| 0 (today) | `gh pr diff` (no checkout) | none | n/a | GitHub only | PR only |
| 1 (this task) | overlayfs `/work/<task-id>` | overlayfs `/repos/<host>/<owner>/<repo>.git` | no — overlayfs wiped per pod | GitHub | PR + branch |
| 2.5 (followup) | emptyDir `/work/<task-id>` | PVC `/repos/<host>/<owner>/<repo>.git` | yes — bare cache survives pod | unchanged | unchanged |
| 3 (later) | unchanged | unchanged | unchanged | + Bitbucket, GitLab | + tag, SHA |

Code uses `/repos` and `/work` paths in **all** steps. Only the K8s mount config changes at step 2.5. CLI mode caches from day one because `/repos` and `/work` are configurable and default to persistent paths under `~/.cache/`. Step 2.5 requires extending `bborbe/agent` `Config` CRD to support multiple volume mounts (PVC + emptyDir) — tracked as a separate task; not blocking step 1.

The agent contract is set in step 1 and does not change in later steps — only the storage backend swaps.

## Component contracts

### Watcher → Controller (Kafka)

```json
{
  "type": "CreateTaskCommand",
  "assignee": "pr-reviewer-agent",
  "stage": "dev|prod",
  "task_id": "<uuid>",
  "clone_url": "https://github.com/bborbe/maintainer.git",
  "ref": "<head_sha>",
  "base_ref": "master",
  "source": {
    "provider": "github",
    "pr_url": "https://github.com/bborbe/maintainer/pull/42",
    "pr_number": 42
  }
}
```

`source` is opaque to the agent — only used by the post-back step.

### Controller → Agent (vault task file)

Frontmatter carries the contract; body is human-readable instructions:

```yaml
---
assignee: pr-reviewer-agent
status: in_progress
stage: dev
task_identifier: <uuid>
clone_url: https://github.com/bborbe/maintainer.git
ref: <head_sha>
base_ref: master
source:
  provider: github
  pr_url: https://github.com/bborbe/maintainer/pull/42
---
Review the pull request.
```

### Agent → Controller (verdict)

JSON block at end of task body:
```json
{"verdict": "approve|request-changes|comment", "reason": "<one-liner>"}
```

Plus structured findings (file, line, severity, message) — see `pkg/verdict/`.

### Controller → Provider (post-back)

Provider-specific module reads the original `source` field and posts the verdict via that provider's API.

## Extensibility points

To add a new git provider:
1. New watcher under `watcher/<provider>/` — same `CreateTaskCommand` output
2. New post-back module — reads `source.provider`, dispatches to provider API
3. Agent unchanged — `clone_url` + `ref` cover any git server with credentials in env

To add a new review command (e.g. `/coding:security-review`):
1. New phase config in `pkg/factory` selects the command
2. Workflow markdown calls the command instead of `/coding:pr-review`
3. Verdict contract stays the same

## References

- `pkg/git/` — `WorktreeManager` interface (CLI-mode today, k8s-mode after step 1)
- `pkg/prompts/` — 3-phase workflow markdown
- `pkg/factory/` — DI wiring + AllowedTools per phase
- `pkg/prurl/` — provider-agnostic PR URL parser
- `specs/` — dark-factory specs that change pieces of this architecture
