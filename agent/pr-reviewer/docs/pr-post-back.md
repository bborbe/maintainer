# PR Post-Back — How Reviews Are Delivered to GitHub

After Claude writes the `## Review` section, the execution phase posts the verdict back to the PR as a GitHub review. This document describes the posting contract, the vault-first invariant, diagnostics, and failure routing.

## Vault-First Invariant

`## Review` is written to the task file **before** any GitHub API call. The order in `checkoutExecutionStep.runClaude` is strict:

1. Claude writes the review body as `runResult.Result`.
2. `md.ReplaceSection(Section{Heading: "## Review", Body: runResult.Result})` — review is in vault.
3. `postAndRoute(...)` — GitHub API call happens only after the vault is updated.

This means the review is never lost: if the pod crashes between steps 2 and 3, the controller re-spawns and sees `## Review` already present — `ShouldRun` returns false and the step is skipped (idempotent). If the pod crashes after step 3, the diagnostic block records the outcome.

## What Gets Posted

The poster receives:
- **Verdict** (`approve` / `request-changes`) — extracted from the JSON block at the end of `## Review` by `ParseVerdict`.
- **Summary** — the `## Review` body with the JSON verdict block stripped, via `StripJSONVerdict`. This is the human-readable part GitHub shows on the review.
- **HeadSHA** — the `ref` frontmatter field, used to anchor the review to the exact commit.
- **WorkDir** — the worktree path, used by the poster to read `.pr-reviewer.yaml` for `autoApprove` config.

## The Posting Flow

```
postAndRoute
  │
  ├─ nil poster? → skip (advance to ai_review, no diagnostic written)
  │
  ├─ extract verdict + summary from ## Review body
  ├─ parse PR URL from task preamble (captured before any md mutations)
  ├─ check platform (non-GitHub → skip posting, advance to ai_review, write diagnostic)
  │
  ├─ PrPoster.Post(ctx, PostRequest{...})
  │     │
  │     ├─ bot identity self-check (GET /user == BOT_GITHUB_LOGIN)
  │     ├─ autoApprove config read (.pr-reviewer.yaml in worktree)
  │     ├─ POST /repos/{owner}/{repo}/pulls/{n}/reviews
  │     └─ verify-after-POST (GET /pulls/{n}/reviews to confirm review appears)
  │
  ├─ appendDiagnosticsSection(md, buildDiagnosticBlock(...))
  │     → always written, success or failure, append-only
  │
  └─ route:
        outcome=success OR class=not-a-failure → advance to ai_review
        anything else                           → advance to human_review
```

## Diagnostic Block Format

One block is appended to `## Diagnostics` per Job run (append-only; history is preserved across re-triggers).

**Success** (compact one-liner):
```
job_run: 2026-05-15T12:00:00Z outcome: success review_id: 12345
```

**Failure** (fenced YAML block):
```yaml
job_run: 2026-05-15T12:00:00Z
trigger_count: 2
outcome: failed
failure_step: post
class: transient
escalate_hint: false
attempt: 2
http_status: 500
error_message: "internal server error"
response_body: "<html>..."
elapsed_ms: 342
```

`failure_step` names the step where the error occurred (`pr_url_extraction`, `pr_url_parse`, `pr_url_platform`, `bot_identity`, `post`, `verify`). `class` is one of `transient`, `permanent`, `unknown`, `not-a-failure`. `escalate_hint` is true when the poster's retry logic recommends human escalation.

## Failure Routing

| Posting outcome | Class | Next phase |
|---|---|---|
| `success` | any | `ai_review` |
| any | `not-a-failure` | `ai_review` |
| any | `transient` / `permanent` / `unknown` | `human_review` |

The `not-a-failure` class covers expected non-error states: 422 Unprocessable Entity (PR already closed or merged), duplicate review (already reviewed at this SHA). These are not errors — the review is simply no longer relevant.

`human_review` is a terminal state that routes the task to a human operator. The full diagnostic block in `## Diagnostics` gives the operator everything needed to diagnose and re-trigger if appropriate.

## nil Poster — Local / Backward-Compatible Mode

`prPoster` is `nil` when using `cmd/run-task` (local test runner). A nil poster skips the entire posting flow and advances directly to `ai_review` without writing any diagnostic. This preserves backward compatibility with the local CLI mode.

## Key Files

| File | Purpose |
|---|---|
| `pkg/poster_types.go` | `PrPoster` interface, `PostRequest`, `PostResult`, `ErrorClass` |
| `pkg/githubposter/poster.go` | Concrete HTTP implementation of `PrPoster` |
| `pkg/githubposter/verifier.go` | verify-after-POST logic |
| `pkg/githubposter/retry.go` | One-retry transient error policy |
| `pkg/steps_checkout_execution.go` | `postAndRoute`, `buildDiagnosticBlock`, `appendDiagnosticsSection` |
| `pkg/factory/factory.go` | `CreatePrPoster` constructor wired into `CreateAgent` |
