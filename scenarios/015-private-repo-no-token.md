---
status: draft
spec: 014-private-github-repo-support
---

# Scenario 015: private GitHub PR with no GH_TOKEN routes to human_review

Validates the no-token failure paths of spec-014:
(A) a private PR with `GH_TOKEN` unset is routed to `phase: human_review` with
    a diagnostic naming `host/owner/repo` and pointing operators at `GH_TOKEN`;
(B) public PRs continue to complete normally with the same empty-token config;
(C) the startup auth-setup step fails loudly when the `gh` binary is missing.

Sub-scenarios A and B require a dev cluster. Sub-scenario A can also be
verified locally using `cmd/run-task` without a cluster (described below).

## Prerequisites
- [ ] Set namespace for cluster sub-scenarios: `export NAMESPACE=dev` (or `prod`) — matches the `NAMESPACE` var in `dev.env`/`prod.env`
- [ ] `agent/pr-reviewer` is built (spec-014 changes): `cd agent/pr-reviewer && go build ./cmd/run-task/` — produces the binary at `agent/pr-reviewer/run-task`
- [ ] Vault CLI available for cluster sub-scenarios
- [ ] A temp directory for local task files: `mkdir -p /tmp/scenario-015`

## Sub-scenario A (local): no-token private-repo task → NeedsInput via cmd/run-task

This sub-scenario is verifiable locally without a cluster.

### Setup
- [ ] Create a task file for a private repo clone:
      ```bash
      cat > /tmp/scenario-015/private-task.md << 'EOF'
      ---
      clone_url: https://github.com/bborbe/trading.git
      ref: master
      base_ref: master
      task_identifier: bd4d883b-0000-0000-0000-000000000001
      phase: in_progress
      status: in_progress
      ---

      # PR Review: scenario 015 test — private repo, no token
      EOF
      ```

### Action
- [ ] Run `cmd/run-task` with no `GH_TOKEN` and REPOS_PATH/WORK_PATH pinned to local temp dirs:
      ```bash
      mkdir -p /tmp/scenario-015/repos /tmp/scenario-015/work
      GH_TOKEN= \
      REPOS_PATH=/tmp/scenario-015/repos \
      WORK_PATH=/tmp/scenario-015/work \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-015/private-task.md \
      ./agent/pr-reviewer/run-task 2>&1 | head -30
      ```

### Expected
- [ ] Agent output contains `"status":"needs_input"` (auth-failure NeedsInput path)
- [ ] The diagnostic message contains `github.com/bborbe/trading` (the parsed repo key)
- [ ] The diagnostic message contains `GH_TOKEN` (the operator hint)
- [ ] The diagnostic does NOT contain the literal `GH_TOKEN` secret value (empty in this run — satisfied by definition)
- [ ] REPOS_PATH and WORK_PATH remain empty (no clone was attempted):
      ```bash
      [ -z "$(ls -A /tmp/scenario-015/repos)" ] && echo "ok: repos empty"
      [ -z "$(ls -A /tmp/scenario-015/work)"  ] && echo "ok: work empty"
      ```
- [ ] Agent startup log indicates the auth-setup no-op was taken when the token is empty:
      ```bash
      grep -E "GH_TOKEN.*not set|skipping.*github.*auth" /tmp/scenario-015/run.log
      ```

## Sub-scenario B (cluster): no-token pod reviews public repo normally

Requires dev cluster with `GH_TOKEN` temporarily removed from the agent's Config CRD.
Note: `agent-pr-reviewer` is a `Config` CRD (`agent.benjamin-borbe.de/v1`), not a Deployment.
Future Jobs spawned by the task-controller pick up the edited spec; existing in-flight Jobs are unaffected.

### Action
- [ ] Temporarily remove the `GH_TOKEN` env entry from the Config CRD:
      ```bash
      kubectl edit config.agent.benjamin-borbe.de/agent-pr-reviewer -n $NAMESPACE
      # In the editor: delete the env entry whose `name: GH_TOKEN` (with valueFrom secretKeyRef)
      # Save and exit. Verify:
      kubectl get config.agent.benjamin-borbe.de/agent-pr-reviewer -n $NAMESPACE \
        -o jsonpath='{.spec.env}' | grep -c GH_TOKEN
      # Expected: 0
      ```
- [ ] Open a PR on `bborbe/code-reviewer` (public repo) from the trusted author account
- [ ] Wait for the next agent Job to be spawned and process it (≤ one full pipeline cycle)

### Expected
- [ ] The vault task for the public PR progresses to `phase: done` with a populated `## Review` section
- [ ] No auth-failure routing occurs for the public repo (empty token is fine for public repos — `gh auth setup-git` is skipped, plain clone works)

### Cleanup
- [ ] Restore the `GH_TOKEN` env entry in the Config CRD by re-editing it:
      ```bash
      kubectl edit config.agent.benjamin-borbe.de/agent-pr-reviewer -n $NAMESPACE
      # Re-add the env entry:
      #   - name: GH_TOKEN
      #     valueFrom:
      #       secretKeyRef:
      #         name: agent-pr-reviewer
      #         key: GH_TOKEN
      kubectl get config.agent.benjamin-borbe.de/agent-pr-reviewer -n $NAMESPACE \
        -o jsonpath='{.spec.env}' | grep -c GH_TOKEN
      # Expected: 1
      ```

