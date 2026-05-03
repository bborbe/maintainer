---
status: completed
tags:
    - dark-factory
    - spec
approved: "2026-04-29T11:25:04Z"
generating: "2026-04-29T11:26:52Z"
prompted: "2026-04-29T11:39:07Z"
verifying: "2026-04-29T12:27:41Z"
completed: "2026-05-03T19:28:57Z"
branch: dark-factory/git-checkout-review-workdir
---

## Summary

- Replace string-only `gh pr diff` review in the execution phase with a real, on-disk working tree the agent can `cd` into.
- Provider-agnostic: input is `(clone_url, ref, base_ref, task_id)` per the agent contract; no GitHub-specific assumptions in the checkout layer.
- Cache-aware: bare clone per repo is reused across tasks (`git fetch` on subsequent calls); each task gets its own worktree pinned to `ref`.
- Read-only: the workdir is a checkout, never a write target — no commits, pushes, branches, or resets allowed.
- Paths are configurable per entry point (`WorkdirConfig{ReposPath, WorkPath}`); K8s mode defaults to `/repos` and `/work` so a later mount-config change (PVC for cache, emptyDir for worktrees) requires zero code change.

## Problem

The execution phase today inspects pull requests through `gh pr diff` text only. Real review tooling needs a working tree: detecting project type from `go.mod` / `pyproject.toml`, running `make precommit`, dispatching sub-agents over actual source files, and diffing `base_ref...HEAD` on the filesystem. Without an on-disk checkout, the execution phase cannot move from string parsing to real static analysis, and it stays GitHub-coupled (because `gh pr diff` is the only entry point). The agent contract already exposes `clone_url` + `ref`; the checkout step is the missing piece.

## Goal

After this work, the execution phase of the agent runs inside a real git working tree at a deterministic path computed from a configurable root. The working tree is created from a per-repo bare cache that is reused across tasks; the cache is populated via fresh clone or incremental fetch. The implementation is provider-agnostic, read-only, crash-safe, and works identically in all three entry modes (K8s Job, run-task, CLI). The K8s defaults (`/repos`, `/work`) are chosen so a future mount yaml change (PVC for cache, emptyDir for worktrees) is the only adjustment needed — code paths stay the same.

## Non-goals

- Multi-volume Config CRD support in `bborbe/agent` (separate task).
- Mounting `/repos` as PVC or `/work` as emptyDir — defaults today are unmounted overlayfs; mount config is a later yaml change.
- Switching the execution phase from the current command to `/coding:pr-review` (separate spec).
- Posting verdicts back to the source provider (separate spec).
- Bitbucket / GitLab watchers (future work).
- Eviction policy for the bare cache (size generously, observe, then add).
- Provider-agnosticizing planning + ai_review phases (they still use `gh pr diff` API and stay unchanged).

## Desired Behavior

