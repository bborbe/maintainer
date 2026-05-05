---
status: draft
spec: 012-watcher-author-trust-filter
---

# Scenario 006: watcher routes untrusted-author PR to human_review

Validates that the watcher routes a PR opened by an account NOT in `TRUSTED_AUTHORS` to a vault task with `phase: human_review` / `status: todo`, and that no agent pod is spawned automatically. The required integration seam test for spec-012's trust gate.

## Setup
- [ ] Dev cluster running and healthy: `kubectl get pods -n dev`
- [ ] Watcher deployed with a known `TRUSTED_AUTHORS`:
  ```bash
  kubectl get deployment maintainer-watcher-github-pr -n dev \
    -o jsonpath='{.spec.template.spec.containers[0].env}' \
    | python3 -m json.tool | grep TRUSTED_AUTHORS
  ```
- [ ] Second GitHub account (NOT in `TRUSTED_AUTHORS`) is available; export `UNTRUSTED_USER` to its login
- [ ] Vault CLI works: `vault kv list secret/maintainer/tasks/` returns results

## Action
- [ ] Open a PR on `bborbe/maintainer` from the `$UNTRUSTED_USER` account (fork PR or branch from a fork); export `PR_NUMBER` to the resulting PR number
- [ ] Wait up to one poll cycle (default 5 min):
  ```bash
  sleep 300
  ```
- [ ] Capture the resulting task ID:
  ```bash
  TASK_ID=$(vault kv list -format=json secret/maintainer/tasks/ \
    | python3 -c "import sys,json; print([t for t in json.load(sys.stdin) if '$PR_NUMBER' in t][0])")
  echo $TASK_ID
  ```

## Expected
- [ ] Vault task exists for the PR: `vault kv list secret/maintainer/tasks/ | grep $PR_NUMBER` matches
- [ ] Vault task frontmatter has `phase: human_review` and `status: todo`:
  ```bash
  vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; \
                  assert t['phase']=='human_review' and t['status']=='todo'; print('ok')"
  ```
- [ ] Vault task body contains `## Untrusted author`:
  ```bash
  vault kv get -format=json secret/maintainer/tasks/$TASK_ID \
    | python3 -c "import sys,json; b=json.load(sys.stdin)['data']['data'].get('body',''); \
                  assert 'Untrusted author' in b; print('ok')"
  ```
- [ ] Vault task body contains `$UNTRUSTED_USER` login string (verify via the same `body` extract)
- [ ] Vault task body contains the promotion instructions (`phase: in_progress` and `status: in_progress`)
- [ ] No agent pod spawned: `kubectl get pods -n dev | grep $TASK_ID` returns no rows

## Cleanup
- [ ] Close the test PR on GitHub
- [ ] Remove the vault task: `vault kv delete secret/maintainer/tasks/$TASK_ID`

## Notes
Last run: (not yet run — scenario created for spec-012)

The defensive "GitHub returns PR with nil User" failure mode is covered by unit tests in `pkg/watcher_test.go`, not by a manual scenario (cannot inject nil-user via public API).
