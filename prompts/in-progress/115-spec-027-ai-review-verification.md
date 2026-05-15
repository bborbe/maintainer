---
status: committing
spec: [027-post-verdict-to-github-pr]
container: maintainer-115-spec-027-ai-review-verification
dark-factory-version: v0.156.1-1-g04f3863-dirty
created: "2026-05-15T18:00:00Z"
queued: "2026-05-15T17:15:23Z"
started: "2026-05-15T18:28:38Z"
---

<summary>
- The ai_review phase adds a post-verification step: after running Claude's quality checks, it calls `GET /pulls/{n}/reviews` to confirm the in_progress posting actually persisted on GitHub
- Verification is skipped when `verdict=failed` was recorded (no post was attempted) or when the most-recent `## Diagnostics` block from in_progress shows `class: permanent` or `class: unknown` (post cannot succeed on retry)
- When verification fails, ai_review writes a diagnostic line to `## Diagnostics` and exits with `failed`, so the controller can re-spawn the Job — same retry loop as in_progress failures
- The `reviewStep` accepts a `ReviewVerifier` dependency (Counterfeiter-mocked in tests); nil = skip verification (backward-compatible)
- `docs/architecture.md` gains a 4th ai_review check in its consistency-check section
- A scenario file `scenarios/017-pr-reviewer-post-verdict.md` is written as a manual verification checklist covering the full end-to-end post-verdict flow (including a Rung-1 simulation where ai_review's verification triggers on an empty review list)
- The factory wires a `ReviewVerifier` (backed by `net/http.DefaultClient` + bot PAT) into the ai_review step via `CreateAgent`
- All existing ai_review quality checks (concerns addressed, hallucinations, verdict consistency) run unconditionally — the new verification step is additive, never a replacement
</summary>

<objective>
Add post-verification to the ai_review phase and write the scenario file. The `reviewStep` gains a `ReviewVerifier` dependency that calls `GET /pulls/{n}/reviews` with skip conditions for non-posting cases. The scenario exercises the full end-to-end flow including the phantom-POST failure mode.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.

Read these guides before writing any code (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-patterns.md` — interface/constructor/struct, counterfeiter annotations
- `go-error-wrapping-guide.md` — `bborbe/errors`
- `go-testing-guide.md` — Ginkgo/Gomega, Counterfeiter, coverage ≥80%
- `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` — which test types to write

**This prompt depends on both prompt 1 and prompt 2 having completed.** The following must exist:
- `agent/pr-reviewer/pkg/githubposter/` package with `ReviewVerifier`, `VerifyRequest`, `VerifyResult`, `ErrorClass`
- `agent/pr-reviewer/mocks/review-verifier.go` (Counterfeiter mock `FakeReviewVerifier`)
- `agent/pr-reviewer/pkg/steps_review.go` — `reviewStep` struct (to be modified)
- `agent/pr-reviewer/pkg/factory/factory.go` — `CreateAgent`, `CreatePrPoster` (to be extended with verifier)

If any of these are missing, STOP and report `{"status":"failed","message":"prior prompt artifacts missing — cannot proceed"}`.

**Files to read fully before making any changes:**

1. `agent/pr-reviewer/pkg/steps_review.go` — full file; `reviewStep` struct, `NewReviewStep`, `Run`, `extractVerdict`
2. `agent/pr-reviewer/pkg/steps_review_test.go` — full file; understand test structure
3. `agent/pr-reviewer/pkg/factory/factory.go` — full file; `CreateAgent`, `CreateAgentProvider`
4. `agent/pr-reviewer/pkg/githubposter/types.go` — `ReviewVerifier`, `VerifyRequest`, `VerifyResult`, `ErrorClass`
5. `agent/pr-reviewer/mocks/review-verifier.go` — generated `FakeReviewVerifier`; confirm import path
6. `agent/pr-reviewer/pkg/prurl.go` — `ParsePRURL`, `PRInfo`, `PlatformGitHub`
7. `agent/pr-reviewer/docs/architecture.md` — `ai_review`'s consistency check section (3 items)
8. `agent/pr-reviewer/pkg/prompts/review_workflow.md` — ai_review quality checks the LLM runs (to understand what "quality checks still run" means)
9. `CHANGELOG.md` (repo root) — check for existing `## Unreleased` section

**Scenario writing guide:**

Mirror the format of `scenarios/016-build-watcher-end-to-end.md` (most recent existing scenario in this repo). There is no separate `scenario-writing.md` — the convention is established by example.

**Symbol verification:**

```bash
# Confirm ReviewVerifier interface in githubposter package:
grep -n "ReviewVerifier\|VerifyReview\|VerifyRequest\|VerifyResult" agent/pr-reviewer/pkg/githubposter/types.go

# Confirm FakeReviewVerifier exists:
ls agent/pr-reviewer/mocks/ | grep -i "verifier\|reviewer"

# Confirm agentlib.Markdown FindSection can locate Diagnostics:
grep -n "FindSection\|ReplaceSection" agent/pr-reviewer/pkg/steps_checkout_execution.go

# Confirm how to detect verdict=failed in the ## Review section:
grep -n "ParseVerdict\|VerdictApprove\|VerdictRequestChanges" agent/pr-reviewer/pkg/verdict.go

# Check if there's a "verdict=failed" sentinel in the task body:
grep -rn "verdict.*failed\|failed.*verdict" agent/pr-reviewer/
```
</context>

<requirements>
Execute steps in order. Run `make test` in `agent/pr-reviewer/` after step 4. Run `make precommit` only at the final step.

---

## Step 1 — Read all referenced files fully

Read each file listed in `<context>` before writing any code.

---

## Step 2 — Update `agent/pr-reviewer/pkg/steps_review.go`

Read the full file first.

### 2a. Add `verifier` field to `reviewStep`

```go
type reviewStep struct {
    runner       claudelib.ClaudeRunner
    instructions claudelib.Instructions
    verifier     githubposter.ReviewVerifier // nil = skip verification
}
```

Add import: `"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubposter"`.

### 2b. Add `verifier` parameter to `NewReviewStep`

```go
func NewReviewStep(
    runner claudelib.ClaudeRunner,
    instructions claudelib.Instructions,
    verifier githubposter.ReviewVerifier,
) agentlib.Step {
    return &reviewStep{
        runner:       runner,
        instructions: instructions,
        verifier:     verifier,
    }
}
```

### 2c. Add post-verification to `Run`

Read the existing `Run` method carefully. The current flow:
1. Marshal task → build prompt → run Claude
2. Write ## Verdict to md
3. Extract verdict from Claude output
4. If verdict == "pass" → done, NextPhase="done"
5. Else → done, NextPhase="human_review"

Add post-verification AFTER writing ## Verdict (step 2) and BEFORE the verdict routing (step 4). The verification must happen even when the meta-verdict is "pass" — a successful quality check doesn't excuse a missing GitHub review.

**Skip conditions (checked before calling the verifier):**

```go
shouldVerify, err := s.shouldVerifyPost(ctx, md)
if err != nil {
    // Diagnostic parsing error — log and skip verification conservatively
    glog.Warningf("ai_review: skip-condition check failed err=%v; skipping verification", err)
    shouldVerify = false
}
```

Implement `shouldVerifyPost(ctx context.Context, md *agentlib.Markdown) (bool, error)`:

1. Check `## Review` section: if absent or body contains no JSON verdict and `ParseVerdict` returns `VerdictRequestChanges` with reason `"empty review text"` — treat as `verdict=failed`, skip (return false, nil).
   - More precisely: the AI execution failure case means there is no `## Review` section OR the review section has no structured content. Check `_, exists := md.FindSection("## Review")` first; if not found, return false.
2. Check the most-recent `## Diagnostics` block from in_progress for `class: permanent` or `class: unknown`:
   - Read the `## Diagnostics` section
   - Find the LAST occurrence of a YAML block (between \`\`\`yaml and \`\`\`)
   - Scan that block for a line matching `class: permanent` or `class: unknown`
   - If found → return false, nil (skip verification)
3. Otherwise → return true, nil (verification should run)

**Verification call (when shouldVerify == true):**

```go
if s.verifier != nil && shouldVerify {
    verifyResult := s.callVerifier(ctx, md)
    if verifyResult != nil {
        // Verification failed — write diagnostic and return failed
        appendVerifyDiagnostic(ctx, md, *verifyResult)
        return &agentlib.Result{
            Status:  agentlib.AgentStatusFailed,
            Message: fmt.Sprintf("ai_review: post verification failed: %s", verifyResult.ErrorMessage),
        }, nil
    }
}
```

Implement `callVerifier(ctx, md) *githubposter.VerifyResult`:

1. Parse PR URL from task (same approach as in_progress: extract from task preamble BEFORE `## ` headings — do NOT scan `md.Marshal()` after the verdict is written). Call `ParsePRURL`.
2. If not GitHub PR or parsing fails: log warning, return nil (skip — Bitbucket is out of scope).
3. Get head SHA from `md.Frontmatter.String("ref")`.
4. The `reviewStep` struct has new fields `ghToken string` and `botLogin string` (added in step 2d). Use `s.ghToken` and `s.botLogin`.
5. **`ExpectedStates`: pass the full set of valid post-review states**: `[]string{"APPROVED", "CHANGES_REQUESTED", "COMMENTED"}`. Rationale: in_progress already validated the exact state at POST time; ai_review's role here is to confirm persistence (a bot review exists for the head SHA), not to re-validate which event was used. Passing the full set avoids ai_review having to re-read `.pr-reviewer.yaml` and recompute the autoApprove demotion — a logic duplication that would race against config edits between phases. Prompt 1's verifier filters `user.login + commit_id + state ∈ ExpectedStates`; with all three states allowed, any bot review on the correct SHA matches. Do NOT modify `pkg/githubposter/` — keep the contract clean.

6. Call `s.verifier.VerifyReview(ctx, req)`.
7. If `result.Found`: return nil (success, no diagnostic needed).
8. If not found: return `&result`.

Implement `appendVerifyDiagnostic(ctx context.Context, md *agentlib.Markdown, result githubposter.VerifyResult)`:
Append a one-line entry under `## Diagnostics`, distinct from in_progress's fenced YAML blocks (so the operator can grep `ai_review verify:` for ai_review entries specifically):
```
ai_review verify: outcome=failed class=<class> escalate_hint=<hint> http_status=<n> error=<message>
```
Use the same nil-safe append pattern as in_progress prompt 2 (`FindSection` returns `(*Section, bool)`; guard nil; `TrimLeft` newlines; `ReplaceSection`). No return value — internal errors logged via `glog`; the failed `VerifyResult` is returned by `verifyPost` regardless.

### 2d. Add ghToken + botLogin fields to reviewStep

Add `ghToken string` and `botLogin string` fields to the `reviewStep` struct. Extend `NewReviewStep` to accept them as the last two arguments. Update the factory's `CreateAgent` call (and `CreatePrPoster` already resolves `botLogin` — pass the same resolved value). Do NOT re-read `BOT_GITHUB_LOGIN` from env here — the factory owns the env-var resolution and binds the value to both the poster and the reviewStep at construction time.

---

## Step 3 — Update `agent/pr-reviewer/pkg/factory/factory.go`

Read the full file first.

### 3a. Add `CreateReviewVerifier` factory function

```go
// CreateReviewVerifier wires a ReviewVerifier backed by net/http.DefaultClient.
// token is the bot PAT; botLogin is the expected bot login.
func CreateReviewVerifier(token, botLogin string) githubposter.ReviewVerifier {
    return githubposter.NewReviewVerifier(http.DefaultClient, token, botLogin)
}
```

### 3b. Update `CreateAgent` to accept and inject `verifier`

Add `verifier githubposter.ReviewVerifier` as a parameter after `prPoster`:

```go
func CreateAgent(
    // ... existing parameters ...
    prPoster githubposter.PrPoster,
    verifier githubposter.ReviewVerifier,
) *agentlib.Agent {
    // ...
    reviewStep := prpkg.NewReviewStep(
        CreateClaudeRunner(claudeConfigDir, agentDir, model, env, reviewTools),
        prompts.BuildReviewInstructions(),
        verifier, // new parameter
    )
    // ...
}
```

### 3c. Update `CreateAgentProvider` to construct and pass the verifier

```go
func CreateAgentProvider(...) agentlib.AgentProvider {
    botLogin := env["BOT_GITHUB_LOGIN"]
    poster := CreatePrPoster(ghToken, botLogin)
    verifier := CreateReviewVerifier(ghToken, botLogin)
    domainAgent := CreateAgent(
        claudeConfigDir, agentDir, model, ghToken, env, repoManager, reviewMode, repoAllowlist,
        poster,
        verifier,
    )
    // ... (unchanged)
}
```

### 3d. Update `CreateAgent` callers with `nil` verifier

Grep for all `CreateAgent` call sites:
```bash
grep -rn "CreateAgent\b" agent/pr-reviewer/
```

Pass `nil` as the `verifier` argument where not explicitly provided (tests that don't need verification).

---

## Step 4 — Run `make test` (fast feedback)

```bash
cd agent/pr-reviewer && make test
```

Fix compile errors before proceeding.

---

## Step 5 — Update `agent/pr-reviewer/pkg/steps_review_test.go`

Read the full file first. Add tests for the verification behavior.

Import `FakeReviewVerifier` from mocks. Add a new `Describe` context for verification:

**Test: skip verification when ## Review is absent**
- Create a task with no `## Review` section
- Use a `FakeReviewVerifier` — after the step runs, assert `VerifyReviewCallCount() == 0`
- Meta-verdict goes to `human_review` (unparseable Verdict output) — acceptable

**Test: skip verification when Diagnostics shows permanent**
- Create a task with `## Review` present AND `## Diagnostics` containing `class: permanent`
- Run the step — assert `VerifyReviewCallCount() == 0`

**Test: skip verification when Diagnostics shows unknown**
- Same but `class: unknown` in diagnostics

**Test: verification runs and succeeds**
- Task has `## Review`, diagnostics block with `class: transient`, verifier mock returns `VerifyResult{Found:true, Outcome:"success"}`
- Assert `VerifyReviewCallCount() == 1`; final result NextPhase=="done" or "human_review" (depending on meta-verdict)

**Test: verification runs and fails — ai_review exits failed**
- Verifier returns `VerifyResult{Found:false, Outcome:"failed", Class:ErrorClassTransient, ErrorMessage:"review not found"}`
- Assert result `Status == AgentStatusFailed`
- Assert `## Diagnostics` section in md contains the verification failure message

**Test: verification skips for nil verifier**
- `NewReviewStep(runner, instructions, nil)`
- No panic; step runs normally

**Test: shouldVerifyPost handles ## Diagnostics absent**
- Task has `## Review` (valid verdict) but NO `## Diagnostics` section
- Assert `shouldVerifyPost` returns `(true, nil)` — verification should run

**Test: shouldVerifyPost selects the MOST RECENT diagnostic block when multiple exist**
- Task has `## Diagnostics` containing TWO YAML blocks: first with `class: permanent` (older — trigger_count 0), second with `class: transient` (newer — trigger_count 1)
- Assert `shouldVerifyPost` returns `(true, nil)` — the newer block is transient, so verification proceeds despite the older permanent entry. This protects against cross-Job-respawn cases where an earlier permanent failure should NOT block verification on the current successful run.

**Test: diagnostic format round-trip with prompt 2's exact output**
- Use the literal fenced YAML block that prompt 2's `buildDiagnosticBlock` produces (copy from spec DB#9 schema): `\`\`\`yaml\njob_run: ...\ntrigger_count: 1\noutcome: failed\nfailure_step: POST /pulls/2/reviews\nclass: permanent\nescalate_hint: true\n...\n\`\`\``
- Place it under `## Diagnostics` in a fixture task
- Assert `shouldVerifyPost` returns `(false, nil)` — parses the YAML, extracts `class: permanent`, decides to skip
- Critical: this is the **boundary test** that catches whitespace/tag/key-name mismatches between prompt 2's writer and prompt 3's reader. If prompt 2 changes its output format, this test fails fast.

---

## Step 6 — Update `agent/pr-reviewer/docs/architecture.md`

Read the full file. In the "ai_review's Consistency Check" section (currently listing 3 checks), add a 4th:

```markdown
4. **Post verification** (conditional): did the `in_progress` step successfully post the review to GitHub? `GET /pulls/{n}/reviews` asserts a review by `pr-review-of-ben` exists for the current head SHA. Skipped when `verdict=failed` (no post attempted) or when the `## Diagnostics` block shows `class: permanent` or `class: unknown` (post cannot succeed on retry). On failure: ai_review exits `failed`; controller may re-spawn.
```

Also update the phase table row for `ai_review` in the Emits column to mention the new 4th check:

Change `verdict consistency)` at the end of the ai_review Emits description to:
`verdict consistency, post verification (conditional))`

---

## Step 7 — Write `scenarios/017-pr-reviewer-post-verdict.md`

Read `scenarios/016-build-watcher-end-to-end.md` to anchor the format (most recent existing scenario).

Write the scenario to `scenarios/017-pr-reviewer-post-verdict.md`. The number `017` is the next free slot — confirm by `ls scenarios/ | grep -E '^[0-9]{3}-' | sort | tail -1` (should show `016-...`). The `spec` field inside the scenario frontmatter uses the full slug: `spec: 027-post-verdict-to-github-pr` (matching the convention in `016-build-watcher-end-to-end.md`).

```markdown
---
status: draft
spec: ["027"]
---

# Scenario: pr-reviewer posts verdict to GitHub PR

End-to-end verification that completing a pr-review task results in a real GitHub review event
on the source PR, attributed to `pr-review-of-ben`.

## Prerequisites

- Dev environment is deployed and healthy
- PR #2 on `github.com/bborbe/maintainer` exists (permanent test fixture)
- The watcher and pr-reviewer agent are running in the `dev` namespace
- `pr-review-of-ben` PAT is configured at teamvault key `ROnG5L`

## Setup

1. Confirm no existing `pr-review-of-ben` reviews on PR #2 for the current head SHA:
   - Navigate to `https://github.com/bborbe/maintainer/pull/2`
   - Check "Conversation" tab — note any existing bot reviews (they will be dismissed on run)
2. Confirm `.pr-reviewer.yaml` is absent from the `bborbe/maintainer` repo root (or note its current `autoApprove` value)

## Local Smoke Test (Rung-1)

Run the agent locally against a task fixture that references PR #2:

```
cd agent/pr-reviewer/cmd/run-task && make run-dummy-task
```

**Expected:**
- Command exits 0
- The resulting task file has a `## Review` section with a non-empty verdict
- The resulting task file has a `## Diagnostics` section with `outcome: success review_id: <N>`
- On GitHub: `https://github.com/bborbe/maintainer/pull/2` shows a new review under "pr-review-of-ben" in the Reviews panel, with the verdict's summary as the body

**Re-run test (duplicate dismissal):**
Run `make run-dummy-task` again against the same PR + head SHA.

**Expected:**
- No second review stacks on top of the first
- The prior bot review is dismissed and replaced
- `## Diagnostics` in the output file shows a new `outcome: success review_id: <M>` line (different review ID from the first run)

## Auto-Approve Demotion Test (Rung-1)

With `.pr-reviewer.yaml` absent (or `autoApprove: false`) at the target repo root:

1. Force the agent to produce `verdict=approve` (edit the dummy-task fixture to contain a review with only Nice-to-Have items, or use a worktree with clean code)
2. Run `make run-dummy-task`

**Expected:**
- `## Diagnostics` shows `outcome: success`
- On GitHub: the review appears as a **COMMENT** (not APPROVE), with body starting "auto-approve disabled for this repo, review submitted as comment"
- Vault `## Review` verdict stays `approve`

## REQUEST_CHANGES Test (Rung-1)

Force the agent to produce `verdict=request-changes`:

1. Edit the dummy-task fixture or use a worktree with a Must Fix issue
2. Run `make run-dummy-task`

**Expected:**
- On GitHub: the review appears as **REQUEST_CHANGES** (red X in PR review panel)
- No demotion regardless of `autoApprove` setting

## Phantom POST Simulation (Unit Test — see poster_test.go)

The phantom-POST failure mode (POST returns 200 but review is absent in subsequent GET) is verified
by the unit tests in `pkg/githubposter/poster_test.go`. The scenario does not require simulating
a real GitHub phantom POST, which is not reproducible on demand.

**Confirm unit test covers it:**
```bash
cd agent/pr-reviewer && go test ./pkg/githubposter/... -v -run "phantom"
```
Expected: at least one test that mocks a phantom POST and asserts transient escalation.

## Dev Deploy Verification (Rung-2)

After deploying to dev:

1. Push a commit to a PR in a watched repository
2. Observe the controller materialise a vault task for the new (PR, SHA) pair
3. Wait for the watcher → agent pipeline to complete (~10 min latency budget)
4. Confirm on GitHub: the PR's Reviews panel shows a review by `pr-review-of-ben` with the verdict's summary as the body

```bash
kubectlquant -n dev logs <pr-reviewer-agent-job-pod> | grep -E "POST.*reviews|GET.*user|dismiss|outcome"
```
Expected: logs show identity check, optional dismissal, POST, verify-GET, and `outcome: success`.

5. Re-trigger by pushing another commit to the same PR
6. Confirm the old bot review is dismissed and a new one appears (no stacking)
```

---

## Step 8 — Add CHANGELOG entry

If `## Unreleased` exists, append to it. Otherwise create it. Add:

```
- feat(agent/pr-reviewer): add post-verification to ai_review phase — ReviewVerifier calls GET /pulls/{n}/reviews to confirm review persistence; skip conditions for verdict=failed and permanent diagnostics; failure exits agent with failed status; scenario file added
```

---

## Step 9 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0.
</requirements>

<constraints>
- **Existing quality checks run unconditionally.** The Claude LLM step (concerns, hallucinations, verdict consistency) runs in all cases — verification is additive, not a replacement. The only conditional logic is whether to call the verifier at all.
- **`verifier == nil` skips verification silently** — backward-compatible for `cmd/run-task` which passes nil.
- **Skip conditions use only the LAST diagnostics block** from in_progress. Older blocks (from prior trigger_count attempts) are ignored — a re-spawn that succeeds clears the skip condition.
- **Empty `ExpectedState` in VerifyRequest** should match any state if the verifier implementation supports it. If the verifier from prompt 1 requires a specific state, extend it to accept empty string as "any state matching the bot + SHA criteria".
- **Only GitHub PRs are verified.** If `ParsePRURL` returns non-GitHub platform or fails, log warning and skip verification (return nil from `callVerifier`). Bitbucket is out of scope.
- **GH_TOKEN availability in reviewStep**: if the runner's env is not accessible, add `ghToken` and `botLogin` fields to `reviewStep` and set them in `NewReviewStep`. Update the factory to pass them.
- **Frozen verdict schema**: no `comment` verdict. `VerdictApprove` and `VerdictRequestChanges` are the only valid values.
- **`CreateAgent` signature changes**: update ALL call sites found by grep. Pass `nil` verifier where appropriate.
- **Do NOT modify `cmd/run-task/main.go`** or `cmd/cli/main.go`.
- **Scenario file**: write to `scenarios/017-pr-reviewer-post-verdict.md` (017 = next free number after `016-build-watcher-end-to-end.md`). dark-factory does NOT manage `scenarios/` numbering — the prompt picks the number explicitly.
- Do NOT commit — dark-factory handles git.
- `make precommit` runs from `agent/pr-reviewer/`, never at repo root.
- Test coverage ≥80% for modified packages.
- Error wrapping: `github.com/bborbe/errors` only — never `fmt.Errorf` in production code paths.
</constraints>

<verification>
Run precommit:
```bash
cd agent/pr-reviewer && make precommit
```
Expected: exit 0.

Confirm verifier is injected:
```bash
grep -n "verifier\|ReviewVerifier\|CreateReviewVerifier" agent/pr-reviewer/pkg/steps_review.go agent/pr-reviewer/pkg/factory/factory.go
```
Expected: field in struct, parameter in constructor, factory function.

Confirm skip conditions:
```bash
grep -n "shouldVerify\|class.*permanent\|class.*unknown\|FindSection.*Review\|FindSection.*Diagnostic" agent/pr-reviewer/pkg/steps_review.go
```
Expected: skip-condition check present; reads ## Review and ## Diagnostics.

Confirm 4th ai_review check in architecture:
```bash
grep -n "Post verification\|post.*verif\|GET.*reviews.*ai_review" agent/pr-reviewer/docs/architecture.md
```
Expected: one match in the ai_review consistency check section.

Confirm scenario file:
```bash
ls scenarios/017-pr-reviewer-post-verdict.md
grep -n "phantom\|REQUEST_CHANGES\|dismiss\|pr-review-of-ben" scenarios/017-pr-reviewer-post-verdict.md | wc -l
```
Expected: file exists; at least 4 matches for the key behaviors.

Confirm no comment verdict:
```bash
grep -rn '"comment"\|VerdictComment' agent/pr-reviewer/pkg/steps_review.go agent/pr-reviewer/pkg/factory/factory.go
```
Expected: zero matches.

Confirm CHANGELOG:
```bash
grep -n "ai_review.*verif\|ReviewVerifier\|post.*verif" CHANGELOG.md
```
Expected: one entry under ## Unreleased.

Confirm test coverage:
```bash
cd agent/pr-reviewer && go test -coverprofile=/tmp/cover.out ./pkg/... && go tool cover -func=/tmp/cover.out | grep -E "steps_review|githubposter"
```
Expected: ≥80% for both packages.
</verification>