1. **Bare cache is created or refreshed before each task.** Given a `clone_url`, the agent ensures a bare clone exists at a deterministic path derived from the URL (`<reposPath>/<host>/<owner>/<repo>.git`). If absent, it clones with `git clone --bare`. If present and valid, it runs `git fetch --prune origin`. If present but invalid (a half-clone from a previous crashed pod), it is removed and re-cloned.
2. **Worktree is created per task, pinned to ref.** Given the bare cache and the contract's `ref`, the agent creates a worktree at `<workPath>/<task_id>` pinned directly to `ref` in a single `git worktree add` (no separate checkout step). If the worktree directory already exists, it is reused as-is — the agent does not reset, force-checkout, or otherwise mutate state in a pre-existing worktree.
3. **Stale worktree admin entries are pruned at pod startup.** When a pod starts, every bare repo under `<reposPath>` has `git worktree prune` run against it, so admin entries left behind by crashed pods do not block fresh worktree creation.
4. **Execution phase runs inside the worktree.** The factory wires the repo manager into the execution phase only; the phase reads `clone_url` and `ref` from the task frontmatter, ensures the bare clone and worktree, and uses the resulting path as the working directory before invoking the review command.
5. **Planning and ai_review phases are unchanged.** Both still operate on `gh pr diff` output via the GitHub API and do not touch the workdir.
6. **Read-only invariant is enforced in two layers.** The execution phase's allowed-tools list permits only `git clone`, `git fetch`, `git worktree add`, `git worktree prune`, and `cd` — and explicitly excludes `git commit`, `git push`, `git checkout -b`, `git branch`, and `git reset`. The execution-phase prompt repeats this guard in plain language: the workdir is a temporary checkout of an existing ref and must not be modified, committed, pushed, or branched.
7. **Paths are configurable in the agent pipeline modes.** A configuration object exposes `reposPath` and `workPath`. K8s mode defaults to `/repos` and `/work` (overridable via `REPOS_PATH` / `WORK_PATH` env vars). `run-task` defaults to `~/.cache/code-reviewer/repos` and `~/.cache/code-reviewer/work` (env override). The standalone CLI (`cmd/cli`) is **out of scope for this spec** — it uses `DockerReviewer` + `WorktreeManager.CreateClone`, a separate code path for local Docker-based review; integrating CLI into the `RepoManager` flow is a follow-up if and when CLI moves onto the agent pipeline.
8. **Job spec accommodates the layout.** The Job's `ephemeral-storage` is raised from 2Gi to 5Gi to fit a full-size worktree under overlayfs (the value drops back when `/repos` becomes a PVC and worktree objects are hardlinked). No Dockerfile change is needed: the container runs as root (no `USER` directive, no `securityContext`), so the repo manager creates `/repos` and `/work` at runtime via `os.MkdirAll` — same code path on K8s, run-task, and CLI.
9. **No diff-based fallback in the execution phase.** The execution phase exclusively uses the on-disk working tree. There is no `gh pr diff` (or any other text-only) fallback path in the code. If the bare clone, fetch, or worktree step fails, the phase fails loud and returns an error to the controller — the controller decides retry policy. Degrading silently to a less-thorough review is not an option.

## Constraints

- Agent contract (`clone_url`, `ref`, `base_ref`, `task_id`) is frozen — see `docs/architecture.md`, "Agent contract". Of these, only `clone_url`, `ref`, and `task_id` are consumed by the checkout layer; `base_ref` is forwarded to the downstream review command and not used here.
- Execution phase uses git-based review only — no `gh pr diff` (or other text-only) fallback exists in code. Checkout failure fails the phase loud; the controller decides retry.
- Phased rollout step 1 (this work) and step 2 / 2.5 (cache PVC + emptyDir worktrees) must be code-identical; only mount yaml differs. See `docs/architecture.md`, "Phased rollout".
- Bare-cache layout is `<reposPath>/<host>/<owner>/<repo>.git` — provider-agnostic, derived from `clone_url`.
- Worktree layout is `<workPath>/<task_id>` — task-id is collision-free by construction.
- Existing CLI-mode `WorktreeManager` behavior (clone from a local repo path) must continue to work; this spec adds a clone-from-URL surface alongside it.
- Execution-phase allowed-tools list must verifiably exclude every write operation enumerated in Desired Behavior #6.
- Error wrapping uses `github.com/bborbe/errors` per the existing `pkg/git/git.go` style.
- Tests use Ginkgo/Gomega and counterfeiter mocks per existing `pkg/git/git_test.go` patterns; isolation via `t.TempDir()`.
- `make precommit` runs from `agent/pr-reviewer/`, never from the repo root.

## Assumptions

- The architecture doc at `docs/architecture.md` is the canonical reference for the agent contract, storage tiers, and phased rollout — this spec defers to it instead of restating.
- `clone_url` always carries a parseable `<host>/<owner>/<repo>` triple; URLs without that shape are out of scope and may fail loud.
- Credentials for `git clone` / `git fetch` are already present in the container environment (same surface as `gh` today).
- Concurrent worktrees on the same bare repo are safe because the controller pins pr-reviewer to one pod via `ResourceQuota`.
- `task_id` is collision-free by construction — UUID generated by the controller, treated as a controller-level invariant; this spec does not handle the collision case.
- The two existing idea specs (`clone-pr-repo-into-workdir`, `pvc-backed-repo-cache`) are superseded in part by this spec; cache-and-mount details land here, eviction stays an idea.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---|---|---|
| Bare cache directory exists but is not a valid git repo (half-clone from crashed prior pod) | Detect via `git rev-parse --git-dir`; remove the directory and re-clone | Automatic on next task |
| Worktree directory already exists at `<workPath>/<task_id>` | Reuse as-is; do not reset or force-checkout | None needed; reuse is the contract |
| Stale worktree admin entry in bare repo from a crashed pod blocks `git worktree add` | Pod-startup prune removes the dangling admin entry before any task runs | Automatic on pod start |
| `git clone` or `git fetch` fails (network, auth, missing ref) | Fail the phase loud with a wrapped error containing `clone_url` and `ref`; **no diff-based fallback path** | Operator inspects logs; task retried by controller |
| `clone_url` cannot be parsed into `<host>/<owner>/<repo>` | Fail the phase loud with a clear error before any disk operation | Operator fixes the contract upstream |
| `ref` does not exist in the remote after fetch | `git worktree add` fails loud with the underlying git error wrapped | Operator inspects; controller retries |
| Disk full on `/work` (worktree exceeds 5Gi ephemeral-storage) | Pod is OOM-killed by kubelet on ephemeral-storage; controller sees Job failure | Raise the limit, or revisit when PVC + hardlinks land |

