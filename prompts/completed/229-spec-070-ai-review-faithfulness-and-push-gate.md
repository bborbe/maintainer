---
status: completed
spec: ["070"]
summary: Extended ai-review with semantic faithfulness LLM check, unexpected-file-change diff check, push gating, and workdir cleanup ownership; added 11 new Ginkgo specs; all tests + lint + gofmt + vet pass
container: maintainer-changelog-rewrite-exec-229-spec-058-ai-review-faithfulness-and-push-gate
dark-factory-version: v0.174.1-dirty
created: "2026-06-02T16:30:08Z"
queued: "2026-06-02T16:43:50Z"
started: "2026-06-02T18:10:24Z"
completed: "2026-06-02T18:37:27Z"
---

<summary>
- AI-review compares the original `## Unreleased` text (captured at planning time) against the final published body and decides per-entry whether the rewrite was faithful.
- AI-review fails the release if an original entry is silently dropped, if a hallucinated entry appears in the final body, or if the commit touched any file other than `CHANGELOG.md`.
- All three existing structural checks (tag exists, tag points at the expected commit, header was rewritten) keep running alongside the new semantic check.
- Push of the commit and tag to the remote now happens here — only after ai-review passes. On any ai-review failure, the local commit and local tag are preserved and the task ends in `human_review`.
- A run where the remote already has the same tag (concurrent release) lands in `human_review` without crashing and without overwriting the remote.
- If ai-review's LLM is unavailable the semantic verdict records `unknown` and the task ends in `human_review` (no push).
- Adds Ginkgo spec coverage for every faithfulness verdict, every structural check, push-gating, concurrency, crash-idempotence on re-fire, and the unexpected-file-change case.
</summary>

<objective>
Extend the ai-review step so it (a) performs a semantic faithfulness check comparing `Plan.OriginalUnreleased` against the final `## vX.Y.Z` body in the local workdir clone, (b) re-asserts the commit touched only `CHANGELOG.md` (plus detected plugin manifests), (c) preserves the existing three structural checks, (d) pushes the commit and tag to the remote IF AND ONLY IF every check passes, and (e) ends the task in `human_review` with the local commit and tag preserved on any failure. This is the final spec-058 slice.
</objective>

<context>
Read `~/Documents/workspaces/maintainer-changelog-rewrite/CLAUDE.md` and `agent/github-releaser/CLAUDE.md`.

Read these files BEFORE editing:
- `agent/github-releaser/pkg/steps_ai_review.go` — current ai-review step. Three structural checks today via `githubreview.Client`. You will EXTEND this step, not replace it.
- `agent/github-releaser/pkg/steps_ai_review_test.go` — existing Ginkgo style for ai-review.
- `agent/github-releaser/pkg/plan_output.go` — should already have `OriginalUnreleased`, `RewriteNeeded`, `RewrittenUnreleased` after prompt 1.
- `agent/github-releaser/pkg/result_output.go` — should already have `Workdir` and `LocalTag` after prompt 2.
- `agent/github-releaser/pkg/steps_execution.go` — read to confirm where the workdir survives execution (used here for push and for the local diff inspection).
- `agent/github-releaser/pkg/git/git.go` — `GitOps.Push` method exists; the ai-review step now becomes the new caller. You will also add `git.GitOps.CommittedFiles(ctx, workdir)` to the ai-review path (already exists; just call it).
- `agent/github-releaser/pkg/changelog/changelog.go` — pure helpers (`ExtractUnreleasedBody` from prompt 1 is now also usable for the body of `## vX.Y.Z` by passing the section heading).
- `agent/github-releaser/pkg/factory/factory.go` — `CreateAIReviewStep` wiring: `NewAIReviewStep(client githubreview.Client, ghToken string) agentlib.Step`. You will EXTEND the constructor signature to accept a `git.GitOps` and a `claudelib.ClaudeRunner` — every caller must be updated in this same prompt.
- `agent/github-releaser/main.go` and `agent/github-releaser/cmd/run-task/` — both `main.go` files build factory dependencies via `factory.Create*`. Use `grep -rn 'factory\.\(CreateAIReviewStep\|CreateAgent\)' agent/github-releaser/` at the start to enumerate all call sites and update each.

