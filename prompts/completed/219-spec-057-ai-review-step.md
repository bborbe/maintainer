---
status: completed
spec: [057-github-releaser-ai-review-phase]
summary: Implemented ai_review step for github-releaser agent with ReviewChecks/ReviewOutput types, AIReviewClient interface, ErrTagNotFound sentinel, and three verification checks (tag exists, tag at expected SHA, CHANGELOG header rewritten)
container: maintainer-releaser-ai-review-exec-219-spec-056-ai-review-step
dark-factory-version: v0.173.0
created: "2026-05-31T20:35:00Z"
queued: "2026-05-31T20:54:57Z"
started: "2026-05-31T20:54:58Z"
completed: "2026-05-31T20:58:12Z"
branch: dark-factory/github-releaser-ai-review-phase
---

<summary>
- Add ReviewOutput type with approved, checks, and notes fields
- ai_review step returns Status=Done+terminal on success, Status=Failed on any verification failure
- Controller handles unassign + ## Failure envelope (NOT the step)
- Step returns Status=Failed with NO next_phase on verification failure
</summary>

<objective>
Implement the ai_review step for the github-releaser agent. This step performs three remote verification checks against the GitHub REST API (tag exists, tag points to expected commit, CHANGELOG header rewritten) and writes a `## Review` section to the task file. On success, the task advances to terminal-completed. On any failure, it returns `Status: failed` so the controller's standard escalation path applies.
</objective>

