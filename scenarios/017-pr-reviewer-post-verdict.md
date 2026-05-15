---
status: active
spec: 027-ai-review-verification
---

# Scenario 017: ai_review post-verification — review confirmed, failed, and skipped

Validates the three post-verification outcomes added in spec-027:

1. **Happy path** — execution posts review, ai_review confirms it via `GET /pulls/{n}/reviews`, routes to `done`.
2. **Review absent** — execution posted but GitHub does not return the review within retry budget; ai_review writes a `ai_review verify:` diagnostic line and returns `AgentStatusFailed`.
3. **Skip on permanent class** — the last `## Diagnostics` YAML block has `class: permanent`; verifier is not called.

## Prerequisites

- Dev cluster is running and healthy
- `GH_TOKEN` (bot PAT) is set and `BOT_GITHUB_LOGIN` matches the token owner
- A test PR exists on `bborbe/maintainer` (PR #2 is the canonical dev fixture)

## Scenario A — happy path

### Setup

Create a task file whose `## Review` section exists and whose `## Diagnostics` block has `class: transient` (or is absent):

```markdown
---
phase: ai_review
ref: <head-sha>
---

Review the pull request at https://github.com/bborbe/maintainer/pull/2

## Plan

…planning output…

## Review

…execution output with verdict…
```

### Expected behaviour

1. ai_review LLM writes `## Verdict` with `{"verdict":"pass","reason":"…"}`.
2. `ReviewVerifier.VerifyReview` calls `GET /repos/bborbe/maintainer/pulls/2/reviews`.
3. Response includes a review submitted by the bot login → `Found: true`.
4. Step returns `AgentStatusDone`, `NextPhase: "done"`.
5. No `ai_review verify:` line appears in `## Diagnostics`.

### Verification

```bash
# Run local one-shot against the task file
cd agent/pr-reviewer && go run ./cmd/run-task/ --file /tmp/task.md
# Confirm final phase in output
grep "NextPhase" /tmp/task.md
```

## Scenario B — review absent (transient failure)

### Setup

Same task file as Scenario A. Configure the HTTP client stub to return an empty review list on both attempts.

### Expected behaviour

1. ai_review LLM writes `## Verdict`.
2. `ReviewVerifier.VerifyReview` makes two GET requests; both return an empty list → `Found: false`, `Class: transient`.
3. `appendVerifyDiagnostic` writes a line to `## Diagnostics`:
   ```
   ai_review verify: outcome=failed class=transient escalate_hint=false http_status=200 error=review not found
   ```
4. Step returns `AgentStatusFailed`, message contains `"post verification failed"`.

### Verification (unit test path)

```bash
cd agent/pr-reviewer && go test -run "verification runs and fails" ./pkg/...
```

## Scenario C — skip on permanent class

### Setup

Task file includes `## Review` **and** a `## Diagnostics` section whose last YAML block contains `class: permanent`.

```markdown
## Diagnostics

` ` `yaml
class: permanent
escalate_hint: true
http_status: 403
` ` `
```

### Expected behaviour

1. `shouldVerifyPost` reads the last YAML block, finds `class: permanent`.
2. Verifier is **not** called (`VerifyReviewCallCount() == 0`).
3. Step proceeds to parse verdict and routes normally.

### Verification (unit test path)

```bash
cd agent/pr-reviewer && go test -run "skip verification when Diagnostics shows class: permanent" ./pkg/...
```

## Pass criteria

- [ ] Scenario A: `NextPhase == "done"` and no `ai_review verify:` in diagnostics
- [ ] Scenario B: `AgentStatusFailed` and `## Diagnostics` contains `ai_review verify: outcome=failed`
- [ ] Scenario C: `VerifyReviewCallCount() == 0` confirmed by unit test
- [ ] `make test` passes in `agent/pr-reviewer/`
