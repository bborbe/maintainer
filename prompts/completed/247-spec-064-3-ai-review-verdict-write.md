---
status: completed
spec: [064-github-releaser-post-check-verdict-from-github-truth]
summary: 'Added review-warning override on the ai_review !approved path: a rejected review that coincides with a confirmed remote tag now writes a ## Review Warning block and closes the task as completed (preserving the rejected ## Review verdict durably), while the existing human_review path stands when the remote is empty or LsRemote errors. Extracted url_helpers to package level, added 6 Ginkgo specs, updated CHANGELOG, and make precommit exits 0.'
container: maintainer-release-postcheck-exec-247-spec-064-3-ai-review-verdict-write
dark-factory-version: v0.175.0
created: "2026-06-08T16:30:00Z"
queued: "2026-06-08T18:48:42Z"
started: "2026-06-09T06:54:12Z"
completed: "2026-06-09T07:13:34Z"
branch: dark-factory/github-releaser-post-check-verdict-from-github-truth
---

<summary>
- The github-releaser ai_review step gains a small, targeted adjustment: a review rejection no longer flips the task status to `failed` when the planned-version tag already exists on the remote at the SHA the agent just produced
- The review verdict is preserved as a recorded warning in a new `## Review Warning` section so the operator audit trail is durably visible on the task body — the section names the failed checks, the planned version, the observed remote SHA, and a one-line note explaining the override
- The task closes as `completed` (frontmatter rewritten to `status: completed`, `phase: done`) when the remote confirms the tag at the agent's expected SHA — even though `output.Approved` is `false`
- When the remote shows the tag at a different SHA (a later release won the slot), the same `## Review Warning` block is written and the task also closes as `completed` (verdict effectively `superseded`)
- When the remote shows no tag, or the remote query errors, the existing `human_review` path stands unchanged — the post-check's `released` / `superseded` verdict in prompt 2 is the agent-side release-confirmation path, the ai_review-side `Review Warning` block is a different signal: "the review rejected, but the remote confirms the release happened"
- The factory wiring is unchanged — `NewAIReviewStep(client, runner, ops, ghToken)` still takes the same `git.GitOps` interface from prompt 1 (the new `LsRemote` method is part of the same interface)
- The ai_review step's external signature is unchanged. The new behavior is internal: a small, separable code path on the `!approved` branch that calls `s.ops.LsRemote` before deciding between `human_review` (existing) and `completed` + `## Review Warning` (new)
- New Ginkgo integration tests in `pkg/steps_ai_review_test.go` cover: (a) `!approved` + remote SHA matches expected → `## Review Warning` block on disk, `## Review` block on disk, `result.Status == AgentStatusDone`, frontmatter `status: completed` / `phase: done`; (b) `!approved` + remote SHA differs from expected → same as (a) but the warning cites the observed remote SHA explicitly; (c) `!approved` + remote empty → existing `human_review` path stands, NO `## Review Warning` block; (d) `!approved` + LsRemote errors → existing `human_review` path stands (redacted error logged, but the verdict-downgrade does not happen); (e) `approved=true` (happy path) is unchanged — the new branch is a no-op for the happy path; (f) when the result is `failed` (not `released`), the new branch is a no-op — `result.Outcome != ResultOutcomeReleased` short-circuits to `writeShortCircuit` and the LsRemote check never fires
- The repo-root `CHANGELOG.md` is updated under `## Unreleased` with a `feat(agent/github-releaser):` entry describing the review-warning-on-confirmed-release behavior. This entry sits alongside (not in place of) the post-check entry from prompt 2.
- All existing `agent/github-releaser/pkg/...` Ginkgo tests continue to pass — including the existing ai_review tests that exercise `Approved=false` paths.

</summary>

<objective>
Adjust the github-releaser ai_review step so a review rejection that follows a confirmed remote tag at the agent's expected SHA does not flip the task status to `failed`. The review verdict is preserved as a recorded warning in a new `## Review Warning` section, and the task closes as `completed` instead of `human_review`. The adjustment is small, targeted, and additive: the existing happy path and the existing `human_review` path are unchanged; only the `!approved` branch gains a sub-decision that consults `LsRemote`.

</objective>

