---
status: approved
spec: [027-post-verdict-to-github-pr]
created: "2026-05-15T18:00:00Z"
queued: "2026-05-15T17:15:23Z"
---

<summary>
- The in_progress phase now posts a real GitHub PR review after writing the ## Review section to the vault — vault write always precedes any GitHub API call
- The execution step accepts a `PrPoster` dependency injected via the constructor; it calls `Post` with PR coordinates parsed from the task body, the head SHA from frontmatter, and the parsed verdict + stripped summary from ## Review
- If posting fails (non-not-a-failure outcome), the phase escalates to `human_review` instead of `ai_review`; the vault verdict is always preserved regardless of API outcome
- Every posting attempt writes a structured YAML diagnostic block to a `## Diagnostics` section in the vault (append-only, one block per Job run), enabling operator triage without reading logs
- The 422 (PR closed) case is treated as success — vault verdict preserved, phase advances to `ai_review`
- The factory gains a `CreatePrPoster` function that wires `net/http.DefaultClient` with the bot PAT and bot login env vars; `CreateAgent` is updated to accept and inject the poster
- The comment in `factory.go` describing the execution-phase posting prohibition is updated to reflect that `in_progress` is now the trusted poster, gated by bot-identity self-check and per-repo `.pr-reviewer.yaml`
- A new `docs/pr-post-back.md` documents bot-identity provisioning, `.pr-reviewer.yaml` schema, duplicate-dismissal flow, verify-after-POST rationale, and branch-protection considerations for operators
- The execution-phase row in `docs/architecture.md` is updated to mention that the phase posts the review to GitHub via REST after writing the verdict
</summary>

<objective>
Wire the `PrPoster` from `pkg/githubposter/` into the execution phase. After Claude writes `## Review`, the step calls `Post`, appends a diagnostic block to the vault, and routes to `human_review` on failure or `ai_review` on success. Factory and docs are updated accordingly.
</objective>

<context>
Read `CLAUDE.md` at the repo root for project conventions.

Read these guides before writing any code (in `~/.claude/plugins/marketplaces/coding/docs/`):
- `go-patterns.md` — interface/constructor/struct, small interfaces
- `go-factory-pattern.md` — `Create*` prefix, zero-logic factories
- `go-error-wrapping-guide.md` — `bborbe/errors`, never `fmt.Errorf`
- `go-testing-guide.md` — Ginkgo/Gomega, Counterfeiter, coverage ≥80%
- `test-pyramid-triggers.md` in `~/.claude/plugins/marketplaces/coding/docs/` — which test types to write
- `go-composition.md` — DI, no package-level calls

**This prompt depends on prompt 1 having completed.** The following must exist before you start:
- `agent/pr-reviewer/pkg/githubposter/` package with `PrPoster`, `PostRequest`, `PostResult`, `ErrorClass`, `ReviewVerifier`, `VerifyRequest`
- `agent/pr-reviewer/mocks/pr-poster.go` (Counterfeiter mock `FakePrPoster`)
- `agent/pr-reviewer/mocks/review-verifier.go` (Counterfeiter mock `FakeReviewVerifier`)

If these files do not exist, STOP and report `{"status":"failed","message":"prompt 1 artifacts missing — cannot proceed"}`.

**Files to read fully before making any changes:**

1. `agent/pr-reviewer/pkg/steps_checkout_execution.go` — full file; the `checkoutExecutionStep` struct, `NewCheckoutExecutionStep`, `Run`, `runClaude`
2. `agent/pr-reviewer/pkg/steps_checkout_execution_test.go` — full file; understand test structure before modifying
3. `agent/pr-reviewer/pkg/factory/factory.go` — full file; `CreateAgent` call site, `executionTools` var, the posting-prohibition comment block
4. `agent/pr-reviewer/pkg/prurl.go` — `ParsePRURL`, `PRInfo` type (owner, repo, number)
5. `agent/pr-reviewer/pkg/verdict.go` — `ParseVerdict`, `StripJSONVerdict`
6. `agent/pr-reviewer/pkg/githubposter/types.go` — `PrPoster`, `PostRequest`, `PostResult`, `ErrorClass`
7. `agent/pr-reviewer/mocks/pr-poster.go` — generated `FakePrPoster` type; confirm its package name and import path
8. `agent/pr-reviewer/cmd/run-task/dummy-task.md` — understand task body format (how the PR URL appears in the body)
9. `agent/pr-reviewer/docs/architecture.md` — current execution-phase row wording
10. `CHANGELOG.md` (repo root) — check for existing `## Unreleased` section before adding

