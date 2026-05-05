---
status: draft
spec: 012-watcher-author-trust-filter
---

# Scenario 009: watcher fast-paths trusted-author PR to planning phase

Validates that a PR opened by an account in `TRUSTED_AUTHORS` bypasses the trust gate and lands directly in `phase: planning` / `status: in_progress`, with a K8s Job spawned automatically by the controller.

## Setup
- [ ] Dev cluster running and healthy: `kubectl get pods -n dev`
- [ ] Watcher deployed with `TRUSTED_AUTHORS` set; export `TRUSTED_USER` to one of the configured logins:
  ```bash
  export TRUSTED_USER=$(kubectl get deployment github-pr-watcher -n dev \
    -o jsonpath='{.spec.template.spec.containers[0].env}' \
    | python3 -c "import sys,json; e=[x for x in json.load(sys.stdin) if x['name']=='TRUSTED_AUTHORS'][0]; print(e['value'].split(',')[0])")
  ```
- [ ] You can open PRs from `$TRUSTED_USER` on `bborbe/maintainer`
- [ ] Vault CLI works: `vault kv list secret/maintainer/tasks/` returns results

## Action
- [ ] Open a PR on `bborbe/maintainer` from the `$TRUSTED_USER` account; export `PR_NUMBER` to the resulting PR number
- [ ] Wait up to one poll cycle:
  ```bash
  sleep 300
  ```
- [ ] Capture the resulting task ID:
  ```bash
  TASK_ID=$(vault kv list -format=json secret/maintainer/tasks/ \
    | python3 -c "import sys,json; print([t for t in json.load(sys.stdin) if '$PR_NUMBER' in t][0])")
  ```

## Expected
- [ ] Vault task frontmatter has `phase: planning` and `status: in_progress`:
  ```bash
  vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; \
                  assert t['phase']=='planning' and t['status']=='in_progress'; print('ok')"
  ```
- [ ] Vault task body does NOT contain `## Untrusted author`:
  ```bash
  vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; b=json.load(sys.stdin)['data']['data'].get('body',''); \
                  assert 'Untrusted author' not in b; print('ok')"
  ```
- [ ] K8s Job spawned for the task within 60 s of vault task creation:
  ```bash
  kubectl get pods -n dev | grep $TASK_ID
  # Expect: a pod in Running or Completed state
  ```

## Cleanup
- [ ] Close the test PR on GitHub
- [ ] Remove the vault task: `vault kv delete secret/maintainer/tasks/$TASK_ID`

## Notes
Last run: (not yet run — scenario created for spec-012)