<context>
Read `/workspace/agent/github-releaser/pkg/steps_ai_review.go` end-to-end. The `aiReviewStep` struct at line 143 has four fields: `client`, `runner`, `ops git.GitOps`, `ghToken`. The `Run` method at line 178 is the entry point. The success path on `!approved` is `s.finishHumanReview` at line 251 (which writes `## Review` and returns `Status=Failed` / `NextPhase=human_review`). The success path on `approved` is `s.finishApproved` at line 271 (which pushes and returns `Status=Done` / `NextPhase=done`).

The `result *ResultOutput` is extracted at line 179 from `## Result`. It carries:
- `result.Outcome` (`released` or `failed`) — line 188 short-circuits `!=released` to `writeShortCircuit` which approves unconditionally.
- `result.CommitSHA` (the agent's expected SHA, populated only on `released`).
- `result.LocalTag` (the local tag name, populated only on `released`).
- `result.Workdir` (the local clone, may be empty on failure path).

The new branch fires when:
- `result.Outcome == "released"` (so we're past the short-circuit at line 188).
- `output.Approved == false` (so we're on the `!approved` branch at line 250).
- The remote `LsRemote(authedURL, ref, result.LocalTag)` returns a SHA equal to `result.CommitSHA` → tag verified at the agent's expected SHA. OR returns a non-empty SHA different from `result.CommitSHA` → tag exists at a different SHA (later release won the slot).

The auth model: `aiReviewStep.ghToken` is the token; build the authed URL the same way the execution step does (mirror `s.injectToken(normalizeCloneURLToHTTPS(cloneURL))` if you can — but `aiReviewStep` does NOT have a private `injectToken` or `normalizeCloneURLToHTTPS` (those are unexported methods on `executionStep`). The cleanest shape: add the helpers as unexported package-level functions in a new file (e.g. `pkg/url_helpers.go`) OR duplicate the body in the ai_review step. The executor picks the cleanest local shape that compiles — preference is a single source of truth (extract to package-level helpers), duplication is acceptable if the executor judges the refactor out of scope for this prompt.

The `cloneURL` is in `md.Frontmatter["clone_url"]` (read it via `md.Frontmatter.String("clone_url")`). The `ref` is in `md.Frontmatter["ref"]` (read it via `md.Frontmatter.String("ref")`).

The `## Review Warning` block is a NEW section in the task body. It sits alongside `## Review` (which ai_review already writes). The new section's typed contract is described in step 1 below. Use the existing `agentlib.MarshalSectionTyped` + `md.ReplaceSection` pattern (already used by `steps_execution.go` for `## Result` and `## Resolution`).

Read `/workspace/agent/github-releaser/pkg/steps_ai_review_test.go` to see the existing test scaffolding:
- `BeforeEach` (line 57) wires `fakeClient`, `fakeRunner`, `fakeOps`, `token` and constructs the step via `pkg.NewAIReviewStep(fakeClient, fakeRunner, fakeOps, token)`.
- `taskWithResult(commitSHA, tag, outcome, workdir)` (line 73) builds a task markdown with `## Plan` + `## Result` sections.
- `runStep(taskMarkdown)` is a closure (search for it in the file — it's defined later) that calls `step.Run(ctx, md)` and returns the result + the parsed `*agentlib.Markdown`.
- `extractReview(md)` (line 227) extracts the typed `pkg.ReviewOutput` from the `## Review` section.

The new tests follow the same scaffolding. The counterfeiter mock `mocks.GitOps` (regenerated in prompt 1) provides `LsRemoteStub`, `LsRemoteReturns(sha, err)`, `LsRemoteCallCount()`, `LsRemoteArgsForCall(i)`. Use these.

Read `/workspace/agent/github-releaser/pkg/factory/factory.go` — `NewAIReviewStep(client, runner, ops, ghToken)` is the constructor (line 129 of `steps_ai_review.go`, called from line 136 of factory.go). The factory does NOT need to change — the new `LsRemote` method is part of the same `git.GitOps` interface, and the ai_review step's signature stays the same.

The repo-root `/workspace/CHANGELOG.md` uses the `feat:` prefix style. The new entry for this prompt belongs under `## Unreleased` (alongside, not replacing, the post-check entry from prompt 2 and the LsRemote seam entry from prompt 1).

</context>

<requirements>

1. **Add a `ReviewWarningOutput` typed struct** in a new file `agent/github-releaser/pkg/review_warning_output.go` (new file, package `pkg`). Round-trips with `agentlib.MarshalSectionTyped` + `agentlib.ExtractSection`. Shape:

   ```go
   // ReviewWarningOutput is the typed contract for the `## Review Warning`
   // JSON section the ai_review step writes when a review rejection
   // coincides with a confirmed remote tag at the agent's expected SHA
   // (or at a different SHA, indicating a later release won the slot).
   // The section preserves the review verdict durably on the task body
   // for the operator audit trail, while the task closes as `completed`
   // (the post-check's `released` / `superseded` verdict).
   //
   // Fields:
   //   - FailedChecks:    the names of the review checks that failed
   //                      (from ReviewOutput.FailedChecks, e.g. "Faithfulness")
   //   - PlannedVersion:  the version the agent was attempting to release
   //                      (e.g. "v1.2.8")
   //   - ObservedRemoteSHA: the SHA the remote shows for the planned tag
   //   - Note:            a one-line human-readable summary
   type ReviewWarningOutput struct {
       FailedChecks       []string `json:"failed_checks"`
       PlannedVersion     string   `json:"planned_version"`
       ObservedRemoteSHA  string   `json:"observed_remote_sha"`
       Note               string   `json:"note,omitempty"`
   }
   ```

2. **Extract the URL helpers to package-level unexported functions** so `aiReviewStep` and `executionStep` share a single source of truth. The `injectToken` + `normalizeCloneURLToHTTPS` helpers are currently unexported methods on `*executionStep` (steps_execution.go:363 and :382). Create a new file `agent/github-releaser/pkg/url_helpers.go` and move the bodies of `normalizeCloneURLToHTTPS` (line 363-377) and `executionStep.injectToken` (line 382-391) into two package-level functions: `normalizeCloneURLToHTTPS(raw string) string` and `injectToken(cloneURL, ghToken string) string`. Update `*executionStep` to call the package-level functions (preserves the existing behavior — verify by `make test` exit 0 in `agent/github-releaser/`). `aiReviewStep` then calls the same package-level functions.

3. **Add the new branch on the `!approved` path** in `aiReviewStep.Run`. After the existing `if !output.Approved { return s.finishHumanReview(...) }` at line 250, INSERT a new sub-decision: BEFORE calling `finishHumanReview`, consult the remote via `LsRemote`. The new code shape (the executor must follow this pattern):

   ```go
   if !output.Approved {
       // Spec 064 DB #7: a review rejection that coincides with a
       // confirmed remote tag at the agent's expected SHA does not
       // flip the task to `failed`. The review verdict is preserved
       // as a recorded warning in ## Review Warning, and the task
       // closes as `completed` (the post-check from prompt 2 also
       // upgrades the verdict — this branch is its ai_review-side
       // mirror for the case where the execution-step post-check
       // did NOT fire because the remote was empty at the time of
       // execution, but is now non-empty by the time ai_review runs).
       if warning := s.checkReviewOverride(ctx, md, &output, result); warning != nil {
           return s.finishReviewOverride(ctx, md, output, warning, &workdirShouldCleanup)
       }
       return s.finishHumanReview(ctx, md, output, &workdirShouldCleanup)
   }
   ```

   Add two new methods on `*aiReviewStep`:

   - `checkReviewOverride(ctx, md, output, result) *ReviewWarningOutput` — calls `s.ops.LsRemote(ctx, authedURL, ref, result.LocalTag)`. Returns `nil` (no override) on empty result or error. Returns a populated `*ReviewWarningOutput` on a non-empty remote SHA. The `Note` is constructed from `output.FailedChecks` + the observed SHA: `fmt.Sprintf("review rejected (%s) but remote confirms release at %s", strings.Join(output.FailedChecks, ","), observedSHA)`.

   - `finishReviewOverride(ctx, md, output, warning, workdirShouldCleanup) (*agentlib.Result, error)` — writes BOTH the existing `## Review` section (the rejected verdict) AND the new `## Review Warning` block. Sets `md.Frontmatter["status"] = "completed"` and `md.Frontmatter["phase"] = "done"`. Returns `&agentlib.Result{Status: agentlib.AgentStatusDone, NextPhase: "done"}`. Sets `*workdirShouldCleanup = true` (the workdir is removed on the terminal transition, same as the existing happy path).

   The new code path must run BEFORE `s.finishHumanReview` so the workdir cleanup defer is set correctly. Mirror the existing `finishApproved` shape at line 271-301.

4. **Authed URL construction in `checkReviewOverride`**. Read `cloneURL` from `md.Frontmatter.String("clone_url")` and `ref` from `md.Frontmatter.String("ref")`. If either is empty, skip the new branch (return `nil`, fall through to `human_review`). Build the authed URL via the package-level helpers from step 2. On any error from `LsRemote`, log via `glog.V(2)` with `redactToken` applied to the err message (mirror the post-check's redaction shape from prompt 2), then return `nil` to fall through to `human_review`.

5. **Verify the existing happy path is unchanged**. The `if output.Approved { return s.finishApproved(...) }` branch at line 253 must continue to behave exactly as today. The new `if !output.Approved { ... }` block at line 250 inserts the new sub-decision; the existing `finishHumanReview` call is still reachable (the new branch is a sub-decision inside the `!approved` path, not a replacement for `finishHumanReview`).

6. **Verify the `result.Outcome != "released"` short-circuit is unchanged**. The `if result.Outcome != ResultOutcomeReleased { return s.writeShortCircuit(...) }` block at line 188-190 must continue to behave exactly as today. The new `checkReviewOverride` branch fires only AFTER this short-circuit (i.e., only when `result.Outcome == "released"`). On the failure-result path the new code is unreachable.

7. **Update the `Run` doc comment** at line 160-177 of `steps_ai_review.go` to mention the new branch (one extra bullet: "On `!approved`: consult `LsRemote`. If the remote shows the tag at the agent's expected SHA (or at any SHA, for the superseded case), write `## Review Warning` + close as `completed`. Otherwise, the existing `human_review` path stands.").

8. **Add new Ginkgo integration tests** in `agent/github-releaser/pkg/steps_ai_review_test.go` under a new `Context("review-warning override (spec 064)", func() { ... })` block. Cover, at minimum:

   - **Remote SHA matches expected → completed + ## Review Warning**: drive the step to a `!approved` outcome (e.g. faithfulness LLM returns a `silent-drop` verdict). Set `fakeOps.LsRemoteReturns(result.CommitSHA, nil)` so the remote shows the tag at the expected SHA. After `Run`, assert: `result.Status == AgentStatusDone`, `result.NextPhase == "done"`, `md.Frontmatter["status"] == "completed"`, `md.Frontmatter["phase"] == "done"`, the `## Review` section is on disk with `Approved=false` (the rejected verdict is preserved), the `## Review Warning` section is on disk with `FailedChecks` containing `Faithfulness` (or whatever the LLM returned), `PlannedVersion` matching `result.LocalTag`, `ObservedRemoteSHA` matching the faked return.

   - **Remote SHA differs from expected → completed + ## Review Warning (superseded mirror)**: same setup, but `fakeOps.LsRemoteReturns("deadbee", nil)` where `"deadbee"` differs from `result.CommitSHA`. After `Run`, assert: same as above, except `ObservedRemoteSHA == "deadbee"`.

   - **Remote empty → existing human_review path stands**: same setup, but `fakeOps.LsRemoteReturns("", nil)`. After `Run`, assert: `result.Status == AgentStatusFailed`, `result.NextPhase == "human_review"`, NO `## Review Warning` block on disk, `fakeOps.LsRemoteCallCount() == 1` (the new code path was attempted, but returned nil on the empty result).

   - **LsRemote errors → existing human_review path stands**: same setup, but `fakeOps.LsRemoteReturns("", errors.New("ls-remote boom"))`. After `Run`, assert: same as the empty case (the error is logged but the verdict-downgrade does not happen).

   - **Happy path (Approved=true) is unchanged**: drive the step to an `Approved=true` outcome (faithfulness LLM returns a pass verdict). After `Run`, assert: `result.Status == AgentStatusDone`, `result.NextPhase == "done"`, `fakeOps.LsRemoteCallCount() == 0` (the new branch is unreachable on the happy path), no `## Review Warning` block.

   - **Short-circuit path (Result.Outcome == "failed") is unchanged**: parse a task with `## Result(outcome=failed)`. After `Run`, assert: `result.Status == AgentStatusDone` (the short-circuit approves), `fakeOps.LsRemoteCallCount() == 0` (the new branch is unreachable on the failure-result path).

   Use the existing test scaffolding (`fakeClient`, `fakeRunner`, `fakeOps`, `taskWithResult`, `runStep`, `extractReview`). For the `## Review Warning` extraction, **define a sibling helper `extractReviewWarning` alongside `extractReview` (after line 227) so the round-trip pattern stays consistent**: `extractReviewWarning := func(md *agentlib.Markdown) *pkg.ReviewWarningOutput { sec, ok := agentlib.ExtractSection[pkg.ReviewWarningOutput](ctx, md, "## Review Warning"); if !ok { return nil }; return sec }`. Tests assert override-case fields via `extractReviewWarning(md).ObservedRemoteSHA == fakeReturnedSHA` etc.

9. **Verify the `result.Workdir != ""` short-circuit in the existing `checkUnexpectedFileChange` does not interact with the new branch**. The new `checkReviewOverride` does not touch the workdir — it consults the remote via `LsRemote` and writes two sections to the markdown. The workdir cleanup defer at line 212-260 still runs at `Run`'s return, governed by the `workdirShouldCleanup` sentinel. The new `finishReviewOverride` sets `*workdirShouldCleanup = true` (terminal transition), so the cleanup runs on the override branch — same as the existing `finishApproved` and `finishHumanReview` paths.

10. **Update the repo-root `CHANGELOG.md`** at `/workspace/CHANGELOG.md`. **First check whether `## Unreleased` exists** (`grep -n '^## Unreleased' /workspace/CHANGELOG.md`). If it does NOT exist (the file currently jumps straight to versioned sections like `## v0.35.0`), create it as the first section above the latest version section — that is the canonical location for in-flight entries. Then append a new bullet under `## Unreleased` (after the LsRemote seam bullet from prompt 1 and the post-check bullet from prompt 2, in that order). Use the prefix `feat(agent/github-releaser):` and describe the review-warning override behavior — DO NOT include the LsRemote seam (prompt 1) or the post-check verdict-upgrade (prompt 2); those are separate entries. Suggested wording:

    ```
    - feat(agent/github-releaser): ai_review step now records a `## Review Warning` block and closes the task as `completed` (frontmatter `status: completed` / `phase: done`) when a review rejection coincides with a confirmed remote tag at the agent's expected SHA (or at a different SHA — superseded mirror). The review verdict (the `## Review` section with `Approved=false`) is preserved durably on the task body for the operator audit trail. When the remote is empty or the LsRemote query errors, the existing `human_review` path stands unchanged. The new branch is a sub-decision on the `!approved` path inside `Run`; the `result.Outcome != "released"` short-circuit and the `Approved=true` happy path are unchanged. The factory wiring is unchanged (spec 064)
    ```

    Place this at the top of `## Unreleased` (after the two earlier spec-064 bullets, which go first because they shipped earlier in the release cycle). Match the existing reverse-chronological order.

11. **No factory changes**. `pkg/factory/factory.go` does NOT need to change — `NewAIReviewStep(client, runner, ops, ghToken)` still takes the same `git.GitOps` interface, and the new `LsRemote` method is part of the same interface. Verify with `cd /workspace/agent/github-releaser && go build ./...` — if it fails, the prompt is wrong; do not modify the factory.

12. **No agent-lib API changes**. Do NOT add a `CompleteCommand` or `CloseCommand`. The verdict change rides on `md.Frontmatter` map writes (`status`, `phase`) and `md.ReplaceSection` of the new `## Review Warning` block. The existing `## Review` section is preserved (not replaced) — the operator sees BOTH the rejected verdict and the override reason.

</requirements>

<constraints>
- **Do NOT commit** — dark-factory handles git
- The new branch is a SUB-DECISION on the existing `!approved` path. Do NOT replace the existing `finishHumanReview` call. Do NOT change the existing `Approved=true` happy path. Do NOT change the existing `result.Outcome != "released"` short-circuit. The change is purely additive inside the `if !output.Approved { ... }` block.
- The review verdict (the `## Review` section with `Approved=false` and `FailedChecks` populated) is PRESERVED on disk. The `## Review Warning` block is APPENDED alongside it — not a replacement. The operator must see both: the rejected verdict (so they know which checks failed) and the override reason (so they know why the task closed anyway).
- The `LsRemote` call in `checkReviewOverride` reuses the same auth model the execution step uses (HTTPS clone URL with installation token injected by `s.injectToken` from step 2). No new token scope, no new secret mount.
- The error string from `LsRemote` is passed through `redactToken` before being logged (mirror the post-check from prompt 2 — same security property).
- All existing passing tests under `agent/github-releaser/pkg/...` continue to pass — including the existing ai_review tests that exercise `Approved=false` paths (the `human_review` outcome).
- The factory wiring is unchanged.
- **No Prometheus metrics** for the override outcome. The spec's Non-goals explicitly forbid this — "log lines are the only observability surface in this spec; metrics are deferred until a concrete consumer demands them".
- **No opt-out flag, config knob, or tunable threshold** for the override behavior. This is a correctness fix, not a feature.

</constraints>

<verification>
```
cd /workspace/agent/github-releaser && go build ./...
```
Expected: exit code 0. The new `ReviewWarningOutput` type, the new `checkReviewOverride` + `finishReviewOverride` methods, the new `## Review Warning` section write, and (if step 2 took the url_helpers.go extraction shape) the package-level helpers all compile.

```
cd /workspace/agent/github-releaser && go vet ./...
```
Expected: exit code 0. No new vet warnings.

```
cd /workspace/agent/github-releaser && make test
```
Expected: exit code 0. All existing tests + new review-warning tests pass. The new test count: at least 6 `It` blocks under `Context("review-warning override (spec 064)", ...)` (remote-matches, remote-differs, remote-empty, ls-remote-error, happy-path-unchanged, short-circuit-unchanged).

```
cd /workspace/agent/github-releaser && make precommit
```
Expected: exit code 0. Format, generate, test, lint, license, gosec all pass.

```
cd /workspace && grep -n "Review Warning" CHANGELOG.md
```
Expected: ≥1 line under `## Unreleased` mentioning `Review Warning` (or `review-warning`).

```
cd /workspace && grep -n "ReviewWarningOutput" agent/github-releaser/pkg/review_warning_output.go
```
Expected: ≥1 hit, the new typed contract file exists at the named path.

```
cd /workspace && grep -nE "checkReviewOverride|finishReviewOverride" agent/github-releaser/pkg/steps_ai_review.go
```
Expected: ≥2 hits — the helper definitions + at least one call site.

```
cd /workspace/agent/github-releaser && go test -run TestSuite -v -ginkgo.focus="review-warning override" ./pkg/...
```
Expected: exit code 0. The new `Context("review-warning override (spec 064)", ...)` block's tests run and pass.

```
cd /workspace/agent/github-releaser && go test -run TestSuite -v -ginkgo.focus="AIReviewStep" ./pkg/...
```
Expected: exit code 0. All pre-existing `Describe("AIReviewStep", ...)` tests still pass — the change is purely additive and the existing Approved-false paths still flow through `finishHumanReview` when the new branch returns nil.

</verification>

<success_criteria>
- AC 5 (partial, ai_review side): The post-check helper in `steps_ai_review.go` participates in the existing `!approved` path. Evidence: Ginkgo integration test under `pkg/steps_ai_review_test.go` exercises the new `checkReviewOverride` branch and asserts the override fires (a faked `LsRemote` is invoked exactly once on the `!approved` path).
- AC 12: All existing `agent/github-releaser/pkg/...` Ginkgo tests continue to pass — including the pre-existing ai_review tests that drive `Approved=false` outcomes through `finishHumanReview` (the new branch is a sub-decision that returns `nil` on the empty/error cases, falling through to the existing `human_review` path). Evidence: `make test` in `agent/github-releaser/` exits 0.
- AC 13: `make precommit` in `agent/github-releaser/` exits 0. Evidence: exit code.
- AC 14: `CHANGELOG.md` in repo root has a new `## Unreleased` entry describing the review-warning override behavior. Evidence: `grep -n 'Review Warning' /workspace/CHANGELOG.md` returns a line under `## Unreleased`.

(Spec AC 6-11 are covered by prompt 2 — the post-check from prompt 2 is the primary verdict-upgrade path. This prompt covers the narrower ai_review-side override for the case where the post-check did NOT fire at execution time but the remote shows the tag at ai_review time.)

</success_criteria>