**Symbol verification — run before implementing:**

```bash
# Confirm agentlib.Markdown methods available for section manipulation:
grep -rn "func.*Markdown.*FindSection\|func.*Markdown.*ReplaceSection\|func.*Markdown.*Marshal\|func.*Markdown.*Body\|Frontmatter" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/... 2>/dev/null | head -20

# Understand how to read task body text from agentlib.Markdown:
grep -rn "\.Body\b\|\.Text\b\|\.Content\b" \
  $(go env GOPATH)/pkg/mod/github.com/bborbe/agent/lib@*/*.go 2>/dev/null | head -20

# Confirm trigger_count frontmatter key used in existing code:
grep -rn "trigger_count\|TriggerCount" agent/pr-reviewer/

# Confirm GH_TOKEN env key used in existing env map:
grep -rn "GH_TOKEN" agent/pr-reviewer/

# Confirm BOT_GITHUB_LOGIN env key (may not exist yet):
grep -rn "BOT_GITHUB_LOGIN" agent/pr-reviewer/
```

**How to get the PR URL from the task body:**

The task body (the text content of the markdown before the first section) contains the PR URL as a link. After reading `dummy-task.md`, confirm the exact format. The simplest approach:

```go
// In steps_checkout_execution.go, after md.Marshal(ctx):
taskBytes, err := md.Marshal(ctx)
// ...
taskStr := string(taskBytes)
// Find the first https://github.com/.../pull/N URL in the task content.
```

If `agentlib.Markdown` has a `Body()` or `Text()` method that returns preamble text, use that instead of full serialization. Grep for it first.

Extract owner/repo/number using `ParsePRURL` from `pkg/prurl.go`.
</context>

<requirements>
Execute steps in order. Run `make test` in `agent/pr-reviewer/` after step 4. Run `make precommit` only at the final step.

---

## Step 1 — Read all referenced files fully

Read each file listed in `<context>` before writing any code. Do not skip any file.

---

## Step 2 — Update `agent/pr-reviewer/pkg/steps_checkout_execution.go`

Read the full file first.

### 2a. Add `prPoster` field to `checkoutExecutionStep`

Add a `prPoster githubposter.PrPoster` field to the struct. This field is optional at runtime (nil = skip posting, for backward compatibility with `cmd/run-task` which may not inject it). Add an import for `github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubposter`.

```go
type checkoutExecutionStep struct {
    repoManager     git.RepoManager
    claudeConfigDir claudelib.ClaudeConfigDir
    agentDir        claudelib.AgentDir
    model           claudelib.ClaudeModel
    env             map[string]string
    allowedTools    claudelib.AllowedTools
    reviewMode      string
    repoAllowlist   []string
    prPoster        githubposter.PrPoster // nil = skip posting
}
```

### 2b. Add `prPoster` parameter to `NewCheckoutExecutionStep`

Add `prPoster githubposter.PrPoster` as the last parameter. Callers that don't have a poster pass `nil`. `factory.go` (step 3) will pass a real poster; `cmd/run-task` uses `nil` (unchanged behavior).

