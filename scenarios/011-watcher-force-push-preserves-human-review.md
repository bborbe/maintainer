---
status: draft
spec: 012-watcher-author-trust-filter
---

# Scenario 011: force-push on untrusted PR preserves human_review status

Validates spec-012's force-push failure mode: when a PR's head SHA changes (force-push or new commit), the watcher re-evaluates trust but does NOT silently flip the existing `human_review` task back into auto-processing. Regression for the "trust decision is sticky across SHA changes" property.

## Setup
- [ ] Dev cluster running and healthy: `kubectl get pods -n dev`
- [ ] Watcher deployed with a known `TRUSTED_AUTHORS`
- [ ] Vault CLI works: `vault kv list secret/maintainer/tasks/` returns results
- [ ] An untrusted-author PR exists with an existing `phase: human_review` vault task (run scenario 006 first, OR seed via vault CLI):
  ```bash
  # Option A: run scenario 006 and reuse its PR_NUMBER + TASK_ID
  # Option B: open an untrusted PR manually and wait for the watcher to process it
  export PR_NUMBER=<untrusted-pr-number>
  export TASK_ID=<corresponding-vault-task-id>
  ```
- [ ] Confirm starting state: `vault kv get -format=json secret/maintainer/tasks/$TASK_ID | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; assert t['phase']=='human_review' and t['status']=='todo'; print('ok')"`
- [ ] Capture original head SHA:
  ```bash
  export ORIGINAL_SHA=$(vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['data'].get('head_sha',''))")
  ```

## Action
- [ ] Push another commit to the untrusted PR's branch (changes the head SHA):
  ```bash
  # In the fork's branch checkout
  git commit --allow-empty -m "trigger re-review" && git push
  ```
- [ ] Wait up to one poll cycle:
  ```bash
  sleep 300
  ```

## Expected
- [ ] Vault task `head_sha` reflects the new SHA (different from `$ORIGINAL_SHA`):
  ```bash
  NEW_SHA=$(vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['data'].get('head_sha',''))")
  [ "$NEW_SHA" != "$ORIGINAL_SHA" ] && echo "ok: SHA changed"
  ```
- [ ] `phase` and `status` remain `human_review` / `todo`:
  ```bash
  vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; \
                  assert t['phase']=='human_review' and t['status']=='todo'; print('ok')"
  ```
- [ ] Vault task body still contains `## Untrusted author`:
  ```bash
  vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; b=json.load(sys.stdin)['data']['data'].get('body',''); \
                  assert 'Untrusted author' in b; print('ok')"
  ```
- [ ] No K8s agent Job is spawned for the new SHA: `kubectl get pods -n dev | grep $TASK_ID` returns no Running pods

## Cleanup
- [ ] Close the test PR on GitHub
- [ ] Remove the vault task: `vault kv delete secret/maintainer/tasks/$TASK_ID`

## Notes
Last run: (not yet run — scenario created for spec-012)
