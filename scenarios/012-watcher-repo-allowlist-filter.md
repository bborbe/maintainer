---
status: draft
spec: 013-repo-allowlist-stage-isolation
---

# Scenario 012: watcher repo-allowlist filter blocks non-allowlisted PRs

Validates the watcher layer of spec-013: the `REPO_ALLOWLIST` env var restricts
which repos the watcher publishes tasks for. An allowlisted PR produces a vault
task; a non-allowlisted PR in the same org produces no task (watcher skips it
silently).

This is a required integration-seam test for spec-013: the filter runs after
all other `TaskCreationFilter` leaves and before the Kafka publish — this path
cannot be covered by unit tests alone because the vault-materialization step
(controller) is the observable outcome.

## Prerequisites
- [ ] Dev cluster is running and healthy (`kubectl get pods -n code-reviewer`)
- [ ] Watcher is deployed to dev with `REPO_ALLOWLIST=github.com/bborbe/code-reviewer`
      (already set in `dev.env`). Confirm:
      ```bash
      kubectl get deployment github-pr-watcher -n code-reviewer \
        -o jsonpath='{.spec.template.spec.containers[0].env}' \
        | python3 -m json.tool | grep REPO_ALLOWLIST
      ```
- [ ] You can open PRs on `bborbe/code-reviewer` (the allowlisted repo) AND on a
      second repo in the same org (e.g. `bborbe/sample-project`) that is NOT on the
      allowlist — call it `non-allowlisted-repo` below
- [ ] Vault CLI is available: `vault kv list secret/code-reviewer/tasks/` returns results
- [ ] Watcher Prometheus metrics are accessible at its `/metrics` endpoint or via:
      ```bash
      kubectl port-forward svc/github-pr-watcher -n code-reviewer 9090:9090 &
      ```

## Sub-scenario A: allowlisted repo PR → vault task created

### Action
- [ ] Open a PR on `bborbe/code-reviewer` (the allowlisted repo) from the trusted
      author account (the login in `TRUSTED_AUTHORS`):
      ```bash
      # e.g. push a test branch and open a PR via gh:
      gh pr create --repo bborbe/code-reviewer --title "test: scenario 012 allowlisted PR" --body ""
      ```
- [ ] Note the PR number: `<pr-number>`
- [ ] Wait up to one poll cycle (default 5 min) for the watcher to process it

### Expected
- [ ] A vault task appears for the PR:
      ```bash
      vault kv list secret/code-reviewer/tasks/ | grep -i code-reviewer
      ```
- [ ] The vault task frontmatter has `phase: planning` and `status: in_progress`
      (trusted author fast-path):
      ```bash
      vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
      # Expected: planning in_progress
      ```
- [ ] The watcher log shows a publish event for this PR (NOT a skip):
      ```bash
      kubectl logs -n code-reviewer deployment/github-pr-watcher | grep "published CreateTaskCommand" | grep "code-reviewer"
      ```

## Sub-scenario B: non-allowlisted repo PR → no vault task

### Action
- [ ] Open a PR on `bborbe/<non-allowlisted-repo>` from any account:
      ```bash
      gh pr create --repo bborbe/<non-allowlisted-repo> --title "test: scenario 012 non-allowlisted" --body ""
      ```
- [ ] Note the PR number: `<other-pr-number>`
- [ ] Wait up to two poll cycles (≤ 10 min) for the watcher to process it

### Expected
- [ ] NO vault task appears for this PR:
      ```bash
      # Derive the expected task ID using the same hash logic as the watcher
      # (or just list all tasks and confirm none match the non-allowlisted repo title):
      vault kv list secret/code-reviewer/tasks/
      # No entry corresponding to <non-allowlisted-repo>#<other-pr-number>
      ```
- [ ] The watcher log shows a skip event for this PR:
      ```bash
      kubectl logs -n code-reviewer deployment/github-pr-watcher \
        | grep "skipping" | grep "<non-allowlisted-repo>"
      # Expected: line containing "reason=filtered"
      ```
- [ ] The Prometheus `pr_published_total{result="skipped"}` counter incremented:
      ```bash
      curl -s http://localhost:9090/metrics | grep 'pr_published_total.*skipped'
      # Expected: value > 0
      ```

## Sub-scenario C: startup failure on malformed REPO_ALLOWLIST

### Action
- [ ] Temporarily patch the watcher deployment to set a malformed allowlist:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer \
        REPO_ALLOWLIST=bborbe/code-reviewer
      # (two segments, no host — deliberately malformed)
      ```
- [ ] Wait for the pod to restart:
      ```bash
      kubectl rollout status deployment/github-pr-watcher -n code-reviewer --timeout=60s
      ```
- [ ] Check logs:
      ```bash
      kubectl logs -n code-reviewer deployment/github-pr-watcher | tail -20
      ```

### Expected
- [ ] Pod fails to start (CrashLoopBackOff or exits non-zero immediately)
- [ ] Log contains a message naming the malformed entry `bborbe/code-reviewer`
      and mentioning `host/owner/repo` format

### Cleanup
- [ ] Restore the correct allowlist:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer \
        REPO_ALLOWLIST=github.com/bborbe/code-reviewer
      kubectl rollout status deployment/github-pr-watcher -n code-reviewer --timeout=60s
      ```

## Sub-scenario D: empty REPO_ALLOWLIST → allow-all (backwards-compatibility)

### Action
- [ ] Temporarily clear the allowlist on the watcher:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer REPO_ALLOWLIST=
      ```
- [ ] Open a PR on `bborbe/<non-allowlisted-repo>` (any repo in scope via `REPO_SCOPE`)
- [ ] Wait one poll cycle

### Expected
- [ ] Watcher log shows `repo-allowlist count=0` at startup
- [ ] A vault task IS created for the non-allowlisted repo (empty allowlist = allow-all)

### Cleanup
- [ ] Restore the allowlist:
      ```bash
      kubectl set env deployment/github-pr-watcher -n code-reviewer \
        REPO_ALLOWLIST=github.com/bborbe/code-reviewer
      ```

## Cleanup
- [ ] Close or merge the test PRs opened in sub-scenarios A and B
- [ ] Confirm the watcher is healthy after all restores:
      ```bash
      kubectl get pods -n code-reviewer | grep github-pr-watcher
      ```

## Notes
Last run: (not yet run — scenario created for spec-013)
