---
status: completed
spec: [014-private-github-repo-support]
summary: Created scenarios/014-private-repo-happy-path.md and scenarios/015-private-repo-no-token.md covering the private-repo happy path (3 sub-scenarios) and no-token failure paths (4 sub-scenarios A–D) for spec-014.
container: code-reviewer-084-spec-014-scenarios
dark-factory-version: v0.147.2-1-g30ba42f
created: "2026-05-03T18:00:00Z"
queued: "2026-05-03T18:14:47Z"
started: "2026-05-03T18:31:29Z"
completed: "2026-05-03T18:33:36Z"
branch: dark-factory/private-github-repo-support
---

<summary>
- Two new scenario files are created under `scenarios/` to provide integration-level validation coverage for the private GitHub repo support introduced by spec-014
- Scenario 014 covers the happy path: a private GitHub PR (`bborbe/trading#110`) is triggered in dev with `GH_TOKEN` set; the vault task progresses to `phase: done` with `## Review` populated; pod logs contain zero hits for the literal token string
- Scenario 015 covers the no-token path: `GH_TOKEN` is unset; a private PR is triggered; the vault task is routed to `phase: human_review` with a diagnostic naming `host/owner/repo` and mentioning `GH_TOKEN`; public PRs continue to complete successfully in the same config
- A fourth sub-scenario in 015 (cluster) covers the invalid/revoked-token path: `gh auth setup-git` succeeds at startup (token non-empty) but git rejects credentials at clone time, routing the task to `phase: human_review` with the same `host/owner/repo` + `GH_TOKEN` diagnostic shape as the empty-token case
- The "gh binary missing" failure mode is documented as covered by unit tests (`pkg/githubauth/setup_test.go`) rather than a manual scenario, because the local `cmd/run-task` tool injects a no-op auth setup and cluster reproduction would require a custom image
- The scenarios are locally verifiable: scenario 014 (happy path) and 015 sub-scenarios B/C/D require a dev cluster; sub-scenario A of 015 (no-token) can be exercised locally via `cmd/run-task` without a cluster using a crafted task file
- No Go code is written; no `make precommit` is needed
</summary>

<objective>
Create two integration scenario files (`scenarios/014-private-repo-happy-path.md` and `scenarios/015-private-repo-no-token.md`) satisfying the scenario coverage requirement of spec-014: one scenario for the private-repo end-to-end success path and one for the no-token failure and human-review routing path. Both include operator-runnable verification commands.
</objective>

<context>
Read CLAUDE.md for project conventions.

Files to read before writing scenarios:
- `scenarios/006-watcher-author-trust-filter.md` — canonical example scenario; mirror its structure: frontmatter, Prerequisites section, Sub-scenario A/B/C with Action + Expected + Cleanup subsections, markdown checklist format (`- [ ]`)
- `scenarios/013-agent-repo-allowlist-clone-refusal.md` — example of a locally-verifiable `cmd/run-task` scenario; mirror its task-file setup blocks
- `docs/architecture.md` — understand the pipeline: watcher → Kafka → controller → vault → agent pod flow; confirm `GH_TOKEN` is in the pod env (K8s secret mount)

