---
tags:
  - dark-factory
  - idea
status: idea
---

# Commit inside the PR

## Idea

After review, the agent pushes a fixup commit to the PR branch when it finds trivial, safe-to-auto-fix issues (typos, formatting, missing license headers, `gofmt`, `goimports`).

## Why

Small nits are noise. If the agent already identified them and can fix them mechanically, pushing a single "review fixups" commit is faster than round-tripping the human through request-changes → manual fix → re-review.

## Sketch

- New allowed tools: `Bash(git push:*)`, configure git identity (`review-bot@…`)
- Container needs PAT / deploy key with write scope on the target repo
- Agent checks out PR branch, applies fixes, commits with `Author: PR author`, `Committer: review-bot`, signs off; `git push`
- Commit message: `review fixup: <summary>`; body lists findings
- Opt-in per repo (config flag `allowAutoFixup: true`)

## Risks / Open questions

- Force push vs new commit on top — probably always new commit, never rewrite author history
- What qualifies as "safe" — needs a whitelist (formatter, license header, trailing whitespace); anything else → comment only
- Conflicts with protected branches requiring signed commits
- Infinite loop: every fixup commit triggers a new review → rule: skip review if last commit is by review-bot
- Blast radius: wrong fixup lands on the PR branch; revert path?
- Separate concern from review: consider a separate `auto-fix` agent rather than overloading `pr-reviewer`

## Related

- Competes with / extends approve-PR idea
