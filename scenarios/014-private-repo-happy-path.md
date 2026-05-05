---
status: draft
spec: 014-private-github-repo-support
---

# Scenario 014: private GitHub repo PR reviewed end-to-end

Validates the primary success path of spec-014: a private GitHub PR
(`bborbe/trading#110`) is reviewed end-to-end by the agent after `gh auth
setup-git` configures git credentials at pod startup using `GH_TOKEN`.

This is the required integration seam test for spec-014: the credential helper
is configured by a subprocess at pod startup — this path cannot be verified by
unit tests alone because git's HTTPS credential lookup depends on the runtime
environment (`~/.gitconfig`, `GH_TOKEN` env var, network access to github.com).

## Prerequisites
- [ ] Set namespace: `export NAMESPACE=dev` (or `prod`) — matches the `NAMESPACE` var in `dev.env`/`prod.env`
- [ ] Dev cluster is running and healthy (`kubectl get pods -n $NAMESPACE`)
- [ ] `agent/pr-reviewer` is deployed to `$NAMESPACE` with spec-014 changes (prompts 1 and 2 merged). Note: `maintainer-agent-pr-reviewer` is deployed as a custom CRD `Config` (`kind: Config`, `apiVersion: agent.benjamin-borbe.de/v1`), NOT a Deployment; the agent runs as ephemeral Jobs spawned by the task-controller from the CRD spec.
- [ ] `GH_TOKEN` env var is set in the Config CRD (already configured via K8s secret for planning phase); confirm:
      ```bash
      kubectl get config.agent.benjamin-borbe.de/maintainer-agent-pr-reviewer -n $NAMESPACE \
        -o jsonpath='{.spec.env}' | python3 -m json.tool | grep GH_TOKEN
      # Expected: entry with name=GH_TOKEN and valueFrom pointing to the secret
      ```
- [ ] The `pr-review-of-ben` GitHub account (the agent's identity) has read access to `bborbe/trading`:
      ```bash
      gh api /repos/bborbe/trading --jq '.permissions'
      # Expected: contains "pull":true (read-only is sufficient for clone)
      ```
- [ ] Vault CLI available: `vault kv list secret/maintainer/tasks/` returns results

## Sub-scenario A: trigger private PR → vault task progresses to phase:done with ## Review

### Action
- [ ] Trigger the watcher to process `bborbe/trading` PR #110 (or open a new PR on `bborbe/trading`):
      ```bash
      # Option 1: open a new draft PR on bborbe/trading from a test branch
      # (use draft to prevent accidental merge)
      gh pr create --repo bborbe/trading --title "test: scenario 014 private-repo review" \
        --body "Scenario 014 verification run" --draft
      # Note the PR number: <pr-number>
      ```
      ```bash
      # Option 2: re-trigger the existing prod task for PR #110
      # (requires direct vault access and the prod agent deployment)
      vault kv patch secret/maintainer/tasks/b0cec7d9-<suffix> phase=in_progress status=in_progress
      ```
- [ ] Wait for the watcher to pick up the PR (≤ one poll cycle, ~5 min) and for the agent pod to complete

### Expected
- [ ] A vault task is created for the PR:
      ```bash
      vault kv list secret/maintainer/tasks/ | grep trading
      ```
- [ ] The vault task progresses to `phase: done`:
      ```bash
      vault kv get -format=json secret/maintainer/tasks/<task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
      # Expected: done completed
      ```
- [ ] The vault task body contains a populated `## Review` section (not empty, not `phase: human_review`):
      ```bash
      vault kv get -format=json secret/maintainer/tasks/<task-id> \
        | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['data'].get('body',''))" \
        | grep -A5 "## Review"
      # Expected: JSON verdict with at least one specialist sub-agent result
      ```
- [ ] Pod startup log contains the auth-setup completion line:
      ```bash
      kubectl logs -n $NAMESPACE <pod-name> | grep "github-auth-setup"
      # Expected: "github-auth-setup: gh auth setup-git complete"
      ```

## Sub-scenario B: pod logs contain zero hits for the literal token string

### Action
- [ ] Capture the pod's full log output:
      ```bash
      kubectl logs -n $NAMESPACE <pod-name> > /tmp/pod-log-<task-id>.txt
      ```
- [ ] Retrieve the literal `GH_TOKEN` secret value:
      ```bash
      GH_TOKEN_VALUE=$(kubectl get secret <gh-token-secret> -n $NAMESPACE \
        -o jsonpath='{.data.GH_TOKEN}' | base64 -d)
      ```
- [ ] Search the log for the token:
      ```bash
      grep -c "$GH_TOKEN_VALUE" /tmp/pod-log-<task-id>.txt || true
      ```

### Expected
- [ ] The grep returns `0` — the literal token value does not appear anywhere in the pod log
- [ ] The log DOES contain `github-auth-setup: running gh auth setup-git` and `github-auth-setup: gh auth setup-git complete` (confirming the setup ran) without exposing the token

## Sub-scenario C: public PR continues to work (no regression)

### Action
- [ ] Open a PR on `bborbe/maintainer` (public repo) from the trusted author account:
      ```bash
      gh pr create --repo bborbe/maintainer --title "test: scenario 014 regression check" \
        --body "" --draft
      ```
- [ ] Wait for the agent to process it

### Expected
- [ ] The vault task progresses to `phase: done` with a populated `## Review` section
- [ ] No `NeedsInput` or `human_review` routing occurs for the public repo task
- [ ] The authenticated token (set via `gh auth setup-git`) does not interfere with public repo clones

## Cleanup
- [ ] Close or merge the test PRs opened in sub-scenarios A and C
- [ ] Remove the temp pod log file: `rm -f /tmp/pod-log-<task-id>.txt`

## Notes
Last run: (not yet run — scenario created for spec-014)