Read these coding plugin guides (in-container paths):
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-error-wrapping-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-testing-guide.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-factory-pattern.md`
- `/home/node/.claude/plugins/marketplaces/coding/docs/go-mocking-guide.md`

Verified symbols:
- `agentlib.Step.Run(ctx, *Markdown) (*Result, error)` and `agentlib.Result{Status, NextPhase, Message}`.
- `domain.TaskPhaseHumanReview = "human_review"` and `domain.TaskPhaseDone = "done"` from `github.com/bborbe/vault-cli@v0.67.5/pkg/domain/task_phase.go`.
- `claudelib.ClaudeRunner` interface: `Run(ctx, prompt string) (*ClaudeResult, error)`; `ClaudeResult{Result string}` from `github.com/bborbe/agent/lib@v0.63.11/claude`.
- `git.GitOps.Push(ctx, workdir string, refs ...string) error` and `git.GitOps.CommittedFiles(ctx, workdir string) ([]string, error)` from `pkg/git/git.go`.
- Existing `mocks.GitOps`, `mocks.ReviewClient`, `mocks.ClaudeRunnerMock` counterfeiter fakes (per directives in `pkg/git/git.go`, `pkg/githubreview/client.go`, `pkg/steps_mocks.go`).
- `agentlib.ExtractSection[T]` / `MarshalSectionTyped[T]`.
</context>

<requirements>

1. **Extend `ReviewOutput` and `ReviewChecks`** in `agent/github-releaser/pkg/steps_ai_review.go`:

   ```go
   // FaithfulnessVerdict captures the semantic comparison of one entry from
   // the original ## Unreleased against the final ## vX.Y.Z body.
   //   Verdict ∈ {"present", "silent-drop", "hallucinated"}
   //   Entry is the verbatim line being judged.
   //   Note is the LLM's one-sentence justification.
   type FaithfulnessVerdict struct {
       Entry   string `json:"entry"`
       Verdict string `json:"verdict"`
       Note    string `json:"note,omitempty"`
   }

   // Faithfulness verdict values — applied ONLY to per_entry verdicts.
   // The `unknown` state lives on Overall (see OverallUnknown below), NOT on
   // individual entries: per the spec's Failure Modes row "LLM unavailability",
   // `unknown` surfaces only at the overall level. When the LLM is
   // unreachable, PerEntry is left empty rather than filled with `unknown`
   // entries — single-purpose constants stay clearer this way.
   const (
       FaithfulnessPresent      = "present"
       FaithfulnessSilentDrop   = "silent-drop"
       FaithfulnessHallucinated = "hallucinated"
   )
   ```

   Add fields to existing `ReviewChecks` (keep the three existing booleans):
   ```go
   // FaithfulnessOK is true when every per-entry verdict is "present"
   // (no silent-drop, no hallucinated). False on any drift OR when the
   // overall verdict is "unknown".
   FaithfulnessOK bool `json:"faithfulness_ok"`

   // UnexpectedFileChange is true when the release commit touched a file
   // other than CHANGELOG.md (plus detected plugin manifests). It is the
   // ai-review-side mirror of the executeLocalRelease pre-commit guard.
   UnexpectedFileChange bool `json:"unexpected_file_change"`
   ```

   Add fields to existing `ReviewOutput`:
   ```go
   // PerEntry holds the per-entry semantic verdict produced by the
   // faithfulness LLM call. Empty when Overall == "unknown" or when the
   // execution step recorded failure (nothing to verify).
   PerEntry []FaithfulnessVerdict `json:"per_entry,omitempty"`

   // Overall is the rolled-up semantic verdict: "pass" | "fail" | "unknown".
   // "pass"   — every PerEntry is "present" AND no UnexpectedFileChange.
   // "fail"   — at least one PerEntry is "silent-drop" or "hallucinated",
   //            OR UnexpectedFileChange is true.
   // "unknown" — the LLM was unreachable; the rest of the review is still
   //            written (structural checks) but Approved is false and
   //            push is skipped.
   Overall string `json:"overall"`

   // UnexpectedFiles lists the file paths the commit touched that were
   // NOT in the expected set. Empty when UnexpectedFileChange is false.
   UnexpectedFiles []string `json:"unexpected_files,omitempty"`

   // FailedChecks names the structural and semantic checks that did not
   // pass. Stable strings — referenced by spec AC 15 assertions.
   // One or more of: "TagExists", "TagAtExpectedSHA",
   // "ChangelogHeaderRewritten", "Faithfulness", "UnexpectedFileChange".
   FailedChecks []string `json:"failed_checks,omitempty"`
   ```

   Overall values (constants):
   ```go
   const (
       OverallPass    = "pass"
       OverallFail    = "fail"
       OverallUnknown = "unknown"
   )
   ```

2. **New seam: faithfulness LLM.** Add a `claudelib.ClaudeRunner` field to `aiReviewStep`. Inject via the constructor (signature change called out in step 7). The runner is invoked exactly once per ai-review: it gets a prompt assembled from a new asset + the captured original + the final body.

3. **New prompt asset.** Create `agent/github-releaser/pkg/prompts/changelog_faithfulness.md`. Content shape:
   - Opening line: "You are auditing a CHANGELOG rewrite for semantic faithfulness."
   - Input layout (described in the prompt; the caller concatenates the actual values below at runtime):
     - `## Original ## Unreleased body` — the verbatim text captured at planning time.
     - `## Final ## vX.Y.Z body` — the body that landed in the commit.
   - Task: for every line in the original that describes a user-observable change (bullet entries; skip blank lines and pure comments), decide whether it appears in the final body. Two failure verdicts per entry: `silent-drop` (present in original, absent or unrecognizable in final) and `hallucinated` (present in final, not derivable from the original). The verdict `present` means the meaning is preserved even if the wording changed.
   - Output format: a single fenced ```json block, no prose outside. Schema:
     ```json
     {
       "per_entry": [
         { "entry": "<verbatim original line>", "verdict": "present|silent-drop", "note": "one sentence" }
       ],
       "extras": [
         { "entry": "<verbatim final line>", "verdict": "hallucinated", "note": "one sentence" }
       ],
       "overall": "pass|fail"
     }
     ```
     - `per_entry` carries one object per original entry. `extras` carries one object per final-body entry that the LLM judged hallucinated. `overall=pass` requires every `per_entry.verdict == "present"` AND `extras` empty.

4. **Embed + parse.** In `agent/github-releaser/pkg/prompts/prompts.go`:

   ```go
   //go:embed changelog_faithfulness.md
   var changelogFaithfulnessPrompt string

   func ChangelogFaithfulnessPrompt() string { return changelogFaithfulnessPrompt }

   type FaithfulnessLLMResponse struct {
       PerEntry []FaithfulnessEntry `json:"per_entry"`
       Extras   []FaithfulnessEntry `json:"extras"`
       Overall  string              `json:"overall"`
   }
   type FaithfulnessEntry struct {
       Entry   string `json:"entry"`
       Verdict string `json:"verdict"`
       Note    string `json:"note,omitempty"`
   }

   // ParseFaithfulnessResponse uses the same three-strategy extraction as
   // ParseBumpVerdict / ParseRewriteVerdict. Validates:
   //   - overall ∈ {"pass","fail"}
   //   - per_entry[i].verdict ∈ {"present","silent-drop"}
   //   - extras[i].verdict == "hallucinated"
   // Errors wrapped via github.com/bborbe/errors, containing "parse faithfulness response".
   func ParseFaithfulnessResponse(ctx context.Context, claudeOutput string) (FaithfulnessLLMResponse, error)
   ```

   Add Ginkgo tests in `pkg/prompts/prompts_test.go` covering plain JSON, fenced JSON, all-`present` → overall=pass, one `silent-drop` → overall=fail required, one `extras.hallucinated` → overall=fail required, bad verdict string errors, missing overall errors.

   **Mapping boundary test (REQUIRED).** Add `It("FaithfulnessLLMResponse → ReviewOutput.PerEntry mapping flattens extras as hallucinated")` in `pkg/steps_ai_review_test.go` (NOT `prompts_test.go` — this exercises the integration seam in `aiReviewStep.Run`, not the parser):
   - Construct a `FaithfulnessLLMResponse{PerEntry: [{entry:"- feat: X", verdict:"present"}, {entry:"- fix: Y", verdict:"silent-drop"}], Extras: [{entry:"- chore: Z", verdict:"hallucinated"}], Overall: "fail"}`.
   - Stub the runner to emit JSON matching that shape.
   - After `step.Run`, parse `## Review` and assert `output.PerEntry` has exactly THREE elements in this order: `{Entry:"- feat: X", Verdict:FaithfulnessPresent}`, `{Entry:"- fix: Y", Verdict:FaithfulnessSilentDrop}`, `{Entry:"- chore: Z", Verdict:FaithfulnessHallucinated}`. The third element proves the mapping flattens the `extras` LLM-output list into `per_entry` carrying `Verdict=hallucinated`.
   - Also assert `output.Overall == OverallFail` and `FailedChecks` contains `"Faithfulness"`.
   - This is the integration seam between the LLM JSON wire format and the `ReviewOutput` shape persisted to the task page — a shape-only test on `ReviewOutput` would not catch a regression where `extras` is dropped or mapped to the wrong verdict constant.

