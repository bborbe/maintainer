---
status: draft
spec: 013-repo-allowlist-stage-isolation
---

# Scenario 013: agent repo-allowlist blocks clone of non-allowlisted task

Validates the agent layer of spec-013: when `REPO_ALLOWLIST` is set and a task's
`clone_url` parses to a repo not on the list, the agent returns `status: needs_input`
without cloning. This is the defense-in-depth layer — it fires even if a stale or
mis-routed task somehow reaches the wrong agent.

This scenario is verifiable **locally** using `cmd/run-task` — no live cluster needed.

## Prerequisites
- [ ] `agent/pr-reviewer` is built and sibling prompts 1 + 2 have been deployed:
      ```bash
      cd agent/pr-reviewer && go build ./cmd/run-task/
      # Binary is placed at agent/pr-reviewer/run-task
      ```
- [ ] Vault / Kafka NOT required — `cmd/run-task` uses file I/O only
- [ ] A temp directory for test task files: `mkdir -p /tmp/scenario-013`

## Sub-scenario A: clone_url outside allowlist → NeedsInput, no clone

### Setup
- [ ] Create a test task file whose `clone_url` is outside the allowlist:
      ```bash
      cat > /tmp/scenario-013/out-of-scope-task.md << 'EOF'
      ---
      clone_url: https://github.com/bborbe/other-repo.git
      ref: abc1234567890abcdef
      base_ref: master
      task_identifier: scenario-013-test-001
      phase: in_progress
      status: in_progress
      ---

      # PR Review: scenario 013 test — non-allowlisted repo
      EOF
      ```

### Action
- [ ] Run the agent against this task with the allowlist restricted to a different repo,
      AND pin REPOS_PATH/WORK_PATH to scenario-local temp dirs so the no-clone assertion
      is hermetic:
      ```bash
      mkdir -p /tmp/scenario-013/repos /tmp/scenario-013/work
      REPO_ALLOWLIST=github.com/bborbe/maintainer \
      REPOS_PATH=/tmp/scenario-013/repos \
      WORK_PATH=/tmp/scenario-013/work \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/out-of-scope-task.md \
      ./agent/pr-reviewer/run-task
      ```

### Expected
- [ ] Agent exits with output containing `"status":"needs_input"`:
      ```bash
      cat /tmp/scenario-013/out-of-scope-task.md | grep -A2 '"status"'
      # Expected: "needs_input"
      ```
      Or check the agent's stdout JSON directly.
- [ ] The diagnostic message names the parsed repo key `github.com/bborbe/other-repo`
- [ ] The diagnostic message names the configured allowlist size (1 entry)
- [ ] NO git clone was performed — the scenario-local repos and work dirs stay empty:
      ```bash
      [ -z "$(ls -A /tmp/scenario-013/repos)" ] && echo "ok: repos empty"
      [ -z "$(ls -A /tmp/scenario-013/work)"  ] && echo "ok: work empty"
      ```

## Sub-scenario B: clone_url on allowlist → proceeds past the allowlist check

### Setup
- [ ] Create a test task file whose `clone_url` IS on the allowlist:
      ```bash
      cat > /tmp/scenario-013/in-scope-task.md << 'EOF'
      ---
      clone_url: https://github.com/bborbe/maintainer.git
      ref: master
      base_ref: master
      task_identifier: scenario-013-test-002
      phase: in_progress
      status: in_progress
      ---

      # PR Review: scenario 013 test — allowlisted repo
      EOF
      ```

### Action
- [ ] Run the agent against this task with the matching allowlist (no GH token needed —
      the test will fail at clone or Claude, but must NOT fail at the allowlist check):
      ```bash
      REPO_ALLOWLIST=github.com/bborbe/maintainer \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/in-scope-task.md \
      ./agent/pr-reviewer/run-task 2>&1 | head -20
      ```

### Expected
- [ ] Output does NOT contain `"needs_input"` (allowlist check passed)
- [ ] Agent proceeds to the clone/checkout phase (may fail later due to missing GH token
      or network access — that is expected and out of scope for this scenario)
- [ ] Log does NOT contain `"not on the allowlist"` or similar allowlist-reject message

## Sub-scenario C: startup failure on malformed REPO_ALLOWLIST

### Action
- [ ] Run `cmd/run-task` with a malformed (two-segment) allowlist:
      ```bash
      REPO_ALLOWLIST=bborbe/maintainer \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/out-of-scope-task.md \
      ./agent/pr-reviewer/run-task 2>&1 | head -10
      ```

### Expected
- [ ] Agent exits non-zero immediately (startup failure before any PR processing)
- [ ] Output contains a message naming the malformed entry `bborbe/maintainer`
      and mentioning the required `host/owner/repo` format

## Sub-scenario D: empty REPO_ALLOWLIST → allow-all (backwards-compatibility)

### Action
- [ ] Run the agent with no allowlist set (or explicitly empty):
      ```bash
      REPO_ALLOWLIST= \
      BRANCH=dev \
      TASK_FILE=/tmp/scenario-013/out-of-scope-task.md \
      ./agent/pr-reviewer/run-task 2>&1 | head -20
      ```

### Expected
- [ ] Agent startup log shows `repo-allowlist count=0`
- [ ] Agent does NOT return `needs_input` from the allowlist check (empty = allow-all)
- [ ] Agent proceeds to clone/checkout (may fail for other reasons — expected)

## Cleanup
- [ ] Remove temp task files: `rm -rf /tmp/scenario-013`

## Notes
Last run: (not yet run — scenario created for spec-013)
