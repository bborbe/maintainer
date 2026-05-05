---
status: draft
spec: 015-github-build-watcher-mvp
---

# Scenario 016: build watcher end-to-end — detect, publish, recover, new episode

Validates the four core behaviors of spec-015: the build watcher detects a red
build, publishes a task within one poll interval; re-polls do not duplicate;
after the build is fixed no new tasks are created; when the build goes red again
with a *different commit*, a new distinct task is created.

This is the required integration-seam test for spec-015. The GitHub Actions API
and Kafka task materialization path cannot be exercised by unit tests alone.

Target repo: `bborbe/go-skeleton` (the existing dev test-bed repo; already red
on its default branch per the spec's AC).

## Prerequisites
- [ ] Dev cluster is running and healthy
- [ ] Build watcher deployed to dev namespace with spec-015 changes:
      ```bash
      kubectl get pods -n dev -l app=maintainer-watcher-github-build
      # Expected: 1/1 Running
      ```
- [ ] `REPO_ALLOWLIST` (in `dev.env`) includes `github.com/bborbe/go-skeleton` (shared with PR watcher in v1)
- [ ] Vault CLI available: `vault kv list secret/maintainer/tasks/` returns results
- [ ] Access to `bborbe/go-skeleton` to trigger CI runs (push to default branch or re-run a workflow)

## Sub-scenario A: red build → vault task materializes within one poll interval

### Action
- [ ] Confirm `bborbe/go-skeleton` default branch currently has a failing workflow:
      ```bash
      gh run list --repo bborbe/go-skeleton --branch master --status failure --limit 5
      # Expected: at least one failed run listed
      ```
- [ ] Note the failing run's head SHA: `<episode-sha>`
- [ ] Compute the expected task ID:
      ```bash
      cd watcher/github-build && go run ./cmd/run-once
      # Or compute manually: UUID5(namespace="8e3f5a2c-...", "bborbe/go-skeleton#build-<episode-sha>")
      ```
- [ ] Wait one poll interval (≤ 5 min) or trigger manually:
      ```bash
      curl -s -X POST http://<build-watcher-service>:9090/trigger
      ```

### Expected
- [ ] A vault task with `assignee: build-fixer-agent` appears:
      ```bash
      vault kv list secret/maintainer/tasks/ | grep -i build
      ```
- [ ] The vault task `task_id` matches the expected UUID5
- [ ] The task body contains the failing workflow name(s), run URL(s), and episode SHA:
      ```bash
      vault kv get -format=json secret/maintainer/tasks/<task-id> \
        | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['data'].get('body',''))"
      # Expected: markdown body with ## Failing Workflows section and episode SHA
      ```
- [ ] Build watcher pod log shows a `green_to_red` transition for `bborbe/go-skeleton`:
      ```bash
      kubectl logs -n dev <build-watcher-pod> | grep "state_transition\|green_to_red\|go-skeleton"
      ```

## Sub-scenario B: re-poll does not duplicate (idempotency)

### Action
- [ ] Trigger a second poll cycle without changing the build state:
      ```bash
      curl -s -X POST http://<build-watcher-service>:9090/trigger
      ```
- [ ] Wait for the poll to complete (check pod logs for "poll cycle complete")

### Expected
- [ ] No new vault task was created (same task_id — controller would dedup anyway,
      but the watcher should skip the publish entirely):
      ```bash
      vault kv list secret/maintainer/tasks/ | grep -c build
      # Expected: same count as after sub-scenario A (no new entry)
      ```
- [ ] Pod logs show no `green_to_red` transition in the second poll cycle:
      ```bash
      kubectl logs -n dev <build-watcher-pod> | grep "green_to_red" | wc -l
      # Expected: still 1 (from sub-scenario A only)
      ```

## Sub-scenario C: build fixed → no new tasks on subsequent polls

### Action
- [ ] Fix the `bborbe/go-skeleton` build: push a commit that makes all workflows pass,
      or manually re-run a failed workflow after fixing the root cause
- [ ] Wait for the GitHub Actions run to complete with `conclusion: success`
- [ ] Trigger a poll cycle:
      ```bash
      curl -s -X POST http://<build-watcher-service>:9090/trigger
      ```

### Expected
- [ ] Pod logs show a `red_to_green` transition:
      ```bash
      kubectl logs -n dev <build-watcher-pod> | grep "red_to_green\|go-skeleton"
      ```
- [ ] No new vault task was published (no `CreateTaskCommand` for this poll):
      ```bash
      vault kv list secret/maintainer/tasks/ | grep -c build
      # Expected: same count — no new build task
      ```
- [ ] Subsequent poll cycles also produce no new tasks (state is now green):
      ```bash
      curl -s -X POST http://<build-watcher-service>:9090/trigger
      vault kv list secret/maintainer/tasks/ | grep -c build
      # Expected: count unchanged
      ```

## Sub-scenario D: new red episode on different SHA → distinct task ID

### Action
- [ ] Push a NEW breaking commit to `bborbe/go-skeleton` default branch (different from
      the commit in sub-scenario A), causing a fresh CI failure
- [ ] Note the new failing run's head SHA: `<new-episode-sha>` (must differ from `<episode-sha>`)
- [ ] Trigger a poll cycle:
      ```bash
      curl -s -X POST http://<build-watcher-service>:9090/trigger
      ```

### Expected
- [ ] A NEW vault task appears with a DIFFERENT task_id than sub-scenario A:
      ```bash
      vault kv list secret/maintainer/tasks/ | grep build
      # Expected: 2 entries total (one from sub-scenario A, one new)
      ```
- [ ] The new task body references `<new-episode-sha>`, not `<episode-sha>`
- [ ] Pod logs show a new `green_to_red` transition for `bborbe/go-skeleton`

## Cleanup
- [ ] Close or cancel any open vault tasks created during this scenario
- [ ] Restore `bborbe/go-skeleton` to a green state if broken during sub-scenario D

## Notes
Last run: (not yet run — scenario created for spec-015)
