---
status: draft
spec: 012-watcher-author-trust-filter
---

# Scenario 010: watcher fails startup when TRUSTED_AUTHORS is empty

Validates the safety guard from spec-012: the watcher refuses to start if `TRUSTED_AUTHORS` is unset or empty, preventing accidental auto-processing of every author. Failure-path scenario.

## Setup
- [ ] Dev cluster running and healthy: `kubectl get pods -n dev`
- [ ] Watcher currently deployed with a non-empty `TRUSTED_AUTHORS`; capture original value:
  ```bash
  export ORIGINAL_TRUSTED_AUTHORS=$(kubectl get deployment github-pr-watcher -n dev \
    -o jsonpath='{.spec.template.spec.containers[0].env}' \
    | python3 -c "import sys,json; e=[x for x in json.load(sys.stdin) if x['name']=='TRUSTED_AUTHORS'][0]; print(e['value'])")
  echo "ORIGINAL=$ORIGINAL_TRUSTED_AUTHORS"
  ```

## Action
- [ ] Unset `TRUSTED_AUTHORS` on the deployment:
  ```bash
  kubectl set env deployment/github-pr-watcher -n dev TRUSTED_AUTHORS-
  ```
- [ ] Wait up to 30 s for the rollout:
  ```bash
  sleep 30
  ```

## Expected
- [ ] Pod is in a failure state (`CrashLoopBackOff` or `Error`):
  ```bash
  kubectl get pod -l app=github-pr-watcher -n dev \
    -o jsonpath='{.items[0].status.containerStatuses[0].state.waiting.reason}'
  # Expect: CrashLoopBackOff or Error
  ```
- [ ] Log contains the diagnostic `no trusted authors configured`:
  ```bash
  kubectl logs -n dev deployment/github-pr-watcher --tail=50 | grep "no trusted authors configured"
  ```
- [ ] No new PRs are processed during this window (confirm by checking vault for new task creation):
  ```bash
  # Count tasks before/after; should be unchanged across the 30s window
  vault kv list secret/maintainer/tasks/ | wc -l
  ```

## Cleanup
- [ ] Restore the deployment to its original `TRUSTED_AUTHORS`:
  ```bash
  kubectl set env deployment/github-pr-watcher -n dev "TRUSTED_AUTHORS=$ORIGINAL_TRUSTED_AUTHORS"
  ```
- [ ] Confirm watcher is healthy after restore:
  ```bash
  sleep 30
  kubectl get pods -n dev | grep github-pr-watcher
  # Expect: pod in Running state
  ```

## Notes
Last run: (not yet run — scenario created for spec-012)
