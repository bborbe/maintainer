---
status: draft
spec: 010-git-checkout-review-workdir
---

# Scenario 003: pr-reviewer reviews arbitrary ref (non-PR commit/branch/tag)

Validates that the pr-reviewer agent reviews any git ref — not just PR heads. Confirms the spec's "any ref on any git server" capability: a SHA, branch, or tag without a corresponding pull request.

## Setup
- [ ] `REPO=$(git rev-parse --show-toplevel)` resolves to the maintainer checkout root
- [ ] maintainer at v0.20.0 or higher (`cd $REPO && git describe --tags` — output starts with `v0.20.` or higher)
- [ ] Local Claude config dir authenticated: `CLAUDE_CONFIG_DIR=~/.claude-agent claude /login`
- [ ] `gh auth status` shows authenticated GitHub session
- [ ] At least 2 commits exist on `master` (`cd $REPO && [ $(git rev-list --count master) -ge 2 ]`)
- [ ] Note: `TASK_ID=44444444-aaaa-bbbb-cccc-555555555555` is fixed by this scenario (used by Action and Cleanup)

## Action
- [ ] Generate task file with arbitrary ref pair (master HEAD vs master~1):
  ```bash
  cd $REPO
  BASE=$(git rev-parse master~1)
  HEAD=$(git rev-parse master)
  cd agent/pr-reviewer/cmd/run-task
  make generate-dummy-task TASK_FILE=test3-branch.md \
    REF=$HEAD BASE_REF=$BASE \
    TASK_ID="44444444-aaaa-bbbb-cccc-555555555555" \
    PR_URL="(none — arbitrary ref review)"
  ```
- [ ] Run agent:
  ```bash
  make run-dummy-task TASK_FILE=test3-branch.md 2>&1 | tee /tmp/agent-003.log
  ```

## Expected
- [ ] `grep "git clone" /tmp/agent-003.log` shows successful clone
- [ ] `grep "git checkout" /tmp/agent-003.log` shows checkout of the SHA from `REF`
- [ ] No `gh pr view` invocation in `/tmp/agent-003.log` (proves PR semantics not required)
- [ ] Final stdout contains a JSON verdict block where `verdict` is one of `approve`, `request_changes`, or `comment`
- [ ] Verdict JSON's `comments` array has at least 1 element OR `summary` length > 100 bytes (proves real-content review, not single-line marker)
- [ ] `grep '"Status":"done"' /tmp/agent-003.log` finds the result line
- [ ] `echo $?` is `0` after the run

## Cleanup
- [ ] `rm test3-branch.md /tmp/agent-003.log`
- [ ] `rm -rf ~/.cache/maintainer/pr-reviewer/work/44444444-aaaa-bbbb-cccc-555555555555`

## Notes
Last run: 2026-05-01 — passed in ~80s reviewing v0.20.0 release commit. Verdict: approve, 3 nuanced comments on tools.env migration details.
