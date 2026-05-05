---
status: draft
spec: 011-swap-execution-to-coding-pr-review
---

# Scenario 007: execution phase delegates to /coding:pr-review (short mode)

Validates that `REVIEW_MODE=short` invokes `/coding:pr-review <base> short`, skips specialist sub-agents (by design), and still produces a valid verdict JSON. Companion to scenario 005 which covers standard mode with sub-agent dispatch.

## Setup
- [ ] `REPO=$(git rev-parse --show-toplevel)` resolves to the maintainer checkout root
- [ ] maintainer at v0.21.3 or higher (`cd $REPO && git describe --tags` — output starts with `v0.21.` or higher)
- [ ] `bborbe/coding` plugin installed: `claude plugin list | grep coding`
- [ ] Local Claude config dir authenticated: `CLAUDE_CONFIG_DIR=~/.claude-agent claude /login`
- [ ] `gh auth status` shows authenticated GitHub session
- [ ] `PR_NUMBER` set to any OPEN PR on `bborbe/maintainer` (short mode does not require substantive changes):
  ```bash
  export PR_NUMBER=<any-open-pr>
  gh pr view $PR_NUMBER --repo bborbe/maintainer | grep -i state
  ```

## Action
- [ ] Generate task file:
  ```bash
  cd $REPO/agent/pr-reviewer/cmd/run-task
  make generate-dummy-task TASK_FILE=test7-short.md PR_NUMBER=$PR_NUMBER
  ```
- [ ] Run the agent with `REVIEW_MODE=short`, capturing all output:
  ```bash
  REVIEW_MODE=short make run-dummy-task TASK_FILE=test7-short.md 2>&1 | tee /tmp/agent-007.log
  ```

## Expected
- [ ] `grep "/coding:pr-review .* short" /tmp/agent-007.log` finds at least one match (mode propagated to slash command)
- [ ] `grep -E "go-quality-assistant|go-security-specialist|go-architecture-assistant|go-factory-pattern-assistant|go-test-coverage-assistant" /tmp/agent-007.log` finds **zero** matches (short mode skips specialists by design)
- [ ] Verdict JSON parses without error:
  ```bash
  awk '/^## Review/{flag=1; next} flag' test7-short.md \
    | python3 -c "import sys,json; v=json.load(sys.stdin); assert v['verdict'] in ('approve','request_changes','comment'); print('ok', v['verdict'])"
  ```
- [ ] `summary` field is a non-empty string
- [ ] `echo $?` is `0` after the run

## Cleanup
- [ ] `rm -f test7-short.md /tmp/agent-007.log`
- [ ] Worktrees: removed automatically on next pod/process startup via `PruneAllWorktrees`

## Notes
Last run: (not yet run — scenario created for spec-011)
