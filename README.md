# Maintainer

[![CI](https://github.com/bborbe/maintainer/actions/workflows/ci.yml/badge.svg)](https://github.com/bborbe/maintainer/actions/workflows/ci.yml)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/bborbe/maintainer)

Autonomous repo-maintenance agents backed by Claude Code. Watchers detect signals (PRs, failed builds, alerts), Pattern B Job agents act on them.

Ships today:
- **`agent/pr-reviewer`** — Pattern B Job agent that reviews GitHub + Bitbucket Server pull requests
- **`agent/github-releaser`** — Pattern B Job agent that classifies semver bumps from `## Unreleased` CHANGELOG entries, rewrites to `## vX.Y.Z`, commits + tags + pushes
- **`watcher/github-pr`** — polls open PRs and emits create-task commands for each new PR
- **`watcher/github-build`** — polls GitHub Actions for failed CI runs on default branches and emits create-task commands on green→red transitions
- **`watcher/github-release`** — polls master for non-empty `## Unreleased` CHANGELOG sections on opted-in repos and emits create-task commands

Planned siblings: build-fixer agent (consume github-build tasks → propose fix PRs), dep-updater, sentry-triager, repo-reviewer.

## Where this fits in the bigger picture

This repo provides **GitHub-source task producers + platform agents** on top of the [bborbe/agent](https://github.com/bborbe/agent) task / agent system:

- Producers: `watcher/github-pr` · `watcher/github-build` · `watcher/github-release` — each polls a GitHub signal and emits `task.CreateCommand` events using the schema published from [`agent/lib`](https://github.com/bborbe/agent/tree/master/lib).
- Agents: `agent/pr-reviewer` · `agent/github-releaser` — register themselves as `Config` CRs of `agent.benjamin-borbe.de/v1` so `agent`'s `task/executor` spawns one Kubernetes Job per task / phase.

Operator surface for the resulting tasks: [vault-cli](https://github.com/bborbe/vault-cli) and [task-orchestrator](https://github.com/bborbe/task-orchestrator).

Full system map: [recurring-task-creator/docs/system-map.md](https://github.com/bborbe/recurring-task-creator/blob/master/docs/system-map.md).

## pr-reviewer (current)

Three modes:

| Mode | Entry | Use case |
|---|---|---|
| **Standalone CLI** | `agent/pr-reviewer/cmd/cli/main.go` | Ad-hoc local review — takes a PR URL, posts a comment back |
| **Local task runner** | `agent/pr-reviewer/cmd/run-task/main.go` | Dev loop for the agent — reads a task markdown file, writes result back |
| **Kubernetes Job agent** | `agent/pr-reviewer/main.go` | Autonomous PR review triggered by the [agent task controller](https://github.com/bborbe/agent); deployed to dev + prod |

Multi-module layout: root has no `go.mod`; each service (`agent/pr-reviewer/`, `watcher/github-pr/`) has its own.

## Standalone CLI

Takes a GitHub or Bitbucket Server PR URL, runs Claude Code review in a `claude-yolo` container against a local checkout, and posts the review back with a verdict (approve / request-changes).

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
- The `/coding` plugin is baked into the image at build time (no PVC seed, no `claude login` required).
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

## github-releaser

Pattern B Job agent that watches a repo's `## Unreleased` and, when non-empty, classifies the bullets as `patch` / `minor` / `major`, rewrites the CHANGELOG header to `## vN+1.x.x`, commits, tags, and pushes directly to master. Three phases (planning → execution → ai_review). Triggered by the [github-release watcher](watcher/github-release) and the `/trigger` admin endpoint.

Config `~/.config/maintainer/github-releaser.yaml` mirrors `.maintainer.yaml` knobs (per-repo `release.autoRelease` etc.); see `agent/github-releaser/README.md` for the service-level walkthrough.

### Major-bump guard (spec 060)

The agent will **never** auto-release a `major` semver bump without an explicit opt-in. The guard fires in the planning phase, after Claude classifies the bump. Trip condition: `bump=major` AND `release.allowMajorBump=false` (or absent) AND `--allow-major` is unset. On trip the planning step writes `## Plan` with `outcome: needs_input` and `precondition_failed: major_bump_not_allowed`, clears `assignee`, sets `previous_assignee: github-releaser-agent`, and returns `Status: NeedsInput` — no silent downgrade, no auto-retry, no advance.

Two levers, used independently or together:

1. **Per-repo opt-in (durable).** Commit `release.allowMajorBump: true` to the target repo's `.maintainer.yaml`:

   ```yaml
   # .maintainer.yaml
   release:
     allowMajorBump: true   # opt in to automatic major-version releases
   ```

   Default is `false` — omit the field, set it `false` explicitly, or omit the `release:` block; all three are equivalent. Per-repo scope; survives re-fires; the YAML is the authoritative policy for that repo.

2. **Per-run override (transient).** Re-fire the agent with `--allow-major=true` on the binary, or `ALLOW_MAJOR=true` in the env. The CLI flag propagates through `application` → `BuildEnv` → the planning step. When the override fires, the planning step emits a `glog.V(2)` line containing the literal `--allow-major override` so `kubectl logs` greps surface operator overrides.

Either lever alone is sufficient; both can be set. `patch` and `minor` verdicts are unaffected — the guard is a no-op on those.

#### Why this guard exists

The bump classifier is prefix-based (`feat:` → minor, `BREAKING CHANGE` → major, everything else → patch). It has a known false-negative class of bug: a `refactor:` bullet that is semantically a breaking library rename (e.g. `refactor(lib): rename TaskTypeClaude → TaskTypeLLM`) reads as `patch` under the rules, yet downstream consumers see an incompatible API. The classifier itself cannot catch every case — prefix rules are a moving target. The guard catches the *symmetric* case: when the classifier *does* emit `major` (whether from a hidden `BREAKING CHANGE:` body line in a `refactor:` bullet or a real breaking change), the operator confirms once before tag + push. The cost of a single wrong major release is high (downstream consumers break under their semver discipline; the fix-forward is a manual revert) — the guard turns that into a one-time human ack.

#### Re-delegating a tripped task

When the guard trips, the task lands with `assignee: ""` and a `## Plan` block naming the cited bullets and the would-be next version. Two ways to re-delegate:

1. **Flip the YAML opt-in and re-set the assignee.** Commit `release.allowMajorBump: true` to the target repo's `.maintainer.yaml`, push, then in the task file re-set `assignee: github-releaser-agent`. The next poll cycle (or manual `/trigger`) re-fires the agent with the durable opt-in in effect.

2. **Re-fire the Job with the CLI override.** Re-create the Job (or set `ALLOW_MAJOR=true` on the controller's spawn env) without editing the target repo's config. The override is transient — it covers the in-flight run only; the next run still needs the YAML opt-in (or another override) to release a major.

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
│   ├── k8s/                    Config CRD, Secret, PriorityClass, ResourceQuota, Makefile
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
{"verdict": "approve|request-changes", "reason": "<one-liner>"}
```

Fallback: heuristic section-header scan (`## Must Fix`, `## Blocking`). See [`pkg/verdict/`](agent/pr-reviewer/pkg/verdict/).

## License

BSD 2-Clause License. See [LICENSE](LICENSE).

<!-- 2026-05-03: prod e2e validation marker — first usable code reviewer agent shipped -->
<!-- 2026-05-05: renamed code-reviewer → maintainer; multi-agent suite -->