5. **Run logic in `aiReviewStep.Run`** (re-architect the existing flow):

   a. Extract `## Result` and `## Plan` BOTH (today only `## Result` is extracted). Add `agentlib.ExtractSection[PlanOutput](ctx, md, "## Plan")` after the existing `## Result` extract. If either is missing → wrapped error to the controller (same pattern as today).

   b. If `result.Outcome != ResultOutcomeReleased`: keep existing short-circuit (write `## Review` with all checks true, overall = `OverallPass`, return Done/NextPhase="done"). No push — execution recorded failure, there's nothing to push.

   c. **Structural checks first** (unchanged from today): `verifyTagExists` → `verifyTagAtExpectedCommit` → `verifyChangelogHeaderRewritten`. These three calls remain remote (GitHub REST) and continue to populate `checks.TagExists`, `checks.TagAtExpectedSHA`, `checks.ChangelogHeaderRewritten`. On any failure, record the named check in `FailedChecks` BUT DO NOT EARLY-RETURN — proceed to the semantic + diff checks so the human reviewer sees the full set of failures.

      Update the existing helpers to accumulate failed-check names into a `*[]string` instead of early-returning. Reuse the existing `githubreview.ErrTagNotFound` handling.

   d. **Unexpected-file-change check.** If `result.Workdir` is non-empty:
      - Call `s.ops.CommittedFiles(ctx, result.Workdir)`.
      - Compute the expected set: `expected := []string{"CHANGELOG.md"}` plus any detected plugin manifests. Re-use `plugin.DetectManifests(ctx, result.Workdir)` from `agent/github-releaser/pkg/plugin` (already imported in `steps_execution.go`).
      - If the committed files set differs from expected (use the existing `sameStringSet` helper — promote it from `steps_execution.go` to a small unexported file, or duplicate it; either is fine, document choice in code comment):
        - `checks.UnexpectedFileChange = true`
        - `output.UnexpectedFiles = <diff: committed - expected>`
        - Append `"UnexpectedFileChange"` to `FailedChecks`.
      - Errors from `CommittedFiles` (e.g. workdir missing because the agent restarted into a fresh container): wrap and return — controller retries.

   e. **Faithfulness LLM call.** Extract the body of the `## vX.Y.Z` section from `<result.Workdir>/CHANGELOG.md` using a new pure helper `changelog.ExtractSectionBody(ctx, content []byte, heading string) (string, error)` (generalize `ExtractUnreleasedBody` from prompt 1 — accept the heading text as a parameter; both can share an unexported core). `heading` here is `plan.NextVersionHeader` (e.g. `"## v1.2.8"`).

      Assemble the prompt:
      ```go
      prompt := prompts.ChangelogFaithfulnessPrompt() +
          "\n\n## Original ## Unreleased body\n\n" + plan.OriginalUnreleased +
          "\n\n## Final " + plan.NextVersionHeader + " body\n\n" + finalBody
      runResult, err := s.runner.Run(ctx, prompt)
      ```

      On LLM error: set `checks.FaithfulnessOK = false`, `output.Overall = OverallUnknown`, append `"Faithfulness"` to `FailedChecks`, and proceed (still write `## Review`, but Approved=false, no push). On parse error: same as LLM error — `unknown` verdict.

      On success: map `FaithfulnessLLMResponse` to `[]FaithfulnessVerdict` in `output.PerEntry` (include extras as entries with verdict `hallucinated`). If `response.Overall == "pass"`: `checks.FaithfulnessOK = true`. Otherwise: `checks.FaithfulnessOK = false` and append `"Faithfulness"` to `FailedChecks`.

   f. **Overall + Approved.**
      - `output.Overall = OverallPass` IFF every structural check is true AND `checks.FaithfulnessOK == true` AND `checks.UnexpectedFileChange == false` AND the runner did not return `unknown`.
      - `output.Approved = (output.Overall == OverallPass)`.
      - If `output.Overall == OverallUnknown`: `output.Approved = false`.

   g. **Push gating.**
      - When `output.Approved == true`: call `s.ops.Push(ctx, result.Workdir, "HEAD", "refs/tags/"+result.LocalTag)`. On Push error: write `## Review` with a note like `"push failed: <err>"`, set `Approved=false`, set the `workdirShouldCleanup` sentinel (per Req 6 (ii)), return `&agentlib.Result{Status: AgentStatusFailed, NextPhase: string(domain.TaskPhaseHumanReview), Message: ...}`. The deferred cleanup then removes the workdir AFTER the `## Review` has been marshaled — operator triage uses the task-page `## Review` section, not the on-disk clone. Use `git.ClassifyError(err)` only for the `Message` field's clarity; do NOT add a new error_category enum.
      - When `output.Approved == false`: write `## Review`, set the `workdirShouldCleanup` sentinel (per Req 6 (ii)), and return `&agentlib.Result{Status: AgentStatusFailed, NextPhase: string(domain.TaskPhaseHumanReview), Message: output.Notes}`. NO Push call. The deferred cleanup removes the workdir AFTER the `## Review` has been written — the task-page `## Review` is the operator's source of truth, not the on-disk clone.
      - When `output.Approved == true` AND Push succeeded: return `&agentlib.Result{Status: AgentStatusDone, NextPhase: "done"}` (unchanged from today's terminal-completion).

   h. **Notes field.** Populate `output.Notes` with a human-readable one-liner naming each failed check (or "all checks passed" on success, mirroring today).

6. **Workdir cleanup — ai-review is the lifetime owner on every terminal transition.** Execution (prompt 2) creates `result.Workdir` and deliberately leaves it on disk on success so ai-review can read it. ai-review owns the cleanup on BOTH terminal transitions:

   (i) **Approved + Push succeeded → Done**: remove the workdir (release is shipped, nothing left to inspect).
   (ii) **Any failure path → human_review**: remove the workdir ONLY AFTER the `## Review` section has been written to the task page. The operator inspects the FAILED CHECKS and Notes from the task page, not the on-disk clone — the local commit and tag are not recoverable from the workdir after `human_review` anyway because the commit was never pushed and the tag was never pushed.

   Implementation — single defer keyed on a sentinel:

   ```go
   var workdirShouldCleanup bool
   defer func() {
       if workdirShouldCleanup && result.Workdir != "" {
           if err := os.RemoveAll(result.Workdir); err != nil {
               glog.Warningf("ai_review: workdir cleanup failed: %v", err)
           }
       }
   }()
   ```

   Set `workdirShouldCleanup = true` at exactly two points:
   - Immediately after the successful `s.ops.Push` call on the Approved branch (before constructing the Done return).
   - Immediately after the `## Review` section has been marshaled and appended to the markdown on the human_review branch (before constructing the Failed/human_review return).

   This makes ai-review the sole lifetime owner of `result.Workdir` once execution returns it — matching the cross-reference declared in prompt 2 Req 5 ("Workdir lifetime ownership").

7. **Constructor signature change.** Change `NewAIReviewStep`:

   ```go
   // Was: func NewAIReviewStep(client githubreview.Client, ghToken string) agentlib.Step
   // Now:
   func NewAIReviewStep(
       client githubreview.Client,
       runner claudelib.ClaudeRunner,
       ops git.GitOps,
       ghToken string,
   ) agentlib.Step
   ```

   Update `aiReviewStep` struct accordingly.

   **Sibling entry-point check (do this BEFORE editing):**
   ```
   grep -rn 'NewAIReviewStep\|factory\.CreateAgent' agent/github-releaser/
   ```
   Three call sites MUST be updated — all three of them DO call `factory.CreateAgent`:

   1. `agent/github-releaser/pkg/factory/factory.go` (`CreateAgent` calls `NewAIReviewStep` — see `factory.go:117`). Inside `CreateAgent`, build a Claude runner for the ai-review phase (mirror the existing planning runner; use `planningTools` or a new empty-tool-set constant if you prefer — the ai-review LLM is read-only) and pass the existing `git.GitOps` (the same `CreateGitOps()` already wired for execution).
   2. `agent/github-releaser/main.go` — the long-running agent entry point. Calls `factory.CreateAgent(...)`; pass any new arguments the updated signature requires.
   3. `agent/github-releaser/cmd/run-task/main.go` — the one-shot CLI entry point. Calls `factory.CreateAgent(...)`; update identically to (2).

   Both `main.go` files DO wire `factory.CreateAgent` — this is not an "if they call it directly" — they DO. Enumerate them as concrete edit targets in the same diff; do not defer. After edits, run `go build ./...` in `agent/github-releaser/` to prove every call site compiles.

   Update the existing ai-review tests in `pkg/steps_ai_review_test.go` constructor calls to pass the new dependencies (use `&mocks.ClaudeRunnerMock{}` and `&mocks.GitOps{}` from the existing mock packages).

8. **Faithful-rewrite happy-path Ginkgo spec.** In `pkg/steps_ai_review_test.go`, add an `It("faithful rewrite → overall=pass, push happens, NextPhase=done")`:
   - Build a `taskMD` whose `## Plan` has `OriginalUnreleased = "- feat: add foo\n- fix: bar\n"`, `RewriteNeeded = false`, `NextVersionHeader = "## v1.2.8"`.
   - `## Result` has `Outcome = "released"`, `Workdir = <tmpdir>`, `LocalTag = "v1.2.8"`, `Tag = "v1.2.8"`, `CommitSHA = "abc1234"`.
   - Seed the tmpdir with a `CHANGELOG.md` whose `## v1.2.8` body equals `- feat: add foo\n- fix: bar\n` (faithful).
   - Mock `githubreview.Client`: TagExists returns a SHA, ResolveTagCommit returns `abc1234...`, FetchChangelog returns content where the top heading is `## v1.2.8`.
   - Mock `mocks.GitOps`: `CommittedFilesReturns([]string{"CHANGELOG.md"}, nil)`, `PushReturns(nil)`.
   - Mock `mocks.ClaudeRunnerMock.RunReturns(&claudelib.ClaudeResult{Result: <faithful JSON with per_entry all present, overall=pass>}, nil)`.
   - Assert: `result.Status == AgentStatusDone`, `result.NextPhase == "done"`, `fakeOps.PushCallCount() == 1`, parsed `## Review` has `Approved == true`, `Overall == "pass"`, `FailedChecks` empty.

9. **Faithfulness fail Ginkgo specs.** Add three `It` cases, each asserting `fakeOps.PushCallCount() == 0`, `result.NextPhase == "human_review"`, parsed `## Review` has `Approved == false` and specific contents:

   a. `It("silent-drop → overall=fail, failed_checks contains Faithfulness, no push, ## Review captured")` — faithfulness LLM returns `per_entry` with one `silent-drop`. Assert `output.PerEntry` contains that entry with `Verdict == "silent-drop"`. Assert `result.NextPhase == "human_review"`, `fakeOps.PushCallCount() == 0`, parsed `## Review.Notes` names the failed Faithfulness check. The workdir is cleaned up by ai-review on the human_review exit (Req 6 (ii)), so do NOT assert `os.Stat(result.Workdir); err == nil` post-return — instead seed the workdir with a real tmpdir AND register `DeferCleanup(func() { _ = os.RemoveAll(workdir) })` immediately after creating it (idempotent if ai-review already removed it).

   b. `It("hallucinated → overall=fail, per_entry contains hallucinated entry")` — faithfulness LLM returns `extras` with one entry, `overall=fail`. Assert `output.PerEntry` contains an entry with `Verdict == "hallucinated"`.

   c. `It("unexpected file change → overall=fail, unexpected_files lists the offending file")` — `mocks.GitOps.CommittedFilesReturns([]string{"CHANGELOG.md", "secrets.env"}, nil)`. Faithfulness LLM is configured to return overall=pass (so we are isolating the diff-check failure). Assert `output.UnexpectedFileChange == true`, `output.UnexpectedFiles` contains `"secrets.env"`, `FailedChecks` contains `"UnexpectedFileChange"`, `fakeOps.PushCallCount() == 0`.

10. **Structural-check regression Ginkgo specs.** Three `It` cases under a `Context("structural checks toggle independently")`:

    a. `It("TagExists fails → approved=false, FailedChecks contains TagExists")` — mock `ReviewClient.TagExistsReturns("", githubreview.ErrTagNotFound)`. Other checks (faithfulness, diff) succeed. Assert `checks.TagExists == false`, `FailedChecks` contains `"TagExists"`, `Approved == false`, no Push.

    b. `It("TagAtExpectedSHA fails → approved=false, FailedChecks contains TagAtExpectedSHA")` — mock `ResolveTagCommit` returns a different SHA than `result.CommitSHA`. Other checks succeed. Assert `checks.TagAtExpectedSHA == false`, `FailedChecks` contains `"TagAtExpectedSHA"`, `Approved == false`, no Push.

    c. `It("ChangelogHeaderRewritten fails → approved=false, FailedChecks contains ChangelogHeaderRewritten")` — mock `FetchChangelog` returns content whose top heading is still `## Unreleased`. Other checks succeed. Assert `checks.ChangelogHeaderRewritten == false`, `FailedChecks` contains `"ChangelogHeaderRewritten"`, `Approved == false`, no Push.

11. **LLM unavailable Ginkgo spec.** `It("ai-review LLM error → overall=unknown, approved=false, no push")` — mock `ClaudeRunnerMock.RunReturns(nil, errors.New("dial tcp: connection refused"))`. Structural checks all pass; diff check passes. Assert: `output.Overall == "unknown"`, `FailedChecks` contains `"Faithfulness"`, `Approved == false`, no Push, `NextPhase == "human_review"`.

12. **Push-failure Ginkgo spec.** `It("push fails after approval → overall=pass but task ends in human_review with push-failed note")` — all checks pass, then `mocks.GitOps.PushReturns(errors.New("dial tcp: rate limited"))`. Assert: `fakeOps.PushCallCount() == 1`, parsed `## Review.Notes` contains the literal `"push failed"`, `result.Status == AgentStatusFailed`, `result.NextPhase == "human_review"`. Workdir cleanup follows Req 6 (ii) — register `DeferCleanup` against the seeded tmpdir; do not assert the directory survives post-return.

13. **Concurrency Ginkgo spec.** `It("concurrent push (tag already exists on upstream) → human_review, push-failed note recorded")` — all checks pass, then `mocks.GitOps.PushReturns(errors.New("! [rejected] refs/tags/v1.2.8 -> refs/tags/v1.2.8 (already exists)"))`. Assert: `fakeOps.PushCallCount() == 1`, `## Review.Notes` contains `"push failed"`, `result.NextPhase == "human_review"`. The workdir is cleaned by ai-review on the human_review exit (Req 6 (ii)) — register `DeferCleanup` against the seeded tmpdir as in 9(a) rather than asserting the directory survives.

14. **Crash-idempotence Ginkgo spec at the execution layer.** Already covered by prompt 2's idempotence test for the execution step. Verify that prompt 2's `It("re-fire produces no duplicate commit and no duplicate tag")` is present and passes; if not, add it here (cross-prompt safety net).

15. **Acceptance gate — `make precommit` exits 0 in `agent/github-releaser`.** Investigate and fix any failures. Counterfeiter regen MAY be needed if any interface in `pkg/githubreview/` or `pkg/git/` changed — they should NOT change in this prompt. The `NewAIReviewStep` signature change is consumed by `factory.CreateAgent` only (verify with grep first).

</requirements>

<constraints>
- The pre-push diff guard in `executeLocalRelease` (from prompt 2) stays as the belt — ai-review's `UnexpectedFileChange` is the braces. Two independent enforcements; do not merge them.
- The 3-phase task lifecycle (`planning → execution → ai_review`) and its `human_review` exit point are frozen — this prompt fills in the contents of ai_review only.
- The `## Plan` block's `OriginalUnreleased` field is the single source of truth for faithfulness — never re-fetch the original from the repo at ai-review time. (Defense against repo-modification-between-planning-and-review attacks.)
- AI-review fail MUST default to "no push" — not "log and continue". This is the last gate before the world sees the release.
- Do NOT add an `ErrorCategoryChangelogQuality` enum entry — failure uses the existing `human_review` phase exit.
- Do NOT change the structural check NAMES (`TagExists`, `TagAtExpectedSHA`, `ChangelogHeaderRewritten`) — they are referenced verbatim in the spec AC 15 assertions and become `FailedChecks` entries.
- Do NOT add a per-repo bypass switch.
- Do NOT extend changelog-quality enforcement to PR-level checks — that is pr-reviewer's scope.
- Do NOT change the single-CHANGELOG assumption (no mono-repo handling).
- Do NOT commit — dark-factory handles git.
- Existing tests must still pass (after the targeted updates listed above).
</constraints>

<verification>
```
cd ~/Documents/workspaces/maintainer-changelog-rewrite/agent/github-releaser
make precommit
```

Expected: exit code 0; every Ginkgo `It` listed in requirements 8-13 passes.

Evidence commands the auditor will run:
- `grep -n 'NewAIReviewStep' agent/github-releaser/` (recursively) → every call site now passes `runner` and `ops` arguments; signature matches the documented one.
- `grep -n 'FaithfulnessOK\|UnexpectedFileChange\|Overall\|FailedChecks' agent/github-releaser/pkg/steps_ai_review.go` → all four documented fields written into `ReviewOutput`/`ReviewChecks`.
- `grep -n 's.ops.Push' agent/github-releaser/pkg/steps_ai_review.go` → exactly ONE Push call site, on the Approved=true branch.
- `grep -n 's.ops.Push\|ops.Push' agent/github-releaser/pkg/steps_execution.go` → ZERO matches (confirms prompt 2's removal stuck).
- `ginkgo --v ./pkg | grep -E 'faithful rewrite|silent-drop|hallucinated|unexpected file change|TagExists fails|TagAtExpectedSHA fails|ChangelogHeaderRewritten fails|LLM error|push fails|concurrent push'` → all required `It` descriptions appear and pass.
</verification>
