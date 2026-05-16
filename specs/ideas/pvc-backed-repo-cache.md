---
tags:
  - dark-factory
  - idea
status: idea
---

# PVC-Backed Bare-Clone Repo Cache

## Idea

Cache bare clones of frequently-reviewed repos on the PVC. Per-task worktrees are created via `git clone --local` (hardlinked) into `/tmp`, reusing objects from the cache. Fetches are incremental; full clones only happen the first time a repo is seen.

## Why

With `clone-pr-repo-into-workdir` naively implemented, every review task re-clones the whole repo — seconds-to-minutes of latency, wasted bandwidth, and unnecessary load on github.com. A persistent bare-clone cache on the PVC gives near-instant startup for all subsequent reviews of the same repo.

## Sketch

- Layout inside existing PVC (`/home/claude/.claude`):
  ```
  /home/claude/.claude/cache/repos/
    github.com/<owner>/<repo>.git   # bare clone, reused
  ```
- Entrypoint or early-agent-step:
  ```sh
  REPO_CACHE=/home/claude/.claude/cache/repos/github.com/<owner>/<repo>.git
  if [ -d "$REPO_CACHE" ]; then
    git -C "$REPO_CACHE" fetch --prune origin
  else
    git clone --bare <url> "$REPO_CACHE"
  fi
  git clone --local "$REPO_CACHE" /tmp/pr-<N>
  cd /tmp/pr-<N> && git checkout <head>
  ```
- `--local` hardlinks objects → negligible disk cost per worktree
- Bump PVC from 1Gi → 5Gi (or 10Gi) to accommodate a handful of full-history repos
- Periodic `git gc --auto` via entrypoint (cheap, idempotent)

## Risks / Open questions

- PVC growth unbounded over time — add LRU eviction? (or just size generously)
- Concurrent pods on same PVC would conflict — but `ResourceQuota` pins pr-reviewer to 1 pod, so serial access is fine today
- `git clone --local` + `--depth` are incompatible — cache must be full-history
- Fetch-on-every-task could still be slow for huge repos — acceptable vs. cold clone, but measure
- Bare cache must have matching URL for `origin` remote (GH_TOKEN auth)
- What if a repo gets renamed/moved? Cache becomes stale — rely on fetch failures to re-seed

## Related

- Builds on: `clone-pr-repo-into-workdir`
- Touches: `agent/pr-reviewer/k8s/maintainer-agent-pr-reviewer-pvc.yaml` (size bump), `docker-entrypoint.sh` or `pkg/prompts/workflow.md`
