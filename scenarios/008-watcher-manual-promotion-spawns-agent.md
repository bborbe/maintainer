---
status: draft
spec: 012-watcher-author-trust-filter
---

# Scenario 008: manual promotion of human_review task spawns agent

Validates that when an operator manually promotes a `phase: human_review` vault task to `phase: in_progress` / `status: in_progress`, the controller spawns a K8s Job and the agent processes the task. Companion to scenario 006 (untrusted PR routing) — proves the escape hatch works.

## Setup
- [ ] Dev cluster running and healthy: `kubectl get pods -n dev`
- [ ] Watcher deployed with a known `TRUSTED_AUTHORS` (any non-empty value)
- [ ] Vault CLI works: `vault kv list secret/maintainer/tasks/` returns results
- [ ] An untrusted-author vault task exists (from a real PR or seeded fixture):
  ```bash
  # Either run scenario 006 first, OR seed a fixture task:
  vault kv put secret/maintainer/tasks/test-008-fixture \
    phase=human_review status=todo \
    body="## Untrusted author\n\nfixture-user opened PR.\n\nTo promote: phase: in_progress / status: in_progress"
  export TASK_ID=test-008-fixture
  ```

## Action
- [ ] Promote the task:
  ```bash
  vault kv patch secret/maintainer/tasks/$TASK_ID phase=in_progress status=in_progress
  ```
- [ ] Wait up to 60 s for the controller cycle:
  ```bash
  sleep 60
  ```

## Expected
- [ ] K8s Job spawned for the task:
  ```bash
  kubectl get pods -n dev | grep $TASK_ID
  # Expect: a pod in Running or Completed state
  ```
- [ ] Pod log shows agent processing (slash command invocation OR verdict write):
  ```bash
  POD=$(kubectl get pods -n dev -o name | grep $TASK_ID | head -1)
  kubectl logs -n dev $POD | grep -E "/coding:pr-review|verdict|Status.*done"
  ```
- [ ] After job completion, vault task body contains a `## Review` section:
  ```bash
  vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; b=json.load(sys.stdin)['data']['data'].get('body',''); \
                  assert '## Review' in b; print('ok')"
  ```

## Cleanup
- [ ] Remove the vault task: `vault kv delete secret/maintainer/tasks/$TASK_ID`
- [ ] Remove the spawned pod (auto-cleaned by Job TTL, or manual): `kubectl delete pod $POD -n dev --ignore-not-found`

## Notes
Last run: (not yet run — scenario created for spec-012)
