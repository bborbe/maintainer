---
status: draft
spec: 010-git-checkout-review-workdir
---

# Scenario 002: pr-reviewer reviews GitHub PR via SCP-style SSH clone URL

Validates that the pr-reviewer agent clones, checks out, and reviews a real PR using an SCP-form SSH clone URL (`git@host:owner/repo.git`). Regression coverage for SCP support shipped in v0.20.0.

## Setup
- [ ] `REPO=$(git rev-parse --show-toplevel)` resolves to the maintainer checkout root
- [ ] maintainer at v0.20.0 or higher (`cd $REPO && git describe --tags` — output starts with `v0.20.` or higher)
- [ ] Local Claude config dir authenticated: `CLAUDE_CONFIG_DIR=~/.claude-agent claude /login`
- [ ] `gh auth status` shows authenticated GitHub session
- [ ] SSH key registered with GitHub: `ssh -T git@github.com 2>&1 | grep "successfully authenticated"`
- [ ] `SSH_AUTH_SOCK` is set OR `~/.ssh/id_*` accessible to the agent process
- [ ] Test PR available: `gh pr view 2 --repo bborbe/maintainer` returns OPEN state
- [ ] Note: `make generate-dummy-task` pins `TASK_ID=22222222-aaaa-bbbb-cccc-333333333333`

## Action
- [ ] Generate task file with SCP/SSH clone URL:
  ```bash
  cd $REPO/agent/pr-reviewer/cmd/run-task
  make generate-dummy-task TASK_FILE=test2-scp.md \
    CLONE_URL=git@github.com:bborbe/maintainer.git
  ```
- [ ] Run agent against the task file:
  ```bash
  make run-dummy-task TASK_FILE=test2-scp.md 2>&1 | tee /tmp/agent-002.log
  ```

## Expected
- [ ] `grep "git clone" /tmp/agent-002.log` shows successful SSH-transport clone of the SCP URL
- [ ] No `parse error` or `invalid URL` strings in `/tmp/agent-002.log`
- [ ] `grep "git checkout" /tmp/agent-002.log` shows checkout of PR head SHA
- [ ] Final stdout contains a JSON verdict block where `verdict` is one of `approve`, `request_changes`, or `comment`
- [ ] `grep '"Status":"done"' /tmp/agent-002.log` finds the result line
- [ ] `echo $?` is `0` after the run

## Cleanup
- [ ] `rm test2-scp.md /tmp/agent-002.log`
- [ ] `rm -rf ~/.cache/maintainer/pr-reviewer/work/22222222-aaaa-bbbb-cccc-333333333333`

## Notes
Last run: 2026-05-01 — passed in ~14s. Verdict: approve, no comments. Confirms SCP-form URL parsing end-to-end.
