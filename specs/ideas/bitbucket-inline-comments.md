---
tags:
  - dark-factory
  - spec
status: idea
---
Tags: [[Dark Factory - Spec Writing Guide]]

---

## Problem

Specs 004-005 post reviews as a single PR-level comment. Bitbucket Server supports inline comments on specific files and lines, and tasks (actionable to-do items). A single wall-of-text comment is harder to act on than targeted inline feedback.

## Goal

After completion, `maintainer` parses the Claude review output into individual findings and posts them as Bitbucket inline comments on the relevant file and line. Critical findings are additionally posted as Bitbucket tasks (which block merge until resolved).

## Non-goals

- GitHub inline comments (separate spec if needed)
- Resolving/completing tasks automatically
- Diff-aware comment placement (mapping to exact diff hunks)
- Updating existing inline comments on re-review

## Desired Behavior

1. Claude review output is parsed into individual findings with file path, line number, severity, and description
2. Each finding is posted as an inline comment on the corresponding file/line via Bitbucket Server REST API
3. "Must Fix" findings are additionally created as Bitbucket tasks (open tasks block merge)
4. A summary comment is still posted at PR level (spec 004 behavior preserved)
5. If inline comment posting fails for a finding, it falls back to including it in the summary comment
6. If file/line cannot be determined for a finding, it goes into the summary comment

## Constraints

- Spec 004 PR-level comment must still be posted (summary)
- Must use Bitbucket Server REST API for inline comments: `POST /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests/{number}/comments`
- Must use Bitbucket Server REST API for tasks: `POST /rest/api/1.0/projects/{project}/repos/{repo}/pull-requests/{number}/blocker-comments` (or task API)
- Inline comments require valid file path and line number from the PR diff
- If Claude output doesn't include file/line references, fall back to summary-only (spec 004)
- `--comment-only` flag still applies (posts summary comment only, no inline)

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| Claude output has no file/line references | Fall back to summary comment only (spec 004) | Automatic |
| File path in finding doesn't match PR diff | Include in summary comment instead of inline | Automatic |
| Line number out of range for file | Include in summary comment instead of inline | Automatic |
| Inline comment API fails | Include in summary comment, log warning | Automatic |
| Task creation API fails | Log warning, continue with inline comment only | User creates task manually |
| Too many inline comments (API rate limit) | Post remaining as summary comment | Automatic |

## Open Questions

- What format does Claude use for file/line references in review output? Need to analyze actual `/code-review` output to design parser.
- Does the current `/code-review` prompt include file paths and line numbers? If not, the review command may need updating first.
- Bitbucket Server task API — is it `blocker-comments` or a separate tasks endpoint? Depends on server version.

## Acceptance Criteria

- [ ] Individual findings posted as inline comments on correct file/line
- [ ] Must Fix findings additionally created as Bitbucket tasks
- [ ] Summary comment still posted at PR level
- [ ] Findings without file/line fall back to summary comment
- [ ] Failed inline posts fall back to summary comment
- [ ] `--comment-only` skips inline comments, posts summary only
- [ ] Existing GitHub flow unchanged
- [ ] `make precommit` passes

## Verification

```
make precommit
```

## Do-Nothing Option

Keep posting single PR-level comments (spec 004/005). Developers read the full comment and find relevant code themselves. Acceptable for small PRs, painful for large multi-file reviews.