## Sub-scenario C (cluster): private-repo task with no-token pod → human_review with diagnostic

Continues from Sub-scenario B (Config CRD has `GH_TOKEN` env entry removed).

### Action
- [ ] Trigger a private-repo PR (e.g., `bborbe/trading` PR #110) while the Config CRD has no `GH_TOKEN`:
      ```bash
      # Force the watcher to re-trigger the private PR task:
      # Either open a new PR on bborbe/trading, or re-promote an existing task:
      vault kv patch secret/code-reviewer/tasks/<trading-task-id> phase=in_progress status=in_progress
      ```
- [ ] Wait for the next agent Job to be spawned, process and complete

### Expected
- [ ] The vault task for the private PR is updated to `phase: human_review`:
      ```bash
      vault kv get -format=json secret/code-reviewer/tasks/<trading-task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
      # Expected: human_review (or needs_input depending on controller mapping)
      ```
- [ ] The vault task body contains a diagnostic mentioning `github.com/bborbe/trading` and `GH_TOKEN`
- [ ] Pod log shows the auth-failure NeedsInput message:
      ```bash
      kubectl logs -n $NAMESPACE <pod-name> | grep "no usable git credentials\|GH_TOKEN"
      ```

## Sub-scenario D (cluster): GH_TOKEN set but invalid/revoked → human_review with diagnostic

Validates that even when `gh auth setup-git` succeeds at startup (because the token is non-empty),
git rejects the credentials at clone time and the task is routed to `human_review` with a
diagnostic similar to the empty-token case.

### Setup
- [ ] Restore the `GH_TOKEN` env entry in the Config CRD if currently removed (see Sub-scenario B cleanup), then patch the underlying secret to a known-bad value:
      ```bash
      # Capture the original token first (for restoration):
      ORIG_GH_TOKEN=$(kubectl get secret agent-pr-reviewer -n $NAMESPACE \
        -o jsonpath='{.data.GH_TOKEN}' | base64 -d)
      echo "$ORIG_GH_TOKEN" > /tmp/scenario-015/orig-gh-token  # save for cleanup; chmod 600
      chmod 600 /tmp/scenario-015/orig-gh-token

      # Patch the secret to a known-bad token:
      BAD=$(printf 'gho_invalid_token_for_test' | base64)
      kubectl patch secret agent-pr-reviewer -n $NAMESPACE \
        -p "{\"data\":{\"GH_TOKEN\":\"$BAD\"}}"
      ```

### Action
- [ ] Trigger a private-repo PR (e.g., `bborbe/trading` PR #110) while the Config CRD has the invalid `GH_TOKEN`:
      ```bash
      vault kv patch secret/code-reviewer/tasks/<trading-task-id> phase=in_progress status=in_progress
      ```
- [ ] Wait for the next agent Job to be spawned, process and complete

### Expected
- [ ] Pod startup log shows `github-auth-setup: gh auth setup-git complete` (setup itself succeeds with non-empty token)
- [ ] The vault task is routed to `phase: human_review` with a diagnostic mentioning `github.com/bborbe/trading` and `GH_TOKEN` (git rejects credentials at clone time)
- [ ] No literal token value (`gho_invalid_token_for_test`) appears in pod logs:
      ```bash
      kubectl logs -n $NAMESPACE <pod-name> | grep -c "gho_invalid_token_for_test" || true
      # Expected: 0
      ```

### Cleanup
- [ ] Restore the original `GH_TOKEN` secret value:
      ```bash
      ORIG=$(base64 < /tmp/scenario-015/orig-gh-token)
      kubectl patch secret agent-pr-reviewer -n $NAMESPACE \
        -p "{\"data\":{\"GH_TOKEN\":\"$ORIG\"}}"
      shred -u /tmp/scenario-015/orig-gh-token 2>/dev/null || rm -f /tmp/scenario-015/orig-gh-token
      ```
- [ ] Restore `GH_TOKEN` in the Config CRD (if not already done in Sub-scenario B cleanup)

## Notes — uncovered failure modes

- **gh binary missing**: This failure mode (pod image missing `github-cli`) is exercised by unit tests in `pkg/githubauth/setup_test.go`, not by this scenario. Manual reproduction via `cmd/run-task` is not viable because the local tool injects a no-op auth setup (the real `NewGhAuthSetupGit` runs only inside the cluster pod startup path). Manual cluster reproduction would require a custom Docker image with `gh` removed, which is out of scope for an operator-runnable scenario.

## Cleanup
- [ ] Remove temp files: `rm -rf /tmp/scenario-015`
- [ ] Confirm the agent Config CRD is healthy with `GH_TOKEN` restored:
      ```bash
      kubectl get config.agent.benjamin-borbe.de/agent-pr-reviewer -n $NAMESPACE \
        -o jsonpath='{.spec.env}' | grep -c GH_TOKEN
      # Expected: 1
      kubectl get pods -n $NAMESPACE -l app=agent-pr-reviewer
      ```

## Notes
Last run: (not yet run — scenario created for spec-014)