Key facts (verified against the codebase and spec):
- Existing scenarios are numbered 001–013; next available numbers are 014 and 015
- The target private PR is `bborbe/trading#110` (confirmed reachable from prod task `b0cec7d9`)
- `GH_TOKEN` is already in the pod env from the K8s secret (the watcher passes it as `GH_TOKEN` to the planning phase's `gh pr view`; spec-014 also uses it for `gh auth setup-git` at pod startup)
- The pod image already has `github-cli` installed (`apk add github-cli` in the Dockerfile — confirmed)
- The `gh auth setup-git` call writes a git credential helper entry to `/home/claude/.gitconfig` inside the pod; this file is ephemeral (pod-local); it is NOT shared with vault or Kafka
- After spec-014 changes: pod startup log should contain `"github-auth-setup: gh auth setup-git complete"` when `GH_TOKEN` is set
- Token non-leakage: `kubectl logs` grep for the literal token string must return zero hits across all log lines
- The no-token `NeedsInput` diagnostic contains `"github.com/bborbe/trading"` and `"GH_TOKEN"` (from `steps_checkout_execution.go` auth-failure path introduced in spec-014 prompt 2)
- For local verification of the no-token path: use `cmd/run-task` with a task file whose `clone_url` is `https://github.com/bborbe/trading.git` and NO `GH_TOKEN` or `GH_TOKEN=""` set — the agent will attempt to clone the private repo and fail with the auth-failure NeedsInput result (this does not require a live cluster)
- For local runs use the run-task binary directly: `cd agent/pr-reviewer && go build ./cmd/run-task/ && ./run-task ...` (no `make run-dummy-task` target exists; only `verify-gh-token` is in `agent/pr-reviewer/Makefile`)
</context>

<requirements>

**Execute both steps. No `make precommit` needed (no Go changes).**

1. **Create `scenarios/014-private-repo-happy-path.md`**:

```markdown
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
- [ ] `agent/pr-reviewer` is deployed to `$NAMESPACE` with spec-014 changes (prompts 1 and 2 merged). Note: `agent-pr-reviewer` is deployed as a custom CRD `Config` (`kind: Config`, `apiVersion: agent.benjamin-borbe.de/v1`), NOT a Deployment; the agent runs as ephemeral Jobs spawned by the task-controller from the CRD spec.
- [ ] `GH_TOKEN` env var is set in the Config CRD (already configured via K8s secret for planning phase); confirm:
      ```bash
      kubectl get config.agent.benjamin-borbe.de/agent-pr-reviewer -n $NAMESPACE \
        -o jsonpath='{.spec.env}' | python3 -m json.tool | grep GH_TOKEN
      # Expected: entry with name=GH_TOKEN and valueFrom pointing to the secret
      ```
- [ ] The `pr-review-of-ben` GitHub account (the agent's identity) has read access to `bborbe/trading`:
      ```bash
      gh api /repos/bborbe/trading --jq '.permissions'
      # Expected: contains "pull":true (read-only is sufficient for clone)
      ```
- [ ] Vault CLI available: `vault kv list secret/code-reviewer/tasks/` returns results

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
      vault kv patch secret/code-reviewer/tasks/b0cec7d9-<suffix> phase=in_progress status=in_progress
      ```
- [ ] Wait for the watcher to pick up the PR (≤ one poll cycle, ~5 min) and for the agent pod to complete

### Expected
- [ ] A vault task is created for the PR:
      ```bash
      vault kv list secret/code-reviewer/tasks/ | grep trading
      ```
- [ ] The vault task progresses to `phase: done`:
      ```bash
      vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
        | python3 -c "import sys,json; t=json.load(sys.stdin)['data']['data']; print(t.get('phase'), t.get('status'))"
      # Expected: done completed
      ```
- [ ] The vault task body contains a populated `## Review` section (not empty, not `phase: human_review`):
      ```bash
      vault kv get -format=json secret/code-reviewer/tasks/<task-id> \
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
- [ ] Open a PR on `bborbe/code-reviewer` (public repo) from the trusted author account:
      ```bash
      gh pr create --repo bborbe/code-reviewer --title "test: scenario 014 regression check" \
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
```

2. **Create `scenarios/015-private-repo-no-token.md`**:

```markdown
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
```

</requirements>

<constraints>
- Only create files under `scenarios/` — do NOT edit any Go source files or CHANGELOG.md in this prompt
- Do NOT commit — dark-factory handles git
- Scenario files are markdown only; no `make precommit` is needed
- The scenario numbers 014 and 015 are the next available numbers after the existing 013 highest scenario
- Do NOT alter existing scenario files
- Sub-scenario A of scenario 015 is explicitly marked as locally verifiable (no cluster) via `cmd/run-task`
- Before writing, check `agent/pr-reviewer/Makefile` for any `run-task` or `run-dummy-task` targets and reference them in the scenario if present:
  ```bash
  grep -n "run-task\|run-dummy\|build" agent/pr-reviewer/Makefile | head -20
  ```
  If a `make run-task` or similar target exists, reference it in the scenario commands instead of the raw binary path
</constraints>

<verification>
# Confirm both scenario files were created and are non-empty:
ls -la scenarios/014-private-repo-happy-path.md scenarios/015-private-repo-no-token.md

# Confirm they have the correct frontmatter spec field:
grep "spec:" scenarios/014-private-repo-happy-path.md scenarios/015-private-repo-no-token.md

# Confirm they reference the key observable outcomes from spec-014:
grep "GH_TOKEN\|github.com/bborbe/trading\|gh auth setup-git" scenarios/014-private-repo-happy-path.md scenarios/015-private-repo-no-token.md

# Confirm sub-scenario sections exist:
grep "Sub-scenario" scenarios/014-private-repo-happy-path.md scenarios/015-private-repo-no-token.md
</verification>
