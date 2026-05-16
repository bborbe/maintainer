---
tags:
  - dark-factory
  - idea
status: idea
---

# Approve PR

## Idea

The agent submits a real GitHub / Bitbucket PR review (approve / request-changes / comment) based on its verdict, instead of only printing JSON to stdout.

## Why

A stdout verdict is useless for humans on the PR page. To function as a reviewer, the agent must post its verdict where reviewers actually look: in the PR review panel (with the green checkmark or red X).

## Sketch

- Map verdict → action:
  - `done` + clean → `gh pr review --approve --body "<summary>"`
  - `done` + concerns → `gh pr review --comment --body "..."`
  - `needs_input` / issues → `gh pr review --request-changes --body "..."`
  - `failed` → no review posted (infra error, not a verdict)
- Extend `AllowedTools` to keep `Bash(gh:*)` (already there)
- Loosen `agent/.claude/CLAUDE.md` guardrail that forbids posting
- Pass `GITHUB_TOKEN` via `CLAUDE_ENV`
- Bitbucket Server variant: REST API `/pull-requests/{id}/approve` + `/comments`
- Opt-in per repo (config flag `autoApprove: true` — `request-changes` and `comment` can always post; only `approve` is gated)

## Risks / Open questions

- Identity: review must come from a bot account, not the human operator's token
- Rate limits / spam: re-running the agent on the same PR = duplicate reviews → detect and dismiss/replace prior bot reviews
- Required reviewers / branch protection rules: does a bot approval count toward required approvals? (usually yes; may be undesirable)
- Bitbucket "approve" is per-user not per-review — different model than GitHub
- What counts as "clean" for auto-approve — needs an explicit threshold (no must-fix, no critical findings)

## Related

- Extends current agent: guardrails + prompt only, no architectural change
- Independent from commit-inside-pr idea, but complementary