```go
func NewCheckoutExecutionStep(
    repoManager git.RepoManager,
    claudeConfigDir claudelib.ClaudeConfigDir,
    agentDir claudelib.AgentDir,
    model claudelib.ClaudeModel,
    env map[string]string,
    allowedTools claudelib.AllowedTools,
    reviewMode string,
    repoAllowlist []string,
    prPoster githubposter.PrPoster,
) agentlib.Step {
    return &checkoutExecutionStep{
        repoManager:     repoManager,
        claudeConfigDir: claudeConfigDir,
        agentDir:        agentDir,
        model:           model,
        env:             env,
        allowedTools:    allowedTools,
        reviewMode:      reviewMode,
        repoAllowlist:   repoAllowlist,
        prPoster:        prPoster,
    }
}
```

### 2c. Modify `runClaude` to call the poster after writing ## Review

Read the existing `runClaude` method. Currently it:
1. Runs Claude
2. Writes ## Review to md
3. Returns `AgentStatusDone, NextPhase:"ai_review"`

Replace step 3 with the new posting sequence. The new `runClaude` should:

1. Run Claude (unchanged)
2. Write `## Review` to md (unchanged)
3. If `s.prPoster == nil`: return `AgentStatusDone, NextPhase:"ai_review"` (backward-compatible skip)
4. Extract verdict + summary from the `## Review` section:
   - Get the `## Review` section body from `md.FindSection("## Review")`
   - Call `ParseVerdict(reviewSection.Body)` to get `verdict`
   - Call `StripJSONVerdict(reviewSection.Body)` to get `summary` (removes the JSON verdict line)
5. Extract PR URL from the task content:
   - Call `md.Marshal(ctx)` (or use a Body() accessor if available) to get the full task text
   - Use a regex to find the first `https://github.com/{owner}/{repo}/pull/{number}` URL
   - Call `ParsePRURL(ctx, prURL)` to get `owner`, `repo`, `number`
   - If ParsePRURL fails: append diagnostic block (class=permanent) and escalate to human_review
6. Get head SHA from frontmatter: `ref, _ := md.Frontmatter.String("ref")`
7. Get GH_TOKEN from `s.env["GH_TOKEN"]`
8. (DROPPED) `BOT_GITHUB_LOGIN` is read ONCE in `CreateAgentProvider` (factory.go) and bound to the poster at construction time. The step does not re-read it.
9. Get worktree path (already available as `worktreePath` parameter in `runClaude`)
10. Build `PostRequest` and call `s.prPoster.Post(ctx, req)`
11. Append diagnostic block to md (always — success and failure)
12. Based on `result.Outcome`:
    - `"success"` or `result.Class == ErrorClassNotAFailure` → return `AgentStatusDone, NextPhase:"ai_review"`
    - `"failed"` → return `AgentStatusDone, NextPhase:"human_review", Message: "posting failed: "+result.ErrorMessage`

