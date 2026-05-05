---
status: draft
spec: 010-git-checkout-review-workdir
---

# Scenario 001: pr-reviewer reviews GitHub PR via HTTPS clone URL

Validates that the pr-reviewer agent clones, checks out, and reviews a real PR using a standard `https://github.com/...` URL — the most common form.

## Setup
- [ ] `REPO=$(git rev-parse --show-toplevel)` resolves to the maintainer checkout root
- [ ] maintainer at v0.20.0 or higher (`cd $REPO && git describe --tags` — output starts with `v0.20.` or higher)
- [ ] Local Claude config dir authenticated: `CLAUDE_CONFIG_DIR=~/.claude-agent claude /login`
- [ ] `gh auth status` shows authenticated GitHub session
- [ ] Test PR available: `gh pr view 2 --repo bborbe/maintainer` returns OPEN state
- [ ] Note: `make generate-dummy-task` pins `TASK_ID=22222222-aaaa-bbbb-cccc-333333333333` (used by Cleanup below)

## Action
- [ ] Generate task file with HTTPS clone URL:
  ```bash
  cd $REPO/agent/pr-reviewer/cmd/run-task
  make generate-dummy-task TASK_FILE=test1-https.md \
    CLONE_URL=https://github.com/bborbe/maintainer.git
  ```
- [ ] Run agent against the task file:
  ```bash
  make run-dummy-task TASK_FILE=test1-https.md 2>&1 | tee /tmp/agent-001.log
  ```

## Expected
- [ ] `grep "git clone" /tmp/agent-001.log` shows successful HTTPS clone (no auth errors)
- [ ] `grep "git checkout" /tmp/agent-001.log` shows checkout of PR head SHA
- [ ] Final stdout contains a JSON verdict block where `verdict` is one of `approve`, `request_changes`, or `comment`
- [ ] `grep '"Status":"done"' /tmp/agent-001.log` finds the result line with `"NextPhase":"ai_review"`
- [ ] `echo $?` is `0` after the run
- [ ] Workdir `~/.cache/maintainer/pr-reviewer/work/22222222-aaaa-bbbb-cccc-333333333333/` contains the cloned repo

## Cleanup
- [ ] `rm test1-https.md /tmp/agent-001.log`
- [ ] `rm -rf ~/.cache/maintainer/pr-reviewer/work/22222222-aaaa-bbbb-cccc-333333333333`

## Notes
Last run: 2026-05-01 — passed in ~17s. Verdict: approve, 1 nit comment on the test PR's marker HTML comment.
