---
status: draft
spec: 011-swap-execution-to-coding-pr-review
---

# Scenario 005: execution phase delegates to /coding:pr-review (standard mode)

Validates that the execution phase invokes the `/coding:pr-review` slash command inside the prepared worktree, dispatches at least one specialist sub-agent, translates the report into the verdict JSON schema, and cleans up the worktree. Standard mode (default `REVIEW_MODE`) is the path that fans out to specialists — required by spec-011 acceptance criterion #102.

## Setup
- [ ] `REPO=$(git rev-parse --show-toplevel)` resolves to the maintainer checkout root
- [ ] maintainer at v0.21.3 or higher (`cd $REPO && git describe --tags` — output starts with `v0.21.` or higher)
- [ ] `bborbe/coding` plugin installed: `claude plugin list | grep coding`
- [ ] Local Claude config dir authenticated: `CLAUDE_CONFIG_DIR=~/.claude-agent claude /login`
- [ ] `gh auth status` shows authenticated GitHub session
- [ ] `PR_NUMBER` set to an OPEN PR on `bborbe/maintainer` with **substantive Go code changes** (not a trivial README change — `/coding:pr-review` skips specialists for trivial diffs):
  ```bash
  export PR_NUMBER=<pr-with-go-changes>
  gh pr diff $PR_NUMBER --repo bborbe/maintainer | grep -c '^[+-].*\.go' # expect > 5
  ```

## Action
- [ ] Generate task file pointing at the substantive PR:
  ```bash
  cd $REPO/agent/pr-reviewer/cmd/run-task
  make generate-dummy-task TASK_FILE=test5-plugin.md PR_NUMBER=$PR_NUMBER
  ```
- [ ] Run the agent in `in_progress` phase, capturing all output:
  ```bash
  make run-dummy-task TASK_FILE=test5-plugin.md 2>&1 | tee /tmp/agent-005.log
  ```

## Expected
- [ ] `grep "/coding:pr-review" /tmp/agent-005.log` finds at least one match (slash command invoked)
- [ ] `grep -E "go-quality-assistant|go-security-specialist|go-architecture-assistant|go-factory-pattern-assistant|go-test-coverage-assistant" /tmp/agent-005.log` finds at least one match (specialist sub-agent dispatched)
- [ ] Verdict JSON parses without error:
  ```bash
  awk '/^## Review/{flag=1; next} flag' test5-plugin.md \
    | python3 -c "import sys,json; v=json.load(sys.stdin); assert v['verdict'] in ('approve','request_changes','comment'); print('ok', v['verdict'])"
  ```
- [ ] `summary` field is a non-empty string
- [ ] `concerns_addressed` is a non-empty list AND at least one entry references a sub-agent name (`go-quality-*`, `go-security-*`, etc.) or a Must-Fix/Should-Fix/Nice-to-Have bucket label
- [ ] Workdir cleaned up (no stale worktree under `~/.cache/maintainer/pr-reviewer/work/` for this task UUID)
- [ ] `echo $?` is `0` after the run

## Cleanup
- [ ] `rm -f test5-plugin.md /tmp/agent-005.log`
- [ ] Worktrees: removed automatically on next pod/process startup via `PruneAllWorktrees`

## Notes
Last run: (not yet run — scenario created for spec-011)
