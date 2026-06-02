---
status: completed
spec: [059-changelog-rewrite-opt-in-flag]
summary: Wired pkg/maintainerconfig.Fetcher into planning step; PlanOutput extended with 4 fields and 2 constants; 11 new Ginkgo tests added; 13 existing test fixtures rewired for new NewPlanningStep signature; make precommit exits 0
container: maintainer-changelog-rewrite-exec-231-spec-059-planning-flag-gating-and-fail-closed
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T18:30:00Z"
queued: "2026-06-02T18:59:48Z"
started: "2026-06-02T19:11:56Z"
completed: "2026-06-02T19:39:14Z"
branch: dark-factory/changelog-rewrite-opt-in-flag
---

<summary>
- The planning step now fetches `.maintainer.yaml` from the target ref via the new `pkg/maintainerconfig.Fetcher` (added in prompt 1) and resolves `release.changelogRewrite` before any LLM call
- When the flag is `false` (default, or explicitly false, or file absent, or `release:` block absent, or field absent): planning emits `## Plan` with `rewrite_needed=false`, `rewritten_unreleased` omitted, and the planning LLM is NEVER invoked for the rewrite call (LLM call count drops from two to one)
- When the flag is `true`: planning runs the existing 058 rewrite pipeline (second LLM call) and may emit `rewrite_needed=true` with a `rewritten_unreleased` body
- Non-boolean values (string, number) cause planning to fail fast with `Status=AgentStatusFailed`, `NextPhase=human_review`, and a new `## Plan(outcome=failed, error_category=invalid_config, invalid_field=release.changelogRewrite, invalid_value="<the bad value>")` block on the task page — no commit, no tag, no push
- The resolved flag value is recorded on the task page in the `## Plan` JSON (new `changelog_rewrite` field) so a reader can tell from the task page alone which mode the run took
- Factory wiring is updated in `pkg/factory/factory.go`; both `main.go` and `cmd/run-task/main.go` entry points are updated; every `NewPlanningStep` call site (test fixtures) is updated to pass the new fetcher
- Adds comprehensive Ginkgo coverage: short-circuit happy path (flag=false, clean + noisy `## Unreleased`), rewrite happy path (flag=true, noisy → `rewrite_needed=true`), no-rewrite-needed-when-flag-true-but-clean, fetch-failure-does-not-block-default (network error → treated as false, NOT invalid_config), empty-yaml-from-fetch (Fetch returns `(nil, nil)` → parsed as zero config → default false, distinct from the 404 path), invalid-value fail-closed (string + number), task-page audit-trail field, flag-read-once semantics (mutating the file mid-run has no effect)
</summary>

<objective>
Wire the new `pkg/maintainerconfig.Fetcher` into the planning step so the spec-059 `changelogRewrite` opt-in flag gates whether the 058 rewrite LLM call is invoked. Plumb the flag into the existing `PlanOutput` for task-page audit-trail visibility. Add a fail-closed error path for invalid (non-boolean) values that returns `Status=AgentStatusFailed` + `NextPhase=human_review` + a structured `## Plan(outcome=failed)` block naming the field and the invalid value. Update the factory and every `NewPlanningStep` call site to pass the new fetcher.
</objective>

<context>
Read `~/Documents/workspaces/maintainer-changelog-rewrite/CLAUDE.md` and `agent/github-releaser/CLAUDE.md` for project conventions.