<context>
Read `agent/github-releaser/pkg/steps_execution.go` for the existing step pattern.
Read `agent/github-releaser/pkg/plan_output.go` for how PlanOutput is structured as a typed section.
Read `agent/github-releaser/pkg/result_output.go` for how ResultOutput is structured.
Read `agent/github-releaser/pkg/factory/factory.go` to understand the factory wiring.
Read `agent/github-releaser/pkg/steps_planning.go` for how agentlib.ExtractSection and MarshalSectionTyped are used.
Read `agent/github-releaser/pkg/githubchangelog/fetcher.go` for the HTTP fetch pattern.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md` for error wrapping conventions.
Read `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md` for factory conventions.

Key verified symbols from module source:
- `agentlib.Step` interface at `github.com/bborbe/agent/lib@v0.63.11/agent_step.go`: `Run(ctx, *Markdown) (*Result, error)`, `Name() string`, `ShouldRun(ctx, *Markdown) (bool, error)`
- `agentlib.Result` struct: `Status AgentStatus`, `NextPhase string`, `Message string`
- `agentlib.AgentStatusDone` = `"done"`, `agentlib.AgentStatusFailed` = `"failed"`
- `agentlib.ParseMarkdown(ctx, string) (*Markdown, error)`
- `agentlib.ExtractSection[T any](ctx, *Markdown, string) (*T, error)`
- `agentlib.MarshalSectionTyped[T any](ctx, string, T) (Section, error)`
- `domain.TaskPhaseAIReview` = `"ai_review"` from `github.com/bborbe/vault-cli@v0.67.5/pkg/domain/task_phase.go:31`
- `domain.TaskPhasePlanning` = `"planning"`, `domain.TaskPhaseExecution` = `"execution"`
- `github.com/bborbe/errors`: `Wrapf(ctx, err, format, args...)`, `Wrap(ctx, err, msg)`, `Errorf(ctx, format, args...)`
- `glog.V(2).Infof`, `glog.V(2).Warningf`, `glog.V(2).Infof`
</context>

<requirements>
1. Create `agent/github-releaser/pkg/steps_ai_review.go` with the following content.

2. Define `ReviewChecks` struct with three boolean fields:
   ```go
   type ReviewChecks struct {
       TagExists             bool `json:"tag_exists"`
       TagAtExpectedSHA      bool `json:"tag_at_expected_sha"`
       ChangelogHeaderRewritten bool `json:"changelog_header_rewritten"`
   }
   ```

3. Define `ReviewOutput` struct:
   ```go
   type ReviewOutput struct {
       Approved bool         `json:"approved"`
       Checks   ReviewChecks `json:"checks"`
       Notes    string      `json:"notes"`
   }
   ```

4. Define the `AIReviewClient` interface (EXPORTED — the seam must be usable from the external `pkg_test` package in prompt 5):
   ```go
   // AIReviewClient is the seam for the three GitHub REST API calls.
   // Mock it in tests with a counterfeiter-generated mock.
   type AIReviewClient interface {
       // TagExists calls GET /repos/{owner}/{repo}/git/ref/tags/{tag} and
       // returns (tagSHA, nil) on 200, or ("", ErrTagNotFound) on 404
       // (the sentinel — step distinguishes 404 → verdict vs 5xx → retry),
       // or ("", wrapped error) on transport / other non-2xx.
       TagExists(ctx context.Context, owner, repo, tag string) (tagSHA string, _ error)

       // ResolveTagCommit calls GET /repos/{owner}/{repo}/git/tags/{sha} and
       // follows annotated tags to their underlying commit SHA. Returns the
       // commit SHA or a wrapped error.
       ResolveTagCommit(ctx context.Context, owner, repo, tagSHA string) (commitSHA string, _ error)

       // FetchChangelog calls GET /repos/{owner}/{repo}/contents/CHANGELOG.md
       // (no ?ref= — relies on API defaulting to the repo's default branch).
       // Returns base64-decoded file bytes or a wrapped error.
       FetchChangelog(ctx context.Context, owner, repo string) ([]byte, error)
   }
   ```

   Also define the package-level sentinel error that prompt 2's HTTP client returns on 404:
   ```go
   // ErrTagNotFound is returned by AIReviewClient.TagExists on a 404 response.
   // The step uses errors.Is(err, ErrTagNotFound) to distinguish 404 (verification
   // failure → write ## Review approved:false, return Status: failed) from
   // 5xx / transport errors (wrap and return; controller retries).
   var ErrTagNotFound = stderrors.New("ai_review: tag not found")
   ```
   (Import `stderrors "errors"` for the sentinel, alongside `github.com/bborbe/errors` for wrapping.)

5. Define `aiReviewStep` struct with two injected fields:
   ```go
   type aiReviewStep struct {
       client  AIReviewClient
       ghToken string
   }
   ```

6. Constructor: `func NewAIReviewStep(client AIReviewClient, ghToken string) agentlib.Step`

7. Implement `agentlib.Step`:
   - `Name()` returns `"github-release-ai-review"`
   - `ShouldRun()` always returns `true, nil`
   - `Run(ctx, md *agentlib.Markdown)` per the logic below

8. In `Run`:
   a. Extract `## Result` section using `agentlib.ExtractSection[ResultOutput](ctx, md, "## Result")`. If the section is missing or decode fails, return a wrapped error (NOT a Review verdict) so the controller's standard failure path runs. Wrapping message: `"ai_review: extract ## Result section"`.
   b. If `result.Outcome != ResultOutcomeReleased`, write `## Review` with `approved: true`, all three check booleans initialized to `true` (vacuously — nothing to verify), and notes: `"execution step recorded failure; nothing to verify"`, then return `&agentlib.Result{Status: agentlib.AgentStatusDone, NextPhase: "done"}`. Zero HTTP calls.
   c. Read frontmatter `repo` field (format: `owner/name`). Return wrapped error if missing or malformed. Wrapping message: `"ai_review: read frontmatter repo"`.
   d. Initialize `checks := ReviewChecks{TagExists: true, TagAtExpectedSHA: true, ChangelogHeaderRewritten: true}` (default to vacuously-true; set to `false` only when a specific check fails). Define a helper `failVerdict(notes string) (*agentlib.Result, error)` that marshals `## Review` with `approved: false`, the current `checks` values, the given `notes`; calls `md.ReplaceSection(section)`; returns `&agentlib.Result{Status: agentlib.AgentStatusFailed, Message: notes}, nil` — no `NextPhase`.
   e. Call `tagSHA, err := client.TagExists(ctx, owner, name, result.Tag)`.
      - If `errors.Is(err, ErrTagNotFound)` (the sentinel from prompt 2 — explicitly a 404): set `checks.TagExists = false`; return `failVerdict(fmt.Sprintf("tag %s not found on remote", result.Tag))`.
      - On any OTHER error (5xx, transport, timeout, malformed JSON): return wrapped error (no `## Review`). Wrapping message: `"ai_review: TagExists"`. Controller retries via Kafka redelivery.
   f. Call `commitSHA, err := client.ResolveTagCommit(ctx, owner, name, tagSHA)`.
      - On error: return wrapped error with message `"ai_review: ResolveTagCommit"` (transient failure → retry).
      - If `commitSHA != result.CommitSHA`: set `checks.TagAtExpectedSHA = false`; return `failVerdict(fmt.Sprintf("tag points to %s, expected %s", commitSHA, result.CommitSHA))`.
   g. Call `changelogBytes, err := client.FetchChangelog(ctx, owner, name)`.
      - On error: return wrapped error with message `"ai_review: FetchChangelog"` (transient failure → retry).
      - Scan for the first `## ` heading: split on `\n`, find the first line matching the regex `^##\s+` (case-sensitive). If that line equals `## Unreleased` exactly (after trimming trailing whitespace), set `checks.ChangelogHeaderRewritten = false`; return `failVerdict("CHANGELOG.md top section is still ## Unreleased on default branch")`.
   h. All three pass: write `## Review` with `approved: true`, all three checks `true`, notes: `"all checks passed"`. Return `&agentlib.Result{Status: agentlib.AgentStatusDone, NextPhase: "done"}` — the literal `"done"` string is the terminal-completed signal per `agent/lib v0.63.x` `agent_agent.go:91`.