## Security / Abuse Cases

- `clone_url` is attacker-influenced via the watcher → controller path; the parser must reject inputs that escape the `<reposPath>/<host>/<owner>/<repo>.git` template (no `..`, no absolute paths, no shell metacharacters).
- `ref` is attacker-influenced; passed only as an argument to `git worktree add`, never interpolated into a shell string.
- `task_id` is generated by the controller (UUID); used only as a directory leaf — must still be validated against a UUID-shaped pattern before joining into a path.
- The execution phase has filesystem write access to `<workPath>/<task_id>` — the prompt + allowed-tools must keep that scope and exclude every write-back-to-remote operation.
- `git fetch` runs against arbitrary `clone_url` hosts; credential exposure surface is the existing one (env-injected token), no new vector.
- No retry loops in the checkout layer — failures bubble up so the controller controls retry policy.

## Acceptance Criteria

- [ ] A repo manager exposes operations to ensure a bare clone, ensure a worktree, and prune stale worktrees, configured via a workdir-config object with `reposPath` / `workPath`.
- [ ] First task on a fresh repo performs a `git clone --bare`; subsequent tasks perform `git fetch --prune origin` against the existing bare.
- [ ] A pre-existing but invalid bare directory is removed and re-cloned on the next task.
- [ ] Worktree creation pins to `ref` in a single `git worktree add` step; a pre-existing worktree at the same path is reused without mutation.
- [ ] Pod-startup pruning runs `git worktree prune` against every bare repo under `<reposPath>`.
- [ ] The factory injects the repo manager into the execution phase only; planning and ai_review phases remain on the API-only path.
- [ ] The execution phase's allowed-tools list permits only the read-only git operations enumerated in Desired Behavior #6 and excludes every write operation listed there.
- [ ] The execution-phase prompt contains a plain-language read-only guard.
- [ ] Defaults differ per entry mode: K8s `/repos` + `/work` (env-overridable); run-task defaults to `~/.cache/code-reviewer/{repos,work}` (env-overridable). CLI (`cmd/cli`) is out of scope for this spec.
- [ ] Repo manager creates `/repos` and `/work` at runtime via `os.MkdirAll` (container runs as root; no Dockerfile change needed).
- [ ] Job's `ephemeral-storage` is set to 5Gi.
- [ ] Unit tests cover: clone path, fetch path, half-clone recovery, worktree create path, worktree reuse path, ref pinning, and stale-worktree pruning — using `t.TempDir()` for isolation.
- [ ] All existing tests pass; `make precommit` from `agent/pr-reviewer/` is green.
- [ ] Execution phase fails loud on bare-clone, fetch, or worktree error — no diff-based fallback path exists anywhere in the execution-phase code or prompt.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Manual smoke (run-task mode): run a task locally; confirm `~/.cache/code-reviewer/repos/github.com/<owner>/<repo>.git` exists after first run and `~/.cache/code-reviewer/work/<task_id>` is created with the expected `ref` checked out. Re-run with the same repo to confirm incremental fetch.

K8s smoke (dev): trigger a task; confirm the execution-phase logs show `cd /work/<task_id>`; confirm the pod's allowed-tools list (or its equivalent surfaced in logs) excludes write operations.

## Do-Nothing Option

Stay on `gh pr diff` text-only review. The execution phase cannot move to real static analysis, the agent stays GitHub-coupled at the data-input layer, and the path to `/coding:pr-review` is blocked. Not acceptable as a long-term position; this spec unblocks the next several tasks on the goal track.
