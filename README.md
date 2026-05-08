# Maintainer

Autonomous repo-maintenance agents backed by Claude Code. Watchers detect signals (PRs, failed builds, alerts), Pattern B Job agents act on them.

Ships today:
- **`agent/pr-reviewer`** — Pattern B Job agent that reviews GitHub + Bitbucket Server pull requests
- **`watcher/github-pr`** — polls open PRs and emits create-task commands for each new PR
- **`watcher/github-build`** — polls GitHub Actions for failed CI runs on default branches and emits create-task commands on green→red transitions

Planned siblings: build-fixer agent (consume github-build tasks → propose fix PRs), dep-updater, sentry-triager, repo-reviewer.

## pr-reviewer (current)

Three modes:

| Mode | Entry | Use case |
|---|---|---|
| **Standalone CLI** | `agent/pr-reviewer/cmd/cli/main.go` | Ad-hoc local review — takes a PR URL, posts a comment back |
| **Local task runner** | `agent/pr-reviewer/cmd/run-task/main.go` | Dev loop for the agent — reads a task markdown file, writes result back |
| **Kubernetes Job agent** | `agent/pr-reviewer/main.go` | Autonomous PR review triggered by the [agent task controller](https://github.com/bborbe/agent); deployed to dev + prod |

Multi-module layout: root has no `go.mod`; each service (`agent/pr-reviewer/`, `watcher/github-pr/`) has its own.

## Standalone CLI

Takes a GitHub or Bitbucket Server PR URL, runs Claude Code review in a `claude-yolo` container against a local checkout, and posts the review back with a verdict (approve / request-changes / comment).

```bash
go install github.com/bborbe/maintainer/agent/pr-reviewer/cmd/cli@latest

pr-reviewer [-v] [--comment-only] <pr-url>
```

Config `~/.config/maintainer/pr-reviewer.yaml`:

```yaml
github:
  token: ${PR_REVIEWER_GITHUB_TOKEN}   # optional; falls back to `gh` CLI auth
model: sonnet                          # sonnet | opus | haiku
autoApprove: false                     # only post comments unless true
repos:
  - url: https://github.com/bborbe/maintainer
    path: ~/Documents/workspaces/maintainer
    reviewCommand: /code-review        # optional
```

Requires: Go 1.26+, [Claude Code CLI](https://claude.com/claude-code), [`gh`](https://cli.github.com/), Docker.

## Kubernetes Job agent

Pattern B Job spawned per task by the [agent task controller](https://github.com/bborbe/agent). Receives `TASK_CONTENT` via env, calls Claude Code with `Bash(gh:*)` access and a `GH_TOKEN` from teamvault, publishes the result back via Kafka.

### Deploy

```bash
cd agent/pr-reviewer
make buca BRANCH=dev     # build + push + apply in dev
make buca BRANCH=prod    # same for prod
```

`make buca` runs image build → `docker push docker.quant.benjamin-borbe.de/maintainer-agent-pr-reviewer:<branch>` → `kubectlquant apply` of rendered manifests in `k8s/`.

### Prerequisites

- Teamvault entries:
  - `SENTRY_DSN_KEY` — Sentry DSN (URL field)
  - `AGENT_PR_REVIEWER_GH_TOKEN_KEY` — GitHub PAT (Password field) with `repo` + `read:org` scopes
- PVC `agent-pr-reviewer` seeded with a valid `.claude/` config (copy from `agent-claude` PVC or run one-time `claude login` in a temp pod; see [claude-oauth-setup.md](https://github.com/bborbe/agent/blob/master/agent/claude/docs/claude-oauth-setup.md)). PVC name preserved across the `code-reviewer` → `maintainer` rename to avoid OAuth re-seed.
- Config CR registered with the task controller (handled by `k8s/maintainer-agent-pr-reviewer.yaml`)

### Trigger a review

Create a markdown task file in the controller-watched vault with `assignee: pr-reviewer-agent`, `status: in_progress`, `stage: dev|prod`, and a `task_identifier: <uuid>`. Body is the task prompt — typically "Review the pull request at `<url>`". Controller publishes to Kafka, executor spawns the Job, result is written back to the task file.

### Debug

See [[Agent Pipeline Debug Guide]] in the Trading vault for the full step-by-step trace. Quick checks:

```bash
kubectlquant -n dev get jobs | grep pr-reviewer
kubectlquant -n dev logs job/pr-reviewer-agent-<uuid>-<timestamp>
```

## Local task runner

Same binary as the k8s agent, driven from a local file instead of Kafka.

```bash
cd agent/pr-reviewer
make run-dummy-task       # generates a sample task file, runs it, writes result back
```

Useful for iterating on prompts (`pkg/prompts/workflow.md`, `pkg/prompts/output-format.md`) and allowed-tool config without a cluster round-trip.

## Layout

```
maintainer/
├── agent/pr-reviewer/          service module (own go.mod)
│   ├── main.go                 k8s Job entry
│   ├── cmd/
│   │   ├── cli/                standalone pr-reviewer CLI
│   │   └── run-task/           local file-driven runner
│   ├── pkg/
│   │   ├── bitbucket/          Bitbucket Data Center REST client (CLI only)
│   │   ├── config/             YAML config
│   │   ├── factory/            DI wiring for k8s/run-task
│   │   ├── git/                worktree / clone manager (CLI only)
│   │   ├── github/             GitHub REST client via `gh`
│   │   ├── prompts/            embedded workflow.md + output-format.md
│   │   ├── prurl/              platform-agnostic PR URL parser
│   │   ├── review/             claude-yolo Docker reviewer (CLI only)
│   │   ├── verdict/            JSON verdict parser
│   │   └── version/            build-time version injection
│   ├── k8s/                    Config CRD, Secret, PVC, PriorityClass, ResourceQuota, Makefile
│   ├── Dockerfile              multi-stage build (Go + claude-code + gh + git)
│   └── agent/.claude/CLAUDE.md headless-review guardrails
├── watcher/github-pr/          GitHub PR watcher service (own go.mod)
│   └── pkg/                    poll loop, GitHub client, cursor, Kafka publisher
├── watcher/github-build/       GitHub build-failure watcher (own go.mod)
│   ├── cmd/run-once/           one-shot CLI for ad-hoc polling
│   └── pkg/                    poll loop, state machine (green→red detection), filter, Kafka publisher
├── Makefile.*                  shared includes (variables, env, docker, k8s, folder, precommit)
├── common.env / dev.env / prod.env
└── prompts/ specs/             dark-factory pipeline metadata
```

## Verdict contract

Review output must end with a JSON block:

```json
{"verdict": "approve|request-changes|comment", "reason": "<one-liner>"}
```

Fallback: heuristic section-header scan (`## Must Fix`, `## Blocking`). See [`pkg/verdict/`](agent/pr-reviewer/pkg/verdict/).

## License

BSD 2-Clause License. See [LICENSE](LICENSE).

<!-- 2026-05-03: prod e2e validation marker — first usable code reviewer agent shipped -->
<!-- 2026-05-05: renamed code-reviewer → maintainer; multi-agent suite -->

