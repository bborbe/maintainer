---
tags:
  - dark-factory
  - spec
status: draft
---

## Summary

- The agent's `in_progress` phase, after producing the verdict, submits a real GitHub PR review via `gh pr review`, mapping `approve | request_changes | comment` 1-to-1 from the verdict. The `ai_review` phase keeps its existing role (verifying review quality) and additionally verifies that the review was posted correctly.
- The summary text is always posted as the review body so PR authors see findings in the GitHub UI, not buried in a vault file.
- Auto-approval is gated per repo via a config file IN THE TARGET REPO (e.g., `.pr-reviewer.yaml` at repo root): if the file is missing, no auto-approve. If `autoApprove: false` (or file missing), `verdict=approve` is demoted to a `--comment` post (with a note in the body) so a flaky reviewer can't merge work. `request_changes` and `comment` always post.
- Bot identity is enforced: posting uses a PAT for the dedicated GitHub bot account `pr-review-of-ben` (NOT the operator's `bborbe` PAT). The agent self-checks via `gh api /user` and refuses to post if the login is anything else. Duplicate-review protection dismisses prior bot reviews on the same head SHA before posting fresh.
- Bitbucket parity is explicitly out of scope (separate idea exists). Inline file:line comments are out of scope; A4 posts top-level summary reviews only.

## Problem

The agent already produces a structured verdict (`approve | request_changes | comment` plus summary, comments, concerns_addressed) and writes it to the vault task file — but that's invisible to the PR author. Goal success criterion #2 ("Verdict posted to PR — visible in GitHub review panel") cannot be met until the verdict turns into a real GitHub review event. Without this step the pipeline produces output no developer ever sees, so the agent fails its functional purpose even when its review quality is high.

## Goal

After this work, completing a pr-review task results in a real GitHub PR review event on the source PR, attributed to the bot account `pr-review-of-ben`, carrying the verdict's summary text as the review body. The verdict's `approve | request_changes | comment` value drives the GitHub review type, with `approve` gated behind a per-repo `autoApprove` opt-in (configured via `.pr-reviewer.yaml` in the target repo, default off when the file is missing). Re-running the agent on the same head SHA replaces the prior bot review rather than stacking duplicates. Posting happens inside the `in_progress` phase as the final step of "do the work"; the existing `ai_review` phase verifies both the verdict quality AND that the review was posted successfully. Failures to post (network, auth, closed PR) surface in the vault diagnostics and never crash the agent.

## Non-goals

- Inline file:line review comments (top-level summary only; inline is a later task using REST `POST /repos/.../pulls/{n}/comments`)
- Bitbucket Server / Bitbucket Cloud variant (separate idea: `bitbucket-inline-comments.md`)
- Posting on `verdict=failed` (infra error from spec 011 — Job phase fails, controller surfaces it; no review posted)
- Changing the verdict schema or the verdict parser (frozen by spec 011)
- Per-PR `autoApprove` overrides (per-repo only)
- Comment-on-`human_review` escalation (separate Task A5)
- Auto-merging on approve (only the review event is posted; merge is the human's job)
- Rewriting planning/execution/ai_review prompts (this is a new step, not a prompt rewrite)

## Desired Behavior

1. The `in_progress` phase, after producing the verdict and writing it to the vault, submits a GitHub review on the source PR using the verdict's value: `approve` → `gh pr review --approve`, `request_changes` → `gh pr review --request-changes`, `comment` → `gh pr review --comment`. The verdict's `summary` is always passed as the review body. Posting is the final step of `in_progress`; on success the phase advances to `ai_review`.
2. The `ai_review` phase keeps its existing role (independent verification that the verdict was sound) AND additionally verifies that the review was posted correctly: it lists reviews on the PR via `gh api`, filters to the bot account's review on the current head SHA, and confirms it exists with the expected verdict type. If the review is missing, the phase fails and surfaces a diagnostic.
3. Per-repo `autoApprove` is sourced from a `.pr-reviewer.yaml` file at the root of the TARGET PR's repo (read from the workdir, which `in_progress` already has checked out). If the file is missing OR `autoApprove: false`, `verdict=approve` is demoted to `--comment` and the body is prefixed with a one-line note that auto-approve is disabled. If the file declares `autoApprove: true`, `--approve` is posted as-is. The vault verdict remains `approve`; only the posted action changes.
4. Before posting, the agent calls `gh api /user` and asserts the authenticated identity equals `pr-review-of-ben` (the dedicated bot account). If the PAT belongs to a human operator (e.g. `bborbe`), the agent refuses to post and escalates to `human_review` with a diagnostic.
5. Before posting, the agent lists prior reviews on the PR (`gh api /repos/{owner}/{repo}/pulls/{n}/reviews`), filters to those authored by the bot identity for the same head SHA as the current task, and dismisses or deletes them so the new review replaces — not stacks on — prior ones.
6. When the verdict is `failed` (spec 011 infra error), no review is posted; the agent's existing failure path runs untouched.
7. GitHub-side failures (API down, PAT scope insufficient, PR closed/merged, rate limit hit) are caught, recorded in the vault diagnostic block, and escalated to `human_review` rather than crashing the agent. The vault verdict still reflects the agent's review opinion; only the posting attempt is marked failed.
8. The agent's tool allowlist for `in_progress` (the phase that now posts) is extended with exactly `Bash(gh pr review:*)` and `Bash(gh api:*)` — no `Bash(gh:*)`. The `ai_review` phase additionally needs `Bash(gh api:*)` for its post-verification step. Other phases' allowlists are unchanged.

## Constraints

- **Frozen verdict schema (spec 011)**: A4 reads the verdict but never modifies it. `verdict ∈ {approve, request_changes, comment, failed}`, `summary` string, `comments[]` of `{file, line, severity, message}`, `concerns_addressed[]`. The downstream parser tests must keep passing.
- **Bot identity is non-negotiable**: posting MUST use the PAT of the dedicated bot account `pr-review-of-ben`, never the human operator's token. **CORRECTION 2026-05-02**: the currently-wired secret (`ROnG5L` in teamvault) ALREADY belongs to `pr-review-of-ben` (verified via `gh api /user` from inside the watcher pod, login=`pr-review-of-ben`). The earlier assumption that `ROnG5L` was bborbe's PAT was incorrect. **A4 is therefore NOT blocked on bot-account provisioning** — only on (a) verifying the PAT has `repo` write scope (it likely needs adding if currently read-only), (b) wiring `gh pr review` calls in the agent. `gh api /user` self-check (asserting login == `pr-review-of-ben`) is the runtime enforcement. If the existing PAT lacks write scope, either rotate to a write-scoped PAT in the same teamvault entry or create a separate write-scoped key.
- **PAT scope**: the bot PAT requires `repo` write scope (needed by `gh pr review --approve`). Verification belongs in the `make verify-gh-token` target (extended from existing pattern).
- **Branch protection interaction**: a bot `approve` may count toward required reviews under branch-protection rules. This is operator-side configuration (GitHub repo settings), not a code concern; document in the runbook/architecture doc but do not attempt to detect or enforce in code.
- **Bash allowlist scoping**: extension must be prefix-scoped — `Bash(gh pr review:*)` and `Bash(gh api:*)`, never `Bash(gh:*)`. The trust boundary is the allowlist; broadening it to `gh:*` would unlock `gh repo delete`, `gh release create`, etc. The `gh api:*` pattern is broad enough to read review lists and dismiss reviews; narrower patterns are not feasible because `gh api` takes the full path as its first positional argument.
- **Existing guardrail relaxation**: the agent's posting prohibition (the comment in `factory.go` "Execution gets broader git/gh access for cross-file reads but still cannot post (no `gh pr comment` / `gh pr review`) — posting happens out-of-band after the human approves the verdict.") must be relaxed and the comment updated to reflect that `in_progress` is now the trusted poster (gated by bot-identity self-check + per-repo `.pr-reviewer.yaml`), and `ai_review` independently verifies the post happened.
- **No mutation of git history**: even though the new step has gh-write capability, it must not push, commit, branch, reset, or otherwise mutate repo state. The allowlist excludes those patterns by construction.
- **Read-only against the workdir**: the post step does not need the workdir at all; it operates against the GitHub API using the verdict and the original PR URL from the task's `source.pr_url` field. No worktree mount, no checkout dependency.
- **Existing knowledge to reference**:
  - `docs/architecture.md` "Agent contract" section — `source.pr_url` is the field this step reads.
  - `docs/architecture.md` "Verdict post-back" section (lines 64–70) describes this step at the architecture level; A4 is the implementation that fulfills it.
  - Spec 011 (`specs/in-progress/011-swap-execution-to-coding-pr-review.md`) — frozen verdict schema this spec consumes.
  - `agent/pr-reviewer/pkg/factory/factory.go` — phase wiring + per-phase `AllowedTools`. A4 adds a new phase (or extends ai_review) here.
- **Domain knowledge candidate (suggest creating)**: a new `docs/pr-post-back.md` documenting (a) the bot identity requirement and how to provision it, (b) the `autoApprove` model, (c) the duplicate-dismissal flow, (d) operator-side branch-protection considerations. Currently scattered across this spec; would benefit from a stable home so future provider variants (Bitbucket) can reference the same contract.

## Failure Modes

| Trigger | Expected behavior | Recovery |
|---------|-------------------|----------|
| GitHub API down / network failure | Verdict stays in vault; post step records the error in diagnostics; phase marked failed; controller schedules retry on next phase tick | Auto-retry; if persistent, escalate to `human_review` after N attempts |
| PAT lacks `repo` write scope (403) | Diagnostic recorded; phase escalates to `human_review`; no review posted | Operator rotates PAT or extends scopes |
| PR closed/merged between verdict generation and posting (422) | Log + skip posting; vault verdict preserved; phase completes successfully (review against a closed PR is moot) | None — expected race |
| `autoApprove: false` + `verdict=approve` | Demote posted review to `--comment` only; prefix body with "auto-approve disabled for this repo, review submitted as comment"; vault verdict stays `approve` | Operator opts the repo into `autoApprove: true` when ready |
| Bot identity check fails (PAT belongs to human) | Refuse to post; escalate to `human_review` with diagnostic naming the misconfiguration | Operator provisions dedicated bot account + PAT; updates the secret |
| Prior bot review exists for same head SHA | Dismiss/delete the prior review before posting fresh; new review replaces old | Automatic |
| Force-push: PR head_sha changed since verdict generation | Post review against the SHA the verdict was computed for (recorded in vault); a fresh task will be created by the watcher for the new SHA | Watcher's synchronize-detection (B2) handles re-review |
| Rate limit hit (429 / 5000-per-hour) | Log; phase marked retryable; controller retries on next tick; after N consecutive rate-limit failures, escalate to `human_review` | Auto-retry; operator inspects if persistent |
| `verdict=failed` (spec 011 infra error) | No review posted; agent's existing failure path runs; phase fails and controller surfaces it | Existing failure path |
| `summary` empty or missing | Post review with a default body ("automated review — no summary produced"); record warning in diagnostics; verdict still drives the review type | Operator inspects; usually indicates an upstream prompt regression |
| `gh pr review` exit code non-zero for unknown reason | Capture stderr in diagnostics; escalate to `human_review`; do not retry blindly | Operator inspects logs |

## Security / Abuse Cases

- **Bot identity is the trust boundary.** The PAT used for posting must belong to a dedicated bot account. If a human operator's PAT is used, every review the agent posts appears as if the human approved it — a regression against the goal and a real audit-trail violation (a flaky agent could "approve" PRs in the human's name). Runtime enforcement: `gh api /user` self-check before any posting attempt; refuse if the login matches a configured operator-account list. Provisioning enforcement: operator-side, documented in the post-back runbook.
- **Verdict body is LLM output, not user input — but is posted to GitHub verbatim.** The verdict's `summary` may contain LLM-generated text, which the agent constructs from a PR's diff (untrusted by PR author). GitHub renders the body as Markdown. The agent must NOT execute or shell-interpolate the summary; passing it as a single argument to `gh pr review --body "$summary"` (or via stdin) is sufficient. PR-author-controlled content cannot escape into shell commands as long as the body is treated as opaque text.
- **Bash allowlist scoping.** `Bash(gh pr review:*)` and `Bash(gh api:*)` are the only new patterns granted. They permit (a) posting/dismissing reviews and (b) calling read-only GitHub APIs. They do NOT permit `gh repo`, `gh release`, `gh issue`, `gh workflow`, etc. — the agent cannot delete the repo, cut a release, or open issues, even with the bot PAT. `gh api:*` is intentionally broad on the path side (cannot be narrowed without breaking duplicate-review dismissal); the bot PAT's scope is the second layer of defense.
- **Auto-approve gating prevents merge runaway.** A flaky agent that emits `verdict=approve` on a bad PR could, with auto-merge enabled on the repo, ship broken code without any human review. Defense in depth: (1) `autoApprove: false` default — bot reviews are comments-only by default; (2) operator opt-in is per-repo and explicit; (3) repo-level branch-protection should require a human reviewer in addition to the bot for any non-trivial repo. Document (3) in the post-back doc.
- **Duplicate-review dismissal is a write op via API.** The bot must dismiss its own prior reviews. It must NOT dismiss reviews authored by humans — filter strictly by login = bot account. Misimplementing this filter could cause the bot to dismiss real reviewer feedback on the PR. Acceptance criterion enforces this.
- **No new outbound network surface beyond GitHub API.** The post step talks to api.github.com via `gh`; existing pod network policy is sufficient.
- **Closed-PR posting is an enumeration weak signal but not exploitable.** A 422 reveals the PR is closed; this is already public via the PR's URL.

## Acceptance Criteria

- [ ] **Prerequisite**: the existing `ROnG5L` PAT (`pr-review-of-ben` bot identity, verified 2026-05-02) has `repo` write scope. Add the scope if currently missing, or provision a separate write-scoped PAT under a new teamvault key. Verified by `make verify-gh-token` after rotation: `X-OAuth-Scopes` header includes `repo` (not just `public_repo`), and `gh pr review --approve` succeeds against a test PR.
- [ ] Posting is wired into the agent's `in_progress` phase as the final step (after verdict is written to vault, before phase advances to `ai_review`). On `verdict=failed`, posting is skipped and existing failure path runs.
- [ ] `ai_review` phase additionally verifies the post happened: lists reviews via `gh api`, asserts a review by `pr-review-of-ben` exists for the current head SHA with the expected verdict type. Missing review → phase fails with diagnostic.
- [ ] The `in_progress` phase tool allowlist is extended with exactly `Bash(gh pr review:*)` and `Bash(gh api:*)`. The `ai_review` phase allowlist is extended with `Bash(gh api:*)` (read-only verification). No `Bash(gh:*)` anywhere. Other phases' allowlists are unchanged.
- [ ] Per-repo `autoApprove` config is read from `.pr-reviewer.yaml` at the root of the workdir (the PR's checkout). Schema: `autoApprove: bool` (other fields ignored for now). File missing OR `autoApprove: false` → `approve` demoted to `--comment`. File present with `autoApprove: true` → `--approve` posted as-is. Documented in `docs/pr-post-back.md`.
- [ ] Verdict-to-action mapping is implemented and tested: `approve` + `autoApprove:true` → `gh pr review --approve`; `approve` + `autoApprove:false` → `gh pr review --comment` with body prefix "auto-approve disabled for this repo, review submitted as comment"; `request_changes` → `gh pr review --request-changes`; `comment` → `gh pr review --comment`. Body always includes the verdict's `summary`.
- [ ] Bot-identity self-check: agent calls `gh api /user`, asserts login equals `pr-review-of-ben`, refuses to post otherwise. The expected login is wired through a CLI flag / env var (`BOT_GITHUB_LOGIN`, default `pr-review-of-ben`) so it can be changed without code edits.
- [ ] Duplicate-review dismissal: before posting, the agent lists reviews on the PR via `gh api`, filters to those by the bot login on the same head SHA, and dismisses or deletes each. Reviews by other authors are never touched.
- [ ] All failure modes (GitHub down, 403, 422, 429) are handled per the table: diagnostics recorded, escalation to `human_review` where appropriate, no agent crash, vault verdict preserved.
- [ ] The `factory.go` comment block describing why "execution cannot post" is updated to reflect the new posting phase.
- [ ] `make precommit` passes in `agent/pr-reviewer/`.
- [ ] **Local smoke test**: `cd agent/pr-reviewer/cmd/run-task && make run-dummy-task` against PR #2 emits a real review on the GitHub UI under the bot account, with the verdict's summary as the body, and the verdict-to-action mapping correctly applied.
- [ ] **Scenario coverage** (integration seam — required by spec-writing.md "When to Write a Scenario"): a new `scenarios/NNN-pr-reviewer-post-verdict.md` exercises the full flow end-to-end against PR #2 and asserts: (a) review appears in GitHub UI under the bot account, (b) `autoApprove:false` demotes `approve` to `--comment` with the documented body prefix, (c) duplicate-dismissal works on a re-run against the same head SHA, (d) `verdict=request_changes` posts as `--request-changes`. Prompt-level tests cannot fake the GitHub API boundary; the scenario is the only layer that runs the real path.
- [ ] `docs/pr-post-back.md` exists and covers (a) bot-identity provisioning steps for `pr-review-of-ben`, (b) the `.pr-reviewer.yaml` schema and per-repo opt-in flow, (c) duplicate-dismissal mechanism, (d) operator-side branch-protection considerations. The doc is the durable home for institutional memory after this spec is implemented and archived.
- [ ] After dev deploy: triggering one PR via the watcher (B1) results in a vault task with a verdict AND a real review event on the PR within the goal's 10-min latency budget. This is the closing acceptance — goal success criterion #2 becomes observable in the GitHub UI.

## Verification

```
cd agent/pr-reviewer && make precommit
```

Local smoke test against PR #2 (will post a real review to bborbe/maintainer PR #2):

```
cd agent/pr-reviewer/cmd/run-task && make run-dummy-task
```

Inspect: the resulting review appears in PR #2's "Reviews" panel on github.com under the bot account, with the verdict's summary as the body. Re-run the same command — the prior review should be dismissed/replaced, not stacked.

After deploy to dev:

```
# Trigger one PR via the watcher; observe controller materialize the vault task,
# then confirm the review event appears on GitHub.
kubectlquant -n dev logs <pr-reviewer-agent-job-pod> | grep -E "gh pr review|gh api /user"

# Manually verify in GitHub UI: PR's "Reviews" panel shows a review by the bot
# account with the expected verdict type and summary body.
```

## Do-Nothing Option

Stay with verdict-in-vault-only. The agent continues to produce reviews no developer ever sees on the PR page, so the goal's #2 success criterion ("Verdict posted to PR — visible in GitHub review panel") cannot be met and the wave does not complete. Developers must manually check the vault to find feedback on their PRs — a workflow no one will adopt. The full pipeline pattern (watcher + agent + posting) the goal validates remains a half-pipeline. The pr-reviewer agent stays mechanically functional but operationally useless.

A weaker alternative — post a `--comment` review unconditionally regardless of verdict type, deferring the `--approve` / `--request-changes` mapping — saves the bot-identity and `autoApprove` work for later. Cost: the GitHub UI's green-check / red-X signal (the most valuable artifact for PR authors at a glance) is never produced; the bot is just another commenter. The bot-identity work is required regardless (a comment posted as the human operator is the same identity-confusion problem as an approve), so the deferral saves only the `autoApprove` config plumbing — modest savings, large UX cost. Not recommended.
