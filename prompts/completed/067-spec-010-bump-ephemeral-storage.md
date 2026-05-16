---
status: completed
spec: [010-git-checkout-review-workdir]
summary: Raised ephemeral-storage from 2Gi to 5Gi in both requests and limits of agent/pr-reviewer/k8s/agent-pr-reviewer.yaml
container: code-reviewer-067-spec-010-bump-ephemeral-storage
dark-factory-version: v0.135.19-1-gc08c946
created: "2026-04-29T09:00:00Z"
queued: "2026-04-29T12:10:41Z"
started: "2026-04-29T12:27:17Z"
completed: "2026-04-29T12:27:41Z"
branch: dark-factory/git-checkout-review-workdir
---

<summary>
- Raise `ephemeral-storage` from 2Gi to 5Gi in both `requests` and `limits` of the K8s Config CR for `agent-pr-reviewer`
- Reason: the execution phase now performs a full-size git clone into `/work/<task-id>` on the pod's overlayfs (no PVC mount yet); 2Gi is too tight for typical Go/Python repos
- The value drops back to 2Gi in a future task once `/repos` becomes a PVC mount and worktree objects are hardlinked from a bare cache
- No Dockerfile change is needed: the container runs as root and creates `/repos` and `/work` at runtime via `os.MkdirAll`
</summary>

<objective>
Raise the `ephemeral-storage` request and limit on the pr-reviewer agent's K8s Config CR so that the per-task worktree fits without OOM-eviction by kubelet.
</objective>

<context>
Read `CLAUDE.md` for project conventions.
Read `docs/architecture.md` — "Storage tiers" table and "Phased rollout" section explain why 5Gi and why this reverts in step 2.5.

File to modify:
- `agent/pr-reviewer/k8s/agent-pr-reviewer.yaml` — `ephemeral-storage` appears in both `spec.resources.requests` and `spec.resources.limits`
</context>

<requirements>
1. **Update `agent/pr-reviewer/k8s/agent-pr-reviewer.yaml`** — change `ephemeral-storage` from `2Gi` to `5Gi` in both the `requests` and `limits` blocks:
   ```yaml
   resources:
     requests:
       cpu: 500m
       memory: 1Gi
       ephemeral-storage: 5Gi
     limits:
       cpu: 500m
       memory: 1Gi
       ephemeral-storage: 5Gi
   ```

2. **Verify**:
   ```bash
   grep -A 8 "resources:" agent/pr-reviewer/k8s/agent-pr-reviewer.yaml
   ```
   Confirm `ephemeral-storage: 5Gi` appears in both `requests` and `limits`. CPU and memory unchanged.
</requirements>

<constraints>
- Only change `agent/pr-reviewer/k8s/agent-pr-reviewer.yaml`
- Do NOT commit — dark-factory handles git
- Do NOT change CPU, memory, or any other resource field
- Do NOT touch the Dockerfile — runtime `os.MkdirAll` from the repo manager covers `/repos` and `/work` creation
- Do NOT add `USER`, `securityContext`, or volume mounts — those belong to a separate spec when multi-volume CRD support lands
- `make precommit` is not required since no Go code changed; only yaml changed
</constraints>

<verification>
grep -A 8 "resources:" agent/pr-reviewer/k8s/agent-pr-reviewer.yaml
# Expected: ephemeral-storage: 5Gi in both requests and limits; cpu/memory unchanged
</verification>