**Diagnostic block format** (spec DB#9):

Read the `## Diagnostics` section (or use empty string if absent). Append a new fenced YAML block:

```go
func buildDiagnosticBlock(jobRunTime time.Time, triggerCount int, result githubposter.PostResult) string {
    if result.Outcome == "success" {
        return fmt.Sprintf("job_run: %s outcome: success review_id: %d\n",
            jobRunTime.UTC().Format(time.RFC3339), result.ReviewID)
    }
    httpStatusStr := "null"
    if result.HTTPStatus != 0 {
        httpStatusStr = fmt.Sprintf("%d", result.HTTPStatus)
    }
    respBody := result.ResponseBody
    if respBody == "" {
        respBody = "<empty>"
    }
    return fmt.Sprintf("```yaml\njob_run: %s\ntrigger_count: %d\noutcome: failed\nfailure_step: %s\nclass: %s\nescalate_hint: %v\nattempt: %d\nhttp_status: %s\nerror_message: %q\nresponse_body: %q\nelapsed_ms: %d\n```\n",
        jobRunTime.UTC().Format(time.RFC3339),
        triggerCount,
        result.FailureStep,
        result.Class,
        result.EscalateHint,
        result.Attempt,
        httpStatusStr,
        result.ErrorMessage,
        respBody,
        result.ElapsedMs,
    )
}
```

To append to `## Diagnostics` (nil-safe — `FindSection` returns `(*Section, bool)` and is nil when absent):
```go
var existingBody string
if existing, ok := md.FindSection("## Diagnostics"); ok && existing != nil {
    existingBody = existing.Body
}
newBody := strings.TrimLeft(existingBody+"\n"+buildDiagnosticBlock(...), "\n")
md.ReplaceSection(agentlib.Section{Heading: "## Diagnostics", Body: newBody})
```

Get trigger_count from frontmatter using the typed accessor: `md.Frontmatter.TriggerCount()` returns `int` (0 if absent). Do NOT scan for the raw key.

**PR URL extraction**: cache `prURL` ONCE at the top of `runClaude`, before any `md.ReplaceSection` call mutates the body — scanning `md.Marshal()` after `## Review` is written would match a URL inside the review body itself. Extract from the preamble (task body before the first `## ` heading).

**PR URL handling rules** (one consistent rule, no synthesis):
- GitHub PR URL found → proceed with posting
- Bitbucket URL OR no URL at all → skip posting (do NOT synthesize `owner="unknown"`); write a `class: permanent` Diagnostics entry naming the missing/non-GitHub URL; phase returns `AgentStatusFailed` → controller escalates per trigger_count cap. (Bitbucket parity is out of scope for spec 027.)

---

## Step 3 — Update `agent/pr-reviewer/pkg/factory/factory.go`

Read the full file first.

### 3a. Add `CreatePrPoster` factory function

Add after `CreateFileResultDeliverer`:

```go
// CreatePrPoster wires a PrPoster backed by net/http.DefaultClient.
// token is the bot PAT (GH_TOKEN env); botLogin is the bot GitHub login (BOT_GITHUB_LOGIN env,
// default "pr-review-of-ben" if empty). Pure plumbing; no logic.
func CreatePrPoster(token, botLogin string) githubposter.PrPoster {
    return githubposter.NewPrPoster(http.DefaultClient, token, botLogin)
}
```

Add imports: `"net/http"` and `"github.com/bborbe/maintainer/agent/pr-reviewer/pkg/githubposter"`.

### 3b. Update `CreateAgent` to accept and inject `prPoster`

Add `prPoster githubposter.PrPoster` as the last parameter of `CreateAgent`. Pass it to `NewCheckoutExecutionStep`:

```go
func CreateAgent(
    claudeConfigDir claudelib.ClaudeConfigDir,
    agentDir claudelib.AgentDir,
    model claudelib.ClaudeModel,
    ghToken string,
    env map[string]string,
    repoManager git.RepoManager,
    reviewMode string,
    repoAllowlist []string,
    prPoster githubposter.PrPoster,
) *agentlib.Agent {
    // ... (tokenCheck, planningStep unchanged)
    executionStep := prpkg.NewCheckoutExecutionStep(
        repoManager,
        claudeConfigDir,
        agentDir,
        model,
        env,
        executionTools,
        reviewMode,
        repoAllowlist,
        prPoster, // new parameter
    )
    // ... (reviewStep, agentlib.NewAgent unchanged)
}
```

### 3c. Update `CreateAgentProvider` to construct and pass the poster

In `CreateAgentProvider`, construct the poster from `ghToken` and the `BOT_GITHUB_LOGIN` from `env`:

```go
func CreateAgentProvider(
    claudeConfigDir claudelib.ClaudeConfigDir,
    agentDir claudelib.AgentDir,
    model claudelib.ClaudeModel,
    ghToken string,
    env map[string]string,
    repoManager git.RepoManager,
    reviewMode string,
    repoAllowlist []string,
) agentlib.AgentProvider {
    botLogin := env["BOT_GITHUB_LOGIN"] // empty → poster uses its default "pr-review-of-ben"
    poster := CreatePrPoster(ghToken, botLogin)
    domainAgent := CreateAgent(
        claudeConfigDir, agentDir, model, ghToken, env, repoManager, reviewMode, repoAllowlist,
        poster,
    )
    // ... (healthcheckRunner, livenessAgent, NewAgentProvider unchanged)
}
```

### 3d. Update the per-phase tool-scope comment block

Replace the existing comment block (currently says "execution still cannot post..."):

```go
// Per-phase tool scopes. Principle: each phase gets the smallest set that
// lets it do its job. Planning + Review are read-only inspection. Execution
// gets broader git access for cross-file reads; posting happens in-process
// via the PrPoster (Go net/http, not gh CLI) after the LLM step completes,
// gated by bot-identity self-check (GET /user == pr-review-of-ben) and
// per-repo .pr-reviewer.yaml (autoApprove: bool). The ai_review phase
// independently verifies the post via GET /pulls/{n}/reviews before
// advancing to done.
```

---

## Step 4 — Update callers of `CreateAgent`

Grep for all other callers (besides `CreateAgentProvider`):

```bash
grep -rn "CreateAgent\b" agent/pr-reviewer/
```

- **`pkg/factory/runner.go` (around line 77)** — `cfg.Agent == nil` fallback constructs `CreateAgent`. **Inject a real poster** here so `cmd/run-task` actually exercises posting locally — required by spec AC line 211 ("local smoke test posts a real review"). Build `poster := CreatePrPoster(cfg.GHToken, cfg.Env["BOT_GITHUB_LOGIN"])` and pass it as the last argument. Do NOT pass nil — that disables the AC's smoke test.
- **`pkg/factory/factory_test.go`** — pass `nil` as the last argument for tests that don't need posting; for the test exercising the end-to-end factory wiring, use a `FakePrPoster` instead.
- Any other caller: pass `nil` if posting is irrelevant; pass a real or fake poster otherwise.

---

## Step 5 — Run `make test` (fast feedback)

```bash
cd agent/pr-reviewer && make test
```

Fix compile errors. Common causes:
- `NewCheckoutExecutionStep` call sites with wrong argument count
- `CreateAgent` call sites in tests missing the `prPoster` argument
- Missing imports

---

## Step 6 — Update `agent/pr-reviewer/pkg/steps_checkout_execution_test.go`

Read the full test file first. Add tests for the new posting behavior. Use `FakePrPoster` from `mocks/`.

Import: `"github.com/bborbe/maintainer/agent/pr-reviewer/mocks"` (check the actual package name from the generated mock file).

Add a new `Describe("posting behavior", func() {...})` context inside the existing suite. Test:

1. **Poster is nil — advances to ai_review without posting**: set up a step with `prPoster=nil`, run it with a task that has a valid PR URL + review section → result has `NextPhase=="ai_review"`, `FakePrPoster.PostCallCount()==0`

2. **Successful post — advances to ai_review**: mock the poster to return `PostResult{Outcome:"success", ReviewID:42}` → result has `NextPhase=="ai_review"`, `## Diagnostics` section is written to md, diagnostic body contains `"outcome: success review_id: 42"`

3. **Failed post — escalates to human_review**: mock the poster to return `PostResult{Outcome:"failed", Class:ErrorClassTransient, ErrorMessage:"timeout"}` → result has `NextPhase=="human_review"`, `## Diagnostics` section contains failure block

4. **422 (not-a-failure) — advances to ai_review**: mock the poster to return `PostResult{Outcome:"success", Class:ErrorClassNotAFailure}` → result has `NextPhase=="ai_review"`

5. **## Review vault is preserved regardless of poster outcome — table-driven across ALL ErrorClass values.** Use `DescribeTable` enumerating every `pkg.githubposter.ErrorClass` value (`transient`, `permanent`, `unknown`, `not-a-failure`, `soft-warning`). For each: run runClaude with the verdict, mock the poster to return the corresponding outcome, then assert `md.FindSection("## Review").Body` equals the canonical post-runClaude body. Prevents future ErrorClass additions from silently breaking the vault-first invariant (spec AC line 201).

6. **Diagnostic blocks are append-only**: call the posting logic twice via two separate runs (or simulate two calls by pre-populating `## Diagnostics`) and assert the second block is appended after the first

Write the task fixture (test task body) to include a GitHub PR URL for `bborbe/maintainer/pull/2`.

---

## Step 7 — Create `agent/pr-reviewer/docs/pr-post-back.md`

Create a new documentation file. Content:

```markdown
# PR Post-Back — GitHub Review Posting

How the pr-reviewer agent posts its verdict as a real GitHub PR review event.

## Bot Identity Provisioning

The agent posts reviews under the `pr-review-of-ben` bot account. This account requires a PAT with `repo` write scope stored at teamvault key `ROnG5L`. The PAT is injected into the agent container as `GH_TOKEN`. A separate env var `BOT_GITHUB_LOGIN` names the expected login (default `pr-review-of-ben`); the agent self-checks identity via `GET /user` before any POST.

**Why not `gh` CLI?** The `gh` CLI ignores `GH_TOKEN` when system keychain auth exists, so it may post as the operator's account rather than the bot. The agent uses `net/http` directly — identity is enforced at the HTTP-header level (`Authorization: token $GH_TOKEN`), independent of container keychain state.

## Per-Repo Auto-Approve

Create `.pr-reviewer.yaml` at the repository root to opt into auto-approve:

```yaml
autoApprove: true
```

| Config | Verdict | Posted as |
|--------|---------|-----------|
| file missing | `approve` | `COMMENT` (with note: "auto-approve disabled for this repo") |
| `autoApprove: false` | `approve` | `COMMENT` (with note: "auto-approve disabled for this repo") |
| `autoApprove: true` | `approve` | `APPROVE` |
| any | `request-changes` | `REQUEST_CHANGES` (never demoted) |

The vault verdict stays `approve` regardless of the demoted action — the vault records what the agent concluded; GitHub records what action the bot took.

## Duplicate-Review Dismissal

Before each POST, the agent calls `GET /pulls/{n}/reviews` and dismisses all prior reviews by `pr-review-of-ben` on the current head SHA via `PUT /pulls/{n}/reviews/{id}/dismissals`. Reviews by humans or by the bot on other SHAs are never touched.

On a controller-triggered re-spawn (after a transient failure), this dismissal runs again first — if the prior phantom POST actually persisted, it gets cleaned up before the fresh POST. This makes the posting sequence idempotent across retries.

## Verify-After-POST

A `POST /pulls/{n}/reviews` returning 200 is not proof of persistence (empirical 2026-05-15: POST returned a synthetic review object that never appeared in `GET /pulls/{n}/reviews`). After every POST, the agent immediately calls `GET /pulls/{n}/reviews` and asserts the new review exists with the expected state. On failure, the call is retried once (transient). A second failure escalates to `human_review` with a diagnostic.

## Branch-Protection Considerations

A bot `APPROVE` counts toward required-review rules in GitHub branch protection. For production repos, configure branch protection to require at least one _human_ reviewer in addition to the bot. This prevents a misconfigured or flaky agent from allowing merges on bad PRs. See GitHub docs: "Require approvals" under branch protection rules.

The `autoApprove: false` default (missing config) is the first defense. Branch-protection requiring human reviews is the second. Together they prevent merge runaway even if the agent emits a spurious `approve` verdict.

## Diagnostics

Every posting attempt writes a YAML block to the `## Diagnostics` section of the vault task. On success, a one-line entry is written. On failure, a multi-field block is written including `class: transient|permanent|unknown|not-a-failure`, `escalate_hint: true|false`, and the first 500 bytes of the GitHub API response body. These blocks are append-only (one per Job run) to preserve history across controller re-spawns.
```

---

## Step 8 — Update `agent/pr-reviewer/docs/architecture.md`

Read the full file first.

In the Phase table (around the `execution` row), update the `Emits` column to:

```
**the review verdict** (`approve` / `request_changes`); **posts review to GitHub via REST** (bot-identity gated, per-repo auto-approve, verify-after-POST)
```

Keep the rest of the row unchanged.

---

## Step 9 — Add CHANGELOG entry

If `## Unreleased` already exists in `CHANGELOG.md` (it may have been created by prompt 1), append to it. Otherwise create it. Add:

```
- feat(agent/pr-reviewer): wire PrPoster into in_progress phase — verdict is posted to GitHub as a real PR review after ## Review is written; vault verdict preserved on API failure; diagnostic blocks written to ## Diagnostics; per-repo autoApprove via .pr-reviewer.yaml; factory updated; docs/pr-post-back.md created
```

---

## Step 10 — Run `make precommit`

```bash
cd agent/pr-reviewer && make precommit
```

Must exit 0.
</requirements>

<constraints>
- **Vault-first invariant**: `## Review` must be written to `md` (via `ReplaceSection`) BEFORE calling `s.prPoster.Post(...)`. Code ordering enforces this — no re-arrangement.
- **Vault verdict is preserved on failure**: if posting fails, the `## Review` section is untouched. The step writes `## Diagnostics` and escalates to `human_review` — it does NOT modify `## Review`.
- **`prPoster == nil` skips posting silently** — backward-compatible for `cmd/run-task` which may not inject a poster.
- **Only GitHub PR URLs are posted.** If `ParsePRURL` returns `Platform != PlatformGitHub`, log a warning and skip posting (do not fail). Bitbucket posting is explicitly out of scope.
- **No new Bash tool allowlist entries.** Posting uses Go `net/http` inside the step; the Claude subprocess tool list is unchanged.
- **`CreateAgent` signature changes**: update ALL call sites (factory.go, factory_test.go, and any others found by grep). Passing `nil` is the correct value for callers that don't inject a poster.
- **`CreateAgentProvider` must not change its own signature** — it creates the poster internally from `ghToken` and `env["BOT_GITHUB_LOGIN"]`.
- **Factory comment updated** — the old "execution cannot post" statement must be replaced exactly as specified in step 3d.
- **Do NOT modify `cmd/run-task/main.go`** or `cmd/cli/main.go` — they use a different code path (`RunConfig.Agent`) and are not affected by this change.
- **Do NOT modify `pkg/verdict.go` or `pkg/prurl.go`** — use them read-only.
- **Frozen verdict schema**: no `comment` verdict value anywhere in this or added code.
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

Confirm poster is injected:
```bash
grep -n "prPoster\|PrPoster\|CreatePrPoster" agent/pr-reviewer/pkg/steps_checkout_execution.go agent/pr-reviewer/pkg/factory/factory.go
```
Expected: field in struct, parameter in constructor, factory function.

Confirm vault-first ordering in runClaude:
```bash
grep -n "ReplaceSection.*Review\|prPoster.Post\|## Review\|## Diagnostics" agent/pr-reviewer/pkg/steps_checkout_execution.go
```
Expected: ReplaceSection("## Review") appears BEFORE the prPoster.Post call.

Confirm factory comment updated:
```bash
grep -n "cannot post\|PrPoster\|gated by bot" agent/pr-reviewer/pkg/factory/factory.go
```
Expected: old "cannot post" text absent; new gated-by-bot-identity comment present.

Confirm docs created/updated:
```bash
ls agent/pr-reviewer/docs/pr-post-back.md
grep -n "posts review to GitHub" agent/pr-reviewer/docs/architecture.md
```
Expected: pr-post-back.md exists; architecture.md mentions REST posting in execution row.

Confirm no Bash tool allowlist change:
```bash
git diff agent/pr-reviewer/pkg/factory/factory.go | grep -E "^\+" | grep -E "Bash\(gh pr|gh pr review|gh pr comment"
```
Expected: zero matches (no new Bash tools added).

Confirm no comment verdict:
```bash
grep -rn '"comment"\|VerdictComment' agent/pr-reviewer/pkg/steps_checkout_execution.go agent/pr-reviewer/pkg/factory/factory.go
```
Expected: zero matches.

Confirm CHANGELOG entry:
```bash
grep -n "in_progress.*poster\|pr-post-back\|PrPoster.*in_progress" CHANGELOG.md
```
Expected: one entry under ## Unreleased.
</verification>
