---
status: draft
spec: 012-watcher-author-trust-filter
---

# Scenario 006: watcher author-trust gate routes untrusted PRs to human review

Validates the end-to-end trust gate introduced in spec-012: the watcher routes
untrusted-author PRs to `phase: human_review / status: todo` in the vault,
ensures no agent pod is spawned automatically, and preserves the trusted-author
fast-path unchanged.

This is the required integration seam test for spec-012. The `phase: human_review`
frontmatter value is new — it cannot be verified by watcher unit tests alone because
the controller is the component that materializes the vault task.

## Prerequisites
- [ ] Dev cluster is running and healthy (`kubectl get pods -n dev`)
- [ ] Watcher is deployed to dev with `TRUSTED_AUTHORS` set to a known GitHub login
      (e.g. `TRUSTED_AUTHORS=bborbe`); confirm via:
      ```bash
      kubectl get deployment maintainer-watcher-github-pr -n dev -o jsonpath='{.spec.template.spec.containers[0].env}' | python3 -m json.tool | grep TRUSTED_AUTHORS
      ```
- [ ] A second GitHub account (not in `TRUSTED_AUTHORS`) is available for the
      untrusted-author test; call this account `untrusted-user` below
- [ ] You can open PRs from both accounts on `bborbe/maintainer` (fork or branch)
- [ ] Vault CLI is available: `vault kv list secret/maintainer/tasks/` returns results

## Sub-scenario A: untrusted-author PR → human_review vault task

### Action
- [ ] Open a PR on `bborbe/maintainer` from the `untrusted-user` account
      (a fork PR or a branch in a fork)
- [ ] Wait up to one poll cycle (default 5 min) for the watcher to process it

### Expected
- [ ] A vault task appears for the PR:
      ```bash
      vault kv list secret/maintainer/tasks/ | grep <pr-number>
      ```
- [ ] The vault task frontmatter has `phase: human_review` and `status: todo`:
      ```bash
      vault kv get -format=json secret/maintainer/tasks/<task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
      # Expected: human_review todo
      ```
- [ ] The vault task body contains `## Untrusted author`:
      ```bash
      vault kv get -format=json secret/maintainer/tasks/<task-id> \
        | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['data'].get('body',''))" \
        | grep "Untrusted author"
      ```
- [ ] The vault task body contains the `untrusted-user` GitHub login
- [ ] The vault task body contains the promotion instructions:
      `phase: in_progress` and `status: in_progress`
- [ ] No agent pod was spawned for this task (controller does not promote `human_review` tasks):
      ```bash
      kubectl get pods -n dev | grep <task-id>
      # Expected: no matching pod
      ```

## Sub-scenario B: manual promotion → agent picks up task

(Continues from sub-scenario A)

### Action
- [ ] Manually update the vault task frontmatter to promote it for agent processing:
      ```bash
      vault kv patch secret/maintainer/tasks/<task-id> phase=in_progress status=in_progress
      ```

### Expected
- [ ] On the next controller cycle (≤ 60 s), a K8s Job is spawned:
      ```bash
      kubectl get pods -n dev | grep <task-id>
      # Expected: pod appears with status Running or Completed
      ```
- [ ] The agent processes the task and writes a verdict back (check pod logs):
      ```bash
      kubectl logs -n dev <pod-name> | tail -20
      ```

## Sub-scenario C: trusted-author PR → fast-path unchanged

### Action
- [ ] Open a PR on `bborbe/maintainer` from the trusted account (the login in `TRUSTED_AUTHORS`)
- [ ] Wait up to one poll cycle for the watcher to process it

### Expected
- [ ] A vault task appears for the PR with `phase: planning` and `status: in_progress`:
      ```bash
      vault kv get -format=json secret/maintainer/tasks/<task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
      # Expected: planning in_progress
      ```
- [ ] The vault task body does NOT contain `## Untrusted author`
- [ ] A K8s Job is spawned on the next controller cycle (agent picks it up automatically)

## Sub-scenario D: startup failure on empty TRUSTED_AUTHORS

### Action
- [ ] Temporarily patch the watcher deployment to unset `TRUSTED_AUTHORS`:
      ```bash
      kubectl set env deployment/maintainer-watcher-github-pr -n dev TRUSTED_AUTHORS-
      ```
- [ ] Wait for the pod to restart and check logs:
      ```bash
      kubectl logs -n dev deployment/maintainer-watcher-github-pr | grep "trusted authors"
      ```

### Expected
- [ ] Pod fails to start (CrashLoopBackOff or exits non-zero immediately)
- [ ] Log contains: `no trusted authors configured`
- [ ] No PRs are processed during this window

### Cleanup
- [ ] Restore the deployment to its previous `TRUSTED_AUTHORS` value:
      ```bash
      kubectl set env deployment/maintainer-watcher-github-pr -n dev TRUSTED_AUTHORS=<original-value>
      ```

## Sub-scenario E: force-push on untrusted PR → trust re-evaluated, stays human_review

Validates spec failure-mode "force-push by untrusted author preserves human_review" (re-review path
on synchronize must NOT flip the task back into auto-processing).

### Setup (continues from sub-scenario A)
- [ ] Sub-scenario A completed: vault task for the untrusted PR exists with `phase: human_review`
- [ ] Do NOT manually promote the task (sub-scenario B is independent)

### Action
- [ ] Push another commit to the same untrusted PR's branch (force-push or normal push — both
      change the PR's head SHA, which triggers the watcher's force-push detection path):
      ```bash
      # In the fork's branch directory
      git commit --allow-empty -m "trigger re-review" && git push
      ```
- [ ] Wait up to one poll cycle (≤5 min)

### Expected
- [ ] The vault task for this PR is updated (frontmatter `head_sha` reflects the new SHA), but
      `phase` and `status` remain `human_review` / `todo`:
      ```bash
      vault kv get -format=json secret/maintainer/tasks/<task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'), t.get('head_sha'))"
      # Expected: human_review todo <new-sha>
      ```
- [ ] The vault task body still contains the `## Untrusted author` block (re-evaluation kept the
      decision, did not silently drop it)
- [ ] No K8s agent Job is spawned for the new SHA

## Cleanup
- [ ] Close or merge the test PRs opened in sub-scenarios A, C, and E
- [ ] Confirm the watcher is healthy after restore:
      ```bash
      kubectl get pods -n dev | grep maintainer-watcher-github-pr
      ```

## Notes — uncovered failure mode

- **PR with no author (defensive):** Spec failure mode "GitHub returns a PR with `nil` `User`"
  is not covered by a manual sub-scenario because GitHub does not allow opening a PR with a null
  user via the public API. The defensive path is exercised by unit tests in
  `pkg/watcher_test.go` (PR with empty `AuthorLogin` → `human_review`); a manual scenario would
  require API-injection tooling outside scope of this verification.

## Notes
Last run: (not yet run — scenario created for spec-012)