Read these files BEFORE editing:
- `agent/github-releaser/pkg/steps_planning.go` — current planning step. EXTEND the `planningStep` struct with a `maintainerConfig` fetcher; do NOT replace the step. The `Run` method signature stays the same.
- `agent/github-releaser/pkg/plan_output.go` — current `PlanOutput`. ADD four new fields (`ChangelogRewrite *bool`, `ErrorCategory string`, `InvalidField string`, `InvalidValue string`) and a new `PlanOutcomeFailed = "failed"` constant. The existing fields and constants stay untouched (round-trip with persisted task pages must still decode).
- `agent/github-releaser/pkg/steps_planning_test.go` — existing Ginkgo style. The step constructor signature is changing (`NewPlanningStep(runner, fetcher, maintainerConfig)`) — every existing test fixture that calls `pkg.NewPlanningStep` MUST be updated to pass a `&mocks.MaintainerConfigFetcher{}` (default returns `(nil, nil)` which means "Fetch ok, empty bytes"; combined with prompt 1's locked contract `Parse(ctx, []byte{}) → (zero, nil)` this yields `changelogRewrite=false` cleanly — fine for the existing tests because the new fetcher is invoked AFTER the frontmatter + CHANGELOG.md validation gates that those tests exercise).
- `agent/github-releaser/pkg/githubchangelog/fetcher.go` — the existing `Fetcher` interface that the new `MaintainerConfigFetcher` mirrors. The mock lives at `mocks/fetcher.go`; the new mock lives at `mocks/maintainer_config_fetcher.go` (created by prompt 1's `go generate`).
- `agent/github-releaser/pkg/factory/factory.go` — `CreateAgent` wires `NewPlanningStep(planningRunner, fetcher)` today. ADD a third argument (the new `maintainerconfig.Fetcher`) — the call site for this lives in `factory.CreateAgent`; update it. `NewPlanningStep` is also called in test code (steps_planning_test.go) — those call sites are NOT in this file, but the `pkg.NewPlanningStep` signature change forces them to be updated.
- `agent/github-releaser/main.go` — the long-running agent entry. It calls `factory.CreateAgent(...)`; the new factory signature change is transparent to this file (no edits required) UNLESS the spec's signature change forces more arguments. Verify with grep after editing factory.go.
- `agent/github-releaser/cmd/run-task/main.go` — the one-shot CLI entry. Same: no edits required unless the signature change forces it.
- `agent/github-releaser/pkg/result_output.go` — current `ResultOutput`. The execution step will read the planning `## Plan` block; do NOT add the new fields here. The `## Plan` block (not `## Result`) is the audit-trail surface for the flag value.
- `agent/github-releaser/pkg/prompts/prompts.go` — the existing `prompts.ParseRewriteVerdict` etc. DO NOT EDIT.
- `agent/github-releaser/pkg/buildenv.go` — `BuildEnv` is the env-builder used by both entry points. DO NOT EDIT; the new fetcher is independent of `ANTHROPIC_*` env vars.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`

Verified symbols (from module source — grep-confirmed):
- `agentlib.ExtractSection[T any](ctx, *Markdown, string) (*T, error)` and `agentlib.MarshalSectionTyped[T any](ctx, string, T) (Section, error)` from `github.com/bborbe/agent/lib@v0.63.11`.
- `claudelib.ClaudeRunner` interface: `Run(ctx context.Context, prompt string) (*ClaudeResult, error)`; `ClaudeResult{Result string}`.
- `domain.TaskPhaseHumanReview = "human_review"`, `domain.TaskPhasePlanning = "planning"` from `github.com/bborbe/vault-cli@v0.67.5/pkg/domain/task_phase.go`.
- `agentlib.AgentStatusFailed` / `AgentStatusDone` / `AgentStatusNeedsInput` (string enums) from agentlib.
- `errors.Wrapf(ctx, err, format, args...)`, `errors.Errorf(ctx, format, args...)` from `github.com/bborbe/errors`.
- `mocks.Fetcher` and the new `mocks.MaintainerConfigFetcher` (generated by prompt 1's `go generate`).
- `pkg/maintainerconfig.Fetcher` (new) and `pkg/maintainerconfig.ErrFileNotFound` (new sentinel declared via `stderrors.New`), `pkg/maintainerconfig.Parse` (alias to lib parser, contract: `Parse(ctx, []byte{}) → (zero, nil)`) — all from prompt 1.
- `git.ErrorCategory` constants in `pkg/git/error_classifier.go` — do NOT add a new one; planning uses its own string `ErrorCategory` field on `PlanOutput` (the existing `ErrorCategory` on `ResultOutput` is `git.ErrorCategory` because the execution step is the one that surfaces it; planning has no `git.ErrorCategory` to classify into).
</context>

<requirements>

1. **Extend `PlanOutput`.** In `agent/github-releaser/pkg/plan_output.go`, ADD the following fields to the existing `PlanOutput` struct (do NOT touch the existing fields — round-trip with persisted task pages must still decode):

   ```go
   // ChangelogRewrite records the spec-059 per-repo opt-in flag value
   // resolved at planning entry. *bool (not bool) so the JSON distinguishes
   // "not resolved" from "resolved false" — the planning step ALWAYS sets
   // this field on the happy path (no-rewrite-needed AND rewrite-needed
   // outcomes), so a reader can audit which mode the run took. Omitted
   // from JSON only on the failure path (outcome="failed" carries the
   // error info instead).
   //
   // JSON encoding contract (load-bearing for spec AC #14 and prompt-2
   // requirement 5a/6 evidence):
   //   - happy path, flag false → field SET to &false → JSON emits
   //     literal substring `"changelog_rewrite":false`
   //   - happy path, flag true  → field SET to &true  → JSON emits
   //     literal substring `"changelog_rewrite":true`
   //   - failure path           → field LEFT nil     → omitempty omits
   //     the token entirely; `changelog_rewrite` is absent from the JSON
   ChangelogRewrite *bool `json:"changelog_rewrite,omitempty"`

   // ErrorCategory names the failure category on outcome="failed". For
   // spec 059 the only value is "invalid_config" (release.changelogRewrite
   // is non-boolean). Future failure categories may extend this set.
   ErrorCategory string `json:"error_category,omitempty"`

   // InvalidField names the .maintainer.yaml field that failed validation.
   // Populated on outcome="failed" only; today always "release.changelogRewrite".
   InvalidField string `json:"invalid_field,omitempty"`

   // InvalidValue captures the literal raw value that failed validation
   // (the YAML-decoded string/number/etc., as it appeared in the file).
   // Populated on outcome="failed" only.
   InvalidValue string `json:"invalid_value,omitempty"`
   ```

   Add a new outcome constant:

   ```go
   const (
       PlanOutcomeReady      = "ready"
       PlanOutcomeNeedsInput = "needs_input"
       // PlanOutcomeFailed signals a hard planning-time failure that
       // ends the task in human_review (Status=AgentStatusFailed,
       // NextPhase=human_review). Distinct from needs_input, which
       // keeps the task in_progress and waits for operator re-delegation.
       // See spec 059 § Desired Behavior 5 and § AC 11/12.
       PlanOutcomeFailed = "failed"
   )

   const (
       // ErrorCategoryInvalidConfig is the only valid value for
       // PlanOutput.ErrorCategory today. spec 059 § Failure Modes:
       // non-boolean release.changelogRewrite.
       ErrorCategoryInvalidConfig = "invalid_config"
   )
   ```

   Update the file's package doc-comment (the `// PlanOutput is the typed contract for the …` block at the top) to mention the third valid shape: `Outcome="failed" — invalid config; ErrorCategory + InvalidField + InvalidValue populated`.

2. **Extend `planningStep`.** In `agent/github-releaser/pkg/steps_planning.go`:

   a. Add a new constructor parameter and a new struct field. The new step has THREE injected seams:

   ```go
   type planningStep struct {
       runner           claudelib.ClaudeRunner
       fetcher          githubchangelog.Fetcher
       maintainerConfig maintainerconfig.Fetcher
   }

   // NewPlanningStep wires the planning step with its three IO seams:
   //   - the Claude runner (LLM verdict for bump + rewrite)
   //   - the CHANGELOG.md fetcher (GitHub contents API)
   //   - the .maintainer.yaml fetcher (GitHub contents API, spec 059)
   func NewPlanningStep(
       runner claudelib.ClaudeRunner,
       fetcher githubchangelog.Fetcher,
       maintainerConfig maintainerconfig.Fetcher,
   ) agentlib.Step {
       return &planningStep{
           runner:           runner,
           fetcher:          fetcher,
           maintainerConfig: maintainerConfig,
       }
   }
   ```

   **Imports.** Add to the import block: `maintainerconfig "github.com/bborbe/maintainer/agent/github-releaser/pkg/maintainerconfig"` (no alias needed — the package name is `maintainerconfig` and does not collide with anything in this file). Also add `stderrors "errors"` for the `errors.Is` call against `maintainerconfig.ErrFileNotFound` (the file currently imports only `github.com/bborbe/errors`; the stdlib alias is needed for `Is`).

   b. **Update `Run`'s flow** to insert the new `resolveChangelogRewrite` step. The new sequence (eight steps; the original six are renumbered but their behavior is preserved):

      ```
      1. Missing frontmatter        → escalate (NeedsInput, ## Plan needs_input, clear assignee)
      2. CHANGELOG fetch fails      → Failed (controller retries)
      3. P1/P2 validation fails     → escalate
      4. Claude verdict unparseable → Failed (controller retries)
      5. semver.BumpVersion fails   → escalate
      6. Resolve release.changelogRewrite from .maintainer.yaml at the ref's tip
         - ErrFileNotFound or any fetch transport error → treat as false, log V(2), continue
         - Parse error containing "unmarshal" → fail-closed (outcome=failed, error_category=invalid_config)
         - Resolved true  → run rewrite LLM call (existing 058 path)
         - Resolved false → SKIP rewrite LLM call, set plan.ChangelogRewrite=ptr(false), plan.RewriteNeeded=false
      7. Rewrite LLM call (only if step 6 returned true)
      8. Happy path                 → Done, NextPhase = execution, ## Plan ready
      ```

   c. Add a new helper `resolveChangelogRewrite` on `*planningStep`:

   ```go
   // resolveChangelogRewrite fetches .maintainer.yaml at the ref's tip and
   // returns the parsed release.changelogRewrite value, with these semantics
   // (per spec 059 § Desired Behavior and § Failure Modes):
   //
   //   - File absent (ErrFileNotFound)        → (false, nil)  // default, no error
   //   - File present, malformed YAML         → (false, wrappedErr) // fail-closed; caller maps to human_review
   //   - File present, non-boolean value      → (false, wrappedErr) // fail-closed; same
   //   - Any other fetch error (5xx, network) → (false, nil)  // treated as default; see Failure Modes row 9
   //
   // The "any other fetch error → default" rule is the spec's Failure Modes
   // "Repo has no .maintainer.yaml" + spec § Desired Behavior 6: missing-yaml
   // is treated as `false` cleanly. The spec does NOT extend fail-closed to
   // transport errors (those are usually transient GitHub flakes; the
   // operator can re-fire). Only the parse / non-boolean boundary is
   // fail-closed (a config typo on a high-trust field, per spec § Security).
   func (s *planningStep) resolveChangelogRewrite(
       ctx context.Context,
       owner, name, ref string,
   ) (bool, error)
   ```

   Implementation:
   ```go
   bytes, err := s.maintainerConfig.Fetch(ctx, owner, name, ref)
   if err != nil {
       if stderrors.Is(err, maintainerconfig.ErrFileNotFound) {
           glog.V(2).Infof("planning: .maintainer.yaml absent at ref=%s — using default changelogRewrite=false", ref)
           return false, nil
       }
       // Transport / non-404 error: log and default to false. NOT a
       // fail-closed condition (see spec 059 § Failure Modes).
       glog.Warningf("planning: .maintainer.yaml fetch failed (treated as default): %v", err)
       return false, nil
   }
   cfg, err := maintainerconfig.Parse(ctx, bytes)
   if err != nil {
       // YAML parse error or non-boolean value: fail-closed. Surface the
       // original error so the caller can include it in the human_review
       // task-page block.
       return false, errors.Wrapf(ctx, err, "parse .maintainer.yaml")
   }
   return cfg.Release.ChangelogRewrite, nil
   ```

   d. **Update `runClassification`** so it accepts the resolved `changelogRewrite` bool and branches:

   ```go
   func (s *planningStep) runClassification(
       ctx context.Context,
       md *agentlib.Markdown,
       currentVersion string,
       bullets []string,
       prefixStyle string,
       originalBody string,
       changelogRewrite bool,
   ) (*agentlib.Result, error)
   ```

   The new flow inside `runClassification`:
   - Bump classification: unchanged (the first LLM call stays — it produces the semver bump regardless of the flag).
   - **NEW branching after bump verdict**:
     - If `changelogRewrite == false`: skip the rewrite LLM call. Set `rewriteVerdict := prompts.RewriteVerdict{RewriteNeeded: false, RewrittenUnreleased: "", Reasoning: "changelogRewrite flag is false (default or explicit) — pre-058 header-rename-only behavior"}` directly. Add a `glog.V(2).Infof("planning: changelogRewrite=false — skipping rewrite LLM call")` line.
     - If `changelogRewrite == true`: invoke the existing `runRewrite` helper (unchanged).
   - The PlanOutput assembly adds:
     ```go
     crValue := changelogRewrite
     output := PlanOutput{
         // ... existing fields ...
         ChangelogRewrite: &crValue, // pointer so JSON distinguishes resolved-false from missing
     }
     ```

   e. **Update the call site in `Run`**: after `originalBody, err := changelog.ExtractUnreleasedBody(...)`, insert:

   ```go
   changelogRewrite, err := s.resolveChangelogRewrite(ctx, owner, name, ref)
   if err != nil {
       // Fail-closed: .maintainer.yaml is malformed OR contains a
       // non-boolean release.changelogRewrite. Write a ## Plan(failed)
       // block, set the controller to human_review, do NOT advance.
       return s.failInvalidConfig(ctx, md, currentVersion, "release.changelogRewrite", err)
   }
   return s.runClassification(ctx, md, currentVersion, bullets, prefixStyle, originalBody, changelogRewrite)
   ```

   f. **Add the new `failInvalidConfig` helper** (sibling to the existing `escalate`):

   ```go
   // failInvalidConfig writes a ## Plan(outcome=failed) section naming
   // the invalid field and the wrapped error, and returns
   // Status=AgentStatusFailed + NextPhase=human_review. The framework's
   // agent runner treats that combination as a terminal human_review
   // escalation (no retry, no advance). The task page is the audit
   // surface — a reader can grep for `error_category=invalid_config`
   // on the `## Plan` block to find the failure.
   func (s *planningStep) failInvalidConfig(
       ctx context.Context,
       md *agentlib.Markdown,
       currentVersion, field string,
       cause error,
   ) (*agentlib.Result, error)
   ```

   Implementation:
   ```go
   msg := ""
   if cause != nil {
       msg = cause.Error()
   }
   output := PlanOutput{
       Outcome:            PlanOutcomeFailed,
       ErrorCategory:      ErrorCategoryInvalidConfig,
       InvalidField:       field,
       InvalidValue:       extractInvalidValue(msg), // see helper below
       CurrentVersion:     currentVersion,
   }
   section, err := agentlib.MarshalSectionTyped(ctx, "## Plan", output)
   if err != nil {
       return nil, errors.Wrapf(ctx, err, "marshal ## Plan section (failed)")
   }
   md.ReplaceSection(section)
   glog.V(2).Infof("planning: invalid config: field=%s err=%v", field, cause)
   return &agentlib.Result{
       Status:    agentlib.AgentStatusFailed,
       NextPhase: string(domain.TaskPhaseHumanReview),
       Message:   "invalid .maintainer.yaml: " + field + ": " + msg,
   }, nil
   ```

   And the small helper:
   ```go
   // extractInvalidValue pulls the raw bad value out of the wrapped
   // parse error message so it lands verbatim in the task-page block.
   // The yaml.v3 error format is e.g.
   //   "yaml: unmarshal errors: line 2: cannot unmarshal !!str `yes` into bool"
   // We surface the offending token; on parse-format drift, fall back
   // to the full error string so the field is never blank.
   func extractInvalidValue(msg string) string {
       if i := strings.Index(msg, "`"); i >= 0 {
           if j := strings.Index(msg[i+1:], "`"); j >= 0 {
               return msg[i+1 : i+1+j]
           }
       }
       return msg
   }
   ```

   g. **Counterfeiter / mock setup is already done by prompt 1** (the `mocks.MaintainerConfigFetcher` is generated). The new tests below will use it.

3. **Update `factory.CreateAgent`.** In `agent/github-releaser/pkg/factory/factory.go`, the `CreateAgent` function builds the planning step. Change the call to pass the new fetcher:

   ```go
   planningRunner := CreateClaudeRunner(claudeConfigDir, agentDir, model, env, planningTools)
   fetcher := githubchangelog.NewHTTPFetcher(ghToken)
   maintainerConfigFetcher := maintainerconfig.NewHTTPFetcher(ghToken)
   planningStep := releaserpkg.NewPlanningStep(
       planningRunner,
       fetcher,
       maintainerConfigFetcher,
   )
   ```

   Add the import:
   ```go
   "github.com/bborbe/maintainer/agent/github-releaser/pkg/maintainerconfig"
   ```

   **Sibling entry-point check (do this BEFORE editing):**
   ```
   grep -rn 'NewPlanningStep\|factory\.CreateAgent' /workspace/agent/github-releaser/
   ```
   Three categories of call site to update:
   1. `agent/github-releaser/pkg/factory/factory.go` — `CreateAgent` (covered above).
   2. `agent/github-releaser/main.go` — calls `factory.CreateAgent(...)`; the factory signature is unchanged (it accepts `(claudeConfigDir, agentDir, model, ghToken, env)` — the new `maintainerConfigFetcher` is built INSIDE `CreateAgent` from `ghToken`, so this file needs no edits. Verify after editing factory.go.
   3. `agent/github-releaser/cmd/run-task/main.go` — same: calls `factory.CreateAgent(...)` with the same signature. Verify after editing factory.go.
   4. `agent/github-releaser/pkg/steps_planning_test.go` — every `pkg.NewPlanningStep(...)` call site must be updated to pass `&mocks.MaintainerConfigFetcher{}` as the third argument. The mock's default returns are zero/nil — the `Fetch` method returns `(nil, nil)`, which the planning step will interpret as "file present, empty bytes" (no 404, no error). By prompt 1's locked contract `Parse(ctx, []byte{}) → (zero, nil)`, this yields `changelogRewrite=false`. Add a small comment in each test fixture: `// maintainerConfigFetcher mock returns nil/nil by default — yields changelogRewrite=false via Parse(empty) contract.`

4. **Update the existing planning Ginkgo tests for the new constructor signature.** Every existing test in `agent/github-releaser/pkg/steps_planning_test.go` that calls `pkg.NewPlanningStep(runner, fetcher)` must be updated to `pkg.NewPlanningStep(runner, fetcher, &mocks.MaintainerConfigFetcher{})`. Find them via `grep -n "NewPlanningStep" /workspace/agent/github-releaser/pkg/steps_planning_test.go` and update each.

   The default `&mocks.MaintainerConfigFetcher{}` returns `(nil, nil)` from `Fetch`. By prompt 1's locked contract `Parse(ctx, []byte{}) → (zero MaintainerConfig, nil)`, this resolves to `changelogRewrite=false` → rewrite LLM call is SKIPPED.

   **This breaks the existing "rewrite decision" Context** because those tests expect TWO LLM calls (bump + rewrite). After this prompt, the default mock returns changelogRewrite=false, so only ONE LLM call is made.

   **Resolution:** update the existing "rewrite decision" Context tests to set up the mock to return a config that signals `changelogRewrite: true`. Add a helper at the top of the test file:

   ```go
   // withChangelogRewriteTrue returns a MaintainerConfigFetcher mock whose
   // Fetch returns a YAML byte slice with `release.changelogRewrite: true`.
   func withChangelogRewriteTrue() *mocks.MaintainerConfigFetcher {
       m := &mocks.MaintainerConfigFetcher{}
       m.FetchReturns(
           []byte("release:\n  changelogRewrite: true\n"),
           nil,
       )
       return m
   }
   ```

   Then every existing "rewrite decision" `It` case replaces `pkg.NewPlanningStep(runner, fetcher)` with `pkg.NewPlanningStep(runner, fetcher, withChangelogRewriteTrue())`. Cases that don't expect a rewrite call (P1 escalation, P2 escalation, missing frontmatter, fetch error, claude parse error, bad current_version, idempotency) can use the default `&mocks.MaintainerConfigFetcher{}` — the planning step never reaches the new fetcher on those paths.

   **Do NOT remove any of the existing five "rewrite decision" `It` cases** — spec 058 acceptance is still in force. Just rewire their mock setup.

5. **Add new planning Ginkgo coverage.** In `agent/github-releaser/pkg/steps_planning_test.go`, add a new `Context("changelogRewrite opt-in flag")` block (parallel to the existing `Context("rewrite decision")` block) with these `It` cases. Each one MUST assert on the parsed `## Plan` JSON via `agentlib.ExtractSection[pkg.PlanOutput]` plus the LLM call count.

   a. `It("flag absent (file 404) → rewrite_needed=false, LLM not called for rewrite, PlanOutput.ChangelogRewrite is *false")` — `maintainerConfigFetcher.FetchReturns(nil, maintainerconfig.ErrFileNotFound)`. Fetcher returns noisy `## Unreleased` body. Assert `fakeRunner.RunCallCount() == 1` (only the bump call), `plan.RewriteNeeded == false`, `plan.RewrittenUnreleased == ""`, `plan.ChangelogRewrite != nil && *plan.ChangelogRewrite == false`. **Also assert the marshaled `## Plan` JSON bytes contain the literal substring `"changelog_rewrite":false`** (capture the section bytes via `md.Section("## Plan").Body` or equivalent and assert with `Expect(string(planJSON)).To(ContainSubstring(`"changelog_rewrite":false`))`). This locks the JSON encoding contract from the `PlanOutput` doc-comment.

   b. `It("Fetch returns (nil, nil): empty bytes → Parse zero config → default false, rewrite LLM not called")` — `maintainerConfigFetcher.FetchReturns(nil, nil)`. This is the default-mock semantics path and is distinct from the 404 path in (a): the resolver does NOT hit the `ErrFileNotFound` branch but instead proceeds to `Parse(ctx, nil)` which (per prompt 1's locked contract) returns `(zero, nil)`. Assert `fakeRunner.RunCallCount() == 1`, `plan.RewriteNeeded == false`, `*plan.ChangelogRewrite == false`. Cross-references prompt 1 requirement 2.

   c. `It("flag absent (file present, no release: block) → rewrite_needed=false, rewrite LLM not called")` — `maintainerConfigFetcher.FetchReturns([]byte("prReviewer:\n  autoApprove: true\n"), nil)`. Assert `fakeRunner.RunCallCount() == 1`, `plan.RewriteNeeded == false`, `*plan.ChangelogRewrite == false`.

   d. `It("flag explicit false → rewrite_needed=false, rewrite LLM not called")` — `maintainerConfigFetcher.FetchReturns([]byte("release:\n  changelogRewrite: false\n"), nil)`. Assert `fakeRunner.RunCallCount() == 1`, `plan.RewriteNeeded == false`, `*plan.ChangelogRewrite == false`.

   e. `It("flag true + noisy Unreleased → rewrite_needed=true, rewrite LLM IS called")` — `maintainerConfigFetcher.FetchReturns([]byte("release:\n  changelogRewrite: true\n"), nil)`. Fetcher returns noisy `## Unreleased`. Mock `ClaudeRunnerMock.RunReturnsOnCall(0, …)` (bump = minor) and `RunReturnsOnCall(1, …)` (rewrite = `{"rewrite_needed": true, "rewritten_unreleased": "- feat: x", "reasoning": "y"}`). Assert `fakeRunner.RunCallCount() == 2`, `plan.RewriteNeeded == true`, `plan.RewrittenUnreleased != ""`, `*plan.ChangelogRewrite == true`. **Also assert the marshaled `## Plan` JSON contains the literal substring `"changelog_rewrite":true`.**

   f. `It("flag true + clean Unreleased → rewrite_needed=false (LLM judges clean), rewrite LLM IS called")` — `maintainerConfigFetcher.FetchReturns([]byte("release:\n  changelogRewrite: true\n"), nil)`. Fetcher returns clean `## Unreleased`. Mock rewrite verdict = `{"rewrite_needed": false, "rewritten_unreleased": "", "reasoning": "already clean"}`. Assert `fakeRunner.RunCallCount() == 2`, `plan.RewriteNeeded == false`, `plan.RewrittenUnreleased == ""`, `*plan.ChangelogRewrite == true`. Proves the spec's Non-goal: "do NOT short-circuit the rewrite-flow when the flag is true but ## Unreleased is already clean — planning still emits rewrite_needed=false (the LLM judges)."

   g. `It("network error on .maintainer.yaml fetch → treated as default false, NOT fail-closed")` — `maintainerConfigFetcher.FetchReturns(nil, stderrors.New("dial tcp: connection refused"))`. Assert no `## Error` block (the planning step succeeds), `fakeRunner.RunCallCount() == 1` (rewrite skipped), `*plan.ChangelogRewrite == false`. This is the "Any other fetch error → default false" branch of `resolveChangelogRewrite`.

   h. `It("invalid value: string \"yes\" → outcome=failed, error_category=invalid_config, human_review")` — `maintainerConfigFetcher.FetchReturns([]byte("release:\n  changelogRewrite: \"yes\"\n"), nil)`. Assert `result.Status == AgentStatusFailed`, `result.NextPhase == "human_review"`, `fakeRunner.RunCallCount() == 0` (NO LLM call, neither bump nor rewrite). Parse `## Plan` and assert `plan.Outcome == "failed"`, `plan.ErrorCategory == "invalid_config"`, `plan.InvalidField == "release.changelogRewrite"`, `plan.InvalidValue == "yes"`. **Also assert the marshaled `## Plan` JSON bytes do NOT contain the token `changelog_rewrite` at all** (verifies the nil pointer + `omitempty` path: on the failure outcome the field is omitted). Assert the Result Message contains the literal field token `release.changelogRewrite` and the bad value.

   i. `It("invalid value: number 1 → outcome=failed, error_category=invalid_config, human_review")` — same fixture as (h) but value `1`. Assert same outcomes; `plan.InvalidValue == "1"`.

   j. `It("task page audit trail: PlanOutput.ChangelogRewrite is the resolved value, not the file content")` — after the happy-path run (case e), assert the `## Plan` JSON's `changelog_rewrite` field is the boolean `true`, recorded as a JSON `true` (not the string `"true"`). This is the audit-trail invariant from spec AC #14.

   k. `It("flag-read-once: mutating the mock mid-run does not affect the in-flight planning step")` — use a counterfeiter `FetchStub` that returns a different config on the second call: `func(ctx, owner, repo, ref) ([]byte, error) { callCount++; if callCount == 1 { return []byte("release:\n  changelogRewrite: true\n"), nil } else { return []byte("release:\n  changelogRewrite: false\n"), nil } }`. After `Run`, assert the resolved `*plan.ChangelogRewrite == true` (the first call's value). This is the spec AC #18 "Flag-read-once semantics" test.

6. **Update `plan_output_test.go`.** In `agent/github-releaser/pkg/plan_output_test.go`, add ONE new `It` case in the existing `Describe("PlanOutput JSON contract", ...)` block:

   - `It("round-trips failed outcome with invalid_config details")` — `in := pkg.PlanOutput{Outcome: pkg.PlanOutcomeFailed, ErrorCategory: pkg.ErrorCategoryInvalidConfig, InvalidField: "release.changelogRewrite", InvalidValue: "yes", CurrentVersion: "v1.2.6"}`. Assert the JSON contains the literal substrings `"outcome":"failed"`, `"error_category":"invalid_config"`, `"invalid_field":"release.changelogRewrite"`, `"invalid_value":"yes"`, `"current_version":"v1.2.6"`. Also assert the token `changelog_rewrite` is NOT in the JSON (it's `omitempty` and stays empty on the failure path). Round-trip Unmarshal the bytes; assert the result equals `in`.

7. **Sibling-coverage check — only `steps_planning.go` consumes the maintainer config.** Run:
   ```
   grep -rn "MaintainerYaml\|maintainerconfig\|changelogRewrite\|ChangelogRewrite" /workspace/agent/github-releaser/pkg/steps_*.go
   ```
   Expected hits: `steps_planning.go` (the file being edited) and `steps_planning_test.go`. If `steps_execution.go` or `steps_ai_review.go` (or their test files) also reference these tokens, the new field must be plumbed there too — re-scope this prompt. If the grep returns ONLY `steps_planning*`, no sibling step files need editing.

   Then acceptance gate — `make precommit` exits 0 in `agent/github-releaser`. Run `cd /workspace/agent/github-releaser && make precommit` and confirm exit code 0. This is the full precommit (format + generate + test + lint + gosec + trivy); it MUST pass. Investigate and fix any failures. Counterfeiter regen MAY be needed if any interface in `pkg/maintainerconfig/` or `pkg/githubchangelog/` changed (the prompt 1 mock was already generated; this prompt does not add new counterfeiter directives).

   The `NewPlanningStep` signature change is consumed by the factory (covered) and by every test fixture in `steps_planning_test.go` (covered by requirement 4). After all edits, run `cd /workspace/agent/github-releaser && go build ./...` to prove every call site compiles.

8. **Changelog entry.** Add a single `## Unreleased` bullet to `/workspace/CHANGELOG.md` describing the spec-059 work. Follow the format and prefix style of the existing entries in that file (read `/home/node/.claude/plugins/marketplaces/coding/docs/changelog-guide.md` for the full style rules). Suggested entry:

   `- feat(agent/github-releaser): planning step now reads release.changelogRewrite from .maintainer.yaml at the target ref's tip via a new pkg/maintainerconfig fetcher; when false (default — file absent, field absent, or explicit false) the planning LLM is NOT invoked for the rewrite call and the resulting ## Plan carries rewrite_needed=false; when true the existing 058 rewrite pipeline runs unchanged. Non-boolean values for release.changelogRewrite fail closed at planning entry (outcome=failed, error_category=invalid_config on ## Plan, task ends in human_review, no commit/tag/push). The resolved flag value is recorded on ## Plan for audit. Adds Ginkgo coverage for all value cases plus the human_review fail-closed path and flag-read-once semantics.`

9. **Cross-prompt dependency declaration.** This prompt depends on prompt 1 having shipped first (the `ChangelogRewrite` field on `lib/maintainerconfig.ReleaseConfig`, the `pkg/maintainerconfig` package, the `mocks.MaintainerConfigFetcher` mock). If prompt 1 has not landed when this prompt is executed, the build will fail at the `go build ./...` step in requirement 7 with a "package github.com/bborbe/maintainer/agent/github-releaser/pkg/maintainerconfig: cannot find package" error — let the build failure surface and the daemon re-queue. Do not stub the package or define a placeholder. The dark-factory prompt ordering ensures prompt 1 runs first.
</requirements>

<constraints>
- The `changelogRewrite` flag is read ONCE per planning step invocation, at the cloned ref's tip (per spec 059 § Constraints). The single `s.maintainerConfig.Fetch` call inside `resolveChangelogRewrite` is the only read; do NOT re-fetch mid-run.
- The flag is per-repo only — do NOT add CLI flags, env vars, or frontmatter overrides. Spec 059 § Non-goals explicitly forbids all three.
- Fail-closed ONLY on parse / non-boolean errors from the lib parser. Transport / network errors (5xx, dial timeouts, 4xx other than 404) are treated as default-false (the spec's Failure Modes table lists these as "treated as `false`" cases).
- The fail-closed return shape is `Status=AgentStatusFailed, NextPhase=string(domain.TaskPhaseHumanReview)`. The framework's `agent_agent.go` exit-condition table treats this combination as a terminal human_review escalation. Do NOT use the existing `escalate` helper for this path — it returns `AgentStatusNeedsInput` and keeps the task `in_progress`, which is the wrong terminal state.
- The `changelog_rewrite` field on `## Plan` JSON is a `*bool` (pointer), NOT a `bool`. This is so the JSON distinguishes "resolved false" (`false` in JSON) from "not resolved" (field absent). The planning step MUST set the field on the happy path (both ready and rewrite-needed outcomes) so the audit trail is complete; happy-path JSON bytes MUST contain the literal substring `"changelog_rewrite":false` or `"changelog_rewrite":true`.
- On the failure path (outcome=failed), `ChangelogRewrite` stays nil and `omitempty` omits the token entirely from the JSON. The audit-trail surface is `ErrorCategory` + `InvalidField` + `InvalidValue` instead.
- The new `pkg/maintainerconfig.Fetcher` mock is generated by prompt 1's `go generate`. If the mock is missing at execution time, run `cd /workspace/agent/github-releaser && go generate ./...` first; the Makefile's `generate` target is the canonical entry point.
- The 3-phase task lifecycle (`planning → execution → ai_review`) and its `human_review` exit point are frozen — this prompt fills in a planning-time failure path that lands in `human_review`, consistent with that contract.
- The 058 rewrite pipeline (the second LLM call inside `runClassification`) is unchanged when the flag is true. The ONLY diff is a guard that skips the call when the flag is false.
- Do NOT add Prometheus metrics, debug logging, or other observability beyond the existing `glog.V(2).Infof` / `glog.Warningf` pattern.
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass (after the targeted updates in requirement 4).
</constraints>

<verification>
```
cd /workspace/agent/github-releaser && make precommit
```
Expected: exit code 0; all Ginkgo `It` cases listed in requirements 5 and 6 pass; the updated existing `It` cases in steps_planning_test.go still pass.

Evidence commands the auditor will run:
- `grep -n 'ChangelogRewrite\|ErrorCategory\|InvalidField\|InvalidValue' /workspace/agent/github-releaser/pkg/plan_output.go` → all four documented fields with JSON tags.
- `grep -n 'PlanOutcomeFailed\|ErrorCategoryInvalidConfig' /workspace/agent/github-releaser/pkg/plan_output.go` → both new constants present.
- `grep -rn 'NewPlanningStep' /workspace/agent/github-releaser/` → every call site now passes THREE arguments (runner, fetcher, maintainerConfigFetcher); signature matches the documented one.
- `grep -n 's.maintainerConfig.Fetch' /workspace/agent/github-releaser/pkg/steps_planning.go` → exactly ONE call site, inside `resolveChangelogRewrite`.
- `grep -n 's.runner.Run' /workspace/agent/github-releaser/pkg/steps_planning.go` → at most TWO call sites (bump + rewrite); the rewrite call must be guarded by `if changelogRewrite`.
- `grep -n 'AgentStatusFailed\|TaskPhaseHumanReview' /workspace/agent/github-releaser/pkg/steps_planning.go` → the new `failInvalidConfig` helper uses both.
- `grep -n 'withChangelogRewriteTrue\|mocks.MaintainerConfigFetcher{}' /workspace/agent/github-releaser/pkg/steps_planning_test.go` → existing "rewrite decision" tests rewire to use the opt-in helper; new "changelogRewrite opt-in flag" tests use the default mock.
- `grep -rn 'MaintainerYaml\|maintainerconfig\|changelogRewrite\|ChangelogRewrite' /workspace/agent/github-releaser/pkg/steps_*.go` → hits ONLY in `steps_planning.go` and `steps_planning_test.go`; no plumbing leaks into `steps_execution.go` or `steps_ai_review.go`.
- `ginkgo --v ./pkg | grep -E 'flag absent|Fetch returns \(nil, nil\)|flag explicit false|flag true \+|network error|invalid value|task page audit trail|flag-read-once'` → all required `It` descriptions appear and pass.
- `cat /workspace/CHANGELOG.md | head -25` → `## Unreleased` section contains the new spec-059 changelog entry.
</verification>
</output>