9. Use `agentlib.MarshalSectionTyped(ctx, "## Review", output)` to create the section, then `md.ReplaceSection(section)` to write it.

10. **The step MUST NOT mutate `md.Frontmatter` at all** — no `assignee`, no `previous_assignee`, no `status`, no `phase`. On `Status: failed` the **controller** performs the unassign + sets `previous_assignee: github-releaser-agent` + appends `## Failure` envelope (per spec 039 / [[Controller Stop Setting human_review on Agent Failure]] doctrine). On `Status: done` + `NextPhase: "done"` the controller writes the terminal-completed mutation. The agent's contract is "do the work, return result + content"; the controller owns the envelope.

11. The step must NOT write a `## Failure` section directly — that is the controller's responsibility on `Status: failed` per spec 039.

12. Logging:
    - `glog.V(2).Infof("ai_review: starting checks for repo=%s tag=%s commit=%s", owner, name, result.Tag, result.CommitSHA)`
    - `glog.V(2).Infof("ai_review: check=%s result=%v", checkName, passed)`
    - `glog.V(2).Infof("ai_review: all checks passed")`
    - `glog.V(2).Infof("ai_review: check=%s failed: %v", checkName, err)`
    - On HTTP errors: `glog.V(2).Infof("ai_review: GitHub API error: %v", err)` — never log the bearer token

13. The `aiReviewClient.FetchChangelog` must NOT hardcode `main` — the GitHub contents API defaults to the repo's default branch when no `?ref=` is passed (same as `githubchangelog/fetcher.go`).

14. Error wrapping: use `errors.Wrapf(ctx, err, "...")` for cause-preserving wraps, `errors.Errorf(ctx, "...")` for new errors. Never bare `return err`, never `fmt.Errorf`.
</requirements>

<constraints>
- The `## Plan` and `## Result` section shapes are frozen — do not modify them.
- Do NOT add any configurable knob, opt-out flag, or tunable threshold for verification rules.
- Do NOT add a retry loop inside the step — controller's Kafka redelivery is the retry surface.
- Do NOT return `next_phase: human_review` on verification failure.
- Do NOT clone the target repo — all checks are via GitHub REST API.
- `github.com/bborbe/errors` everywhere. No bare `return err`, no `fmt.Errorf`.
- Bearer token must never appear in any log line at any verbosity.
- Tests live in `pkg_test` package with Counterfeiter mocks.
- Existing tests under `pkg/...` and `pkg/factory/...` must continue to pass.
</constraints>

<verification>
Run `cd agent/github-releaser && make precommit` — must pass (this is the project's `validationCommand` per `.dark-factory.yaml`).
The step file must compile without errors.
</verification>