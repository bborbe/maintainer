# Maintainer

[![CI](https://github.com/bborbe/maintainer/actions/workflows/ci.yml/badge.svg)](https://github.com/bborbe/maintainer/actions/workflows/ci.yml)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/bborbe/maintainer)

Shared **Go library** + **Helm chart** for the `bborbe` autonomous
repo-maintenance fleet: watchers detect GitHub signals (open PRs, failed CI,
release-ready CHANGELOGs) and emit tasks; Pattern B Job agents act on them
(review PRs, cut releases). It runs on top of the
[bborbe/agent](https://github.com/bborbe/agent) task/agent system.

> **Layout note (2026-07):** the five services were split out of this monorepo
> into their own publish-only repos (below), and the shared library was flattened
> to this repo's root — module `github.com/bborbe/maintainer` (matching the
> `bborbe/agent` layout). This repo now ships **only the library + the chart**;
> it builds no image.

## The fleet

| Repo | Role | Image |
|---|---|---|
| [github-pr-watcher](https://github.com/bborbe/github-pr-watcher) | Producer — polls open PRs, emits review tasks | `docker.io/bborbe/github-pr-watcher` |
| [github-build-watcher](https://github.com/bborbe/github-build-watcher) | Producer — polls CI, emits build-fix tasks on green→red | `docker.io/bborbe/github-build-watcher` |
| [github-release-watcher](https://github.com/bborbe/github-release-watcher) | Producer — polls master for `## Unreleased`, emits release tasks | `docker.io/bborbe/github-release-watcher` |
| [github-pr-review-agent](https://github.com/bborbe/github-pr-review-agent) | Agent — reviews a PR, posts a verdict | `docker.io/bborbe/github-pr-review-agent` |
| [github-releaser-agent](https://github.com/bborbe/github-releaser-agent) | Agent — classifies the semver bump, rewrites CHANGELOG, tags + pushes | `docker.io/bborbe/github-releaser-agent` |

**Flow:** a *watcher* polls a GitHub signal and publishes a `task.CreateCommand`
to Kafka → the [agent task controller](https://github.com/bborbe/agent)
materialises a vault task → the agent-task-executor spawns one Kubernetes Job per
task/phase → the *agent* (pr-review / releaser) does the work. Operator surface:
[vault-cli](https://github.com/bborbe/vault-cli) and
[task-orchestrator](https://github.com/bborbe/task-orchestrator).

## This repo

### 1. Shared library — `github.com/bborbe/maintainer`

Imported by all five services. Packages:

| Package | Purpose |
|---|---|
| `githubapp` | GitHub App auth (installation tokens from an App ID + PEM) |
| `repoallowlist` | `REPO_ALLOWLIST` parsing + host-qualified include/exclude matching |
| `prurl` | Platform-agnostic PR URL parser (GitHub / Bitbucket) |
| `maintainerconfig` | `.maintainer.yaml` parsing (`prReviewer.autoApprove`, `release.autoRelease`, `release.allowMajorBump`, `goUpdate.autoUpdate`, `autoMerge.trivial`) |
| (root) | shared CQRS/CDB task schema helpers |

```bash
go get github.com/bborbe/maintainer@latest
```

### 2. Helm chart — [`helm/`](helm/)

Deploys the fleet onto Kubernetes: the 3 watcher StatefulSets + the 2 agent
`Config` CRs (of `agent.benjamin-borbe.de/v1`, consumed by the core `agent`
chart's executor). Generic/values-driven — image names, allowlists, App IDs,
topic prefixes and secrets are all supplied per-stage by the consuming values.
Published to OCI as `oci://registry-1.docker.io/bborbe/maintainer`.

The quant deployment (dev + prod values + keel-pattern Makefile) lives in the
private `bborbe/quant` config repo under `maintainer/`.

## Per-repo bot config — `.maintainer.yaml`

Each *target* repo opts into the fleet via a root `.maintainer.yaml` (read from
the PR head / master):

```yaml
release:
  autoRelease: true      # github-release-watcher may emit a release task
  allowMajorBump: false  # github-releaser-agent needs explicit opt-in for major bumps
prReviewer:
  autoApprove: true      # github-pr-review-agent's APPROVE counts toward the merge gate
goUpdate:
  autoUpdate: true       # github-update-go-watcher may open a Go-version-bump PR
autoMerge:
  trivial: true          # github-pr-watcher may auto-merge trivial PRs
```

## Build

```bash
make precommit   # fmt, generate, test, lint, vet, vuln, license (single root module)
```

## License

BSD 2-Clause License. See [LICENSE](LICENSE).
